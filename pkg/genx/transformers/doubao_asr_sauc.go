package transformers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkg/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

// DoubaoASRSAUC is an ASR transformer using Doubao BigModel ASR (大模型语音识别).
//
// Resource ID: volc.bigasr.sauc.duration
//
// Input type: audio/* (audio/ogg, audio/pcm, etc.)
// Output type: text/plain
//
// EoS Handling:
//   - When receiving an audio/* EoS marker, finish current ASR, emit results, then emit text/plain EoS
//   - Non-audio chunks are passed through unchanged
//
// Note: The transformer adapts common audio containers/codecs to Doubao's
// session format. MP3 and raw PCM input are sent through a PCM ASR session.
type DoubaoASRSAUC struct {
	client         *doubaospeech.Client
	format         string
	sampleRate     int
	channels       int
	bits           int
	language       string
	enableITN      bool
	enablePunc     bool
	hotwords       []string
	resultType     string // "single" (default) or "full"
	resourceID     string
	chunkSize      int
	realtimePacing bool
	emitInterim    bool

	newSession func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error)
}

var _ genx.Transformer = (*DoubaoASRSAUC)(nil)

type doubaoASRSession interface {
	SendAudio(context.Context, []byte, bool) error
	Recv() iter.Seq2[*doubaospeech.ASRV2Result, error]
	Close() error
}

type doubaoASRSessionConfig struct {
	format     string
	sampleRate int
	channels   int
	bits       int
}

// DoubaoASRSAUCOption is a functional option for DoubaoASRSAUC.
type DoubaoASRSAUCOption func(*DoubaoASRSAUC)

// WithDoubaoASRSAUCFormat sets the audio format (pcm, wav, mp3, ogg_opus).
func WithDoubaoASRSAUCFormat(format string) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.format = format
	}
}

// WithDoubaoASRSAUCSampleRate sets the sample rate (8000, 16000, etc.).
func WithDoubaoASRSAUCSampleRate(sampleRate int) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.sampleRate = sampleRate
	}
}

// WithDoubaoASRSAUCChannels sets the number of channels (1 or 2).
func WithDoubaoASRSAUCChannels(channels int) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.channels = channels
	}
}

// WithDoubaoASRSAUCBits sets the bits per sample (16, etc.).
func WithDoubaoASRSAUCBits(bits int) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.bits = bits
	}
}

// WithDoubaoASRSAUCLanguage sets the language (zh-CN, en-US, ja-JP, etc.).
func WithDoubaoASRSAUCLanguage(language string) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.language = language
	}
}

// WithDoubaoASRSAUCEnableITN enables Inverse Text Normalization.
func WithDoubaoASRSAUCEnableITN(enable bool) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.enableITN = enable
	}
}

// WithDoubaoASRSAUCEnablePunc enables punctuation.
func WithDoubaoASRSAUCEnablePunc(enable bool) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.enablePunc = enable
	}
}

// WithDoubaoASRSAUCHotwords sets hotwords for recognition boost.
func WithDoubaoASRSAUCHotwords(hotwords []string) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.hotwords = hotwords
	}
}

// WithDoubaoASRSAUCResultType sets the result type.
// Options: "single" (default, only definite results), "full" (all results including interim).
func WithDoubaoASRSAUCResultType(resultType string) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.resultType = resultType
	}
}

// WithDoubaoASRSAUCEmitInterim emits non-definite ASR text chunks.
func WithDoubaoASRSAUCEmitInterim(enabled bool) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.emitInterim = enabled
	}
}

// WithDoubaoASRSAUCResourceID sets the ASR resource ID.
func WithDoubaoASRSAUCResourceID(resourceID string) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.resourceID = resourceID
	}
}

// WithDoubaoASRSAUCChunkSize sets the maximum audio frame size sent to Doubao.
func WithDoubaoASRSAUCChunkSize(chunkSize int) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.chunkSize = chunkSize
	}
}

// WithDoubaoASRSAUCRealtimePacing controls whether PCM frames are sent no
// faster than their audio duration. Streaming ASR services can truncate or
// delay results when long PCM files are uploaded as fast as memory allows.
func WithDoubaoASRSAUCRealtimePacing(enabled bool) DoubaoASRSAUCOption {
	return func(t *DoubaoASRSAUC) {
		t.realtimePacing = enabled
	}
}

// NewDoubaoASRSAUC creates a new DoubaoASRSAUC transformer.
//
// Parameters:
//   - client: Doubao speech client
//   - opts: Optional configuration
func NewDoubaoASRSAUC(client *doubaospeech.Client, opts ...DoubaoASRSAUCOption) *DoubaoASRSAUC {
	t := &DoubaoASRSAUC{
		client:         client,
		format:         "pcm",
		sampleRate:     16000,
		channels:       1,
		bits:           16,
		language:       "zh-CN",
		enableITN:      true,
		enablePunc:     true,
		resultType:     "single", // only definite results
		resourceID:     doubaospeech.ResourceASRStream,
		realtimePacing: true,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// DoubaoASRSAUCCtxKey is the context key for runtime options.
type doubaoASRSAUCCtxKey struct{}

// DoubaoASRSAUCCtxOptions are runtime options passed via context.
type DoubaoASRSAUCCtxOptions struct{}

// WithDoubaoASRSAUCCtxOptions attaches runtime options to context.
func WithDoubaoASRSAUCCtxOptions(ctx context.Context, opts DoubaoASRSAUCCtxOptions) context.Context {
	return context.WithValue(ctx, doubaoASRSAUCCtxKey{}, opts)
}

// Transform converts audio Blob chunks to Text chunks.
// DoubaoASRSAUC creates sessions on demand, so it returns immediately.
// The ctx is unused (session creation happens lazily in the loop);
// the goroutine lifetime is governed by the input Stream.
func (t *DoubaoASRSAUC) Transform(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	output := newBufferStream(100)

	go t.transformLoop(input, output)

	return output, nil
}

func (t *DoubaoASRSAUC) transformLoop(input genx.Stream, output *bufferStream) {
	defer output.Close()

	// Local cancel context tied to the loop lifecycle.
	// When the loop exits, defer cancel() cancels any in-flight WebSocket
	// dial or audio send operation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track last chunk for metadata
	var lastChunk *genx.MessageChunk
	var session doubaoASRSession
	var resultsCh chan *genx.MessageChunk
	var resultsDone chan error
	var resultsForwarded chan struct{}
	var pendingAudio []byte
	var sessionConfig doubaoASRSessionConfig
	var sessionStartedAt time.Time
	var sentAudioDuration time.Duration
	var rawOpusDecoder *opus.Decoder
	defer func() {
		if rawOpusDecoder != nil {
			_ = rawOpusDecoder.Close()
		}
	}()

	// Helper to start a new ASR session
	startSession := func(cfg doubaoASRSessionConfig) error {
		var err error
		openSession := t.openSession
		if t.newSession != nil {
			openSession = t.newSession
		}
		session, err = openSession(ctx, cfg)
		if err != nil {
			return err
		}
		sessionConfig = cfg
		sessionStartedAt = time.Time{}
		sentAudioDuration = 0
		resultsCh = make(chan *genx.MessageChunk, 100)
		resultsDone = make(chan error, 1)
		resultsForwarded = make(chan struct{})
		go t.receiveResults(session, lastChunk, resultsCh, resultsDone)
		// Forward results to output as they arrive
		go func() {
			defer close(resultsForwarded)
			for chunk := range resultsCh {
				output.Push(chunk)
			}
		}()
		return nil
	}

	sendAudio := func(data []byte, isLast bool) error {
		if t.realtimePacing && len(data) > 0 && sessionConfig.isPCM() {
			if sessionStartedAt.IsZero() {
				sessionStartedAt = time.Now()
			}
			if delay := sessionStartedAt.Add(sentAudioDuration).Sub(time.Now()); delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
				case <-ctx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return ctx.Err()
				}
			}
		}
		if err := session.SendAudio(ctx, data, isLast); err != nil {
			return err
		}
		if len(data) > 0 && sessionConfig.isPCM() {
			sentAudioDuration += audioDuration(data, sessionConfig)
		}
		return nil
	}

	// Helper to finish current session
	finishSession := func() error {
		if session == nil {
			return nil
		}
		if len(pendingAudio) > 0 {
			if err := sendAudio(pendingAudio, true); err != nil {
				session.Close()
				session = nil
				pendingAudio = nil
				return err
			}
			pendingAudio = nil
		} else if err := sendAudio(nil, true); err != nil {
			session.Close()
			session = nil
			return err
		}
		err := <-resultsDone
		<-resultsForwarded
		session.Close()
		session = nil
		sessionConfig = doubaoASRSessionConfig{}
		return err
	}

	// Process input stream
	for {

		chunk, err := input.Next()
		if err != nil {
			if !errors.Is(err, genx.ErrDone) && !errors.Is(err, io.EOF) {
				if session != nil {
					session.Close()
				}
				output.CloseWithError(err)
				return
			}
			// EOF: finish current session
			if err := finishSession(); err != nil {
				output.CloseWithError(err)
				return
			}
			return
		}

		if chunk == nil {
			continue
		}

		lastChunk = chunk

		// Check for EoS marker with audio MIME type
		if chunk.IsEndOfStream() {
			if blob, ok := chunk.Part.(*genx.Blob); ok && isAudioMIME(blob.MIMEType) {
				// Audio EoS: finish current session, emit text EoS
				if err := finishSession(); err != nil {
					output.CloseWithError(err)
					return
				}
				if !t.emitInterim {
					eosChunk := genx.NewTextEndOfStream()
					eosChunk.Role = lastChunk.Role
					eosChunk.Name = lastChunk.Name
					if err := output.Push(eosChunk); err != nil {
						return
					}
				}
				continue
			}
			// Non-audio EoS: pass through
			if err := output.Push(chunk); err != nil {
				return
			}
			continue
		}

		// Handle audio blob
		if blob, ok := chunk.Part.(*genx.Blob); ok && isAudioMIME(blob.MIMEType) {
			var target *doubaoASRSessionConfig
			if session != nil {
				target = &sessionConfig
			}
			audioData, cfg, err := t.prepareAudioBlob(blob, target, &rawOpusDecoder)
			if err != nil {
				if session != nil {
					session.Close()
				}
				output.CloseWithError(err)
				return
			}
			// Start session on first audio chunk after MIME-based format detection.
			if session == nil {
				if err := startSession(cfg); err != nil {
					output.CloseWithError(err)
					return
				}
			}
			if len(audioData) > 0 {
				for audio := range splitDoubaoASRAudio(audioData, t.audioChunkSize(cfg)) {
					if len(pendingAudio) > 0 {
						if err := sendAudio(pendingAudio, false); err != nil {
							session.Close()
							output.CloseWithError(err)
							return
						}
					}
					pendingAudio = audio
				}
			}
		} else {
			// Non-audio chunk: pass through
			if err := output.Push(chunk); err != nil {
				return
			}
		}
	}
}

func (t *DoubaoASRSAUC) prepareAudioBlob(blob *genx.Blob, target *doubaoASRSessionConfig, rawOpusDecoder **opus.Decoder) ([]byte, doubaoASRSessionConfig, error) {
	cfg := t.defaultSessionConfig()
	if target != nil {
		cfg = *target
	}
	if blob == nil || len(blob.Data) == 0 {
		return nil, cfg, nil
	}

	mimeType := baseAudioMIME(blob.MIMEType)
	if isASRMP3MIME(mimeType) {
		cfg = cfg.withPCM()
		pcm, err := t.decodeMP3ToPCM(blob.Data, cfg)
		if err != nil {
			return nil, cfg, err
		}
		return pcm, cfg, nil
	}
	if isASRPCMMIME(mimeType) {
		cfg = cfg.withPCM()
		return blob.Data, cfg, nil
	}
	if cfg.isPCM() && isOggAudioMIME(mimeType) {
		var pcm bytes.Buffer
		if _, err := codecconv.OggToPCM(&pcm, bytes.NewReader(blob.Data), opus.OpusSampleRate(cfg.sampleRate)); err != nil {
			return nil, cfg, fmt.Errorf("decode ogg opus for doubao asr pcm input: %w", err)
		}
		return pcm.Bytes(), cfg, nil
	}
	if cfg.isPCM() && isASROpusMIME(mimeType) {
		pcm, err := decodeRawOpusToPCM(blob.Data, cfg, rawOpusDecoder)
		if err != nil {
			return nil, cfg, err
		}
		return pcm, cfg, nil
	}
	return blob.Data, cfg, nil
}

func (t *DoubaoASRSAUC) defaultSessionConfig() doubaoASRSessionConfig {
	return doubaoASRSessionConfig{
		format:     t.format,
		sampleRate: t.sampleRate,
		channels:   t.channels,
		bits:       t.bits,
	}
}

func (c doubaoASRSessionConfig) withPCM() doubaoASRSessionConfig {
	c.format = "pcm"
	if c.sampleRate <= 0 {
		c.sampleRate = 16000
	}
	if c.channels <= 0 {
		c.channels = 1
	}
	if c.bits <= 0 {
		c.bits = 16
	}
	return c
}

func (c doubaoASRSessionConfig) isPCM() bool {
	format := strings.ToLower(strings.TrimSpace(c.format))
	return format == "pcm" || format == "pcm_s16le"
}

func (t *DoubaoASRSAUC) openSession(ctx context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
	format := cfg.format
	if format == "ogg" {
		format = string(doubaospeech.FormatOGG)
	}

	config := &doubaospeech.ASRV2Config{
		Format:     doubaospeech.AudioFormat(format),
		SampleRate: doubaospeech.SampleRate(cfg.sampleRate),
		Channels:   cfg.channels,
		Bits:       cfg.bits,
		Language:   doubaospeech.Language(t.language),
		EnableITN:  t.enableITN,
		EnablePunc: t.enablePunc,
		Hotwords:   t.hotwords,
		ResultType: t.resultType,
		ResourceID: t.resourceID,
	}
	return t.client.ASRV2.OpenStreamSession(ctx, config)
}

func (t *DoubaoASRSAUC) audioChunkSize(cfg doubaoASRSessionConfig) int {
	if t.chunkSize > 0 {
		return t.chunkSize
	}
	if cfg.isPCM() {
		bytesPerSample := cfg.bits / 8
		if bytesPerSample <= 0 {
			bytesPerSample = 2
		}
		channels := cfg.channels
		if channels <= 0 {
			channels = 1
		}
		sampleRate := cfg.sampleRate
		if sampleRate <= 0 {
			sampleRate = 16000
		}
		return sampleRate * bytesPerSample * channels / 10
	}
	return 256
}

func audioDuration(data []byte, cfg doubaoASRSessionConfig) time.Duration {
	bytesPerSample := cfg.bits / 8
	if bytesPerSample <= 0 {
		bytesPerSample = 2
	}
	channels := cfg.channels
	if channels <= 0 {
		channels = 1
	}
	sampleRate := cfg.sampleRate
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	bytesPerSecond := sampleRate * channels * bytesPerSample
	if bytesPerSecond <= 0 {
		return 0
	}
	return time.Duration(len(data)) * time.Second / time.Duration(bytesPerSecond)
}

func (t *DoubaoASRSAUC) decodeMP3ToPCM(data []byte, cfg doubaoASRSessionConfig) ([]byte, error) {
	decoded, sampleRate, channels, err := mp3.DecodeFull(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode mp3 for doubao asr pcm input: %w", err)
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("decode mp3 for doubao asr pcm input: invalid sample rate %d", sampleRate)
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("decode mp3 for doubao asr pcm input: unsupported channels %d", channels)
	}
	if cfg.channels != 1 && cfg.channels != 2 {
		return nil, fmt.Errorf("doubao asr unsupported target channels %d", cfg.channels)
	}

	srcFmt := resampler.Format{SampleRate: sampleRate, Stereo: channels == 2}
	dstFmt := resampler.Format{SampleRate: cfg.sampleRate, Stereo: cfg.channels == 2}
	if srcFmt == dstFmt {
		return decoded, nil
	}

	rs, err := resampler.New(bytes.NewReader(decoded), srcFmt, dstFmt)
	if err != nil {
		return nil, fmt.Errorf("create mp3 pcm resampler for doubao asr: %w", err)
	}
	defer func() {
		_ = rs.Close()
	}()
	pcm, err := io.ReadAll(rs)
	if err != nil {
		return nil, fmt.Errorf("resample mp3 pcm for doubao asr: %w", err)
	}
	return pcm, nil
}

func decodeRawOpusToPCM(data []byte, cfg doubaoASRSessionConfig, decoder **opus.Decoder) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if cfg.sampleRate <= 0 {
		cfg.sampleRate = 16000
	}
	if cfg.channels <= 0 {
		cfg.channels = 1
	}
	if cfg.channels != 1 && cfg.channels != 2 {
		return nil, fmt.Errorf("doubao asr unsupported raw opus target channels %d", cfg.channels)
	}
	if *decoder == nil {
		dec, err := opus.NewDecoder(cfg.sampleRate, cfg.channels)
		if err != nil {
			return nil, fmt.Errorf("create raw opus decoder for doubao asr pcm input: %w", err)
		}
		*decoder = dec
	}
	frameSize := (cfg.sampleRate * 3) / 50
	samples, err := (*decoder).Decode(data, frameSize, false)
	if err != nil {
		return nil, fmt.Errorf("decode raw opus for doubao asr pcm input: %w", err)
	}
	return pcm16LE(samples), nil
}

func splitDoubaoASRAudio(data []byte, chunkSize int) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		if chunkSize <= 0 {
			chunkSize = 256
		}
		for offset := 0; offset < len(data); offset += chunkSize {
			end := min(offset+chunkSize, len(data))
			if !yield(data[offset:end]) {
				return
			}
		}
	}
}

func (t *DoubaoASRSAUC) receiveResults(session doubaoASRSession, lastChunk *genx.MessageChunk, resultsCh chan<- *genx.MessageChunk, done chan<- error) {
	defer close(resultsCh)

	// Track processed utterances by identity. SAUC utterance timestamps are not
	// guaranteed to be globally monotonic across incremental frames.
	seenUtterances := map[string]struct{}{}
	resultCount := 0
	textCount := 0
	lastText := ""
	lastInterimText := ""
	lastUtteranceCount := 0
	lastFinal := false
	transcriptOpen := false
	transcriptDefinite := false

	streamCtrl := func(begin, end bool, errText string) *genx.StreamCtrl {
		ctrl := &genx.StreamCtrl{
			Label:         "transcript",
			BeginOfStream: begin,
			EndOfStream:   end,
			Error:         errText,
		}
		if lastChunk != nil && lastChunk.Ctrl != nil {
			ctrl.StreamID = lastChunk.Ctrl.StreamID
		}
		return ctrl
	}
	emitTranscriptBOS := func() {
		if !t.emitInterim || transcriptOpen {
			return
		}
		outChunk := &genx.MessageChunk{
			Ctrl: streamCtrl(true, false, ""),
		}
		if lastChunk != nil {
			outChunk.Role = lastChunk.Role
			outChunk.Name = lastChunk.Name
		}
		resultsCh <- outChunk
		transcriptOpen = true
		transcriptDefinite = false
		lastInterimText = ""
	}
	emitTranscriptText := func(text string, definite bool) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if t.emitInterim {
			emitTranscriptBOS()
		}
		outChunk := &genx.MessageChunk{
			Part: genx.Text(text),
		}
		if t.emitInterim {
			outChunk.Ctrl = streamCtrl(false, false, "")
		}
		if lastChunk != nil {
			outChunk.Role = lastChunk.Role
			outChunk.Name = lastChunk.Name
		}
		resultsCh <- outChunk
		if definite {
			transcriptDefinite = true
		}
	}
	closeTranscript := func(errText string) {
		if !t.emitInterim || !transcriptOpen {
			return
		}
		if errText == "" && !transcriptDefinite {
			errText = "asr transcript ended before definite result"
		}
		outChunk := &genx.MessageChunk{
			Part: genx.Text(""),
			Ctrl: streamCtrl(false, true, errText),
		}
		if lastChunk != nil {
			outChunk.Role = lastChunk.Role
			outChunk.Name = lastChunk.Name
		}
		resultsCh <- outChunk
		transcriptOpen = false
		transcriptDefinite = false
		lastInterimText = ""
	}

	for result, err := range session.Recv() {
		if err != nil {
			closeTranscript(err.Error())
			done <- err
			return
		}
		resultCount++
		lastText = result.Text
		lastUtteranceCount = len(result.Utterances)
		lastFinal = result.IsFinal

		// Process definite utterances from the utterances array
		emittedResultText := false
		if len(result.Utterances) > 0 {
			for _, utt := range result.Utterances {
				if utt.Definite && utt.Text != "" {
					key := fmt.Sprintf("%d:%d:%s", utt.StartTime, utt.EndTime, utt.Text)
					if _, ok := seenUtterances[key]; ok {
						continue
					}
					seenUtterances[key] = struct{}{}
					emitTranscriptText(utt.Text, true)
					textCount++
					emittedResultText = true
					closeTranscript("")
				}
			}
		}
		if !emittedResultText && t.emitInterim && !result.IsFinal {
			interimText := strings.TrimSpace(result.Text)
			if interimText == "" {
				for _, utt := range result.Utterances {
					if !utt.Definite && strings.TrimSpace(utt.Text) != "" {
						interimText = strings.TrimSpace(utt.Text)
						break
					}
				}
			}
			if interimText != "" && interimText != lastInterimText {
				lastInterimText = interimText
				emitTranscriptText(interimText, false)
			}
		}
		if !emittedResultText && result.IsFinal && result.Text != "" && textCount == 0 {
			emitTranscriptText(result.Text, true)
			textCount++
			closeTranscript("")
		}
	}
	if textCount == 0 {
		err := fmt.Errorf("doubao asr returned no text: results=%d last_final=%t last_text=%q last_utterances=%d", resultCount, lastFinal, lastText, lastUtteranceCount)
		closeTranscript(err.Error())
		done <- err
		return
	}
	closeTranscript("")
	done <- nil
}

// isAudioMIME checks if a MIME type is audio
func isAudioMIME(mimeType string) bool {
	return strings.HasPrefix(baseAudioMIME(mimeType), "audio/")
}

func isOggAudioMIME(mimeType string) bool {
	mimeType = baseAudioMIME(mimeType)
	return mimeType == "audio/ogg" || mimeType == "application/ogg"
}

func isASRMP3MIME(mimeType string) bool {
	mimeType = baseAudioMIME(mimeType)
	return mimeType == "audio/mpeg" || mimeType == "audio/mp3" || mimeType == "audio/x-mpeg" || mimeType == "audio/x-mp3"
}

func isASRPCMMIME(mimeType string) bool {
	mimeType = baseAudioMIME(mimeType)
	return strings.HasPrefix(mimeType, "audio/l16") || mimeType == "audio/pcm" || mimeType == "audio/x-pcm"
}

func isASROpusMIME(mimeType string) bool {
	return baseAudioMIME(mimeType) == "audio/opus"
}

func baseAudioMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}
