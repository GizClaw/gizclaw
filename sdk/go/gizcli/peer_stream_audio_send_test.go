package gizcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

func TestPeerStreamPCMSenderAcknowledgesAndFlushesBeforeEOS(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus")
	}
	for _, format := range []string{"", ";bits=16;format=s16le", ";bits=16;format=pcm_s16le", ";bits=16;format=PCM_S16LE"} {
		for _, terminal := range []string{"pcm", "whole-route"} {
			t.Run(format+"/"+terminal, func(t *testing.T) {
				client, server := net.Pipe()
				defer server.Close()
				writer := &recordingPeerPacketWriter{ch: make(chan []byte, 2)}
				s := &PeerStream{events: client, conn: writer, done: make(chan struct{}), eventResults: make(chan peerStreamEventResult, 2)}
				defer s.Close()
				go s.readEvents()
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				if err := server.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
					t.Fatal(err)
				}
				pushed := make(chan error, 1)
				mimeType := "audio/pcm;rate=16000;channels=1" + format
				blob := &genx.Blob{MIMEType: mimeType, Data: make([]byte, 320), OriginalAudio: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}}
				ctrl := &genx.StreamCtrl{StreamID: "pcm", BeginOfStream: true}
				chunk := &genx.MessageChunk{Part: blob, Ctrl: ctrl}
				go func() { pushed <- s.Push(ctx, chunk) }()
				bos, err := ReadPeerStreamEvent(server)
				if err != nil || bos.Type != eventpb.PeerEventType_PEER_EVENT_TYPE_BOS || bos.GetBos().MimeType != "audio/opus" {
					t.Fatalf("wire BOS=%+v,%v", bos, err)
				}
				select {
				case <-pushed:
					t.Fatal("PCM BOS completed before acknowledgment")
				default:
				}
				if err := WritePeerStreamEvent(server, &eventpb.PeerEvent{Version: eventpb.Version, Type: eventpb.PeerEventType_PEER_EVENT_TYPE_AUDIO_INPUT_READY, Payload: &eventpb.PeerEvent_AudioInputReady{AudioInputReady: &eventpb.AudioInputReady{StreamId: "pcm"}}}); err != nil {
					t.Fatal(err)
				}
				if err := <-pushed; err != nil {
					t.Fatal(err)
				}
				if chunk.Part != blob || chunk.Ctrl != ctrl || blob.MIMEType != mimeType || len(blob.Data) != 320 || !ctrl.BeginOfStream || ctrl.EndOfStream {
					t.Fatal("wire metadata conversion changed the caller's PCM chunk")
				}
				if len(writer.ch) != 0 {
					t.Fatal("partial PCM frame was encoded before it was complete")
				}
				end := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "pcm", EndOfStream: true}}
				if terminal == "pcm" {
					end.Part = &genx.Blob{MIMEType: mimeType}
				}
				go func() { pushed <- s.Push(ctx, end) }()
				eos, err := ReadPeerStreamEvent(server)
				if err != nil || eos.Type != eventpb.PeerEventType_PEER_EVENT_TYPE_EOS || eos.StreamID() != "pcm" {
					t.Fatalf("wire EOS=%+v,%v", eos, err)
				}
				if terminal == "whole-route" && (end.Part != nil || eos.GetEos().GetMimeType() != "" || eos.StreamKindValue() != eventpb.StreamKind_STREAM_KIND_UNSPECIFIED) {
					t.Fatal("whole-route EOS acquired a MIME channel")
				}
				select {
				case packet := <-writer.ch:
					decoder, err := opus.NewDecoder(16000, 1)
					if err != nil {
						t.Fatal(err)
					}
					defer decoder.Close()
					if pcm, err := decoder.Decode(packet, 320, false); err != nil || len(pcm) != 320 {
						t.Fatalf("encoded PCM tail=%d,%v", len(pcm), err)
					}
				default:
					t.Fatal("EOS overtook the final encoded PCM frame")
				}
				if err := <-pushed; err != nil {
					t.Fatal(err)
				}
				if err := s.Push(ctx, &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}, Ctrl: &genx.StreamCtrl{StreamID: "pcm"}}); err == nil {
					t.Fatal("encoded original was mixed into the PCM encoder epoch")
				}
			})
		}
	}
}

type peerStreamTestEvents struct{ bytes.Buffer }

func (*peerStreamTestEvents) Close() error { return nil }

func TestPeerStreamPCMControlEOSOnlyFlushesMatchingRoute(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus")
	}
	events := &peerStreamTestEvents{}
	writer := &recordingPeerPacketWriter{ch: make(chan []byte, 2)}
	ready := &peerAudioInputReady{streamID: "pcm", done: make(chan struct{}), completed: true}
	close(ready.done)
	s := &PeerStream{events: events, conn: writer, done: make(chan struct{}), inputReady: ready, sendPCM: true}
	defer s.Close()
	if err := s.Push(t.Context(), &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 320)}, Ctrl: &genx.StreamCtrl{StreamID: "pcm"}}); err != nil {
		t.Fatal(err)
	}
	encoder := s.pcmInput
	for _, end := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "unrelated", EndOfStream: true}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "pcm", EndOfStream: true}},
		{Part: &genx.Blob{MIMEType: "video/h264"}, Ctrl: &genx.StreamCtrl{StreamID: "pcm", EndOfStream: true}},
	} {
		if err := s.Push(t.Context(), end); err != nil {
			t.Fatal(err)
		}
		event, err := ReadPeerStreamEvent(events)
		if err != nil || event.StreamID() != end.Ctrl.StreamID {
			t.Fatalf("unrelated EOS route changed: %+v, %v", event, err)
		}
		if len(writer.ch) != 0 || s.pcmInput != encoder {
			t.Fatal("unrelated route or MIME EOS changed the active PCM encoder")
		}
	}
	end := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "pcm", Label: "mic", EndOfStream: true}}
	if err := s.Push(t.Context(), end); err != nil {
		t.Fatal(err)
	}
	event, err := ReadPeerStreamEvent(events)
	if err != nil || event.Type != eventpb.PeerEventType_PEER_EVENT_TYPE_EOS || event.StreamID() != "pcm" || event.Label() != "mic" || event.GetEos().GetMimeType() != "" || end.Part != nil {
		t.Fatalf("whole-route EOS identity changed: %+v, %v", event, err)
	}
	if len(writer.ch) != 1 {
		t.Fatalf("matching whole-route EOS sent %d tail packets, want 1", len(writer.ch))
	}
	if err := s.Push(t.Context(), end); err != nil {
		t.Fatal(err)
	}
	if len(writer.ch) != 1 {
		t.Fatal("repeated EOS duplicated the PCM tail")
	}
}

func TestPeerInputPCMFormat(t *testing.T) {
	for _, suffix := range []string{"", ";bits=16", ";format=s16le", ";format=pcm_s16le", ";format=S16LE", ";format=PCM_S16LE", ";rate=16000;channels=1;bits=16;format=pcm_s16le"} {
		t.Run(suffix, func(t *testing.T) {
			pcm, err := peerInputPCM(&genx.Blob{MIMEType: "audio/pcm" + suffix})
			if err != nil || !pcm {
				t.Fatalf("valid PCM rejected: pcm=%t, %v", pcm, err)
			}
		})
	}
	if pcm, err := peerInputPCM(nil); err != nil || pcm {
		t.Fatalf("nil blob became PCM: %t, %v", pcm, err)
	}
	if pcm, err := peerInputPCM(&genx.Blob{MIMEType: "audio/opus"}); err != nil || pcm {
		t.Fatalf("Opus became PCM: %t, %v", pcm, err)
	}
}

func TestPeerStreamRejectsPCMFormatBeforeBOS(t *testing.T) {
	for _, tc := range []struct{ params, want string }{
		{"bits=32", "16 bits"},
		{"bits=8;format=s16le", "16 bits"},
		{"bits=32;format=pcm_s16le", "16 bits"},
		{"bits=16;format=f32le", "s16le"},
		{"format=F32LE", "s16le"},
		{"format=s16be", "s16le"},
		{"rate=48000", "16000 Hz"},
		{"channels=2", "mono"},
	} {
		t.Run(tc.params, func(t *testing.T) {
			events := &peerStreamTestEvents{}
			writer := &recordingPeerPacketWriter{ch: make(chan []byte, 1)}
			s := &PeerStream{events: events, conn: writer, done: make(chan struct{})}
			defer s.Close()
			chunk := &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm;" + tc.params, Data: make([]byte, 640)}, Ctrl: &genx.StreamCtrl{StreamID: "invalid", BeginOfStream: true}}
			if err := s.Push(t.Context(), chunk); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("invalid PCM error=%v, want %q", err, tc.want)
			}
			if events.Len() != 0 || len(writer.ch) != 0 || s.inputReady != nil || s.pcmInput != nil {
				t.Fatal("invalid PCM established a BOS or touched the media sender")
			}
		})
	}
}

type blockingAudioWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w *blockingAudioWriter) Write(_ byte, data []byte) (int, error) {
	close(w.entered)
	<-w.release
	return len(data), nil
}

func TestPeerStreamCloseDefersNativeCleanupUntilSenderExits(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus")
	}
	writer := &blockingAudioWriter{entered: make(chan struct{}), release: make(chan struct{})}
	ready := &peerAudioInputReady{streamID: "pcm", done: make(chan struct{}), completed: true}
	close(ready.done)
	s := &PeerStream{conn: writer, done: make(chan struct{}), inputReady: ready, sendPCM: true}
	finished := make(chan error, 1)
	go func() {
		finished <- s.Push(t.Context(), &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 640)}, Ctrl: &genx.StreamCtrl{StreamID: "pcm"}})
	}()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		t.Fatal("sender did not reach transport")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// The send operation still owns its encoder until the transport returns.
	if s.pcmInput == nil {
		t.Fatal("Close freed an admitted sender's native state")
	}
	close(writer.release)
	select {
	case err := <-finished:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("closed sender=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("sender leaked after transport returned")
	}
	if s.pcmInput != nil {
		t.Fatal("last sender did not release its native state")
	}
}
