package gizcli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

func TestPeerStreamAudioBeforeInitialBOS(t *testing.T) {
	s := &PeerStream{out: make(chan *genx.MessageChunk, 8), done: make(chan struct{})}
	defer s.Close()
	payload := []byte{0xf8, 0xff, 0xfe}
	for range 2 {
		if err := s.pushMergedPacket(payload); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case c := <-s.out:
		t.Fatalf("audio escaped before BOS binding: StreamID=%s", c.Ctrl.StreamID)
	default:
	}
	bos, err := peerStreamEventToChunk(bosEvent("answer", "assistant", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(bos); err != nil {
		t.Fatal(err)
	}
	first, err := s.Next()
	if err != nil || !first.IsBeginOfStream() {
		t.Fatalf("first=%v, err=%v", first, err)
	}
	for range 2 {
		c, err := s.Next()
		if err != nil || c.Ctrl.StreamID != "answer" || c.Ctrl.Label != "assistant" || !bytes.Equal(c.Part.(*genx.Blob).Data, payload) {
			t.Fatalf("packet=%v, err=%v", c, err)
		}
	}
}

func TestPeerStreamInitialBOSBufferLimits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
		count   int
	}{
		{"packets", []byte{1}, 64},
		{"bytes", make([]byte, 64*1024), 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &PeerStream{out: make(chan *genx.MessageChunk, 1), done: make(chan struct{})}
			defer s.Close()
			for range tc.count {
				if err := s.pushMergedPacket(tc.payload); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.pushMergedPacket([]byte{1}); err == nil {
				t.Fatal("overflow accepted")
			}
		})
	}
}

func TestPeerStreamInitialBOSTimeout(t *testing.T) {
	packets := make(chan []byte, 1)
	packets <- []byte{1}
	s := &PeerStream{packets: packets, out: make(chan *genx.MessageChunk, 1), done: make(chan struct{})}
	defer s.Close()
	finished := make(chan struct{})
	go func() { s.mergeOutput(); close(finished) }()
	select {
	case <-finished:
		if !strings.Contains(s.closeErr().Error(), "initial audio BOS timeout") {
			t.Fatal(s.closeErr())
		}
		if len(s.startupAudio) != 0 {
			t.Fatal("retained startup audio")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing BOS did not terminate stream")
	}
}

func TestPeerStreamDoesNotReassignPostEOSAudio(t *testing.T) {
	s := &PeerStream{out: make(chan *genx.MessageChunk, 4), done: make(chan struct{})}
	defer s.Close()
	for _, e := range []*eventpb.PeerEvent{bosEvent("old", "assistant", ""), eosEvent("old", "assistant", "", nil)} {
		c, err := peerStreamEventToChunk(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.pushMergedEvent(c); err != nil {
			t.Fatal(err)
		}
		<-s.out
	}
	if err := s.pushMergedPacket([]byte{1}); err != nil {
		t.Fatal(err)
	}
	c := <-s.out
	if c.Ctrl.StreamID != "audio" || len(s.startupAudio) != 0 {
		t.Fatalf("late packet silently reassigned: %v", c)
	}
}

func TestPeerStreamInitialBOSIgnoresTextAndCopiesPackets(t *testing.T) {
	s := &PeerStream{out: make(chan *genx.MessageChunk, 4), done: make(chan struct{})}
	defer s.Close()
	payload := []byte{1, 2}
	if err := s.pushMergedPacket(payload); err != nil {
		t.Fatal(err)
	}
	payload[0] = 9
	text, err := peerStreamEventToChunk(textEvent("answer", "assistant", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(text); err != nil {
		t.Fatal(err)
	}
	if got := <-s.out; got != text {
		t.Fatal("text was blocked by pending audio")
	}
	if len(s.out) != 0 {
		t.Fatal("text released pending audio")
	}
	bos, err := peerStreamEventToChunk(bosEvent("answer", "assistant", "audio/opus"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(bos); err != nil {
		t.Fatal(err)
	}
	<-s.out
	if c := <-s.out; !bytes.Equal(c.Part.(*genx.Blob).Data, []byte{1, 2}) {
		t.Fatal("payload alias changed buffered audio")
	}
}

func TestPeerStreamCloseReleasesInitialAudio(t *testing.T) {
	packets := make(chan []byte)
	events := make(chan peerStreamEventResult)
	s := &PeerStream{packets: packets, eventResults: events, out: make(chan *genx.MessageChunk, 1), done: make(chan struct{})}
	finished := make(chan struct{})
	go func() { s.mergeOutput(); close(finished) }()
	packets <- []byte{1}
	// The event rendezvous proves the packet was processed before Close.
	events <- peerStreamEventResult{}
	_ = s.Close()
	select {
	case <-finished:
		if len(s.startupAudio) != 0 {
			t.Fatal("close retained startup audio")
		}
	case <-time.After(time.Second):
		t.Fatal("merge did not exit on close")
	}
}
