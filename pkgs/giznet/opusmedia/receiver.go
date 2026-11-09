// Package opusmedia receives encoded Opus media and produces ordered PCM.
// RTP reordering and packet-loss concealment stay within this media boundary.
package opusmedia

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/rtpstream"
)

// Config controls one receiver's jitter bounds and signed PCM16 output format.
// SampleRate and Channels default to 16000 Hz and mono when omitted.
type Config struct {
	Jitter     rtpstream.Config
	SampleRate int
	Channels   int
}

// Frame is ordered PCM with optional process-local provenance. Original holds
// only a real received encoded packet. Concealed frames never invent encoded
// bytes. Info retains the original RTP clock independently of scheduling time.
type Frame struct {
	PCM       []byte
	Original  []byte
	Concealed bool
	Info      *packet.OpusInfo
}

// Receiver owns a jitter buffer and one continuous Opus decoder. Its owner
// serializes all calls and drives Deadline/Pop; it starts no goroutines or I/O.
type Receiver struct {
	buffer          *rtpstream.Buffer
	decoder         *codecconv.OpusPacketDecoder
	gap             *packet.OpusInfo
	closed          bool
	maxBytes        int
	invalidPackets  uint64
	lastDecodeError error
}

// NewReceiver validates the PCM format and creates an idle, codec-free receiver.
// Native decoder startup is deferred until a packet or valid gap needs decoding.
func NewReceiver(config Config) (*Receiver, error) {
	if config.SampleRate == 0 {
		config.SampleRate = 16000
	}
	if config.Channels == 0 {
		config.Channels = 1
	}
	// The decoder bounds consecutive PLC to 500 ms. Larger timestamp jumps
	// must be discontinuities instead of requests for unbounded synthesis.
	if config.Jitter.MaxGap48 > packet.OpusClockRate/2 {
		config.Jitter.MaxGap48 = packet.OpusClockRate / 2
	}
	decoder, err := codecconv.NewOpusPacketDecoder(config.SampleRate, config.Channels)
	if err != nil {
		return nil, fmt.Errorf("opusmedia: create decoder: %w", err)
	}
	maxBytes := config.Jitter.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 128 * 1024
	}
	return &Receiver{buffer: rtpstream.New(config.Jitter), decoder: decoder, maxBytes: maxBytes}, nil
}

// Push accepts one real encoded packet or an explicit media gap. Untimed
// packets decode immediately without adding a jitter wait. Returned frames
// successfully decoded before an error remain valid and owned by the caller.
func (r *Receiver) Push(payload []byte, info *packet.OpusInfo, now time.Time) ([]Frame, error) {
	if r == nil || r.closed {
		return nil, io.ErrClosedPipe
	}
	if info == nil {
		if len(payload) == 0 {
			return nil, nil
		}
		out, err := r.decodeOwned(r.buffer.Flush())
		if err != nil {
			return out, err
		}
		r.buffer.Discard()
		r.gap = nil
		frames, err := r.decodeOwned([]rtpstream.Frame{{Payload: append([]byte(nil), payload...)}})
		return append(out, frames...), err
	}
	if len(payload) > 0 && !info.Missing && (len(payload) > r.maxBytes || info.Header.Version != 2 || info.Duration48 == 0 || info.Duration48 > 5760) {
		r.invalidPackets++
		r.lastDecodeError = fmt.Errorf("opusmedia: invalid packet size or RTP Opus metadata")
		return nil, nil
	}
	if info.Missing {
		out, err := r.decodeOwned(r.buffer.Flush())
		if err != nil {
			return out, err
		}
		// The gap is already known. Clear only the reorder clock, preserving
		// decoder history for PLC and the buffer's delivered-sequence fence.
		r.buffer.Discard()
		frames, err := r.decodeOwned([]rtpstream.Frame{{Payload: append([]byte(nil), payload...), Info: info.Clone()}})
		if err == nil {
			r.gap = info.Clone()
		}
		return append(out, frames...), err
	}
	if info.Discontinuity {
		r.buffer.Reset()
		r.gap = nil
		if _, err := r.decoder.Decode(nil, info); err != nil {
			return nil, fmt.Errorf("opusmedia: reset decoder: %w", err)
		}
	}
	if len(payload) == 0 {
		return nil, nil
	}
	if r.gap != nil && info.SourceID == r.gap.SourceID {
		end := r.gap.Header.Timestamp + r.gap.Duration48
		if int32(info.Header.Timestamp-end) < 0 {
			return nil, nil
		}
		r.gap = nil
	}
	return r.decodeOwned(r.buffer.Push(payload, info, now))
}

// Pop releases packets whose reorder deadline has elapsed and decodes both
// the real packets and any exactly timed missing intervals using one decoder.
func (r *Receiver) Pop(now time.Time) ([]Frame, error) {
	if r == nil || r.closed {
		return nil, io.ErrClosedPipe
	}
	return r.decodeOwned(r.buffer.Pop(now))
}

// Flush decodes all pending media before an EOS or another draining boundary.
// Unknown trailing loss is not synthesized.
func (r *Receiver) Flush() ([]Frame, error) {
	if r == nil || r.closed {
		return nil, io.ErrClosedPipe
	}
	return r.decodeOwned(r.buffer.Flush())
}

// Discard abandons pending media for a new BOS and resets decoder history.
// The jitter buffer retains its sequence fence against late old packets.
func (r *Receiver) Discard() {
	if r == nil || r.closed {
		return
	}
	r.buffer.Discard()
	r.gap = nil
	_ = r.decoder.Reset()
}

// Deadline reports when the owner should call Pop again.
func (r *Receiver) Deadline() (time.Time, bool) {
	if r == nil || r.closed {
		return time.Time{}, false
	}
	return r.buffer.Deadline()
}

// Close drops remaining pending packets and releases the native decoder. Call
// Flush first when the pending media belongs to a normally completed stream.
func (r *Receiver) Close() error {
	if r == nil || r.closed {
		return nil
	}
	r.buffer.Discard()
	r.gap = nil
	r.closed = true
	return r.decoder.Close()
}

// InvalidPackets reports received encoded packets rejected as malformed or
// replaced by concealment. It does not count ordinary inferred missing frames.
// Like the media methods, diagnostic reads are serialized by the owner.
func (r *Receiver) InvalidPackets() uint64 {
	if r == nil {
		return 0
	}
	return r.invalidPackets
}

// LastDecodeError returns the latest rejected-media cause without logging or
// terminating the parent connection. It never includes encoded payload bytes.
func (r *Receiver) LastDecodeError() error {
	if r == nil {
		return nil
	}
	return r.lastDecodeError
}

func knownOpusDuration(frame rtpstream.Frame) bool {
	info := frame.Info
	if info == nil || info.Missing || len(frame.Payload) == 0 || info.Duration48 == 0 || info.Duration48 > 5760 || info.Duration48%120 != 0 {
		return false
	}
	// A code-3 TOC requires its frame-count byte. The duration helper's
	// compatibility fallback for a truncated count is not proof of duration.
	if frame.Payload[0]&3 == 3 && (len(frame.Payload) < 2 || frame.Payload[1]&0x3f == 0) {
		return false
	}
	return packet.OpusDuration48(frame.Payload) == info.Duration48
}

// decodeOwned consumes independently owned payloads and metadata. Jitter releases
// owned frames; Push copies borrowed untimed packets and explicit gaps first.
func (r *Receiver) decodeOwned(encoded []rtpstream.Frame) ([]Frame, error) {
	var out []Frame
	for _, frame := range encoded {
		samples, err := r.decoder.Decode(frame.Payload, frame.Info)
		if err != nil {
			if errors.Is(err, codecconv.ErrOpusDecoderInit) {
				return out, fmt.Errorf("opusmedia: decode packet: %w", err)
			}
			r.lastDecodeError = fmt.Errorf("opusmedia: decode packet: %w", err)
			if len(frame.Payload) > 0 {
				r.invalidPackets++
			}
			if knownOpusDuration(frame) {
				concealed := frame.Info.Clone()
				concealed.Missing, concealed.Discontinuity = true, false
				samples, err = r.decoder.Decode(nil, concealed)
				frame.Info, frame.Payload = concealed, nil
			}
			if err != nil {
				// Corruption can invalidate native history, but it is not a media
				// boundary. Keep the source and concealment budget until a real
				// packet decodes successfully or an explicit boundary resets them.
				_ = r.decoder.ResetDecoder()
				continue
			}
		}
		if len(samples) == 0 {
			continue
		}
		decoded := Frame{PCM: make([]byte, len(samples)*2), Info: frame.Info}
		for i, sample := range samples {
			binary.LittleEndian.PutUint16(decoded.PCM[i*2:], uint16(sample))
		}
		decoded.Concealed = frame.Info != nil && frame.Info.Missing
		if !decoded.Concealed {
			decoded.Original = frame.Payload
		}
		out = append(out, decoded)
	}
	return out, nil
}
