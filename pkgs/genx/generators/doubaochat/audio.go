package doubaochat

import (
	"bytes"
	"fmt"
	"mime"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// requestAudioFormat is the PCM format request audio is decoded to before it
// is wrapped as WAV.
const requestAudioFormat = pcm.L16Mono16K

// prepareModelContext replaces the audio Blobs of every user message with the
// single MP3 or WAV Blob the chat completions API accepts. It reports whether
// the latest user message carries audio.
func prepareModelContext(mctx genx.ModelContext) (genx.ModelContext, bool, error) {
	builder := copyModelContext(mctx)
	latestUser := -1
	for index, message := range builder.Messages {
		if message != nil && message.Role == genx.RoleUser {
			latestUser = index
		}
	}
	converted := false
	audioTurn := false
	for index, message := range builder.Messages {
		if message == nil || message.Role != genx.RoleUser {
			continue
		}
		contents, ok := message.Payload.(genx.Contents)
		if !ok {
			continue
		}
		var kept genx.Contents
		var audio []*genx.Blob
		for _, part := range contents {
			if blob, ok := part.(*genx.Blob); ok && blob != nil && isAudioMIME(blob.MIMEType) {
				audio = append(audio, blob)
				continue
			}
			kept = append(kept, part)
		}
		if len(audio) == 0 {
			continue
		}
		blob, err := requestAudioBlob(audio)
		if err != nil {
			return nil, false, err
		}
		copied := *message
		copied.Payload = append(kept, blob)
		builder.Messages[index] = &copied
		converted = true
		audioTurn = audioTurn || index == latestUser
	}
	if !converted {
		return mctx, false, nil
	}
	return builder.Build(), audioTurn, nil
}

func isAudioMIME(mimeType string) bool {
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		mediaType = mimeType
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return strings.HasPrefix(mediaType, "audio/") || mediaType == "application/ogg"
}

// requestAudioBlob converts one user message's audio Blobs into one MP3 or WAV
// Blob. Opus (raw packets or Ogg) and signed 16-bit PCM are decoded to 16 kHz
// mono WAV; one WAV Blob and MP3 Blobs pass through.
func requestAudioBlob(audio []*genx.Blob) (*genx.Blob, error) {
	var (
		mediaType string
		params    map[string]string
		payloads  [][]byte
	)
	for index, blob := range audio {
		partType, partParams, err := mime.ParseMediaType(blob.MIMEType)
		if err != nil {
			return nil, fmt.Errorf("doubaochat: user audio MIME type %q: %w", blob.MIMEType, err)
		}
		partType = strings.ToLower(partType)
		if index == 0 {
			mediaType, params = partType, partParams
		} else if partType != mediaType {
			return nil, fmt.Errorf("doubaochat: user audio mixes %s and %s", mediaType, partType)
		}
		payloads = append(payloads, blob.Data)
	}
	switch mediaType {
	case "audio/mpeg", "audio/mp3":
		return &genx.Blob{MIMEType: "audio/mpeg", Data: bytes.Join(payloads, nil)}, nil
	case "audio/wav", "audio/wave", "audio/x-wav":
		if len(payloads) != 1 {
			return nil, fmt.Errorf("doubaochat: user audio has %d WAV parts; want one", len(payloads))
		}
		return &genx.Blob{MIMEType: "audio/wav", Data: payloads[0]}, nil
	case "audio/pcm", "audio/l16", "audio/x-pcm":
		format, err := pcmFormat(params)
		if err != nil {
			return nil, err
		}
		return &genx.Blob{MIMEType: "audio/wav", Data: format.WAV(bytes.Join(payloads, nil))}, nil
	case "audio/ogg", "application/ogg", "audio/opus":
		samples, err := decodeOpus(payloads)
		if err != nil {
			return nil, err
		}
		return &genx.Blob{MIMEType: "audio/wav", Data: requestAudioFormat.WAV(samples)}, nil
	default:
		return nil, fmt.Errorf("doubaochat: user audio MIME type %q is unsupported", mediaType)
	}
}

func pcmFormat(params map[string]string) (pcm.Format, error) {
	rate, channels := requestAudioFormat.SampleRate(), requestAudioFormat.Channels()
	var err error
	if value := params["rate"]; value != "" {
		if rate, err = strconv.Atoi(value); err != nil {
			return 0, fmt.Errorf("doubaochat: user PCM rate %q: %w", value, err)
		}
	}
	if value := params["channels"]; value != "" {
		if channels, err = strconv.Atoi(value); err != nil {
			return 0, fmt.Errorf("doubaochat: user PCM channels %q: %w", value, err)
		}
	}
	return pcm.L16Format(rate, channels)
}

// decodeOpus decodes one message's Opus payloads to mono PCM. Each payload is
// either Ogg pages or one raw Opus packet; libopus downmixes stereo streams
// for the mono decoder.
func decodeOpus(payloads [][]byte) ([]byte, error) {
	decoder, err := opus.NewDecoder(requestAudioFormat.SampleRate(), requestAudioFormat.Channels())
	if err != nil {
		return nil, fmt.Errorf("doubaochat: create Opus decoder: %w", err)
	}
	defer decoder.Close()
	maxFrameSize := requestAudioFormat.SampleRate() * 3 / 50
	var out bytes.Buffer
	decode := func(packet []byte) error {
		if len(packet) == 0 || codecconv.IsOpusHeadPacket(packet) || codecconv.IsOpusTagsPacket(packet) {
			return nil
		}
		samples, err := decoder.Decode(packet, maxFrameSize, false)
		if err != nil {
			return fmt.Errorf("doubaochat: decode user Opus packet: %w", err)
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
				return nil, fmt.Errorf("doubaochat: read user Ogg audio: %w", err)
			}
			if err := decode(packet.Data); err != nil {
				return nil, err
			}
		}
	}
	if out.Len() == 0 {
		return nil, fmt.Errorf("doubaochat: user audio contains no Opus audio packets")
	}
	return out.Bytes(), nil
}
