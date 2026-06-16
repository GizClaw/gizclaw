package transformers

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkg/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

// DoubaoRealtime is a realtime transformer using Doubao realtime dialogue.
//
// Resource ID: volc.speech.dialog
//
// This is a bidirectional transformer:
// Input: genx.Stream with audio Blob chunks (user audio)
// Output: genx.Stream with audio Blob chunks (model response)
//
// Internally uses ASR → LLM → TTS pipeline.
type DoubaoRealtime struct {
	client            *doubaospeech.Client
	speaker           string
	format            string
	sampleRate        int
	channels          int
	inputFormat       string
	inputSampleRate   int
	inputChannels     int
	inputTranscode    bool
	botName           string
	systemRole        string
	vadWindowMs       int // VAD end detection window in milliseconds
	speakingStyle     string
	characterManifest string
	model             string // Model version: O, SC, 1.2.1.0 (O2.0), 2.2.0.0 (SC2.0)
}

var _ genx.Transformer = (*DoubaoRealtime)(nil)

const (
	doubaoRealtimeTranscriptLabel = "transcript"
	doubaoRealtimeAssistantLabel  = "assistant"

	doubaoRealtimeFixedInputFormat      = "speech_opus"
	doubaoRealtimeFixedInputSampleRate  = 16000
	doubaoRealtimeFixedInputChannels    = 1
	doubaoRealtimeFixedOutputFormat     = "ogg_opus"
	doubaoRealtimeFixedOutputSampleRate = 24000
	doubaoRealtimeFixedOutputChannels   = 1
)

// DoubaoRealtimeOption is a functional option for DoubaoRealtime.
type DoubaoRealtimeOption func(*DoubaoRealtime)

// WithDoubaoRealtimeSpeaker sets the TTS speaker voice.
func WithDoubaoRealtimeSpeaker(speaker string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.speaker = speaker
	}
}

// WithDoubaoRealtimeFormat sets the TTS output audio format.
func WithDoubaoRealtimeFormat(format string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.format = format
	}
}

// WithDoubaoRealtimeSampleRate sets the sample rate.
func WithDoubaoRealtimeSampleRate(sampleRate int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.sampleRate = sampleRate
	}
}

// WithDoubaoRealtimeChannels sets the number of channels.
func WithDoubaoRealtimeChannels(channels int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.channels = channels
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

// WithDoubaoRealtimeInputChannels sets the input audio channel count sent to Doubao.
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

// WithDoubaoRealtimeBotName sets the bot name.
func WithDoubaoRealtimeBotName(botName string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.botName = botName
	}
}

// WithDoubaoRealtimeSystemRole sets the system role/prompt.
func WithDoubaoRealtimeSystemRole(systemRole string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.systemRole = systemRole
	}
}

// WithDoubaoRealtimeVADWindow sets the VAD end detection window in milliseconds.
// Smaller values (100-200ms) give faster response but may cut off speech.
// Larger values (500-1000ms) are more tolerant of pauses but slower.
func WithDoubaoRealtimeVADWindow(windowMs int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.vadWindowMs = windowMs
	}
}

// WithDoubaoRealtimeSpeakingStyle sets the speaking style.
func WithDoubaoRealtimeSpeakingStyle(style string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.speakingStyle = style
	}
}

// WithDoubaoRealtimeCharacterManifest sets the character manifest for role-playing.
func WithDoubaoRealtimeCharacterManifest(manifest string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.characterManifest = manifest
	}
}

// WithDoubaoRealtimeModel sets the model version.
// Valid values: "O" (default), "SC", "1.2.1.0" (O2.0), "2.2.0.0" (SC2.0)
func WithDoubaoRealtimeModel(model string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.model = model
	}
}

// NewDoubaoRealtime creates a new DoubaoRealtime transformer.
//
// Parameters:
//   - client: Doubao speech client
//   - opts: Optional configuration
func NewDoubaoRealtime(client *doubaospeech.Client, opts ...DoubaoRealtimeOption) *DoubaoRealtime {
	t := &DoubaoRealtime{
		client:          client,
		speaker:         "zh_female_vv_jupiter_bigtts", // O version default voice
		format:          doubaoRealtimeFixedOutputFormat,
		sampleRate:      doubaoRealtimeFixedOutputSampleRate,
		channels:        doubaoRealtimeFixedOutputChannels,
		inputFormat:     doubaoRealtimeFixedInputFormat,
		inputSampleRate: doubaoRealtimeFixedInputSampleRate,
		inputChannels:   doubaoRealtimeFixedInputChannels,
		inputTranscode:  true,
		vadWindowMs:     200,  // Default VAD window
		model:           "O",  // Default to O version
		botName:         "豆包", // Default bot name
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// DoubaoRealtimeCtxKey is the context key for runtime options.
type doubaoRealtimeCtxKey struct{}

// DoubaoRealtimeCtxOptions are runtime options passed via context.
// TODO: Add fields as needed for runtime configuration.
type DoubaoRealtimeCtxOptions struct{}

// WithDoubaoRealtimeCtxOptions attaches runtime options to context.
func WithDoubaoRealtimeCtxOptions(ctx context.Context, opts DoubaoRealtimeCtxOptions) context.Context {
	return context.WithValue(ctx, doubaoRealtimeCtxKey{}, opts)
}

// Transform converts audio input to audio output via realtime dialogue.
// It synchronously waits for the connection to be established before returning.
func (t *DoubaoRealtime) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	// Build config with ASR settings
	inputFormat := doubaoRealtimeAudioFormat(t.inputFormat)
	inputSampleRate := doubaoRealtimeAudioSampleRate(t.inputSampleRate)
	inputChannels := doubaoRealtimeAudioChannels(t.inputChannels)
	config := &doubaospeech.RealtimeConfig{
		ASR: doubaospeech.RealtimeASRConfig{
			Extra: map[string]any{
				"end_smooth_window_ms": t.vadWindowMs,
				"audio_info": map[string]any{
					"format":      inputFormat,
					"sample_rate": inputSampleRate,
					"channel":     inputChannels,
				},
			},
		},
		TTS: doubaospeech.RealtimeTTSConfig{
			Speaker: t.speaker,
			AudioConfig: doubaospeech.RealtimeAudioConfig{
				Format:     doubaospeech.AudioFormat(t.format),
				SampleRate: doubaospeech.SampleRate(t.sampleRate),
				Channel:    t.channels,
			},
		},
		Dialog: doubaospeech.RealtimeDialogConfig{
			BotName:           t.botName,
			SystemRole:        t.systemRole,
			SpeakingStyle:     t.speakingStyle,
			CharacterManifest: t.characterManifest,
			Extra: map[string]any{
				"model": t.model, // Model version: O, SC, etc.
			},
		},
	}
	slog.Info(
		"doubao: realtime session config",
		"inputFormat", inputFormat,
		"inputSampleRate", inputSampleRate,
		"inputChannels", inputChannels,
		"inputTranscode", t.inputTranscode,
		"outputFormat", t.format,
		"outputSampleRate", t.sampleRate,
		"outputChannels", t.channels,
	)

	// Connect to realtime service (synchronous)
	session, err := t.client.Realtime.Connect(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("doubao realtime connect: %w", err)
	}

	output := newBufferStream(100)
	go t.processLoop(input, output, session)

	return output, nil
}

func (t *DoubaoRealtime) processLoop(input genx.Stream, output *bufferStream, session *doubaospeech.RealtimeSession) {
	defer output.Close()
	defer session.Close()

	// StreamID management - queue for correlating input to output
	var streamIDMu sync.Mutex
	var streamIDQueue []string
	var inputStreamID string
	var responseStreamID string

	pushStreamID := func(id string) {
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		streamIDQueue = append(streamIDQueue, id)
		inputStreamID = id
		slog.Debug("doubao: pushed streamID", "id", id, "queueLen", len(streamIDQueue))
	}

	popStreamIDForResponse := func() {
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		if len(streamIDQueue) > 0 {
			responseStreamID = streamIDQueue[0]
			streamIDQueue = streamIDQueue[1:]
			slog.Debug("doubao: popped streamID for response", "id", responseStreamID)
		} else {
			// Generate a new StreamID if queue is empty
			responseStreamID = genx.NewStreamID()
			slog.Debug("doubao: generated new streamID for response", "id", responseStreamID)
		}
	}

	getResponseStreamID := func() string {
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		return responseStreamID
	}
	getInputStreamID := func() string {
		streamIDMu.Lock()
		defer streamIDMu.Unlock()
		return inputStreamID
	}

	// Start goroutine to receive events
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		lastTranscriptText := ""
		for event, err := range session.Recv() {
			if err != nil {
				slog.Error("doubao: recv error", "error", err)
				output.CloseWithError(err)
				return
			}

			slog.Debug("doubao: received event", "type", event.Type, "text", event.Text, "audioLen", len(event.Audio))

			// Get StreamID for this response
			streamID := getResponseStreamID()

			// Handle different event types
			switch event.Type {
			case doubaospeech.EventASRInfo:
				// ASR detected speech - log for debugging
				// Note: Do NOT interrupt here. EventASRInfo is just speech detection,
				// not a user interruption. Interruption should be handled by the
				// cortex layer based on device state changes (e.g., user pressing button).
				slog.Info("doubao: ASR info - speech detected")

			case doubaospeech.EventASRResponse:
				// ASR text result
				text := strings.TrimSpace(event.Text)
				if text == "" {
					text = realtimeASRText(event.Payload)
				}
				slog.Info("doubao: ASR response", "text", text)
				if text != "" {
					delta := realtimeTextDelta(lastTranscriptText, text)
					lastTranscriptText = text
					if delta == "" {
						continue
					}
					outChunk := &genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text(delta),
						Ctrl: &genx.StreamCtrl{StreamID: getInputStreamID(), Label: doubaoRealtimeTranscriptLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case doubaospeech.EventASREnded:
				// User speech ended - pop StreamID for upcoming response
				slog.Info("doubao: ASR ended")
				doneChunk := &genx.MessageChunk{
					Role: genx.RoleUser,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{
						StreamID:    getInputStreamID(),
						Label:       doubaoRealtimeTranscriptLabel,
						EndOfStream: true,
					},
				}
				if err := output.Push(doneChunk); err != nil {
					return
				}
				lastTranscriptText = ""
				popStreamIDForResponse()

			case doubaospeech.EventTTSStarted:
				// TTS started - send BOS to signal start of audio stream
				slog.Info("doubao: TTS started, sending BOS", "streamID", streamID)
				bosChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: t.outputMIMEType()},
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, BeginOfStream: true},
				}
				if err := output.Push(bosChunk); err != nil {
					return
				}

				// Also send text if available
				if event.Text != "" {
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Text),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case doubaospeech.EventChatResponse:
				// Model text response
				text := strings.TrimSpace(event.Text)
				if text != "" {
					slog.Debug("doubao: chat response", "text", text)
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(text),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
				}

			case doubaospeech.EventTTSAudioData:
				// Audio chunk received
				if len(event.Audio) > 0 {
					slog.Debug("doubao: audio received", "len", len(event.Audio))
					blobs, err := t.outputAudioBlobs(event.Audio)
					if err != nil {
						output.CloseWithError(err)
						return
					}
					for _, blob := range blobs {
						outChunk := &genx.MessageChunk{
							Role: genx.RoleModel,
							Part: blob,
							Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
						}
						if err := output.Push(outChunk); err != nil {
							return
						}
					}
				}

			case doubaospeech.EventTTSFinished:
				// TTS finished - send EOS to signal end of audio stream
				slog.Info("doubao: TTS finished, sending EOS", "streamID", streamID)
				eosChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: t.outputMIMEType()},
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
				}
				if err := output.Push(eosChunk); err != nil {
					return
				}
				// Don't return - continue listening for more events (multi-turn)

			case doubaospeech.EventChatEnded:
				// Model response ended (text complete, audio may follow)
				slog.Debug("doubao: chat ended")
				doneChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{
						StreamID:    streamID,
						Label:       doubaoRealtimeAssistantLabel,
						EndOfStream: true,
					},
				}
				if err := output.Push(doneChunk); err != nil {
					return
				}

			case doubaospeech.EventSessionFinished:
				// Session ended
				slog.Info("doubao: session ended")
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
			slog.Info("doubao: events done")
			return
		default:
		}

		chunk, err := input.Next()
		if err != nil {
			if err != io.EOF && err != genx.ErrDone {
				slog.Error("doubao: input error", "error", err)
				output.CloseWithError(err)
			} else {
				slog.Info("doubao: input EOF", "audioSent", audioSent)
			}
			// Wait for remaining events
			<-eventsDone
			return
		}

		if chunk == nil {
			continue
		}

		// Track StreamID from BOS marker only
		if chunk.IsBeginOfStream() && chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
			pushStreamID(chunk.Ctrl.StreamID)
			slog.Info("doubao: received BOS", "streamID", chunk.Ctrl.StreamID)
			continue
		}

		// Handle EOS - send silence to help VAD detect end of speech
		if chunk.IsEndOfStream() {
			streamID := chunkInputStreamID(chunk, getInputStreamID())
			slog.Info("doubao: received EOS, sending silence for VAD", "streamID", streamID, "audioSent", audioSent)
			t.sendVADSilence(session, audioInputs.stream(streamID))
			audioInputs.closeStream(streamID)
			// Don't return - wait for Doubao to process and respond
			continue
		}

		// Send based on part type
		switch p := chunk.Part.(type) {
		case *genx.Blob:
			// Send audio blob
			if len(p.Data) > 0 {
				streamID := chunkInputStreamID(chunk, getInputStreamID())
				audioInput, err := audioInputs.streamForBlob(streamID, p)
				if err != nil {
					slog.Error("doubao: prepare audio error", "error", err)
					t.pushInputEOSError(output, streamID, err)
					audioInputs.closeStream(streamID)
					output.CloseWithError(err)
					return
				}
				frames, err := audioInput.prepareFrames(p)
				if err != nil {
					slog.Error("doubao: prepare audio error", "error", err)
					t.pushInputEOSError(output, streamID, err)
					audioInputs.closeStream(streamID)
					output.CloseWithError(err)
					return
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
						output.CloseWithError(err)
						return
					}
				}
			}
		case genx.Text:
			// Send text query
			if len(p) > 0 {
				slog.Info("doubao: sending text", "text", string(p))
				if err := session.SendText(context.Background(), string(p)); err != nil {
					slog.Error("doubao: send text error", "error", err)
					output.CloseWithError(err)
					return
				}
			}
		}
	}
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

func (t *DoubaoRealtime) sendVADSilence(session *doubaospeech.RealtimeSession, audioInput *doubaoRealtimeAudioInput) {
	if session == nil {
		return
	}
	if audioInput == nil {
		audioInput = newDoubaoRealtimeAudioInput(t.inputFormat, t.inputSampleRate, t.inputChannels, t.inputTranscode)
		defer audioInput.close()
	}
	frames, err := audioInput.silenceFrames(50)
	if err != nil {
		slog.Error("doubao: prepare silence error", "error", err)
		return
	}
	for _, frame := range frames {
		if err := session.SendAudio(context.Background(), frame); err != nil {
			slog.Error("doubao: send silence error", "error", err)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
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
	switch strings.ToLower(strings.TrimSpace(t.format)) {
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
	if strings.EqualFold(strings.TrimSpace(t.format), "ogg_opus") {
		return "audio/opus"
	}
	return t.mimeType()
}

func (t *DoubaoRealtime) outputAudioBlobs(audio []byte) ([]*genx.Blob, error) {
	if len(audio) == 0 {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(t.format), "ogg_opus") {
		return []*genx.Blob{{MIMEType: t.mimeType(), Data: audio}}, nil
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
