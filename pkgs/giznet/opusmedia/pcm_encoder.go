package opusmedia

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
)

// PCMEncoder converts a continuous PCM16 little-endian source into 20 ms Opus
// packets. Its owner serializes calls, including Close. It has no I/O or workers.
// A PCM source, including any concealed intervals, uses this one encoder for its
// whole epoch; encoded originals are never spliced into its predictive stream.
type PCMEncoder struct {
	encoder      *opus.Encoder
	pending      []byte
	samples      []int16
	encoded      []byte
	frameSamples int
	channels     int
	closed       bool
}

// NewPCMEncoder creates an encoder for the specified PCM sample rate/channels.
func NewPCMEncoder(sampleRate, channels int) (*PCMEncoder, error) {
	encoder, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationVoIP)
	if err != nil {
		return nil, err
	}
	return &PCMEncoder{
		encoder: encoder, frameSamples: sampleRate / 50, channels: channels,
		pending: make([]byte, 0, sampleRate/50*channels*2),
	}, nil
}

// Push accepts at most 128 KiB of whole PCM16 sample frames. Partial 20 ms codec
// frames are retained, bounded by one frame, until another Push or Flush.
func (e *PCMEncoder) Push(data []byte) ([][]byte, error) {
	if e == nil || e.closed {
		return nil, io.ErrClosedPipe
	}
	if len(data) > 128*1024 || len(data)%(2*e.channels) != 0 {
		return nil, errors.New("opusmedia: invalid PCM chunk size")
	}
	frameBytes := e.frameSamples * e.channels * 2
	var packets [][]byte
	for len(data) > 0 {
		n := min(len(data), frameBytes-len(e.pending))
		e.pending = append(e.pending, data[:n]...)
		data = data[n:]
		if len(e.pending) == frameBytes {
			encoded, err := e.encode()
			if err != nil {
				return packets, err
			}
			packets = append(packets, encoded)
		}
	}
	return packets, nil
}

// Flush pads only the last partial codec frame with silence. It adds at most
// 20 ms of codec padding and does not synthesize missing network packets.
func (e *PCMEncoder) Flush() ([][]byte, error) {
	if e == nil || e.closed {
		return nil, io.ErrClosedPipe
	}
	if len(e.pending) == 0 {
		return nil, nil
	}
	tail := len(e.pending)
	e.pending = e.pending[:e.frameSamples*e.channels*2]
	clear(e.pending[tail:])
	encoded, err := e.encode()
	if err != nil {
		return nil, err
	}
	return [][]byte{encoded}, nil
}

// Close releases the encoder and discards any unflushed trailing PCM.
func (e *PCMEncoder) Close() error {
	if e == nil || e.closed {
		return nil
	}
	e.closed = true
	e.pending, e.samples, e.encoded = nil, nil, nil
	return e.encoder.Close()
}

func (e *PCMEncoder) encode() ([]byte, error) {
	if e.samples == nil {
		e.samples = make([]int16, e.frameSamples*e.channels)
		e.encoded = make([]byte, opus.DefaultMaxPacketSize)
	}
	for i := range e.samples {
		e.samples[i] = int16(binary.LittleEndian.Uint16(e.pending[i*2:]))
	}
	n, err := e.encoder.EncodeInto(e.samples, e.frameSamples, e.encoded)
	if err != nil {
		return nil, err
	}
	e.pending = e.pending[:0]
	// The 4000-byte native output scratch is reused, while retained packets
	// own only their actual encoded bytes and cannot alias later encodes.
	return append([]byte(nil), e.encoded[:n]...), nil
}
