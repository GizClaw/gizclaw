package transformers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const (
	doubaoASTTranslateTranscriptLabel = "transcript"
	doubaoASTTranslateAssistantLabel  = "assistant"
	doubaoASTTranslatePTTOutputLimit  = 2 * time.Minute

	doubaoASTTranslateSourceSampleRate = 16000
	doubaoASTTranslateSourceChannels   = 1
	doubaoASTTranslateSourceBits       = 16
)

var errDoubaoASTTranslatePTTOutputLimit = errors.New("doubao ast translate: push-to-talk output audio limit exceeded")

type DoubaoASTTranslate struct {
	client                     *doubaospeech.Client
	resourceID                 string
	mode                       doubaospeech.ASTTranslateMode
	inputMode                  DoubaoASTTranslateInputMode
	sourceLanguage             string
	targetLanguage             string
	speakerID                  string
	isCustomSpeaker            bool
	ttsResourceID              string
	speechRate                 int
	enableSourceLanguageDetect bool
	denoise                    *bool
	realtimePacing             bool

	newSession func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error)
}

var _ genx.Transformer = (*DoubaoASTTranslate)(nil)

type doubaoASTTranslateSession interface {
	SendAudio(context.Context, []byte) error
	Finish(context.Context) error
	Recv() iter.Seq2[*doubaospeech.ASTTranslateEvent, error]
	Close() error
}

type DoubaoASTTranslateOption func(*DoubaoASTTranslate)

type DoubaoASTTranslateInputMode string

const (
	DoubaoASTTranslateInputModeRealtime   DoubaoASTTranslateInputMode = "realtime"
	DoubaoASTTranslateInputModePushToTalk DoubaoASTTranslateInputMode = "push-to-talk"
)

func WithDoubaoASTTranslateResourceID(resourceID string) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.resourceID = resourceID
	}
}

func WithDoubaoASTTranslateMode(mode doubaospeech.ASTTranslateMode) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		if mode != "" {
			t.mode = mode
		}
	}
}

func WithDoubaoASTTranslateInputMode(mode DoubaoASTTranslateInputMode) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		if mode != "" {
			t.inputMode = mode
		}
	}
}

func WithDoubaoASTTranslateSourceLanguage(language string) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.sourceLanguage = language
	}
}

func WithDoubaoASTTranslateTargetLanguage(language string) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.targetLanguage = language
	}
}

func WithDoubaoASTTranslateSpeakerID(speakerID string) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.speakerID = speakerID
	}
}

func WithDoubaoASTTranslateCustomSpeaker(enabled bool) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.isCustomSpeaker = enabled
	}
}

func WithDoubaoASTTranslateTTSResourceID(resourceID string) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.ttsResourceID = resourceID
	}
}

func WithDoubaoASTTranslateSpeechRate(rate int) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.speechRate = rate
	}
}

func WithDoubaoASTTranslateSourceLanguageDetect(enabled bool) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.enableSourceLanguageDetect = enabled
	}
}

func WithDoubaoASTTranslateDenoise(enabled bool) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.denoise = &enabled
	}
}

func WithDoubaoASTTranslateRealtimePacing(enabled bool) DoubaoASTTranslateOption {
	return func(t *DoubaoASTTranslate) {
		t.realtimePacing = enabled
	}
}

func NewDoubaoASTTranslate(client *doubaospeech.Client, opts ...DoubaoASTTranslateOption) *DoubaoASTTranslate {
	t := &DoubaoASTTranslate{
		client:         client,
		resourceID:     doubaospeech.ResourceASTTranslate,
		mode:           doubaospeech.ASTTranslateModeS2T,
		inputMode:      DoubaoASTTranslateInputModeRealtime,
		sourceLanguage: "zhen",
		targetLanguage: "zhen",
		realtimePacing: true,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *DoubaoASTTranslate) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.client == nil {
		return nil, fmt.Errorf("doubao ast translate: client is required")
	}
	if input == nil {
		return nil, fmt.Errorf("doubao ast translate: input stream is required")
	}
	output := newBufferStream(64)
	go t.transformLoop(ctx, input, output)
	return output, nil
}

func (t *DoubaoASTTranslate) transformLoop(parent context.Context, input genx.Stream, output *bufferStream) {
	defer output.Close()
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var session doubaoASTTranslateSession
	var sessionGate *astTranslatePTTOutputGate
	var recvDone chan error
	var recvStart chan struct{}
	var streamID string
	var limitedStreamID string
	var sessionFinishing bool
	var rawOpusDecoder *opus.Decoder
	var sessionStartedAt time.Time
	var sentAudioDuration time.Duration
	var sessionSeq uint64
	var activeSessionSeq atomic.Uint64
	historyAudio := newASTTranslateHistoryAudioBuffer()
	defer func() {
		if rawOpusDecoder != nil {
			_ = rawOpusDecoder.Close()
		}
	}()

	startSession := func(id string) error {
		if session != nil {
			return nil
		}
		cfg := t.sessionConfig()
		openSession := t.openSession
		if t.newSession != nil {
			openSession = t.newSession
		}
		next, err := openSession(ctx, cfg)
		if err != nil {
			return err
		}
		session = next
		streamID = strings.TrimSpace(id)
		if streamID == "" {
			streamID = genx.NewStreamID()
		}
		historyAudio.reset()
		sessionStartedAt = time.Time{}
		sentAudioDuration = 0
		sessionFinishing = false
		limitedStreamID = ""
		sessionSeq++
		activeSessionSeq.Store(sessionSeq)
		seq := sessionSeq
		active := func() bool {
			return activeSessionSeq.Load() == seq
		}
		eventOutput := astTranslateOutput(astTranslateGatedOutput{
			output: output,
			active: active,
		})
		if t.inputMode == DoubaoASTTranslateInputModePushToTalk {
			sessionGate = newASTTranslatePTTOutputGate(output, active, streamID)
			eventOutput = sessionGate
		} else {
			sessionGate = nil
		}
		done := make(chan error, 1)
		start := make(chan struct{})
		pttGate := sessionGate
		recvDone = done
		recvStart = start
		go func(activeStreamID string, start <-chan struct{}, eventOutput astTranslateOutput, pttGate *astTranslatePTTOutputGate) {
			select {
			case <-ctx.Done():
				pttGate.Discard()
				done <- ctx.Err()
				return
			case <-start:
			}
			err := t.forwardEvents(eventOutput, next, activeStreamID, historyAudio)
			if errors.Is(err, errDoubaoASTTranslatePTTOutputLimit) {
				_ = next.Close()
			} else if err != nil {
				pttGate.Discard()
			}
			done <- err
		}(streamID, start, eventOutput, pttGate)
		return nil
	}

	sendAudio := func(audio []byte) error {
		if t.realtimePacing && len(audio) > 0 {
			if sessionStartedAt.IsZero() {
				sessionStartedAt = time.Now()
			}
			if delay := sessionStartedAt.Add(sentAudioDuration).Sub(time.Now()); delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return ctx.Err()
				case <-timer.C:
				}
			}
		}
		if err := session.SendAudio(ctx, audio); err != nil {
			return err
		}
		sentAudioDuration += audioDuration(audio, doubaoASRSessionConfig{
			sampleRate: doubaoASTTranslateSourceSampleRate,
			channels:   doubaoASTTranslateSourceChannels,
			bits:       doubaoASTTranslateSourceBits,
		})
		return nil
	}

	finishSession := func(wait bool) error {
		if session == nil {
			return nil
		}
		active := session
		done := recvDone
		if recvStart != nil {
			close(recvStart)
			recvStart = nil
		}
		if !sessionFinishing {
			sessionFinishing = true
			if err := active.Finish(ctx); err != nil {
				session = nil
				sessionGate = nil
				recvDone = nil
				sessionFinishing = false
				activeSessionSeq.Store(0)
				_ = active.Close()
				return err
			}
		}
		if !wait {
			return nil
		}
		session = nil
		sessionGate = nil
		recvDone = nil
		sessionFinishing = false
		if done == nil {
			activeSessionSeq.Store(0)
			_ = active.Close()
			streamID = ""
			return nil
		}
		err := <-done
		activeSessionSeq.Store(0)
		_ = active.Close()
		streamID = ""
		return err
	}

	completeFinishedSession := func() error {
		if session == nil || !sessionFinishing || recvDone == nil {
			return nil
		}
		select {
		case err := <-recvDone:
			active := session
			if sessionGate != nil && err != nil {
				sessionGate.Discard()
			}
			session = nil
			sessionGate = nil
			recvDone = nil
			sessionFinishing = false
			activeSessionSeq.Store(0)
			_ = active.Close()
			streamID = ""
			return err
		default:
			return nil
		}
	}

	handlePTTOutputLimit := func() error {
		if session == nil || sessionGate == nil || !sessionGate.LimitExceeded() {
			return nil
		}
		active := session
		done := recvDone
		failedID := streamID
		session = nil
		sessionGate = nil
		recvDone = nil
		sessionFinishing = false
		activeSessionSeq.Store(0)
		if recvStart != nil {
			close(recvStart)
			recvStart = nil
		}
		_ = active.Close()
		if done != nil {
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, errDoubaoASTTranslatePTTOutputLimit) {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		limitedStreamID = failedID
		historyAudio.reset()
		sessionStartedAt = time.Time{}
		sentAudioDuration = 0
		return nil
	}

	interruptSession := func(id string) error {
		if session == nil {
			streamID = strings.TrimSpace(id)
			return nil
		}
		active := session
		done := recvDone
		limitExceeded := false
		if sessionGate != nil {
			sessionGate.Discard()
			limitExceeded = sessionGate.LimitExceeded()
		}
		interruptedStreamID := strings.TrimSpace(streamID)
		if interruptedStreamID == "" {
			interruptedStreamID = strings.TrimSpace(id)
		}
		if interruptedStreamID == "" {
			interruptedStreamID = "audio"
		}
		session = nil
		sessionGate = nil
		recvDone = nil
		sessionFinishing = false
		activeSessionSeq.Store(0)
		if recvStart != nil {
			close(recvStart)
			recvStart = nil
		}
		if !limitExceeded {
			for _, chunk := range astTranslateInterruptedChunks(interruptedStreamID) {
				if err := output.Push(chunk); err != nil {
					_ = active.Close()
					return err
				}
			}
		}
		_ = active.Close()
		if done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		streamID = strings.TrimSpace(id)
		limitedStreamID = ""
		historyAudio.reset()
		sessionStartedAt = time.Time{}
		sentAudioDuration = 0
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			if sessionGate != nil {
				sessionGate.Discard()
			}
			output.CloseWithError(err)
			return
		}
		chunk, err := input.Next()
		if limitErr := handlePTTOutputLimit(); limitErr != nil {
			output.CloseWithError(limitErr)
			return
		}
		if err != nil {
			if !errors.Is(err, genx.ErrDone) && !errors.Is(err, io.EOF) {
				if session != nil {
					if sessionGate != nil {
						sessionGate.Discard()
					}
					_ = session.Close()
				}
				output.CloseWithError(err)
				return
			}
			if sessionGate != nil {
				sessionGate.Discard()
			}
			if err := finishSession(true); err != nil {
				output.CloseWithError(err)
			}
			return
		}
		if chunk == nil {
			continue
		}
		if err := completeFinishedSession(); err != nil {
			output.CloseWithError(err)
			return
		}
		id := chunkInputStreamID(chunk, streamID)
		if chunk.IsBeginOfStream() {
			if limitedStreamID != "" {
				limitedStreamID = ""
				streamID = id
				continue
			}
			if strings.TrimSpace(streamID) != "" && strings.TrimSpace(id) != "" && id != streamID {
				if err := interruptSession(id); err != nil {
					output.CloseWithError(err)
					return
				}
			}
			streamID = id
			continue
		}
		if limitedStreamID != "" {
			if chunk.IsEndOfStream() && id == limitedStreamID {
				limitedStreamID = ""
				streamID = ""
			}
			continue
		}
		if strings.TrimSpace(streamID) != "" && strings.TrimSpace(id) != "" && id != streamID {
			continue
		}
		if chunk.IsEndOfStream() {
			if blob, ok := chunk.Part.(*genx.Blob); ok && isAudioMIME(blob.MIMEType) {
				if sessionGate != nil {
					if err := sessionGate.Commit(); err != nil {
						if errors.Is(err, errDoubaoASTTranslatePTTOutputLimit) {
							if limitErr := handlePTTOutputLimit(); limitErr != nil {
								output.CloseWithError(limitErr)
								return
							}
							continue
						}
						if session != nil {
							_ = session.Close()
						}
						output.CloseWithError(err)
						return
					}
				}
				wait := t.inputMode != DoubaoASTTranslateInputModeRealtime
				if err := finishSession(wait); err != nil {
					output.CloseWithError(err)
					return
				}
				continue
			}
			if err := output.Push(chunk); err != nil {
				return
			}
			continue
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || !isAudioMIME(blob.MIMEType) {
			if err := output.Push(chunk); err != nil {
				return
			}
			continue
		}
		audio, err := t.prepareAudioBlob(blob, &rawOpusDecoder)
		if err != nil {
			if sessionGate != nil {
				sessionGate.Discard()
			}
			if session != nil {
				_ = session.Close()
			}
			output.CloseWithError(err)
			return
		}
		if len(audio) == 0 {
			continue
		}
		if err := startSession(id); err != nil {
			output.CloseWithError(err)
			return
		}
		if sessionFinishing {
			output.CloseWithError(fmt.Errorf("doubao ast translate: received audio for finishing session %q", streamID))
			return
		}
		historyAudio.appendChunk(chunk, id)
		if recvStart != nil {
			close(recvStart)
			recvStart = nil
		}
		limitedDuringSend := false
		for audioChunk := range splitDoubaoASRAudio(audio, t.audioChunkSize()) {
			if err := sendAudio(audioChunk); err != nil {
				if sessionGate != nil && sessionGate.LimitExceeded() {
					if limitErr := handlePTTOutputLimit(); limitErr != nil {
						output.CloseWithError(limitErr)
						return
					}
					limitedDuringSend = true
					break
				}
				if sessionGate != nil {
					sessionGate.Discard()
				}
				_ = session.Close()
				output.CloseWithError(err)
				return
			}
		}
		if limitedDuringSend {
			continue
		}
	}
}

func (t *DoubaoASTTranslate) sessionConfig() doubaospeech.ASTTranslateConfig {
	cfg := doubaospeech.DefaultASTTranslateConfig()
	cfg.ResourceID = strings.TrimSpace(t.resourceID)
	cfg.Mode = t.mode
	cfg.SourceLanguage = strings.TrimSpace(t.sourceLanguage)
	if cfg.SourceLanguage == "" {
		cfg.SourceLanguage = "zhen"
	}
	cfg.TargetLanguage = strings.TrimSpace(t.targetLanguage)
	cfg.SourceAudio = doubaospeech.ASTAudioConfig{
		Format:  doubaospeech.FormatWAV,
		Codec:   "raw",
		Rate:    doubaospeech.SampleRate(doubaoASTTranslateSourceSampleRate),
		Bits:    doubaoASTTranslateSourceBits,
		Channel: doubaoASTTranslateSourceChannels,
	}
	cfg.SpeakerID = strings.TrimSpace(t.speakerID)
	cfg.IsCustomSpeaker = t.isCustomSpeaker
	cfg.TTSResourceID = strings.TrimSpace(t.ttsResourceID)
	cfg.SpeechRate = t.speechRate
	cfg.EnableSourceLanguageDetect = t.enableSourceLanguageDetect
	if t.denoise != nil {
		cfg.Denoise = t.denoise
	}
	if cfg.Mode == doubaospeech.ASTTranslateModeS2S {
		cfg.TargetAudio.Format = doubaospeech.FormatOGG
		cfg.TargetAudio.Rate = doubaospeech.SampleRate48000
		cfg.TargetAudio.Channel = 1
	}
	return cfg
}

func (t *DoubaoASTTranslate) openSession(ctx context.Context, cfg doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
	return t.client.ASTTranslate.OpenSession(ctx, &cfg)
}

func (t *DoubaoASTTranslate) prepareAudioBlob(blob *genx.Blob, rawOpusDecoder **opus.Decoder) ([]byte, error) {
	if blob == nil || len(blob.Data) == 0 {
		return nil, nil
	}
	cfg := doubaoASRSessionConfig{
		format:     "pcm",
		sampleRate: doubaoASTTranslateSourceSampleRate,
		channels:   doubaoASTTranslateSourceChannels,
		bits:       doubaoASTTranslateSourceBits,
	}
	mimeType := baseAudioMIME(blob.MIMEType)
	switch {
	case isASRMP3MIME(mimeType):
		return (&DoubaoASRSAUC{}).decodeMP3ToPCM(blob.Data, cfg)
	case isASRPCMMIME(mimeType):
		return blob.Data, nil
	case isOggAudioMIME(mimeType):
		var pcm bytes.Buffer
		if _, err := codecconv.OggToPCM(&pcm, bytes.NewReader(blob.Data), opus.OpusSampleRate(cfg.sampleRate)); err != nil {
			return nil, fmt.Errorf("decode ogg opus for doubao ast translate: %w", err)
		}
		return pcm.Bytes(), nil
	case isASROpusMIME(mimeType):
		return decodeRawOpusToPCM(blob.Data, cfg, rawOpusDecoder)
	default:
		return nil, fmt.Errorf("doubao ast translate input requires audio/opus, audio/ogg, PCM, or MP3 input, got %q", blob.MIMEType)
	}
}

func (t *DoubaoASTTranslate) audioChunkSize() int {
	return doubaoASTTranslateSourceSampleRate * doubaoASTTranslateSourceChannels * (doubaoASTTranslateSourceBits / 8) / 10
}

type astTranslateOutput interface {
	Push(*genx.MessageChunk) error
}

type astTranslatePTTOutputGate struct {
	mu       sync.Mutex
	output   astTranslateOutput
	active   func() bool
	streamID string

	committed        bool
	terminal         bool
	retained         []*genx.MessageChunk
	retainedDuration time.Duration
	limitErr         error
	terminalErr      error
}

func newASTTranslatePTTOutputGate(output astTranslateOutput, active func() bool, streamID string) *astTranslatePTTOutputGate {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	return &astTranslatePTTOutputGate{
		output:   output,
		active:   active,
		streamID: streamID,
	}
}

func (g *astTranslatePTTOutputGate) Push(chunk *genx.MessageChunk) error {
	if g == nil || chunk == nil {
		return nil
	}
	if g.active != nil && !g.active() {
		return nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active != nil && !g.active() {
		return nil
	}
	if g.terminal {
		return nil
	}
	if g.committed {
		return g.output.Push(chunk)
	}

	duration := astTranslateAssistantOpusDuration(chunk)
	if duration > 0 && duration > doubaoASTTranslatePTTOutputLimit-g.retainedDuration {
		g.retained = nil
		g.retainedDuration = 0
		g.terminal = true
		g.limitErr = fmt.Errorf(
			"%w for StreamID %q (limit %s)",
			errDoubaoASTTranslatePTTOutputLimit,
			g.streamID,
			doubaoASTTranslatePTTOutputLimit,
		)
		if err := g.output.Push(astTranslatePTTOutputLimitChunk(g.streamID, g.limitErr)); err != nil {
			return err
		}
		return g.limitErr
	}

	g.retainedDuration += duration
	g.retained = append(g.retained, chunk.Clone())
	return nil
}

func (g *astTranslatePTTOutputGate) Commit() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.limitErr != nil {
		return g.limitErr
	}
	if g.terminalErr != nil {
		return g.terminalErr
	}
	if g.terminal || g.committed {
		return nil
	}
	for _, chunk := range g.retained {
		if err := g.output.Push(chunk); err != nil {
			g.terminal = true
			g.retained = nil
			g.retainedDuration = 0
			return err
		}
	}
	g.retained = nil
	g.retainedDuration = 0
	g.committed = true
	return nil
}

func (g *astTranslatePTTOutputGate) Discard() {
	g.discard(nil)
}

func (g *astTranslatePTTOutputGate) Fail(err error) {
	g.discard(err)
}

func (g *astTranslatePTTOutputGate) discard(err error) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.terminal || g.committed {
		return
	}
	g.terminal = true
	g.terminalErr = err
	g.retained = nil
	g.retainedDuration = 0
}

func (g *astTranslatePTTOutputGate) LimitExceeded() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.limitErr != nil
}

func astTranslateAssistantOpusDuration(chunk *genx.MessageChunk) time.Duration {
	if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.Label != doubaoASTTranslateAssistantLabel {
		return 0
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil || len(blob.Data) == 0 || baseAudioMIME(blob.MIMEType) != "audio/opus" {
		return 0
	}
	return time.Duration(historyOpusPacketDurationMS(blob.Data)) * time.Millisecond
}

func astTranslatePTTOutputLimitChunk(streamID string, err error) *genx.MessageChunk {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: doubaoASTTranslateAssistantLabel,
		Ctrl: &genx.StreamCtrl{
			StreamID:    streamID,
			Label:       doubaoASTTranslateAssistantLabel,
			EndOfStream: true,
			Error:       errText,
		},
	}
}

type astTranslateGatedOutput struct {
	output astTranslateOutput
	active func() bool
}

func (o astTranslateGatedOutput) Push(chunk *genx.MessageChunk) error {
	if o.active != nil && !o.active() {
		return nil
	}
	return o.output.Push(chunk)
}

func (t *DoubaoASTTranslate) forwardEvents(output astTranslateOutput, session doubaoASTTranslateSession, streamID string, historyAudio *astTranslateHistoryAudioBuffer) (retErr error) {
	source := astTranslateTextState{role: genx.RoleUser, label: doubaoASTTranslateTranscriptLabel, streamID: streamID}
	translation := astTranslateTextState{role: genx.RoleModel, label: doubaoASTTranslateAssistantLabel, streamID: streamID}
	audio := astTranslateAudioState{streamID: streamID, mimeType: "audio/opus", decoder: newASTOggOpusFrameDecoder()}
	segment := 0
	segmentByProvider := t.inputMode != DoubaoASTTranslateInputModePushToTalk
	ensureSegment := func() string {
		if segment == 0 {
			segment = 1
		}
		id := astTranslateSegmentStreamID(streamID, segment)
		source.streamID = id
		translation.streamID = id
		audio.streamID = id
		return id
	}
	startSegment := func() string {
		if !segmentByProvider {
			return ensureSegment()
		}
		segment++
		return ensureSegment()
	}
	defer func() {
		if retErr != nil {
			if gate, ok := output.(*astTranslatePTTOutputGate); ok {
				gate.Discard()
			}
		}
		_ = source.close(output, "")
		_ = translation.close(output, "")
		_ = audio.close(output, "")
	}()
	failPTTGate := func(err error) error {
		if gate, ok := output.(*astTranslatePTTOutputGate); ok {
			gate.Fail(err)
		}
		return err
	}
	for event, err := range session.Recv() {
		if err != nil {
			failPTTGate(err)
			_ = source.close(output, err.Error())
			_ = translation.close(output, err.Error())
			_ = audio.close(output, err.Error())
			return err
		}
		if event == nil {
			continue
		}
		switch event.Type {
		case doubaospeech.ASTEventSourceSubtitleStart:
			if segmentByProvider && source.active {
				if err := source.close(output, ""); err != nil {
					return failPTTGate(err)
				}
			}
			startSegment()
			if err := source.open(output); err != nil {
				return failPTTGate(err)
			}
		case doubaospeech.ASTEventSourceSubtitleResponse:
			ensureSegment()
			if err := source.addToken(output, event.Text); err != nil {
				return failPTTGate(err)
			}
		case doubaospeech.ASTEventSourceSubtitleEnd:
			id := ensureSegment()
			if err := source.addFinal(output, event.Text); err != nil {
				return failPTTGate(err)
			}
			if segmentByProvider {
				if err := historyAudio.emitSegment(output, id, event.StartTimeMS, event.EndTimeMS); err != nil {
					return failPTTGate(err)
				}
				if err := source.close(output, ""); err != nil {
					return failPTTGate(err)
				}
			}
		case doubaospeech.ASTEventTranslationSubtitleStart:
			ensureSegment()
			if err := translation.open(output); err != nil {
				return failPTTGate(err)
			}
		case doubaospeech.ASTEventTranslationSubtitleResponse:
			ensureSegment()
			if err := translation.addToken(output, event.Text); err != nil {
				return failPTTGate(err)
			}
		case doubaospeech.ASTEventTranslationSubtitleEnd:
			ensureSegment()
			if err := translation.addFinal(output, event.Text); err != nil {
				return failPTTGate(err)
			}
			if segmentByProvider {
				if err := translation.close(output, ""); err != nil {
					return failPTTGate(err)
				}
			}
		case doubaospeech.ASTEventTTSSentenceStart:
			ensureSegment()
			if err := audio.open(output); err != nil {
				return failPTTGate(err)
			}
		case doubaospeech.ASTEventTTSResponse:
			if len(event.Audio) > 0 {
				if err := audio.add(output, event.Audio); err != nil {
					return failPTTGate(err)
				}
			}
		case doubaospeech.ASTEventTTSSentenceEnd:
			if len(event.Audio) > 0 {
				if err := audio.add(output, event.Audio); err != nil {
					return failPTTGate(err)
				}
			}
			if segmentByProvider {
				if err := audio.close(output, ""); err != nil {
					return failPTTGate(err)
				}
			} else if err := audio.finishDecoder(""); err != nil {
				return failPTTGate(err)
			}
		case doubaospeech.ASTEventSessionFinished:
			if !segmentByProvider {
				if err := historyAudio.emitSegment(output, ensureSegment(), 0, 0); err != nil {
					return failPTTGate(err)
				}
			}
			return nil
		case doubaospeech.ASTEventSessionCanceled, doubaospeech.ASTEventSessionFailed:
			var terminalErr error = event.Error
			if terminalErr == nil {
				terminalErr = fmt.Errorf("doubao ast translate terminal event %d", event.Type)
			}
			return failPTTGate(terminalErr)
		}
	}
	return nil
}

func astTranslateSegmentStreamID(base string, segment int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "audio"
	}
	if segment <= 1 {
		return base
	}
	return fmt.Sprintf("%s:ast:%d", base, segment)
}

func astTranslateInterruptedChunks(streamID string) []*genx.MessageChunk {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	textEOS := &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: doubaoASTTranslateAssistantLabel,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoASTTranslateAssistantLabel, EndOfStream: true, Error: "interrupted"},
	}
	audioEOS := &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: doubaoASTTranslateAssistantLabel,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoASTTranslateAssistantLabel, EndOfStream: true, Error: "interrupted"},
	}
	return []*genx.MessageChunk{textEOS, audioEOS}
}

type astTranslateHistoryAudioBuffer struct {
	mu   sync.Mutex
	opus timestampedHistoryAudioBuffer
}

func newASTTranslateHistoryAudioBuffer() *astTranslateHistoryAudioBuffer {
	return &astTranslateHistoryAudioBuffer{}
}

func (b *astTranslateHistoryAudioBuffer) reset() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.opus.reset()
}

func (b *astTranslateHistoryAudioBuffer) appendChunk(chunk *genx.MessageChunk, streamID string) {
	if b == nil || chunk == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.opus.append(chunk, streamID)
}

func (b *astTranslateHistoryAudioBuffer) emitSegment(output astTranslateOutput, streamID string, startMS, endMS int) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	chunks := b.opus.segment(startMS, endMS)
	b.mu.Unlock()
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			chunk.Ctrl = &genx.StreamCtrl{}
		}
		chunk.Ctrl.StreamID = streamID
	}
	return pushHistoryAudioSegment(output, streamID, chunks)
}

type astTranslateTextState struct {
	role     genx.Role
	label    string
	streamID string
	active   bool
	text     string
}

func (s *astTranslateTextState) open(output astTranslateOutput) error {
	if s.active {
		return nil
	}
	s.active = true
	return output.Push(&genx.MessageChunk{
		Role: s.role,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: s.streamID, Label: s.label, BeginOfStream: true},
	})
}

func (s *astTranslateTextState) addToken(output astTranslateOutput, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if err := s.open(output); err != nil {
		return err
	}
	delta := text
	if astTranslateNeedsSpace(s.text, delta) {
		delta = " " + delta
	}
	s.text += delta
	return output.Push(&genx.MessageChunk{
		Role: s.role,
		Part: genx.Text(delta),
		Ctrl: &genx.StreamCtrl{StreamID: s.streamID, Label: s.label},
	})
}

func (s *astTranslateTextState) addFinal(output astTranslateOutput, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if err := s.open(output); err != nil {
		return err
	}
	if realtimeNormalizeText(s.text) == realtimeNormalizeText(text) {
		s.text = text
		return nil
	}
	delta := realtimeTextDelta(s.text, text)
	if delta == "" {
		s.text = text
		return nil
	}
	if delta == text && s.text != "" {
		if astTranslateNeedsSpace(s.text, delta) {
			delta = " " + delta
		}
		s.text += delta
	} else {
		s.text = text
	}
	return output.Push(&genx.MessageChunk{
		Role: s.role,
		Part: genx.Text(delta),
		Ctrl: &genx.StreamCtrl{StreamID: s.streamID, Label: s.label},
	})
}

func astTranslateNeedsSpace(previous, next string) bool {
	if previous == "" || next == "" {
		return false
	}
	last := previous[len(previous)-1]
	first := next[0]
	return astTranslateASCIIWordByte(last) && astTranslateASCIIWordByte(first)
}

func astTranslateASCIIWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func (s *astTranslateTextState) close(output astTranslateOutput, errText string) error {
	if !s.active {
		return nil
	}
	s.active = false
	s.text = ""
	return output.Push(&genx.MessageChunk{
		Role: s.role,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: s.streamID, Label: s.label, EndOfStream: true, Error: errText},
	})
}

type astTranslateAudioState struct {
	streamID string
	mimeType string
	active   bool
	decoder  *astOggOpusFrameDecoder
}

func (s *astTranslateAudioState) open(output astTranslateOutput) error {
	if s.active {
		return nil
	}
	s.active = true
	return output.Push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: s.mimeType},
		Ctrl: &genx.StreamCtrl{StreamID: s.streamID, Label: doubaoASTTranslateAssistantLabel, BeginOfStream: true},
	})
}

func (s *astTranslateAudioState) add(output astTranslateOutput, audio []byte) error {
	if len(audio) == 0 {
		return nil
	}
	if s.decoder == nil {
		s.decoder = newASTOggOpusFrameDecoder()
	}
	frames, err := s.decoder.Write(audio)
	if err != nil {
		return fmt.Errorf("doubao ast translate decode target ogg opus: %w", err)
	}
	if len(frames) == 0 {
		return nil
	}
	if err := s.open(output); err != nil {
		return err
	}
	for _, frame := range frames {
		if err := output.Push(&genx.MessageChunk{
			Role: genx.RoleModel,
			Part: &genx.Blob{MIMEType: s.mimeType, Data: frame},
			Ctrl: &genx.StreamCtrl{StreamID: s.streamID, Label: doubaoASTTranslateAssistantLabel},
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *astTranslateAudioState) close(output astTranslateOutput, errText string) error {
	if !s.active {
		return nil
	}
	if err := s.finishDecoder(errText); err != nil {
		errText = err.Error()
	}
	s.active = false
	return output.Push(&genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: s.mimeType},
		Ctrl: &genx.StreamCtrl{StreamID: s.streamID, Label: doubaoASTTranslateAssistantLabel, EndOfStream: true, Error: errText},
	})
}

func (s *astTranslateAudioState) finishDecoder(errText string) error {
	if s.decoder == nil {
		return nil
	}
	if err := s.decoder.Close(); err != nil && errText == "" {
		s.decoder = newASTOggOpusFrameDecoder()
		return err
	}
	s.decoder = newASTOggOpusFrameDecoder()
	return nil
}

type astOggOpusFrameDecoder struct {
	pending               []byte
	packet                []byte
	expectingContinuation bool
	currentPacketBOS      bool
}

func newASTOggOpusFrameDecoder() *astOggOpusFrameDecoder {
	return &astOggOpusFrameDecoder{}
}

func (d *astOggOpusFrameDecoder) Write(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	d.pending = append(d.pending, data...)
	var frames [][]byte
	for {
		page, ok, err := d.nextPage()
		if err != nil {
			return nil, err
		}
		if !ok {
			return frames, nil
		}
		pageFrames, err := d.consumePage(page)
		if err != nil {
			return nil, err
		}
		frames = append(frames, pageFrames...)
	}
}

func (d *astOggOpusFrameDecoder) Close() error {
	if len(d.pending) != 0 {
		return fmt.Errorf("truncated ogg page: %d pending bytes", len(d.pending))
	}
	if d.expectingContinuation || len(d.packet) != 0 {
		return fmt.Errorf("stream ended with unterminated ogg packet")
	}
	return nil
}

func (d *astOggOpusFrameDecoder) nextPage() (*ogg.Page, bool, error) {
	const oggPageHeaderSize = 27
	if len(d.pending) == 0 {
		return nil, false, nil
	}
	if len(d.pending) < oggPageHeaderSize {
		if !strings.HasPrefix(ogg.CapturePattern, string(d.pending)) {
			return nil, false, fmt.Errorf("invalid ogg capture pattern prefix %q", d.pending)
		}
		return nil, false, nil
	}
	if string(d.pending[:4]) != ogg.CapturePattern {
		return nil, false, fmt.Errorf("invalid ogg capture pattern %q", d.pending[:4])
	}
	segmentCount := int(d.pending[26])
	headerLen := oggPageHeaderSize + segmentCount
	if len(d.pending) < headerLen {
		return nil, false, nil
	}
	payloadLen := 0
	for _, segment := range d.pending[oggPageHeaderSize:headerLen] {
		payloadLen += int(segment)
	}
	pageLen := headerLen + payloadLen
	if len(d.pending) < pageLen {
		return nil, false, nil
	}
	page, err := ogg.ParsePage(d.pending[:pageLen])
	if err != nil {
		return nil, false, err
	}
	d.pending = d.pending[pageLen:]
	return page, true, nil
}

func (d *astOggOpusFrameDecoder) consumePage(page *ogg.Page) ([][]byte, error) {
	if page == nil {
		return nil, fmt.Errorf("ogg page is nil")
	}
	if page.HasContinuation() {
		if !d.expectingContinuation {
			return nil, fmt.Errorf("unexpected ogg continuation page")
		}
	} else if d.expectingContinuation {
		return nil, fmt.Errorf("missing ogg continuation page")
	}

	var frames [][]byte
	payloadOffset := 0
	for segmentIndex, segment := range page.Segments {
		if !d.expectingContinuation && len(d.packet) == 0 {
			d.currentPacketBOS = page.HasBOS() && segmentIndex == 0
		}
		chunkLen := int(segment)
		if payloadOffset+chunkLen > len(page.Payload) {
			return nil, fmt.Errorf("ogg segment overflows payload")
		}
		if chunkLen > 0 {
			d.packet = append(d.packet, page.Payload[payloadOffset:payloadOffset+chunkLen]...)
		}
		payloadOffset += chunkLen
		if segment == 255 {
			d.expectingContinuation = true
			continue
		}
		packet := append([]byte(nil), d.packet...)
		d.packet = d.packet[:0]
		d.expectingContinuation = false
		d.currentPacketBOS = false
		if len(packet) == 0 || codecconv.IsOpusHeadPacket(packet) || codecconv.IsOpusTagsPacket(packet) {
			continue
		}
		frames = append(frames, packet)
	}
	if payloadOffset != len(page.Payload) {
		return nil, fmt.Errorf("ogg page has trailing payload")
	}
	return frames, nil
}
