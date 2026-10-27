package giztestcmd

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// pushRealtimeSilence keeps a finite recording's microphone open until the
// response completes. It preserves its route and emits no new BOS or EOS.
func pushRealtimeSilence(ctx context.Context, stream peerStream, streamID, label string, packet []byte, sent *atomic.Int64) error {
	for {
		if err := waitPeerInput(ctx, 20*time.Millisecond); err != nil {
			return err
		}
		if err := stream.Push(ctx, &genx.MessageChunk{
			Role: genx.RoleUser,
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label},
		}); err != nil {
			return err
		}
		sent.Add(1)
	}
}
