package doubaoast

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/streamlog"
)

const (
	doubaoASTTranslateTranscriptLabel           = "transcript"
	doubaoASTTranslateAssistantLabel            = "assistant"
	doubaoASTTranslateRealtimeCompletionTimeout = 45 * time.Second
	// doubaoASTTranslateTrailingSilence is appended when the client ends its
	// audio route, before Finish. Outside zh/en, the provider drops the final
	// sentence of audio that ends without a pause, which is how a device ends
	// a turn when the key is released as the last word ends, in either input
	// mode; 200 ms already recovers the sentence.
	doubaoASTTranslateTrailingSilence = 500 * time.Millisecond

	doubaoASTTranslateSourceSampleRate = 16000
	doubaoASTTranslateSourceChannels   = 1
	doubaoASTTranslateSourceBits       = 16
)

var (
	errDoubaoASTTranslateRealtimeCompletionTimeout = errors.New("doubao ast translate: realtime session completion timeout")
)

type Transformer struct {
	client                     *doubaospeech.Client
	resourceID                 string
	mode                       doubaospeech.ASTTranslateMode
	sourceLanguage             string
	targetLanguage             string
	speakerID                  string
	isCustomSpeaker            bool
	ttsResourceID              string
	speechRate                 int
	enableSourceLanguageDetect bool
	denoise                    *bool
	realtimePacing             bool
	realtimeCompletionTimeout  time.Duration

	newSession func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error)
}

var _ genx.Transformer = (*Transformer)(nil)

type doubaoASTTranslateSession interface {
	SendAudio(context.Context, []byte) error
	Finish(context.Context) error
	Recv() iter.Seq2[*doubaospeech.ASTTranslateEvent, error]
	Close() error
}

type option func(*Transformer)

func withResourceID(resourceID string) option {
	return func(t *Transformer) {
		t.resourceID = resourceID
	}
}

func withMode(mode doubaospeech.ASTTranslateMode) option {
	return func(t *Transformer) {
		if mode != "" {
			t.mode = mode
		}
	}
}

func withSourceLanguage(language string) option {
	return func(t *Transformer) {
		t.sourceLanguage = language
	}
}

func withTargetLanguage(language string) option {
	return func(t *Transformer) {
		t.targetLanguage = language
	}
}

func withSpeakerID(speakerID string) option {
	return func(t *Transformer) {
		t.speakerID = speakerID
	}
}

func withCustomSpeaker(enabled bool) option {
	return func(t *Transformer) {
		t.isCustomSpeaker = enabled
	}
}

func withTTSResourceID(resourceID string) option {
	return func(t *Transformer) {
		t.ttsResourceID = resourceID
	}
}

func withSpeechRate(rate int) option {
	return func(t *Transformer) {
		t.speechRate = rate
	}
}

func withSourceLanguageDetect(enabled bool) option {
	return func(t *Transformer) {
		t.enableSourceLanguageDetect = enabled
	}
}

func withDenoise(enabled bool) option {
	return func(t *Transformer) {
		t.denoise = &enabled
	}
}

func withRealtimePacing(enabled bool) option {
	return func(t *Transformer) {
		t.realtimePacing = enabled
	}
}

func withRealtimeCompletionTimeout(timeout time.Duration) option {
	return func(t *Transformer) {
		t.realtimeCompletionTimeout = timeout
	}
}

func newTransformer(client *doubaospeech.Client, opts ...option) *Transformer {
	t := &Transformer{
		client:                    client,
		resourceID:                doubaospeech.ResourceASTTranslate,
		mode:                      doubaospeech.ASTTranslateModeS2T,
		sourceLanguage:            "zhen",
		targetLanguage:            "zhen",
		realtimePacing:            true,
		realtimeCompletionTimeout: doubaoASTTranslateRealtimeCompletionTimeout,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	source, err := t.transform(ctx, input)
	if err != nil {
		return nil, err
	}
	return streamkit.NewResponseStream(source)
}

func (t *Transformer) transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if t == nil || t.client == nil {
		return nil, fmt.Errorf("doubao ast translate: client is required")
	}
	if input == nil {
		return nil, fmt.Errorf("doubao ast translate: input stream is required")
	}
	ctx = streamlog.StartStage(ctx, "doubaoast", streamlog.Model{Provider: "volc", Model: t.resourceID, Kind: "ast"})
	output := newBufferStream(64, ctx)
	go t.transformLoop(ctx, input, output)
	return output, nil
}

func (t *Transformer) transformLoop(parent context.Context, input genx.Stream, output *bufferStream) {
	defer output.Close()
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(context.Canceled)
	go func() {
		select {
		case <-ctx.Done():
		case <-output.Done():
			cancel(context.Canceled)
		}
		cause := context.Cause(ctx)
		_ = output.CloseWithError(cause)
		_ = input.CloseWithError(cause)
	}()

	var session doubaoASTTranslateSession
	var inputEpoch *genx.ResponseEpoch
	var sessionEpoch *genx.ResponseEpoch
	var sessionRoutes *astTranslateRouteLifecycle
	var recvDone chan error
	var recvStart chan struct{}
	var recvFinished chan struct{}
	var recvCompletionState *atomic.Uint32
	var streamID string
	var sessionFinishing bool
	var rawOpusDecoder *opus.Decoder
	var sessionStartedAt time.Time
	var sentAudioDuration time.Duration
	var sessionSeq uint64
	var activeSessionSeq atomic.Uint64
	var terminalSessionSeq atomic.Uint64
	historyAudio := newASTTranslateHistoryAudioBuffer()
	defer func() {
		_ = output.Close()
		cancel(context.Canceled)
		activeSessionSeq.Store(0)
		if session != nil {
			_ = session.Close()
		}
		if recvFinished != nil {
			<-recvFinished
		}

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
		sessionSeq++
		activeSessionSeq.Store(sessionSeq)
		seq := sessionSeq
		active := func() bool {
			return activeSessionSeq.Load() == seq
		}
		sessionRoutes = newASTTranslateRouteLifecycle()
		visibleOutput := astTranslateLifecycleOutput{output: output, routes: sessionRoutes}
		epoch := inputEpoch
		if epoch == nil {
			epoch = genx.NewResponseEpoch(streamID)
		}
		sessionEpoch = epoch
		eventOutput := astTranslateEpochOutput{
			output: astTranslateGatedOutput{output: visibleOutput, active: active},
			epoch:  epoch,
		}
		done := make(chan error, 1)
		finished := make(chan struct{})
		completionState := &atomic.Uint32{}
		// Closing the provider releases Recv even when no more events arrive.
		stopClose := context.AfterFunc(ctx, func() { _ = next.Close() })
		markTerminal := func() {
			terminalSessionSeq.Store(seq)
		}
		start := make(chan struct{})
		recvDone = done
		recvStart = start
		recvFinished = finished
		recvCompletionState = completionState
		go func(activeStreamID string, start <-chan struct{}, eventOutput astTranslateOutput) {
			defer stopClose()
			select {
			case <-ctx.Done():
				markTerminal()
				completionState.CompareAndSwap(0, 1)
				done <- ctx.Err()
				close(finished)
				return
			case <-start:
			}
			err := t.forwardEvents(ctx, eventOutput, next, activeStreamID, historyAudio, markTerminal)
			if cause := context.Cause(ctx); cause != nil {
				err = cause
			}
			if err != nil {
				// A receive failure must not wait for the next input chunk or EOS.
				// An interrupted session no longer owns this invocation's output.
				if activeSessionSeq.CompareAndSwap(seq, 0) {
					_ = output.Fail(err)
					cancel(err)
				}
			}
			completionState.CompareAndSwap(0, 1)
			done <- err
			close(finished)
		}(streamID, start, eventOutput)
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
		finished := recvFinished
		completionState := recvCompletionState
		activeSeq := activeSessionSeq.Load()
		activeStreamID := streamID
		if recvStart != nil {
			close(recvStart)
			recvStart = nil
		}
		if !sessionFinishing {
			sessionFinishing = true
			if err := active.Finish(ctx); err != nil {
				session = nil
				sessionRoutes = nil
				recvDone = nil
				recvFinished = nil
				recvCompletionState = nil
				sessionFinishing = false
				activeSessionSeq.Store(0)
				_ = active.Close()
				return err
			}
			if t.realtimeCompletionTimeout > 0 &&
				finished != nil && completionState != nil && activeSeq != 0 {
				timeout := t.realtimeCompletionTimeout
				go func() {
					timer := time.NewTimer(timeout)
					defer timer.Stop()
					select {
					case <-ctx.Done():
						return
					case <-finished:
						return
					case <-timer.C:
						if !completionState.CompareAndSwap(0, 2) || !activeSessionSeq.CompareAndSwap(activeSeq, 0) {
							return
						}
						timeoutErr := fmt.Errorf("%w after %s for stream %q", errDoubaoASTTranslateRealtimeCompletionTimeout, timeout, activeStreamID)
						// Close input before output: closing output wakes the
						// cancellation watcher, which would otherwise close
						// input first with context.Canceled.
						_ = input.CloseWithError(timeoutErr)
						_ = output.CloseWithError(timeoutErr)
						cancel(timeoutErr)
						_ = active.Close()
					}
				}()
			}
		}
		if !wait {
			return nil
		}
		session = nil
		sessionRoutes = nil
		recvDone = nil
		recvFinished = nil
		recvCompletionState = nil
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
			session = nil
			sessionRoutes = nil
			recvDone = nil
			recvFinished = nil
			recvCompletionState = nil
			sessionFinishing = false
			activeSessionSeq.Store(0)
			_ = active.Close()
			streamID = ""
			return err
		default:
			return nil
		}
	}

	interruptSession := func(id string) error {
		if session == nil {
			streamID = strings.TrimSpace(id)
			return nil
		}
		active := session
		done := recvDone
		routes := sessionRoutes
		activeSessionSeq.Store(0)
		interruptedStreamID := strings.TrimSpace(streamID)
		if interruptedStreamID == "" {
			interruptedStreamID = strings.TrimSpace(id)
		}
		if interruptedStreamID == "" {
			interruptedStreamID = "audio"
		}
		session = nil
		sessionRoutes = nil
		recvDone = nil
		recvFinished = nil
		recvCompletionState = nil
		sessionFinishing = false
		if recvStart != nil {
			close(recvStart)
			recvStart = nil
		}
		{
			chunks := astTranslateInterruptedChunks(interruptedStreamID, t.mode == doubaospeech.ASTTranslateModeS2S)
			if routes != nil {
				chunks = routes.interruptedChunks(interruptedStreamID, t.mode == doubaospeech.ASTTranslateModeS2S)
			}
			for _, chunk := range chunks {
				if err := (astTranslateEpochOutput{output: output, epoch: sessionEpoch}).Push(chunk); err != nil {
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
		historyAudio.reset()
		sessionStartedAt = time.Time{}
		sentAudioDuration = 0
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			output.CloseWithError(err)
			return
		}
		chunk, err := streamlog.ReadInput(ctx, input)
		if err != nil {
			if !errors.Is(err, genx.ErrDone) && !errors.Is(err, io.EOF) {
				if session != nil {
					_ = session.Close()
				}
				output.CloseWithError(err)
				return
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
			if strings.TrimSpace(streamID) != "" && strings.TrimSpace(id) != "" && (id != streamID || (chunk.Ctrl.ResponseEpoch != nil && chunk.Ctrl.ResponseEpoch != inputEpoch)) {
				var err error
				activeSeq := activeSessionSeq.Load()
				if activeSeq != 0 && terminalSessionSeq.Load() == activeSeq {
					sessionFinishing = true
					err = finishSession(true)
				} else {
					err = interruptSession(id)
				}
				if err != nil {
					output.CloseWithError(err)
					return
				}
			}
			streamID = id
			inputEpoch = chunk.Ctrl.ResponseEpoch
			continue
		}
		if strings.TrimSpace(streamID) != "" && strings.TrimSpace(id) != "" && id != streamID {
			continue
		}
		if chunk.IsEndOfStream() {
			if blob, ok := chunk.Part.(*genx.Blob); ok && isAudioMIME(blob.MIMEType) {
				if session != nil {
					t.sendTrailingSilence(ctx, session)
				}

				if err := finishSession(false); err != nil {
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
		for audioChunk := range splitDoubaoASRAudio(audio, t.audioChunkSize()) {
			if err := sendAudio(audioChunk); err != nil {
				_ = session.Close()
				output.CloseWithError(err)
				return
			}
		}
	}
}

func (t *Transformer) sessionConfig() doubaospeech.ASTTranslateConfig {
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

func (t *Transformer) openSession(ctx context.Context, cfg doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
	session, err := t.client.ASTTranslate.OpenSession(ctx, &cfg)
	if session == nil {
		// Avoid wrapping a nil *ASTTranslateSession in a non-nil interface.
		return nil, err
	}
	return session, err
}

func (t *Transformer) prepareAudioBlob(blob *genx.Blob, rawOpusDecoder **opus.Decoder) ([]byte, error) {
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
		return decodeMP3ToPCM(blob.Data, cfg)
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

func (t *Transformer) audioChunkSize() int {
	return doubaoASTTranslateSourceSampleRate * doubaoASTTranslateSourceChannels * (doubaoASTTranslateSourceBits / 8) / 10
}

// sendTrailingSilence gives the provider the pause that ends the last sentence
// of an audio route the client closed. It is sent at once, not paced: the
// provider segments on the audio it receives, not on arrival time, so pacing
// would only delay the reply. It is not user speech, so it stays out of the
// history audio. A send failure stops the padding only: a session that already
// ended reports its terminal state through Finish and the event stream.
func (t *Transformer) sendTrailingSilence(ctx context.Context, session doubaoASTTranslateSession) {
	silence := make([]byte, doubaoASTTranslateSourceSampleRate*doubaoASTTranslateSourceChannels*(doubaoASTTranslateSourceBits/8)*int(doubaoASTTranslateTrailingSilence/time.Millisecond)/1000)
	for audio := range splitDoubaoASRAudio(silence, t.audioChunkSize()) {
		if session.SendAudio(ctx, audio) != nil {
			return
		}
	}
}

type astTranslateOutput interface {
	Push(*genx.MessageChunk) error
}

type astTranslateRouteLifecycle struct {
	mu     sync.Mutex
	routes map[string]*astTranslateOutputRoute
}

type astTranslateOutputRoute struct {
	role     genx.Role
	name     string
	label    string
	streamID string
	mimeType string
	text     bool
	begun    bool
	done     bool
}

func newASTTranslateRouteLifecycle() *astTranslateRouteLifecycle {
	return &astTranslateRouteLifecycle{routes: make(map[string]*astTranslateOutputRoute)}
}

func (l *astTranslateRouteLifecycle) observe(chunk *genx.MessageChunk) {
	if l == nil || chunk == nil || chunk.Ctrl == nil {
		return
	}
	mimeType, ok := chunk.MIMEType()
	if !ok {
		return
	}
	key := string(chunk.Role) + "\x00" + chunk.Ctrl.Label + "\x00" + chunk.Ctrl.StreamID + "\x00" + mimeType
	l.mu.Lock()
	defer l.mu.Unlock()
	route := l.routes[key]
	if route == nil {
		_, text := chunk.Part.(genx.Text)
		route = &astTranslateOutputRoute{
			role: chunk.Role, name: chunk.Name, label: chunk.Ctrl.Label,
			streamID: chunk.Ctrl.StreamID, mimeType: mimeType, text: text,
		}
		l.routes[key] = route
	}
	if chunk.IsBeginOfStream() {
		route.begun = true
	}
	if chunk.IsEndOfStream() {
		route.done = true
	}
}

func (l *astTranslateRouteLifecycle) interruptedChunks(fallback string, wantAudio bool) []*genx.MessageChunk {
	if l == nil {
		return astTranslateInterruptedChunks(fallback, wantAudio)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	keys := make([]string, 0, len(l.routes))
	for key := range l.routes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]*genx.MessageChunk, 0, len(keys)+4)
	assistantTextSeen := false
	assistantAudioSeen := false
	for _, key := range keys {
		route := l.routes[key]
		if route.role == genx.RoleModel && route.label == doubaoASTTranslateAssistantLabel {
			if route.text {
				assistantTextSeen = true
			} else if strings.HasPrefix(baseAudioMIME(route.mimeType), "audio/") {
				assistantAudioSeen = true
			}
		}
		if !route.begun || route.done {
			continue
		}
		result = append(result, astTranslateRouteBoundary(route, false, "interrupted"))
	}
	if !assistantTextSeen {
		result = append(result, astTranslateEmptyInterruptedRoute(fallback, true)...)
	}
	if wantAudio && !assistantAudioSeen {
		result = append(result, astTranslateEmptyInterruptedRoute(fallback, false)...)
	}
	return result
}

type astTranslateLifecycleOutput struct {
	output astTranslateOutput
	routes *astTranslateRouteLifecycle
}

func (o astTranslateLifecycleOutput) Push(chunk *genx.MessageChunk) error {
	o.routes.observe(chunk)
	if err := o.output.Push(chunk); err != nil {
		return err
	}
	return nil
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

func (t *Transformer) forwardEvents(
	ctx context.Context,
	output astTranslateOutput,
	session doubaoASTTranslateSession,
	streamID string,
	historyAudio *astTranslateHistoryAudioBuffer,
	markTerminal func(),
) (retErr error) {
	defer markTerminal()
	source := astTranslateTextState{role: genx.RoleUser, label: doubaoASTTranslateTranscriptLabel, streamID: streamID}
	translation := astTranslateTextState{role: genx.RoleModel, label: doubaoASTTranslateAssistantLabel, streamID: streamID}
	audio := astTranslateAudioState{streamID: streamID, mimeType: "audio/opus", decoder: newASTOggOpusFrameDecoder()}
	segment := 0
	translated := false
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
		segment++
		return ensureSegment()
	}
	defer func() {
		_ = source.close(output, "")
		_ = translation.close(output, "")
		_ = audio.close(output, "")
		if retErr == nil {
			// Process-local completion covers every provider subtitle in this
			// session. It has no wire boundary or payload of its own.
			_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: translation.streamID, Label: doubaoASTTranslateAssistantLabel, ResponseEpochEnd: true}})
		}
	}()
	events := session.Recv()
	for event, err := range events {
		if err != nil {
			markTerminal()
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
			if source.active {
				if err := source.close(output, ""); err != nil {
					return err
				}
			}
			if segment > 0 {
				if err := translation.close(output, ""); err != nil {
					return err
				}
			}
			startSegment()
			if err := source.open(output); err != nil {
				return err
			}
		case doubaospeech.ASTEventSourceSubtitleResponse:
			ensureSegment()
			if err := source.addToken(output, event.Text); err != nil {
				return err
			}
		case doubaospeech.ASTEventSourceSubtitleEnd:
			id := ensureSegment()
			if err := source.addFinal(output, event.Text); err != nil {
				return err
			}
			if err := historyAudio.emitSegment(output, id, event.StartTimeMS, event.EndTimeMS); err != nil {
				return err
			}
			if err := source.close(output, ""); err != nil {
				return err
			}
		case doubaospeech.ASTEventTranslationSubtitleStart:
			translated = true
			ensureSegment()
			if err := translation.startSubtitle(output); err != nil {
				return err
			}
		case doubaospeech.ASTEventTranslationSubtitleResponse:
			ensureSegment()
			if err := translation.addToken(output, event.Text); err != nil {
				return err
			}
		case doubaospeech.ASTEventTranslationSubtitleEnd:
			ensureSegment()
			if err := translation.addFinal(output, event.Text); err != nil {
				return err
			}
			// Every confirmed subtitle flushes synthesis and closes its text
			// channel. A later subtitle opens a new lifecycle independently
			// of how the provider groups its source subtitles.
			if err := output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: translation.streamID, Label: doubaoASTTranslateAssistantLabel, MessageEnd: true}}); err != nil {
				return err
			}
			if err := translation.endSubtitle(output); err != nil {
				return err
			}
		case doubaospeech.ASTEventTTSSentenceStart:
			translated = true
			ensureSegment()
			if err := audio.open(output); err != nil {
				return err
			}
		case doubaospeech.ASTEventTTSResponse:
			if len(event.Audio) > 0 {
				if err := audio.add(output, event.Audio); err != nil {
					return err
				}
			}
		case doubaospeech.ASTEventTTSSentenceEnd:
			if len(event.Audio) > 0 {
				if err := audio.add(output, event.Audio); err != nil {
					return err
				}
			}
			if err := audio.close(output, ""); err != nil {
				return err
			}
		case doubaospeech.ASTEventUsageResponse:
			t.recordUsage(ctx, event.Usage)
		case doubaospeech.ASTEventSessionFinished:
			markTerminal()
			if !translated {
				ensureSegment()
				if err := t.closeEmptyTranslation(output, &translation, &audio); err != nil {
					return err
				}
			}
			return nil
		case doubaospeech.ASTEventSessionCanceled, doubaospeech.ASTEventSessionFailed:
			markTerminal()
			if event.Error != nil {
				return event.Error
			}
			return fmt.Errorf("doubao ast translate terminal event %d", event.Type)
		}
	}
	return fmt.Errorf("doubao ast translate session closed before SessionFinished: %w", io.ErrUnexpectedEOF)
}

// closeEmptyTranslation opens and normally closes the assistant routes a turn
// would have carried: text always, and audio when the provider synthesizes it.
func (t *Transformer) closeEmptyTranslation(output astTranslateOutput, translation *astTranslateTextState, audio *astTranslateAudioState) error {
	if err := translation.open(output); err != nil {
		return err
	}
	if err := translation.close(output, ""); err != nil {
		return err
	}
	if t.mode != doubaospeech.ASTTranslateModeS2S {
		return nil
	}
	if err := audio.open(output); err != nil {
		return err
	}
	return audio.close(output, "")
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

func astTranslateInterruptedChunks(streamID string, wantAudio bool) []*genx.MessageChunk {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	chunks := astTranslateEmptyInterruptedRoute(streamID, true)
	if wantAudio {
		chunks = append(chunks, astTranslateEmptyInterruptedRoute(streamID, false)...)
	}
	return chunks
}

func astTranslateEmptyInterruptedRoute(streamID string, text bool) []*genx.MessageChunk {
	route := &astTranslateOutputRoute{
		role: genx.RoleModel, name: doubaoASTTranslateAssistantLabel,
		label: doubaoASTTranslateAssistantLabel, streamID: streamID,
		mimeType: "audio/opus", text: text,
	}
	if text {
		route.mimeType = "text/plain"
	}
	return []*genx.MessageChunk{
		astTranslateRouteBoundary(route, true, ""),
		astTranslateRouteBoundary(route, false, "interrupted"),
	}
}

func astTranslateRouteBoundary(route *astTranslateOutputRoute, begin bool, errText string) *genx.MessageChunk {
	var part genx.Part = genx.Text("")
	if !route.text {
		part = &genx.Blob{MIMEType: route.mimeType}
	}
	ctrl := &genx.StreamCtrl{StreamID: route.streamID, Label: route.label, BeginOfStream: begin}
	if !begin {
		ctrl.EndOfStream = true
		ctrl.Error = errText
	}
	return &genx.MessageChunk{Role: route.role, Name: route.name, Part: part, Ctrl: ctrl}
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
	role             genx.Role
	label            string
	streamID         string
	active           bool
	text             string
	allText          string
	previousSubtitle string
	// sentenceEnded marks that a provider subtitle ended, so the next token
	// starts a new sentence that may need a separating space.
	sentenceEnded bool
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

func (s *astTranslateTextState) startSubtitle(output astTranslateOutput) error {
	if s.sentenceEnded {
		s.previousSubtitle = s.text
		s.text = ""
		s.sentenceEnded = false
	}
	return s.open(output)
}

func (s *astTranslateTextState) endSubtitle(output astTranslateOutput) error {
	s.sentenceEnded = true
	return s.closeRoute(output, "")
}

// addToken forwards one provider subtitle token. Provider tokens are subword
// pieces that carry their own word spacing (for example "Bon", "jour", " à"),
// so tokens inside one subtitle are joined verbatim. Only the first token of a
// new subtitle may need a separating space from the previous sentence.
func (s *astTranslateTextState) addToken(output astTranslateOutput, text string) error {
	if s.text == "" && s.previousSubtitle == "" {
		text = strings.TrimLeftFunc(text, unicode.IsSpace)
	}
	if text == "" {
		return nil
	}
	if err := s.open(output); err != nil {
		return err
	}
	if s.sentenceEnded && astTranslateNeedsSpace(s.text, text) {
		text = " " + text
	}
	if s.text == "" && astTranslateNeedsSpace(s.previousSubtitle, text) {
		text = " " + text
	}
	if s.text == "" && astTranslateNeedsSpace(s.allText, text) {
		s.allText += " "
	}
	s.sentenceEnded = false
	s.text += text
	s.allText += text
	return output.Push(&genx.MessageChunk{
		Role: s.role,
		Part: genx.Text(text),
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
	// A final can cover this subtitle or the complete session. Convert a
	// cumulative final to this provider segment before deriving its delta.
	if previous := strings.TrimSpace(s.allText); previous != "" && previous != strings.TrimSpace(s.text) {
		delta, cumulative := strings.CutPrefix(text, previous)
		if !cumulative {
			normalized := realtimeNormalizeText(previous)
			cumulative = normalized != "" && strings.HasPrefix(realtimeNormalizeText(text), normalized)
			delta = realtimeTextDelta(previous, text)
		}
		// With no interim tokens, only an extending final is cumulative.
		// An equal final can be a fresh subtitle repeating the same words.
		extends := strings.IndexFunc(delta, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) >= 0
		if cumulative && (s.text != "" || extends) {
			text = s.text + delta
		}
	}
	s.sentenceEnded = true
	if s.text != "" && realtimeNormalizeText(s.text) == realtimeNormalizeText(text) {
		// Nothing is emitted, so keep the whitespace the stream already ends
		// with; the next subtitle must not add a second separator.
		s.text = text + trailingSpace(s.text)
		return nil
	}
	delta := realtimeTextDelta(s.text, text)
	if delta == "" {
		s.text = text
		return nil
	}
	firstInSegment := s.text == ""
	if delta == text && s.text != "" {
		if astTranslateNeedsSpace(s.text, delta) {
			delta = " " + delta
		}
		s.text += delta
	} else {
		s.text = text
	}
	if firstInSegment && astTranslateNeedsSpace(s.previousSubtitle, delta) {
		delta = " " + delta
	}
	if firstInSegment && astTranslateNeedsSpace(s.allText, delta) {
		s.allText += " "
	}
	s.allText += delta
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

func trailingSpace(text string) string {
	return text[len(strings.TrimRightFunc(text, unicode.IsSpace)):]
}

func (s *astTranslateTextState) close(output astTranslateOutput, errText string) error {
	err := s.closeRoute(output, errText)
	s.text = ""
	s.previousSubtitle = ""
	s.sentenceEnded = false
	return err
}

func (s *astTranslateTextState) closeRoute(output astTranslateOutput, errText string) error {
	if !s.active {
		return nil
	}
	s.active = false
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
