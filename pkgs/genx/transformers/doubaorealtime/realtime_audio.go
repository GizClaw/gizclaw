package doubaorealtime

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func realtimeBaseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func realtimeAudioFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "pcm"
	}
	return format
}

func realtimeAudioSampleRate(sampleRate int) int {
	if sampleRate <= 0 {
		return 16000
	}
	return sampleRate
}

func realtimeAudioChannels(channels int) int {
	if channels <= 0 {
		return 1
	}
	return channels
}

func realtimeStreamKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return "default"
	}
	return streamID
}

func realtimeAudioInputEOS(chunk *genx.MessageChunk) bool {
	if chunk == nil || !chunk.IsEndOfStream() {
		return false
	}
	if chunk.Part == nil {
		return true
	}
	mimeType, ok := chunk.MIMEType()
	if !ok {
		return false
	}
	mimeType = realtimeBaseMIME(mimeType)
	return strings.HasPrefix(mimeType, "audio/") || mimeType == "application/ogg"
}

func isRealtimeOpusMIME(mimeType string) bool {
	mimeType = realtimeBaseMIME(mimeType)
	return mimeType == "audio/opus" || strings.HasPrefix(mimeType, "audio/ogg")
}

func isRealtimePCMInputMIME(mimeType string) bool {
	mimeType = realtimeBaseMIME(mimeType)
	return strings.HasPrefix(mimeType, "audio/l16") || mimeType == "audio/pcm" || mimeType == "audio/x-pcm"
}

func isRealtimeMP3InputMIME(mimeType string) bool {
	mimeType = realtimeBaseMIME(mimeType)
	return mimeType == "audio/mpeg" || mimeType == "audio/mp3" || mimeType == "audio/x-mpeg" || mimeType == "audio/x-mp3"
}

func realtimePCM16LE(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
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
	return isRealtimeOpusMIME(mimeType)
}

func isDoubaoRealtimePCMInputMIME(mimeType string) bool {
	return isRealtimePCMInputMIME(mimeType)
}

func isDoubaoRealtimeMP3InputMIME(mimeType string) bool {
	return isRealtimeMP3InputMIME(mimeType)
}

func blobMIMEType(blob *genx.Blob) string {
	if blob == nil {
		return ""
	}
	return blob.MIMEType
}

func doubaoRealtimeBaseMIME(mimeType string) string {
	return realtimeBaseMIME(mimeType)
}

func doubaoRealtimeStreamKey(streamID string) string {
	return realtimeStreamKey(streamID)
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
	return realtimeAudioFormat(format)
}

func doubaoRealtimeAudioSampleRate(sampleRate int) int {
	return realtimeAudioSampleRate(sampleRate)
}

func doubaoRealtimeAudioChannels(channels int) int {
	return realtimeAudioChannels(channels)
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
	return realtimePCM16LE(samples)
}
