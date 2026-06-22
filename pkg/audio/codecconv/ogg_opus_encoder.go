package codecconv

import (
	"encoding/binary"
	"fmt"
	"io"
	"iter"

	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
)

const defaultOggOpusSerial = 0x67636846

// PCMToOggOpusEncoder incrementally encodes PCM16LE bytes into an Ogg/Opus stream.
type PCMToOggOpusEncoder struct {
	encoder    *opus.Encoder
	ogg        *ogg.StreamWriter
	sampleRate int
	channels   int
	frameSize  int
	granule    uint64
	pending    []byte
	packet     []byte
	closed     bool
}

// NewPCMToOggOpusEncoder creates a streaming PCM16LE -> Ogg/Opus encoder.
func NewPCMToOggOpusEncoder(w io.Writer, sampleRate, channels int, app opus.Application) (*PCMToOggOpusEncoder, error) {
	encoder, err := opus.NewEncoder(sampleRate, channels, app)
	if err != nil {
		return nil, err
	}
	stream, err := ogg.NewStreamWriter(w, defaultOggOpusSerial)
	if err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if _, err := stream.WritePacket(OpusHeadPacket(sampleRate, channels), 0, false); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if _, err := stream.WritePacket(OpusTagsPacket("gizclaw"), 0, false); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	return &PCMToOggOpusEncoder{
		encoder:    encoder,
		ogg:        stream,
		sampleRate: sampleRate,
		channels:   channels,
		frameSize:  sampleRate / 50,
	}, nil
}

func (e *PCMToOggOpusEncoder) Write(data []byte) (int, error) {
	if e == nil || e.encoder == nil {
		return 0, fmt.Errorf("codecconv: pcm to ogg opus encoder is nil")
	}
	if e.closed {
		return 0, fmt.Errorf("codecconv: pcm to ogg opus encoder is closed")
	}
	if len(data) == 0 {
		return 0, nil
	}
	e.pending = append(e.pending, data...)
	frameBytes := e.frameSize * e.channels * 2
	for len(e.pending) >= frameBytes {
		if err := e.encodeFrame(e.pending[:frameBytes]); err != nil {
			return 0, err
		}
		e.pending = e.pending[frameBytes:]
	}
	return len(data), nil
}

func (e *PCMToOggOpusEncoder) Close() error {
	if e == nil {
		return nil
	}
	if e.closed {
		return nil
	}
	e.closed = true
	defer func() {
		if e.encoder != nil {
			_ = e.encoder.Close()
			e.encoder = nil
		}
	}()
	if len(e.pending) > 0 {
		frameBytes := e.frameSize * e.channels * 2
		frame := make([]byte, frameBytes)
		copy(frame, e.pending)
		if err := e.encodeFrame(frame); err != nil {
			return err
		}
		e.pending = nil
	}
	if len(e.packet) == 0 {
		return fmt.Errorf("codecconv: pcm to ogg opus encoder has no opus frames")
	}
	e.granule += uint64(e.frameSize)
	if _, err := e.ogg.WritePacket(e.packet, e.granule, true); err != nil {
		return err
	}
	e.packet = nil
	return nil
}

func (e *PCMToOggOpusEncoder) encodeFrame(data []byte) error {
	packet, err := e.encoder.Encode(bytesToInt16LE(data), e.frameSize)
	if err != nil {
		return err
	}
	if len(packet) == 0 {
		return nil
	}
	if len(e.packet) != 0 {
		e.granule += uint64(e.frameSize)
		if _, err := e.ogg.WritePacket(e.packet, e.granule, false); err != nil {
			return err
		}
	}
	e.packet = append(e.packet[:0], packet...)
	return nil
}

func bytesToInt16LE(data []byte) []int16 {
	out := make([]int16, len(data)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return out
}

// OpusPacketsToOgg writes Opus packets into an Ogg/Opus stream.
func OpusPacketsToOgg(w io.Writer, sampleRate, channels int, packets [][]byte) error {
	if err := opus.OpusSampleRate(sampleRate).Validate(); err != nil {
		return err
	}
	if channels != 1 && channels != 2 {
		return fmt.Errorf("codecconv: invalid opus channels %d", channels)
	}
	totalAudioPackets := nonEmptyPacketCount(packets)
	if totalAudioPackets == 0 {
		return fmt.Errorf("codecconv: no opus packets to write")
	}
	stream, err := ogg.NewStreamWriter(w, defaultOggOpusSerial)
	if err != nil {
		return err
	}
	if _, err := stream.WritePacket(OpusHeadPacket(sampleRate, channels), 0, false); err != nil {
		return err
	}
	if _, err := stream.WritePacket(OpusTagsPacket("gizclaw"), 0, false); err != nil {
		return err
	}
	frameSize := sampleRate / 50
	audioPackets := 0
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		audioPackets++
		eos := audioPackets == totalAudioPackets
		granule := uint64(audioPackets * frameSize)
		if _, err := stream.WritePacket(packet, granule, eos); err != nil {
			return err
		}
	}
	return nil
}

func nonEmptyPacketCount(packets [][]byte) int {
	count := 0
	for _, packet := range packets {
		if len(packet) != 0 {
			count++
		}
	}
	return count
}

// OggOpusPackets streams audio Opus packets from an Ogg/Opus stream.
func OggOpusPackets(r io.Reader) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		for packet, err := range ogg.Packets(r) {
			if err != nil {
				yield(nil, err)
				return
			}
			if IsOpusHeadPacket(packet.Data) || IsOpusTagsPacket(packet.Data) || len(packet.Data) == 0 {
				continue
			}
			if !yield(append([]byte(nil), packet.Data...), nil) {
				return
			}
		}
	}
}

// OpusHeadPacket builds a minimal OpusHead packet for an Ogg/Opus stream.
func OpusHeadPacket(sampleRate int, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet[:8], "OpusHead")
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:16], uint32(sampleRate))
	return packet
}

// OpusTagsPacket builds an OpusTags packet with a vendor string.
func OpusTagsPacket(vendor string) []byte {
	vendorBytes := []byte(vendor)
	packet := make([]byte, 8+4+len(vendorBytes)+4)
	copy(packet[:8], "OpusTags")
	binary.LittleEndian.PutUint32(packet[8:12], uint32(len(vendorBytes)))
	copy(packet[12:12+len(vendorBytes)], vendorBytes)
	return packet
}
