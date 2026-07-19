package transformers

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

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
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

// DoubaoRealtimeDuplexFunctionCallHandler resolves one provider-ordered call
// set. It is configured per Transform so one shared provider primitive can be
// used by Agent runtimes with different executable Toolkits.
type DoubaoRealtimeDuplexFunctionCallHandler func(context.Context, []doubaospeech.RealtimeDuplexFunctionCall) ([]doubaospeech.RealtimeDuplexFunctionCallOutput, error)

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

// DoubaoRealtimeDuplexCtxOptions are per-Agent runtime options passed via context.
type DoubaoRealtimeDuplexCtxOptions struct {
	Model               string
	Tools               []doubaospeech.RealtimeDuplexFunctionTool
	FunctionCallHandler DoubaoRealtimeDuplexFunctionCallHandler
	MaxToolCalls        int
}

// WithDoubaoRealtimeDuplexCtxOptions attaches runtime options to context.
func WithDoubaoRealtimeDuplexCtxOptions(ctx context.Context, opts DoubaoRealtimeDuplexCtxOptions) context.Context {
	opts.Tools = append([]doubaospeech.RealtimeDuplexFunctionTool(nil), opts.Tools...)
	return context.WithValue(ctx, doubaoRealtimeDuplexCtxKey{}, opts)
}

// Transform converts audio input to audio output via realtime dialogue.
// It returns the output stream immediately and reports connection errors on it.
func (t *DoubaoRealtimeDuplex) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	config := t.realtimeConfig(ctx)
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

func (t *DoubaoRealtimeDuplex) realtimeConfig(ctx context.Context) *doubaospeech.RealtimeDuplexConfig {
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
	if runtime, ok := ctx.Value(doubaoRealtimeDuplexCtxKey{}).(DoubaoRealtimeDuplexCtxOptions); ok {
		if model := strings.TrimSpace(runtime.Model); model != "" {
			config.Session.Model = model
		}
		config.Session.Tools = append([]doubaospeech.RealtimeDuplexFunctionTool(nil), runtime.Tools...)
	}
	return config
}

func (t *DoubaoRealtimeDuplex) sessionLoop(ctx context.Context, input genx.Stream, output *bufferStream) {
	defer output.Close()
	input = newRealtimeInputReader(input)
	defer input.Close()
	var pending *genx.MessageChunk
	for {
		if err := ctx.Err(); err != nil {
			output.CloseWithError(err)
			return
		}
		config := t.realtimeConfig(ctx)
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
		// A provider session may close immediately after the last audio frame,
		// leaving only the local route EOS pending. Consume that boundary before
		// opening another provider session; the next session starts only when a
		// meaningful input chunk is available.
		for pending == nil || realtimeAudioInputEOS(pending) {
			pending, err = input.Next()
			if err != nil {
				if err != io.EOF && err != genx.ErrDone {
					output.CloseWithError(err)
				}
				return
			}
		}
	}
}

func (t *DoubaoRealtimeDuplex) processLoop(ctx context.Context, input genx.Stream, output *bufferStream, session doubaoRealtimeDuplexSession) (*genx.MessageChunk, error) {
	defer session.Close()
	var restarting atomic.Bool
	assistant := newRealtimeAssistantLifecycle()
	runtime, _ := ctx.Value(doubaoRealtimeDuplexCtxKey{}).(DoubaoRealtimeDuplexCtxOptions)
	var toolMu sync.Mutex
	var cancelTool context.CancelCauseFunc
	var toolCallsUsed atomic.Int64
	cancelPendingTool := func(cause error) {
		toolMu.Lock()
		cancel := cancelTool
		cancelTool = nil
		toolMu.Unlock()
		if cancel != nil {
			cancel(cause)
		}
	}
	defer cancelPendingTool(context.Canceled)

	markAssistantStarted := func(streamID string) uint64 {
		return assistant.markStarted(streamID)
	}
	output.setOutputObserver(func(chunk *genx.MessageChunk) {
		observeRealtimeAssistantOutput(assistant, doubaoRealtimeDuplexAssistantLabel, chunk)
	})
	defer output.setOutputObserver(nil)
	interruptAssistantState := func(streamID string) bool {
		interruptedStreamID, interrupted := assistant.interrupt(streamID, false)
		if !interrupted {
			return false
		}
		cancelPendingTool(errors.New(doubaoRealtimeDuplexInterrupted))
		output.discard(func(chunk *genx.MessageChunk) bool {
			return isDoubaoRealtimeDuplexAssistantChunk(chunk, interruptedStreamID)
		})
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
		return true
	}
	interruptAssistant := func(streamID string) (bool, error) {
		if !interruptAssistantState(streamID) {
			return false, nil
		}
		if err := session.CancelResponse(ctx); err != nil {
			return true, fmt.Errorf("doubao realtime duplex cancel response: %w", err)
		}
		return true, nil
	}
	pushAssistantOutput := func(epoch uint64, chunk *genx.MessageChunk) error {
		if !assistant.canPush(epoch) {
			return nil
		}
		return output.Push(chunk)
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

	sessionDone := make(chan struct{})
	var sessionDoneOnce sync.Once
	markSessionDone := func() {
		sessionDoneOnce.Do(func() { close(sessionDone) })
	}
	cleanupDone := make(chan struct{})
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
		assistantAudioStarted := make(map[string]bool)
		assistantAudioDone := make(map[string]bool)
		assistantCompleted := make(map[string]bool)
		toolResponses := make(map[string]bool)
		activeAssistantStreamID := ""
		toolCapable := runtime.FunctionCallHandler != nil
		assistantStreamID := func(providerResponseID string) string {
			if !toolCapable {
				return providerResponseID
			}
			if activeAssistantStreamID == "" {
				activeAssistantStreamID = providerResponseID
			}
			return activeAssistantStreamID
		}
		completeAssistantStream := func(streamID string) {
			assistantCompleted[streamID] = true
			if !assistantTextStarted[streamID] {
				assistant.markRouteDoneStream(streamID, true)
			}
			if !assistantAudioStarted[streamID] {
				assistant.markRouteDoneStream(streamID, false)
			}
		}
		finishAssistantStream := func(streamID string) error {
			if assistantCompleted[streamID] {
				return nil
			}
			epoch := assistant.currentEpoch()
			if assistantTextStarted[streamID] {
				if err := pushAssistantOutput(epoch, &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true},
				}); err != nil {
					return err
				}
			}
			if assistantAudioStarted[streamID] {
				if err := pushAssistantOutput(epoch, &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: t.outputMIMEType()},
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true},
				}); err != nil {
					return err
				}
			}
			completeAssistantStream(streamID)
			return nil
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
			// Stop the send loop at the provider session boundary, then publish
			// transcript EOS before allowing it to hand input to a new session.
			markSessionDone()
			if transcriptOpen {
				if err := closeInputSegment(); err != nil {
					finishEventError(err)
				}
			}
			close(cleanupDone)
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
			providerResponseID := firstNonEmptyString(event.ResponseID, event.QuestionID, streamIDs.response())
			streamID := providerResponseID
			switch event.Type {
			case doubaospeech.RealtimeDuplexEventTranscriptionStarted:
				toolCallsUsed.Store(0)
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
				assistant.setAccept(true)
				assistant.nextEpoch()
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
				assistant.setAccept(true)
				assistant.nextEpoch()
				if transcriptOpen {
					if err := closeInputSegment(); err != nil {
						finishEventError(err)
						return
					}
				}
			case doubaospeech.RealtimeDuplexEventResponseOutputTextDelta:
				streamID = assistantStreamID(providerResponseID)
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
				streamID = assistantStreamID(providerResponseID)
				if !assistant.acceptsOutput() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				epoch := assistant.currentEpoch()
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
				if !toolCapable {
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
				}
				// Tool-capable sessions delay external EOS until response.done.
				// A ToolCall may continue the same Agent response after this
				// provider route ends.
			case doubaospeech.RealtimeDuplexEventResponseOutputAudioStarted:
				streamID = assistantStreamID(providerResponseID)
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
			case doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta:
				streamID = assistantStreamID(providerResponseID)
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
				streamID = assistantStreamID(providerResponseID)
				if !assistant.acceptsOutput() {
					continue
				}
				if assistantCompleted[streamID] {
					continue
				}
				if !toolCapable && audioStarted {
					epoch := assistant.currentEpoch()
					if err := pushAssistantOutput(epoch, &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: t.outputMIMEType()},
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeDuplexAssistantLabel, EndOfStream: true},
					}); err != nil {
						finishEventError(err)
						return
					}
					assistantAudioDone[streamID] = true
					if assistantTextDone[streamID] {
						completeAssistantStream(streamID)
					}
				}
				audioStarted = false
				audioStartedStreamID = ""
				// External EOS is delayed until response.done.
			case doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone:
				if len(event.FunctionCalls) == 0 {
					continue
				}
				toolResponses[providerResponseID] = true
				if runtime.FunctionCallHandler == nil {
					finishEventError(fmt.Errorf("doubao realtime duplex function-call handler is required"))
					return
				}
				if runtime.MaxToolCalls > 0 && toolCallsUsed.Add(int64(len(event.FunctionCalls))) > int64(runtime.MaxToolCalls) {
					finishEventError(fmt.Errorf("doubao realtime duplex tool call limit exceeded: maximum %d", runtime.MaxToolCalls))
					return
				}
				toolCtx, cancel := context.WithCancelCause(ctx)
				toolMu.Lock()
				if cancelTool != nil {
					cancelTool(fmt.Errorf("doubao realtime duplex received overlapping function-call sets"))
				}
				cancelTool = cancel
				toolMu.Unlock()
				outputs, err := runtime.FunctionCallHandler(toolCtx, append([]doubaospeech.RealtimeDuplexFunctionCall(nil), event.FunctionCalls...))
				toolMu.Lock()
				if cancelTool != nil {
					cancelTool = nil
				}
				toolMu.Unlock()
				if err != nil {
					interrupted := context.Cause(toolCtx) != nil && ctx.Err() == nil
					cancel(err)
					if interrupted {
						continue
					}
					finishEventError(err)
					return
				}
				if len(outputs) != len(event.FunctionCalls) {
					cancel(fmt.Errorf("invalid function-call output count"))
					finishEventError(fmt.Errorf("doubao realtime duplex function-call handler returned %d outputs for %d calls", len(outputs), len(event.FunctionCalls)))
					return
				}
				err = session.SendFunctionCallOutputs(toolCtx, outputs...)
				interrupted := context.Cause(toolCtx) != nil && ctx.Err() == nil
				cancel(err)
				if err != nil {
					if interrupted {
						continue
					}
					finishEventError(err)
					return
				}
			case doubaospeech.RealtimeDuplexEventResponseCanceled:
				streamID = assistantStreamID(providerResponseID)
				completeAssistantStream(streamID)
				activeAssistantStreamID = ""
				assistant.setAccept(false)
			case doubaospeech.RealtimeDuplexEventResponseDone:
				if toolCapable {
					if toolResponses[providerResponseID] {
						delete(toolResponses, providerResponseID)
						continue
					}
					streamID = assistantStreamID(providerResponseID)
					if err := finishAssistantStream(streamID); err != nil {
						finishEventError(err)
						return
					}
					activeAssistantStreamID = ""
				} else {
					completeAssistantStream(streamID)
				}
			case doubaospeech.RealtimeDuplexEventSessionClosed:
				slog.Info("doubao: realtime duplex session closed")
				// Publish the terminal session boundary before unwinding Recv so a
				// concurrently unblocked input chunk belongs to the next session.
				markSessionDone()
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
		chunk, err, done := realtimeNextOrDone(input, sessionDone)
		if done {
			<-cleanupDone
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
					if chunk.IsBeginOfStream() && chunk.Ctrl != nil {
						interruptAssistantState(chunk.Ctrl.StreamID)
					}
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
			<-cleanupDone
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
			toolCallsUsed.Store(0)
			interrupted, err := interruptAssistant(chunk.Ctrl.StreamID)
			if err != nil {
				return nil, err
			}
			if interrupted {
				slog.Info("doubao: restarting realtime session after interrupt", "streamID", chunk.Ctrl.StreamID)
				restarting.Store(true)
				return chunk.Clone(), nil
			}
			streamIDs.beginInput(chunk.Ctrl.StreamID)
			slog.Info("doubao: received BOS", "streamID", chunk.Ctrl.StreamID)
			continue
		}

		// Duplex uses server-side turn detection. Audio-channel or route EOS
		// only closes the local stream boundary; it must not commit audio.
		if realtimeAudioInputEOS(chunk) {
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
					if err := session.SendAudio(ctx, audio); err != nil {
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

func isDoubaoRealtimeDuplexAssistantChunk(chunk *genx.MessageChunk, streamID string) bool {
	return chunk != nil && chunk.Role == genx.RoleModel && chunk.Ctrl != nil &&
		chunk.Ctrl.StreamID == streamID && chunk.Ctrl.Label == doubaoRealtimeDuplexAssistantLabel
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
		select {
		case <-done:
			return nil, nil, true
		default:
		}
		chunk := s.first
		s.first = nil
		return chunk, nil, false
	}
	if stream, ok := s.rest.(realtimeDoneAwareStream); ok {
		return stream.NextOrDone(done)
	}
	return realtimeNextOrDone(s.rest, done)
}

func (s *doubaoRealtimeDuplexPendingChunkStream) Close() error {
	return s.rest.Close()
}

func (s *doubaoRealtimeDuplexPendingChunkStream) CloseWithError(err error) error {
	return s.rest.CloseWithError(err)
}

type realtimeDoneAwareStream interface {
	genx.Stream
	NextOrDone(<-chan struct{}) (*genx.MessageChunk, error, bool)
}

type realtimeInputResult struct {
	chunk *genx.MessageChunk
	err   error
}

type realtimeInputReader struct {
	source    genx.Stream
	results   chan realtimeInputResult
	done      chan struct{}
	pending   *realtimeInputResult
	closeOnce sync.Once
}

func newRealtimeInputReader(source genx.Stream) *realtimeInputReader {
	reader := &realtimeInputReader{
		source:  source,
		results: make(chan realtimeInputResult, 1),
		done:    make(chan struct{}),
	}
	go reader.read()
	return reader
}

func (r *realtimeInputReader) read() {
	defer close(r.results)
	for {
		chunk, err := r.source.Next()
		result := realtimeInputResult{chunk: chunk, err: err}
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

func (r *realtimeInputReader) Next() (*genx.MessageChunk, error) {
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

func (r *realtimeInputReader) NextOrDone(done <-chan struct{}) (*genx.MessageChunk, error, bool) {
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

func (r *realtimeInputReader) Close() error {
	return r.CloseWithError(io.EOF)
}

func (r *realtimeInputReader) CloseWithError(err error) error {
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

func realtimeNextOrDone(input genx.Stream, done <-chan struct{}) (*genx.MessageChunk, error, bool) {
	if stream, ok := input.(realtimeDoneAwareStream); ok {
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

type doubaoRealtimeDuplexAudioInput = doubaoRealtimeAudioInput

type doubaoRealtimeDuplexAudioInputs = doubaoRealtimeAudioInputs

func newDoubaoRealtimeDuplexAudioInputs(format string, sampleRate, channels int, transcode bool) *doubaoRealtimeDuplexAudioInputs {
	return newDoubaoRealtimeAudioInputs(format, sampleRate, channels, transcode)
}

func newDoubaoRealtimeDuplexAudioInput(format string, sampleRate, channels int, transcode bool) *doubaoRealtimeDuplexAudioInput {
	return newDoubaoRealtimeAudioInput(format, sampleRate, channels, transcode)
}

func doubaoRealtimeDuplexChunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	return realtimeChunkInputStreamID(chunk, fallback)
}

type doubaoRealtimeDuplexStreamIDs = doubaoRealtimeStreamIDs

func newDoubaoRealtimeDuplexStreamIDs() *doubaoRealtimeDuplexStreamIDs {
	return newDoubaoRealtimeStreamIDs(DoubaoRealtimeModeRealtime)
}

func doubaoRealtimeDuplexAudioFormat(format string) string {
	return realtimeAudioFormat(format)
}

func doubaoRealtimeDuplexAudioSampleRate(sampleRate int) int {
	return realtimeAudioSampleRate(sampleRate)
}

func doubaoRealtimeDuplexPCM16LE(samples []int16) []byte {
	return realtimePCM16LE(samples)
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
