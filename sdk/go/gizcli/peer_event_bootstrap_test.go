package gizcli

import (
	"bytes"
	"io"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func publishBootstrapEvent(t *testing.T, session *peerEventSession, event *eventpb.PeerEvent) {
	t.Helper()
	var frame bytes.Buffer
	if err := WritePeerStreamEvent(&frame, event); err != nil {
		t.Fatal(err)
	}
	session.publishEvent(frame.Bytes(), event)
}

func readBootstrapEvent(t *testing.T, conn net.Conn) *eventpb.PeerEvent {
	t.Helper()
	result := make(chan *eventpb.PeerEvent, 1)
	go func() {
		event, _ := ReadPeerStreamEvent(conn)
		result <- event
	}()
	select {
	case event := <-result:
		if event == nil {
			t.Fatal("event stream closed")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("event stream made no progress")
		return nil
	}
}

func TestPeerEventSessionLateSubscriberReceivesCurrentAudioRoute(t *testing.T) {
	for _, mime := range []string{"", "audio/opus", "Audio/Opus;rate=16000"} {
		t.Run(mime, func(t *testing.T) {
			server, physical := net.Pipe()
			defer server.Close()
			session := newPeerEventSession(physical, nil)
			defer session.close()
			session.start()
			client := &Client{events: session}
			first, err := session.subscribe()
			if err != nil {
				t.Fatal(err)
			}
			if err := WritePeerStreamEvent(server, bosEvent("ongoing", "assistant", mime)); err != nil {
				t.Fatal(err)
			}
			readBootstrapEvent(t, first)
			_ = first.Close()
			late, err := client.OpenPeerStream(8)
			if err != nil {
				t.Fatal(err)
			}
			defer late.Close()
			bos, err := late.Next()
			if err != nil || !bos.IsBeginOfStream() || bos.Ctrl.StreamID != "ongoing" {
				t.Fatalf("late subscriber BOS=%v, error=%v", bos, err)
			}
			packet := []byte{0xf8, 0xff, 0xfe}
			client.dispatchPeerPacket(giznet.ProtocolOpusPacket, packet)
			chunk, err := late.Next()
			if err != nil || chunk.Ctrl.StreamID != "ongoing" || chunk.Ctrl.Label != "assistant" || !bytes.Equal(chunk.Part.(*genx.Blob).Data, packet) {
				t.Fatalf("live packet=%v, error=%v", chunk, err)
			}
		})
	}
}

func TestPeerEventSessionBootstrapDoesNotReplayCompletedOrSupersededAudio(t *testing.T) {
	for _, finish := range []string{"text-eos", "old-audio-eos", "current-audio-eos"} {
		t.Run(finish, func(t *testing.T) {
			session := newPeerEventSession(nil, nil)
			defer session.shutdown()
			publishBootstrapEvent(t, session, bosEvent("old", "assistant", "audio/opus"))
			publishBootstrapEvent(t, session, bosEvent("current", "assistant", "audio/opus"))
			end := eosEvent("current", "assistant", "audio/opus", nil)
			if finish == "text-eos" {
				end.GetEos().Kind = eventpb.StreamKind_STREAM_KIND_TEXT
				end.GetEos().MimeType = "text/plain"
			} else if finish == "old-audio-eos" {
				end.GetEos().StreamId = "old"
			}
			publishBootstrapEvent(t, session, end)
			late, err := session.subscribe()
			if err != nil {
				t.Fatal(err)
			}
			defer late.Close()
			publishBootstrapEvent(t, session, textEvent("current", "assistant", "live-only"))
			got := readBootstrapEvent(t, late)
			if finish == "current-audio-eos" {
				if got.Text() != "live-only" {
					t.Fatal("completed audio was replayed")
				}
			} else {
				if got.GetBos() == nil || got.GetBos().StreamId != "current" {
					t.Fatal("text or obsolete EOS cleared the active route")
				}
				if readBootstrapEvent(t, late).Text() != "live-only" {
					t.Fatal("bootstrap was not ordered before live events")
				}
			}
		})
	}
}

func TestPeerEventSessionBootstrapOverflowDoesNotBlockNewSubscribers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		session := newPeerEventSession(nil, nil)
		defer session.shutdown()
		old, err := session.subscribe()
		if err != nil {
			t.Fatal(err)
		}
		blocked := old.(*peerEventConn)
		for range cap(blocked.queue) {
			blocked.queue <- []byte("unconsumed")
		}
		done := make(chan struct{})
		go func() {
			publishBootstrapEvent(t, session, bosEvent("active", "assistant", "audio/opus"))
			close(done)
		}()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("overflow held the session lock")
		}
		late, err := session.subscribe()
		if err != nil {
			t.Fatal(err)
		}
		defer late.Close()
		if readBootstrapEvent(t, late).GetBos().StreamId != "active" {
			t.Fatal("overflow lost the current route")
		}
	})
}

func TestPeerEventSessionShutdownDiscardsBootstrapPrefix(t *testing.T) {
	session := newPeerEventSession(nil, nil)
	publishBootstrapEvent(t, session, bosEvent("active", "assistant", "audio/opus"))
	late, err := session.subscribe()
	if err != nil {
		t.Fatal(err)
	}
	session.shutdown()
	if _, err := late.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("shutdown returned bootstrap bytes: %v", err)
	}
	publishBootstrapEvent(t, session, bosEvent("stale", "assistant", "audio/opus"))
	if len(session.audioStart) != 0 {
		t.Fatal("publication restored a closed session's route")
	}
}
