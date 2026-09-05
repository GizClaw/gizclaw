package peertelemetry

import (
	"context"
	"fmt"
	"maps"
	"regexp"
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

// firmwareSha256Pattern mirrors the firmware_sha256 pattern of the PeerStatus
// schema in api/http/shared/peer_status.json.
var firmwareSha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

const (
	telemetryStatusDetailsKey          = "telemetry_status"
	telemetryStatusBatteryPercentAtKey = "battery_percent_at_unix_ms"
	telemetryStatusChargingAtKey       = "charging_at_unix_ms"
	telemetryStatusGNSSLatitudeAtKey   = "gnss_latitude_at_unix_ms"
	telemetryStatusGNSSLongitudeAtKey  = "gnss_longitude_at_unix_ms"
	telemetryStatusGNSSAltitudeMAtKey  = "gnss_altitude_m_at_unix_ms"
	telemetryStatusGNSSAccuracyMAtKey  = "gnss_accuracy_m_at_unix_ms"
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
	if !applyTelemetryStatusPatch(&status, patch) {
		return nil
	}
	_, err = s.Store.PutStatus(ctx, peer, status)
	return err
}

// ApplyDeviceStatus merges a PeerStatus that the device reported in response
// to a Server-initiated control command into the stored owner-scoped status.
//
// Volume, muted, labels, and the running firmware digest come from the control
// response verbatim because no telemetry frame carries them. Battery and GNSS
// fields use the same per-field observation ordering as telemetry reports, so
// an older control response never overwrites a newer telemetry observation. The reported time
// defaults to now when the device omits it; the stored reported_at never moves
// backwards.
func (s StatusSync) ApplyDeviceStatus(ctx context.Context, peer giznet.PublicKey, reported apitypes.PeerStatus, now time.Time) (apitypes.PeerStatus, error) {
	if s.Store == nil {
		return apitypes.PeerStatus{}, ErrStatusServiceNil
	}
	reportedAt := now.UTC()
	if reported.ReportedAt != nil && !reported.ReportedAt.IsZero() {
		reportedAt = reported.ReportedAt.UTC()
	}
	status, err := s.Store.GetStatus(ctx, peer)
	if err != nil {
		return apitypes.PeerStatus{}, err
	}
	patch := StatusPatch{ReportedAt: reportedAt, BatteryPercent: reported.BatteryPercent, Charging: reported.Charging}
	if reported.GnssLatitude != nil {
		patch.GNSSLatitude = new(float64(*reported.GnssLatitude))
	}
	if reported.GnssLongitude != nil {
		patch.GNSSLongitude = new(float64(*reported.GnssLongitude))
	}
	if reported.GnssAltitudeM != nil {
		patch.GNSSAltitudeM = new(float64(*reported.GnssAltitudeM))
	}
	if reported.GnssAccuracyM != nil {
		patch.GNSSAccuracyM = new(float64(*reported.GnssAccuracyM))
	}
	changed := applyTelemetryStatusPatch(&status, patch)
	if reported.Volume != nil {
		value := *reported.Volume
		status.Volume = &value
		changed = true
	}
	if reported.Muted != nil {
		value := *reported.Muted
		status.Muted = &value
		changed = true
	}
	if reported.Labels != nil {
		labels := maps.Clone(*reported.Labels)
		status.Labels = &labels
		changed = true
	}
	// The digest comes from the device, so it is untrusted: a value outside the
	// PeerStatus contract is dropped instead of stored, and the previously
	// stored digest is left in place rather than replaced with a bad one.
	if reported.FirmwareSha256 != nil && firmwareSha256Pattern.MatchString(*reported.FirmwareSha256) {
		value := *reported.FirmwareSha256
		status.FirmwareSha256 = &value
		changed = true
	}
	if !changed {
		return status, nil
	}
	return s.Store.PutStatus(ctx, peer, status)
}

func applyTelemetryStatusPatch(status *apitypes.PeerStatus, patch StatusPatch) bool {
	changed := false
	if !patch.ReportedAt.IsZero() {
		reportedAt := patch.ReportedAt.UTC()
		if status.ReportedAt == nil || reportedAt.After(status.ReportedAt.UTC()) {
			status.ReportedAt = &reportedAt
			changed = true
		}
	}
	if patch.BatteryPercent != nil && shouldApplyTelemetryStatusField(*status, telemetryStatusBatteryPercentAtKey, status.BatteryPercent == nil, patch.BatteryPercentAt, patch.ReportedAt) {
		value := *patch.BatteryPercent
		status.BatteryPercent = &value
		setTelemetryStatusFieldTime(status, telemetryStatusBatteryPercentAtKey, patch.BatteryPercentAt, patch.ReportedAt)
		changed = true
	}
	if patch.Charging != nil && shouldApplyTelemetryStatusField(*status, telemetryStatusChargingAtKey, status.Charging == nil, patch.ChargingAt, patch.ReportedAt) {
		value := *patch.Charging
		status.Charging = &value
		setTelemetryStatusFieldTime(status, telemetryStatusChargingAtKey, patch.ChargingAt, patch.ReportedAt)
		changed = true
	}
	if patch.GNSSLatitude != nil && shouldApplyTelemetryStatusField(*status, telemetryStatusGNSSLatitudeAtKey, status.GnssLatitude == nil, patch.GNSSLatitudeAt, patch.ReportedAt) {
		value := float32(*patch.GNSSLatitude)
		status.GnssLatitude = &value
		setTelemetryStatusFieldTime(status, telemetryStatusGNSSLatitudeAtKey, patch.GNSSLatitudeAt, patch.ReportedAt)
		changed = true
	}
	if patch.GNSSLongitude != nil && shouldApplyTelemetryStatusField(*status, telemetryStatusGNSSLongitudeAtKey, status.GnssLongitude == nil, patch.GNSSLongitudeAt, patch.ReportedAt) {
		value := float32(*patch.GNSSLongitude)
		status.GnssLongitude = &value
		setTelemetryStatusFieldTime(status, telemetryStatusGNSSLongitudeAtKey, patch.GNSSLongitudeAt, patch.ReportedAt)
		changed = true
	}
	if patch.GNSSAltitudeM != nil && shouldApplyTelemetryStatusField(*status, telemetryStatusGNSSAltitudeMAtKey, status.GnssAltitudeM == nil, patch.GNSSAltitudeMAt, patch.ReportedAt) {
		value := float32(*patch.GNSSAltitudeM)
		status.GnssAltitudeM = &value
		setTelemetryStatusFieldTime(status, telemetryStatusGNSSAltitudeMAtKey, patch.GNSSAltitudeMAt, patch.ReportedAt)
		changed = true
	}
	if patch.GNSSAccuracyM != nil && shouldApplyTelemetryStatusField(*status, telemetryStatusGNSSAccuracyMAtKey, status.GnssAccuracyM == nil, patch.GNSSAccuracyMAt, patch.ReportedAt) {
		value := float32(*patch.GNSSAccuracyM)
		status.GnssAccuracyM = &value
		setTelemetryStatusFieldTime(status, telemetryStatusGNSSAccuracyMAtKey, patch.GNSSAccuracyMAt, patch.ReportedAt)
		changed = true
	}
	return changed
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
	fields, ok := raw.(map[string]any)
	if !ok {
		return time.Time{}, false
	}
	unixMS, ok := telemetryStatusUnixMS(fields[fieldKey])
	if !ok || unixMS <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(unixMS).UTC(), true
}

func telemetryStatusUnixMS(value any) (int64, bool) {
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
	details := map[string]any{}
	if status.Details != nil && *status.Details != nil {
		maps.Copy(details, *status.Details)
	}
	fields := map[string]any{}
	if raw, ok := details[telemetryStatusDetailsKey].(map[string]any); ok {
		maps.Copy(fields, raw)
	}
	fields[fieldKey] = fmt.Sprintf("%d", fieldAt.UTC().UnixMilli())
	details[telemetryStatusDetailsKey] = fields
	status.Details = &details
}
