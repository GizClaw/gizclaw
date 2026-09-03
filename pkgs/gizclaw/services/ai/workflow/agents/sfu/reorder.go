package sfu

import (
	"sort"

	"github.com/pion/rtp"
)

const (
	// defaultReorderDepth bounds how many packets may wait for a missing
	// predecessor before the gap is declared lost. At 20 ms per packet this
	// is roughly 100 ms of reorder tolerance.
	defaultReorderDepth = 5
	// defaultReorderSpan bounds the RTP timestamp distance (48 kHz clock)
	// between the oldest waiting packet and the newest one; a larger span
	// means the missing packet is late beyond usefulness.
	defaultReorderSpan = 48000 / 10
)

// reorderBuffer restores RTP sequence order for one remote track. It never
// synthesizes packets: a gap that is declared lost is simply skipped, so the
// downstream decoder sees an ordered but possibly shorter stream. The
// AgentHost Opus decoder has no PLC entry point, and forwarding an out-of-
// order or duplicated packet would corrupt its decoder state, which is worse
// than a short dropout.
type reorderBuffer struct {
	maxDepth int
	maxSpan  uint32

	started bool
	next    uint16
	pending map[uint16]*rtp.Packet
}

func newReorderBuffer(maxDepth int, maxSpan uint32) *reorderBuffer {
	if maxDepth <= 0 {
		maxDepth = defaultReorderDepth
	}
	if maxSpan == 0 {
		maxSpan = defaultReorderSpan
	}
	return &reorderBuffer{maxDepth: maxDepth, maxSpan: maxSpan, pending: make(map[uint16]*rtp.Packet)}
}

// Push accepts one packet and returns every packet that became deliverable
// in sequence order.
func (b *reorderBuffer) Push(packet *rtp.Packet) []*rtp.Packet {
	if packet == nil {
		return nil
	}
	seq := packet.SequenceNumber
	if !b.started {
		b.started = true
		b.next = seq
	}
	if seqBefore(seq, b.next) {
		// Late or duplicate; the decoder already moved past it.
		return nil
	}
	if _, dup := b.pending[seq]; dup {
		return nil
	}
	b.pending[seq] = packet
	out := b.drain(nil)
	for len(b.pending) > 0 && (len(b.pending) > b.maxDepth || b.spanExceeded()) {
		b.skipToOldest()
		out = b.drain(out)
	}
	return out
}

// Flush returns every pending packet in sequence order and declares the gaps
// between them lost. Callers use it on idle timeouts and at track end.
func (b *reorderBuffer) Flush() []*rtp.Packet {
	var out []*rtp.Packet
	for len(b.pending) > 0 {
		b.skipToOldest()
		out = b.drain(out)
	}
	return out
}

// Len reports how many packets wait for a missing predecessor.
func (b *reorderBuffer) Len() int {
	return len(b.pending)
}

func (b *reorderBuffer) drain(out []*rtp.Packet) []*rtp.Packet {
	for {
		packet, ok := b.pending[b.next]
		if !ok {
			return out
		}
		delete(b.pending, b.next)
		b.next++
		out = append(out, packet)
	}
}

func (b *reorderBuffer) skipToOldest() {
	seqs := make([]uint16, 0, len(b.pending))
	for seq := range b.pending {
		seqs = append(seqs, seq)
	}
	sort.Slice(seqs, func(i, j int) bool { return seqBefore(seqs[i], seqs[j]) })
	b.next = seqs[0]
}

func (b *reorderBuffer) spanExceeded() bool {
	var oldest, newest *rtp.Packet
	for _, packet := range b.pending {
		if oldest == nil || seqBefore(packet.SequenceNumber, oldest.SequenceNumber) {
			oldest = packet
		}
		if newest == nil || seqBefore(newest.SequenceNumber, packet.SequenceNumber) {
			newest = packet
		}
	}
	if oldest == nil || newest == nil {
		return false
	}
	return newest.Timestamp-oldest.Timestamp > b.maxSpan
}

// seqBefore reports whether a precedes b under RTP sequence wraparound.
func seqBefore(a, b uint16) bool {
	return int16(a-b) < 0
}
