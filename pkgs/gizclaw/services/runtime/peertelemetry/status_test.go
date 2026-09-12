package peertelemetry

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type memoryStatusStore struct {
	status map[giznet.PublicKey]apitypes.PeerStatus
	puts   int
}

func (s *memoryStatusStore) GetStatus(_ context.Context, peer giznet.PublicKey) (apitypes.PeerStatus, error) {
	return s.status[peer], nil
}

func (s *memoryStatusStore) PutStatus(_ context.Context, peer giznet.PublicKey, status apitypes.PeerStatus) (apitypes.PeerStatus, error) {
	if s.status == nil {
		s.status = make(map[giznet.PublicKey]apitypes.PeerStatus)
	}
	s.status[peer] = status
	s.puts++
	return status, nil
}

func TestApplyDeviceStatusWritesControlResponseFields(t *testing.T) {
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{1}
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	reportedAt := now.Add(-time.Second)

	got, err := sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{
		Volume: new(40), Muted: new(true), BatteryPercent: new(77), ReportedAt: &reportedAt,
		Labels: &map[string]string{"zone": "kitchen"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Volume == nil || *got.Volume != 40 || got.Muted == nil || !*got.Muted || got.BatteryPercent == nil || *got.BatteryPercent != 77 {
		t.Fatalf("applied status = %+v", got)
	}
	if got.ReportedAt == nil || !got.ReportedAt.Equal(reportedAt) {
		t.Fatalf("reported_at = %v, want device time %v", got.ReportedAt, reportedAt)
	}
	if got.Labels == nil || (*got.Labels)["zone"] != "kitchen" {
		t.Fatalf("labels = %v", got.Labels)
	}
	if store.puts != 1 {
		t.Fatalf("puts = %d, want 1", store.puts)
	}

	// A device response without a timestamp uses the Server clock.
	later := now.Add(time.Minute)
	got, err = sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{Volume: new(12), Muted: new(false)}, later)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Volume != 12 || *got.Muted || !got.ReportedAt.Equal(later) || *got.BatteryPercent != 77 {
		t.Fatalf("second apply = %+v", got)
	}
}

func TestApplyDeviceStatusKeepsNewerTelemetryObservations(t *testing.T) {
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{2}
	base := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: base, BatteryPercent: new(90), BatteryPercentAt: base, GNSSLatitude: new(31.2), GNSSLatitudeAt: base,
	}); err != nil {
		t.Fatal(err)
	}

	stale := base.Add(-time.Minute)
	got, err := sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{
		Volume: new(55), BatteryPercent: new(10), GnssLatitude: new(float32(0)), ReportedAt: &stale,
	}, base.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if *got.Volume != 55 {
		t.Fatalf("volume = %d, want control response value", *got.Volume)
	}
	if *got.BatteryPercent != 90 || *got.GnssLatitude != float32(31.2) {
		t.Fatalf("stale control response overwrote telemetry: %+v", got)
	}
	if !got.ReportedAt.Equal(base) {
		t.Fatalf("reported_at moved backwards to %v", got.ReportedAt)
	}

	fresh := base.Add(time.Minute)
	got, err = sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{BatteryPercent: new(85), ReportedAt: &fresh}, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if *got.BatteryPercent != 85 || !got.ReportedAt.Equal(fresh) || *got.Volume != 55 {
		t.Fatalf("fresh control response = %+v", got)
	}

	// Telemetry reported after the control response still wins per field.
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: fresh.Add(time.Second), BatteryPercent: new(84), BatteryPercentAt: fresh.Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	stored := store.status[peer]
	if *stored.BatteryPercent != 84 || *stored.Volume != 55 {
		t.Fatalf("telemetry after control = %+v", stored)
	}
}

func TestApplyDeviceStatusRequiresStore(t *testing.T) {
	if _, err := (StatusSync{}).ApplyDeviceStatus(context.Background(), giznet.PublicKey{3}, apitypes.PeerStatus{}, time.Now()); err != ErrStatusServiceNil {
		t.Fatalf("error = %v, want ErrStatusServiceNil", err)
	}
	store := &memoryStatusStore{}
	if _, err := (StatusSync{Store: store}).ApplyDeviceStatus(context.Background(), giznet.PublicKey{3}, apitypes.PeerStatus{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if store.puts != 1 {
		t.Fatalf("empty control response puts = %d, want 1 (reported_at advances)", store.puts)
	}
}

func TestApplyDeviceStatusPreservesTelemetryObservedAtAndReplacesLabels(t *testing.T) {
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{4}
	base := time.Date(2026, 9, 3, 6, 0, 0, 0, time.UTC)
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{ReportedAt: base, BatteryPercent: new(70), BatteryPercentAt: base}); err != nil {
		t.Fatal(err)
	}
	before := store.status[peer]
	if _, ok := telemetryStatusFieldTime(before, observedAtBatteryPercent); !ok {
		t.Fatalf("telemetry field time missing before control write: %+v", before)
	}
	deviceClaimedAt := base.Add(72 * time.Hour)
	got, err := sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{
		Volume: new(3), Labels: &map[string]string{"room": "a"},
		TelemetryObservedAt: &apitypes.PeerStatusTelemetryObservedAt{BatteryPercent: &deviceClaimedAt},
	}, base.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if at, ok := telemetryStatusFieldTime(got, observedAtBatteryPercent); !ok || !at.Equal(base) {
		t.Fatalf("telemetry field time after control write = %v, %v", at, ok)
	}
	if at, ok := telemetryStatusFieldTime(got, observedAtBatteryPercent); !ok || at.Equal(deviceClaimedAt) {
		t.Fatal("device-reported telemetry_observed_at must not overwrite the stored observation time")
	}
	got, err = sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{Labels: &map[string]string{"room": "b"}}, base.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if (*got.Labels)["room"] != "b" || len(*got.Labels) != 1 || *got.Volume != 3 {
		t.Fatalf("labels replaced / volume kept = %+v", got)
	}
}

func TestApplyDeviceStatusRejectsMalformedFirmwareDigest(t *testing.T) {
	valid := "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{2}
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	got, err := sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{FirmwareSha256: &valid}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.FirmwareSha256 == nil || *got.FirmwareSha256 != valid {
		t.Fatalf("firmware_sha256 = %v, want the reported digest", got.FirmwareSha256)
	}

	// The digest is device-reported, so anything outside the PeerStatus
	// contract is dropped and the stored digest is left in place.
	for name, reported := range map[string]string{
		"empty":      "",
		"too short":  valid[:63],
		"too long":   valid + "0",
		"uppercase":  "A1B2C3D4E5F60718293A4B5C6D7E8F90A1B2C3D4E5F60718293A4B5C6D7E8F90",
		"not hex":    "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
		"free text":  "1.0.3",
		"whitespace": valid[:63] + " ",
	} {
		got, err := sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{FirmwareSha256: &reported}, now)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.FirmwareSha256 == nil || *got.FirmwareSha256 != valid {
			t.Fatalf("%s: firmware_sha256 = %v, want the last valid digest", name, got.FirmwareSha256)
		}
	}

	// A malformed digest reported before any valid one leaves the field unset.
	fresh := &memoryStatusStore{}
	bad := "1.0.3"
	got, err = (StatusSync{Store: fresh}).ApplyDeviceStatus(context.Background(), giznet.PublicKey{3}, apitypes.PeerStatus{FirmwareSha256: &bad}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.FirmwareSha256 != nil {
		t.Fatalf("firmware_sha256 = %v, want none", got.FirmwareSha256)
	}
}

func TestSyncTelemetryStatusNetworkIdentityOrdering(t *testing.T) {
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{5}
	base := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	imei := "490154203237518"
	imsi := "460001234567890"

	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: base, NetworkIMEI: &imei, NetworkIMEIAt: base, NetworkIMSI: &imsi, NetworkIMSIAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	got := store.status[peer]
	if got.NetworkImei == nil || *got.NetworkImei != imei || got.NetworkImsi == nil || *got.NetworkImsi != imsi {
		t.Fatalf("stored identity = %+v", got)
	}
	if at, ok := telemetryStatusFieldTime(got, observedAtNetworkIMSI); !ok || !at.Equal(base) {
		t.Fatalf("network_imsi_at = %v, %v", at, ok)
	}
	if store.puts != 1 {
		t.Fatalf("puts = %d, want 1", store.puts)
	}

	// An older observation never overwrites the newer stored value.
	stale := base.Add(-time.Minute)
	staleIMSI := "460009999999999"
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: stale, NetworkIMSI: &staleIMSI, NetworkIMSIAt: stale,
	}); err != nil {
		t.Fatal(err)
	}
	got = store.status[peer]
	if *got.NetworkImsi != imsi || store.puts != 1 {
		t.Fatalf("stale observation rewrote status: %+v puts=%d", got, store.puts)
	}

	// The same value at the same time is a no-op.
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: base, NetworkIMEI: &imei, NetworkIMEIAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	if store.puts != 1 {
		t.Fatalf("unchanged identity rewrote status: puts=%d", store.puts)
	}

	// The same value at a later time only refreshes the field timestamp.
	later := base.Add(time.Minute)
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: later, NetworkIMEI: &imei, NetworkIMEIAt: later,
	}); err != nil {
		t.Fatal(err)
	}
	got = store.status[peer]
	if *got.NetworkImei != imei || store.puts != 2 {
		t.Fatalf("equal identity refresh: %+v puts=%d", got, store.puts)
	}
	if at, ok := telemetryStatusFieldTime(got, observedAtNetworkIMEI); !ok || !at.Equal(later) {
		t.Fatalf("network_imei_at after refresh = %v, %v, want %v", at, ok, later)
	}
	if at, ok := telemetryStatusFieldTime(got, observedAtNetworkIMSI); !ok || !at.Equal(base) {
		t.Fatalf("network_imsi_at must stay %v, got %v, %v", base, at, ok)
	}
	if !got.ReportedAt.Equal(later) {
		t.Fatalf("reported_at = %v, want %v", got.ReportedAt, later)
	}

	// A SIM swap replaces the IMSI and leaves the IMEI in place; the identity
	// is never cleared by telemetry that omits it.
	swapped := "460001111111111"
	swapAt := later.Add(time.Minute)
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: swapAt, NetworkIMSI: &swapped, NetworkIMSIAt: swapAt,
	}); err != nil {
		t.Fatal(err)
	}
	got = store.status[peer]
	if *got.NetworkImsi != swapped || *got.NetworkImei != imei {
		t.Fatalf("sim swap = %+v", got)
	}
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: swapAt.Add(time.Minute), BatteryPercent: new(50), BatteryPercentAt: swapAt.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	got = store.status[peer]
	if got.NetworkImsi == nil || got.NetworkImei == nil {
		t.Fatalf("identity cleared by unrelated telemetry: %+v", got)
	}

	// Control responses never overwrite a newer telemetry identity.
	stale = swapAt.Add(-time.Second)
	if _, err := sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{Volume: new(9), ReportedAt: &stale}, swapAt.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got = store.status[peer]
	if *got.NetworkImsi != swapped || *got.Volume != 9 {
		t.Fatalf("control response = %+v", got)
	}
}

// An activity observation replaces the stored activity and its detail together,
// records its own observation time, and never lets a stale report win.
func TestSyncTelemetryStatusAppliesActivityWithDetailAndOrdering(t *testing.T) {
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{9}
	base := time.Date(2026, 9, 8, 4, 0, 0, 0, time.UTC)

	chat := "chat"
	detail := "Talking to the agent"
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: base, Activity: &chat, ActivityDetail: &detail, ActivityAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	got := store.status[peer]
	if got.Activity == nil || *got.Activity != chat {
		t.Fatalf("Activity = %#v, want %q", got.Activity, chat)
	}
	if got.ActivityDetail == nil || *got.ActivityDetail != detail {
		t.Fatalf("ActivityDetail = %#v, want %q", got.ActivityDetail, detail)
	}
	if at, ok := telemetryStatusFieldTime(got, observedAtActivity); !ok || !at.Equal(base) {
		t.Fatalf("activity observed at = %v, %v, want %s", at, ok, base)
	}

	// A newer activity with no detail clears the detail of the previous one.
	idle := "idle"
	later := base.Add(time.Minute)
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: later, Activity: &idle, ActivityAt: later,
	}); err != nil {
		t.Fatal(err)
	}
	got = store.status[peer]
	if got.Activity == nil || *got.Activity != idle {
		t.Fatalf("Activity = %#v, want %q", got.Activity, idle)
	}
	if got.ActivityDetail != nil {
		t.Fatalf("ActivityDetail = %#v, want cleared with the new activity", got.ActivityDetail)
	}

	// A report observed before the stored one never replaces it.
	store.puts = 0
	stale := "ota"
	staleAt := base.Add(-time.Hour)
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: staleAt, Activity: &stale, ActivityAt: staleAt,
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.status[peer]; got.Activity == nil || *got.Activity != idle {
		t.Fatalf("stale activity overwrote the stored one: %#v", got.Activity)
	}
	if store.puts != 0 {
		t.Fatalf("stale activity puts = %d, want 0", store.puts)
	}

	// Repeating the stored activity at the same time does not rewrite the store.
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: later, Activity: &idle, ActivityAt: later,
	}); err != nil {
		t.Fatal(err)
	}
	if store.puts != 0 {
		t.Fatalf("unchanged activity puts = %d, want 0", store.puts)
	}
}

// The firmware version is ordered per field like every other telemetry-sourced
// member, so a late-arriving older report cannot roll the version backwards.
func TestSyncTelemetryStatusOrdersFirmwareVersion(t *testing.T) {
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{10}
	base := time.Date(2026, 9, 8, 5, 0, 0, 0, time.UTC)

	newer := "1.4.2"
	newerAt := base.Add(time.Hour)
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: newerAt, FirmwareVersion: &newer, FirmwareVersionAt: newerAt,
	}); err != nil {
		t.Fatal(err)
	}
	older := "1.4.1"
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{
		ReportedAt: base, FirmwareVersion: &older, FirmwareVersionAt: base,
	}); err != nil {
		t.Fatal(err)
	}
	got := store.status[peer]
	if got.FirmwareVersion == nil || *got.FirmwareVersion != newer {
		t.Fatalf("FirmwareVersion = %#v, want preserved %q", got.FirmwareVersion, newer)
	}
	if at, ok := telemetryStatusFieldTime(got, observedAtFirmwareVer); !ok || !at.Equal(newerAt) {
		t.Fatalf("firmware version observed at = %v, %v, want %s", at, ok, newerAt)
	}
}

// The device supplies the activity on a control response too; an unparseable
// value is dropped rather than stored, like the firmware digest.
func TestApplyDeviceStatusValidatesReportedActivity(t *testing.T) {
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{11}
	now := time.Date(2026, 9, 8, 6, 0, 0, 0, time.UTC)

	bad := "Chat With Agent"
	got, err := sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{Activity: &bad}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Activity != nil {
		t.Fatalf("Activity = %#v, want the out-of-contract value dropped", got.Activity)
	}
	good := "audioplayer"
	got, err = sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{Activity: &good}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got.Activity == nil || *got.Activity != good {
		t.Fatalf("Activity = %#v, want %q", got.Activity, good)
	}
}
