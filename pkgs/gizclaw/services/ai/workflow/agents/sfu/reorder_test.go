package sfu

import (
	"testing"

	"github.com/pion/rtp"
)

func rtpPacket(seq uint16, ts uint32) *rtp.Packet {
	return &rtp.Packet{Header: rtp.Header{SequenceNumber: seq, Timestamp: ts}, Payload: []byte{byte(seq)}}
}

func seqs(packets []*rtp.Packet) []uint16 {
	out := make([]uint16, 0, len(packets))
	for _, packet := range packets {
		out = append(out, packet.SequenceNumber)
	}
	return out
}

func equalSeqs(got []uint16, want ...uint16) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestReorderBufferEmitsInOrder(t *testing.T) {
	b := newReorderBuffer(3, 48000)
	if got := seqs(b.Push(rtpPacket(10, 0))); !equalSeqs(got, 10) {
		t.Fatalf("first push = %v", got)
	}
	if got := seqs(b.Push(rtpPacket(12, 1920))); len(got) != 0 {
		t.Fatalf("out-of-order push emitted %v", got)
	}
	if got := seqs(b.Push(rtpPacket(11, 960))); !equalSeqs(got, 11, 12) {
		t.Fatalf("gap fill = %v", got)
	}
	if got := seqs(b.Push(rtpPacket(11, 960))); len(got) != 0 {
		t.Fatalf("late duplicate emitted %v", got)
	}
	if b.Len() != 0 {
		t.Fatalf("pending = %d, want 0", b.Len())
	}
}

func TestReorderBufferSkipsLostPacketAtDepth(t *testing.T) {
	b := newReorderBuffer(2, 48000)
	b.Push(rtpPacket(1, 0))
	if got := seqs(b.Push(rtpPacket(3, 1920))); len(got) != 0 {
		t.Fatalf("push 3 = %v", got)
	}
	if got := seqs(b.Push(rtpPacket(4, 2880))); len(got) != 0 {
		t.Fatalf("push 4 = %v", got)
	}
	// Depth exceeded: sequence 2 is declared lost and 3..5 are released.
	if got := seqs(b.Push(rtpPacket(5, 3840))); !equalSeqs(got, 3, 4, 5) {
		t.Fatalf("push 5 = %v", got)
	}
	if got := seqs(b.Push(rtpPacket(2, 960))); len(got) != 0 {
		t.Fatalf("late packet emitted %v", got)
	}
}

func TestReorderBufferSkipsLostPacketBySpan(t *testing.T) {
	b := newReorderBuffer(100, 4800)
	b.Push(rtpPacket(1, 0))
	if got := seqs(b.Push(rtpPacket(3, 1920))); len(got) != 0 {
		t.Fatalf("push 3 = %v", got)
	}
	if got := seqs(b.Push(rtpPacket(4, 8000))); !equalSeqs(got, 3, 4) {
		t.Fatalf("span flush = %v", got)
	}
}

func TestReorderBufferWrapsSequence(t *testing.T) {
	b := newReorderBuffer(3, 48000)
	if got := seqs(b.Push(rtpPacket(65535, 0))); !equalSeqs(got, 65535) {
		t.Fatalf("push 65535 = %v", got)
	}
	if got := seqs(b.Push(rtpPacket(1, 1920))); len(got) != 0 {
		t.Fatalf("push 1 = %v", got)
	}
	if got := seqs(b.Push(rtpPacket(0, 960))); !equalSeqs(got, 0, 1) {
		t.Fatalf("wrap fill = %v", got)
	}
}

func TestReorderBufferFlushReleasesPendingInOrder(t *testing.T) {
	b := newReorderBuffer(10, 48000)
	b.Push(rtpPacket(1, 0))
	b.Push(rtpPacket(4, 2880))
	b.Push(rtpPacket(3, 1920))
	if got := seqs(b.Flush()); !equalSeqs(got, 3, 4) {
		t.Fatalf("Flush = %v", got)
	}
	if got := seqs(b.Push(rtpPacket(5, 3840))); !equalSeqs(got, 5) {
		t.Fatalf("push after flush = %v", got)
	}
	if got := seqs(b.Flush()); len(got) != 0 {
		t.Fatalf("empty Flush = %v", got)
	}
}
