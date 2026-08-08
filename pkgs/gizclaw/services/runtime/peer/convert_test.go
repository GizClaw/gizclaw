package peer

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestConvertHelpers(t *testing.T) {
	now := time.Unix(1_700_600_000, 0).UTC()
	autoRegistered := true
	deviceName := "convert-device"
	publicKey := giznet.PublicKey{1}
	peer := apitypes.Peer{
		PublicKey:      publicKey.String(),
		Role:           apitypes.PeerRoleServer,
		Status:         apitypes.PeerRegistrationStatusActive,
		AutoRegistered: &autoRegistered,
		CreatedAt:      now,
		UpdatedAt:      now,
		Device: apitypes.DeviceInfo{
			Name: &deviceName,
		},
	}

	adminRegistrations := toAdminRegistrationList([]apitypes.Peer{peer}, false, nil)
	if len(adminRegistrations.Items) != 1 {
		t.Fatalf("toAdminRegistrationList = %+v", adminRegistrations)
	}
	item, err := adminRegistrations.Items[0].AsExternalRef0Registration()
	if err != nil || item.PublicKey != peer.PublicKey {
		t.Fatalf("toAdminRegistrationList item = %+v, %v", item, err)
	}
	if item.Device == nil || item.Device.Name == nil || *item.Device.Name != deviceName {
		t.Fatalf("toAdminRegistrationList device = %+v", item.Device)
	}

	convertedDevice, err := toPeerDeviceInfo(peer.Device)
	if err != nil {
		t.Fatalf("toPeerDeviceInfo error: %v", err)
	}
	if convertedDevice.Name == nil || *convertedDevice.Name != *peer.Device.Name {
		t.Fatalf("toPeerDeviceInfo = %+v", convertedDevice)
	}

	adminDevice, err := toAdminDeviceInfo(apitypes.DeviceInfo{
		Name:        peer.Device.Name,
		Identifiers: peer.Device.Identifiers,
	})
	if err != nil {
		t.Fatalf("toAdminDeviceInfo error: %v", err)
	}
	if adminDevice.Name == nil || *adminDevice.Name != *peer.Device.Name {
		t.Fatalf("toAdminDeviceInfo = %+v", adminDevice)
	}

	rxBytes := uint64(123)
	txBytes := uint64(456)
	adminRuntime := toAdminRuntime(apitypes.Runtime{Online: true, LastSeenAt: now, RxBytes: &rxBytes, TxBytes: &txBytes})
	if !adminRuntime.Online || !adminRuntime.LastSeenAt.Equal(now) || adminRuntime.RxBytes == nil || *adminRuntime.RxBytes != rxBytes || adminRuntime.TxBytes == nil || *adminRuntime.TxBytes != txBytes {
		t.Fatalf("toAdminRuntime = %+v", adminRuntime)
	}
}
