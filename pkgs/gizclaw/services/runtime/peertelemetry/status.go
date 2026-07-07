package peertelemetry

import (
	"context"
	"fmt"
	"strconv"
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

const (
	telemetryStatusDetailsKey          = "telemetry_status"
	telemetryStatusBatteryPercentAtKey = "battery_percent_at_unix_ms"
	telemetryStatusChargingAtKey       = "charging_at_unix_ms"
)

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
	changed := false
	if !patch.ReportedAt.IsZero() {
		reportedAt := patch.ReportedAt.UTC()
		if status.ReportedAt == nil || reportedAt.After(status.ReportedAt.UTC()) {
			status.ReportedAt = &reportedAt
			changed = true
		}
	}
	if patch.BatteryPercent != nil && shouldApplyTelemetryStatusField(status, telemetryStatusBatteryPercentAtKey, status.BatteryPercent == nil, patch.BatteryPercentAt, patch.ReportedAt) {
		value := *patch.BatteryPercent
		status.BatteryPercent = &value
		setTelemetryStatusFieldTime(&status, telemetryStatusBatteryPercentAtKey, patch.BatteryPercentAt, patch.ReportedAt)
		changed = true
	}
	if patch.Charging != nil && shouldApplyTelemetryStatusField(status, telemetryStatusChargingAtKey, status.Charging == nil, patch.ChargingAt, patch.ReportedAt) {
		value := *patch.Charging
		status.Charging = &value
		setTelemetryStatusFieldTime(&status, telemetryStatusChargingAtKey, patch.ChargingAt, patch.ReportedAt)
		changed = true
	}
	if !changed {
		return nil
	}
	_, err = s.Store.PutStatus(ctx, peer, status)
	return err
}

func shouldApplyTelemetryStatusField(status apitypes.PeerStatus, fieldKey string, currentMissing bool, fieldAt time.Time, fallback time.Time) bool {
	currentAt, ok := telemetryStatusFieldTime(status, fieldKey)
	if !ok {
		if status.ReportedAt == nil || status.ReportedAt.IsZero() {
			return true
		}
		currentAt = status.ReportedAt.UTC()
	}
	if fieldAt.IsZero() {
		fieldAt = fallback
	}
	if fieldAt.IsZero() {
		return currentMissing
	}
	if fieldAt.UTC().Before(currentAt) {
		return currentMissing
	}
	return true
}

func telemetryStatusFieldTime(status apitypes.PeerStatus, fieldKey string) (time.Time, bool) {
	if status.Details == nil || *status.Details == nil {
		return time.Time{}, false
	}
	raw := (*status.Details)[telemetryStatusDetailsKey]
	fields, ok := raw.(map[string]interface{})
	if !ok {
		return time.Time{}, false
	}
	unixMS, ok := telemetryStatusUnixMS(fields[fieldKey])
	if !ok || unixMS <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(unixMS).UTC(), true
}

func telemetryStatusUnixMS(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func setTelemetryStatusFieldTime(status *apitypes.PeerStatus, fieldKey string, fieldAt time.Time, fallback time.Time) {
	if status == nil {
		return
	}
	if fieldAt.IsZero() {
		fieldAt = fallback
	}
	if fieldAt.IsZero() {
		return
	}
	details := map[string]interface{}{}
	if status.Details != nil && *status.Details != nil {
		for k, v := range *status.Details {
			details[k] = v
		}
	}
	fields := map[string]interface{}{}
	if raw, ok := details[telemetryStatusDetailsKey].(map[string]interface{}); ok {
		for k, v := range raw {
			fields[k] = v
		}
	}
	fields[fieldKey] = fmt.Sprintf("%d", fieldAt.UTC().UnixMilli())
	details[telemetryStatusDetailsKey] = fields
	status.Details = &details
}
