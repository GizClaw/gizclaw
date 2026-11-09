package gizcli

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestPeerStreamBindAudioRoutePreservesOwnedPart(t *testing.T) {
	for _, hasCtrl := range []bool{false, true} {
		blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: []byte{1, 0}, OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}
		chunk := &genx.MessageChunk{Role: genx.RoleModel, Name: "speaker", Part: blob}
		if hasCtrl {
			chunk.Ctrl = &genx.StreamCtrl{StreamID: "audio", Timestamp: 123, MessageID: "message", MessageEnd: true}
		}
		s := &PeerStream{}
		if got := s.bindAudioRoute(chunk); got != chunk {
			t.Fatal("empty route changed chunk identity")
		}
		s.audioRoute = genx.StreamCtrl{StreamID: "answer", Label: "assistant"}
		got := s.bindAudioRoute(chunk)
		if got == chunk || got.Ctrl == nil || got.Ctrl == chunk.Ctrl || got.Part != blob {
			t.Fatal("route binding did not retain the owned Part with independent metadata")
		}
		if got.Role != chunk.Role || got.Name != chunk.Name || got.Ctrl.StreamID != "answer" || got.Ctrl.Label != "assistant" {
			t.Fatalf("route binding changed chunk metadata: %+v", got)
		}
		if hasCtrl && (got.Ctrl.Timestamp != 123 || got.Ctrl.MessageID != "message" || !got.Ctrl.MessageEnd) {
			t.Fatal("route binding dropped existing control metadata")
		}
		got.Ctrl.StreamID = "changed"
		if hasCtrl && (chunk.Ctrl.StreamID != "audio" || chunk.Ctrl.Label != "") || !hasCtrl && chunk.Ctrl != nil {
			t.Fatal("route binding modified the source control")
		}
	}
}

func TestClientPeerAudioSubscribersOwnPayloadsAndProvenance(t *testing.T) {
	client := &Client{}
	first, unsubscribeFirst := client.subscribePeerAudio(1)
	defer unsubscribeFirst()
	second, unsubscribeSecond := client.subscribePeerAudio(1)
	defer unsubscribeSecond()
	encoded, unsubscribeEncoded := client.subscribePeerPackets(giznet.ProtocolOpusPacket, 1)
	defer unsubscribeEncoded()
	blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: []byte{1, 0}, OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}
	client.dispatchPeerMedia(giznet.ProtocolOpusPacket, blob.OriginalAudio.Data, blob)
	blob.Data[0] = 9
	blob.OriginalAudio.Data[0] = 0
	a, b := <-first, <-second
	if a == blob || b == blob || a == b || a.OriginalAudio == blob.OriginalAudio || a.OriginalAudio == b.OriginalAudio {
		t.Fatal("audio subscribers shared mutable Blob metadata")
	}
	for _, got := range []*genx.Blob{a, b} {
		if !bytes.Equal(got.Data, []byte{1, 0}) || !bytes.Equal(got.OriginalAudio.Data, []byte{0xf8, 0xff, 0xfe}) {
			t.Fatal("source mutation changed subscriber audio")
		}
	}
	a.Data[0], a.OriginalAudio.Data[0] = 8, 0
	if b.Data[0] != 1 || b.OriginalAudio.Data[0] != 0xf8 {
		t.Fatal("one subscriber changed another subscriber's payload")
	}
	if packet := <-encoded; !bytes.Equal(packet, []byte{0xf8, 0xff, 0xfe}) {
		t.Fatalf("encoded subscription lost the real packet: %x", packet)
	}
	concealed := &genx.Blob{MIMEType: blob.MIMEType, Data: []byte{0, 0}, ConcealedAudio: true}
	client.dispatchPeerMedia(giznet.ProtocolOpusPacket, nil, concealed)
	for _, got := range []*genx.Blob{<-first, <-second} {
		if !got.ConcealedAudio || got.OriginalAudio != nil {
			t.Fatal("concealed audio acquired encoded provenance")
		}
	}
	select {
	case packet := <-encoded:
		t.Fatalf("concealed audio emitted a fake encoded packet: %x", packet)
	default:
	}
	unsubscribeFirst()
	client.dispatchPeerAudio(blob)
	select {
	case <-first:
		t.Fatal("unsubscribed audio consumer received another frame")
	default:
	}
	if got := <-second; !bytes.Equal(got.Data, blob.Data) {
		t.Fatal("unsubscribing one consumer stopped another")
	}
}

func TestClientPeerAudioFullSubscriberSkipsCloning(t *testing.T) {
	client := &Client{}
	slow, unsubscribeSlow := client.subscribePeerAudio(1)
	defer unsubscribeSlow()
	blob := &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 640), OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}
	for range cap(slow) {
		client.dispatchPeerAudio(blob)
	}
	if allocs := testing.AllocsPerRun(20, func() { client.dispatchPeerAudio(blob) }); allocs > 1 {
		t.Fatalf("already-full subscriber allocated %.0f times while dropping audio", allocs)
	}
	fast, unsubscribeFast := client.subscribePeerAudio(1)
	defer unsubscribeFast()
	client.dispatchPeerAudio(blob)
	select {
	case got := <-fast:
		if got == blob || got.OriginalAudio == blob.OriginalAudio || !bytes.Equal(got.Data, blob.Data) {
			t.Fatal("slow subscriber weakened another subscriber's ownership")
		}
	default:
		t.Fatal("slow subscriber blocked an available subscriber")
	}
	if len(slow) != cap(slow) {
		t.Fatal("dropping a frame changed the queued audio")
	}
}

func TestClientPeerAudioConcurrentDispatchAndUnsubscribe(t *testing.T) {
	client := &Client{}
	audio, unsubscribe := client.subscribePeerAudio(1)
	defer unsubscribe()
	blob := &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			for range 100 {
				client.dispatchPeerAudio(blob)
			}
		})
	}
	for range 100 {
		_, stop := client.subscribePeerAudio(1)
		stop()
		select {
		case <-audio:
		default:
		}
	}
	workers.Wait()
}

func TestPeerStreamDrainsRecoveredPCMBeforeEOS(t *testing.T) {
	audio := make(chan *genx.Blob, 2)
	s := &PeerStream{audio: audio, pcmAudio: true, out: make(chan *genx.MessageChunk, 8), done: make(chan struct{})}
	defer s.Close()
	bos, err := peerStreamEventToChunk(bosEvent("answer", "assistant", ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(bos); err != nil {
		t.Fatal(err)
	}
	<-s.out
	flushed := false
	s.flushAudio = func(context.Context) error {
		flushed = true
		audio <- &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640), OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}
		audio <- &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640), ConcealedAudio: true}
		return nil
	}
	eos, err := peerStreamEventToChunk(eosEvent("answer", "assistant", "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.pushMergedEvent(eos); err != nil {
		t.Fatal(err)
	}
	if !flushed {
		t.Fatal("EOS did not drain the media receiver")
	}
	for i := range 2 {
		chunk := <-s.out
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || chunk.IsEndOfStream() || chunk.Ctrl.StreamID != "answer" || chunk.Ctrl.Label != "assistant" || blob.ConcealedAudio != (i == 1) {
			t.Fatalf("recovered frame %d was lost or overtaken: %+v", i, chunk)
		}
		if i == 1 && blob.OriginalAudio != nil {
			t.Fatal("PLC invented original audio")
		}
	}
	if chunk := <-s.out; !chunk.IsEndOfStream() {
		t.Fatal("EOS missing after recovered media")
	}
	if err := s.pushMergedAudio(&genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640)}); err != nil {
		t.Fatal(err)
	}
	if len(s.out) != 0 || s.IgnoredPostEOSAudioPackets() != 1 {
		t.Fatal("late decoded tail crossed EOS")
	}
}
