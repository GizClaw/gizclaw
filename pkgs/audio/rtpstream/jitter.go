// Package rtpstream provides bounded, per-source media reordering. It retains
// the RTP clock and describes known gaps; the codec consumer performs PLC.
package rtpstream

import (
	"slices"
	"time"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/pion/rtp"
)

// Config bounds latency and storage for one encoded Opus source.
type Config struct {
	Wait       time.Duration
	MaxPackets int
	// MaxBytes includes retained frame metadata, CSRCs and RTP extensions.
	MaxBytes int
	MaxGap48 uint32
}

// Frame is an original payload or a known, exactly timed missing interval.
// Missing intervals have no encoded payload and are never fake Opus packets.
type Frame struct {
	Payload []byte
	Info    *packet.OpusInfo
}

type pendingFrame struct {
	Frame
	arrived time.Time
	bytes   int
}

// A large sequence jump needs two consecutive packets with a continuous media
// clock. Only scalar evidence is retained until the second packet confirms it.
type restartProbe struct {
	next    uint16
	end     uint32
	arrived time.Time
	due     time.Time
}

const (
	// These are the dropout/misorder regions from RTP's sequence validation.
	// Ordinary late packets never enter restart probation.
	maxSequenceDropout  = 3000
	maxSequenceMisorder = 100
)

// Buffer is owned by one reader. Calls must be serialized by the stream owner.
// No goroutines, I/O, callbacks, or connection-global ordering are introduced.
type Buffer struct {
	cfg      Config
	pending  map[uint16]pendingFrame
	bytes    int
	source   string
	retired  []string
	next     uint16
	priming  bool
	started  bool
	clock    bool
	lastEnd  uint32
	firstDue time.Time
	probe    restartProbe
}

// New creates an idle jitter buffer with bounded realtime defaults.
func New(cfg Config) *Buffer {
	if cfg.Wait <= 0 {
		cfg.Wait = 40 * time.Millisecond
	}
	if cfg.MaxPackets <= 0 {
		cfg.MaxPackets = 64
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 128 * 1024
	}
	if cfg.MaxGap48 == 0 {
		cfg.MaxGap48 = packet.OpusClockRate / 2
	}
	return &Buffer{cfg: cfg, pending: make(map[uint16]pendingFrame)}
}

// Push retains an original packet and releases everything already deliverable.
// Old, duplicated, malformed, or oversized packets are discarded locally.
func (b *Buffer) Push(payload []byte, info *packet.OpusInfo, now time.Time) []Frame {
	if info == nil || info.Missing || len(payload) == 0 || info.Header.Version != 2 ||
		info.Duration48 == 0 || info.Duration48 > 5760 || info.Duration48%120 != 0 {
		return nil
	}
	bytes, fits := retainedBytes(len(payload), info, b.cfg.MaxBytes)
	if !fits {
		return nil
	}
	if info.SourceID != b.source {
		if slices.Contains(b.retired, info.SourceID) {
			return nil
		}
		if b.source != "" {
			b.retired = append(b.retired, b.source)
			if len(b.retired) > 8 {
				b.retired = b.retired[1:]
			}
		}
		b.resetPending()
		b.source = info.SourceID
	}
	if info.Discontinuity {
		b.Reset()
	}
	seq := info.Header.SequenceNumber
	resynced := false
	if b.started {
		var accepted bool
		accepted, resynced = b.acceptSequence(info, now)
		if !accepted {
			return nil
		}
	}
	if _, duplicate := b.pending[seq]; duplicate {
		return nil
	}
	var out []Frame
	if len(b.pending) >= b.cfg.MaxPackets || bytes > b.cfg.MaxBytes-b.bytes {
		out = b.Flush()
		if b.started && int16(seq-b.next) < 0 {
			return out
		}
	}
	if !b.started && !b.priming {
		b.next = seq
		b.priming = true
		b.firstDue = now.Add(b.cfg.Wait)
	} else if b.priming && int16(seq-b.next) < 0 {
		b.next = seq
	}
	owned := info.Clone()
	owned.Discontinuity = owned.Discontinuity || resynced
	b.pending[seq] = pendingFrame{Frame: Frame{Payload: append([]byte(nil), payload...), Info: owned}, arrived: now, bytes: bytes}
	b.bytes += bytes
	return append(out, b.Pop(now)...)
}

// Pop releases in-order packets and declares gaps after their reorder deadline.
func (b *Buffer) Pop(now time.Time) []Frame {
	if b.priming && now.Before(b.firstDue) {
		return nil
	}
	if b.priming {
		b.priming = false
		b.started = true
	}
	return b.release(now, false)
}

// Flush releases pending media before a stream boundary. Only gaps bounded by
// real packets are concealed; an unknown trailing loss or DTX is not invented.
func (b *Buffer) Flush() []Frame {
	if b.priming {
		b.priming = false
		b.started = true
	}
	return b.release(time.Time{}, true)
}

// Reset abandons pending media and the delivered sequence and media-clock fences.
// Use it only for an explicit source discontinuity; an ordinary stream boundary
// must use Discard so late packets cannot reenter the next stream.
func (b *Buffer) Reset() {
	b.resetPending()
}

// Discard abandons pending media while retaining the delivered sequence fence.
func (b *Buffer) Discard() {
	for seq, frame := range b.pending {
		if !b.started || int16(seq-b.next) >= 0 {
			b.next = seq + 1
			b.lastEnd = frame.Info.Header.Timestamp + frame.Info.Duration48
			b.started = true
		}
	}
	clear(b.pending)
	b.bytes = 0
	b.priming = false
	b.firstDue = time.Time{}
	b.probe = restartProbe{}
	// A stream boundary is not evidence that the silent interval was lost.
	b.clock = false
}

// Deadline returns the next bounded wait, if any packet is pending.
func (b *Buffer) Deadline() (time.Time, bool) {
	if len(b.pending) == 0 {
		return time.Time{}, false
	}
	if b.priming {
		return b.firstDue, true
	}
	return b.pendingDeadline(), true
}

// A newly arrived lower sequence must not restart an already open gap's wait.
// The earliest retained arrival remains evidence of that gap until it is filled
// or skipped. Sequence selection remains independent of arrival order.
func (b *Buffer) pendingDeadline() time.Time {
	var first time.Time
	set := false
	for _, frame := range b.pending {
		if !set || frame.arrived.Before(first) {
			first = frame.arrived
			set = true
		}
	}
	return first.Add(b.cfg.Wait)
}

func (b *Buffer) release(now time.Time, force bool) []Frame {
	var out []Frame
	var concealed uint32
	for len(b.pending) > 0 {
		frame, ok := b.pending[b.next]
		if !ok {
			frame = b.oldest()
			if !force && now.Before(b.pendingDeadline()) {
				break
			}
			if b.clock {
				delta := int32(frame.Info.Header.Timestamp - b.lastEnd)
				missing := uint32(uint16(frame.Info.Header.SequenceNumber - b.next))
				// A marked talkspurt can follow DTX, so its clock gap is not
				// proof that the whole interval belongs to missing packets.
				if !frame.Info.Header.Marker && delta > 0 && uint32(delta) <= b.cfg.MaxGap48-concealed && delta%120 == 0 &&
					uint32(delta) >= missing*120 && uint32(delta) <= missing*5760 {
					gap := frame.Info.Clone()
					gap.Header.SequenceNumber = b.next
					gap.Header.Timestamp = b.lastEnd
					gap.Header.Marker = false
					gap.Duration48 = uint32(delta)
					gap.Missing = true
					out = append(out, Frame{Info: gap})
					concealed += uint32(delta)
				} else {
					frame.Info.Discontinuity = true
				}
			}
			b.next = frame.Info.Header.SequenceNumber
		}
		if b.clock && int32(frame.Info.Header.Timestamp-b.lastEnd) < 0 {
			frame.Info.Discontinuity = true
		}
		delete(b.pending, b.next)
		b.bytes -= frame.bytes
		b.next++
		b.lastEnd = frame.Info.Header.Timestamp + frame.Info.Duration48
		b.clock = true
		out = append(out, frame.Frame)
	}
	return out
}

func (b *Buffer) oldest() pendingFrame {
	var oldest pendingFrame
	set := false
	for _, frame := range b.pending {
		if !set || int16(frame.Info.Header.SequenceNumber-oldest.Info.Header.SequenceNumber) < 0 {
			oldest = frame
			set = true
		}
	}
	return oldest
}

func (b *Buffer) acceptSequence(info *packet.OpusInfo, now time.Time) (accepted, resynced bool) {
	seq := info.Header.SequenceNumber
	delta := uint16(seq - b.next)
	if delta < maxSequenceDropout {
		b.probe = restartProbe{}
		return true, false
	}
	if delta > ^uint16(0)-maxSequenceMisorder || int32(info.Header.Timestamp-b.lastEnd) <= 0 {
		return false, false
	}
	if !b.probe.due.IsZero() {
		if seq == b.probe.next && info.Header.Timestamp == b.probe.end &&
			!now.Before(b.probe.arrived) && !now.After(b.probe.due) {
			b.Reset()
			return true, true
		}
		if seq+1 == b.probe.next && info.Header.Timestamp+info.Duration48 == b.probe.end {
			// A duplicate is not fresh restart evidence and cannot extend probation.
			return false, false
		}
	}
	b.probe = restartProbe{
		next: seq + 1, end: info.Header.Timestamp + info.Duration48,
		arrived: now, due: now.Add(time.Duration(info.Duration48) * time.Second / packet.OpusClockRate).Add(b.cfg.Wait),
	}
	return false, false
}

// retainedBytes bounds every allocation made by retaining the frame, before
// Clone copies its RTP metadata. The extension view deliberately includes every
// entry, even for opaque profiles, duplicate IDs, or a disabled extension flag.
func retainedBytes(payloadBytes int, info *packet.OpusInfo, limit int) (int, bool) {
	left := limit - int(unsafe.Sizeof(pendingFrame{})+unsafe.Sizeof(packet.OpusInfo{}))
	if left < 0 || payloadBytes > left {
		return 0, false
	}
	left -= payloadBytes
	if len(info.SourceID) > left {
		return 0, false
	}
	left -= len(info.SourceID)
	if len(info.Header.CSRC) > left/4 {
		return 0, false
	}
	left -= len(info.Header.CSRC) * 4
	descriptorBytes := int(unsafe.Sizeof(rtp.Extension{}))
	if len(info.Header.Extensions) > left/descriptorBytes {
		return 0, false
	}
	left -= len(info.Header.Extensions) * descriptorBytes
	for i := range info.Header.Extensions {
		h := rtp.Header{Extension: true, ExtensionProfile: rtp.ExtensionProfileTwoByte, Extensions: info.Header.Extensions[i : i+1]}
		// The fixed RTP/extension headers are 16 bytes. The remainder is
		// a conservative, padded bound on this entry's private payload.
		bytes := h.MarshalSize() - 16
		if bytes < 0 || bytes > left {
			return 0, false
		}
		left -= bytes
	}
	return limit - left, true
}

func (b *Buffer) resetPending() {
	clear(b.pending)
	b.bytes = 0
	b.next = 0
	b.priming = false
	b.started = false
	b.clock = false
	b.lastEnd = 0
	b.firstDue = time.Time{}
	b.probe = restartProbe{}
}
