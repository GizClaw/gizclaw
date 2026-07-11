package transformers

import (
	"bytes"
	"context"
	"encoding/binary"
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
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// DoubaoRealtimeDuplex is a realtime-only transformer backed by the Doubao
// Realtime Duplex API. Client-side push-to-talk turns are handled by
// DoubaoRealtime, not this Duplex API.
type DoubaoRealtimeDuplex struct {
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
	tools            []doubaospeech.RealtimeDuplexFunctionTool
	extension        *doubaospeech.RealtimeDuplexExtension
}

var _ genx.Transformer = (*DoubaoRealtimeDuplex)(nil)

// DoubaoRealtimeDuplexRealtime is a Duplex transformer for continuous audio.
type DoubaoRealtimeDuplexRealtime struct {
	*DoubaoRealtimeDuplex
}

var _ genx.Transformer = (*DoubaoRealtimeDuplexRealtime)(nil)

const (
	doubaoRealtimeDuplexTranscriptLabel = "transcript"
	doubaoRealtimeDuplexAssistantLabel  = "assistant"
	doubaoRealtimeDuplexInterrupted     = "interrupted"

	doubaoRealtimeDuplexFixedInputFormat      = "speech_opus"
	doubaoRealtimeDuplexFixedInputSampleRate  = 16000
	doubaoRealtimeDuplexFixedInputChannels    = 1
	doubaoRealtimeDuplexFixedOutputFormat     = "ogg_opus"
	doubaoRealtimeDuplexFixedOutputSampleRate = 24000

	doubaoRealtimeDuplexOpusFrameDuration = 20 * time.Millisecond
)

type doubaoRealtimeDuplexOpener interface {
	OpenSession(context.Context, *doubaospeech.RealtimeDuplexConfig) (doubaoRealtimeDuplexSession, error)
}

type doubaoRealtimeDuplexSession interface {
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

// DoubaoRealtimeDuplexOption is a functional option for DoubaoRealtimeDuplex.
type DoubaoRealtimeDuplexOption func(*DoubaoRealtimeDuplex)

// WithDoubaoRealtimeDuplexSpeaker sets the Duplex output voice.
func WithDoubaoRealtimeDuplexSpeaker(speaker string) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.outputVoice = speaker
	}
}

// WithDoubaoRealtimeDuplexFormat sets the Duplex output audio format.
func WithDoubaoRealtimeDuplexFormat(format string) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.outputFormat = format
	}
}

// WithDoubaoRealtimeDuplexSampleRate sets the Duplex output sample rate.
func WithDoubaoRealtimeDuplexSampleRate(sampleRate int) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.outputSampleRate = sampleRate
	}
}

// WithDoubaoRealtimeDuplexInputFormat sets the audio format sent to Doubao.
func WithDoubaoRealtimeDuplexInputFormat(format string) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.inputFormat = format
	}
}

// WithDoubaoRealtimeDuplexInputSampleRate sets the input audio sample rate sent to Doubao.
func WithDoubaoRealtimeDuplexInputSampleRate(sampleRate int) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.inputSampleRate = sampleRate
	}
}

// WithDoubaoRealtimeDuplexInputChannels sets the local input audio channel count used for transcoding.
func WithDoubaoRealtimeDuplexInputChannels(channels int) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.inputChannels = channels
	}
}

// WithDoubaoRealtimeDuplexInputTranscode forces input audio through the local codec
// before sending it to Doubao. This keeps network transport compressed while
// normalizing peer Opus packets to Doubao's expected speech_opus settings.
func WithDoubaoRealtimeDuplexInputTranscode(enabled bool) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.inputTranscode = enabled
	}
}

// WithDoubaoRealtimeDuplexModel sets the upstream Duplex model version.
func WithDoubaoRealtimeDuplexModel(model string) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.model = model
	}
}

func WithDoubaoRealtimeDuplexSessionID(sessionID string) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.sessionID = sessionID
	}
}

func WithDoubaoRealtimeDuplexInstructions(instructions string) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.instructions = instructions
	}
}

func WithDoubaoRealtimeDuplexOutputSpeed(speed int) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.outputSpeed = &speed
	}
}

func WithDoubaoRealtimeDuplexOutputLoudness(loudness int) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.outputLoudness = &loudness
	}
}

func WithDoubaoRealtimeDuplexTools(tools []doubaospeech.RealtimeDuplexFunctionTool) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.tools = append([]doubaospeech.RealtimeDuplexFunctionTool(nil), tools...)
	}
}

func WithDoubaoRealtimeDuplexExtension(extension *doubaospeech.RealtimeDuplexExtension) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.extension = extension
	}
}

func withDoubaoRealtimeDuplexOpener(opener doubaoRealtimeDuplexOpener) DoubaoRealtimeDuplexOption {
	return func(t *DoubaoRealtimeDuplex) {
		t.duplex = opener
	}
}

// NewDoubaoRealtimeDuplexRealtime creates a Duplex realtime transformer.
func NewDoubaoRealtimeDuplexRealtime(client *doubaospeech.Client, opts ...DoubaoRealtimeDuplexOption) *DoubaoRealtimeDuplexRealtime {
	return &DoubaoRealtimeDuplexRealtime{DoubaoRealtimeDuplex: newDoubaoRealtimeDuplex(client, opts...)}
}

// NewDoubaoRealtimeDuplex creates a new DoubaoRealtimeDuplex transformer.
//
// Parameters:
//   - client: Doubao speech client
//   - opts: Optional configuration
func NewDoubaoRealtimeDuplex(client *doubaospeech.Client, opts ...DoubaoRealtimeDuplexOption) *DoubaoRealtimeDuplex {
	return newDoubaoRealtimeDuplex(client, opts...)
}

func newDoubaoRealtimeDuplex(client *doubaospeech.Client, opts ...DoubaoRealtimeDuplexOption) *DoubaoRealtimeDuplex {
	t := &DoubaoRealtimeDuplex{
		client:           client,
		model:            doubaospeech.RealtimeDuplexModelDefault,
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

// DoubaoRealtimeDuplexCtxKey is the context key for runtime options.
type doubaoRealtimeDuplexCtxKey struct{}

// DoubaoRealtimeDuplexCtxOptions are runtime options passed via context.
type DoubaoRealtimeDuplexCtxOptions struct{}

// WithDoubaoRealtimeDuplexCtxOptions attaches runtime options to context.
func WithDoubaoRealtimeDuplexCtxOptions(ctx context.Context, opts DoubaoRealtimeDuplexCtxOptions) context.Context {
	return context.WithValue(ctx, doubaoRealtimeDuplexCtxKey{}, opts)
}

// Transform converts audio input to audio output via realtime dialogue.
// It returns the output stream immediately and reports connection errors on it.
func (t *DoubaoRealtimeDuplex) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	config := t.realtimeConfig()
	slog.Info(
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
	go t.sessionLoop(ctx, input, output)

	return output, nil
}

func (t *DoubaoRealtimeDuplex) realtimeConfig() *doubaospeech.RealtimeDuplexConfig {
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
			Tools: append([]doubaospeech.RealtimeDuplexFunctionTool(nil), t.tools...),
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

func (t *DoubaoRealtimeDuplex) sessionLoop(ctx context.Context, input genx.Stream, output *bufferStream) {
	defer output.Close()
	input = newDoubaoRealtimeDuplexInputReader(input)
	defer input.Close()
	var pending *genx.MessageChunk
	for {
		if err := ctx.Err(); err != nil {
			output.CloseWithError(err)
			return
		}
		config := t.realtimeConfig()
		session, err := t.duplex.OpenSession(ctx, config)
		if err != nil {
			output.CloseWithError(fmt.Errorf("doubao realtime duplex open session: %w", err))
			return
		}
		next, err := t.processLoop(ctx, withDoubaoRealtimeDuplexPendingChunk(input, pending), output, session)
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

func (t *DoubaoRealtimeDuplex) processLoop(ctx context.Context, input genx.Stream, output *bufferStream, session doubaoRealtimeDuplexSession) (*genx.MessageChunk, error) {
	defer session.Close()
	var responseEpoch atomic.Uint64
	var acceptAssistant atomic.Bool
	var restarting atomic.Bool
	responseEpoch.Store(1)
	acceptAssistant.Store(true)
	var assistantMu sync.Mutex
	assistantActive := false
	assistantStreamID := ""

	markAssistantStarted := func(streamID string) uint64 {
		epoch := responseEpoch.Load()
		assistantMu.Lock()
		assistantActive = true
		assistantStreamID = streamID
		assistantMu.Unlock()
		return epoch
	}
	markAssistantDoneForStream := func(streamID string) {
		assistantMu.Lock()
		assistantActive = false
		assistantMu.Unlock()
	}
	interruptAssistant := func(streamID string) bool {
		assistantMu.Lock()
		active := assistantActive
		if !active {
			assistantMu.Unlock()
			return false
		}
		interruptedStreamID := strings.TrimSpace(assistantStreamID)
		if interruptedStreamID == "" {
			interruptedStreamID = strings.TrimSpace(streamID)
		}
		if interruptedStreamID == "" {
			interruptedStreamID = "audio"
		}
		assistantActive = false
		acceptAssistant.Store(false)
		responseEpoch.Add(1)
		assistantMu.Unlock()
		textEOS := &genx.MessageChunk{
			Role: genx.RoleModel,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: interruptedStreamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true, Error: doubaoRealtimeDuplexInterrupted},
		}
		audioEOS := &genx.MessageChunk{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: t.outputMIMEType()},
			Ctrl: &genx.StreamCtrl{StreamID: interruptedStreamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true, Error: doubaoRealtimeDuplexInterrupted},
		}
		_ = output.Push(textEOS)
		_ = output.Push(audioEOS)
		if err := session.CancelResponse(context.Background()); err != nil {
			slog.Debug("doubao: cancel current duplex response failed", "error", err)
		}
		return true
	}
	pushAssistantOutput := func(epoch uint64, chunk *genx.MessageChunk) error {
		if !acceptAssistant.Load() || responseEpoch.Load() != epoch {
			return nil
		}
		return output.Push(chunk)
	}
	waitOutputFrame := func(epoch uint64) bool {
		timer := time.NewTimer(doubaoRealtimeDuplexOpusFrameDuration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
		}
		return acceptAssistant.Load() && responseEpoch.Load() == epoch
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
		textDeltaSeen := make(map[string]bool)
		assistantTextStarted := make(map[string]bool)
		assistantTextDone := make(map[string]bool)
		assistantAudioDone := make(map[string]bool)
		assistantCompleted := make(map[string]bool)
		completeAssistantStream := func(streamID string) {
			assistantCompleted[streamID] = true
			markAssistantDoneForStream(streamID)
		}
		closeInputSegment := func() error {
			inputStreamID := streamIDs.endInputSegment()
			doneChunk := &genx.MessageChunk{
				Role: genx.RoleUser,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{
					StreamID:    inputStreamID,
					Label:       doubaoRealtimeDuplexTranscriptLabel,
					EndOfStream: true,
				},
			}
			if err := output.Push(doneChunk); err != nil {
				return err
			}
			lastTranscriptText = ""
			transcriptOpen = false
			return nil
		}
		defer func() {
			if transcriptOpen {
				if err := closeInputSegment(); err != nil {
					finishEventError(err)
				}
			}
			close(eventsDone)
		}()
		for event, err := range session.Recv() {
			if err != nil {
				if restarting.Load() {
					slog.Info("doubao: realtime duplex session stopped for restart", "error", err)
					return
				}
				slog.Error("doubao: recv error", "error", err)
				finishEventError(err)
				return
			}

			slog.Debug("doubao: received duplex event", "type", event.Type, "text", event.Text, "transcript", event.Transcript, "audioLen", len(event.Audio), "functionCalls", len(event.FunctionCalls))
			streamID := firstNonEmptyString(event.ResponseID, event.QuestionID, streamIDs.response())
			switch event.Type {
			case doubaospeech.RealtimeDuplexEventTranscriptionStarted:
				transcriptOpen = true
			case doubaospeech.RealtimeDuplexEventTranscriptionDelta:
				text := firstNonEmptyString(event.Delta, event.Transcript)
				if text == "" {
					continue
				}
				if event.Delta == "" {
					text = realtimeDuplexTextDelta(lastTranscriptText, text)
				}
				if text == "" {
					continue
				}
				if !transcriptOpen && !realtimeDuplexTextHasSemantic(text) {
					lastTranscriptText = ""
					continue
				}
				lastTranscriptText += text
				if err := output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Part: genx.Text(text),
					Ctrl: &genx.StreamCtrl{StreamID: streamIDs.input(), Label: doubaoRealtimeDuplexTranscriptLabel},
				}); err != nil {
					finishEventError(err)
					return
				}
				transcriptOpen = true
			case doubaospeech.RealtimeDuplexEventTranscriptionCompleted:
				text := firstNonEmptyString(event.Transcript, event.Text, event.Delta)
				if text != "" {
					delta := realtimeDuplexTextDelta(lastTranscriptText, text)
					if delta != "" {
						if err := output.Push(&genx.MessageChunk{
							Role: genx.RoleUser,
							Part: genx.Text(delta),
							Ctrl: &genx.StreamCtrl{StreamID: streamIDs.input(), Label: doubaoRealtimeDuplexTranscriptLabel},
						}); err != nil {
							finishEventError(err)
							return
						}
						transcriptOpen = true
					}
				}
				if transcriptOpen {
					if err := closeInputSegment(); err != nil {
						finishEventError(err)
						return
					}
				}
				acceptAssistant.Store(true)
				responseEpoch.Add(1)
			case doubaospeech.RealtimeDuplexEventTranscriptionFailed:
				errText := "transcription failed"
				if event.Error != nil && strings.TrimSpace(event.Error.Message) != "" {
					errText = event.Error.Message
				}
				if err := output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{
						StreamID:    streamIDs.endInputSegment(),
						Label:       doubaoRealtimeDuplexTranscriptLabel,
						EndOfStream: true,
						Error:       errText,
					},
				}); err != nil {
					finishEventError(err)
					return
				}
				transcriptOpen = false
			case doubaospeech.RealtimeDuplexEventInputAudioBufferCommitted:
				acceptAssistant.Store(true)
				responseEpoch.Add(1)
				if transcriptOpen {
					if err := closeInputSegment(); err != nil {
						finishEventError(err)
						return
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputTextDelta:
				if !acceptAssistant.Load() {
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
				if err := pushAssistantOutput(epoch, &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(text),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel},
				}); err != nil {
					finishEventError(err)
					return
				}
				textDeltaSeen[streamID] = true
				assistantTextStarted[streamID] = true
			case doubaospeech.RealtimeDuplexEventResponseOutputTextDone:
				if !acceptAssistant.Load() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := responseEpoch.Load()
				if event.Text != "" && !textDeltaSeen[streamID] {
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Text),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel},
					}); err != nil {
						finishEventError(err)
						return
					}
					assistantTextStarted[streamID] = true
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
				if !acceptAssistant.Load() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := responseEpoch.Load()
				if err := startAudioOutput(epoch, streamID); err != nil {
					finishEventError(err)
					return
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta:
				if !acceptAssistant.Load() || len(event.Audio) == 0 {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := responseEpoch.Load()
				if err := startAudioOutput(epoch, streamID); err != nil {
					finishEventError(err)
					return
				}
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
					if !waitOutputFrame(epoch) {
						break
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputAudioDone:
				if !acceptAssistant.Load() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := responseEpoch.Load()
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
				outputs := make([]doubaospeech.RealtimeDuplexFunctionCallOutput, 0, len(event.FunctionCalls))
				for _, call := range event.FunctionCalls {
					outputs = append(outputs, doubaospeech.RealtimeDuplexFunctionCallOutput{
						CallID: call.CallID,
						Output: doubaoRealtimeDuplexFakeToolOutput(call),
					})
				}
				if len(outputs) > 0 {
					if err := session.SendFunctionCallOutputs(context.Background(), outputs...); err != nil {
						finishEventError(err)
						return
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseCanceled:
				completeAssistantStream(streamID)
				acceptAssistant.Store(false)
			case doubaospeech.RealtimeDuplexEventResponseDone:
				completeAssistantStream(streamID)
			case doubaospeech.RealtimeDuplexEventSessionClosed:
				slog.Info("doubao: realtime duplex session closed")
				return
			case doubaospeech.RealtimeDuplexEventError:
				err := fmt.Errorf("doubao realtime duplex event error")
				if event.Error != nil {
					err = event.Error
				}
				finishEventError(err)
				return
			}
		}
	}()

	slog.Info("doubao: starting audio send loop")

	// Send audio to realtime service
	audioSent := 0
	audioInputs := newDoubaoRealtimeDuplexAudioInputs(t.inputFormat, t.inputSampleRate, t.inputChannels, t.inputTranscode)
	defer audioInputs.close()
	for {
		chunk, err, done := doubaoRealtimeDuplexNextOrDone(input, eventsDone)
		if done {
			if err := eventError(); err != nil {
				return nil, err
			}
			slog.Info("doubao: events done, waiting for next input")
			for {
				chunk, err := input.Next()
				if err != nil {
					if err != io.EOF && err != genx.ErrDone {
						slog.Error("doubao: input error after events done", "error", err)
						return nil, err
					}
					slog.Info("doubao: input EOF after events done", "audioSent", audioSent)
					return nil, nil
				}
				if chunk != nil {
					return chunk.Clone(), nil
				}
			}
		}
		if err != nil {
			if err != io.EOF && err != genx.ErrDone {
				slog.Error("doubao: input error", "error", err)
				return nil, err
			} else {
				slog.Info("doubao: input EOF", "audioSent", audioSent)
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

		// Track StreamID from BOS marker only
		if chunk.IsBeginOfStream() && chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
			if interruptAssistant(chunk.Ctrl.StreamID) {
				slog.Info("doubao: restarting realtime session after interrupt", "streamID", chunk.Ctrl.StreamID)
				restarting.Store(true)
				return chunk.Clone(), nil
			}
			streamIDs.beginInput(chunk.Ctrl.StreamID)
			slog.Info("doubao: received BOS", "streamID", chunk.Ctrl.StreamID)
			continue
		}

		// Duplex uses server-side turn detection. Input EOS only closes the
		// local stream boundary; it must not commit audio.
		if chunk.IsEndOfStream() {
			streamID := streamIDs.serviceInput(chunk)
			slog.Debug("doubao: received realtime EOS, closing local audio stream without commit", "streamID", streamID, "audioSent", audioSent)
			audioInputs.closeStream(streamID)
			continue
		}

		// Send based on part type
		switch p := chunk.Part.(type) {
		case *genx.Blob:
			// Send audio blob
			if len(p.Data) > 0 {
				streamID := streamIDs.serviceInput(chunk)
				audioInput, err := audioInputs.streamForBlob(streamID, p)
				if err != nil {
					slog.Error("doubao: prepare audio error", "error", err)
					t.pushInputEOSError(output, streamID, err)
					audioInputs.closeStream(streamID)
					return nil, err
				}
				frames, err := audioInput.prepareFrames(p)
				if err != nil {
					slog.Error("doubao: prepare audio error", "error", err)
					t.pushInputEOSError(output, streamID, err)
					audioInputs.closeStream(streamID)
					return nil, err
				}
				if len(frames) == 0 {
					continue
				}
				for _, audio := range frames {
					if len(audio) == 0 {
						continue
					}
					audioSent++
					if audioSent%50 == 1 { // Log every 50 chunks (1 second at 20ms chunks)
						slog.Debug("doubao: sending audio chunk", "streamID", streamID, "len", len(audio), "mime", p.MIMEType, "inputFormat", audioInput.format, "totalSent", audioSent)
					}
					if err := session.SendAudio(context.Background(), audio); err != nil {
						slog.Error("doubao: send audio error", "error", err)
						return nil, err
					}
				}
			}
		case genx.Text:
			if len(p) > 0 {
				return nil, fmt.Errorf("doubao realtime duplex does not accept text input")
			}
		}
	}
}

type doubaoRealtimeDuplexPendingChunkStream struct {
	first *genx.MessageChunk
	rest  genx.Stream
}

func withDoubaoRealtimeDuplexPendingChunk(rest genx.Stream, first *genx.MessageChunk) genx.Stream {
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
		chunk := s.first
		s.first = nil
		return chunk, nil, false
	}
	if stream, ok := s.rest.(doubaoRealtimeDuplexDoneAwareStream); ok {
		return stream.NextOrDone(done)
	}
	return doubaoRealtimeDuplexNextOrDone(s.rest, done)
}

func (s *doubaoRealtimeDuplexPendingChunkStream) Close() error {
	return s.rest.Close()
}

func (s *doubaoRealtimeDuplexPendingChunkStream) CloseWithError(err error) error {
	return s.rest.CloseWithError(err)
}

type doubaoRealtimeDuplexDoneAwareStream interface {
	genx.Stream
	NextOrDone(<-chan struct{}) (*genx.MessageChunk, error, bool)
}

type doubaoRealtimeDuplexInputResult struct {
	chunk *genx.MessageChunk
	err   error
}

type doubaoRealtimeDuplexInputReader struct {
	source    genx.Stream
	results   chan doubaoRealtimeDuplexInputResult
	done      chan struct{}
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
	result, ok := <-r.results
	if !ok {
		return nil, io.EOF
	}
	return result.chunk, result.err
}

func (r *doubaoRealtimeDuplexInputReader) NextOrDone(done <-chan struct{}) (*genx.MessageChunk, error, bool) {
	select {
	case <-done:
		return nil, nil, true
	case result, ok := <-r.results:
		if !ok {
			return nil, io.EOF, false
		}
		return result.chunk, result.err, false
	default:
	}
	select {
	case <-done:
		return nil, nil, true
	case result, ok := <-r.results:
		if !ok {
			return nil, io.EOF, false
		}
		return result.chunk, result.err, false
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

func doubaoRealtimeDuplexNextOrDone(input genx.Stream, done <-chan struct{}) (*genx.MessageChunk, error, bool) {
	if stream, ok := input.(doubaoRealtimeDuplexDoneAwareStream); ok {
		return stream.NextOrDone(done)
	}
	select {
	case <-done:
		return nil, nil, true
	default:
	}
	chunk, err := input.Next()
	return chunk, err, false
}

func (t *DoubaoRealtimeDuplex) pushInputEOSError(output *bufferStream, streamID string, err error) {
	if output == nil || err == nil {
		return
	}
	_ = output.Push(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{
			StreamID:    streamID,
			Label:       doubaoRealtimeDuplexTranscriptLabel,
			EndOfStream: true,
			Error:       err.Error(),
		},
	})
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func doubaoRealtimeDuplexFakeToolOutput(call doubaospeech.RealtimeDuplexFunctionCall) string {
	data, err := json.Marshal(map[string]string{
		"status":  "ok",
		"source":  "gizclaw-internal-fake",
		"tool":    call.Name,
		"call_id": call.CallID,
	})
	if err != nil {
		return `{"status":"ok","source":"gizclaw-internal-fake"}`
	}
	return string(data)
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

type doubaoRealtimeDuplexAudioInput struct {
	format    string
	transcode bool

	sampleRate int
	channels   int
	frameSize  int
	decoder    *opus.Decoder
	encoder    *opus.Encoder
}

type doubaoRealtimeDuplexAudioInputs struct {
	format     string
	sampleRate int
	channels   int
	transcode  bool

	streams   map[string]*doubaoRealtimeDuplexAudioInput
	mimeTypes map[string]string
}

func newDoubaoRealtimeDuplexAudioInputs(format string, sampleRate, channels int, transcode bool) *doubaoRealtimeDuplexAudioInputs {
	return &doubaoRealtimeDuplexAudioInputs{
		format:     format,
		sampleRate: sampleRate,
		channels:   channels,
		transcode:  transcode,
		streams:    make(map[string]*doubaoRealtimeDuplexAudioInput),
		mimeTypes:  make(map[string]string),
	}
}

func (a *doubaoRealtimeDuplexAudioInputs) stream(streamID string) *doubaoRealtimeDuplexAudioInput {
	if a == nil {
		return newDoubaoRealtimeDuplexAudioInput("", 0, 0, true)
	}
	streamID = doubaoRealtimeDuplexStreamKey(streamID)
	if input := a.streams[streamID]; input != nil {
		return input
	}
	input := newDoubaoRealtimeDuplexAudioInput(a.format, a.sampleRate, a.channels, a.transcode)
	a.streams[streamID] = input
	return input
}

func (a *doubaoRealtimeDuplexAudioInputs) streamForBlob(streamID string, blob *genx.Blob) (*doubaoRealtimeDuplexAudioInput, error) {
	if a == nil {
		return newDoubaoRealtimeDuplexAudioInput("", 0, 0, true), nil
	}
	key := doubaoRealtimeDuplexStreamKey(streamID)
	if mimeType := doubaoRealtimeDuplexBaseMIME(doubaoRealtimeDuplexBlobMIMEType(blob)); mimeType != "" {
		if previous := a.mimeTypes[key]; previous != "" && previous != mimeType {
			return nil, &doubaoRealtimeDuplexStreamMIMEChangeError{
				StreamID: key,
				From:     previous,
				To:       mimeType,
			}
		}
		a.mimeTypes[key] = mimeType
	}
	return a.stream(key), nil
}

func (a *doubaoRealtimeDuplexAudioInputs) closeStream(streamID string) {
	if a == nil {
		return
	}
	streamID = doubaoRealtimeDuplexStreamKey(streamID)
	if input := a.streams[streamID]; input != nil {
		input.close()
		delete(a.streams, streamID)
	}
	delete(a.mimeTypes, streamID)
}

func (a *doubaoRealtimeDuplexAudioInputs) close() {
	if a == nil {
		return
	}
	for streamID, input := range a.streams {
		input.close()
		delete(a.streams, streamID)
	}
	for streamID := range a.mimeTypes {
		delete(a.mimeTypes, streamID)
	}
}

func doubaoRealtimeDuplexChunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	if chunk != nil && chunk.Ctrl != nil {
		streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
		if streamID != "" && streamID != "audio" {
			return streamID
		}
	}
	return fallback
}

type doubaoRealtimeDuplexStreamIDs struct {
	mu sync.Mutex

	baseInput  string
	inputID    string
	responseID string
	segment    int
}

func newDoubaoRealtimeDuplexStreamIDs() *doubaoRealtimeDuplexStreamIDs {
	return &doubaoRealtimeDuplexStreamIDs{}
}

func (s *doubaoRealtimeDuplexStreamIDs) beginInput(id string) {
	if s == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = "audio"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseInput = id
	s.segment = 1
	s.inputID = s.inputForSegmentLocked()
}

func (s *doubaoRealtimeDuplexStreamIDs) input() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.inputID) == "" {
		s.inputID = s.inputForSegmentLocked()
	}
	return s.inputID
}

func (s *doubaoRealtimeDuplexStreamIDs) response() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.responseID
}

func (s *doubaoRealtimeDuplexStreamIDs) serviceInput(chunk *genx.MessageChunk) string {
	if s == nil {
		return doubaoRealtimeDuplexChunkInputStreamID(chunk, "")
	}
	s.mu.Lock()
	s.ensureBaseFromChunkLocked(chunk)
	base := s.baseInput
	s.mu.Unlock()
	return doubaoRealtimeDuplexChunkInputStreamID(chunk, base)
}

func (s *doubaoRealtimeDuplexStreamIDs) endInputSegment() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.inputID) == "" {
		s.inputID = s.inputForSegmentLocked()
	}
	ended := s.inputID
	s.responseID = ended
	s.segment++
	s.inputID = s.inputForSegmentLocked()
	return ended
}

func (s *doubaoRealtimeDuplexStreamIDs) ensureBaseFromChunkLocked(chunk *genx.MessageChunk) {
	if s == nil || strings.TrimSpace(s.baseInput) != "" {
		return
	}
	id := doubaoRealtimeDuplexChunkInputStreamID(chunk, "")
	if strings.TrimSpace(id) == "" {
		return
	}
	s.baseInput = id
	if s.segment <= 0 {
		s.segment = 1
	}
	s.inputID = s.inputForSegmentLocked()
}

func (s *doubaoRealtimeDuplexStreamIDs) inputForSegmentLocked() string {
	base := strings.TrimSpace(s.baseInput)
	if base == "" {
		base = "audio"
	}
	segment := s.segment
	if segment <= 0 {
		segment = 1
	}
	return fmt.Sprintf("%s:rt:%d", base, segment)
}

func newDoubaoRealtimeDuplexAudioInput(format string, sampleRate, channels int, transcode bool) *doubaoRealtimeDuplexAudioInput {
	format = doubaoRealtimeDuplexAudioFormat(format)
	sampleRate = doubaoRealtimeDuplexAudioSampleRate(sampleRate)
	channels = doubaoRealtimeDuplexAudioChannels(channels)
	return &doubaoRealtimeDuplexAudioInput{
		format:    format,
		transcode: transcode,

		sampleRate: sampleRate,
		channels:   channels,
		frameSize:  sampleRate / 50,
	}
}

func (a *doubaoRealtimeDuplexAudioInput) prepare(blob *genx.Blob) ([]byte, error) {
	frames, err := a.prepareFrames(blob)
	if err != nil {
		return nil, err
	}
	if len(frames) == 0 {
		return nil, nil
	}
	if len(frames) > 1 {
		return nil, fmt.Errorf("doubao realtime audio input produced %d frames; use prepareFrames", len(frames))
	}
	return frames[0], nil
}

func (a *doubaoRealtimeDuplexAudioInput) prepareFrames(blob *genx.Blob) ([][]byte, error) {
	if blob == nil || len(blob.Data) == 0 {
		return nil, nil
	}
	mimeType := doubaoRealtimeDuplexBaseMIME(blob.MIMEType)
	switch a.format {
	case "pcm", "pcm_s16le":
		if isDoubaoRealtimeDuplexOpusMIME(mimeType) {
			pcm, err := a.decodeOpus(blob.Data)
			if err != nil {
				return nil, err
			}
			return [][]byte{pcm}, nil
		}
		if isDoubaoRealtimeDuplexMP3InputMIME(mimeType) {
			pcm, err := a.decodeMP3ToPCM(blob.Data)
			if err != nil {
				return nil, err
			}
			return [][]byte{pcm}, nil
		}
		return [][]byte{blob.Data}, nil
	case "speech_opus", "opus":
		if mimeType == "audio/opus" {
			if a.transcode {
				frame, err := a.transcodeOpus(blob.Data)
				if err != nil {
					return nil, err
				}
				return [][]byte{frame}, nil
			}
			return [][]byte{blob.Data}, nil
		}
		if strings.HasPrefix(mimeType, "audio/ogg") {
			return nil, fmt.Errorf("doubao realtime input format %q requires raw Opus packets, got Ogg/Opus pages", a.format)
		}
		if isDoubaoRealtimeDuplexPCMInputMIME(mimeType) {
			return a.encodeOpusFrames(blob.Data)
		}
		if isDoubaoRealtimeDuplexMP3InputMIME(mimeType) {
			pcm, err := a.decodeMP3ToPCM(blob.Data)
			if err != nil {
				return nil, err
			}
			return a.encodeOpusFrames(pcm)
		}
		return nil, fmt.Errorf("doubao realtime input format %q requires audio/opus, PCM, or MP3 input, got %q", a.format, blob.MIMEType)
	case "ogg_opus":
		if strings.HasPrefix(mimeType, "audio/ogg") {
			return [][]byte{blob.Data}, nil
		}
		if mimeType == "audio/opus" {
			return nil, fmt.Errorf("doubao realtime input format %q requires Ogg/Opus pages, got raw Opus packet", a.format)
		}
		return [][]byte{blob.Data}, nil
	default:
		if isDoubaoRealtimeDuplexOpusMIME(mimeType) {
			return [][]byte{blob.Data}, nil
		}
		return [][]byte{blob.Data}, nil
	}
}

func (a *doubaoRealtimeDuplexAudioInput) encodeOpus(pcm []byte) ([]byte, error) {
	frames, err := a.encodeOpusFrames(pcm)
	if err != nil {
		return nil, err
	}
	if len(frames) == 0 {
		return nil, nil
	}
	if len(frames) > 1 {
		return nil, fmt.Errorf("doubao realtime pcm input produced %d opus frames; use encodeOpusFrames", len(frames))
	}
	return frames[0], nil
}

func (a *doubaoRealtimeDuplexAudioInput) encodeOpusFrames(pcm []byte) ([][]byte, error) {
	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("doubao realtime pcm input length must be even, got %d", len(pcm))
	}
	samples := make([]int16, len(pcm)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	if len(samples) == 0 {
		return nil, nil
	}
	samplesPerFrame := a.frameSize * a.channels
	if samplesPerFrame <= 0 {
		return nil, fmt.Errorf("doubao realtime invalid opus frame size %d", samplesPerFrame)
	}
	frames := make([][]byte, 0, (len(samples)+samplesPerFrame-1)/samplesPerFrame)
	for offset := 0; offset < len(samples); offset += samplesPerFrame {
		frame := make([]int16, samplesPerFrame)
		copy(frame, samples[offset:min(offset+samplesPerFrame, len(samples))])
		packet, err := a.encodeOpusSamples(frame)
		if err != nil {
			return nil, err
		}
		frames = append(frames, packet)
	}
	return frames, nil
}

func (a *doubaoRealtimeDuplexAudioInput) encodeOpusSamples(samples []int16) ([]byte, error) {
	if a.encoder == nil {
		enc, err := opus.NewEncoder(a.sampleRate, a.channels, opus.ApplicationAudio)
		if err != nil {
			return nil, err
		}
		a.encoder = enc
	}
	if len(samples) != a.frameSize*a.channels {
		return nil, fmt.Errorf("doubao realtime opus input frame has %d samples, want %d", len(samples), a.frameSize*a.channels)
	}
	return a.encoder.Encode(samples, a.frameSize)
}

func (a *doubaoRealtimeDuplexAudioInput) transcodeOpus(packet []byte) ([]byte, error) {
	samples, err := a.decodeOpusSamples(packet)
	if err != nil {
		return nil, err
	}
	return a.encodeOpusSamples(samples)
}

func isDoubaoRealtimeDuplexOpusMIME(mimeType string) bool {
	mimeType = doubaoRealtimeDuplexBaseMIME(mimeType)
	return mimeType == "audio/opus" || strings.HasPrefix(mimeType, "audio/ogg")
}

func isDoubaoRealtimeDuplexPCMInputMIME(mimeType string) bool {
	mimeType = doubaoRealtimeDuplexBaseMIME(mimeType)
	return strings.HasPrefix(mimeType, "audio/l16") || mimeType == "audio/pcm" || mimeType == "audio/x-pcm"
}

func isDoubaoRealtimeDuplexMP3InputMIME(mimeType string) bool {
	mimeType = doubaoRealtimeDuplexBaseMIME(mimeType)
	return mimeType == "audio/mpeg" || mimeType == "audio/mp3" || mimeType == "audio/x-mpeg" || mimeType == "audio/x-mp3"
}

func doubaoRealtimeDuplexBlobMIMEType(blob *genx.Blob) string {
	if blob == nil {
		return ""
	}
	return blob.MIMEType
}

func doubaoRealtimeDuplexBaseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func doubaoRealtimeDuplexStreamKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return "default"
	}
	return streamID
}

type doubaoRealtimeDuplexStreamMIMEChangeError struct {
	StreamID string
	From     string
	To       string
}

func (e *doubaoRealtimeDuplexStreamMIMEChangeError) Error() string {
	return fmt.Sprintf("doubao realtime stream %q changed MIME type from %q to %q", e.StreamID, e.From, e.To)
}

func doubaoRealtimeDuplexAudioFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "pcm"
	}
	return format
}

func doubaoRealtimeDuplexAudioSampleRate(sampleRate int) int {
	if sampleRate <= 0 {
		return 16000
	}
	return sampleRate
}

func doubaoRealtimeDuplexAudioChannels(channels int) int {
	if channels <= 0 {
		return 1
	}
	return channels
}

func (a *doubaoRealtimeDuplexAudioInput) decodeOpus(packet []byte) ([]byte, error) {
	samples, err := a.decodeOpusSamples(packet)
	if err != nil {
		return nil, err
	}
	return doubaoRealtimeDuplexPCM16LE(samples), nil
}

func (a *doubaoRealtimeDuplexAudioInput) decodeOpusSamples(packet []byte) ([]int16, error) {
	if a.decoder == nil {
		dec, err := opus.NewDecoder(a.sampleRate, a.channels)
		if err != nil {
			return nil, err
		}
		a.decoder = dec
	}
	samples, err := a.decoder.Decode(packet, a.frameSize, false)
	if err != nil {
		return nil, err
	}
	return samples, nil
}

func (a *doubaoRealtimeDuplexAudioInput) decodeMP3ToPCM(data []byte) ([]byte, error) {
	decoded, sampleRate, channels, err := mp3.DecodeFull(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode mp3: %w", err)
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("decode mp3: invalid sample rate %d", sampleRate)
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("decode mp3: unsupported channels %d", channels)
	}
	if a.channels != 1 && a.channels != 2 {
		return nil, fmt.Errorf("doubao realtime unsupported target channels %d", a.channels)
	}

	srcFmt := resampler.Format{SampleRate: sampleRate, Stereo: channels == 2}
	dstFmt := resampler.Format{SampleRate: a.sampleRate, Stereo: a.channels == 2}
	if srcFmt == dstFmt {
		return decoded, nil
	}

	rs, err := resampler.New(bytes.NewReader(decoded), srcFmt, dstFmt)
	if err != nil {
		return nil, fmt.Errorf("create mp3 pcm resampler: %w", err)
	}
	defer func() {
		_ = rs.Close()
	}()
	pcm, err := io.ReadAll(rs)
	if err != nil {
		return nil, fmt.Errorf("resample mp3 pcm: %w", err)
	}
	return pcm, nil
}

func (a *doubaoRealtimeDuplexAudioInput) close() {
	if a != nil && a.decoder != nil {
		_ = a.decoder.Close()
		a.decoder = nil
	}
	if a != nil && a.encoder != nil {
		_ = a.encoder.Close()
		a.encoder = nil
	}
}

func doubaoRealtimeDuplexPCM16LE(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}

func (t *DoubaoRealtimeDuplex) mimeType() string {
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

func (t *DoubaoRealtimeDuplex) outputMIMEType() string {
	if strings.EqualFold(strings.TrimSpace(t.outputFormat), "ogg_opus") {
		return "audio/opus"
	}
	return t.mimeType()
}

func (t *DoubaoRealtimeDuplex) outputAudioBlobs(audio []byte) ([]*genx.Blob, error) {
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
