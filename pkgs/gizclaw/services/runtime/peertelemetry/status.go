package peertelemetry

import (
	"context"
	"time"

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
	currentReportedAt := status.ReportedAt
	changed := false
	if !patch.ReportedAt.IsZero() {
		reportedAt := patch.ReportedAt.UTC()
		if status.ReportedAt == nil || reportedAt.After(status.ReportedAt.UTC()) {
			status.ReportedAt = &reportedAt
			changed = true
		}
	}
	if patch.BatteryPercent != nil && shouldApplyTelemetryStatusField(currentReportedAt, status.BatteryPercent == nil, patch.BatteryPercentAt, patch.ReportedAt) {
		value := *patch.BatteryPercent
		status.BatteryPercent = &value
		changed = true
	}
	if patch.Charging != nil && shouldApplyTelemetryStatusField(currentReportedAt, status.Charging == nil, patch.ChargingAt, patch.ReportedAt) {
		value := *patch.Charging
		status.Charging = &value
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = s.Store.PutStatus(ctx, peer, status)
	return err
}

func shouldApplyTelemetryStatusField(currentReportedAt *time.Time, currentMissing bool, fieldAt time.Time, fallback time.Time) bool {
	if currentReportedAt == nil {
		return true
	}
	if fieldAt.IsZero() {
		fieldAt = fallback
	}
	if fieldAt.IsZero() {
		return currentMissing
	}
	if fieldAt.UTC().Before(currentReportedAt.UTC()) {
		return currentMissing
	}
	return true
}
