package transformers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// DashScopeRealtime is a realtime transformer using DashScope Qwen Realtime.
//
// The Tool-capable Agent runtime uses the Qwen3.5 Omni Plus/Flash Realtime
// model family whose typed function-call contract this implementation supports.
//
// This is a bidirectional transformer:
// Input: genx.Stream with audio Blob chunks (PCM16 16kHz)
// Output: genx.Stream with audio Blob chunks (PCM16 24kHz)
//
// Internally uses the configured Qwen model for speech-to-speech.
type DashScopeRealtime struct {
	client       *dashscope.Client
	model        string
	voice        string
	instructions string
	modalities   []string
	vadType      string

	// Additional options
	temperature                   *float64
	maxOutputTokens               *int
	enableInputAudioTranscription bool
	inputAudioTranscriptionModel  string
	turnDetection                 *dashscope.TurnDetection
	inputAudioFormat              string // pcm16, mp3, wav
	outputAudioFormat             string // pcm16, mp3, wav
}

// DashScopeRealtimeFunctionCall is one completed typed provider call.
type DashScopeRealtimeFunctionCall struct {
	CallID      string
	Name        string
	Arguments   string
	OutputIndex int
}

// DashScopeRealtimeFunctionCallOutput is one typed result submitted to DashScope.
type DashScopeRealtimeFunctionCallOutput struct {
	CallID string
	Output string
}

// DashScopeRealtimeFunctionCallHandler resolves calls in provider output-index order.
type DashScopeRealtimeFunctionCallHandler func(context.Context, []DashScopeRealtimeFunctionCall) ([]DashScopeRealtimeFunctionCallOutput, error)

var _ genx.Transformer = (*DashScopeRealtime)(nil)

const (
	dashScopeRealtimeAssistantLabel  = "assistant"
	dashScopeRealtimeTranscriptLabel = "transcript"
)

// DashScopeRealtimeOption is a functional option for DashScopeRealtime.
type DashScopeRealtimeOption func(*DashScopeRealtime)

// WithDashScopeRealtimeModel sets the model.
// The Tool-capable Agent runtime passes a Qwen3.5 Omni Plus/Flash Realtime ID.
func WithDashScopeRealtimeModel(model string) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.model = model
	}
}

// WithDashScopeRealtimeVoice sets the TTS voice.
// Options: Chelsie, Cherry, Serena, Ethan
func WithDashScopeRealtimeVoice(voice string) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.voice = voice
	}
}

// WithDashScopeRealtimeInstructions sets the system prompt.
func WithDashScopeRealtimeInstructions(instructions string) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.instructions = instructions
	}
}

// WithDashScopeRealtimeModalities sets the output modalities.
// Options: ["text"], ["audio"], ["text", "audio"]
func WithDashScopeRealtimeModalities(modalities []string) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.modalities = modalities
	}
}

// WithDashScopeRealtimeVAD sets the VAD mode.
// Options: server_vad, disabled (empty string means manual mode)
func WithDashScopeRealtimeVAD(vadType string) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.vadType = vadType
	}
}

// WithDashScopeRealtimeTemperature sets the temperature for response generation.
// Range: 0.0-2.0. Higher values make output more random.
func WithDashScopeRealtimeTemperature(temp float64) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.temperature = &temp
	}
}

// WithDashScopeRealtimeMaxOutputTokens sets the maximum output tokens.
// Use -1 for unlimited.
func WithDashScopeRealtimeMaxOutputTokens(tokens int) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.maxOutputTokens = &tokens
	}
}

// WithDashScopeRealtimeEnableASR enables input audio transcription (ASR).
// When enabled, the transformer will emit user speech transcription.
func WithDashScopeRealtimeEnableASR(enable bool) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.enableInputAudioTranscription = enable
	}
}

// WithDashScopeRealtimeASRModel sets the model for input audio transcription.
// Example: "qwen-audio-turbo"
func WithDashScopeRealtimeASRModel(model string) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.inputAudioTranscriptionModel = model
	}
}

// WithDashScopeRealtimeTurnDetection sets detailed VAD configuration.
// Use this for fine-grained control over voice activity detection.
func WithDashScopeRealtimeTurnDetection(td *dashscope.TurnDetection) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.turnDetection = td
	}
}

// WithDashScopeRealtimeInputAudioFormat sets the input audio format.
// Options: pcm16 (default, 16kHz), mp3, wav
func WithDashScopeRealtimeInputAudioFormat(format string) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.inputAudioFormat = format
	}
}

// WithDashScopeRealtimeOutputAudioFormat sets the output audio format.
// Options: pcm16 (default, 24kHz), mp3, wav
func WithDashScopeRealtimeOutputAudioFormat(format string) DashScopeRealtimeOption {
	return func(t *DashScopeRealtime) {
		t.outputAudioFormat = format
	}
}

// NewDashScopeRealtime creates a new DashScopeRealtime transformer.
//
// Parameters:
//   - client: DashScope client
//   - opts: Optional configuration
func NewDashScopeRealtime(client *dashscope.Client, opts ...DashScopeRealtimeOption) *DashScopeRealtime {
	t := &DashScopeRealtime{
		client:                        client,
		model:                         dashscope.ModelQwenOmniTurboRealtimeLatest,
		voice:                         dashscope.VoiceChelsie,
		modalities:                    []string{dashscope.ModalityAudio, dashscope.ModalityText},
		vadType:                       "",   // Empty means manual mode (no auto VAD)
		enableInputAudioTranscription: true, // Enable ASR by default
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// getOutputAudioMIMEType returns the MIME type based on the configured output format.
func (t *DashScopeRealtime) getOutputAudioMIMEType() string {
	switch t.outputAudioFormat {
	case dashscope.AudioFormatMP3:
		return "audio/mpeg"
	case dashscope.AudioFormatWAV:
		return "audio/wav"
	default:
		return "audio/pcm"
	}
}

// DashScopeRealtimeCtxKey is the context key for runtime options.
type dashScopeRealtimeCtxKey struct{}

// DashScopeRealtimeCtxOptions are per-Agent runtime options passed via context.
type DashScopeRealtimeCtxOptions struct {
	Model               string
	Tools               []dashscope.FunctionTool
	FunctionCallHandler DashScopeRealtimeFunctionCallHandler
	MaxToolCalls        int
}

// WithDashScopeRealtimeCtxOptions attaches runtime options to context.
func WithDashScopeRealtimeCtxOptions(ctx context.Context, opts DashScopeRealtimeCtxOptions) context.Context {
	opts.Tools = append([]dashscope.FunctionTool(nil), opts.Tools...)
	return context.WithValue(ctx, dashScopeRealtimeCtxKey{}, opts)
}

type dashScopeRealtimeSession interface {
	UpdateSession(*dashscope.SessionConfig) error
	AppendAudio([]byte) error
	CommitInput() error
	CreateResponse(*dashscope.ResponseCreateOptions) error
	SubmitFunctionCallOutput(string, string) error
	CancelResponse() error
	Events() iter.Seq2[*dashscope.RealtimeEvent, error]
	Close() error
}

// DashScopeStream is a Stream returned by DashScopeRealtime.Transform().
// It provides methods to dynamically update session configuration.
type DashScopeStream struct {
	*bufferStream
	session     *dashscope.RealtimeSession
	transformer *DashScopeRealtime
}

// UpdateRequest contains fields that can be updated mid-session.
// Use pointer fields to distinguish "not set" from "set to empty".
type UpdateRequest struct {
	// Voice is the TTS voice ID.
	// Available voices: Chelsie, Cherry, Serena, Ethan (and more for Flash model).
	Voice *string

	// Instructions is the system prompt.
	Instructions *string

	// Modalities specifies output modalities.
	// Use ["text"] for text-only, or ["text", "audio"] for both.
	Modalities []string

	// InputAudioFormat specifies input audio format (e.g., "pcm16").
	InputAudioFormat *string

	// OutputAudioFormat specifies output audio format (e.g., "pcm16", "mp3").
	OutputAudioFormat *string

	// TurnDetection configures VAD settings.
	TurnDetection *dashscope.TurnDetection
}

// Update updates the session configuration.
// Only non-nil fields are included in the update request.
func (s *DashScopeStream) Update(req *UpdateRequest) error {
	config := &dashscope.SessionConfig{}

	if req.Voice != nil {
		config.Voice = *req.Voice
	}
	if req.Instructions != nil {
		config.Instructions = *req.Instructions
	}
	if len(req.Modalities) > 0 {
		config.Modalities = req.Modalities
	}
	if req.InputAudioFormat != nil {
		config.InputAudioFormat = *req.InputAudioFormat
	}
	if req.OutputAudioFormat != nil {
		config.OutputAudioFormat = *req.OutputAudioFormat
	}
	if req.TurnDetection != nil {
		config.TurnDetection = req.TurnDetection
	}

	return s.session.UpdateSession(config)
}

// CancelResponse cancels the current response being generated.
// Use this to interrupt the AI when the user starts speaking.
func (s *DashScopeStream) CancelResponse() error {
	return s.session.CancelResponse()
}

// ClearAudioBuffer clears the input audio buffer.
func (s *DashScopeStream) ClearAudioBuffer() error {
	return s.session.ClearInput()
}

// TriggerResponse commits the current input audio and requests a response.
// Use this in manual mode (without VAD) to trigger the AI to respond.
func (s *DashScopeStream) TriggerResponse() error {
	if err := s.session.CommitInput(); err != nil {
		return err
	}
	return s.session.CreateResponse(nil)
}

// Transform converts audio input to audio output via Qwen-Omni realtime.
// It synchronously waits for the WebSocket connection to be established
// and session.created event to be received before returning.
func (t *DashScopeRealtime) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	runtime, _ := ctx.Value(dashScopeRealtimeCtxKey{}).(DashScopeRealtimeCtxOptions)
	model := t.model
	if value := strings.TrimSpace(runtime.Model); value != "" {
		model = value
	}
	// Connect to realtime service
	session, err := t.client.Realtime.Connect(ctx, &dashscope.RealtimeConfig{
		Model: model,
	})
	if err != nil {
		return nil, fmt.Errorf("dashscope connect: %w", err)
	}

	// Wait for session.created event
	var sessionCreated bool
	for event, err := range session.Events() {
		if err != nil {
			session.Close()
			return nil, fmt.Errorf("dashscope wait session: %w", err)
		}
		if event.Type == dashscope.EventTypeSessionCreated {
			sessionCreated = true
			break
		}
	}

	if !sessionCreated {
		session.Close()
		return nil, fmt.Errorf("dashscope: session.created not received")
	}

	// Update session configuration
	sessionConfig := &dashscope.SessionConfig{
		Voice:                         t.voice,
		Modalities:                    t.modalities,
		Instructions:                  t.instructions,
		EnableInputAudioTranscription: t.enableInputAudioTranscription,
		InputAudioTranscriptionModel:  t.inputAudioTranscriptionModel,
		Temperature:                   t.temperature,
		MaxOutputTokens:               t.maxOutputTokens,
		InputAudioFormat:              t.inputAudioFormat,
		OutputAudioFormat:             t.outputAudioFormat,
		Tools:                         append([]dashscope.FunctionTool(nil), runtime.Tools...),
	}

	// Configure turn detection (VAD)
	if t.turnDetection != nil {
		sessionConfig.TurnDetection = t.turnDetection
	} else if t.vadType != "" {
		sessionConfig.TurnDetection = &dashscope.TurnDetection{
			Type: t.vadType,
		}
	}

	if err := session.UpdateSession(sessionConfig); err != nil {
		session.Close()
		return nil, fmt.Errorf("dashscope update session: %w", err)
	}

	// Create output stream
	output := newBufferStream(100)
	stream := &DashScopeStream{
		bufferStream: output,
		session:      session,
		transformer:  t,
	}

	// Start background processing
	go t.processLoop(ctx, input, output, session, runtime)

	return stream, nil
}

func (t *DashScopeRealtime) processLoop(ctx context.Context, input genx.Stream, output *bufferStream, session dashScopeRealtimeSession, runtime DashScopeRealtimeCtxOptions) {
	defer output.Close()
	defer session.Close()
	inputReader := newRealtimeInputReader(input)
	defer inputReader.Close()

	// StreamID tracking for correlating input/output
	// We use a queue because input and output are processed asynchronously.
	// Input StreamIDs are queued as they arrive, and popped when a response starts.
	var streamIDMu sync.Mutex
	var streamIDQueue []string
	var responseStreamID string
	var providerResponseID string
	responseAllowed := true
	ignoredResponseIDs := make(map[string]struct{})
	assistant := newRealtimeAssistantLifecycle()
	output.setOutputObserver(func(chunk *genx.MessageChunk) {
		observeRealtimeAssistantOutput(assistant, dashScopeRealtimeAssistantLabel, chunk)
	})
	defer output.setOutputObserver(nil)

	// Push a new input StreamID to the queue
	pushStreamID := func(id string) {
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		// Only push if different from the last one
		if len(streamIDQueue) == 0 || streamIDQueue[len(streamIDQueue)-1] != id {
			streamIDQueue = append(streamIDQueue, id)
		}
	}

	// Pop the input route used by transcription. Assistant responses always use
	// their own fresh StreamID and never reuse a device input ID.
	popStreamIDForResponse := func() string {
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		if len(streamIDQueue) > 0 {
			inputID := streamIDQueue[0]
			streamIDQueue = streamIDQueue[1:]
			return inputID
		}
		return ""
	}

	// Get the current response StreamID
	getResponseStreamID := func() string {
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		return responseStreamID
	}
	allowProviderResponse := func() {
		streamIDMu.Lock()
		responseAllowed = true
		streamIDMu.Unlock()
	}
	suppressProviderResponse := func() {
		streamIDMu.Lock()
		responseAllowed = false
		if providerResponseID != "" {
			ignoredResponseIDs[providerResponseID] = struct{}{}
			providerResponseID = ""
		}
		streamIDMu.Unlock()
	}
	beginProviderResponse := func(id string) (string, bool, bool) {
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		if !responseAllowed {
			if id != "" {
				ignoredResponseIDs[id] = struct{}{}
			}
			return "", false, false
		}
		providerResponseID = id
		fresh := responseStreamID == ""
		if responseStreamID == "" {
			responseStreamID = genx.NewStreamID()
		}
		return responseStreamID, true, fresh
	}
	ignoreProviderEvent := func(event *dashscope.RealtimeEvent) bool {
		if event == nil || event.ResponseID == "" {
			return false
		}
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		_, ignored := ignoredResponseIDs[event.ResponseID]
		if ignored && event.Type == dashscope.EventTypeResponseDone {
			delete(ignoredResponseIDs, event.ResponseID)
		}
		return ignored
	}
	finishProviderResponse := func(id string, clearStream bool) {
		streamIDMu.Lock()
		if id == "" || providerResponseID == id {
			providerResponseID = ""
		}
		if clearStream {
			responseStreamID = ""
		}
		streamIDMu.Unlock()
	}

	// Start goroutine to receive events
	eventsDone := make(chan struct{})
	var toolMu sync.Mutex
	var cancelTool context.CancelCauseFunc
	var toolCallsUsed atomic.Int64
	var pendingMu sync.Mutex
	var pendingCalls []DashScopeRealtimeFunctionCall
	clearPendingCalls := func() {
		pendingMu.Lock()
		pendingCalls = nil
		pendingMu.Unlock()
	}
	takePendingCalls := func() []DashScopeRealtimeFunctionCall {
		pendingMu.Lock()
		calls := pendingCalls
		pendingCalls = nil
		pendingMu.Unlock()
		return calls
	}
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
	interruptAssistant := func() {
		suppressProviderResponse()
		clearPendingCalls()
		interruption := assistant.interruptRoutes(getResponseStreamID(), false)
		if !interruption.interrupted {
			return
		}
		cancelPendingTool(errors.New("interrupted"))
		output.discard(func(chunk *genx.MessageChunk) bool {
			return chunk != nil && chunk.Role == genx.RoleModel && chunk.Ctrl != nil && chunk.Ctrl.StreamID == interruption.streamID
		})
		if interruption.textOpen {
			_ = output.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{StreamID: interruption.streamID, Label: dashScopeRealtimeAssistantLabel, EndOfStream: true, Error: "interrupted"},
			})
		}
		if interruption.audioOpen {
			_ = output.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: &genx.Blob{MIMEType: t.getOutputAudioMIMEType()},
				Ctrl: &genx.StreamCtrl{StreamID: interruption.streamID, Label: dashScopeRealtimeAssistantLabel, EndOfStream: true, Error: "interrupted"},
			})
		}
	}
	go func() {
		defer close(eventsDone)
		var transcriptionStreamID string
		var textRouteOpen bool
		var audioRouteOpen bool
		finishExternalResponse := func(streamID string) bool {
			if textRouteOpen {
				if err := output.Push(&genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeAssistantLabel, EndOfStream: true},
				}); err != nil {
					return false
				}
			}
			if audioRouteOpen {
				if err := output.Push(&genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: t.getOutputAudioMIMEType()},
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeAssistantLabel, EndOfStream: true},
				}); err != nil {
					return false
				}
			}
			textRouteOpen = false
			audioRouteOpen = false
			return true
		}
		startProviderResponse := func(providerID string) (string, bool, error) {
			streamID, accepted, fresh := beginProviderResponse(providerID)
			if !accepted || !fresh {
				return streamID, accepted, nil
			}
			textRouteOpen = false
			audioRouteOpen = true
			assistant.setAccept(true)
			assistant.nextEpoch()
			assistant.markStarted(streamID)
			if err := output.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: &genx.Blob{MIMEType: t.getOutputAudioMIMEType()},
				Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeAssistantLabel, BeginOfStream: true},
			}); err != nil {
				return "", false, err
			}
			return streamID, true, nil
		}
		for event, err := range session.Events() {
			if err != nil {
				output.CloseWithError(err)
				return
			}
			if ignoreProviderEvent(event) {
				continue
			}

			// Pop StreamID for response on:
			// 1. response.created - start of a new response cycle
			// 2. input_audio_transcription.completed - ASR marks end of user turn
			// This handles servers that may not send response.created
			switch event.Type {
			case dashscope.EventTypeInputSpeechStarted:
				// User started speaking - cancel current response
				slog.Info("dashscope: speech started - canceling response")
				toolCallsUsed.Store(0)
				interruptAssistant()
				if err := session.CancelResponse(); err != nil {
					slog.Error("dashscope: cancel response error", "error", err)
				}

			case dashscope.EventTypeInputSpeechStopped:
				allowProviderResponse()

			case dashscope.EventTypeResponseCreated:
				_, accepted, err := startProviderResponse(event.ResponseID)
				if err != nil {
					return
				}
				if !accepted {
					continue
				}

			case dashscope.EventTypeInputAudioTranscriptionCompleted:
				allowProviderResponse()
				if transcriptionStreamID == "" {
					transcriptionStreamID = popStreamIDForResponse()
				}
				streamID := transcriptionStreamID
				// ASR result for user input - emit text then EOS
				if event.Transcript != "" {
					outChunk := &genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text(event.Transcript),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeTranscriptLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
					// Emit ASR EOS
					eosChunk := &genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text(""),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeTranscriptLabel, EndOfStream: true},
					}
					if err := output.Push(eosChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseTextDelta:
				if !assistant.acceptsOutput() {
					continue
				}
				streamID, accepted, err := startProviderResponse(event.ResponseID)
				if err != nil {
					return
				}
				if !accepted {
					continue
				}
				// Model text response
				if event.Delta != "" {
					textRouteOpen = true
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Delta),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeAssistantLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseTextDone:
				// Route EOS is delayed until the outer Agent response finishes.
				// A provider response.done with ToolCalls ends only one model round.

			case dashscope.EventTypeResponseTranscriptDelta:
				if !assistant.acceptsOutput() {
					continue
				}
				streamID, accepted, err := startProviderResponse(event.ResponseID)
				if err != nil {
					return
				}
				if !accepted {
					continue
				}
				// TTS transcript (what the model is saying)
				if event.Delta != "" {
					textRouteOpen = true
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Delta),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeAssistantLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseTranscriptDone:
				// Delayed until the outer Agent response finishes.

			case dashscope.EventTypeResponseAudioDelta:
				if !assistant.acceptsOutput() {
					continue
				}
				streamID, accepted, err := startProviderResponse(event.ResponseID)
				if err != nil {
					return
				}
				if !accepted {
					continue
				}
				// Audio response
				if len(event.Audio) > 0 {
					audioRouteOpen = true
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{
							MIMEType: t.getOutputAudioMIMEType(),
							Data:     event.Audio,
						},
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeAssistantLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseAudioDone:
				// Delayed until the outer Agent response finishes.

			case dashscope.EventTypeChoicesResponse:
				if !assistant.acceptsOutput() {
					continue
				}
				streamID, accepted, err := startProviderResponse(event.ResponseID)
				if err != nil {
					return
				}
				if !accepted {
					continue
				}
				// DashScope's choices format response
				if event.Delta != "" {
					textRouteOpen = true
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Delta),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: dashScopeRealtimeAssistantLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseFunctionCallArgumentsDone:
				if !assistant.acceptsOutput() {
					continue
				}
				pendingMu.Lock()
				pendingCalls = append(pendingCalls, DashScopeRealtimeFunctionCall{
					CallID:      event.CallID,
					Name:        event.Name,
					Arguments:   event.Arguments,
					OutputIndex: event.OutputIndex,
				})
				pendingMu.Unlock()

			case dashscope.EventTypeResponseDone:
				calls := takePendingCalls()
				if !assistant.acceptsOutput() {
					finishProviderResponse(event.ResponseID, false)
					continue
				}
				if _, accepted, err := startProviderResponse(event.ResponseID); err != nil {
					return
				} else if !accepted {
					continue
				}
				if len(calls) == 0 {
					if !finishExternalResponse(getResponseStreamID()) {
						return
					}
					finishProviderResponse(event.ResponseID, true)
					transcriptionStreamID = ""
					continue
				}
				if runtime.FunctionCallHandler == nil {
					output.CloseWithError(fmt.Errorf("dashscope realtime function-call handler is required"))
					return
				}
				if runtime.MaxToolCalls > 0 && toolCallsUsed.Add(int64(len(calls))) > int64(runtime.MaxToolCalls) {
					err := fmt.Errorf("dashscope realtime tool call limit exceeded: maximum %d", runtime.MaxToolCalls)
					output.CloseWithError(err)
					return
				}
				sort.SliceStable(calls, func(i, j int) bool { return calls[i].OutputIndex < calls[j].OutputIndex })
				toolCtx, cancel := context.WithCancelCause(ctx)
				toolMu.Lock()
				cancelTool = cancel
				toolMu.Unlock()
				outputs, callErr := runtime.FunctionCallHandler(toolCtx, append([]DashScopeRealtimeFunctionCall(nil), calls...))
				toolMu.Lock()
				cancelTool = nil
				toolMu.Unlock()
				interrupted := context.Cause(toolCtx) != nil && ctx.Err() == nil
				if callErr != nil {
					cancel(callErr)
					if interrupted {
						continue
					}
					output.CloseWithError(callErr)
					return
				}
				if interrupted {
					continue
				}
				if len(outputs) != len(calls) {
					err := fmt.Errorf("dashscope realtime function-call handler returned %d outputs for %d calls", len(outputs), len(calls))
					cancel(err)
					output.CloseWithError(err)
					return
				}
				for i, result := range outputs {
					if result.CallID != calls[i].CallID {
						err := fmt.Errorf("dashscope realtime function result ID %q does not match call ID %q", result.CallID, calls[i].CallID)
						cancel(err)
						output.CloseWithError(err)
						return
					}
					if err := session.SubmitFunctionCallOutput(result.CallID, result.Output); err != nil {
						cancel(err)
						output.CloseWithError(err)
						return
					}
				}
				finishProviderResponse(event.ResponseID, false)
				if err := session.CreateResponse(nil); err != nil {
					cancel(err)
					output.CloseWithError(err)
					return
				}
				cancel(nil)

			case dashscope.EventTypeError:
				// Business error event - log but don't close session
				// Examples: "Conversation has none active response" when CancelResponse
				// is called without an active response
				if event.Error != nil {
					slog.Warn("dashscope error event",
						"code", event.Error.Code,
						"message", event.Error.Message,
						"type", event.Error.Type)
				}
			}
		}
	}()

	// Audio buffer for rate-limited sending
	// DashScope expects PCM16 at 16kHz, so 100ms = 3200 bytes
	const chunkSize = 3200 // 100ms at 16kHz PCM16
	var audioBuffer []byte

	// Send audio to realtime service
	for {
		select {
		case <-eventsDone:
			return
		default:
		}

		chunk, err, done := inputReader.NextOrDone(eventsDone)
		if done {
			return
		}
		if err != nil {
			if err != io.EOF {
				output.CloseWithError(err)
			}

			// Flush remaining audio buffer
			for len(audioBuffer) > 0 {
				sendSize := min(chunkSize, len(audioBuffer))
				if err := session.AppendAudio(audioBuffer[:sendSize]); err != nil {
					output.CloseWithError(err)
					return
				}
				audioBuffer = audioBuffer[sendSize:]
				time.Sleep(30 * time.Millisecond)
			}

			// Send trailing silence to ensure speech_stopped is detected
			// 2 seconds of silence at 16kHz PCM16 = 64000 bytes
			trailingSilence := make([]byte, 64000)
			for i := 0; i < len(trailingSilence); i += chunkSize {
				end := min(i+chunkSize, len(trailingSilence))
				if err := session.AppendAudio(trailingSilence[i:end]); err != nil {
					output.CloseWithError(err)
					return
				}
				time.Sleep(30 * time.Millisecond)
			}

			// Commit audio and request response (manual mode)
			time.Sleep(200 * time.Millisecond)
			if err := session.CommitInput(); err != nil {
				output.CloseWithError(err)
				return
			}
			if err := session.CreateResponse(nil); err != nil {
				output.CloseWithError(err)
				return
			}
			// Wait for remaining events
			<-eventsDone
			return
		}

		if chunk == nil {
			continue
		}

		// Track StreamID from input chunks - push to queue for response correlation
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
			pushStreamID(chunk.Ctrl.StreamID)
		}

		// Cancel ongoing response when new user input starts (BOS)
		// This interrupts the AI to let the user speak
		// If no response is active, server returns an error event which we log and ignore
		if chunk.Ctrl != nil && chunk.Ctrl.BeginOfStream {
			toolCallsUsed.Store(0)
			interruptAssistant()
			finishProviderResponse("", true)
			_ = session.CancelResponse()
		}

		// Collect audio blob into buffer
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			audioBuffer = append(audioBuffer, blob.Data...)

			// Send audio in chunks with rate limiting
			for len(audioBuffer) >= chunkSize {
				if err := session.AppendAudio(audioBuffer[:chunkSize]); err != nil {
					output.CloseWithError(err)
					return
				}
				audioBuffer = audioBuffer[chunkSize:]
				time.Sleep(30 * time.Millisecond) // ~3x real-time
			}

			// On audio EOS, flush buffer and trigger response
			if chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
				// Flush remaining audio
				if len(audioBuffer) > 0 {
					if err := session.AppendAudio(audioBuffer); err != nil {
						output.CloseWithError(err)
						return
					}
					audioBuffer = nil
				}
				// Trigger response
				time.Sleep(100 * time.Millisecond)
				allowProviderResponse()
				if err := session.CommitInput(); err != nil {
					output.CloseWithError(err)
					return
				}
				if err := session.CreateResponse(nil); err != nil {
					output.CloseWithError(err)
					return
				}
			}
		}
	}
}
