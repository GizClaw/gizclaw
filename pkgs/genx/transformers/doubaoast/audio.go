package doubaoast

import (
	"bytes"
	"fmt"
	"io"
	"iter"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
)

type doubaoASRSessionConfig struct {
	format     string
	sampleRate int
	channels   int
	bits       int
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

func decodeMP3ToPCM(data []byte, cfg doubaoASRSessionConfig) ([]byte, error) {
	decoded, sampleRate, channels, err := mp3.DecodeFull(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode mp3 for doubao ast translate: %w", err)
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("decode mp3 for doubao ast translate: invalid sample rate %d", sampleRate)
	}
	if channels != 1 && channels != 2 {
		return nil, fmt.Errorf("decode mp3 for doubao ast translate: unsupported channels %d", channels)
	}
	if cfg.channels != 1 && cfg.channels != 2 {
		return nil, fmt.Errorf("doubao ast translate: unsupported target channels %d", cfg.channels)
	}
	srcFmt := resampler.Format{SampleRate: sampleRate, Stereo: channels == 2}
	dstFmt := resampler.Format{SampleRate: cfg.sampleRate, Stereo: cfg.channels == 2}
	if srcFmt == dstFmt {
		return decoded, nil
	}
	rs, err := resampler.New(bytes.NewReader(decoded), srcFmt, dstFmt)
	if err != nil {
		return nil, fmt.Errorf("create mp3 pcm resampler for doubao ast translate: %w", err)
	}
	defer rs.Close()
	pcm, err := io.ReadAll(rs)
	if err != nil {
		return nil, fmt.Errorf("resample mp3 pcm for doubao ast translate: %w", err)
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
		return nil, fmt.Errorf("doubao ast translate: unsupported raw opus target channels %d", cfg.channels)
	}
	if *decoder == nil {
		dec, err := opus.NewDecoder(cfg.sampleRate, cfg.channels)
		if err != nil {
			return nil, fmt.Errorf("create raw opus decoder for doubao ast translate: %w", err)
		}
		*decoder = dec
	}
	frameSize := (cfg.sampleRate * 3) / 50
	samples, err := (*decoder).Decode(data, frameSize, false)
	if err != nil {
		return nil, fmt.Errorf("decode raw opus for doubao ast translate: %w", err)
	}
	return pcm16LE(samples), nil
}

func pcm16LE(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		data[i*2] = byte(sample)
		data[i*2+1] = byte(uint16(sample) >> 8)
	}
	return data
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
			if !yield(data[offset:min(offset+chunkSize, len(data))]) {
				return
			}
		}
	}
}

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
