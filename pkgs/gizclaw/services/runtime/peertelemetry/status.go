package peertelemetry

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type PeerStatusStore interface {
	GetStatus(context.Context, giznet.PublicKey) (apitypes.PeerStatus, error)
	PutStatus(context.Context, giznet.PublicKey, apitypes.PeerStatus) (apitypes.PeerStatus, error)
}

type StatusSync struct {
	Store PeerStatusStore
}

func (s StatusSync) SyncTelemetryStatus(ctx context.Context, peer giznet.PublicKey, patch StatusPatch) error {
	if patch.Empty() {
		return nil
	}
	if s.Store == nil {
		return ErrStatusServiceNil
	}
	status, err := s.Store.GetStatus(ctx, peer)
	if err != nil {
		return err
	}
	stale := false
	if !patch.ReportedAt.IsZero() {
		reportedAt := patch.ReportedAt.UTC()
		if status.ReportedAt != nil && reportedAt.Before(status.ReportedAt.UTC()) {
			stale = true
		} else {
			status.ReportedAt = &reportedAt
		}
	}
	changed := false
	if patch.BatteryPercent != nil && (!stale || status.BatteryPercent == nil) {
		value := *patch.BatteryPercent
		status.BatteryPercent = &value
		changed = true
	}
	if patch.Charging != nil && (!stale || status.Charging == nil) {
		value := *patch.Charging
		status.Charging = &value
		changed = true
	}
	if !patch.ReportedAt.IsZero() && !stale {
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = s.Store.PutStatus(ctx, peer, status)
	return err
}
