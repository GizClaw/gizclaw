package rpcapi

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// MHS v0 bounds match api/proto/rpc/nanopb.options. Byte capacities exclude NUL.
const (
	MaxMhsStates            = 32
	MaxMhsKeyBytes          = 64
	MaxMhsStringBytes       = 256
	MaxMhsInt         int64 = 9007199254740991
)

var mhsKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*([.-][a-z0-9]+)*$`)

// ValidMhsKey checks the bounded, case-sensitive hardware key syntax.
func ValidMhsKey(key string) bool {
	return len(key) <= MaxMhsKeyBytes && mhsKeyPattern.MatchString(key)
}

// ValidMhsString checks the nanopb UTF-8 string representation.
func ValidMhsString(value string) bool {
	return len(value) <= MaxMhsStringBytes && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

// ValidateMhsValue checks wire presence and representability without a manifest.
func ValidateMhsValue(value *rpcpb.MhsValue) error {
	if value == nil {
		return fmt.Errorf("value is required")
	}
	switch v := value.Value.(type) {
	case *rpcpb.MhsValue_BoolValue:
		if v != nil {
			return nil
		}
	case *rpcpb.MhsValue_IntValue:
		if v != nil && v.IntValue >= -MaxMhsInt && v.IntValue <= MaxMhsInt {
			return nil
		}
	case *rpcpb.MhsValue_DoubleValue:
		if v != nil && !math.IsNaN(v.DoubleValue) && !math.IsInf(v.DoubleValue, 0) {
			return nil
		}
	case *rpcpb.MhsValue_StringValue:
		if v != nil && ValidMhsString(v.StringValue) {
			return nil
		}
	}
	return fmt.Errorf("value must contain one finite, bounded MHS value")
}

// ValidateMhsRefs checks a read batch before invoking a device handler.
func ValidateMhsRefs(states []*rpcpb.MhsStateRef) error {
	if len(states) == 0 || len(states) > MaxMhsStates {
		return fmt.Errorf("states must contain 1-%d entries", MaxMhsStates)
	}
	seen := make(map[[2]string]bool, len(states))
	for _, state := range states {
		if state == nil || !ValidMhsKey(state.DeviceId) || !ValidMhsKey(state.State) {
			return fmt.Errorf("invalid MHS device_id or state")
		}
		key := [2]string{state.DeviceId, state.State}
		if seen[key] {
			return fmt.Errorf("duplicate MHS state %s/%s", key[0], key[1])
		}
		seen[key] = true
	}
	return nil
}

// ValidateMhsStates checks a write/result batch before crossing the device boundary.
func ValidateMhsStates(states []*rpcpb.MhsStateValue) error {
	refs := make([]*rpcpb.MhsStateRef, len(states))
	for i, state := range states {
		if state == nil {
			return fmt.Errorf("state is required")
		}
		refs[i] = &rpcpb.MhsStateRef{DeviceId: state.DeviceId, State: state.State}
		if err := ValidateMhsValue(state.Value); err != nil {
			return fmt.Errorf("%s/%s: %w", state.DeviceId, state.State, err)
		}
	}
	return ValidateMhsRefs(refs)
}

// AsClientMhsV0ReadRequest decodes the original protobuf message, preserving oneof presence.
func (t RPCPayload) AsClientMhsV0ReadRequest() (*rpcpb.ClientMhsV0ReadRequest, error) {
	body := new(rpcpb.ClientMhsV0ReadRequest)
	err := t.decode("ClientMhsV0ReadRequest", body)
	return body, err
}

// FromClientMhsV0ReadRequest encodes the original protobuf message.
func (t *RPCPayload) FromClientMhsV0ReadRequest(v *rpcpb.ClientMhsV0ReadRequest) error {
	return t.encode("ClientMhsV0ReadRequest", v)
}

// AsClientMhsV0ReadResponse decodes the original protobuf message, preserving oneof presence.
func (t RPCPayload) AsClientMhsV0ReadResponse() (*rpcpb.ClientMhsV0ReadResponse, error) {
	body := new(rpcpb.ClientMhsV0ReadResponse)
	err := t.decode("ClientMhsV0ReadResponse", body)
	return body, err
}

// FromClientMhsV0ReadResponse encodes the original protobuf message.
func (t *RPCPayload) FromClientMhsV0ReadResponse(v *rpcpb.ClientMhsV0ReadResponse) error {
	return t.encode("ClientMhsV0ReadResponse", v)
}

// AsClientMhsV0WriteRequest decodes the original protobuf message, preserving oneof presence.
func (t RPCPayload) AsClientMhsV0WriteRequest() (*rpcpb.ClientMhsV0WriteRequest, error) {
	body := new(rpcpb.ClientMhsV0WriteRequest)
	err := t.decode("ClientMhsV0WriteRequest", body)
	return body, err
}

// FromClientMhsV0WriteRequest encodes the original protobuf message.
func (t *RPCPayload) FromClientMhsV0WriteRequest(v *rpcpb.ClientMhsV0WriteRequest) error {
	return t.encode("ClientMhsV0WriteRequest", v)
}

// AsClientMhsV0WriteResponse decodes the original protobuf message, preserving oneof presence.
func (t RPCPayload) AsClientMhsV0WriteResponse() (*rpcpb.ClientMhsV0WriteResponse, error) {
	body := new(rpcpb.ClientMhsV0WriteResponse)
	err := t.decode("ClientMhsV0WriteResponse", body)
	return body, err
}

// FromClientMhsV0WriteResponse encodes the original protobuf message.
func (t *RPCPayload) FromClientMhsV0WriteResponse(v *rpcpb.ClientMhsV0WriteResponse) error {
	return t.encode("ClientMhsV0WriteResponse", v)
}
