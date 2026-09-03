package agenthost

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type audioOutputKey struct {
	streamID string
	mimeType string
}

type audioOutputTracks struct {
	creator        AudioTrackCreator
	channels       map[audioOutputKey]*audioOutputChannel
	pending        []audioOutputPending
	labels         map[audioOutputKey]string
	cutoverPending bool
}

type audioOutputPending struct {
	key   audioOutputKey
	label string
	ctrl  *pcm.TrackCtrl
}

type audioOutputChannel struct {
	track   pcm.Track
	ctrl    *pcm.TrackCtrl
	decoder audioPCMDecoder
	label   string
}

type audioPCMDecoder interface {
	Decode([]byte) ([]pcm.Chunk, error)
	Close() error
}

type audioPCMFinalizer interface {
	Finalize() ([]pcm.Chunk, error)
}

type audioPCMAborter interface {
	Abort() error
}

func newAudioOutputTracks(creator AudioTrackCreator) *audioOutputTracks {
	return &audioOutputTracks{
		creator:  creator,
		channels: make(map[audioOutputKey]*audioOutputChannel),
		labels:   make(map[audioOutputKey]string),
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
	label := ""
	if chunk.Ctrl != nil {
		label = strings.TrimSpace(chunk.Ctrl.Label)
	}
	if chunk.IsBeginOfStream() && label != "" {
		if err := o.cutover(streamID, label); err != nil {
			return err
		}
		o.labels[key] = label
	}
	if len(blob.Data) > 0 {
		channel, err := o.channel(key, label)
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
		err := o.closeChannel(key, errorText)
		delete(o.labels, key)
		return err
	}
	return nil
}

func (o *audioOutputTracks) channel(key audioOutputKey, label string) (*audioOutputChannel, error) {
	if label == "" {
		label = o.labels[key]
	}
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
	channel := &audioOutputChannel{track: track, ctrl: ctrl, decoder: decoder, label: label}
	o.channels[key] = channel
	return channel, nil
}

// cutover closes every other route that shares the new stream's label: a BOS
// with the same label is a barge-in replacement, not a concurrent speaker.
// Concurrent routes are keyed by (stream_id, mime_type) and mix freely, so a
// producer that needs several simultaneous speakers (the SFU driver mapping
// remote participants to routes) must give each participant a distinct
// stream_id AND a distinct label, or its participants will cut each other off.
func (o *audioOutputTracks) cutover(streamID, label string) error {
	var errs error
	cutover := false
	errs = errors.Join(errs, o.closePending(func(pending audioOutputPending) bool {
		matched := pending.key.streamID != streamID && pending.label == label
		cutover = cutover || matched
		return matched
	}, "interrupted"))
	for key, channel := range o.channels {
		if key.streamID != streamID && channel.label == label {
			cutover = true
			errs = errors.Join(errs, o.closeChannelWithPending(key, "interrupted", true))
		}
	}
	o.cutoverPending = o.cutoverPending || cutover
	return errs
}

func (o *audioOutputTracks) takeCutoverPending() bool {
	pending := o.cutoverPending
	o.cutoverPending = false
	return pending
}

func (o *audioOutputTracks) closeRoute(streamID, errorText string) error {
	var errs error
	if errorText != "" {
		errs = errors.Join(errs, o.closePending(func(pending audioOutputPending) bool {
			return pending.key.streamID == streamID
		}, errorText))
	}
	for key := range o.channels {
		if key.streamID == streamID {
			errs = errors.Join(errs, o.closeChannelWithPending(key, errorText, errorText == "interrupted"))
		}
	}
	for key := range o.labels {
		if key.streamID == streamID {
			delete(o.labels, key)
		}
	}
	return errs
}

func (o *audioOutputTracks) closeChannel(key audioOutputKey, errorText string) error {
	return o.closeChannelWithPending(key, errorText, errorText == "interrupted")
}

func (o *audioOutputTracks) closeChannelWithPending(key audioOutputKey, errorText string, retainPending bool) error {
	channel := o.channels[key]
	if channel == nil {
		if errorText != "" {
			return o.closePending(func(pending audioOutputPending) bool {
				return pending.key == key
			}, errorText)
		}
		return nil
	}
	delete(o.channels, key)
	if errorText == "" {
		if finalizer, ok := channel.decoder.(audioPCMFinalizer); ok {
			chunks, err := finalizer.Finalize()
			if err != nil {
				decoderErr := fmt.Errorf("agenthost: finalize audio decoder stream_id=%q mime=%q: %w", key.streamID, key.mimeType, err)
				return errors.Join(decoderErr, channel.decoder.Close(), channel.ctrl.CloseWithError(decoderErr))
			}
			for _, chunk := range chunks {
				if err := channel.track.Write(chunk); err != nil {
					writeErr := fmt.Errorf("agenthost: write final audio stream_id=%q mime=%q: %w", key.streamID, key.mimeType, err)
					return errors.Join(writeErr, channel.decoder.Close(), channel.ctrl.CloseWithError(writeErr))
				}
			}
		}
	}
	var decoderErr error
	if errorText != "" {
		decoderErr = abortAudioPCMDecoder(channel.decoder)
	} else {
		decoderErr = channel.decoder.Close()
	}
	if decoderErr != nil {
		decoderErr = fmt.Errorf("agenthost: close audio decoder stream_id=%q mime=%q: %w", key.streamID, key.mimeType, decoderErr)
	}
	if errorText != "" {
		closeErr := fmt.Errorf("agenthost: audio stream_id=%q mime=%q: %s", key.streamID, key.mimeType, errorText)
		pendingErr := o.closePending(func(pending audioOutputPending) bool {
			return pending.key == key
		}, errorText)
		ctrlErr := channel.ctrl.CloseWithError(closeErr)
		if retainPending {
			o.pending = append(o.pending, audioOutputPending{key: key, label: channel.label, ctrl: channel.ctrl})
		}
		return errors.Join(decoderErr, pendingErr, ctrlErr)
	}
	if decoderErr != nil {
		return errors.Join(decoderErr, channel.ctrl.CloseWithError(decoderErr))
	}
	if err := channel.ctrl.CloseWrite(); err != nil {
		return err
	}
	o.pending = append(o.pending, audioOutputPending{key: key, label: channel.label, ctrl: channel.ctrl})
	return nil
}

func abortAudioPCMDecoder(decoder audioPCMDecoder) error {
	if aborter, ok := decoder.(audioPCMAborter); ok {
		return aborter.Abort()
	}
	return decoder.Close()
}

func (o *audioOutputTracks) closePending(match func(audioOutputPending) bool, errorText string) error {
	var errs error
	kept := o.pending[:0]
	for _, pending := range o.pending {
		if !match(pending) {
			kept = append(kept, pending)
			continue
		}
		closeErr := fmt.Errorf("agenthost: audio stream_id=%q mime=%q: %s", pending.key.streamID, pending.key.mimeType, errorText)
		errs = errors.Join(errs, pending.ctrl.CloseWithError(closeErr))
		kept = append(kept, pending)
	}
	o.pending = kept
	return errs
}

func (o *audioOutputTracks) nextPendingDone() <-chan struct{} {
	if len(o.pending) == 0 {
		return nil
	}
	return o.pending[0].ctrl.Done()
}

func (o *audioOutputTracks) removeDrainedPending() {
	for len(o.pending) > 0 {
		select {
		case <-o.pending[0].ctrl.Done():
			o.pending = o.pending[1:]
		default:
			return
		}
	}
}

func (o *audioOutputTracks) hasPending() bool {
	return len(o.pending) > 0
}

func (o *audioOutputTracks) waitPending(ctx context.Context) error {
	for len(o.pending) > 0 {
		pending := o.pending[0]
		select {
		case <-pending.ctrl.Done():
			o.pending = o.pending[1:]
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (o *audioOutputTracks) waitInterrupted(ctx context.Context, chunk *genx.MessageChunk) (bool, error) {
	if chunk == nil || chunk.Ctrl == nil || !chunk.IsEndOfStream() || chunk.Ctrl.Error != "interrupted" {
		return false, nil
	}
	streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
	if streamID == "" {
		return false, nil
	}
	mimeType, typed := chunk.MIMEType()
	matched := false
	for _, pending := range o.pending {
		match := pending.key.streamID == streamID && (!typed || pending.key.mimeType == mimeType)
		if !match {
			continue
		}
		matched = true
		select {
		case <-pending.ctrl.Done():
		case <-ctx.Done():
			return matched, ctx.Err()
		}
	}
	kept := o.pending[:0]
	for _, pending := range o.pending {
		if pending.key.streamID == streamID && (!typed || pending.key.mimeType == mimeType) {
			continue
		}
		kept = append(kept, pending)
	}
	o.pending = kept
	return matched, nil
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
		errs = errors.Join(errs, abortAudioPCMDecoder(channel.decoder), channel.ctrl.CloseWithError(err))
	}
	for _, pending := range o.pending {
		errs = errors.Join(errs, pending.ctrl.CloseWithError(err))
	}
	o.pending = o.pending[:0]
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
	case "audio/mpeg", "audio/mp3", "audio/x-mpeg", "audio/x-mp3":
		return &mp3PCMDecoder{}, nil
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

type mp3PCMDecoder struct {
	data []byte
}

func (d *mp3PCMDecoder) Decode(data []byte) ([]pcm.Chunk, error) {
	if d == nil {
		return nil, fmt.Errorf("MP3 decoder is closed")
	}
	d.data = append(d.data, data...)
	return nil, nil
}

func (d *mp3PCMDecoder) Finalize() ([]pcm.Chunk, error) {
	if d == nil || len(d.data) == 0 {
		return nil, fmt.Errorf("empty MP3 stream")
	}
	decoded, sampleRate, channels, err := mp3.DecodeFull(bytes.NewReader(d.data))
	if err != nil {
		return nil, err
	}
	source := resampler.Format{SampleRate: sampleRate, Stereo: channels == 2}
	target := resampler.Format{SampleRate: 48000, Stereo: false}
	if source != target {
		converter, err := resampler.New(bytes.NewReader(decoded), source, target)
		if err != nil {
			return nil, err
		}
		decoded, err = io.ReadAll(converter)
		closeErr := converter.Close()
		if err != nil || closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
	}
	return []pcm.Chunk{pcm.L16Mono48K.DataChunk(decoded)}, nil
}

func (d *mp3PCMDecoder) Close() error {
	if d != nil {
		d.data = nil
	}
	return nil
}

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
	maxFrameSize := d.format.SampleRate() * 3 / 25
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
			if d.opus != nil {
				if err := d.opus.Close(); err != nil {
					return nil, err
				}
				d.opus = nil
			}
			d.started = false
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

func (d *oggOpusPCMDecoder) Abort() error {
	if d == nil {
		return nil
	}
	d.packets = ogg.PacketDecoder{}
	d.started = false
	if d.opus == nil {
		return nil
	}
	err := d.opus.Close()
	d.opus = nil
	return err
}
