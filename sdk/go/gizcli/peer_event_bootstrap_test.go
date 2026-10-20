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
		for _, late := range []bool{false, true} {
			name := mime + "/before-bos"
			if late {
				name = mime + "/after-bos"
			}
			t.Run(name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					server, physical := net.Pipe()
					defer server.Close()
					session := newPeerEventSession(physical, nil)
					defer session.close()
					session.start()
					client := &Client{events: session}
					first, err := client.DialPeerEventStream()
					if err != nil {
						t.Fatal(err)
					}
					defer first.Close()
					sendInitialBOS := func() {
						t.Helper()
						if err := WritePeerStreamEvent(server, bosEvent("ongoing", "assistant", mime)); err != nil {
							t.Fatal(err)
						}
						event, err := ReadPeerStreamEvent(first)
						if err != nil || event.GetBos().GetStreamId() != "ongoing" {
							t.Fatalf("initial subscriber: event=%v error=%v", event, err)
						}
					}
					if late {
						sendInitialBOS()
					}
					receiver, err := client.OpenPeerStream(16)
					if err != nil {
						t.Fatal(err)
					}
					defer receiver.Close()
					if !late {
						sendInitialBOS()
					}
					first.Close()
					if err := WritePeerStreamEvent(server, textEvent("ongoing", "assistant", "still playing")); err != nil {
						t.Fatal(err)
					}
					packet := []byte{0xf8, 0xff, 0xfe}
					client.dispatchPeerPacket(giznet.ProtocolOpusPacket, packet)
					synctest.Wait()
					// Exercise packets beyond the initial one-second reorder budget.
					time.Sleep(2 * time.Second)
					synctest.Wait()
					client.dispatchPeerPacket(giznet.ProtocolOpusPacket, packet)
					synctest.Wait()
					audio, bos, texts := 0, 0, 0
				drain:
					for {
						select {
						case chunk := <-receiver.out:
							if chunk.IsBeginOfStream() {
								bos++
							}
							if text, ok := chunk.Part.(genx.Text); ok && string(text) == "still playing" {
								texts++
							}
							if blob, ok := chunk.Part.(*genx.Blob); ok && bytes.Equal(blob.Data, packet) {
								audio++
								if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "ongoing" || chunk.Ctrl.Label != "assistant" {
									t.Errorf("lost active audio route: %v", chunk.Ctrl)
								}
							}
						default:
							break drain
						}
					}
					t.Logf("subscribe_after_BOS=%t: BOS=%d text=%d audio_packets=%d", late, bos, texts, audio)
					if texts != 1 {
						t.Errorf("live text count=%d, want 1", texts)
					}
					if bos != 1 || audio != 2 {
						t.Errorf("active audio not covered: BOS=%d audio_packets=%d, want 1 and 2", bos, audio)
					}
				})
			})
		}
	}
}

func TestPeerEventSessionBootstrapDoesNotReplayCompletedOrSupersededAudio(t *testing.T) {
	for _, finish := range []string{"text-eos", "old-audio-eos", "input-audio-bos", "input-audio-eos", "current-audio-eos", "non-opus-bos"} {
		t.Run(finish, func(t *testing.T) {
			session := newPeerEventSession(nil, nil)
			defer session.shutdown()
			publishBootstrapEvent(t, session, bosEvent("old", "assistant", "audio/opus"))
			publishBootstrapEvent(t, session, bosEvent("current", "assistant", "audio/opus"))
			end := eosEvent("current", "assistant", "audio/opus", nil)
			switch finish {
			case "text-eos":
				end.GetEos().Kind = eventpb.StreamKind_STREAM_KIND_TEXT
				end.GetEos().MimeType = "text/plain"
			case "old-audio-eos":
				end.GetEos().StreamId = "old"
			case "input-audio-bos":
				end = bosEvent("input", "user", "audio/opus")
			case "input-audio-eos":
				end.GetEos().Label = "user"
			case "non-opus-bos":
				end = bosEvent("replacement", "assistant", "audio/wav")
			}
			publishBootstrapEvent(t, session, end)
			late, err := session.subscribe()
			if err != nil {
				t.Fatal(err)
			}
			defer late.Close()
			publishBootstrapEvent(t, session, textEvent("current", "assistant", "live-only"))
			got := readBootstrapEvent(t, late)
			if finish == "current-audio-eos" || finish == "non-opus-bos" {
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
