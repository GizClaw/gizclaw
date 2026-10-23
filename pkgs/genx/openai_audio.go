package genx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
)

// requestAudioFormat is the PCM format request audio is decoded to before it
// is wrapped as WAV.
const requestAudioFormat = pcm.L16Mono16K

// requestAudioBlob converts one user message's audio Blobs into one MP3 or WAV
// Blob. Opus (raw packets or Ogg) and signed 16-bit PCM are converted to
// 16 kHz mono WAV; fragments of one WAV file and MP3 Blobs are reassembled.
func requestAudioBlob(audio []*Blob) (*Blob, error) {
	var (
		mediaType string
		params    map[string]string
		canonical string
		payloads  [][]byte
	)
	for index, blob := range audio {
		partMIME, err := requestAudioMIME(blob)
		if err != nil {
			return nil, err
		}
		partType, partParams, err := mime.ParseMediaType(blob.MIMEType)
		if err != nil {
			return nil, fmt.Errorf("openai: user audio MIME type %q: %w", blob.MIMEType, err)
		}
		partType = strings.ToLower(partType)
		if index == 0 {
			mediaType, params = partType, partParams
			canonical = partMIME
		} else if partMIME != canonical {
			return nil, fmt.Errorf("openai: user audio mixes %s and %s", canonical, partMIME)
		}
		payloads = append(payloads, blob.Data)
	}
	switch mediaType {
	case "audio/mpeg", "audio/mp3":
		return &Blob{MIMEType: "audio/mpeg", Data: bytes.Join(payloads, nil)}, nil
	case "audio/wav", "audio/wave", "audio/x-wav":
		data := bytes.Join(payloads, nil)
		if len(payloads) > 1 && (len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data))) {
			return nil, fmt.Errorf("openai: user audio WAV parts must contain one complete file")
		}
		return &Blob{MIMEType: "audio/wav", Data: data}, nil
	case "audio/pcm", "audio/l16", "audio/x-pcm":
		format, err := pcmFormat(params)
		if err != nil {
			return nil, err
		}
		samples, err := resamplePCM(bytes.Join(payloads, nil), format)
		if err != nil {
			return nil, err
		}
		return &Blob{MIMEType: "audio/wav", Data: requestAudioFormat.WAV(samples)}, nil
	case "audio/ogg", "application/ogg", "audio/opus":
		samples, err := decodeOpus(payloads)
		if err != nil {
			return nil, err
		}
		return &Blob{MIMEType: "audio/wav", Data: requestAudioFormat.WAV(samples)}, nil
	default:
		return nil, fmt.Errorf("openai: user audio MIME type %q is unsupported", mediaType)
	}
}

func requestAudioMIME(blob *Blob) (string, error) {
	if blob == nil {
		return "", fmt.Errorf("openai: user audio contains a nil Blob")
	}
	mediaType, params, err := mime.ParseMediaType(blob.MIMEType)
	if err != nil {
		return "", fmt.Errorf("openai: user audio MIME type %q: %w", blob.MIMEType, err)
	}
	return mime.FormatMediaType(mediaType, params), nil
}

func pcmFormat(params map[string]string) (pcm.Format, error) {
	rate, channels := requestAudioFormat.SampleRate(), requestAudioFormat.Channels()
	var err error
	if value := params["rate"]; value != "" {
		if rate, err = strconv.Atoi(value); err != nil {
			return 0, fmt.Errorf("openai: user PCM rate %q: %w", value, err)
		}
	}
	if value := params["channels"]; value != "" {
		if channels, err = strconv.Atoi(value); err != nil {
			return 0, fmt.Errorf("openai: user PCM channels %q: %w", value, err)
		}
	}
	return pcm.L16Format(rate, channels)
}

// resamplePCM converts signed 16-bit PCM in format to requestAudioFormat.
func resamplePCM(data []byte, format pcm.Format) ([]byte, error) {
	if format == requestAudioFormat {
		return data, nil
	}
	converter, err := resampler.New(bytes.NewReader(data),
		resampler.Format{SampleRate: format.SampleRate(), Stereo: format.Channels() == 2},
		resampler.Format{SampleRate: requestAudioFormat.SampleRate(), Stereo: requestAudioFormat.Channels() == 2})
	if err != nil {
		return nil, fmt.Errorf("openai: resample user PCM: %w", err)
	}
	defer converter.Close()
	out, err := io.ReadAll(converter)
	if err != nil {
		return nil, fmt.Errorf("openai: resample user PCM: %w", err)
	}
	return out, nil
}

// decodeOpus decodes one message's Opus payloads to mono PCM. Each payload is
// either Ogg pages or one raw Opus packet; libopus downmixes stereo streams
// for the mono decoder.
func decodeOpus(payloads [][]byte) ([]byte, error) {
	decoder, err := opus.NewDecoder(requestAudioFormat.SampleRate(), requestAudioFormat.Channels())
	if err != nil {
		return nil, fmt.Errorf("openai: create Opus decoder: %w", err)
	}
	defer decoder.Close()
	// Opus packets carry at most 120 ms of audio.
	maxFrameSize := requestAudioFormat.SampleRate() * 120 / 1000
	var out bytes.Buffer
	decode := func(packet []byte) error {
		if len(packet) == 0 || codecconv.IsOpusHeadPacket(packet) || codecconv.IsOpusTagsPacket(packet) {
			return nil
		}
		samples, err := decoder.Decode(packet, maxFrameSize, false)
		if err != nil {
			return fmt.Errorf("openai: decode user Opus packet: %w", err)
		}
		for _, sample := range samples {
			out.WriteByte(byte(sample))
			out.WriteByte(byte(uint16(sample) >> 8))
		}
		return nil
	}
	var oggStream bytes.Buffer
	for _, payload := range payloads {
		if bytes.HasPrefix(payload, []byte("OggS")) || oggStream.Len() != 0 {
			oggStream.Write(payload)
			continue
		}
		if err := decode(payload); err != nil {
			return nil, err
		}
	}
	if oggStream.Len() != 0 {
		for packet, err := range ogg.Packets(&oggStream) {
			if err != nil {
				return nil, fmt.Errorf("openai: read user Ogg audio: %w", err)
			}
			if err := decode(packet.Data); err != nil {
				return nil, err
			}
		}
	}
	if out.Len() == 0 {
		return nil, fmt.Errorf("openai: user audio contains no Opus audio packets")
	}
	return out.Bytes(), nil
}
