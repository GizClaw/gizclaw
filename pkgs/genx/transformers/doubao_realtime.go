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
	"sync/atomic"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
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
	realtime          doubaoRealtimeOpener
	speaker           string
	format            string
	sampleRate        int
	channels          int
	speechRate        *int
	loudnessRate      *int
	inputFormat       string
	inputSampleRate   int
	inputChannels     int
	inputTranscode    bool
	asrExtra          *doubaospeech.RealtimeASRExtra
	ttsExtra          *doubaospeech.RealtimeTTSExtra
	botName           string
	systemRole        string
	vadWindowMs       int
	speakingStyle     string
	characterManifest string
	dialogID          string
	dialogExtra       *doubaospeech.RealtimeDialogExtra
	model             string // Model version: O, SC, 1.2.1.0 (O2.0), 2.2.0.0 (SC2.0)
	mode              DoubaoRealtimeMode
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
	doubaoRealtimeFixedOutputChannels   = 1

	doubaoRealtimeOpusFrameDuration = 20 * time.Millisecond
)

// DoubaoRealtimeMode controls how client input boundaries are interpreted.
type DoubaoRealtimeMode string

const (
	DoubaoRealtimeModePushToTalk DoubaoRealtimeMode = "push_to_talk"
	DoubaoRealtimeModeRealtime   DoubaoRealtimeMode = "realtime"
	DoubaoRealtimeModeText       DoubaoRealtimeMode = "text"
)

type doubaoRealtimeOpener interface {
	OpenSession(context.Context, *doubaospeech.RealtimeConfig) (doubaoRealtimeSession, error)
}

type doubaoRealtimeSession interface {
	SendAudio(context.Context, []byte) error
	SendText(context.Context, string) error
	EndASR(context.Context) error
	Interrupt(context.Context) error
	Recv() iter.Seq2[*doubaospeech.RealtimeEvent, error]
	Close() error
}

type doubaoRealtimeClient struct {
	client *doubaospeech.Client
}

func (c doubaoRealtimeClient) OpenSession(ctx context.Context, cfg *doubaospeech.RealtimeConfig) (doubaoRealtimeSession, error) {
	if c.client == nil {
		return nil, fmt.Errorf("doubao realtime client is required")
	}
	return c.client.Realtime.Connect(ctx, cfg)
}

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

// WithDoubaoRealtimeSpeechRate sets the realtime TTS speech rate.
func WithDoubaoRealtimeSpeechRate(rate int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.speechRate = &rate
	}
}

// WithDoubaoRealtimeLoudnessRate sets the realtime TTS loudness rate.
func WithDoubaoRealtimeLoudnessRate(rate int) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.loudnessRate = &rate
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

// WithDoubaoRealtimeASRExtra sets provider-specific typed ASR options.
func WithDoubaoRealtimeASRExtra(extra doubaospeech.RealtimeASRExtra) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.asrExtra = cloneDoubaoRealtimeASRExtra(&extra)
	}
}

// WithDoubaoRealtimeTTSExtra sets provider-specific typed TTS options.
func WithDoubaoRealtimeTTSExtra(extra doubaospeech.RealtimeTTSExtra) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.ttsExtra = cloneDoubaoRealtimeTTSExtra(&extra)
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

// WithDoubaoRealtimeDialogID sets the stable dialog id used to continue
// provider-side conversation memory.
func WithDoubaoRealtimeDialogID(dialogID string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.dialogID = dialogID
	}
}

// WithDoubaoRealtimeDialogExtra sets provider-specific typed dialog options.
func WithDoubaoRealtimeDialogExtra(extra doubaospeech.RealtimeDialogExtra) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.dialogExtra = cloneDoubaoRealtimeDialogExtra(&extra)
	}
}

// WithDoubaoRealtimeSearchAPIKey fills the credential-backed web search key on
// an already enabled dialog search extra.
func WithDoubaoRealtimeSearchAPIKey(apiKey string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		if strings.TrimSpace(apiKey) == "" {
			return
		}
		if t.dialogExtra == nil {
			t.dialogExtra = &doubaospeech.RealtimeDialogExtra{}
		}
		t.dialogExtra.VolcWebsearchAPIKey = strings.TrimSpace(apiKey)
	}
}

// WithDoubaoRealtimeModel sets the model version.
// Valid values: "O" (default), "SC", "1.2.1.0" (O2.0), "2.2.0.0" (SC2.0)
func WithDoubaoRealtimeModel(model string) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.model = model
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

func withDoubaoRealtimeOpener(opener doubaoRealtimeOpener) DoubaoRealtimeOption {
	return func(t *DoubaoRealtime) {
		t.realtime = opener
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
		model:           "O",  // Default to O version
		botName:         "豆包", // Default bot name
		mode:            DoubaoRealtimeModePushToTalk,
	}
	for _, opt := range opts {
		opt(t)
	}
	if t.realtime == nil {
		t.realtime = doubaoRealtimeClient{client: client}
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
		"doubao: realtime session config",
		"inputFormat", doubaoRealtimeAudioFormat(t.inputFormat),
		"inputSampleRate", doubaoRealtimeAudioSampleRate(t.inputSampleRate),
		"inputChannels", doubaoRealtimeAudioChannels(t.inputChannels),
		"inputTranscode", t.inputTranscode,
		"inputMode", config.InputMode,
		"dialogID", t.dialogID,
		"outputFormat", t.format,
		"outputSampleRate", t.sampleRate,
		"outputChannels", t.channels,
	)

	output := newBufferStream(16)
	go t.sessionLoop(ctx, input, output)

	return output, nil
}

func (t *DoubaoRealtime) realtimeConfig() *doubaospeech.RealtimeConfig {
	asrExtra := cloneDoubaoRealtimeASRExtra(t.asrExtra)
	if t.vadWindowMs > 0 {
		if asrExtra == nil {
			asrExtra = &doubaospeech.RealtimeASRExtra{}
		}
		asrExtra.EndSmoothWindowMS = t.vadWindowMs
	}
	config := &doubaospeech.RealtimeConfig{
		ASR: doubaospeech.RealtimeASRConfig{
			AudioInfo: &doubaospeech.RealtimeASRAudioInfo{
				Format:     doubaospeech.AudioFormat(doubaoRealtimeAudioFormat(t.inputFormat)),
				SampleRate: doubaospeech.SampleRate(doubaoRealtimeAudioSampleRate(t.inputSampleRate)),
				Channel:    doubaoRealtimeAudioChannels(t.inputChannels),
			},
			Extra: asrExtra,
		},
		TTS: doubaospeech.RealtimeTTSConfig{
			Speaker: t.speaker,
			AudioConfig: doubaospeech.RealtimeAudioConfig{
				Format:     doubaospeech.AudioFormat(t.format),
				SampleRate: doubaospeech.SampleRate(t.sampleRate),
				Channel:    t.channels,
			},
			Extra: cloneDoubaoRealtimeTTSExtra(t.ttsExtra),
		},
		Dialog: doubaospeech.RealtimeDialogConfig{
			DialogID:          t.dialogID,
			BotName:           t.botName,
			SystemRole:        t.systemRole,
			SpeakingStyle:     t.speakingStyle,
			CharacterManifest: t.characterManifest,
			Extra:             cloneDoubaoRealtimeDialogExtra(t.dialogExtra),
		},
		InputMode: t.realtimeInputMode(),
		Model:     doubaospeech.RealtimeModelVersion(t.model),
	}
	if t.speechRate != nil {
		config.TTS.AudioConfig.SpeechRate = *t.speechRate
	}
	if t.loudnessRate != nil {
		config.TTS.AudioConfig.LoudnessRate = *t.loudnessRate
	}
	return config
}

func cloneDoubaoRealtimeASRExtra(extra *doubaospeech.RealtimeASRExtra) *doubaospeech.RealtimeASRExtra {
	if extra == nil {
		return nil
	}
	copied := *extra
	if extra.EnableCustomVAD != nil {
		value := *extra.EnableCustomVAD
		copied.EnableCustomVAD = &value
	}
	if extra.EnableASRTwopass != nil {
		value := *extra.EnableASRTwopass
		copied.EnableASRTwopass = &value
	}
	if extra.Context != nil {
		copied.Context = &doubaospeech.RealtimeASRContext{}
		if len(extra.Context.Hotwords) > 0 {
			copied.Context.Hotwords = append([]doubaospeech.RealtimeHotword(nil), extra.Context.Hotwords...)
		}
		if len(extra.Context.CorrectWords) > 0 {
			copied.Context.CorrectWords = make(map[string]string, len(extra.Context.CorrectWords))
			for key, value := range extra.Context.CorrectWords {
				copied.Context.CorrectWords[key] = value
			}
		}
	}
	return &copied
}

func cloneDoubaoRealtimeTTSExtra(extra *doubaospeech.RealtimeTTSExtra) *doubaospeech.RealtimeTTSExtra {
	if extra == nil {
		return nil
	}
	copied := *extra
	if extra.AIGCMetadata != nil {
		copied.AIGCMetadata = &doubaospeech.RealtimeAIGCMetadata{
			ContentProducer:   extra.AIGCMetadata.ContentProducer,
			ProduceID:         extra.AIGCMetadata.ProduceID,
			ContentPropagator: extra.AIGCMetadata.ContentPropagator,
			PropagateID:       extra.AIGCMetadata.PropagateID,
		}
		if extra.AIGCMetadata.Enable != nil {
			value := *extra.AIGCMetadata.Enable
			copied.AIGCMetadata.Enable = &value
		}
	}
	return &copied
}

func cloneDoubaoRealtimeDialogExtra(extra *doubaospeech.RealtimeDialogExtra) *doubaospeech.RealtimeDialogExtra {
	if extra == nil {
		return nil
	}
	copied := *extra
	if extra.StrictAudit != nil {
		value := *extra.StrictAudit
		copied.StrictAudit = &value
	}
	if extra.EnableVolcWebsearch != nil {
		value := *extra.EnableVolcWebsearch
		copied.EnableVolcWebsearch = &value
	}
	if extra.EnableMusic != nil {
		value := *extra.EnableMusic
		copied.EnableMusic = &value
	}
	if extra.EnableLoudnessNorm != nil {
		value := *extra.EnableLoudnessNorm
		copied.EnableLoudnessNorm = &value
	}
	if extra.EnableConversationTruncate != nil {
		value := *extra.EnableConversationTruncate
		copied.EnableConversationTruncate = &value
	}
	if extra.EnableUserQueryExit != nil {
		value := *extra.EnableUserQueryExit
		copied.EnableUserQueryExit = &value
	}
	return &copied
}

func (t *DoubaoRealtime) realtimeInputMode() doubaospeech.RealtimeInputMode {
	switch t.mode {
	case DoubaoRealtimeModePushToTalk:
		return doubaospeech.RealtimeInputModePushToTalk
	case DoubaoRealtimeModeText:
		return doubaospeech.RealtimeInputModeText
	default:
		return doubaospeech.RealtimeInputModeDefault
	}
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
		session, err := t.realtime.OpenSession(ctx, config)
		if err != nil {
			output.CloseWithError(fmt.Errorf("doubao realtime connect: %w", err))
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

func (t *DoubaoRealtime) processLoop(ctx context.Context, input genx.Stream, output *bufferStream, session doubaoRealtimeSession) (*genx.MessageChunk, error) {
	defer session.Close()
	var restarting atomic.Bool
	assistant := newRealtimeAssistantLifecycle()
	pushToTalk := &doubaoPushToTalkState{}

	markAssistantPending := func(streamID string, epoch uint64) {
		assistant.markPending(streamID, epoch)
	}
	markAssistantStarted := func(streamID string) uint64 {
		return assistant.markStarted(streamID)
	}
	markAssistantDone := func(epoch uint64) {
		assistant.markDone(epoch)
	}
	interruptAssistant := func(streamID string, force bool) (bool, error) {
		interruptedStreamID, interrupted := assistant.interrupt(streamID, force)
		if !interrupted {
			return false, nil
		}
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
		if err := session.Interrupt(ctx); err != nil {
			return true, fmt.Errorf("doubao realtime interrupt response: %w", err)
		}
		return true, nil
	}
	pushAssistantOutput := func(epoch uint64, chunk *genx.MessageChunk) error {
		if !assistant.canPush(epoch) {
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
		return assistant.canPush(epoch)
	}

	streamIDs := newDoubaoRealtimeStreamIDs(t.mode)

	// Start goroutine to receive events
	eventsDone := make(chan struct{})
	go func() {
		lastTranscriptText := ""
		transcriptOpen := false
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
					output.CloseWithError(err)
				}
			}
			close(eventsDone)
		}()
		for event, err := range session.Recv() {
			if err != nil {
				if restarting.Load() {
					slog.Info("doubao: realtime session stopped for restart", "error", err)
					return
				}
				if isDoubaoRealtimeIdleTimeout(err) {
					slog.Info("doubao: realtime session idle timeout", "error", err)
					return
				}
				if !assistant.acceptsOutput() && isDoubaoRealtimeInterruptedDone(err) {
					slog.Info("doubao: realtime session interrupted", "error", err)
					return
				}
				slog.Error("doubao: recv error", "error", err)
				output.CloseWithError(err)
				return
			}

			slog.Debug("doubao: received event", "type", event.Type, "text", event.Text, "audioLen", len(event.Audio))

			// Get StreamID for this response
			streamID := streamIDs.response()

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
					if delta == "" {
						continue
					}
					if t.mode == DoubaoRealtimeModeRealtime && !transcriptOpen && !realtimeTextHasSemantic(delta) {
						lastTranscriptText = ""
						continue
					}
					lastTranscriptText = text
					outChunk := &genx.MessageChunk{
						Role: genx.RoleUser,
						Part: genx.Text(delta),
						Ctrl: &genx.StreamCtrl{StreamID: streamIDs.input(), Label: doubaoRealtimeTranscriptLabel},
					}
					if err := output.Push(outChunk); err != nil {
						return
					}
					transcriptOpen = true
					if t.mode == DoubaoRealtimeModeRealtime && realtimeASRResponseEndsSegment(event, delta) {
						if err := closeInputSegment(); err != nil {
							return
						}
					}
				}

			case doubaospeech.EventASREnded:
				// User speech ended - pop StreamID for upcoming response
				slog.Info("doubao: ASR ended")
				assistant.setAccept(true)
				epoch := assistant.nextEpoch()
				responseStreamID := ""
				switch {
				case t.mode == DoubaoRealtimeModePushToTalk || transcriptOpen:
					if err := closeInputSegment(); err != nil {
						return
					}
					responseStreamID = streamIDs.response()
				case streamIDs.response() == "":
					responseStreamID = streamIDs.endInputSegment()
				default:
					responseStreamID = streamIDs.response()
				}
				markAssistantPending(responseStreamID, epoch)

			case doubaospeech.EventTTSStarted:
				if !assistant.acceptsOutput() {
					continue
				}
				// TTS started - send BOS to signal start of audio stream
				slog.Info("doubao: TTS started, sending BOS", "streamID", streamID)
				epoch := markAssistantStarted(streamID)
				if t.mode == DoubaoRealtimeModePushToTalk {
					pushToTalk.responseStarted(true)
				}
				bosChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: t.outputMIMEType()},
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, BeginOfStream: true},
				}
				if err := pushAssistantOutput(epoch, bosChunk); err != nil {
					return
				}

				// Also send text if available
				if event.Text != "" {
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(event.Text),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
					}
					if err := pushAssistantOutput(epoch, outChunk); err != nil {
						return
					}
				}

			case doubaospeech.EventChatResponse:
				if !assistant.acceptsOutput() {
					continue
				}
				if t.mode == DoubaoRealtimeModePushToTalk {
					pushToTalk.responseStarted(false)
				}
				// Model text response
				text := strings.TrimSpace(event.Text)
				if text != "" {
					slog.Debug("doubao: chat response", "text", text)
					epoch := assistant.currentEpoch()
					outChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text(text),
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
					}
					if err := pushAssistantOutput(epoch, outChunk); err != nil {
						return
					}
				}

			case doubaospeech.EventTTSAudioData:
				if !assistant.acceptsOutput() {
					continue
				}
				if t.mode == DoubaoRealtimeModePushToTalk {
					pushToTalk.responseStarted(true)
				}
				// Audio chunk received
				if len(event.Audio) > 0 {
					slog.Debug("doubao: audio received", "len", len(event.Audio))
					epoch := assistant.currentEpoch()
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
						if err := pushAssistantOutput(epoch, outChunk); err != nil {
							return
						}
						if !waitOutputFrame(epoch) {
							break
						}
					}
				}

			case doubaospeech.EventTTSFinished:
				if !assistant.acceptsOutput() {
					continue
				}
				// TTS finished - send EOS to signal end of audio stream
				slog.Info("doubao: TTS finished, sending EOS", "streamID", streamID)
				epoch := assistant.currentEpoch()
				eosChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: t.outputMIMEType()},
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
				}
				if err := pushAssistantOutput(epoch, eosChunk); err != nil {
					return
				}
				markAssistantDone(epoch)
				if t.mode == DoubaoRealtimeModePushToTalk {
					pushToTalk.ttsFinished()
				}
				// Keep the realtime session open for normal multi-turn dialogue.
				// Interrupt-driven BOS restarts the session explicitly so the
				// utterance that interrupted the assistant is not lost.

			case doubaospeech.EventChatEnded:
				if !assistant.acceptsOutput() {
					continue
				}
				if t.mode == DoubaoRealtimeModePushToTalk {
					pushToTalk.chatEnded()
				}
				// Model response ended (text complete, audio may follow)
				slog.Debug("doubao: chat ended")
				epoch := assistant.currentEpoch()
				doneChunk := &genx.MessageChunk{
					Role: genx.RoleModel,
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{
						StreamID:    streamID,
						Label:       doubaoRealtimeAssistantLabel,
						EndOfStream: true,
					},
				}
				if err := pushAssistantOutput(epoch, doneChunk); err != nil {
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
			return nil, nil
		}

		if chunk == nil {
			continue
		}

		// Track StreamID from BOS marker only
		if chunk.IsBeginOfStream() && chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
			bargeIn := false
			interruptStreamID := chunk.Ctrl.StreamID
			if t.mode == DoubaoRealtimeModePushToTalk {
				var err error
				var previousStreamID string
				bargeIn, previousStreamID, err = pushToTalk.begin(chunk.Ctrl.StreamID)
				if err != nil {
					return nil, err
				}
				if previousStreamID != "" {
					interruptStreamID = previousStreamID
				}
			}
			interrupted, err := interruptAssistant(interruptStreamID, bargeIn)
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

		// Handle EOS according to the configured input mode.
		if chunk.IsEndOfStream() {
			streamID := streamIDs.serviceInput(chunk)
			if t.mode == DoubaoRealtimeModePushToTalk {
				if err := pushToTalk.end(); err != nil {
					return nil, err
				}
				historyStreamID := streamIDs.historyInput(chunk)
				slog.Info("doubao: received EOS, ending ASR", "streamID", streamID, "historyStreamID", historyStreamID, "audioSent", audioSent)
				mimeType := ""
				if blob, ok := chunk.Part.(*genx.Blob); ok {
					mimeType = blob.MIMEType
				}
				if err := output.Push(historyUserAudioEOSChunk(historyStreamID, mimeType)); err != nil {
					return nil, err
				}
				if err := session.EndASR(ctx); err != nil {
					slog.Error("doubao: end ASR error", "error", err)
					return nil, err
				}
			} else if t.mode != DoubaoRealtimeModeText {
				slog.Info("doubao: received EOS, sending silence for VAD", "streamID", streamID, "audioSent", audioSent)
				t.sendVADSilence(ctx, session, audioInputs.stream(streamID))
			}
			audioInputs.closeStream(streamID)
			// Don't return - wait for Doubao to process and respond
			continue
		}

		// Send based on part type
		switch p := chunk.Part.(type) {
		case *genx.Blob:
			if t.mode == DoubaoRealtimeModeText {
				return nil, fmt.Errorf("doubao realtime text mode does not accept audio input")
			}
			if t.mode == DoubaoRealtimeModePushToTalk {
				if err := pushToTalk.requireCapturing("audio"); err != nil {
					return nil, err
				}
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
					if err := session.SendAudio(ctx, audio); err != nil {
						slog.Error("doubao: send audio error", "error", err)
						return nil, err
					}
				}
			}
		case genx.Text:
			// Send text query
			if len(p) > 0 {
				slog.Info("doubao: sending text", "text", string(p))
				if err := session.SendText(ctx, string(p)); err != nil {
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

func isDoubaoRealtimeIdleTimeout(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *doubaospeech.Error
	if errors.As(err, &apiErr) {
		message := strings.ToLower(apiErr.Message)
		if apiErr.Code == 55000001 && strings.Contains(message, "dialogaudioidletimeouterror") {
			return true
		}
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "dialogaudioidletimeouterror")
}

func isDoubaoRealtimeInterruptedDone(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "the stream is done") || strings.Contains(text, "code = 13")
}

func (t *DoubaoRealtime) sendVADSilence(ctx context.Context, session doubaoRealtimeSession, audioInput *doubaoRealtimeAudioInput) {
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
		if err := session.SendAudio(ctx, frame); err != nil {
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
