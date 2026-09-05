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

func TestApplyDeviceStatusPreservesTelemetryDetailsAndReplacesLabels(t *testing.T) {
	store := &memoryStatusStore{}
	sync := StatusSync{Store: store}
	peer := giznet.PublicKey{4}
	base := time.Date(2026, 9, 3, 6, 0, 0, 0, time.UTC)
	if err := sync.SyncTelemetryStatus(context.Background(), peer, StatusPatch{ReportedAt: base, BatteryPercent: new(70), BatteryPercentAt: base}); err != nil {
		t.Fatal(err)
	}
	before := store.status[peer]
	if _, ok := telemetryStatusFieldTime(before, telemetryStatusBatteryPercentAtKey); !ok {
		t.Fatalf("telemetry field time missing before control write: %+v", before)
	}
	got, err := sync.ApplyDeviceStatus(context.Background(), peer, apitypes.PeerStatus{
		Volume: new(3), Labels: &map[string]string{"room": "a"}, Details: &map[string]any{"device_only": true},
	}, base.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if at, ok := telemetryStatusFieldTime(got, telemetryStatusBatteryPercentAtKey); !ok || !at.Equal(base) {
		t.Fatalf("telemetry field time after control write = %v, %v", at, ok)
	}
	if _, leaked := (*got.Details)["device_only"]; leaked {
		t.Fatal("device details must not overwrite stored details")
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
