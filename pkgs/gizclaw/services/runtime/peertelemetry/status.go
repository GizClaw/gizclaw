package peertelemetry

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type PeerStatusStore interface {
	GetStatus(context.Context, giznet.PublicKey) (apitypes.PeerStatus, error)
	PutStatus(context.Context, giznet.PublicKey, apitypes.PeerStatus) (apitypes.PeerStatus, error)
}

type otaStatusStore interface {
	PutOTAStatus(context.Context, giznet.PublicKey, apitypes.PeerOtaStatus) error
}

type StatusSync struct {
	Store PeerStatusStore
}

// firmwareSha256Pattern mirrors the firmware_sha256 pattern of the PeerStatus
// schema in api/http/shared/peer_status.json.
var firmwareSha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// observedAtSelector addresses one member of PeerStatus.telemetry_observed_at.
// The per-field observation times are typed members rather than an untyped
// map, so each field is reached through its own selector instead of a string
// key that only the writer understood.
type observedAtSelector func(*apitypes.PeerStatusTelemetryObservedAt) **time.Time

var (
	observedAtBatteryPercent observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.BatteryPercent }
	observedAtCharging       observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.Charging }
	observedAtGNSSLatitude   observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.GnssLatitude }
	observedAtGNSSLongitude  observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.GnssLongitude }
	observedAtGNSSAltitudeM  observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.GnssAltitudeM }
	observedAtGNSSAccuracyM  observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.GnssAccuracyM }
	observedAtNetworkIMEI    observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.NetworkImei }
	observedAtNetworkIMSI    observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.NetworkImsi }
	observedAtActivity       observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.Activity }
	observedAtFirmwareVer    observedAtSelector = func(o *apitypes.PeerStatusTelemetryObservedAt) **time.Time { return &o.FirmwareVersion }
)

// activityPattern, activityDetailMaxLen and firmwareVersionMaxLen mirror the
// activity, activity_detail and firmware_version constraints of the PeerStatus
// schema in api/http/shared/peer_status.json.
var activityPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,31}$`)

const (
	activityDetailMaxLen  = 128
	firmwareVersionMaxLen = 128
)

func (s StatusSync) SyncTelemetryStatus(ctx context.Context, peer giznet.PublicKey, patch StatusPatch) error {
	if patch.Empty() {
		return nil
	}
	if s.Store == nil {
		return ErrStatusServiceNil
	}
	if len(patch.OTA) > 0 {
		store, ok := s.Store.(otaStatusStore)
		if !ok {
			return fmt.Errorf("peertelemetry: runtime store does not support ota status")
		}
		for _, ota := range patch.OTA {
			if err := store.PutOTAStatus(ctx, peer, ota); err != nil {
				return err
			}
		}
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
	if reported.Activity != nil && activityPattern.MatchString(*reported.Activity) {
		activity := *reported.Activity
		patch.Activity = &activity
		if reported.ActivityDetail != nil && len(*reported.ActivityDetail) <= activityDetailMaxLen {
			detail := *reported.ActivityDetail
			patch.ActivityDetail = &detail
		}
	}
	if reported.Audioplayer != nil {
		player := *reported.Audioplayer
		if player.ObservedAtUnixMs == 0 {
			player.ObservedAtUnixMs = reportedAt.UnixMilli()
		}
		if validAudioPlayerSnapshot(player) {
			patch.AudioPlayer = &player
		}
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
	if reported.FirmwareVersion != nil && len(*reported.FirmwareVersion) <= firmwareVersionMaxLen {
		value := *reported.FirmwareVersion
		status.FirmwareVersion = &value
		changed = true
	}
	if !changed {
		return status, nil
	}
	return s.Store.PutStatus(ctx, peer, status)
}

func applyTelemetryStatusPatch(status *apitypes.PeerStatus, patch StatusPatch) bool {
	changed := false
	if patch.AudioPlayer != nil && (status.Audioplayer == nil || patch.AudioPlayer.ObservedAtUnixMs >= status.Audioplayer.ObservedAtUnixMs) {
		value := *patch.AudioPlayer
		status.Audioplayer = &value
		changed = true
	}

	if !patch.ReportedAt.IsZero() {
		reportedAt := patch.ReportedAt.UTC()
		if status.ReportedAt == nil || reportedAt.After(status.ReportedAt.UTC()) {
			status.ReportedAt = &reportedAt
			changed = true
		}
	}
	if patch.BatteryPercent != nil && shouldApplyTelemetryStatusField(*status, observedAtBatteryPercent, status.BatteryPercent == nil, patch.BatteryPercentAt, patch.ReportedAt) {
		value := *patch.BatteryPercent
		status.BatteryPercent = &value
		setTelemetryStatusFieldTime(status, observedAtBatteryPercent, patch.BatteryPercentAt, patch.ReportedAt)
		changed = true
	}
	if patch.Charging != nil && shouldApplyTelemetryStatusField(*status, observedAtCharging, status.Charging == nil, patch.ChargingAt, patch.ReportedAt) {
		value := *patch.Charging
		status.Charging = &value
		setTelemetryStatusFieldTime(status, observedAtCharging, patch.ChargingAt, patch.ReportedAt)
		changed = true
	}
	if patch.GNSSLatitude != nil && shouldApplyTelemetryStatusField(*status, observedAtGNSSLatitude, status.GnssLatitude == nil, patch.GNSSLatitudeAt, patch.ReportedAt) {
		value := float32(*patch.GNSSLatitude)
		status.GnssLatitude = &value
		setTelemetryStatusFieldTime(status, observedAtGNSSLatitude, patch.GNSSLatitudeAt, patch.ReportedAt)
		changed = true
	}
	if patch.GNSSLongitude != nil && shouldApplyTelemetryStatusField(*status, observedAtGNSSLongitude, status.GnssLongitude == nil, patch.GNSSLongitudeAt, patch.ReportedAt) {
		value := float32(*patch.GNSSLongitude)
		status.GnssLongitude = &value
		setTelemetryStatusFieldTime(status, observedAtGNSSLongitude, patch.GNSSLongitudeAt, patch.ReportedAt)
		changed = true
	}
	if patch.GNSSAltitudeM != nil && shouldApplyTelemetryStatusField(*status, observedAtGNSSAltitudeM, status.GnssAltitudeM == nil, patch.GNSSAltitudeMAt, patch.ReportedAt) {
		value := float32(*patch.GNSSAltitudeM)
		status.GnssAltitudeM = &value
		setTelemetryStatusFieldTime(status, observedAtGNSSAltitudeM, patch.GNSSAltitudeMAt, patch.ReportedAt)
		changed = true
	}
	if patch.GNSSAccuracyM != nil && shouldApplyTelemetryStatusField(*status, observedAtGNSSAccuracyM, status.GnssAccuracyM == nil, patch.GNSSAccuracyMAt, patch.ReportedAt) {
		value := float32(*patch.GNSSAccuracyM)
		status.GnssAccuracyM = &value
		setTelemetryStatusFieldTime(status, observedAtGNSSAccuracyM, patch.GNSSAccuracyMAt, patch.ReportedAt)
		changed = true
	}
	if applyTelemetryStatusString(status, &status.NetworkImei, observedAtNetworkIMEI, patch.NetworkIMEI, patch.NetworkIMEIAt, patch.ReportedAt) {
		changed = true
	}
	if applyTelemetryStatusString(status, &status.NetworkImsi, observedAtNetworkIMSI, patch.NetworkIMSI, patch.NetworkIMSIAt, patch.ReportedAt) {
		changed = true
	}
	if applyTelemetryStatusString(status, &status.FirmwareVersion, observedAtFirmwareVer, patch.FirmwareVersion, patch.FirmwareVersionAt, patch.ReportedAt) {
		changed = true
	}
	if applyTelemetryStatusActivity(status, patch) {
		changed = true
	}
	return changed
}

// applyTelemetryStatusActivity merges the reported activity and its optional
// detail as one unit: the detail describes the activity it arrived with, so an
// accepted observation always replaces both, and an observation with no detail
// clears a detail left over from the previous activity.
func applyTelemetryStatusActivity(status *apitypes.PeerStatus, patch StatusPatch) bool {
	if patch.Activity == nil {
		return false
	}
	if !shouldApplyTelemetryStatusField(*status, observedAtActivity, status.Activity == nil, patch.ActivityAt, patch.ReportedAt) {
		return false
	}
	at := patch.ActivityAt
	if at.IsZero() {
		at = patch.ReportedAt
	}
	changed := status.Activity == nil || *status.Activity != *patch.Activity ||
		!equalOptionalString(status.ActivityDetail, patch.ActivityDetail)
	if !changed && !at.IsZero() {
		storedAt, ok := telemetryStatusFieldTime(*status, observedAtActivity)
		changed = !ok || !storedAt.Equal(at.UTC().Truncate(time.Millisecond))
	}
	if !changed {
		return false
	}
	activity := *patch.Activity
	status.Activity = &activity
	status.ActivityDetail = nil
	if patch.ActivityDetail != nil {
		detail := *patch.ActivityDetail
		status.ActivityDetail = &detail
	}
	setTelemetryStatusFieldTime(status, observedAtActivity, patch.ActivityAt, patch.ReportedAt)
	return true
}

func equalOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// applyTelemetryStatusString merges one device-reported string field with the
// same per-field observation ordering as battery and GNSS. An observation that
// repeats the stored value only refreshes the field timestamp; when neither the
// value nor the timestamp moves, the status is reported unchanged so the store
// is not rewritten.
func applyTelemetryStatusString(status *apitypes.PeerStatus, current **string, sel observedAtSelector, value *string, fieldAt time.Time, fallback time.Time) bool {
	if value == nil || !shouldApplyTelemetryStatusField(*status, sel, *current == nil, fieldAt, fallback) {
		return false
	}
	at := fieldAt
	if at.IsZero() {
		at = fallback
	}
	changed := *current == nil || **current != *value
	if !changed && !at.IsZero() {
		storedAt, ok := telemetryStatusFieldTime(*status, sel)
		changed = !ok || !storedAt.Equal(at.UTC().Truncate(time.Millisecond))
	}
	if !changed {
		return false
	}
	next := *value
	*current = &next
	setTelemetryStatusFieldTime(status, sel, fieldAt, fallback)
	return true
}

func shouldApplyTelemetryStatusField(status apitypes.PeerStatus, sel observedAtSelector, currentMissing bool, fieldAt time.Time, fallback time.Time) bool {
	currentAt, ok := telemetryStatusFieldTime(status, sel)
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

func telemetryStatusFieldTime(status apitypes.PeerStatus, sel observedAtSelector) (time.Time, bool) {
	if status.TelemetryObservedAt == nil {
		return time.Time{}, false
	}
	at := *sel(status.TelemetryObservedAt)
	if at == nil || at.IsZero() {
		return time.Time{}, false
	}
	return at.UTC(), true
}

func setTelemetryStatusFieldTime(status *apitypes.PeerStatus, sel observedAtSelector, fieldAt time.Time, fallback time.Time) {
	if status == nil {
		return
	}
	if fieldAt.IsZero() {
		fieldAt = fallback
	}
	if fieldAt.IsZero() {
		return
	}
	if status.TelemetryObservedAt == nil {
		status.TelemetryObservedAt = &apitypes.PeerStatusTelemetryObservedAt{}
	} else {
		// Copy before mutating: the caller's status may share this pointer with
		// a value the store handed out.
		observed := *status.TelemetryObservedAt
		status.TelemetryObservedAt = &observed
	}
	at := fieldAt.UTC().Truncate(time.Millisecond)
	*sel(status.TelemetryObservedAt) = &at
}
