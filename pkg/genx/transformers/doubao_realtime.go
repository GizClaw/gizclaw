package transformers

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkg/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

// DoubaoRealtime is a bidirectional transformer using Doubao Realtime Duplex.
type DoubaoRealtime struct {
	client           *doubaospeech.Client
	duplex           doubaoRealtimeDuplexOpener
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
	mode             DoubaoRealtimeMode
	tools            []doubaospeech.RealtimeDuplexFunctionTool
	extension        *doubaospeech.RealtimeDuplexExtension
}

var _ genx.Transformer = (*DoubaoRealtime)(nil)

const (
	doubaoRealtimeTranscriptLabel = "transcript"
	doubaoRealtimeAssistantLabel  = "assistant"
	doubaoRealtimeInterrupted     = "interrupted"

	doubaoRealtimeFixedInputFormat      = "speech_opus"
	doubaoRealtimeFixedInputSampleRate  = 16000
	doubaoRealtimeFixedInputChannels    = 1
	doubaoRealtimeFixedOutputFormat     = "ogg_opus"
	doubaoRealtimeFixedOutputSampleRate = 24000

	doubaoRealtimeOpusFrameDuration = 20 * time.Millisecond
)

// DoubaoRealtimeMode controls how client input boundaries are interpreted.
type DoubaoRealtimeMode string

const (
	DoubaoRealtimeModePushToTalk DoubaoRealtimeMode = "push_to_talk"
	DoubaoRealtimeModeRealtime   DoubaoRealtimeMode = "realtime"
	DoubaoRealtimeModeText       DoubaoRealtimeMode = "text"
)

type doubaoRealtimeDuplexOpener interface {
	OpenSession(context.Context, *doubaospeech.RealtimeDuplexConfig) (doubaoRealtimeDuplexSession, error)
}

type doubaoRealtimeDuplexSession interface {
	SendAudio(context.Context, []byte) error
	CommitAudio(context.Context) error
	SendSpeechText(context.Context, doubaospeech.RealtimeDuplexSpeechTextRequest) error
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

// DoubaoRealtimeOption is a functional option for DoubaoRealtime.
type DoubaoRealtimeOption func(*DoubaoRealtime)

// WithDoubaoRealtimeSpeaker sets the Duplex output voice.
func WithDoubaoRealtimeSpeaker(speaker string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.outputVoice = speaker
	}
}

// WithDoubaoRealtimeFormat sets the Duplex output audio format.
func WithDoubaoRealtimeFormat(format string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.outputFormat = format
	}
}

// WithDoubaoRealtimeSampleRate sets the Duplex output sample rate.
func WithDoubaoRealtimeSampleRate(sampleRate int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.outputSampleRate = sampleRate
	}
}

// WithDoubaoRealtimeInputFormat sets the audio format sent to Doubao.
func WithDoubaoRealtimeInputFormat(format string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.inputFormat = format
	}
}

// WithDoubaoRealtimeInputSampleRate sets the input audio sample rate sent to Doubao.
func WithDoubaoRealtimeInputSampleRate(sampleRate int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.inputSampleRate = sampleRate
	}
}

// WithDoubaoRealtimeInputChannels sets the local input audio channel count used for transcoding.
func WithDoubaoRealtimeInputChannels(channels int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.inputChannels = channels
	}
}

// WithDoubaoRealtimeInputTranscode forces input audio through the local codec
// before sending it to Doubao. This keeps network transport compressed while
// normalizing peer Opus packets to Doubao's expected speech_opus settings.
func WithDoubaoRealtimeInputTranscode(enabled bool) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.inputTranscode = enabled
	}
}

// WithDoubaoRealtimeModel sets the upstream Duplex model version.
func WithDoubaoRealtimeModel(model string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.model = model
	}
}

func WithDoubaoRealtimeInstructions(instructions string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.instructions = instructions
	}
}

func WithDoubaoRealtimeOutputSpeed(speed int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.outputSpeed = &speed
	}
}

func WithDoubaoRealtimeOutputLoudness(loudness int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.outputLoudness = &loudness
	}
}

func WithDoubaoRealtimeTools(tools []doubaospeech.RealtimeDuplexFunctionTool) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.tools = append([]doubaospeech.RealtimeDuplexFunctionTool(nil), tools...)
	}
}

func WithDoubaoRealtimeExtension(extension *doubaospeech.RealtimeDuplexExtension) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.extension = extension
	}
}

func withDoubaoRealtimeDuplexOpener(opener doubaoRealtimeDuplexOpener) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.duplex = opener
	}
}

// WithDoubaoRealtimeMode sets the client input mode.
func WithDoubaoRealtimeMode(mode DoubaoRealtimeMode) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		switch mode {
		case DoubaoRealtimeModePushToTalk, DoubaoRealtimeModeRealtime, DoubaoRealtimeModeText:
			t.mode = mode
		}
	}
}

// NewDoubaoRealtime creates a new DoubaoRealtime transformer.
//
// Parameters:
//   - client: Doubao speech client
//   - opts: Optional configuration
func NewDoubaoRealtime(client *doubaospeech.Client, opts ...DoubaoRealtimeOption) *DoubaoRealtime {
	t := &DoubaoRealtime{
		client:           client,
		model:            doubaospeech.RealtimeDuplexModelDefault,
		inputFormat:      doubaoRealtimeFixedInputFormat,
		inputSampleRate:  doubaoRealtimeFixedInputSampleRate,
		inputChannels:    doubaoRealtimeFixedInputChannels,
		inputTranscode:   true,
		outputFormat:     doubaoRealtimeFixedOutputFormat,
		outputSampleRate: doubaoRealtimeFixedOutputSampleRate,
		outputVoice:      "zh_female_vv_jupiter_bigtts",
		mode:             DoubaoRealtimeModePushToTalk,
	}
	for _, opt := range opts {
		opt(t)
	}
	if t.duplex == nil {
		t.duplex = doubaoRealtimeDuplexClient{client: client}
	}
	return t
}

// DoubaoRealtimeCtxKey is the context key for runtime options.
type doubaoRealtimeCtxKey struct{}

// DoubaoRealtimeCtxOptions are runtime options passed via context.
type DoubaoRealtimeCtxOptions struct{}

// WithDoubaoRealtimeCtxOptions attaches runtime options to context.
func WithDoubaoRealtimeCtxOptions(ctx context.Context, opts DoubaoRealtimeCtxOptions) context.Context {
	return context.WithValue(ctx, doubaoRealtimeCtxKey{}, opts)
}

// Transform converts audio input to audio output via realtime dialogue.
// It returns the output stream immediately and reports connection errors on it.
func (t *DoubaoRealtime) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	config := t.realtimeConfig()
	slog.Info(
		"doubao: realtime duplex session config",
		"model", config.Session.Model,
		"inputFormat", config.Session.Audio.Input.Format.Type,
		"inputSampleRate", config.Session.Audio.Input.Format.Rate,
		"inputTranscode", t.inputTranscode,
		"inputMode", t.mode,
		"outputFormat", config.Session.Audio.Output.Format.Type,
		"outputSampleRate", config.Session.Audio.Output.Format.Rate,
		"outputVoice", config.Session.Audio.Output.Voice,
		"tools", len(config.Session.Tools),
	)

	output := newBufferStream(16)
	go t.sessionLoop(ctx, input, output)

	return output, nil
}

func (t *DoubaoRealtime) realtimeConfig() *doubaospeech.RealtimeDuplexConfig {
	config := &doubaospeech.RealtimeDuplexConfig{
		Session: doubaospeech.RealtimeDuplexSessionConfig{
			Model:        strings.TrimSpace(t.model),
			Instructions: t.instructions,
			Audio: doubaospeech.RealtimeDuplexAudioConfig{
				Input: doubaospeech.RealtimeDuplexAudioInputConfig{
					Format: doubaospeech.RealtimeDuplexAudioFormat{
						Type: doubaoRealtimeAudioFormat(t.inputFormat),
						Rate: doubaoRealtimeAudioSampleRate(t.inputSampleRate),
					},
				},
				Output: doubaospeech.RealtimeDuplexAudioOutputConfig{
					Format: doubaospeech.RealtimeDuplexAudioFormat{
						Type: doubaoRealtimeAudioFormat(t.outputFormat),
						Rate: doubaoRealtimeAudioSampleRate(t.outputSampleRate),
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

func (t *DoubaoRealtime) sessionLoop(ctx context.Context, input genx.Stream, output *bufferStream) {
	defer output.Close()
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
		next, err := t.processLoop(ctx, withPendingChunk(input, pending), output, session)
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

func (t *DoubaoRealtime) processLoop(ctx context.Context, input genx.Stream, output *bufferStream, session doubaoRealtimeDuplexSession) (*genx.MessageChunk, error) {
	defer session.Close()
	var responseEpoch atomic.Uint64
	var acceptAssistant atomic.Bool
	var restarting atomic.Bool
	responseEpoch.Store(1)
	acceptAssistant.Store(true)
	var assistantMu sync.Mutex
	assistantActive := false
	assistantStreamID := ""
	assistantEpoch := uint64(1)

	markAssistantStarted := func(streamID string) uint64 {
		epoch := responseEpoch.Load()
		assistantMu.Lock()
		assistantActive = true
		assistantStreamID = streamID
		assistantEpoch = epoch
		assistantMu.Unlock()
		return epoch
	}
	markAssistantDone := func(epoch uint64) {
		assistantMu.Lock()
		if assistantEpoch == epoch {
			assistantActive = false
		}
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
		epoch := responseEpoch.Add(1)
		assistantEpoch = epoch
		assistantMu.Unlock()
		textEOS := &genx.MessageChunk{
			Role: genx.RoleModel,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: interruptedStreamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true, Error: doubaoRealtimeInterrupted},
		}
		audioEOS := &genx.MessageChunk{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: t.outputMIMEType()},
			Ctrl: &genx.StreamCtrl{StreamID: interruptedStreamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true, Error: doubaoRealtimeInterrupted},
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
		timer := time.NewTimer(doubaoRealtimeOpusFrameDuration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
		}
		return acceptAssistant.Load() && responseEpoch.Load() == epoch
	}

	streamIDs := newDoubaoRealtimeStreamIDs(t.mode)
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
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, BeginOfStream: true},
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
		closeInputSegment := func() error {
			inputStreamID := streamIDs.endInputSegment()
			doneChunk := &genx.MessageChunk{
				Role: genx.RoleUser,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{
					StreamID:    inputStreamID,
					Label:       doubaoRealtimeTranscriptLabel,
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
			if t.mode == DoubaoRealtimeModeRealtime && transcriptOpen {
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
					text = realtimeTextDelta(lastTranscriptText, text)
				}
				if text == "" {
					continue
				}
				if t.mode == DoubaoRealtimeModeRealtime && !transcriptOpen && !realtimeTextHasSemantic(text) {
					lastTranscriptText = ""
					continue
				}
				lastTranscriptText += text
				if err := output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Part: genx.Text(text),
					Ctrl: &genx.StreamCtrl{StreamID: streamIDs.input(), Label: doubaoRealtimeTranscriptLabel},
				}); err != nil {
					finishEventError(err)
					return
				}
				transcriptOpen = true
			case doubaospeech.RealtimeDuplexEventTranscriptionCompleted:
				text := firstNonEmptyString(event.Transcript, event.Text, event.Delta)
				if text != "" {
					delta := realtimeTextDelta(lastTranscriptText, text)
					if delta != "" {
						if err := output.Push(&genx.MessageChunk{
							Role: genx.RoleUser,
							Part: genx.Text(delta),
							Ctrl: &genx.StreamCtrl{StreamID: streamIDs.input(), Label: doubaoRealtimeTranscriptLabel},
						}); err != nil {
							finishEventError(err)
							return
						}
						transcriptOpen = true
					}
				}
				if transcriptOpen || t.mode == DoubaoRealtimeModePushToTalk {
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
						Label:       doubaoRealtimeTranscriptLabel,
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
				if t.mode == DoubaoRealtimeModeRealtime && transcriptOpen {
					if err := closeInputSegment(); err != nil {
						finishEventError(err)
						return
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputTextDelta:
				if !acceptAssistant.Load() {
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
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
				}); err != nil {
					finishEventError(err)
					return
				}
				textDeltaSeen[streamID] = true
			case doubaospeech.RealtimeDuplexEventResponseOutputTextDone:
				if !acceptAssistant.Load() {
					continue
				}
				epoch := responseEpoch.Load()
				if event.Text != "" && !textDeltaSeen[streamID] {
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Text),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
					}); err != nil {
						finishEventError(err)
						return
					}
				}
				delete(textDeltaSeen, streamID)
				if err := pushAssistantOutput(epoch, &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
				}); err != nil {
					finishEventError(err)
					return
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputAudioStarted:
				if !acceptAssistant.Load() {
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
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
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
				epoch := responseEpoch.Load()
				if audioStarted {
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: t.outputMIMEType()},
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
					}); err != nil {
						finishEventError(err)
						return
					}
				}
				audioStarted = false
				audioStartedStreamID = ""
				markAssistantDone(epoch)
			case doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone:
				outputs := make([]doubaospeech.RealtimeDuplexFunctionCallOutput, 0, len(event.FunctionCalls))
				for _, call := range event.FunctionCalls {
					outputs = append(outputs, doubaospeech.RealtimeDuplexFunctionCallOutput{
						CallID: call.CallID,
						Output: doubaoRealtimeFakeToolOutput(call),
					})
				}
				if len(outputs) > 0 {
					if err := session.SendFunctionCallOutputs(context.Background(), outputs...); err != nil {
						finishEventError(err)
						return
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseCanceled:
				epoch := responseEpoch.Load()
				markAssistantDone(epoch)
				acceptAssistant.Store(false)
			case doubaospeech.RealtimeDuplexEventResponseDone:
				epoch := responseEpoch.Load()
				markAssistantDone(epoch)
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
	audioInputs := newDoubaoRealtimeAudioInputs(t.inputFormat, t.inputSampleRate, t.inputChannels, t.inputTranscode)
	defer audioInputs.close()
	for {
		select {
		case <-eventsDone:
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
		default:
		}

		chunk, err := input.Next()
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

		// Handle EOS according to the configured input mode.
		if chunk.IsEndOfStream() {
			streamID := streamIDs.serviceInput(chunk)
			if t.mode != DoubaoRealtimeModeText {
				historyStreamID := streamIDs.historyInput(chunk)
				if t.mode != DoubaoRealtimeModeRealtime {
					mimeType := ""
					if blob, ok := chunk.Part.(*genx.Blob); ok {
						mimeType = blob.MIMEType
					}
					if err := output.Push(historyUserAudioEOSChunk(historyStreamID, mimeType)); err != nil {
						return nil, err
					}
				}
				slog.Info("doubao: received EOS, committing duplex audio", "streamID", streamID, "audioSent", audioSent)
				if err := session.CommitAudio(context.Background()); err != nil {
					slog.Error("doubao: commit audio error", "error", err)
					return nil, err
				}
			}
			audioInputs.closeStream(streamID)
			continue
		}

		// Send based on part type
		switch p := chunk.Part.(type) {
		case *genx.Blob:
			if t.mode == DoubaoRealtimeModeText {
				return nil, fmt.Errorf("doubao realtime text mode does not accept audio input")
			}
			// Send audio blob
			if len(p.Data) > 0 {
				streamID := streamIDs.serviceInput(chunk)
				historyStreamID := streamIDs.historyInput(chunk)
				if t.mode != DoubaoRealtimeModeRealtime {
					if err := output.Push(historyUserAudioChunk(chunk, historyStreamID)); err != nil {
						return nil, err
					}
				}
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
				slog.Info("doubao: sending text", "text", string(p))
				if err := session.SendSpeechText(context.Background(), doubaospeech.RealtimeDuplexSpeechTextRequest{Text: string(p)}); err != nil {
					slog.Error("doubao: send text error", "error", err)
					return nil, err
				}
			}
		}
	}
}

type pendingChunkStream struct {
	first *genx.MessageChunk
	rest  genx.Stream
}

func withPendingChunk(rest genx.Stream, first *genx.MessageChunk) genx.Stream {
	if first == nil {
		return rest
	}
	return &pendingChunkStream{first: first, rest: rest}
}

func (s *pendingChunkStream) Next() (*genx.MessageChunk, error) {
	if s.first != nil {
		chunk := s.first
		s.first = nil
		return chunk, nil
	}
	return s.rest.Next()
}

func (s *pendingChunkStream) Close() error {
	return s.rest.Close()
}

func (s *pendingChunkStream) CloseWithError(err error) error {
	return s.rest.CloseWithError(err)
}

func (t *DoubaoRealtime) pushInputEOSError(output *bufferStream, streamID string, err error) {
	if output == nil || err == nil {
		return
	}
	_ = output.Push(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{
			StreamID:    streamID,
			Label:       doubaoRealtimeTranscriptLabel,
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

func doubaoRealtimeFakeToolOutput(call doubaospeech.RealtimeDuplexFunctionCall) string {
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

func realtimeASRText(payload []byte) string {
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

func realtimeTextDelta(previous, current string) string {
	if current == "" || current == previous {
		return ""
	}
	if previous != "" && strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	if previous != "" {
		if suffix, ok := realtimeTextSuffixAfterNormalizedPrefix(previous, current); ok {
			return suffix
		}
		previousNorm := realtimeNormalizeText(previous)
		currentNorm := realtimeNormalizeText(current)
		if previousNorm != "" && currentNorm != "" && strings.Contains(previousNorm, currentNorm) {
			return ""
		}
	}
	return current
}

func realtimeTextSuffixAfterNormalizedPrefix(previous, current string) (string, bool) {
	previousNorm := realtimeNormalizeText(previous)
	if previousNorm == "" {
		return current, true
	}
	matched := 0
	for i, r := range current {
		norm := realtimeNormalizeText(string(r))
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

func realtimeNormalizeText(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '\u4e00' && r <= '\u9fff') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func realtimeTextHasSemantic(text string) bool {
	return realtimeNormalizeText(text) != ""
}

func realtimeASRResponseEndsSegment(event *doubaospeech.RealtimeEvent, delta string) bool {
	if event == nil || !realtimeTextHasSemantic(delta) {
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
		if !result.IsInterim && realtimeTextHasSemantic(text) {
			return true
		}
	}
	if event.IsFinal {
		return true
	}
	return false
}

type doubaoRealtimeAudioInput struct {
	format    string
	transcode bool

	sampleRate int
	channels   int
	frameSize  int
	decoder    *opus.Decoder
	encoder    *opus.Encoder
}

type doubaoRealtimeAudioInputs struct {
	format     string
	sampleRate int
	channels   int
	transcode  bool

	streams   map[string]*doubaoRealtimeAudioInput
	mimeTypes map[string]string
}

func newDoubaoRealtimeAudioInputs(format string, sampleRate, channels int, transcode bool) *doubaoRealtimeAudioInputs {
	return &doubaoRealtimeAudioInputs{
		format:     format,
		sampleRate: sampleRate,
		channels:   channels,
		transcode:  transcode,
		streams:    make(map[string]*doubaoRealtimeAudioInput),
		mimeTypes:  make(map[string]string),
	}
}

func (a *doubaoRealtimeAudioInputs) stream(streamID string) *doubaoRealtimeAudioInput {
	if a == nil {
		return newDoubaoRealtimeAudioInput("", 0, 0, true)
	}
	streamID = doubaoRealtimeStreamKey(streamID)
	if input := a.streams[streamID]; input != nil {
		return input
	}
	input := newDoubaoRealtimeAudioInput(a.format, a.sampleRate, a.channels, a.transcode)
	a.streams[streamID] = input
	return input
}

func (a *doubaoRealtimeAudioInputs) streamForBlob(streamID string, blob *genx.Blob) (*doubaoRealtimeAudioInput, error) {
	if a == nil {
		return newDoubaoRealtimeAudioInput("", 0, 0, true), nil
	}
	key := doubaoRealtimeStreamKey(streamID)
	if mimeType := doubaoRealtimeBaseMIME(blobMIMEType(blob)); mimeType != "" {
		if previous := a.mimeTypes[key]; previous != "" && previous != mimeType {
			return nil, &doubaoRealtimeStreamMIMEChangeError{
				StreamID: key,
				From:     previous,
				To:       mimeType,
			}
		}
		a.mimeTypes[key] = mimeType
	}
	return a.stream(key), nil
}

func (a *doubaoRealtimeAudioInputs) closeStream(streamID string) {
	if a == nil {
		return
	}
	streamID = doubaoRealtimeStreamKey(streamID)
	if input := a.streams[streamID]; input != nil {
		input.close()
		delete(a.streams, streamID)
	}
	delete(a.mimeTypes, streamID)
}

func (a *doubaoRealtimeAudioInputs) close() {
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

func chunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	if chunk != nil && chunk.Ctrl != nil {
		streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
		if streamID != "" && streamID != "audio" {
			return streamID
		}
	}
	return fallback
}

type doubaoRealtimeStreamIDs struct {
	mu sync.Mutex

	mode       DoubaoRealtimeMode
	baseInput  string
	inputID    string
	responseID string
	segment    int
}

func newDoubaoRealtimeStreamIDs(mode DoubaoRealtimeMode) *doubaoRealtimeStreamIDs {
	return &doubaoRealtimeStreamIDs{mode: mode}
}

func (s *doubaoRealtimeStreamIDs) beginInput(id string) {
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

func (s *doubaoRealtimeStreamIDs) input() string {
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

func (s *doubaoRealtimeStreamIDs) response() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.responseID
}

func (s *doubaoRealtimeStreamIDs) serviceInput(chunk *genx.MessageChunk) string {
	if s == nil {
		return chunkInputStreamID(chunk, "")
	}
	s.mu.Lock()
	s.ensureBaseFromChunkLocked(chunk)
	base := s.baseInput
	s.mu.Unlock()
	return chunkInputStreamID(chunk, base)
}

func (s *doubaoRealtimeStreamIDs) historyInput(chunk *genx.MessageChunk) string {
	if s == nil {
		return chunkInputStreamID(chunk, "")
	}
	if s.mode != DoubaoRealtimeModeRealtime {
		return chunkInputStreamID(chunk, s.input())
	}
	s.mu.Lock()
	s.ensureBaseFromChunkLocked(chunk)
	s.mu.Unlock()
	current := s.input()
	if strings.TrimSpace(current) != "" {
		return current
	}
	return chunkInputStreamID(chunk, "")
}

func (s *doubaoRealtimeStreamIDs) endInputSegment() string {
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
	if s.mode == DoubaoRealtimeModeRealtime {
		s.segment++
		s.inputID = s.inputForSegmentLocked()
	}
	return ended
}

func (s *doubaoRealtimeStreamIDs) ensureBaseFromChunkLocked(chunk *genx.MessageChunk) {
	if s == nil || strings.TrimSpace(s.baseInput) != "" {
		return
	}
	id := chunkInputStreamID(chunk, "")
	if strings.TrimSpace(id) == "" {
		return
	}
	s.baseInput = id
	if s.segment <= 0 {
		s.segment = 1
	}
	s.inputID = s.inputForSegmentLocked()
}

func (s *doubaoRealtimeStreamIDs) inputForSegmentLocked() string {
	base := strings.TrimSpace(s.baseInput)
	if base == "" {
		base = "audio"
	}
	if s.mode != DoubaoRealtimeModeRealtime {
		return base
	}
	segment := s.segment
	if segment <= 0 {
		segment = 1
	}
	return fmt.Sprintf("%s:rt:%d", base, segment)
}

func newDoubaoRealtimeAudioInput(format string, sampleRate, channels int, transcode bool) *doubaoRealtimeAudioInput {
	format = doubaoRealtimeAudioFormat(format)
	sampleRate = doubaoRealtimeAudioSampleRate(sampleRate)
	channels = doubaoRealtimeAudioChannels(channels)
	return &doubaoRealtimeAudioInput{
		format:    format,
		transcode: transcode,

		sampleRate: sampleRate,
		channels:   channels,
		frameSize:  sampleRate / 50,
	}
}

func (a *doubaoRealtimeAudioInput) prepare(blob *genx.Blob) ([]byte, error) {
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

func (a *doubaoRealtimeAudioInput) prepareFrames(blob *genx.Blob) ([][]byte, error) {
	if blob == nil || len(blob.Data) == 0 {
		return nil, nil
	}
	mimeType := doubaoRealtimeBaseMIME(blob.MIMEType)
	switch a.format {
	case "pcm", "pcm_s16le":
		if isDoubaoRealtimeOpusMIME(mimeType) {
			pcm, err := a.decodeOpus(blob.Data)
			if err != nil {
				return nil, err
			}
			return [][]byte{pcm}, nil
		}
		if isDoubaoRealtimeMP3InputMIME(mimeType) {
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
		if isDoubaoRealtimePCMInputMIME(mimeType) {
			return a.encodeOpusFrames(blob.Data)
		}
		if isDoubaoRealtimeMP3InputMIME(mimeType) {
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
		if isDoubaoRealtimeOpusMIME(mimeType) {
			return [][]byte{blob.Data}, nil
		}
		return [][]byte{blob.Data}, nil
	}
}

func (a *doubaoRealtimeAudioInput) silenceFrames(frameCount int) ([][]byte, error) {
	if frameCount <= 0 {
		return nil, nil
	}
	switch a.format {
	case "speech_opus", "opus":
		silence := make([]int16, a.frameSize*a.channels)
		frames := make([][]byte, 0, frameCount)
		for i := 0; i < frameCount; i++ {
			frame, err := a.encodeOpusSamples(silence)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
		}
		return frames, nil
	case "ogg_opus":
		return nil, fmt.Errorf("doubao realtime Ogg/Opus silence frames are not supported")
	default:
		frameBytes := a.frameSize * a.channels * 2
		if frameBytes <= 0 {
			frameBytes = 640
		}
		frames := make([][]byte, 0, frameCount)
		for i := 0; i < frameCount; i++ {
			frames = append(frames, make([]byte, frameBytes))
		}
		return frames, nil
	}
}

func (a *doubaoRealtimeAudioInput) encodeOpus(pcm []byte) ([]byte, error) {
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

func (a *doubaoRealtimeAudioInput) encodeOpusFrames(pcm []byte) ([][]byte, error) {
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

func (a *doubaoRealtimeAudioInput) encodeOpusSamples(samples []int16) ([]byte, error) {
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

func (a *doubaoRealtimeAudioInput) transcodeOpus(packet []byte) ([]byte, error) {
	samples, err := a.decodeOpusSamples(packet)
	if err != nil {
		return nil, err
	}
	return a.encodeOpusSamples(samples)
}

func isDoubaoRealtimeOpusMIME(mimeType string) bool {
	mimeType = doubaoRealtimeBaseMIME(mimeType)
	return mimeType == "audio/opus" || strings.HasPrefix(mimeType, "audio/ogg")
}

func isDoubaoRealtimePCMInputMIME(mimeType string) bool {
	mimeType = doubaoRealtimeBaseMIME(mimeType)
	return strings.HasPrefix(mimeType, "audio/l16") || mimeType == "audio/pcm" || mimeType == "audio/x-pcm"
}

func isDoubaoRealtimeMP3InputMIME(mimeType string) bool {
	mimeType = doubaoRealtimeBaseMIME(mimeType)
	return mimeType == "audio/mpeg" || mimeType == "audio/mp3" || mimeType == "audio/x-mpeg" || mimeType == "audio/x-mp3"
}

func blobMIMEType(blob *genx.Blob) string {
	if blob == nil {
		return ""
	}
	return blob.MIMEType
}

func doubaoRealtimeBaseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func doubaoRealtimeStreamKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return "default"
	}
	return streamID
}

type doubaoRealtimeStreamMIMEChangeError struct {
	StreamID string
	From     string
	To       string
}

func (e *doubaoRealtimeStreamMIMEChangeError) Error() string {
	return fmt.Sprintf("doubao realtime stream %q changed MIME type from %q to %q", e.StreamID, e.From, e.To)
}

func doubaoRealtimeAudioFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "pcm"
	}
	return format
}

func doubaoRealtimeAudioSampleRate(sampleRate int) int {
	if sampleRate <= 0 {
		return 16000
	}
	return sampleRate
}

func doubaoRealtimeAudioChannels(channels int) int {
	if channels <= 0 {
		return 1
	}
	return channels
}

func (a *doubaoRealtimeAudioInput) decodeOpus(packet []byte) ([]byte, error) {
	samples, err := a.decodeOpusSamples(packet)
	if err != nil {
		return nil, err
	}
	return pcm16LE(samples), nil
}

func (a *doubaoRealtimeAudioInput) decodeOpusSamples(packet []byte) ([]int16, error) {
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

func (a *doubaoRealtimeAudioInput) decodeMP3ToPCM(data []byte) ([]byte, error) {
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

func (a *doubaoRealtimeAudioInput) close() {
	if a != nil && a.decoder != nil {
		_ = a.decoder.Close()
		a.decoder = nil
	}
	if a != nil && a.encoder != nil {
		_ = a.encoder.Close()
		a.encoder = nil
	}
}

func pcm16LE(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}

func (t *DoubaoRealtime) mimeType() string {
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

func (t *DoubaoRealtime) outputMIMEType() string {
	if strings.EqualFold(strings.TrimSpace(t.outputFormat), "ogg_opus") {
		return "audio/opus"
	}
	return t.mimeType()
}

func (t *DoubaoRealtime) outputAudioBlobs(audio []byte) ([]*genx.Blob, error) {
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
