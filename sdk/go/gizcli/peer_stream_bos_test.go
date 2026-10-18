package gizcli

import (
	"bytes"
	"fmt"
	"testing"
	"testing/synctest"
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
			if err := s.pushMergedPacket([]byte{1}); err != nil {
				t.Fatal(err)
			}
			if len(s.startupAudio) != 0 || s.startupBytes != 0 || !s.startupExpired {
				t.Fatal("overflow did not discard the startup window")
			}
			if err := s.pushMergedPacket([]byte{2}); err != nil {
				t.Fatal(err)
			}
			if len(s.startupAudio) != 0 || len(s.out) != 0 {
				t.Fatal("expired window accepted orphan audio")
			}
			bos, err := peerStreamEventToChunk(bosEvent("recovered", "assistant", ""))
			if err != nil {
				t.Fatal(err)
			}
			if err := s.pushMergedEvent(bos); err != nil {
				t.Fatal(err)
			}
			if c := <-s.out; !c.IsBeginOfStream() {
				t.Fatal("missing recovery BOS")
			}
			if err := s.pushMergedPacket([]byte{3}); err != nil {
				t.Fatal(err)
			}
			if c := <-s.out; c.Ctrl.StreamID != "recovered" || !bytes.Equal(c.Part.(*genx.Blob).Data, []byte{3}) {
				t.Fatal("overflow prevented recovery")
			}
		})
	}
}

// Virtual time exercises the production deadline without scheduler sleeps.
func TestPeerStreamInitialBOSTimeout(t *testing.T) {
	for _, tc := range []struct{ interrupted, lateBOS bool }{
		{false, false}, {false, true}, {true, false}, {true, true},
	} {
		t.Run(fmt.Sprintf("interrupted=%t/lateBOS=%t", tc.interrupted, tc.lateBOS), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				packets := make(chan []byte)
				events := make(chan peerStreamEventResult)
				s := &PeerStream{packets: packets, eventResults: events, out: make(chan *genx.MessageChunk, 8), done: make(chan struct{})}
				defer s.Close()
				go s.mergeOutput()
				if tc.interrupted {
					// A reopened receiver can see the old EOS without its BOS.
					eos, err := peerStreamEventToChunk(eosEvent("cancelled", "assistant", "", nil))
					if err != nil {
						t.Fatal(err)
					}
					events <- peerStreamEventResult{chunk: eos}
					<-s.out
				}
				packets <- []byte{1}
				synctest.Wait()
				time.Sleep(2 * time.Second)
				synctest.Wait()
				select {
				case <-s.done:
					t.Fatalf("orphan packet terminated conversation: %v", s.closeErr())
				default:
				}
				if len(s.startupAudio) != 0 || s.startupBytes != 0 || len(s.out) != 0 {
					t.Fatal("expired audio retained or emitted")
				}
				packets <- []byte{3}
				synctest.Wait()
				if len(s.startupAudio) != 0 || len(s.out) != 0 {
					t.Fatal("orphan restarted expired window")
				}
				if tc.lateBOS {
					bos, err := peerStreamEventToChunk(bosEvent("new", "assistant", ""))
					if err != nil {
						t.Fatal(err)
					}
					events <- peerStreamEventResult{chunk: bos}
					if c := <-s.out; !c.IsBeginOfStream() {
						t.Fatal("missing BOS")
					}
					packets <- []byte{2}
					c := <-s.out
					if c.Ctrl.StreamID != "new" || !bytes.Equal(c.Part.(*genx.Blob).Data, []byte{2}) {
						t.Fatalf("wrong route: %v", c)
					}
				} else {
					text, err := peerStreamEventToChunk(textEvent("new", "assistant", "still alive"))
					if err != nil {
						t.Fatal(err)
					}
					events <- peerStreamEventResult{chunk: text}
					if c := <-s.out; c != text {
						t.Fatal("text stopped after timeout")
					}
				}
			})
		})
	}
}

func TestPeerStreamIgnoresPostEOSAudio(t *testing.T) {
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
	select {
	case c := <-s.out:
		t.Fatalf("post-EOS packet was delivered: %v", c)
	default:
	}
	if s.IgnoredPostEOSAudioPackets() != 1 || len(s.startupAudio) != 0 {
		t.Fatalf("ignored=%d, buffered=%d", s.IgnoredPostEOSAudioPackets(), len(s.startupAudio))
	}
	text, err := peerStreamEventToChunk(textEvent("next", "assistant", "still alive"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(text); err != nil {
		t.Fatal(err)
	}
	if c := <-s.out; c != text {
		t.Fatal("post-EOS audio prevented text delivery")
	}
	bos, err := peerStreamEventToChunk(bosEvent("next", "assistant", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(bos); err != nil {
		t.Fatal(err)
	}
	<-s.out
	if err := s.pushMergedPacket([]byte{2}); err != nil {
		t.Fatal(err)
	}
	if c := <-s.out; c.Ctrl.StreamID != "next" || !bytes.Equal(c.Part.(*genx.Blob).Data, []byte{2}) {
		t.Fatalf("next BOS did not resume audio: %v", c)
	}
	if s.IgnoredPostEOSAudioPackets() != 1 {
		t.Fatal("valid next-turn audio was counted as ignored")
	}
}

func TestPeerStreamEOSBeforeInitialBOSDiscardsOrphanAudio(t *testing.T) {
	s := &PeerStream{out: make(chan *genx.MessageChunk, 4), done: make(chan struct{})}
	defer s.Close()
	if err := s.pushMergedPacket([]byte{1}); err != nil {
		t.Fatal(err)
	}
	eos, err := peerStreamEventToChunk(eosEvent("old", "assistant", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(eos); err != nil {
		t.Fatal(err)
	}
	<-s.out
	for range 128 {
		if err := s.pushMergedPacket([]byte{2}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.out) != 0 || len(s.startupAudio) != 0 || s.startupBytes != 0 || s.IgnoredPostEOSAudioPackets() != 128 {
		t.Fatalf("ended audio was retained: ignored=%d, queued=%d, buffered=%d", s.IgnoredPostEOSAudioPackets(), len(s.out), len(s.startupAudio))
	}
	bos, err := peerStreamEventToChunk(bosEvent("next", "assistant", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(bos); err != nil {
		t.Fatal(err)
	}
	if c := <-s.out; !c.IsBeginOfStream() || len(s.out) != 0 {
		t.Fatal("old startup audio escaped into the next BOS")
	}
}

func TestPeerStreamNewBOSDiscardsQueuedPostEOSAudio(t *testing.T) {
	packets := make(chan []byte, 2)
	s := &PeerStream{packets: packets, out: make(chan *genx.MessageChunk, 4), done: make(chan struct{})}
	defer s.Close()
	for _, event := range []*eventpb.PeerEvent{bosEvent("old", "assistant", ""), eosEvent("old", "assistant", "", nil)} {
		chunk, err := peerStreamEventToChunk(event)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.pushMergedEvent(chunk); err != nil {
			t.Fatal(err)
		}
		<-s.out
	}
	packets <- []byte{1}
	packets <- []byte{2}
	bos, err := peerStreamEventToChunk(bosEvent("next", "assistant", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(bos); err != nil {
		t.Fatal(err)
	}
	<-s.out
	if len(packets) != 0 || s.IgnoredPostEOSAudioPackets() != 2 {
		t.Fatalf("queued tail packets survived the new BOS: queued=%d ignored=%d", len(packets), s.IgnoredPostEOSAudioPackets())
	}
	if err := s.pushMergedPacket([]byte{3}); err != nil {
		t.Fatal(err)
	}
	if chunk := <-s.out; chunk.Ctrl.StreamID != "next" || !bytes.Equal(chunk.Part.(*genx.Blob).Data, []byte{3}) {
		t.Fatalf("fresh audio was not delivered: %v", chunk)
	}
}

func TestPeerStreamInputEOSDoesNotEndOutputAudio(t *testing.T) {
	s := &PeerStream{out: make(chan *genx.MessageChunk, 4), done: make(chan struct{})}
	defer s.Close()
	for _, event := range []*eventpb.PeerEvent{
		bosEvent("reply", "assistant", ""),
		eosEvent("input", "user", "", &eventpb.EventError{Code: "INPUT_ROUTE_RELOADED", Retryable: true}),
	} {
		chunk, err := peerStreamEventToChunk(event)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.pushMergedEvent(chunk); err != nil {
			t.Fatal(err)
		}
		<-s.out
	}
	if err := s.pushMergedPacket([]byte{1}); err != nil {
		t.Fatal(err)
	}
	select {
	case chunk := <-s.out:
		if chunk.Ctrl.StreamID != "reply" || chunk.Ctrl.Label != "assistant" {
			t.Fatalf("input EOS changed output ownership: %v", chunk)
		}
	default:
		t.Fatal("input EOS suppressed active output audio")
	}
	if s.IgnoredPostEOSAudioPackets() != 0 {
		t.Fatal("input EOS caused valid output audio to be ignored")
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
