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
	"maps"
	"strings"
	"sync"
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
	retryInitial      time.Duration
	retryMax          time.Duration
	retryWait         func(context.Context, <-chan struct{}, time.Duration) bool
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

	doubaoRealtimePTTOutputLimit    = 2 * time.Minute
	doubaoRealtimePTTOutputMaxBytes = 32 << 20
	doubaoRealtimeRetryInitial      = 100 * time.Millisecond
	doubaoRealtimeRetryMax          = 5 * time.Second
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
		retryInitial:    doubaoRealtimeRetryInitial,
		retryMax:        doubaoRealtimeRetryMax,
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
func (t *DoubaoRealtime) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
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
			maps.Copy(copied.Context.CorrectWords, extra.Context.CorrectWords)
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

type doubaoRealtimeRecoverableError struct {
	op    string
	cause error
}

func (e *doubaoRealtimeRecoverableError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.op) == "" {
		return e.cause.Error()
	}
	return fmt.Sprintf("doubao realtime %s: %v", e.op, e.cause)
}

func (e *doubaoRealtimeRecoverableError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func doubaoRealtimeRecoverable(op string, err error) error {
	if err == nil {
		err = io.EOF
	}
	return &doubaoRealtimeRecoverableError{op: op, cause: err}
}

func isDoubaoRealtimeRecoverable(err error) bool {
	var recoverable *doubaoRealtimeRecoverableError
	return errors.As(err, &recoverable)
}

type doubaoRealtimeRuntime struct {
	assistant   *realtimeAssistantLifecycle
	pushToTalk  *doubaoPushToTalkState
	pttTurn     *doubaoRealtimePTTTurn
	streamIDs   *doubaoRealtimeStreamIDs
	audioInputs *doubaoRealtimeAudioInputs
}

func newDoubaoRealtimeRuntime(t *DoubaoRealtime) *doubaoRealtimeRuntime {
	return &doubaoRealtimeRuntime{
		assistant:   newRealtimeAssistantLifecycle(),
		pushToTalk:  &doubaoPushToTalkState{},
		pttTurn:     &doubaoRealtimePTTTurn{},
		streamIDs:   newDoubaoRealtimeStreamIDs(t.mode),
		audioInputs: newDoubaoRealtimeAudioInputs(t.inputFormat, t.inputSampleRate, t.inputChannels, t.inputTranscode),
	}
}

func (r *doubaoRealtimeRuntime) close() {
	if r != nil && r.audioInputs != nil {
		r.audioInputs.close()
	}
}

func (r *doubaoRealtimeRuntime) providerLost(t *DoubaoRealtime, output realtimeChunkOutput, cause error) {
	if r == nil {
		return
	}
	errText := "doubao realtime provider session lost"
	if cause != nil {
		errText = cause.Error()
	}
	pttStreamID, pttActive, pttCommitted := r.pttTurn.discard()
	stateStreamID, _, stateActive := r.pushToTalk.abort()
	if pttStreamID == "" {
		pttStreamID = stateStreamID
	}
	if (pttActive && !pttCommitted) || (!pttActive && stateActive) {
		_ = output.Push(&genx.MessageChunk{
			Role: genx.RoleUser,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{
				StreamID:    pttStreamID,
				Label:       doubaoRealtimeTranscriptLabel,
				EndOfStream: true,
				Error:       errText,
			},
		})
	}

	interruption := r.assistant.interruptRoutes(pttStreamID, false)
	if interruption.interrupted && interruption.textOpen {
		_ = output.Push(&genx.MessageChunk{
			Role: genx.RoleModel,
			Part: genx.Text(""),
			Ctrl: &genx.StreamCtrl{StreamID: interruption.streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true, Error: errText},
		})
	}
	if interruption.interrupted && interruption.audioOpen {
		_ = output.Push(&genx.MessageChunk{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: t.outputMIMEType()},
			Ctrl: &genx.StreamCtrl{StreamID: interruption.streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true, Error: errText},
		})
	}
}

type doubaoRealtimeInputResult struct {
	chunk *genx.MessageChunk
	err   error
}

type doubaoRealtimeInputReader struct {
	source      genx.Stream
	results     chan doubaoRealtimeInputResult
	done        chan struct{}
	terminal    chan struct{}
	terminalErr error
	pending     *doubaoRealtimeInputResult
	closeOnce   sync.Once
}

func newDoubaoRealtimeInputReader(source genx.Stream) *doubaoRealtimeInputReader {
	reader := &doubaoRealtimeInputReader{
		source:   source,
		results:  make(chan doubaoRealtimeInputResult, 1),
		done:     make(chan struct{}),
		terminal: make(chan struct{}),
	}
	go reader.read()
	return reader
}

func (r *doubaoRealtimeInputReader) read() {
	defer close(r.results)
	for {
		chunk, err := r.source.Next()
		result := doubaoRealtimeInputResult{chunk: chunk, err: err}
		if err != nil {
			r.terminalErr = err
			close(r.terminal)
		}
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

func (r *doubaoRealtimeInputReader) terminalError() (error, bool) {
	if r == nil || r.pending != nil {
		return nil, false
	}
	select {
	case <-r.terminal:
		select {
		case result, ok := <-r.results:
			if !ok {
				return r.terminalErr, true
			}
			if result.err == nil {
				r.pending = &result
				return nil, false
			}
			return result.err, true
		default:
			return r.terminalErr, true
		}
	default:
		return nil, false
	}
}

func (r *doubaoRealtimeInputReader) terminalDone() <-chan struct{} {
	if r == nil || r.pending != nil {
		return nil
	}
	return r.terminal
}

func (r *doubaoRealtimeInputReader) Next() (*genx.MessageChunk, error) {
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

func (r *doubaoRealtimeInputReader) NextOrDone(done <-chan struct{}) (*genx.MessageChunk, error, bool) {
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

func (r *doubaoRealtimeInputReader) Close() error {
	return r.CloseWithError(io.EOF)
}

func (r *doubaoRealtimeInputReader) CloseWithError(err error) error {
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

type doubaoRealtimeDoneAwareStream interface {
	genx.Stream
	NextOrDone(<-chan struct{}) (*genx.MessageChunk, error, bool)
}

func doubaoRealtimeNextOrDone(input genx.Stream, done <-chan struct{}) (*genx.MessageChunk, error, bool) {
	if stream, ok := input.(doubaoRealtimeDoneAwareStream); ok {
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

func (t *DoubaoRealtime) sessionLoop(ctx context.Context, input genx.Stream, output *bufferStream) {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-workerCtx.Done():
		case <-output.Done():
			cancel()
		}
	}()

	reader := newDoubaoRealtimeInputReader(input)
	defer reader.Close()
	runtime := newDoubaoRealtimeRuntime(t)
	defer runtime.close()
	output.setOutputObserver(func(chunk *genx.MessageChunk) {
		observeRealtimeAssistantOutput(runtime.assistant, doubaoRealtimeAssistantLabel, chunk)
	})
	defer output.setOutputObserver(nil)
	defer output.Close()

	stopForTerminalInput := func() bool {
		err, ended := reader.terminalError()
		if !ended {
			return false
		}
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, genx.ErrDone) {
			_ = reader.CloseWithError(err)
			_ = output.CloseWithError(err)
		}
		return true
	}
	retryInitial := t.retryInitial
	if retryInitial <= 0 {
		retryInitial = doubaoRealtimeRetryInitial
	}
	retryDelay := retryInitial
	retryMax := max(t.retryMax, retryDelay)
	for {
		if err := workerCtx.Err(); err != nil {
			if ctx.Err() != nil {
				_ = reader.CloseWithError(ctx.Err())
				_ = output.CloseWithError(ctx.Err())
			}
			return
		}
		if stopForTerminalInput() {
			return
		}
		config := t.realtimeConfig()
		session, err := t.realtime.OpenSession(workerCtx, config)
		if err != nil {
			if stopForTerminalInput() {
				return
			}
			slog.Warn("doubao: realtime provider connection failed; retrying", "error", err, "retryDelay", retryDelay)
			if !t.waitRetry(workerCtx, output.Done(), reader.terminalDone(), retryDelay) {
				continue
			}
			retryDelay = min(retryDelay*2, retryMax)
			continue
		}
		retryDelay = retryInitial
		err = t.processSession(workerCtx, reader, output, session, runtime)
		if err == nil {
			return
		}
		if isDoubaoRealtimeRecoverable(err) {
			runtime.providerLost(t, output, err)
			if stopForTerminalInput() {
				return
			}
			slog.Warn("doubao: realtime provider session lost; reconnecting", "error", err, "retryDelay", retryDelay)
			if !t.waitRetry(workerCtx, output.Done(), reader.terminalDone(), retryDelay) {
				continue
			}
			retryDelay = min(retryDelay*2, retryMax)
			continue
		}
		if workerCtx.Err() != nil {
			if ctx.Err() != nil {
				_ = reader.CloseWithError(ctx.Err())
				_ = output.CloseWithError(ctx.Err())
			}
			return
		}
		_ = reader.CloseWithError(err)
		_ = output.CloseWithError(err)
		return
	}
}

func (t *DoubaoRealtime) waitRetry(ctx context.Context, outputDone, inputDone <-chan struct{}, delay time.Duration) bool {
	if t != nil && t.retryWait != nil {
		return t.retryWait(ctx, outputDone, delay)
	}
	return waitDoubaoRealtimeRetry(ctx, outputDone, inputDone, delay)
}

func (t *DoubaoRealtime) processLoop(ctx context.Context, input genx.Stream, output *bufferStream, session doubaoRealtimeSession) (*genx.MessageChunk, error) {
	reader := newDoubaoRealtimeInputReader(input)
	defer reader.Close()
	runtime := newDoubaoRealtimeRuntime(t)
	defer runtime.close()
	err := t.processSession(ctx, reader, output, session, runtime)
	if isDoubaoRealtimeRecoverable(err) {
		return nil, nil
	}
	return nil, err
}

func waitDoubaoRealtimeRetry(ctx context.Context, outputDone, inputDone <-chan struct{}, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-outputDone:
		return false
	case <-inputDone:
		return false
	case <-timer.C:
		return true
	}
}

func (t *DoubaoRealtime) processSession(
	ctx context.Context,
	input genx.Stream,
	output *bufferStream,
	session doubaoRealtimeSession,
	runtime *doubaoRealtimeRuntime,
) error {
	assistant := runtime.assistant
	pushToTalk := runtime.pushToTalk
	pttTurn := runtime.pttTurn
	pttASR := &doubaoRealtimePTTASRQueue{}
	pttResponses := &doubaoRealtimePTTResponses{}
	var pttControl sync.Mutex
	textResponses := &doubaoRealtimeTextResponses{}
	streamIDs := runtime.streamIDs
	audioInputs := runtime.audioInputs

	markAssistantPending := func(streamID string, epoch uint64) {
		assistant.markPending(streamID, epoch)
	}
	markAssistantStarted := func(streamID string) uint64 {
		return assistant.markStarted(streamID)
	}
	interruptAssistantState := func(streamID string, force bool) bool {
		interruption := assistant.interruptRoutes(streamID, force)
		if !interruption.interrupted {
			return false
		}
		output.discard(func(chunk *genx.MessageChunk) bool {
			return isDoubaoRealtimeAssistantChunk(chunk, interruption.streamID)
		})
		if interruption.textOpen {
			textEOS := &genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{StreamID: interruption.streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true, Error: doubaoRealtimeInterrupted},
			}
			_ = output.Push(textEOS)
		}
		if interruption.audioOpen {
			audioEOS := &genx.MessageChunk{
				Role: genx.RoleModel,
				Part: &genx.Blob{MIMEType: t.outputMIMEType()},
				Ctrl: &genx.StreamCtrl{StreamID: interruption.streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true, Error: doubaoRealtimeInterrupted},
			}
			_ = output.Push(audioEOS)
		}
		return true
	}
	interruptAssistant := func(streamID string, force bool) (bool, error) {
		interrupted := interruptAssistantState(streamID, force)
		if !interrupted {
			return false, nil
		}
		if err := session.Interrupt(ctx); err != nil {
			return true, doubaoRealtimeRecoverable("interrupt response", err)
		}
		return true, nil
	}
	pushAssistantOutput := func(epoch uint64, response *doubaoRealtimePTTResponse, chunk *genx.MessageChunk) error {
		if !assistant.canPush(epoch) {
			return nil
		}
		if t.mode != DoubaoRealtimeModePushToTalk {
			return output.Push(chunk)
		}
		if response == nil {
			return nil
		}
		err := response.push(chunk)
		if !errors.Is(err, errRealtimePTTOutputLimit) {
			return err
		}
		if _, active, _ := pttTurn.discardResponse(response); active {
			_, _, _ = pushToTalk.abort()
		}
		assistant.setAccept(false)
		assistant.nextEpoch()
		if interruptErr := session.Interrupt(ctx); interruptErr != nil {
			return doubaoRealtimeRecoverable("interrupt response after push-to-talk output limit", interruptErr)
		}
		return nil
	}
	eventsDone := make(chan struct{})
	eventsResult := make(chan error, 1)
	go func() {
		defer close(eventsDone)
		lastTranscriptText := ""
		transcriptOpen := false
		closeInputSegment := func(errText string) error {
			inputStreamID := streamIDs.endInputSegment()
			doneChunk := &genx.MessageChunk{
				Role: genx.RoleUser,
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{
					StreamID:    inputStreamID,
					Label:       doubaoRealtimeTranscriptLabel,
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
		receive := func() (retErr error) {
			defer func() {
				if t.mode == DoubaoRealtimeModeRealtime && transcriptOpen {
					errText := ""
					if isDoubaoRealtimeRecoverable(retErr) {
						errText = retErr.Error()
					}
					if err := closeInputSegment(errText); retErr == nil && err != nil {
						retErr = err
					}
				}
			}()
			for event, err := range session.Recv() {
				if err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					return doubaoRealtimeRecoverable("receive events", err)
				}
				if event == nil {
					continue
				}

				slog.Debug("doubao: received event", "type", event.Type, "text", event.Text, "audioLen", len(event.Audio))

				streamID := streamIDs.response()
				if t.mode == DoubaoRealtimeModePushToTalk {
					streamID = firstNonEmptyString(pttTurn.stream(), streamID)
				}

				switch event.Type {
				case doubaospeech.EventASRInfo:
					slog.Info("doubao: ASR info - speech detected")
					if t.mode == DoubaoRealtimeModeRealtime {
						if _, err := interruptAssistant(streamID, false); err != nil {
							return err
						}
					}

				case doubaospeech.EventASRResponse:
					text := strings.TrimSpace(event.Text)
					if text == "" {
						text = realtimeASRText(event.Payload)
					}
					slog.Info("doubao: ASR response", "text", text)
					if t.mode == DoubaoRealtimeModePushToTalk {
						pttControl.Lock()
						generation := pttASR.peek(pttTurn.currentGeneration())
						pttTurn.updateHypothesisFor(generation, text)
						pttControl.Unlock()
						continue
					}
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
							return err
						}
						transcriptOpen = true
						if t.mode == DoubaoRealtimeModeRealtime && realtimeASRResponseEndsSegment(event, delta) {
							if err := closeInputSegment(""); err != nil {
								return err
							}
						}
					}

				case doubaospeech.EventASREnded:
					slog.Info("doubao: ASR ended")
					if t.mode == DoubaoRealtimeModePushToTalk {
						pttControl.Lock()
						pttGeneration, ok := pttASR.take()
						if !ok {
							pttControl.Unlock()
							continue
						}
						text := strings.TrimSpace(event.Text)
						if text == "" {
							text = realtimeASRText(event.Payload)
						}
						pttTurn.updateHypothesisFor(pttGeneration, text)
						matched, err := pttTurn.markASREndedFor(pttGeneration)
						if err != nil {
							pttControl.Unlock()
							return err
						}
						if !matched {
							pttControl.Unlock()
							continue
						}
						assistant.setAccept(true)
						epoch := assistant.nextEpoch()
						response := pttTurn.bindResponseFor(pttGeneration, epoch, doubaoRealtimeEventResponseIdentity(event))
						if response == nil {
							pttControl.Unlock()
							continue
						}
						pttResponses.add(response)
						markAssistantPending(response.streamID, epoch)
						pttControl.Unlock()
						continue
					}
					assistant.setAccept(true)
					epoch := assistant.nextEpoch()
					responseStreamID := ""
					switch {
					case transcriptOpen:
						if err := closeInputSegment(""); err != nil {
							return err
						}
						responseStreamID = streamIDs.response()
					case streamIDs.response() == "":
						responseStreamID = streamIDs.endInputSegment()
					default:
						responseStreamID = streamIDs.response()
					}
					markAssistantPending(responseStreamID, epoch)

				case doubaospeech.EventTTSStarted:
					var response *doubaoRealtimePTTResponse
					var epoch uint64
					if t.mode == DoubaoRealtimeModePushToTalk {
						response = pttResponses.match(doubaoRealtimeEventResponseIdentity(event))
						if response == nil {
							continue
						}
						streamID = response.streamID
						epoch = response.epoch
						response.ttsStarted = true
						pushToTalk.responseStarted(streamID, true)
					} else {
						if !assistant.acceptsOutput() {
							continue
						}
						if t.mode == DoubaoRealtimeModeText {
							textResponses.markTTSStarted()
						}
						epoch = markAssistantStarted(streamID)
					}
					slog.Info("doubao: TTS started, sending BOS", "streamID", streamID)
					bosChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: t.outputMIMEType()},
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, BeginOfStream: true},
					}
					if err := pushAssistantOutput(epoch, response, bosChunk); err != nil {
						return err
					}

					if event.Text != "" {
						outChunk := &genx.MessageChunk{
							Role: genx.RoleModel,
							Part: genx.Text(event.Text),
							Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
						}
						if err := pushAssistantOutput(epoch, response, outChunk); err != nil {
							return err
						}
					}

				case doubaospeech.EventChatResponse:
					var response *doubaoRealtimePTTResponse
					epoch := assistant.currentEpoch()
					if t.mode == DoubaoRealtimeModePushToTalk {
						response = pttResponses.match(doubaoRealtimeEventResponseIdentity(event))
						if response == nil {
							continue
						}
						streamID = response.streamID
						epoch = response.epoch
						pushToTalk.responseStarted(streamID, false)
					} else if !assistant.acceptsOutput() {
						continue
					}
					text := strings.TrimSpace(event.Text)
					if text != "" {
						slog.Debug("doubao: chat response", "text", text)
						outChunk := &genx.MessageChunk{
							Role: genx.RoleModel,
							Part: genx.Text(text),
							Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
						}
						if err := pushAssistantOutput(epoch, response, outChunk); err != nil {
							return err
						}
					}

				case doubaospeech.EventTTSAudioData:
					var response *doubaoRealtimePTTResponse
					epoch := assistant.currentEpoch()
					if t.mode == DoubaoRealtimeModePushToTalk {
						response = pttResponses.match(doubaoRealtimeEventResponseIdentity(event))
						if response == nil {
							continue
						}
						streamID = response.streamID
						epoch = response.epoch
						response.ttsStarted = true
						pushToTalk.responseStarted(streamID, true)
					} else if !assistant.acceptsOutput() {
						continue
					} else if t.mode == DoubaoRealtimeModeText {
						textResponses.markTTSStarted()
					}
					if len(event.Audio) > 0 {
						slog.Debug("doubao: audio received", "len", len(event.Audio))
						blobs, err := t.outputAudioBlobs(event.Audio)
						if err != nil {
							return err
						}
						for _, blob := range blobs {
							outChunk := &genx.MessageChunk{
								Role: genx.RoleModel,
								Part: blob,
								Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel},
							}
							if err := pushAssistantOutput(epoch, response, outChunk); err != nil {
								return err
							}
						}
					}

				case doubaospeech.EventTTSFinished:
					var response *doubaoRealtimePTTResponse
					epoch := assistant.currentEpoch()
					if t.mode == DoubaoRealtimeModePushToTalk {
						response = pttResponses.match(doubaoRealtimeEventResponseIdentity(event))
						if response == nil {
							continue
						}
						streamID = response.streamID
						epoch = response.epoch
					} else if !assistant.acceptsOutput() {
						if t.mode == DoubaoRealtimeModeText {
							textResponses.markTTSFinished()
						}
						continue
					}
					slog.Info("doubao: TTS finished, sending EOS", "streamID", streamID)
					eosChunk := &genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: t.outputMIMEType()},
						Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeAssistantLabel, EndOfStream: true},
					}
					if err := pushAssistantOutput(epoch, response, eosChunk); err != nil {
						return err
					}
					if t.mode == DoubaoRealtimeModePushToTalk {
						pushToTalk.ttsFinished(streamID)
						response.ttsFinished = true
						pttResponses.finish(response)
					} else if t.mode == DoubaoRealtimeModeText {
						textResponses.markTTSFinished()
					}

				case doubaospeech.EventChatEnded:
					var response *doubaoRealtimePTTResponse
					epoch := assistant.currentEpoch()
					if t.mode == DoubaoRealtimeModePushToTalk {
						response = pttResponses.match(doubaoRealtimeEventResponseIdentity(event))
						if response == nil {
							continue
						}
						streamID = response.streamID
						epoch = response.epoch
						pushToTalk.chatEnded(streamID)
					} else if !assistant.acceptsOutput() {
						if t.mode == DoubaoRealtimeModeText {
							textResponses.markChatEnded()
						}
						continue
					}
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
					if err := pushAssistantOutput(epoch, response, doneChunk); err != nil {
						return err
					}
					if response != nil {
						response.chatEnded = true
						pttResponses.finish(response)
					} else if t.mode == DoubaoRealtimeModeText {
						textResponses.markChatEnded()
					}

				case doubaospeech.EventSessionFinished:
					slog.Info("doubao: session ended")
					return doubaoRealtimeRecoverable("session finished", io.EOF)
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return doubaoRealtimeRecoverable("receive events", io.EOF)
		}
		eventsResult <- receive()
	}()
	defer func() {
		_ = session.Close()
		<-eventsDone
	}()

	slog.Info("doubao: starting audio send loop")

	audioSent := 0
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-output.Done():
		case <-eventsDone:
		}
		close(stop)
	}()
	for {
		chunk, err, stopped := doubaoRealtimeNextOrDone(input, stop)
		if stopped {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			select {
			case <-output.Done():
				return nil
			default:
			}
			return <-eventsResult
		}
		if err != nil {
			if err != io.EOF && err != genx.ErrDone {
				slog.Error("doubao: input error", "error", err)
				return err
			}
			slog.Info("doubao: input EOF", "audioSent", audioSent)
			var responseDone <-chan struct{}
			var wait bool
			switch t.mode {
			case DoubaoRealtimeModePushToTalk:
				responseDone, wait = pttTurn.responseDone()
			case DoubaoRealtimeModeText:
				responseDone, wait = textResponses.responseDone()
			}
			if wait {
				select {
				case <-responseDone:
					return nil
				default:
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-output.Done():
					return nil
				case <-responseDone:
					return nil
				case eventErr := <-eventsResult:
					return eventErr
				}
			}
			return nil
		}

		if chunk == nil {
			continue
		}
		if t.mode == DoubaoRealtimeModePushToTalk && pushToTalk.discard(chunk) {
			if realtimeAudioInputEOS(chunk) {
				streamID := streamIDs.serviceInput(chunk)
				historyStreamID := streamIDs.historyInput(chunk)
				mimeType := ""
				if blob, ok := chunk.Part.(*genx.Blob); ok {
					mimeType = blob.MIMEType
				}
				if err := output.Push(historyUserAudioEOSChunk(historyStreamID, mimeType)); err != nil {
					return err
				}
				audioInputs.closeStream(streamID)
			}
			continue
		}

		if chunk.IsBeginOfStream() && chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
			if t.mode == DoubaoRealtimeModePushToTalk {
				pttControl.Lock()
				bargeIn, previousStreamID, err := pushToTalk.begin(chunk.Ctrl.StreamID)
				if err != nil {
					pttControl.Unlock()
					return err
				}
				interruptStreamID := chunk.Ctrl.StreamID
				if previousStreamID != "" {
					interruptStreamID = previousStreamID
				}
				interrupted := interruptAssistantState(interruptStreamID, bargeIn)
				if interrupted {
					_, _, _ = pttTurn.discard()
				}
				streamIDs.beginInput(chunk.Ctrl.StreamID)
				pttTurn.begin(
					output,
					streamIDs.input(),
					doubaoRealtimeAssistantLabel,
					doubaoRealtimePTTOutputLimit,
					realtimePTTOutputByteLimit(doubaoRealtimePTTOutputLimit, t.sampleRate, t.channels),
				)
				pttControl.Unlock()
				if interrupted {
					if err := session.Interrupt(ctx); err != nil {
						return doubaoRealtimeRecoverable("interrupt response", err)
					}
				}
				slog.Info("doubao: received BOS", "streamID", chunk.Ctrl.StreamID)
				continue
			}
			if _, err := interruptAssistant(chunk.Ctrl.StreamID, false); err != nil {
				return err
			}
			streamIDs.beginInput(chunk.Ctrl.StreamID)
			slog.Info("doubao: received BOS", "streamID", chunk.Ctrl.StreamID)
			continue
		}

		if realtimeAudioInputEOS(chunk) {
			streamID := streamIDs.serviceInput(chunk)
			if t.mode == DoubaoRealtimeModePushToTalk {
				if err := pushToTalk.end(); err != nil {
					return err
				}
				historyStreamID := streamIDs.historyInput(chunk)
				slog.Info("doubao: received EOS, ending ASR", "streamID", streamID, "historyStreamID", historyStreamID, "audioSent", audioSent)
				mimeType := ""
				if blob, ok := chunk.Part.(*genx.Blob); ok {
					mimeType = blob.MIMEType
				}
				if err := output.Push(historyUserAudioEOSChunk(historyStreamID, mimeType)); err != nil {
					return err
				}
				pttASR.add(pttTurn.currentGeneration())
				if err := session.EndASR(ctx); err != nil {
					slog.Error("doubao: end ASR error", "error", err)
					return doubaoRealtimeRecoverable("end ASR", err)
				}
				if err := pttTurn.markInputEnded(); err != nil {
					return err
				}
			} else if t.mode != DoubaoRealtimeModeText {
				slog.Info("doubao: received realtime EOS, closing local audio input", "streamID", streamID, "audioSent", audioSent)
			}
			audioInputs.closeStream(streamID)
			continue
		}

		switch p := chunk.Part.(type) {
		case *genx.Blob:
			if t.mode == DoubaoRealtimeModeText {
				return fmt.Errorf("doubao realtime text mode does not accept audio input")
			}
			if t.mode == DoubaoRealtimeModePushToTalk {
				if err := pushToTalk.requireCapturing("audio"); err != nil {
					return err
				}
			}
			if len(p.Data) > 0 {
				streamID := streamIDs.serviceInput(chunk)
				historyStreamID := streamIDs.historyInput(chunk)
				if t.mode != DoubaoRealtimeModeRealtime {
					if err := output.Push(historyUserAudioChunk(chunk, historyStreamID)); err != nil {
						return err
					}
				}
				audioInput, err := audioInputs.streamForBlob(streamID, p)
				if err != nil {
					slog.Error("doubao: prepare audio error", "error", err)
					t.pushInputEOSError(output, streamID, err)
					audioInputs.closeStream(streamID)
					return err
				}
				frames, err := audioInput.prepareFrames(p)
				if err != nil {
					slog.Error("doubao: prepare audio error", "error", err)
					t.pushInputEOSError(output, streamID, err)
					audioInputs.closeStream(streamID)
					return err
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
						return doubaoRealtimeRecoverable("send audio", err)
					}
				}
			}
		case genx.Text:
			if len(p) > 0 {
				slog.Info("doubao: sending text", "text", string(p))
				var response *doubaoRealtimeTextResponse
				if t.mode == DoubaoRealtimeModeText {
					response = textResponses.begin()
					if inputStreamID := streamIDs.serviceInput(chunk); inputStreamID != "" {
						streamIDs.beginInput(inputStreamID)
					}
					responseStreamID := streamIDs.endInputSegment()
					assistant.setAccept(true)
					assistant.markPending(responseStreamID, assistant.currentEpoch())
				}
				if err := session.SendText(ctx, string(p)); err != nil {
					textResponses.cancel(response)
					slog.Error("doubao: send text error", "error", err)
					return doubaoRealtimeRecoverable("send text", err)
				}
			}
		}
	}
}

func isDoubaoRealtimeAssistantChunk(chunk *genx.MessageChunk, streamID string) bool {
	return chunk != nil && chunk.Role == genx.RoleModel && chunk.Ctrl != nil &&
		chunk.Ctrl.StreamID == streamID && chunk.Ctrl.Label == doubaoRealtimeAssistantLabel
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

func doubaoRealtimeEventResponseIdentity(event *doubaospeech.RealtimeEvent) doubaoRealtimePTTResponseIdentity {
	if event == nil {
		return doubaoRealtimePTTResponseIdentity{}
	}
	return doubaoRealtimePTTResponseIdentity{
		replyID:    strings.TrimSpace(event.ReplyID),
		questionID: strings.TrimSpace(event.QuestionID),
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

func realtimePTTOutputByteLimit(limit time.Duration, sampleRate, channels int) int64 {
	if limit <= 0 {
		return 0
	}
	if sampleRate <= 0 {
		sampleRate = doubaoRealtimeFixedOutputSampleRate
	}
	if channels <= 0 {
		channels = doubaoRealtimeFixedOutputChannels
	}
	const bytesPerSample = int64(2)
	const maxInt64 = int64(^uint64(0) >> 1)
	sampleRate64 := int64(sampleRate)
	channels64 := int64(channels)
	if sampleRate64 > maxInt64/channels64/bytesPerSample {
		return doubaoRealtimePTTOutputMaxBytes
	}
	bytesPerSecond := sampleRate64 * channels64 * bytesPerSample
	seconds := int64(limit / time.Second)
	if limit%time.Second != 0 {
		seconds++
	}
	if seconds > maxInt64/bytesPerSecond {
		return doubaoRealtimePTTOutputMaxBytes
	}
	return min(bytesPerSecond*seconds, int64(doubaoRealtimePTTOutputMaxBytes))
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
