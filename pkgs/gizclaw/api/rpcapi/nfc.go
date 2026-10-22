package rpcapi

import (
	"fmt"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// ValidateNFCSnapshot accepts absence before the first attempt and preserves
// failed attempts, binary data and independent physical-instance state.
func ValidateNFCSnapshot(snapshot *rpcpb.NfcReadSnapshot) error {
	if snapshot == nil {
		return nil
	}
	valid := snapshot.ReadAtUnixMs > 0 && len(snapshot.Uid) <= 32 && len(snapshot.RawData) <= 1024
	for _, text := range []struct {
		value *string
		limit int
	}{{snapshot.Technology, 32}, {snapshot.DataFormat, 32}, {snapshot.ErrorCode, 64}} {
		valid = valid && (text.value == nil || managementText(*text.value, text.limit))
	}
	switch snapshot.Result {
	case "success":
		valid = valid && snapshot.ErrorCode == nil
	case "failed":
		valid = valid && snapshot.ErrorCode != nil
	default:
		valid = false
	}
	if !valid {
		return fmt.Errorf("rpc: invalid NFC snapshot")
	}
	return nil
}
