package doubaoasr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Transformer is an ASR transformer using Doubao BigModel ASR (大模型语音识别).
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
type Transformer struct {
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

var _ genx.Transformer = (*Transformer)(nil)

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

// Config contains immutable Doubao SAUC configuration. Pointer booleans
// distinguish explicit false values from defaults.
type Config struct {
	Client            *doubaospeech.Client
	Format            string
	SampleRate        int
	Channels          int
	Bits              int
	Language          string
	EnableITN         *bool
	EnablePunctuation *bool
	Hotwords          []string
	ResultType        string
	EmitInterim       bool
	ResourceID        string
	ChunkSize         int
	RealtimePacing    *bool
}

// New creates a configured SAUC Transformer without opening a provider
// connection.
func New(config Config) (*Transformer, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("doubaoasr: client is required")
	}
	return newTransformer(config), nil
}

func newTransformer(config Config) *Transformer {
	t := &Transformer{
		client:         config.Client,
		format:         firstNonEmpty(config.Format, "pcm"),
		sampleRate:     firstPositive(config.SampleRate, 16000),
		channels:       firstPositive(config.Channels, 1),
		bits:           firstPositive(config.Bits, 16),
		language:       firstNonEmpty(config.Language, "zh-CN"),
		enableITN:      boolValue(config.EnableITN, true),
		enablePunc:     boolValue(config.EnablePunctuation, true),
		hotwords:       slices.Clone(config.Hotwords),
		resultType:     firstNonEmpty(config.ResultType, "single"),
		emitInterim:    config.EmitInterim,
		resourceID:     firstNonEmpty(config.ResourceID, doubaospeech.ResourceASRStream),
		chunkSize:      config.ChunkSize,
		realtimePacing: boolValue(config.RealtimePacing, true),
	}
	return t
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func firstPositive(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func firstNonEmpty(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

// Transform converts audio Blob chunks to Text chunks.
// Transformer creates sessions on demand, so it returns immediately.
// The ctx is the parent lifecycle for lazy session creation, provider sends,
// pacing timers, and result forwarding.
func (t *Transformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	output := newBufferStream(100)

	go t.transformLoop(ctx, input, output)

	return output, nil
}

func (t *Transformer) transformLoop(parentCtx context.Context, input genx.Stream, output *bufferStream) {
	defer output.Close()

	// Local cancel context tied to the loop lifecycle.
	// When the loop exits, defer cancel() cancels any in-flight WebSocket
	// dial or audio send operation.
	ctx, cancel := context.WithCancel(parentCtx)
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
	var historyAudio *doubaoASRHistoryAudioBuffer
	var activeStreamID string
	var sessionSourceChunk *genx.MessageChunk
	var rawOpusDecoder *opus.Decoder
	defer func() {
		if rawOpusDecoder != nil {
			_ = rawOpusDecoder.Close()
		}
	}()

	// Helper to start a new ASR session
	sourceChunkForStream := func(chunk *genx.MessageChunk, streamID string) *genx.MessageChunk {
		if chunk == nil {
			chunk = &genx.MessageChunk{}
		} else {
			chunk = chunk.Clone()
		}
		if chunk.Ctrl == nil {
			chunk.Ctrl = &genx.StreamCtrl{}
		}
		chunk.Ctrl.StreamID = streamID
		return chunk
	}
	resolveStreamID := func(chunk *genx.MessageChunk) string {
		streamID := chunkInputStreamID(chunk, activeStreamID)
		if strings.TrimSpace(streamID) == "" {
			streamID = "audio"
		}
		return streamID
	}

	startSession := func(cfg doubaoASRSessionConfig, sourceChunk *genx.MessageChunk) error {
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
		historyAudio = nil
		if t.emitInterim {
			historyAudio = newDoubaoASRHistoryAudioBuffer(cfg)
		}
		sessionSourceChunk = sourceChunk
		resultsCh = make(chan *genx.MessageChunk, 100)
		resultsDone = make(chan error, 1)
		resultsForwarded = make(chan struct{})
		go t.receiveResults(session, sessionSourceChunk, historyAudio, resultsCh, resultsDone)
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
		historyAudio = nil
		sessionSourceChunk = nil
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
			hadSession := session != nil
			sourceChunk := sessionSourceChunk
			if sourceChunk == nil {
				sourceChunk = lastChunk
			}
			if err := finishSession(); err != nil {
				output.CloseWithError(err)
				return
			}
			if hadSession && !t.emitInterim {
				if err := output.Push(transcriptTextChunk(sourceChunk, "", true)); err != nil {
					return
				}
			}
			return
		}

		if chunk == nil {
			continue
		}

		lastChunk = chunk
		if chunk.IsBeginOfStream() && chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.StreamID) != "" {
			activeStreamID = strings.TrimSpace(chunk.Ctrl.StreamID)
		}

		// Check for EoS marker with audio MIME type
		if chunk.IsEndOfStream() {
			if blob, ok := chunk.Part.(*genx.Blob); ok && isAudioMIME(blob.MIMEType) {
				historyStreamID := resolveStreamID(chunk)
				sourceChunk := sessionSourceChunk
				if sourceChunk == nil {
					sourceChunk = sourceChunkForStream(chunk, historyStreamID)
				}
				if !t.emitInterim {
					if err := output.Push(historyUserAudioEOSChunk(historyStreamID, blob.MIMEType)); err != nil {
						return
					}
				}
				// Audio EoS: finish current session, emit text EoS
				if err := finishSession(); err != nil {
					output.CloseWithError(err)
					return
				}
				if !t.emitInterim {
					eosChunk := transcriptTextChunk(sourceChunk, "", true)
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
			historyStreamID := resolveStreamID(chunk)
			if activeStreamID == "" {
				activeStreamID = historyStreamID
			}
			if len(blob.Data) > 0 && !t.emitInterim {
				if err := output.Push(historyUserAudioChunk(chunk, historyStreamID)); err != nil {
					return
				}
			}
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
				if err := startSession(cfg, sourceChunkForStream(chunk, historyStreamID)); err != nil {
					output.CloseWithError(err)
					return
				}
			}
			if len(audioData) > 0 {
				if historyAudio != nil {
					historyAudio.appendChunk(chunk, historyStreamID, audioData, cfg)
				}
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

func (t *Transformer) prepareAudioBlob(blob *genx.Blob, target *doubaoASRSessionConfig, rawOpusDecoder **opus.Decoder) ([]byte, doubaoASRSessionConfig, error) {
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
	if isASRWAVMIME(mimeType) {
		cfg = cfg.withWAV()
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

func (t *Transformer) defaultSessionConfig() doubaoASRSessionConfig {
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

func (c doubaoASRSessionConfig) withWAV() doubaoASRSessionConfig {
	c.format = "wav"
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

func (t *Transformer) openSession(ctx context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
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

func (t *Transformer) audioChunkSize(cfg doubaoASRSessionConfig) int {
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
	return 0
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

type doubaoASRHistoryAudioBuffer struct {
	mu      sync.Mutex
	cfg     doubaoASRSessionConfig
	pcm     []byte
	opus    timestampedHistoryAudioBuffer
	hasOpus bool
}

func newDoubaoASRHistoryAudioBuffer(cfg doubaoASRSessionConfig) *doubaoASRHistoryAudioBuffer {
	return &doubaoASRHistoryAudioBuffer{cfg: cfg.withPCM()}
}

func (b *doubaoASRHistoryAudioBuffer) appendChunk(chunk *genx.MessageChunk, streamID string, data []byte, cfg doubaoASRSessionConfig) {
	if b == nil || len(data) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if chunk != nil {
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob != nil && baseAudioMIME(blob.MIMEType) == "audio/opus" {
			b.opus.append(chunk, streamID)
			b.hasOpus = true
			return
		}
	}
	if cfg.isPCM() {
		b.pcm = append(b.pcm, data...)
	}
}

func (b *doubaoASRHistoryAudioBuffer) emitSegment(resultsCh chan<- *genx.MessageChunk, streamID string, startMS, endMS int) {
	if b == nil || resultsCh == nil {
		return
	}
	if b.hasOpus {
		chunks := b.opusSegment(streamID, startMS, endMS)
		if len(chunks) == 0 {
			return
		}
		for _, chunk := range chunks {
			resultsCh <- chunk
		}
		resultsCh <- historyUserAudioEOSChunk(streamID, "audio/opus")
		return
	}
	data, mimeType := b.segment(startMS, endMS)
	if len(data) == 0 {
		return
	}
	resultsCh <- &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: "transcript",
		Part: &genx.Blob{MIMEType: mimeType, Data: data},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: genx.HistoryUserAudioLabel},
	}
	resultsCh <- historyUserAudioEOSChunk(streamID, mimeType)
}

func (b *doubaoASRHistoryAudioBuffer) opusSegment(streamID string, startMS, endMS int) []*genx.MessageChunk {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	chunks := b.opus.segment(startMS, endMS)
	for _, chunk := range chunks {
		if chunk.Ctrl == nil {
			chunk.Ctrl = &genx.StreamCtrl{}
		}
		chunk.Ctrl.StreamID = streamID
	}
	return chunks
}

func (b *doubaoASRHistoryAudioBuffer) segment(startMS, endMS int) ([]byte, string) {
	if b == nil || endMS <= startMS {
		return nil, ""
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	bytesPerSecond := b.bytesPerSecond()
	frameBytes := b.frameBytes()
	if bytesPerSecond <= 0 || frameBytes <= 0 {
		return nil, ""
	}
	start := int((int64(max(startMS, 0)) * int64(bytesPerSecond)) / 1000)
	end := int((int64(endMS) * int64(bytesPerSecond)) / 1000)
	start = alignPCMOffset(start, frameBytes)
	end = alignPCMOffset(end, frameBytes)
	if start < 0 {
		start = 0
	}
	if end > len(b.pcm) {
		end = alignPCMOffset(len(b.pcm), frameBytes)
	}
	if start >= end {
		return nil, b.mimeType()
	}
	return append([]byte(nil), b.pcm[start:end]...), b.mimeType()
}

func (b *doubaoASRHistoryAudioBuffer) bytesPerSecond() int {
	if b == nil {
		return 0
	}
	return b.cfg.sampleRate * b.cfg.channels * b.bytesPerSample()
}

func (b *doubaoASRHistoryAudioBuffer) frameBytes() int {
	if b == nil {
		return 0
	}
	return b.cfg.channels * b.bytesPerSample()
}

func (b *doubaoASRHistoryAudioBuffer) bytesPerSample() int {
	if b == nil {
		return 0
	}
	bytesPerSample := b.cfg.bits / 8
	if bytesPerSample <= 0 {
		return 2
	}
	return bytesPerSample
}

func (b *doubaoASRHistoryAudioBuffer) mimeType() string {
	if b == nil {
		return "audio/pcm"
	}
	sampleRate := b.cfg.sampleRate
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	channels := b.cfg.channels
	if channels <= 0 {
		channels = 1
	}
	return fmt.Sprintf("audio/L16; rate=%d; channels=%d", sampleRate, channels)
}

func alignPCMOffset(offset, frameBytes int) int {
	if frameBytes <= 0 {
		return offset
	}
	return offset - offset%frameBytes
}

func transcriptTextChunk(chunk *genx.MessageChunk, text string, eos bool) *genx.MessageChunk {
	streamID := ""
	if chunk != nil && chunk.Ctrl != nil {
		streamID = strings.TrimSpace(chunk.Ctrl.StreamID)
	}
	streamID = asrSegmentStreamID(streamID, 1)
	return &genx.MessageChunk{
		Role: genx.RoleUser,
		Name: "transcript",
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript", EndOfStream: eos},
	}
}

func (t *Transformer) decodeMP3ToPCM(data []byte, cfg doubaoASRSessionConfig) ([]byte, error) {
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
			if len(data) > 0 {
				yield(data)
			}
			return
		}
		for offset := 0; offset < len(data); offset += chunkSize {
			end := min(offset+chunkSize, len(data))
			if !yield(data[offset:end]) {
				return
			}
		}
	}
}

func (t *Transformer) receiveResults(session doubaoASRSession, lastChunk *genx.MessageChunk, historyAudio *doubaoASRHistoryAudioBuffer, resultsCh chan<- *genx.MessageChunk, done chan<- error) {
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
	sawInterimText := false
	transcriptOpen := false
	transcriptDefinite := false
	transcriptSegment := 1
	baseStreamID := ""
	if lastChunk != nil && lastChunk.Ctrl != nil {
		baseStreamID = strings.TrimSpace(lastChunk.Ctrl.StreamID)
	}

	streamCtrl := func(begin, end bool, errText string) *genx.StreamCtrl {
		ctrl := &genx.StreamCtrl{
			Label:         "transcript",
			BeginOfStream: begin,
			EndOfStream:   end,
			Error:         errText,
		}
		ctrl.StreamID = asrSegmentStreamID(baseStreamID, transcriptSegment)
		return ctrl
	}
	emitTranscriptBOS := func() {
		if !t.emitInterim || transcriptOpen {
			return
		}
		outChunk := &genx.MessageChunk{
			Role: genx.RoleUser,
			Name: "transcript",
			Ctrl: streamCtrl(true, false, ""),
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
			Role: genx.RoleUser,
			Name: "transcript",
			Part: genx.Text(text),
			Ctrl: streamCtrl(false, false, ""),
		}
		resultsCh <- outChunk
		if definite {
			transcriptDefinite = true
		}
	}
	var closeTranscript func(string)
	emitDefiniteUtterance := func(text string, startTime, endTime int) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		streamID := asrSegmentStreamID(baseStreamID, transcriptSegment)
		if t.emitInterim && historyAudio != nil {
			historyAudio.emitSegment(resultsCh, streamID, startTime, endTime)
		}
		emitTranscriptText(text, true)
	}
	closeTranscript = func(errText string) {
		if !t.emitInterim || !transcriptOpen {
			return
		}
		if errText == "" && !transcriptDefinite {
			errText = "asr transcript ended before definite result"
		}
		outChunk := &genx.MessageChunk{
			Role: genx.RoleUser,
			Name: "transcript",
			Part: genx.Text(""),
			Ctrl: streamCtrl(false, true, errText),
		}
		resultsCh <- outChunk
		transcriptOpen = false
		transcriptDefinite = false
		lastInterimText = ""
		if errText == "" {
			transcriptSegment++
		}
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
				if !utt.Definite && strings.TrimSpace(utt.Text) != "" {
					sawInterimText = true
				}
				if utt.Definite && strings.TrimSpace(utt.Text) != "" {
					key := fmt.Sprintf("%d:%d:%s", utt.StartTime, utt.EndTime, utt.Text)
					if _, ok := seenUtterances[key]; ok {
						continue
					}
					seenUtterances[key] = struct{}{}
					emitDefiniteUtterance(utt.Text, utt.StartTime, utt.EndTime)
					textCount++
					emittedResultText = true
					closeTranscript("")
				}
			}
		}
		if !result.IsFinal && strings.TrimSpace(result.Text) != "" {
			sawInterimText = true
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
		if !emittedResultText && result.IsFinal && strings.TrimSpace(result.Text) != "" && textCount == 0 {
			emitTranscriptText(result.Text, true)
			textCount++
			closeTranscript("")
		}
	}
	if textCount == 0 {
		if !sawInterimText {
			done <- nil
			return
		}
		err := fmt.Errorf("doubao asr returned no text: results=%d last_final=%t last_text=%q last_utterances=%d", resultCount, lastFinal, lastText, lastUtteranceCount)
		closeTranscript(err.Error())
		done <- err
		return
	}
	closeTranscript("")
	done <- nil
}

func asrSegmentStreamID(base string, segment int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "audio"
	}
	if segment <= 1 {
		return base
	}
	return fmt.Sprintf("%s:asr:%d", base, segment)
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

func isASRWAVMIME(mimeType string) bool {
	mimeType = baseAudioMIME(mimeType)
	return mimeType == "audio/wav" || mimeType == "audio/wave" || mimeType == "audio/x-wav"
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
