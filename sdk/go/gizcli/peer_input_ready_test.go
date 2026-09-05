package gizcli

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

func TestPeerStreamInputReadyFailure(t *testing.T) {
	for _, mode := range []string{"cancel", "closed", "remote_closed", "denied"} {
		t.Run(mode, func(t *testing.T) {
			clientSide, serverSide := net.Pipe()
			defer serverSide.Close()
			writer := &recordingPeerPacketWriter{ch: make(chan []byte, 1)}
			stream := &PeerStream{events: clientSide, conn: writer, eventResults: make(chan peerStreamEventResult, 2), done: make(chan struct{})}
			defer stream.Close()
			go stream.readEvents()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- stream.Push(ctx, &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}}, Ctrl: &genx.StreamCtrl{StreamID: "waiting", BeginOfStream: true}})
			}()
			if _, err := ReadPeerStreamEvent(serverSide); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "cancel":
				cancel()
			case "closed":
				_ = stream.Close()
			case "remote_closed":
				_ = serverSide.Close()
			case "denied":
				if err := WritePeerStreamEvent(serverSide, eosEvent("waiting", "assistant", "audio/opus", &eventpb.EventError{Code: "MEMBER_REMOVED", Message: "removed"})); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("unacknowledged audio succeeded")
				}
				if mode == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatalf("lost cancellation: %v", err)
				}
				if mode == "denied" && !strings.Contains(err.Error(), "MEMBER_REMOVED") {
					t.Fatalf("lost denial: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("input acknowledgement did not unblock")
			}
			select {
			case <-writer.ch:
				t.Fatal("audio sent without acknowledgement")
			default:
			}
		})
	}
}
