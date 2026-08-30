package doubaorealtimeduplex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/toolrun"
)

// Transformer is a realtime-only transformer backed by the Doubao
// Realtime Duplex API. Client-side push-to-talk turns are handled by
// DoubaoRealtime, not this Duplex API.
type Transformer struct {
	client           *doubaospeech.Client
	duplex           doubaoRealtimeDuplexOpener
	sessionID        string
	model            string
	instructions     string
	inputFormat      string
	inputSampleRate  int
	inputChannels    int
	inputTranscode   bool
	outputFormat     string
	outputSampleRate int
	outputVoice      string
	outputSpeed      *int
	outputLoudness   *int
	toolInvoker      genx.ToolInvoker
	maxToolCalls     int
	extension        *doubaospeech.RealtimeDuplexExtension
}

var _ genx.Transformer = (*Transformer)(nil)

const (
	doubaoRealtimeDuplexTranscriptLabel = "transcript"
	doubaoRealtimeDuplexAssistantLabel  = "assistant"
	doubaoRealtimeDuplexInterrupted     = "interrupted"

	doubaoRealtimeDuplexFixedInputFormat      = "speech_opus"
	doubaoRealtimeDuplexFixedInputSampleRate  = 16000
	doubaoRealtimeDuplexFixedInputChannels    = 1
	doubaoRealtimeDuplexFixedOutputFormat     = "ogg_opus"
	doubaoRealtimeDuplexFixedOutputSampleRate = 24000
	doubaoRealtimeDuplexAudioKeepalive        = 20 * time.Millisecond
	doubaoRealtimeDuplexAudioIdleGrace        = 100 * time.Millisecond
)

const doubaoRealtimeTranscriptLabel = doubaoRealtimeDuplexTranscriptLabel

type doubaoRealtimeDuplexOpener interface {
	OpenSession(context.Context, *doubaospeech.RealtimeDuplexConfig) (doubaoRealtimeDuplexSession, error)
}

type doubaoRealtimeDuplexSession interface {
	LogID() string
	SendAudio(context.Context, []byte) error
	CancelResponse(context.Context) error
	SendFunctionCallOutputs(context.Context, ...doubaospeech.RealtimeDuplexFunctionCallOutput) error
	Recv() iter.Seq2[*doubaospeech.RealtimeDuplexEvent, error]
	Close() error
}

type doubaoRealtimeDuplexClient struct {
	client *doubaospeech.Client
}

func (c doubaoRealtimeDuplexClient) OpenSession(ctx context.Context, cfg *doubaospeech.RealtimeDuplexConfig) (doubaoRealtimeDuplexSession, error) {
	if c.client == nil {
		return nil, fmt.Errorf("doubao realtime duplex client is required")
	}
	return c.client.RealtimeDuplex.OpenSession(ctx, cfg)
}

// option is a functional option for Transformer.
type option func(*Transformer)

// withSpeaker sets the Duplex output voice.
func withSpeaker(speaker string) option {
	return func(t *Transformer) {
		t.outputVoice = speaker
	}
}

// withFormat sets the Duplex output audio format.
func withFormat(format string) option {
	return func(t *Transformer) {
		t.outputFormat = format
	}
}

// withSampleRate sets the Duplex output sample rate.
func withSampleRate(sampleRate int) option {
	return func(t *Transformer) {
		t.outputSampleRate = sampleRate
	}
}

// withInputFormat sets the audio format sent to Doubao.
func withInputFormat(format string) option {
	return func(t *Transformer) {
		t.inputFormat = format
	}
}

// withInputSampleRate sets the input audio sample rate sent to Doubao.
func withInputSampleRate(sampleRate int) option {
	return func(t *Transformer) {
		t.inputSampleRate = sampleRate
	}
}

// withInputChannels sets the local input audio channel count used for transcoding.
func withInputChannels(channels int) option {
	return func(t *Transformer) {
		t.inputChannels = channels
	}
}

// withInputTranscode forces input audio through the local codec
// before sending it to Doubao. This keeps network transport compressed while
// normalizing peer Opus packets to Doubao's expected speech_opus settings.
func withInputTranscode(enabled bool) option {
	return func(t *Transformer) {
		t.inputTranscode = enabled
	}
}

// withModel sets the upstream Duplex model version.
func withModel(model string) option {
	return func(t *Transformer) {
		t.model = model
	}
}

func withSessionID(sessionID string) option {
	return func(t *Transformer) {
		t.sessionID = sessionID
	}
}

func withInstructions(instructions string) option {
	return func(t *Transformer) {
		t.instructions = instructions
	}
}

func withOutputSpeed(speed int) option {
	return func(t *Transformer) {
		t.outputSpeed = &speed
	}
}

func withOutputLoudness(loudness int) option {
	return func(t *Transformer) {
		t.outputLoudness = &loudness
	}
}

func withToolInvoker(invoker genx.ToolInvoker) option {
	return func(t *Transformer) {
		t.toolInvoker = invoker
	}
}

func withMaxToolCalls(maximum int) option {
	return func(t *Transformer) {
		t.maxToolCalls = maximum
	}
}

func withExtension(extension *doubaospeech.RealtimeDuplexExtension) option {
	return func(t *Transformer) {
		t.extension = extension
	}
}

func withDoubaoRealtimeDuplexOpener(opener doubaoRealtimeDuplexOpener) option {
	return func(t *Transformer) {
		t.duplex = opener
	}
}

func newTransformer(client *doubaospeech.Client, opts ...option) *Transformer {
	t := &Transformer{
		client:           client,
		inputFormat:      doubaoRealtimeDuplexFixedInputFormat,
		inputSampleRate:  doubaoRealtimeDuplexFixedInputSampleRate,
		inputChannels:    doubaoRealtimeDuplexFixedInputChannels,
		inputTranscode:   true,
		outputFormat:     doubaoRealtimeDuplexFixedOutputFormat,
		outputSampleRate: doubaoRealtimeDuplexFixedOutputSampleRate,
		outputVoice:      "zh_female_vv_jupiter_bigtts",
	}
	for _, opt := range opts {
		opt(t)
	}
	if t.duplex == nil {
		t.duplex = doubaoRealtimeDuplexClient{client: client}
	}
	return t
}

// Transform converts audio input to audio output via realtime dialogue.
// It returns the output stream immediately and reports connection errors on it.
func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	source, err := t.transform(ctx, input)
	if err != nil {
		return nil, err
	}
	return streamkit.NewResponseStream(source)
}

func (t *Transformer) transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.client == nil && t.duplex == nil {
		return nil, fmt.Errorf("doubao realtime duplex: transformer is not initialized")
	}
	if input == nil {
		return nil, fmt.Errorf("doubao realtime duplex: input stream is required")
	}
	tools, err := resolveDoubaoRealtimeDuplexTools(ctx, t.toolInvoker)
	if err != nil {
		return nil, err
	}
	config := t.realtimeConfig(tools)
	slog.InfoContext(ctx,
		"doubao: realtime duplex session config",
		"model", config.Session.Model,
		"inputFormat", config.Session.Audio.Input.Format.Type,
		"inputSampleRate", config.Session.Audio.Input.Format.Rate,
		"inputTranscode", t.inputTranscode,
		"inputMode", "realtime",
		"outputFormat", config.Session.Audio.Output.Format.Type,
		"outputSampleRate", config.Session.Audio.Output.Format.Rate,
		"outputVoice", config.Session.Audio.Output.Voice,
		"tools", len(config.Session.Tools),
	)

	output := newBufferStream(16)
	go t.sessionLoop(ctx, input, output, tools, toolrun.New(t.toolInvoker, t.maxToolCalls))

	return output, nil
}

func (t *Transformer) realtimeConfig(
	tools []doubaospeech.RealtimeDuplexFunctionTool,
) *doubaospeech.RealtimeDuplexConfig {
	config := &doubaospeech.RealtimeDuplexConfig{
		Session: doubaospeech.RealtimeDuplexSessionConfig{
			ID:           strings.TrimSpace(t.sessionID),
			Model:        strings.TrimSpace(t.model),
			Instructions: t.instructions,
			Audio: doubaospeech.RealtimeDuplexAudioConfig{
				Input: doubaospeech.RealtimeDuplexAudioInputConfig{
					Format: doubaospeech.RealtimeDuplexAudioFormat{
						Type: doubaoRealtimeDuplexAudioFormat(t.inputFormat),
						Rate: doubaoRealtimeDuplexAudioSampleRate(t.inputSampleRate),
					},
				},
				Output: doubaospeech.RealtimeDuplexAudioOutputConfig{
					Format: doubaospeech.RealtimeDuplexAudioFormat{
						Type: doubaoRealtimeDuplexAudioFormat(t.outputFormat),
						Rate: doubaoRealtimeDuplexAudioSampleRate(t.outputSampleRate),
					},
					Voice: strings.TrimSpace(t.outputVoice),
				},
			},
			Tools: append([]doubaospeech.RealtimeDuplexFunctionTool(nil), tools...),
		},
		Extension: t.extension,
	}
	if t.outputSpeed != nil {
		config.Session.Audio.Output.Speed = *t.outputSpeed
	}
	if t.outputLoudness != nil {
		config.Session.Audio.Output.Loudness = *t.outputLoudness
	}
	return config
}

func (t *Transformer) sessionLoop(
	ctx context.Context,
	input genx.Stream,
	output *bufferStream,
	tools []doubaospeech.RealtimeDuplexFunctionTool,
	toolState *toolrun.State,
) {
	defer output.Close()
	inputReader := newDoubaoRealtimeDuplexInputReader(input)
	defer inputReader.Close()
	var pending *genx.MessageChunk
	for {
		if err := ctx.Err(); err != nil {
			output.CloseWithError(err)
			return
		}
		config := t.realtimeConfig(tools)
		session, err := t.duplex.OpenSession(ctx, config)
		if err != nil {
			output.CloseWithError(fmt.Errorf("doubao realtime duplex open session: %w", err))
			return
		}
		next, err := t.processLoop(
			ctx,
			withDoubaoRealtimeDuplexPendingChunk(inputReader, pending),
			output,
			session,
			toolState,
		)
		if err != nil {
			output.CloseWithError(err)
			return
		}
		if next == nil {
			return
		}
		pending = next
	}
}

func (t *Transformer) processLoop(
	ctx context.Context,
	input doubaoRealtimeDuplexSessionInput,
	output *bufferStream,
	session doubaoRealtimeDuplexSession,
	toolState *toolrun.State,
) (*genx.MessageChunk, error) {
	defer session.Close()
	var restarting atomic.Bool
	assistant := newRealtimeAssistantLifecycle()

	markAssistantStarted := func(streamID string) uint64 {
		return assistant.markStarted(streamID)
	}
	output.setOutputObserver(func(chunk *genx.MessageChunk) {
		observeRealtimeAssistantOutput(assistant, doubaoRealtimeDuplexAssistantLabel, chunk)
	})
	defer output.setOutputObserver(nil)
	interruptAssistantState := func(streamID string) (bool, error) {
		return t.interruptAssistantOutput(output, assistant, streamID)
	}
	interruptAssistant := func(streamID string) (bool, error) {
		interrupted, err := interruptAssistantState(streamID)
		if err != nil {
			return interrupted, err
		}
		if !interrupted {
			return false, nil
		}
		if err := session.CancelResponse(ctx); err != nil {
			return true, fmt.Errorf("doubao realtime duplex cancel response: %w", err)
		}
		return true, nil
	}
	pushAssistantOutput := func(epoch uint64, chunk *genx.MessageChunk) error {
		_, err := assistant.pushIfCurrent(epoch, chunk, func() error {
			return output.Push(chunk)
		})
		return err
	}
	streamIDs := newDoubaoRealtimeDuplexStreamIDs()
	audioStarted := false
	audioStartedStreamID := ""
	startAudioOutput := func(epoch uint64, streamID string) error {
		if audioStarted && audioStartedStreamID == streamID {
			return nil
		}
		audioStarted = true
		audioStartedStreamID = streamID
		markAssistantStarted(streamID)
		return pushAssistantOutput(epoch, &genx.MessageChunk{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: t.outputMIMEType()},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, BeginOfStream: true},
		})
	}

	eventsDone := make(chan struct{})
	eventsErr := make(chan error, 1)
	finishEventError := func(err error) {
		if err == nil {
			return
		}
		output.CloseWithError(err)
		_ = input.CloseWithError(err)
		select {
		case eventsErr <- err:
		default:
		}
	}
	eventError := func() error {
		select {
		case err := <-eventsErr:
			return err
		default:
			return nil
		}
	}
	go func() {
		lastTranscriptText := ""
		transcriptOpen := false
		eventsFinished := false
		routeProduced := false
		textDeltaSeen := make(map[string]bool)
		assistantTextStarted := make(map[string]bool)
		assistantTextDone := make(map[string]bool)
		assistantAudioStarted := make(map[string]bool)
		assistantAudioDone := make(map[string]bool)
		assistantCompleted := make(map[string]bool)
		completeAssistantStream := func(streamID string) {
			assistantCompleted[streamID] = true
			if !assistantTextStarted[streamID] {
				assistant.markRouteDoneStream(streamID, true)
			}
			if !assistantAudioStarted[streamID] {
				assistant.markRouteDoneStream(streamID, false)
			}
		}
		openInputSegment := func() error {
			if transcriptOpen {
				return nil
			}
			if err := output.Push(&genx.MessageChunk{
				Role: genx.RoleUser,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{
					StreamID:      streamIDs.input(),
					Label:         doubaoRealtimeDuplexTranscriptLabel,
					BeginOfStream: true,
				},
			}); err != nil {
				return err
			}
			routeProduced = true
			transcriptOpen = true
			return nil
		}
		closeInputSegment := func(errText string) error {
			if err := openInputSegment(); err != nil {
				return err
			}
			inputStreamID := streamIDs.endInputSegment()
			doneChunk := &genx.MessageChunk{
				Role: genx.RoleUser,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{
					StreamID:    inputStreamID,
					Label:       doubaoRealtimeDuplexTranscriptLabel,
					EndOfStream: true,
					Error:       errText,
				},
			}
			if err := output.Push(doneChunk); err != nil {
				return err
			}
			lastTranscriptText = ""
			transcriptOpen = false
			return nil
		}
		finishEvents := func() {
			if eventsFinished {
				return
			}
			eventsFinished = true
			if transcriptOpen {
				if err := closeInputSegment(""); err != nil {
					finishEventError(err)
				}
			}
			close(eventsDone)
		}
		finishProviderError := func(providerErr error) {
			if providerErr == nil {
				return
			}
			errText := providerErr.Error()
			if transcriptOpen {
				if err := closeInputSegment(errText); err != nil {
					finishEventError(err)
					return
				}
			}
			streamID := streamIDs.response()
			if streamID == "" {
				for candidate := range assistantTextStarted {
					streamID = candidate
					break
				}
			}
			if streamID == "" {
				for candidate := range assistantAudioStarted {
					streamID = candidate
					break
				}
			}
			for candidate, started := range assistantTextStarted {
				if started && !assistantTextDone[candidate] {
					if err := output.Push(&genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(""),
						Ctrl: &genx.StreamCtrl{StreamID: candidate, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true, Error: errText},
					}); err != nil {
						finishEventError(err)
						return
					}
					assistantTextDone[candidate] = true
					routeProduced = true
					streamID = candidate
				}
			}
			for candidate, started := range assistantAudioStarted {
				if started && !assistantAudioDone[candidate] {
					if err := output.Push(&genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: t.outputMIMEType()},
						Ctrl: &genx.StreamCtrl{StreamID: candidate, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true, Error: errText},
					}); err != nil {
						finishEventError(err)
						return
					}
					assistantAudioDone[candidate] = true
					routeProduced = true
					streamID = candidate
				}
			}
			if streamID != "" && !assistantCompleted[streamID] {
				if !assistantTextStarted[streamID] {
					for _, chunk := range []*genx.MessageChunk{
						{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, BeginOfStream: true}},
						{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true, Error: errText}},
					} {
						if err := output.Push(chunk); err != nil {
							finishEventError(err)
							return
						}
					}
					assistantTextStarted[streamID] = true
					assistantTextDone[streamID] = true
					routeProduced = true
				}
				if !assistantAudioStarted[streamID] {
					for _, chunk := range []*genx.MessageChunk{
						{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: t.outputMIMEType()}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, BeginOfStream: true}},
						{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: t.outputMIMEType()}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true, Error: errText}},
					} {
						if err := output.Push(chunk); err != nil {
							finishEventError(err)
							return
						}
					}
					assistantAudioStarted[streamID] = true
					assistantAudioDone[streamID] = true
					routeProduced = true
				}
			}
			if streamID != "" && assistantTextDone[streamID] && assistantAudioDone[streamID] {
				completeAssistantStream(streamID)
			}
			if routeProduced {
				_ = output.Close()
			}
			finishEventError(providerErr)
		}
		defer finishEvents()
		for event, err := range session.Recv() {
			if err != nil {
				err = doubaoRealtimeDuplexProviderErrorWithLogID(err, session.LogID())
				if restarting.Load() {
					slog.InfoContext(ctx, "doubao: realtime duplex session stopped for restart", "error", err)
					return
				}
				slog.ErrorContext(ctx, "doubao: recv error", "error", err)
				finishProviderError(err)
				return
			}

			slog.DebugContext(ctx, "doubao: received duplex event", "type", event.Type, "text", event.Text, "transcript", event.Transcript, "audioLen", len(event.Audio), "functionCalls", len(event.FunctionCalls))
			// Provider response and question identifiers are event-local protocol
			// metadata and may change between text, audio-start, audio-delta, and
			// audio-done events. The Transformer owns one stable GenX StreamID for
			// every input segment and all MIME routes generated for its response.
			streamID := firstNonEmptyString(streamIDs.response(), streamIDs.input())
			switch event.Type {
			case doubaospeech.RealtimeDuplexEventTranscriptionStarted:
				if err := openInputSegment(); err != nil {
					finishEventError(err)
					return
				}
			case doubaospeech.RealtimeDuplexEventTranscriptionDelta:
				text := firstNonEmptyString(event.Delta, event.Transcript)
				if text == "" {
					continue
				}
				// The provider may put either an incremental token or the full
				// current ASR hypothesis in Delta. Normalize both forms against the
				// text already emitted so cumulative hypotheses are not duplicated.
				text = realtimeDuplexTextDelta(lastTranscriptText, text)
				if text == "" {
					continue
				}
				if !transcriptOpen && !realtimeDuplexTextHasSemantic(text) {
					lastTranscriptText = ""
					continue
				}
				lastTranscriptText += text
				if err := openInputSegment(); err != nil {
					finishEventError(err)
					return
				}
				if err := output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Part: genx.Text(text),
					Ctrl: &genx.StreamCtrl{StreamID: streamIDs.input(), Label: doubaoRealtimeDuplexTranscriptLabel},
				}); err != nil {
					finishEventError(err)
					return
				}
			case doubaospeech.RealtimeDuplexEventTranscriptionCompleted:
				text := firstNonEmptyString(event.Transcript, event.Text, event.Delta)
				if text != "" {
					delta := realtimeDuplexTextDelta(lastTranscriptText, text)
					if delta != "" {
						if err := openInputSegment(); err != nil {
							finishEventError(err)
							return
						}
						if err := output.Push(&genx.MessageChunk{
							Role: genx.RoleUser,
							Part: genx.Text(delta),
							Ctrl: &genx.StreamCtrl{StreamID: streamIDs.input(), Label: doubaoRealtimeDuplexTranscriptLabel},
						}); err != nil {
							finishEventError(err)
							return
						}
					}
				}
				if transcriptOpen {
					if err := closeInputSegment(""); err != nil {
						finishEventError(err)
						return
					}
				}
				assistant.setAccept(true)
				assistant.nextEpoch()
			case doubaospeech.RealtimeDuplexEventTranscriptionFailed:
				errText := "transcription failed"
				if event.Error != nil && strings.TrimSpace(event.Error.Message) != "" {
					errText = event.Error.Message
				}
				if err := closeInputSegment(errText); err != nil {
					finishEventError(err)
					return
				}
			case doubaospeech.RealtimeDuplexEventInputAudioBufferCommitted:
				assistant.setAccept(true)
				assistant.nextEpoch()
				if transcriptOpen {
					if err := closeInputSegment(""); err != nil {
						finishEventError(err)
						return
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputTextDelta:
				if !assistant.acceptsOutput() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				text := event.Delta
				if strings.TrimSpace(text) == "" {
					continue
				}
				epoch := markAssistantStarted(streamID)
				if !assistantTextStarted[streamID] {
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(""),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, BeginOfStream: true},
					}); err != nil {
						finishEventError(err)
						return
					}
					assistantTextStarted[streamID] = true
					routeProduced = true
				}
				if err := pushAssistantOutput(epoch, &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(text),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel},
				}); err != nil {
					finishEventError(err)
					return
				}
				textDeltaSeen[streamID] = true
			case doubaospeech.RealtimeDuplexEventResponseOutputTextDone:
				if !assistant.acceptsOutput() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := assistant.currentEpoch()
				if !assistantTextStarted[streamID] {
					epoch = markAssistantStarted(streamID)
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(""),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, BeginOfStream: true},
					}); err != nil {
						finishEventError(err)
						return
					}
					assistantTextStarted[streamID] = true
					routeProduced = true
				}
				if event.Text != "" && !textDeltaSeen[streamID] {
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Text),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel},
					}); err != nil {
						finishEventError(err)
						return
					}
				}
				delete(textDeltaSeen, streamID)
				if err := pushAssistantOutput(epoch, &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true},
				}); err != nil {
					finishEventError(err)
					return
				}
				assistantTextDone[streamID] = true
				if assistantAudioDone[streamID] {
					completeAssistantStream(streamID)
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputAudioStarted:
				if !assistant.acceptsOutput() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := assistant.currentEpoch()
				if err := startAudioOutput(epoch, streamID); err != nil {
					finishEventError(err)
					return
				}
				assistantAudioStarted[streamID] = true
				routeProduced = true
			case doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta:
				if !assistant.acceptsOutput() || len(event.Audio) == 0 {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := assistant.currentEpoch()
				if err := startAudioOutput(epoch, streamID); err != nil {
					finishEventError(err)
					return
				}
				assistantAudioStarted[streamID] = true
				routeProduced = true
				blobs, err := t.outputAudioBlobs(event.Audio)
				if err != nil {
					finishEventError(err)
					return
				}
				for _, blob := range blobs {
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: blob,
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel},
					}); err != nil {
						finishEventError(err)
						return
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputAudioDone:
				if !assistant.acceptsOutput() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := assistant.currentEpoch()
				if audioStarted {
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: t.outputMIMEType()},
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true},
					}); err != nil {
						finishEventError(err)
						return
					}
				}
				audioStarted = false
				audioStartedStreamID = ""
				assistantAudioDone[streamID] = true
				if assistantTextDone[streamID] {
					completeAssistantStream(streamID)
				}
			case doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone:
				for _, call := range event.FunctionCalls {
					result, err := toolState.Invoke(ctx, genx.ToolCall{
						ID: call.CallID,
						FuncCall: &genx.FuncCall{
							Name:      call.Name,
							Arguments: call.Arguments,
						},
					})
					if err != nil {
						finishEventError(fmt.Errorf("doubao realtime duplex invoke tool: %w", err))
						return
					}
					if err := session.SendFunctionCallOutputs(ctx, doubaospeech.RealtimeDuplexFunctionCallOutput{
						CallID: result.ID,
						Output: result.Result,
					}); err != nil {
						finishEventError(fmt.Errorf(
							"doubao realtime duplex submit tool result: %w",
							err,
						))
						return
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseCanceled:
				completeAssistantStream(streamID)
				assistant.setAccept(false)
			case doubaospeech.RealtimeDuplexEventResponseDone:
				completeAssistantStream(streamID)
			case doubaospeech.RealtimeDuplexEventSessionClosed:
				slog.InfoContext(ctx, "doubao: realtime duplex session closed")
				// Signal the send loop before Recv performs any iterator cleanup.
				// Otherwise input that arrives after SessionClosed can still be
				// sent to the provider session that has already closed.
				finishEvents()
				return
			case doubaospeech.RealtimeDuplexEventError:
				err := fmt.Errorf("doubao realtime duplex event error")
				if event.Error != nil {
					err = event.Error
				}
				err = doubaoRealtimeDuplexProviderErrorWithLogID(err, session.LogID())
				finishProviderError(err)
				return
			}
		}
	}()

	slog.InfoContext(ctx, "doubao: starting audio send loop")

	// Send audio to realtime service
	audioSent := 0
	keepaliveInput := newDoubaoRealtimeDuplexAudioInput(
		t.inputFormat,
		t.inputSampleRate,
		t.inputChannels,
		false,
	)
	defer keepaliveInput.close()
	keepaliveFrames, err := keepaliveInput.keepaliveFrames(1)
	if err != nil {
		return nil, fmt.Errorf("doubao realtime duplex prepare audio keepalive: %w", err)
	}
	if len(keepaliveFrames) != 1 || len(keepaliveFrames[0]) == 0 {
		return nil, fmt.Errorf("doubao realtime duplex prepare audio keepalive: empty silence frame")
	}
	keepaliveAudio := keepaliveFrames[0]
	keepalive := time.NewTimer(doubaoRealtimeDuplexAudioIdleGrace)
	defer keepalive.Stop()
	inputRouteID := ""
	inputAudioEnded := false
	audioInputs := newDoubaoRealtimeDuplexAudioInputs(
		t.inputFormat,
		t.inputSampleRate,
		t.inputChannels,
		t.inputTranscode,
	)
	defer audioInputs.close()
	for {
		var chunk *genx.MessageChunk
		var err error
		var done, sendKeepalive bool
		chunk, err, done, sendKeepalive = input.nextOrDoneOrKeepalive(eventsDone, keepalive.C)
		if done {
			if err := eventError(); err != nil {
				return nil, err
			}
			slog.InfoContext(ctx, "doubao: events done, waiting for next input")
			for {
				chunk, err := input.Next()
				if err != nil {
					if err != io.EOF && err != genx.ErrDone {
						slog.ErrorContext(ctx, "doubao: input error after events done", "error", err)
						return nil, err
					}
					slog.InfoContext(ctx, "doubao: input EOF after events done", "audioSent", audioSent)
					return nil, nil
				}
				if chunk != nil {
					if chunk.IsBeginOfStream() && chunk.Ctrl != nil {
						interruptAssistantState(chunk.Ctrl.StreamID)
					}
					return chunk.Clone(), nil
				}
			}
		}
		if sendKeepalive {
			if err := session.SendAudio(ctx, keepaliveAudio); err != nil {
				return nil, fmt.Errorf("doubao realtime duplex send audio keepalive: %w", err)
			}
			audioSent++
			resetDoubaoRealtimeDuplexKeepalive(keepalive, doubaoRealtimeDuplexAudioKeepalive)
			continue
		}
		if err != nil {
			if err != io.EOF && err != genx.ErrDone {
				slog.ErrorContext(ctx, "doubao: input error", "error", err)
				return nil, err
			} else {
				slog.InfoContext(ctx, "doubao: input EOF", "audioSent", audioSent)
			}
			// Wait for remaining events
			<-eventsDone
			if err := eventError(); err != nil {
				return nil, err
			}
			return nil, nil
		}

		if chunk == nil {
			continue
		}

		// A control BOS starts the StreamID route. A MIME-bearing BOS may either
		// start that route directly or declare its audio channel after the
		// control BOS; in both cases its payload still belongs to the channel.
		if chunk.IsBeginOfStream() && chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
			streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
			if streamID != inputRouteID {
				interrupted, err := interruptAssistant(streamID)
				if err != nil {
					return nil, err
				}
				if interrupted {
					slog.InfoContext(ctx, "doubao: restarting realtime session after interrupt", "streamID", streamID)
					restarting.Store(true)
					return chunk.Clone(), nil
				}
				streamIDs.beginInput(streamID)
				inputRouteID = streamID
				inputAudioEnded = false
				slog.InfoContext(ctx, "doubao: received route BOS", "streamID", streamID)
			}
			if chunk.Part == nil && !chunk.IsEndOfStream() {
				continue
			}
		}

		// Duplex uses server-side turn detection. Audio-channel or route EOS
		// only closes the local stream boundary; it must not commit audio. An
		// EOS may carry final data, so close only after processing its payload.
		audioEOS := realtimeAudioInputEOS(chunk)

		// Send based on part type
		switch p := chunk.Part.(type) {
		case *genx.Blob:
			// Send audio blob
			streamID := streamIDs.serviceInput(chunk)
			audioInput, err := audioInputs.streamForBlob(streamID, p)
			if err != nil {
				slog.ErrorContext(ctx, "doubao: prepare audio error", "error", err)
				boundaryErr := t.pushInputEOSError(output, streamID, err)
				audioInputs.closeStream(streamID)
				return nil, errors.Join(err, boundaryErr)
			}
			if len(p.Data) > 0 {
				frames, err := audioInput.prepareFrames(p)
				if err != nil {
					slog.ErrorContext(ctx, "doubao: prepare audio error", "error", err)
					boundaryErr := t.pushInputEOSError(output, streamID, err)
					audioInputs.closeStream(streamID)
					return nil, errors.Join(err, boundaryErr)
				}
				for _, audio := range frames {
					if len(audio) == 0 {
						continue
					}
					audioSent++
					if audioSent%50 == 1 { // Log every 50 chunks (1 second at 20ms chunks)
						slog.DebugContext(ctx, "doubao: sending audio chunk", "streamID", streamID, "len", len(audio), "mime", p.MIMEType, "inputFormat", audioInput.format, "totalSent", audioSent)
					}
					if err := session.SendAudio(ctx, audio); err != nil {
						slog.ErrorContext(ctx, "doubao: send audio error", "error", err)
						return nil, err
					}
				}
				resetDoubaoRealtimeDuplexKeepalive(keepalive, doubaoRealtimeDuplexAudioIdleGrace)
			}
		case genx.Text:
			if len(p) > 0 {
				return nil, fmt.Errorf("doubao realtime duplex does not accept text input")
			}
		}
		if audioEOS && !inputAudioEnded {
			streamID := streamIDs.serviceInput(chunk)
			slog.DebugContext(ctx, "doubao: received realtime EOS, closing local audio stream without commit", "streamID", streamID, "audioSent", audioSent)
			audioInputs.closeStream(streamID)
			inputAudioEnded = true
		}
		if audioEOS && chunk.Part == nil && chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.StreamID) == inputRouteID {
			inputRouteID = ""
		}
	}
}

func resetDoubaoRealtimeDuplexKeepalive(timer *time.Timer, delay time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(delay)
}

func isDoubaoRealtimeDuplexAssistantChunk(chunk *genx.MessageChunk, streamID string) bool {
	return chunk != nil && chunk.Role == genx.RoleModel && chunk.Ctrl != nil &&
		chunk.Ctrl.StreamID == streamID && chunk.Ctrl.Label == doubaoRealtimeDuplexAssistantLabel
}

func (t *Transformer) interruptAssistantOutput(
	output *bufferStream,
	assistant *realtimeAssistantLifecycle,
	streamID string,
) (bool, error) {
	if output == nil || assistant == nil {
		return false, nil
	}
	interruption := assistant.interruptRoutes(streamID, false)
	if !interruption.Interrupted {
		return false, nil
	}
	discarded := output.discardChunks(func(chunk *genx.MessageChunk) bool {
		return isDoubaoRealtimeDuplexAssistantChunk(chunk, interruption.StreamID)
	})
	for _, chunk := range discarded {
		if !chunk.IsBeginOfStream() {
			continue
		}
		switch chunk.Part.(type) {
		case genx.Text:
			interruption.TextStarted = false
		case *genx.Blob:
			interruption.AudioStarted = false
		}
	}
	if interruption.TextOpen {
		if !interruption.TextStarted {
			if err := output.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{StreamID: interruption.StreamID, Label: doubaoRealtimeDuplexAssistantLabel, BeginOfStream: true},
			}); err != nil {
				return true, fmt.Errorf("doubao realtime duplex emit interrupted text BOS: %w", err)
			}
		}
		if err := output.Push(&genx.MessageChunk{
			Role: genx.RoleModel,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: interruption.StreamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true, Error: doubaoRealtimeDuplexInterrupted},
		}); err != nil {
			return true, fmt.Errorf("doubao realtime duplex emit interrupted text EOS: %w", err)
		}
	}
	if interruption.AudioOpen {
		if !interruption.AudioStarted {
			if err := output.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: &genx.Blob{MIMEType: t.outputMIMEType()},
				Ctrl: &genx.StreamCtrl{StreamID: interruption.StreamID, Label: doubaoRealtimeDuplexAssistantLabel, BeginOfStream: true},
			}); err != nil {
				return true, fmt.Errorf("doubao realtime duplex emit interrupted audio BOS: %w", err)
			}
		}
		if err := output.Push(&genx.MessageChunk{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: t.outputMIMEType()},
			Ctrl: &genx.StreamCtrl{StreamID: interruption.StreamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true, Error: doubaoRealtimeDuplexInterrupted},
		}); err != nil {
			return true, fmt.Errorf("doubao realtime duplex emit interrupted audio EOS: %w", err)
		}
	}
	return true, nil
}

type doubaoRealtimeDuplexPendingChunkStream struct {
	first *genx.MessageChunk
	rest  doubaoRealtimeDuplexSessionInput
}

func withDoubaoRealtimeDuplexPendingChunk(
	rest doubaoRealtimeDuplexSessionInput,
	first *genx.MessageChunk,
) doubaoRealtimeDuplexSessionInput {
	if first == nil {
		return rest
	}
	return &doubaoRealtimeDuplexPendingChunkStream{first: first, rest: rest}
}

func (s *doubaoRealtimeDuplexPendingChunkStream) Next() (*genx.MessageChunk, error) {
	if s.first != nil {
		chunk := s.first
		s.first = nil
		return chunk, nil
	}
	return s.rest.Next()
}

func (s *doubaoRealtimeDuplexPendingChunkStream) NextOrDone(done <-chan struct{}) (*genx.MessageChunk, error, bool) {
	if s.first != nil {
		select {
		case <-done:
			return nil, nil, true
		default:
		}
		chunk := s.first
		s.first = nil
		return chunk, nil, false
	}
	return s.rest.NextOrDone(done)
}

func (s *doubaoRealtimeDuplexPendingChunkStream) nextOrDoneOrKeepalive(
	done <-chan struct{},
	keepalive <-chan time.Time,
) (*genx.MessageChunk, error, bool, bool) {
	if s.first != nil {
		select {
		case <-done:
			return nil, nil, true, false
		default:
		}
		chunk := s.first
		s.first = nil
		return chunk, nil, false, false
	}
	return s.rest.nextOrDoneOrKeepalive(done, keepalive)
}

func (s *doubaoRealtimeDuplexPendingChunkStream) Close() error {
	return s.rest.Close()
}

func (s *doubaoRealtimeDuplexPendingChunkStream) CloseWithError(err error) error {
	return s.rest.CloseWithError(err)
}

type doubaoRealtimeDuplexSessionInput interface {
	genx.Stream
	NextOrDone(<-chan struct{}) (*genx.MessageChunk, error, bool)
	nextOrDoneOrKeepalive(<-chan struct{}, <-chan time.Time) (*genx.MessageChunk, error, bool, bool)
}

type doubaoRealtimeDuplexInputResult struct {
	chunk *genx.MessageChunk
	err   error
}

type doubaoRealtimeDuplexInputReader struct {
	source    genx.Stream
	results   chan doubaoRealtimeDuplexInputResult
	done      chan struct{}
	pending   *doubaoRealtimeDuplexInputResult
	closeOnce sync.Once
}

func newDoubaoRealtimeDuplexInputReader(source genx.Stream) *doubaoRealtimeDuplexInputReader {
	reader := &doubaoRealtimeDuplexInputReader{
		source:  source,
		results: make(chan doubaoRealtimeDuplexInputResult, 1),
		done:    make(chan struct{}),
	}
	go reader.read()
	return reader
}

func (r *doubaoRealtimeDuplexInputReader) read() {
	defer close(r.results)
	for {
		chunk, err := r.source.Next()
		result := doubaoRealtimeDuplexInputResult{chunk: chunk, err: err}
		select {
		case r.results <- result:
		case <-r.done:
			return
		}
		if err != nil {
			return
		}
	}
}

func (r *doubaoRealtimeDuplexInputReader) Next() (*genx.MessageChunk, error) {
	if r.pending != nil {
		result := *r.pending
		r.pending = nil
		return result.chunk, result.err
	}
	result, ok := <-r.results
	if !ok {
		return nil, io.EOF
	}
	return result.chunk, result.err
}

func (r *doubaoRealtimeDuplexInputReader) NextOrDone(done <-chan struct{}) (*genx.MessageChunk, error, bool) {
	if r.pending != nil {
		select {
		case <-done:
			return nil, nil, true
		default:
		}
		result := *r.pending
		r.pending = nil
		return result.chunk, result.err, false
	}
	select {
	case <-done:
		return nil, nil, true
	default:
	}
	select {
	case result, ok := <-r.results:
		if !ok {
			return nil, io.EOF, false
		}
		select {
		case <-done:
			r.pending = &result
			return nil, nil, true
		default:
		}
		return result.chunk, result.err, false
	default:
	}
	select {
	case <-done:
		return nil, nil, true
	default:
	}
	select {
	case <-done:
		return nil, nil, true
	case result, ok := <-r.results:
		if !ok {
			return nil, io.EOF, false
		}
		select {
		case <-done:
			r.pending = &result
			return nil, nil, true
		default:
		}
		return result.chunk, result.err, false
	}
}

func (r *doubaoRealtimeDuplexInputReader) nextOrDoneOrKeepalive(
	done <-chan struct{},
	keepalive <-chan time.Time,
) (*genx.MessageChunk, error, bool, bool) {
	if r.pending != nil {
		select {
		case <-done:
			return nil, nil, true, false
		default:
		}
		result := *r.pending
		r.pending = nil
		return result.chunk, result.err, false, false
	}
	select {
	case <-done:
		return nil, nil, true, false
	default:
	}
	select {
	case result, ok := <-r.results:
		if !ok {
			return nil, io.EOF, false, false
		}
		select {
		case <-done:
			r.pending = &result
			return nil, nil, true, false
		default:
		}
		return result.chunk, result.err, false, false
	default:
	}
	select {
	case <-done:
		return nil, nil, true, false
	case <-keepalive:
		return nil, nil, false, true
	case result, ok := <-r.results:
		if !ok {
			return nil, io.EOF, false, false
		}
		select {
		case <-done:
			r.pending = &result
			return nil, nil, true, false
		default:
		}
		return result.chunk, result.err, false, false
	}
}

func (r *doubaoRealtimeDuplexInputReader) Close() error {
	return r.CloseWithError(io.EOF)
}

func (r *doubaoRealtimeDuplexInputReader) CloseWithError(err error) error {
	r.closeOnce.Do(func() {
		close(r.done)
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			_ = r.source.Close()
			return
		}
		_ = r.source.CloseWithError(err)
	})
	return nil
}

func (t *Transformer) pushInputEOSError(output *bufferStream, streamID string, err error) error {
	if output == nil || err == nil {
		return nil
	}
	if pushErr := output.Push(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{
			StreamID:      streamID,
			Label:         doubaoRealtimeDuplexTranscriptLabel,
			BeginOfStream: true,
		},
	}); pushErr != nil {
		return fmt.Errorf("doubao realtime duplex emit input error BOS: %w", pushErr)
	}
	if pushErr := output.Push(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{
			StreamID:    streamID,
			Label:       doubaoRealtimeDuplexTranscriptLabel,
			EndOfStream: true,
			Error:       err.Error(),
		},
	}); pushErr != nil {
		return fmt.Errorf("doubao realtime duplex emit input error EOS: %w", pushErr)
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func doubaoRealtimeDuplexProviderErrorWithLogID(err error, logID string) error {
	logID = strings.TrimSpace(logID)
	if err == nil || logID == "" {
		return err
	}
	var providerErr *doubaospeech.Error
	if !errors.As(err, &providerErr) {
		return err
	}
	if strings.TrimSpace(providerErr.LogID) == "" {
		providerErr.LogID = logID
	}
	// Return the provider error itself so its formatted metadata reflects the
	// newly attached handshake log ID. fmt-wrapped errors cache their message
	// when created and would otherwise continue to print an empty log_id.
	return providerErr
}

func resolveDoubaoRealtimeDuplexTools(
	ctx context.Context,
	invoker genx.ToolInvoker,
) ([]doubaospeech.RealtimeDuplexFunctionTool, error) {
	definitions, err := toolrun.ResolveTools(ctx, invoker)
	if err != nil {
		return nil, fmt.Errorf("doubao realtime duplex: %w", err)
	}
	tools := make([]doubaospeech.RealtimeDuplexFunctionTool, 0, len(definitions))
	for _, definition := range definitions {
		encoded, err := json.Marshal(definition.Argument)
		if err != nil {
			return nil, fmt.Errorf(
				"doubao realtime duplex: encode tool %q schema: %w",
				definition.Name,
				err,
			)
		}
		var parameters doubaospeech.RealtimeDuplexJSONSchema
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&parameters); err != nil {
			return nil, fmt.Errorf(
				"doubao realtime duplex: convert tool %q schema: %w",
				definition.Name,
				err,
			)
		}
		tools = append(tools, doubaospeech.RealtimeDuplexFunctionTool{
			Type:        "function",
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  &parameters,
		})
	}
	return tools, nil
}

func realtimeDuplexASRText(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	var decoded struct {
		Extra struct {
			OriginText               string `json:"origin_text"`
			SoftFinishParalinguistic *struct {
				ASRText string `json:"asr_text"`
			} `json:"soft_finish_paralinguistic"`
		} `json:"extra"`
		Results []struct {
			Alternatives []struct {
				Text string `json:"text"`
			} `json:"alternatives"`
		} `json:"results"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return ""
	}
	if decoded.Extra.SoftFinishParalinguistic != nil {
		if text := strings.TrimSpace(decoded.Extra.SoftFinishParalinguistic.ASRText); text != "" {
			return text
		}
	}
	if text := strings.TrimSpace(decoded.Extra.OriginText); text != "" {
		return text
	}
	for i := len(decoded.Results) - 1; i >= 0; i-- {
		alternatives := decoded.Results[i].Alternatives
		for j := len(alternatives) - 1; j >= 0; j-- {
			if text := strings.TrimSpace(alternatives[j].Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func realtimeDuplexTextDelta(previous, current string) string {
	if current == "" || current == previous {
		return ""
	}
	if previous != "" && strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	if previous != "" {
		if suffix, ok := realtimeDuplexTextSuffixAfterNormalizedPrefix(previous, current); ok {
			return suffix
		}
		previousNorm := realtimeDuplexNormalizeText(previous)
		currentNorm := realtimeDuplexNormalizeText(current)
		if previousNorm != "" && currentNorm != "" && strings.Contains(previousNorm, currentNorm) {
			return ""
		}
	}
	return current
}

func realtimeDuplexTextSuffixAfterNormalizedPrefix(previous, current string) (string, bool) {
	previousNorm := realtimeDuplexNormalizeText(previous)
	if previousNorm == "" {
		return current, true
	}
	matched := 0
	for i, r := range current {
		norm := realtimeDuplexNormalizeText(string(r))
		if norm == "" {
			continue
		}
		if matched >= len(previousNorm) || !strings.HasPrefix(previousNorm[matched:], norm) {
			return "", false
		}
		matched += len(norm)
		if matched == len(previousNorm) {
			return current[i+len(string(r)):], true
		}
	}
	return "", matched == len(previousNorm)
}

func realtimeDuplexNormalizeText(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '\u4e00' && r <= '\u9fff') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func realtimeDuplexTextHasSemantic(text string) bool {
	return realtimeDuplexNormalizeText(text) != ""
}

func realtimeDuplexASRResponseEndsSegment(event *doubaospeech.RealtimeEvent, delta string) bool {
	if event == nil || !realtimeDuplexTextHasSemantic(delta) {
		return false
	}
	for _, result := range event.Results {
		text := strings.TrimSpace(result.Text)
		if text == "" {
			text = strings.TrimSpace(event.Text)
		}
		if text == "" {
			text = strings.TrimSpace(delta)
		}
		if !result.IsInterim && realtimeDuplexTextHasSemantic(text) {
			return true
		}
	}
	if event.IsFinal {
		return true
	}
	return false
}

func (t *Transformer) mimeType() string {
	switch strings.ToLower(strings.TrimSpace(t.outputFormat)) {
	case "mp3":
		return "audio/mpeg"
	case "ogg_opus":
		return "audio/ogg"
	case "pcm", "pcm_s16le":
		return "audio/pcm"
	default:
		return "audio/pcm"
	}
}

func (t *Transformer) outputMIMEType() string {
	if strings.EqualFold(strings.TrimSpace(t.outputFormat), "ogg_opus") {
		return "audio/opus"
	}
	return t.mimeType()
}

func (t *Transformer) outputAudioBlobs(audio []byte) ([]*genx.Blob, error) {
	if len(audio) == 0 {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(t.outputFormat), "ogg_opus") {
		return []*genx.Blob{{MIMEType: t.mimeType(), Data: append([]byte(nil), audio...)}}, nil
	}
	var blobs []*genx.Blob
	for packet, err := range ogg.Packets(bytes.NewReader(audio)) {
		if err != nil {
			return nil, fmt.Errorf("extract doubao realtime ogg opus packets: %w", err)
		}
		if len(packet.Data) == 0 || codecconv.IsOpusHeadPacket(packet.Data) || codecconv.IsOpusTagsPacket(packet.Data) {
			continue
		}
		frame := append([]byte(nil), packet.Data...)
		blobs = append(blobs, &genx.Blob{MIMEType: "audio/opus", Data: frame})
	}
	return blobs, nil
}
