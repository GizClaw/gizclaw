package gizclaw

import (
	"context"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"time"
)

func (c *deviceController) applyAudioPlayer(ctx context.Context, owner giznet.PublicKey, wire *rpcpb.AudioPlayerStatus) error {
	if c.status == nil {
		return nil
	}
	at := time.UnixMilli(wire.ObservedAtUnixMs).UTC()
	status := peertelemetry.AudioPlayerStatus(wire)
	mu := c.manager.telemetryStatusLock(owner)
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	return (peertelemetry.StatusSync{Store: c.status}).SyncTelemetryStatus(ctx, owner, peertelemetry.StatusPatch{ReportedAt: at, AudioPlayer: &status})
}
