package peer

import (
	"encoding/json"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func convertViaJSON[T any](in any) (T, error) {
	var out T
	data, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func toAdminRegistrationList(items []apitypes.Peer, hasNext bool, nextCursor *string) adminhttp.RegistrationList {
	out := make([]apitypes.Registration, 0, len(items))
	for _, item := range items {
		out = append(out, toAdminRegistration(item))
	}
	return adminhttp.RegistrationList{
		HasNext:    hasNext,
		Items:      out,
		NextCursor: nextCursor,
	}
}

func toAdminRegistration(peer apitypes.Peer) apitypes.Registration {
	return apitypes.Registration{
		ApprovedAt:     peer.ApprovedAt,
		AutoRegistered: peer.AutoRegistered,
		CreatedAt:      peer.CreatedAt,
		Device:         &peer.Device,
		PublicKey:      peer.PublicKey,
		Role:           apitypes.PeerRole(peer.Role),
		Status:         apitypes.PeerRegistrationStatus(peer.Status),
		UpdatedAt:      peer.UpdatedAt,
	}
}

func toAdminRuntime(in apitypes.Runtime) apitypes.Runtime {
	return apitypes.Runtime{
		LastAddr:   in.LastAddr,
		LastSeenAt: in.LastSeenAt,
		Online:     in.Online,
		RxBytes:    in.RxBytes,
		TxBytes:    in.TxBytes,
	}
}

func toPeerDeviceInfo(in apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	return convertViaJSON[apitypes.DeviceInfo](in)
}

func toAdminDeviceInfo(in apitypes.DeviceInfo) (apitypes.DeviceInfo, error) {
	return convertViaJSON[apitypes.DeviceInfo](in)
}
