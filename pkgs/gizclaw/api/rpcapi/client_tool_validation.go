package rpcapi

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

var clientToolFirmwareDigest = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

// ValidateClientToolRequest validates the shared device procedure contract.
// Product-specific limits and hardware preconditions remain the provider's responsibility.
func ValidateClientToolRequest(message proto.Message) error {
	if message == nil {
		return fmt.Errorf("rpc: missing tool arguments")
	}
	invalid := fmt.Errorf("rpc: invalid tool arguments")
	text := func(value string, max int) bool {
		return value != "" && len(value) <= max && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
	}
	switch request := message.(type) {
	case *rpcpb.ClientDeviceSoundPlayRequest:
		if !text(request.Sound, 32) || request.GetDurationMs() < 0 {
			return invalid
		}
	case *rpcpb.ClientDeviceFindRequest:
		if request.GetDurationMs() < 0 {
			return invalid
		}
	case *rpcpb.ClientDeviceRebootRequest:
		if request.GetDelayMs() < 0 {
			return invalid
		}
	case *rpcpb.ClientWifiConnectRequest:
		if !text(request.Ssid, 32) || (request.Passphrase != nil && (len(*request.Passphrase) < 8 || !text(*request.Passphrase, 63))) {
			return invalid
		}
	case *rpcpb.ClientWifiSavedForgetRequest:
		if !text(request.Ssid, 32) {
			return invalid
		}
	case *rpcpb.ClientWifiScanRequest:
		if request.TimeoutMs != nil && (*request.TimeoutMs < 1000 || *request.TimeoutMs > 15000) {
			return invalid
		}
	case *rpcpb.ClientRunWorkspaceSetRequest:
		if !text(request.WorkspaceName, 256) {
			return invalid
		}
	case *rpcpb.ClientFirmwareUpdateRequest:
		if request.Sha256 != nil && !clientToolFirmwareDigest.MatchString(*request.Sha256) {
			return invalid
		}
		if request.Channel != nil && (*request.Channel < rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_STABLE || *request.Channel > rpcpb.FirmwareChannelName_FIRMWARE_CHANNEL_NAME_DEVELOP) {
			return invalid
		}
	}
	if strings.HasPrefix(string(message.ProtoReflect().Descriptor().Name()), "ClientDeviceAudioPlayer") {
		return ValidateAudioPlayerRequest(message)
	}
	return nil
}
