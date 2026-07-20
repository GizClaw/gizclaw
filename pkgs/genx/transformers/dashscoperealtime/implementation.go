package dashscoperealtime

import (
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"sync"
	"time"

	"github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Transformer is a realtime transformer using DashScope Qwen-Omni-Realtime.
//
// Model: qwen-omni-turbo-realtime-latest (default) or qwen3-omni-flash-realtime
//
// This is a bidirectional transformer:
// Input: genx.Stream with audio Blob chunks (PCM16 16kHz)
// Output: genx.Stream with audio Blob chunks (PCM16 24kHz)
//
// Internally uses Qwen-Omni model for speech-to-speech.
type Transformer struct {
	client       *dashscope.Client
	realtime     dashScopeRealtimeOpener
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

type dashScopeRealtimeOpener interface {
	Connect(context.Context, *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error)
}

type dashScopeRealtimeSession interface {
	UpdateSession(*dashscope.SessionConfig) error
	AppendAudio([]byte) error
	CommitInput() error
	ClearInput() error
	CreateResponse(*dashscope.ResponseCreateOptions) error
	CancelResponse() error
	Events() iter.Seq2[*dashscope.RealtimeEvent, error]
	Close() error
}

type dashScopeRealtimeClient struct {
	client *dashscope.Client
}

func (c dashScopeRealtimeClient) Connect(ctx context.Context, config *dashscope.RealtimeConfig) (dashScopeRealtimeSession, error) {
	return c.client.Realtime.Connect(ctx, config)
}

type dashScopeStreamIDs struct {
	mu sync.Mutex

	turns             []*dashScopeStreamTurn
	responses         map[string]*dashScopeStreamTurn
	currentResponse   *dashScopeStreamTurn
	lastInputStreamID string
}

type dashScopeStreamTurn struct {
	inputStreamID              string
	responseStreamID           string
	responseTranscriptStreamID string
	transcriptionSeen          bool
	responseSeen               bool
}

func (s *dashScopeStreamIDs) pushInput(streamID string) {
	if streamID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if streamID == s.lastInputStreamID {
		return
	}
	s.lastInputStreamID = streamID
	s.turns = append(s.turns, &dashScopeStreamTurn{
		inputStreamID:    streamID,
		responseStreamID: genx.NewStreamID(),
	})
}

func (s *dashScopeStreamIDs) bindTranscription() (inputStreamID, responseStreamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, turn := range s.turns {
		if !turn.transcriptionSeen {
			turn.transcriptionSeen = true
			return turn.inputStreamID, turn.responseStreamID
		}
	}
	return "", ""
}

func (s *dashScopeStreamIDs) bindResponse(providerResponseID string) (inputStreamID, responseStreamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if providerResponseID != "" && s.responses != nil {
		if turn := s.responses[providerResponseID]; turn != nil {
			s.currentResponse = turn
			return turn.inputStreamID, turn.responseStreamID
		}
	}
	for _, turn := range s.turns {
		if !turn.responseSeen {
			turn.responseSeen = true
			s.rememberResponseLocked(providerResponseID, turn)
			return turn.inputStreamID, turn.responseStreamID
		}
	}
	turn := &dashScopeStreamTurn{
		responseStreamID:  genx.NewStreamID(),
		transcriptionSeen: true,
		responseSeen:      true,
	}
	s.turns = append(s.turns, turn)
	s.rememberResponseLocked(providerResponseID, turn)
	return "", turn.responseStreamID
}

func (s *dashScopeStreamIDs) response(providerResponseID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if providerResponseID != "" && s.responses != nil {
		if turn := s.responses[providerResponseID]; turn != nil {
			return turn.responseStreamID
		}
	}
	if s.currentResponse != nil && providerResponseID == "" {
		return s.currentResponse.responseStreamID
	}
	for _, turn := range s.turns {
		if !turn.responseSeen {
			turn.responseSeen = true
			s.rememberResponseLocked(providerResponseID, turn)
			return turn.responseStreamID
		}
	}
	turn := &dashScopeStreamTurn{
		responseStreamID:  genx.NewStreamID(),
		transcriptionSeen: true,
		responseSeen:      true,
	}
	s.turns = append(s.turns, turn)
	s.rememberResponseLocked(providerResponseID, turn)
	return turn.responseStreamID
}

func (s *dashScopeStreamIDs) responseTranscript(providerResponseID string) string {
	responseStreamID := s.response(providerResponseID)
	s.mu.Lock()
	defer s.mu.Unlock()
	var turn *dashScopeStreamTurn
	if providerResponseID != "" && s.responses != nil {
		turn = s.responses[providerResponseID]
	}
	if turn == nil && s.currentResponse != nil && s.currentResponse.responseStreamID == responseStreamID {
		turn = s.currentResponse
	}
	if turn == nil {
		for _, candidate := range s.turns {
			if candidate.responseStreamID == responseStreamID {
				turn = candidate
				break
			}
		}
	}
	if turn == nil {
		return genx.NewStreamID()
	}
	if turn.responseTranscriptStreamID == "" {
		turn.responseTranscriptStreamID = genx.NewStreamID()
	}
	return turn.responseTranscriptStreamID
}

func (s *dashScopeStreamIDs) rememberResponseLocked(providerResponseID string, turn *dashScopeStreamTurn) {
	s.currentResponse = turn
	if providerResponseID == "" {
		return
	}
	if s.responses == nil {
		s.responses = make(map[string]*dashScopeStreamTurn)
	}
	s.responses[providerResponseID] = turn
}

func dashScopeResponseID(event *dashscope.RealtimeEvent) string {
	if event.ResponseID != "" {
		return event.ResponseID
	}
	if event.Response != nil {
		return event.Response.ID
	}
	return ""
}

var _ genx.Transformer = (*Transformer)(nil)

// option is a functional option for Transformer.
type option func(*Transformer)

// withModel sets the model.
// Options: qwen-omni-turbo-realtime-latest, qwen3-omni-flash-realtime
func withModel(model string) option {
	return func(t *Transformer) {
		t.model = model
	}
}

// withVoice sets the TTS voice.
// Options: Chelsie, Cherry, Serena, Ethan
func withVoice(voice string) option {
	return func(t *Transformer) {
		t.voice = voice
	}
}

// withInstructions sets the system prompt.
func withInstructions(instructions string) option {
	return func(t *Transformer) {
		t.instructions = instructions
	}
}

// withModalities sets the output modalities.
// Options: ["text"], ["audio"], ["text", "audio"]
func withModalities(modalities []string) option {
	return func(t *Transformer) {
		t.modalities = modalities
	}
}

// withVAD sets the VAD mode.
// Options: server_vad, disabled (empty string means manual mode)
func withVAD(vadType string) option {
	return func(t *Transformer) {
		t.vadType = vadType
	}
}

// withTemperature sets the temperature for response generation.
// Range: 0.0-2.0. Higher values make output more random.
func withTemperature(temp float64) option {
	return func(t *Transformer) {
		t.temperature = &temp
	}
}

// withMaxOutputTokens sets the maximum output tokens.
// Use -1 for unlimited.
func withMaxOutputTokens(tokens int) option {
	return func(t *Transformer) {
		t.maxOutputTokens = &tokens
	}
}

// withEnableASR enables input audio transcription (ASR).
// When enabled, the transformer will emit user speech transcription.
func withEnableASR(enable bool) option {
	return func(t *Transformer) {
		t.enableInputAudioTranscription = enable
	}
}

// withASRModel sets the model for input audio transcription.
// Example: "qwen-audio-turbo"
func withASRModel(model string) option {
	return func(t *Transformer) {
		t.inputAudioTranscriptionModel = model
	}
}

// withTurnDetection sets detailed VAD configuration.
// Use this for fine-grained control over voice activity detection.
func withTurnDetection(td *dashscope.TurnDetection) option {
	return func(t *Transformer) {
		t.turnDetection = td
	}
}

// withInputAudioFormat sets the input audio format.
// Options: pcm16 (default, 16kHz), mp3, wav
func withInputAudioFormat(format string) option {
	return func(t *Transformer) {
		t.inputAudioFormat = format
	}
}

// withOutputAudioFormat sets the output audio format.
// Options: pcm16 (default, 24kHz), mp3, wav
func withOutputAudioFormat(format string) option {
	return func(t *Transformer) {
		t.outputAudioFormat = format
	}
}

// newTransformer creates a Transformer.
//
// Parameters:
//   - client: DashScope client
//   - opts: Optional configuration
func newTransformer(client *dashscope.Client, opts ...option) *Transformer {
	t := &Transformer{
		client:                        client,
		realtime:                      dashScopeRealtimeClient{client: client},
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
func (t *Transformer) getOutputAudioMIMEType() string {
	switch t.outputAudioFormat {
	case dashscope.AudioFormatMP3:
		return "audio/mpeg"
	case dashscope.AudioFormatWAV:
		return "audio/wav"
	default:
		return "audio/pcm"
	}
}

// Stream is returned by Transformer.Transform and exposes supported live controls.
// It provides methods to dynamically update session configuration.
type Stream struct {
	genx.Stream
	session dashScopeRealtimeSession
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
func (s *Stream) Update(req *UpdateRequest) error {
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
func (s *Stream) CancelResponse() error {
	return s.session.CancelResponse()
}

// ClearAudioBuffer clears the input audio buffer.
func (s *Stream) ClearAudioBuffer() error {
	return s.session.ClearInput()
}

// TriggerResponse commits the current input audio and requests a response.
// Use this in manual mode (without VAD) to trigger the AI to respond.
func (s *Stream) TriggerResponse() error {
	if err := s.session.CommitInput(); err != nil {
		return err
	}
	return s.session.CreateResponse(nil)
}

// Transform converts audio input to audio output via Qwen-Omni realtime.
// It synchronously waits for the WebSocket connection to be established
// and session.created event to be received before returning.
func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.realtime == nil {
		return nil, fmt.Errorf("dashscope realtime: transformer is not initialized")
	}
	if input == nil {
		return nil, fmt.Errorf("dashscope realtime: input stream is required")
	}
	// Connect to realtime service
	session, err := t.realtime.Connect(ctx, &dashscope.RealtimeConfig{
		Model: t.model,
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
	stream := &Stream{
		Stream:  output,
		session: session,
	}

	// Start background processing
	go t.processLoop(input, output, session)

	return stream, nil
}

func (t *Transformer) processLoop(input genx.Stream, output *bufferStream, session dashScopeRealtimeSession) {
	defer output.Close()
	defer session.Close()

	// Input IDs correlate ASR output to the caller's route. Every model
	// response receives a fresh ID so its text/audio routes cannot collide with
	// the user transcript's text/plain EOS.
	streamIDs := &dashScopeStreamIDs{}

	// Start goroutine to receive events
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		for event, err := range session.Events() {
			if err != nil {
				output.CloseWithError(err)
				return
			}

			switch event.Type {
			case dashscope.EventTypeInputSpeechStarted:
				// User started speaking - cancel current response
				slog.Info("dashscope: speech started - canceling response")
				if err := session.CancelResponse(); err != nil {
					slog.Error("dashscope: cancel response error", "error", err)
				}

			case dashscope.EventTypeResponseCreated:
				_, responseStreamID := streamIDs.bindResponse(dashScopeResponseID(event))
				// Send BOS to signal start of new audio stream
				bosChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: t.getOutputAudioMIMEType()},
					Ctrl: &genx.StreamCtrl{StreamID: responseStreamID, BeginOfStream: true},
				}
				if err := output.Push(bosChunk); err != nil {
					return
				}

			case dashscope.EventTypeInputAudioTranscriptionCompleted:
				inputStreamID, _ := streamIDs.bindTranscription()
				// ASR result for user input - emit text then EOS
				if event.Transcript != "" {
					outChunk := &genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text(event.Transcript),
						Ctrl: &genx.StreamCtrl{StreamID: inputStreamID},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
					// Emit ASR EOS
					eosChunk := &genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text(""),
						Ctrl: &genx.StreamCtrl{StreamID: inputStreamID, EndOfStream: true},
					}
					if err := output.Push(eosChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseTextDelta:
				responseStreamID := streamIDs.response(dashScopeResponseID(event))
				// Model text response
				if event.Delta != "" {
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Delta),
						Ctrl: &genx.StreamCtrl{StreamID: responseStreamID},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseTextDone:
				responseStreamID := streamIDs.response(dashScopeResponseID(event))
				// Model text response done - emit EOS
				eosChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{StreamID: responseStreamID, EndOfStream: true},
				}
				if err := output.Push(eosChunk); err != nil {
					return
				}

			case dashscope.EventTypeResponseTranscriptDelta:
				responseStreamID := streamIDs.responseTranscript(dashScopeResponseID(event))
				// TTS transcript (what the model is saying)
				if event.Delta != "" {
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Delta),
						Ctrl: &genx.StreamCtrl{StreamID: responseStreamID},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseTranscriptDone:
				responseStreamID := streamIDs.responseTranscript(dashScopeResponseID(event))
				// TTS transcript done - emit text EOS
				eosChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{StreamID: responseStreamID, EndOfStream: true},
				}
				if err := output.Push(eosChunk); err != nil {
					return
				}

			case dashscope.EventTypeResponseAudioDelta:
				responseStreamID := streamIDs.response(dashScopeResponseID(event))
				// Audio response
				if len(event.Audio) > 0 {
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{
							MIMEType: t.getOutputAudioMIMEType(),
							Data:     event.Audio,
						},
						Ctrl: &genx.StreamCtrl{StreamID: responseStreamID},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case dashscope.EventTypeResponseAudioDone:
				responseStreamID := streamIDs.response(dashScopeResponseID(event))
				// Audio response done - emit EOS
				eosChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: t.getOutputAudioMIMEType()},
					Ctrl: &genx.StreamCtrl{StreamID: responseStreamID, EndOfStream: true},
				}
				if err := output.Push(eosChunk); err != nil {
					return
				}

			case dashscope.EventTypeChoicesResponse:
				responseStreamID := streamIDs.response(dashScopeResponseID(event))
				// DashScope's choices format response
				if event.Delta != "" {
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Delta),
						Ctrl: &genx.StreamCtrl{StreamID: responseStreamID},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

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

		chunk, err := input.Next()
		if err != nil {
			if err != io.EOF {
				output.CloseWithError(err)
			}

			// Flush remaining audio buffer
			for len(audioBuffer) > 0 {
				sendSize := chunkSize
				if sendSize > len(audioBuffer) {
					sendSize = len(audioBuffer)
				}
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
				end := i + chunkSize
				if end > len(trailingSilence) {
					end = len(trailingSilence)
				}
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
			streamIDs.pushInput(chunk.Ctrl.StreamID)
		}

		// Cancel ongoing response when new user input starts (BOS)
		// This interrupts the AI to let the user speak
		// If no response is active, server returns an error event which we log and ignore
		if chunk.Ctrl != nil && chunk.Ctrl.BeginOfStream {
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
