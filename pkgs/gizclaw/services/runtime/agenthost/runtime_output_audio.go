package agenthost

import (
	"encoding/binary"
	"errors"
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

type audioOutputKey struct {
	streamID string
	mimeType string
}

type audioOutputTracks struct {
	creator  AudioTrackCreator
	channels map[audioOutputKey]*audioOutputChannel
}

type audioOutputChannel struct {
	track   pcm.Track
	ctrl    *pcm.TrackCtrl
	decoder audioPCMDecoder
}

type audioPCMDecoder interface {
	Decode([]byte) ([]pcm.Chunk, error)
	Close() error
}

func newAudioOutputTracks(creator AudioTrackCreator) *audioOutputTracks {
	return &audioOutputTracks{
		creator:  creator,
		channels: make(map[audioOutputKey]*audioOutputChannel),
	}
}

func (o *audioOutputTracks) consume(chunk *genx.MessageChunk) error {
	if chunk == nil {
		return nil
	}
	if chunk.Part == nil {
		if chunk.IsEndOfStream() && chunk.Ctrl != nil {
			return o.closeRoute(chunk.Ctrl.StreamID, chunk.Ctrl.Error)
		}
		return nil
	}

	streamID := ""
	errorText := ""
	if chunk.Ctrl != nil {
		streamID = chunk.Ctrl.StreamID
		errorText = chunk.Ctrl.Error
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok {
		return nil
	}
	mimeType, validMIME := chunk.MIMEType()
	if !validMIME {
		if looksLikeMixerAudioMIME(blob.MIMEType) {
			return fmt.Errorf("agenthost: invalid audio MIME stream_id=%q mime=%q", streamID, blob.MIMEType)
		}
		return nil
	}
	if !isMixerAudioMIME(mimeType) {
		return nil
	}
	key := audioOutputKey{streamID: streamID, mimeType: mimeType}
	if len(blob.Data) > 0 {
		channel, err := o.channel(key)
		if err != nil {
			return err
		}
		chunks, err := channel.decoder.Decode(blob.Data)
		if err != nil {
			_ = o.closeChannel(key, err.Error())
			return fmt.Errorf("agenthost: decode audio stream_id=%q mime=%q: %w", streamID, mimeType, err)
		}
		for _, pcmChunk := range chunks {
			if err := channel.track.Write(pcmChunk); err != nil {
				_ = o.closeChannel(key, err.Error())
				return fmt.Errorf("agenthost: write audio stream_id=%q mime=%q: %w", streamID, mimeType, err)
			}
		}
	}
	if chunk.IsEndOfStream() {
		return o.closeChannel(key, errorText)
	}
	return nil
}

func (o *audioOutputTracks) channel(key audioOutputKey) (*audioOutputChannel, error) {
	if channel := o.channels[key]; channel != nil {
		return channel, nil
	}
	if o.creator == nil {
		return nil, fmt.Errorf("agenthost: audio track creator is required")
	}
	decoder, err := newAudioPCMDecoder(key.mimeType)
	if err != nil {
		return nil, fmt.Errorf("agenthost: create audio decoder stream_id=%q mime=%q: %w", key.streamID, key.mimeType, err)
	}
	track, ctrl, err := o.creator.CreateAudioTrack(pcm.WithTrackLabel("agent"))
	if err != nil {
		_ = decoder.Close()
		return nil, fmt.Errorf("agenthost: create audio track stream_id=%q mime=%q: %w", key.streamID, key.mimeType, err)
	}
	if track == nil || ctrl == nil {
		_ = decoder.Close()
		if ctrl != nil {
			_ = ctrl.Close()
		}
		return nil, fmt.Errorf("agenthost: create audio track stream_id=%q mime=%q returned nil track or control", key.streamID, key.mimeType)
	}
	channel := &audioOutputChannel{track: track, ctrl: ctrl, decoder: decoder}
	o.channels[key] = channel
	return channel, nil
}

func (o *audioOutputTracks) closeRoute(streamID, errorText string) error {
	var errs error
	for key := range o.channels {
		if key.streamID == streamID {
			errs = errors.Join(errs, o.closeChannel(key, errorText))
		}
	}
	return errs
}

func (o *audioOutputTracks) closeChannel(key audioOutputKey, errorText string) error {
	channel := o.channels[key]
	if channel == nil {
		return nil
	}
	delete(o.channels, key)
	decoderErr := channel.decoder.Close()
	if decoderErr != nil {
		decoderErr = fmt.Errorf("agenthost: close audio decoder stream_id=%q mime=%q: %w", key.streamID, key.mimeType, decoderErr)
	}
	if errorText != "" {
		closeErr := fmt.Errorf("agenthost: audio stream_id=%q mime=%q: %s", key.streamID, key.mimeType, errorText)
		return errors.Join(decoderErr, channel.ctrl.CloseWithError(closeErr))
	}
	if decoderErr != nil {
		return errors.Join(decoderErr, channel.ctrl.CloseWithError(decoderErr))
	}
	return channel.ctrl.CloseWrite()
}

func (o *audioOutputTracks) closeWrite() error {
	var errs error
	for key := range o.channels {
		errs = errors.Join(errs, o.closeChannel(key, ""))
	}
	return errs
}

func (o *audioOutputTracks) closeWithError(err error) error {
	if err == nil {
		err = errors.New("agenthost: audio output closed")
	}
	var errs error
	for key, channel := range o.channels {
		delete(o.channels, key)
		errs = errors.Join(errs, channel.decoder.Close(), channel.ctrl.CloseWithError(err))
	}
	return errs
}

func isMixerAudioMIME(mimeType string) bool {
	base, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return false
	}
	base = strings.ToLower(base)
	return strings.HasPrefix(base, "audio/") || base == "application/ogg"
}

func looksLikeMixerAudioMIME(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return strings.HasPrefix(mimeType, "audio/") || strings.HasPrefix(mimeType, "application/ogg")
}

func newAudioPCMDecoder(mimeType string) (audioPCMDecoder, error) {
	base, params, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return nil, err
	}
	base = strings.ToLower(base)
	switch base {
	case "audio/opus":
		format, err := audioPCMFormat(params, 48000, 1)
		if err != nil {
			return nil, err
		}
		return newRawOpusPCMDecoder(format)
	case "audio/ogg", "application/ogg":
		return &oggOpusPCMDecoder{}, nil
	case "audio/l16", "audio/pcm", "audio/x-pcm":
		format, err := audioPCMFormat(params, 16000, 1)
		if err != nil {
			return nil, err
		}
		return pcmBlobDecoder{format: format}, nil
	default:
		return nil, fmt.Errorf("unsupported audio MIME %q", mimeType)
	}
}

func audioPCMFormat(params map[string]string, defaultRate, defaultChannels int) (pcm.Format, error) {
	rate, err := audioMIMEInt(params, "rate", defaultRate)
	if err != nil {
		return 0, err
	}
	channels, err := audioMIMEInt(params, "channels", defaultChannels)
	if err != nil {
		return 0, err
	}
	return pcm.L16Format(rate, channels)
}

func audioMIMEInt(params map[string]string, key string, fallback int) (int, error) {
	value := strings.TrimSpace(params[key])
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid %s parameter %q", key, value)
	}
	return parsed, nil
}

type pcmBlobDecoder struct {
	format pcm.Format
}

func (d pcmBlobDecoder) Decode(data []byte) ([]pcm.Chunk, error) {
	frameBytes := d.format.Channels() * 2
	if len(data)%frameBytes != 0 {
		return nil, fmt.Errorf("PCM byte length %d is not aligned to %d-byte samples", len(data), frameBytes)
	}
	return []pcm.Chunk{d.format.DataChunk(data)}, nil
}

func (pcmBlobDecoder) Close() error { return nil }

type rawOpusPCMDecoder struct {
	format  pcm.Format
	decoder *opus.Decoder
}

func newRawOpusPCMDecoder(format pcm.Format) (*rawOpusPCMDecoder, error) {
	decoder, err := opus.NewDecoder(format.SampleRate(), format.Channels())
	if err != nil {
		return nil, err
	}
	return &rawOpusPCMDecoder{format: format, decoder: decoder}, nil
}

func (d *rawOpusPCMDecoder) Decode(data []byte) ([]pcm.Chunk, error) {
	if d == nil || d.decoder == nil {
		return nil, fmt.Errorf("Opus decoder is closed")
	}
	maxFrameSize := d.format.SampleRate() * 3 / 50
	samples, err := d.decoder.Decode(data, maxFrameSize, false)
	if err != nil {
		return nil, err
	}
	pcmData := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(pcmData[i*2:], uint16(sample))
	}
	return []pcm.Chunk{d.format.DataChunk(pcmData)}, nil
}

func (d *rawOpusPCMDecoder) Close() error {
	if d == nil || d.decoder == nil {
		return nil
	}
	err := d.decoder.Close()
	d.decoder = nil
	return err
}

type oggOpusPCMDecoder struct {
	packets ogg.PacketDecoder
	opus    *rawOpusPCMDecoder
	started bool
}

func (d *oggOpusPCMDecoder) Decode(data []byte) ([]pcm.Chunk, error) {
	packets, err := d.packets.Write(data)
	if err != nil {
		return nil, err
	}
	var chunks []pcm.Chunk
	for _, packet := range packets {
		switch {
		case codecconv.IsOpusHeadPacket(packet.Data):
			if d.started || d.opus != nil {
				return nil, fmt.Errorf("unexpected OpusHead after audio started")
			}
			_, channels, err := codecconv.ParseOpusHeadPacket(packet.Data)
			if err != nil {
				return nil, err
			}
			format, err := pcm.L16Format(48000, channels)
			if err != nil {
				return nil, err
			}
			d.opus, err = newRawOpusPCMDecoder(format)
			if err != nil {
				return nil, err
			}
		case codecconv.IsOpusTagsPacket(packet.Data), len(packet.Data) == 0:
			continue
		default:
			if d.opus == nil {
				d.opus, err = newRawOpusPCMDecoder(pcm.L16Mono48K)
				if err != nil {
					return nil, err
				}
			}
			d.started = true
			decoded, err := d.opus.Decode(packet.Data)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, decoded...)
		}
	}
	return chunks, nil
}

func (d *oggOpusPCMDecoder) Close() error {
	if d == nil {
		return nil
	}
	var opusErr error
	if d.opus != nil {
		opusErr = d.opus.Close()
		d.opus = nil
	}
	return errors.Join(d.packets.Close(), opusErr)
}
