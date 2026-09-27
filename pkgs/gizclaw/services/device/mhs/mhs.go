// Package mhs validates GizClaw-owned MHS v0 hardware device instances.
package mhs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ValidateManifest checks instance identity and HWD names before persistence.
func ValidateManifest(manifest apitypes.MhsV0Manifest) error {
	if manifest.Devices == nil {
		return fmt.Errorf("devices must be an array")
	}
	seen := make(map[string]bool, len(manifest.Devices))
	for _, device := range manifest.Devices {
		if !rpcapi.ValidMhsKey(device.Id) {
			return fmt.Errorf("invalid HWD instance id %q", device.Id)
		}
		if seen[device.Id] {
			return fmt.Errorf("duplicate HWD instance id %q", device.Id)
		}
		seen[device.Id] = true
		if _, err := rpcapi.ClientHwdByName(string(device.Hwd)); err != nil {
			return fmt.Errorf("%s: %w", device.Id, err)
		}
	}
	return nil
}

func manifestHwd(manifest apitypes.MhsV0Manifest, id, name string, writing bool) (rpcpb.ClientHwd, error) {
	if !rpcapi.ValidMhsKey(id) {
		return 0, fmt.Errorf("invalid HWD instance id %q", id)
	}
	hwd, err := rpcapi.ClientHwdByName(name)
	if err != nil {
		return 0, err
	}
	meta, err := rpcapi.ClientHwdMetadata(hwd)
	if err != nil {
		return 0, err
	}
	if writing && meta.WriteRequest == "" {
		return 0, fmt.Errorf("%s HWD is read-only", name)
	}
	for _, device := range manifest.Devices {
		if device.Id == id {
			if string(device.Hwd) != name {
				return 0, fmt.Errorf("HWD instance %s is %s, not %s", id, device.Hwd, name)
			}
			return hwd, nil
		}
	}
	return 0, fmt.Errorf("unknown HWD instance %s", id)
}

// ReadRequest verifies one manifest-declared HWD before device RPC.
func ReadRequest(manifest apitypes.MhsV0Manifest, request apitypes.MhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadRequest, error) {
	hwd, err := manifestHwd(manifest, request.Id, string(request.Hwd), false)
	if err != nil {
		return nil, err
	}
	return &rpcpb.ClientMhsV0ReadRequest{Id: request.Id, Hwd: hwd}, nil
}

// WriteRequest validates the typed write payload before device RPC.
func WriteRequest(manifest apitypes.MhsV0Manifest, request apitypes.MhsV0WriteRequest) (*rpcpb.ClientMhsV0WriteRequest, error) {
	data, err := request.MarshalJSON()
	if err != nil {
		return nil, err
	}
	if err := apitypes.ValidateMhsV0WriteJSON(data); err != nil {
		return nil, err
	}
	var body struct {
		Id    string          `json:"id"`
		Hwd   string          `json:"hwd"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, err
	}
	hwd, err := manifestHwd(manifest, body.Id, body.Hwd, true)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(body.Value), []byte("{}")) {
		return nil, fmt.Errorf("HWD write value must not be empty")
	}
	message, err := rpcapi.ClientHwdWriteRequestJSON(hwd, body.Value)
	if err != nil {
		return nil, err
	}
	if err := validateWrite(message); err != nil {
		return nil, err
	}
	payload, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	return &rpcpb.ClientMhsV0WriteRequest{Id: body.Id, Hwd: hwd, Payload: payload}, nil
}

func validateWrite(message proto.Message) error {
	switch value := message.(type) {
	case *rpcpb.DisplayHwdWriteRequest:
		if value.BrightnessPercent == nil && value.Enabled == nil && value.OffTimeoutMs == nil {
			return fmt.Errorf("display write value must not be empty")
		}
		if value.BrightnessPercent != nil && *value.BrightnessPercent > 100 {
			return fmt.Errorf("display brightness_percent must be 0-100")
		}
	case *rpcpb.LedHwdWriteRequest:
		if value.Enabled == nil && value.BrightnessPercent == nil {
			return fmt.Errorf("led write value must not be empty")
		}
		if value.BrightnessPercent != nil && *value.BrightnessPercent > 100 {
			return fmt.Errorf("led brightness_percent must be 0-100")
		}
	case *rpcpb.SpeakerHwdWriteRequest:
		if value.VolumePercent == nil && value.Muted == nil {
			return fmt.Errorf("speaker write value must not be empty")
		}
		if value.VolumePercent != nil && *value.VolumePercent > 100 {
			return fmt.Errorf("speaker volume_percent must be 0-100")
		}
	default:
		return fmt.Errorf("unsupported HWD write payload %T", message)
	}
	return nil
}

func validString(value *string, maxBytes int) bool {
	return value == nil || (len(*value) <= maxBytes && utf8.ValidString(*value) && !bytes.ContainsRune([]byte(*value), 0))
}

func validateRead(message proto.Message) error {
	hasField := false
	message.ProtoReflect().Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
		hasField = true
		return false
	})
	if !hasField {
		return fmt.Errorf("HWD read payload is empty")
	}
	switch value := message.(type) {
	case *rpcpb.WifiHwdReadResponse:
		if !validString(value.Ssid, 32) || !validString(value.Bssid, 17) || !validString(value.Ip, 45) {
			return fmt.Errorf("invalid wifi read string")
		}
	case *rpcpb.ModemHwdReadResponse:
		if !validString(value.Rat, 16) {
			return fmt.Errorf("invalid modem RAT")
		}
	case *rpcpb.BatteryHwdReadResponse:
		if value.Percent != nil && (math.IsNaN(*value.Percent) || math.IsInf(*value.Percent, 0)) {
			return fmt.Errorf("invalid battery percent")
		}
		if value.VoltageMv != nil && (math.IsNaN(*value.VoltageMv) || math.IsInf(*value.VoltageMv, 0)) {
			return fmt.Errorf("invalid battery voltage")
		}
	case *rpcpb.BleHwdReadResponse, *rpcpb.MicHwdReadResponse,
		*rpcpb.DisplayHwdReadResponse, *rpcpb.LedHwdReadResponse, *rpcpb.SpeakerHwdReadResponse:
	default:
		return fmt.Errorf("unsupported HWD read payload %T", message)
	}
	return nil
}

func resultJSON(id string, hwd rpcpb.ClientHwd, message proto.Message) ([]byte, error) {
	meta, err := rpcapi.ClientHwdMetadata(hwd)
	if err != nil {
		return nil, err
	}
	if err := validateRead(message); err != nil {
		return nil, err
	}
	value, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(message)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Id    string          `json:"id"`
		Hwd   string          `json:"hwd"`
		Value json.RawMessage `json:"value"`
	}{id, meta.Name, value})
}

// ReadResponse checks and projects one typed device response.
func ReadResponse(request *rpcpb.ClientMhsV0ReadRequest, response *rpcpb.ClientMhsV0ReadResponse) (apitypes.MhsV0ReadResult, error) {
	var out apitypes.MhsV0ReadResult
	if response == nil || len(response.Payload) == 0 {
		return out, fmt.Errorf("missing HWD read payload")
	}
	message, err := rpcapi.ClientHwdReadResponseMessage(request.Hwd, response.Payload)
	if err != nil {
		return out, err
	}
	data, err := resultJSON(request.Id, request.Hwd, message)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

// WriteResponse checks the applied-value message returned by the device.
func WriteResponse(request *rpcpb.ClientMhsV0WriteRequest, response *rpcpb.ClientMhsV0WriteResponse) (apitypes.MhsV0WriteResult, error) {
	var out apitypes.MhsV0WriteResult
	if response == nil || len(response.Payload) == 0 {
		return out, fmt.Errorf("missing HWD write payload")
	}
	message, err := rpcapi.ClientHwdWriteResponseMessage(request.Hwd, response.Payload)
	if err != nil {
		return out, err
	}
	var applied proto.Message
	switch value := message.(type) {
	case *rpcpb.DisplayHwdWriteResponse:
		applied = value.Applied
	case *rpcpb.LedHwdWriteResponse:
		applied = value.Applied
	case *rpcpb.SpeakerHwdWriteResponse:
		applied = value.Applied
	}
	if applied == nil {
		return out, fmt.Errorf("missing HWD applied value")
	}
	data, err := resultJSON(request.Id, request.Hwd, applied)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
