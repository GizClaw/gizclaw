// Package mhs validates GizClaw's MHS-inspired pre-standard v0 hardware states.
package mhs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// ValidateManifest rejects ambiguous keys and constraints devices cannot represent.
func ValidateManifest(manifest apitypes.MhsV0Manifest) error {
	if manifest.Devices == nil {
		return fmt.Errorf("devices must be an array")
	}
	devices := make(map[string]bool, len(manifest.Devices))
	for _, device := range manifest.Devices {
		if !rpcapi.ValidMhsKey(device.Id) {
			return fmt.Errorf("invalid device id %q (1-64 ASCII bytes)", device.Id)
		}
		if devices[device.Id] {
			return fmt.Errorf("duplicate device id %q", device.Id)
		}
		devices[device.Id] = true
		if strings.TrimSpace(device.Kind) == "" {
			return fmt.Errorf("device %s: kind is required", device.Id)
		}
		if len(device.States) == 0 {
			return fmt.Errorf("device %s: states must not be empty", device.Id)
		}
		states := make(map[string]bool, len(device.States))
		for _, state := range device.States {
			if !rpcapi.ValidMhsKey(state.Name) {
				return fmt.Errorf("device %s: invalid state name %q", device.Id, state.Name)
			}
			if states[state.Name] {
				return fmt.Errorf("device %s: duplicate state %q", device.Id, state.Name)
			}
			states[state.Name] = true
			if err := validateState(state); err != nil {
				return fmt.Errorf("%s/%s: %w", device.Id, state.Name, err)
			}
		}
	}
	return nil
}

func validateState(state apitypes.MhsV0State) error {
	if state.Access != "read" && state.Access != "read_write" {
		return fmt.Errorf("access must be read or read_write")
	}
	switch state.Type {
	case "bool", "int", "double", "string", "enum":
	default:
		return fmt.Errorf("unknown type %q", state.Type)
	}
	numeric := state.Type == "int" || state.Type == "double"
	for _, constraint := range []struct {
		name  string
		value *float64
	}{{"min", state.Min}, {"max", state.Max}, {"step", state.Step}} {
		if constraint.value == nil {
			continue
		}
		v := *constraint.value
		if !numeric {
			return fmt.Errorf("%s is only allowed for numeric types", constraint.name)
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%s must be finite", constraint.name)
		}
		if state.Type == "int" && (math.Trunc(v) != v || v < -float64(rpcapi.MaxMhsInt) || v > float64(rpcapi.MaxMhsInt)) {
			return fmt.Errorf("%s must be an integral JSON-safe number", constraint.name)
		}
	}
	if state.Min != nil && state.Max != nil && *state.Min > *state.Max {
		return fmt.Errorf("min must not exceed max")
	}
	if state.Step != nil && *state.Step <= 0 {
		return fmt.Errorf("step must be positive")
	}
	if state.Type != "enum" {
		if state.EnumValues != nil {
			return fmt.Errorf("enum_values is forbidden for non-enum states")
		}
		return nil
	}
	if state.EnumValues == nil || len(*state.EnumValues) == 0 {
		return fmt.Errorf("enum_values is required and must not be empty")
	}
	values := make(map[string]bool, len(*state.EnumValues))
	for _, value := range *state.EnumValues {
		if !rpcapi.ValidMhsString(value) {
			return fmt.Errorf("enum_values must be UTF-8 without NUL and at most 256 bytes")
		}
		if values[value] {
			return fmt.Errorf("duplicate enum value %q", value)
		}
		values[value] = true
	}
	return nil
}

func findState(manifest apitypes.MhsV0Manifest, deviceID, name string) (apitypes.MhsV0State, error) {
	for _, device := range manifest.Devices {
		if device.Id != deviceID {
			continue
		}
		for _, state := range device.States {
			if state.Name == name {
				return state, nil
			}
		}
	}
	return apitypes.MhsV0State{}, fmt.Errorf("unknown MHS state %s/%s", deviceID, name)
}

// ReadRequest validates the whole batch before any device RPC.
func ReadRequest(manifest apitypes.MhsV0Manifest, request apitypes.MhsV0ReadRequest) (*rpcpb.ClientMhsV0ReadRequest, error) {
	out := &rpcpb.ClientMhsV0ReadRequest{States: make([]*rpcpb.MhsStateRef, len(request.States))}
	for i, state := range request.States {
		out.States[i] = &rpcpb.MhsStateRef{DeviceId: state.DeviceId, State: state.State}
	}
	if err := rpcapi.ValidateMhsRefs(out.States); err != nil {
		return nil, err
	}
	for _, state := range out.States {
		if _, err := findState(manifest, state.DeviceId, state.State); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// WriteRequest validates access, presence, types, ranges and the step grid.
func WriteRequest(manifest apitypes.MhsV0Manifest, request apitypes.MhsV0States) (*rpcpb.ClientMhsV0WriteRequest, error) {
	refs := apitypes.MhsV0ReadRequest{States: make([]apitypes.MhsV0StateRef, len(request.States))}
	for i, state := range request.States {
		refs.States[i] = apitypes.MhsV0StateRef{DeviceId: state.DeviceId, State: state.State}
	}
	if _, err := ReadRequest(manifest, refs); err != nil {
		return nil, err
	}
	out := &rpcpb.ClientMhsV0WriteRequest{States: make([]*rpcpb.MhsStateValue, len(request.States))}
	for i, entry := range request.States {
		state, err := findState(manifest, entry.DeviceId, entry.State)
		if err != nil {
			return nil, err
		}
		if state.Access != "read_write" {
			return nil, fmt.Errorf("%s/%s is read-only", entry.DeviceId, entry.State)
		}
		value, err := decodeValue(state, entry.Value)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", entry.DeviceId, entry.State, err)
		}
		out.States[i] = &rpcpb.MhsStateValue{DeviceId: entry.DeviceId, State: entry.State, Value: value}
	}
	return out, nil
}

func decodeValue(state apitypes.MhsV0State, raw apitypes.MhsV0Value) (*rpcpb.MhsValue, error) {
	data, err := raw.MarshalJSON()
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("invalid value: %w", err)
	}
	out := new(rpcpb.MhsValue)
	switch state.Type {
	case "bool":
		v, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("value must be bool")
		}
		out.Value = &rpcpb.MhsValue_BoolValue{BoolValue: v}
	case "int", "double":
		v, ok := value.(json.Number)
		if !ok {
			return nil, fmt.Errorf("value must be %s", state.Type)
		}
		if state.Type == "int" {
			n, ok := integralJSONNumber(string(v))
			if !ok {
				return nil, fmt.Errorf("value must be an integral JSON-safe number")
			}
			out.Value = &rpcpb.MhsValue_IntValue{IntValue: n}
		} else {
			n, err := v.Float64()
			if err != nil {
				return nil, fmt.Errorf("value must be finite")
			}
			out.Value = &rpcpb.MhsValue_DoubleValue{DoubleValue: n}
		}
	case "string", "enum":
		v, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("value must be string")
		}
		out.Value = &rpcpb.MhsValue_StringValue{StringValue: v}
	default:
		return nil, fmt.Errorf("unknown state type")
	}
	if err := validateValue(state, out, true); err != nil {
		return nil, err
	}
	return out, nil
}

// integralJSONNumber consumes a syntactically valid JSON number without float
// rounding or allocating a big integer for an attacker-controlled exponent.
func integralJSONNumber(text string) (int64, bool) {
	negative := strings.HasPrefix(text, "-")
	mantissa := strings.TrimPrefix(text, "-")
	exponentText := "0"
	if index := strings.IndexAny(mantissa, "eE"); index >= 0 {
		mantissa, exponentText = mantissa[:index], mantissa[index+1:]
	}
	fractional := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractional = len(mantissa) - index - 1
		mantissa = mantissa[:index] + mantissa[index+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return 0, true
	}
	exponent, err := strconv.Atoi(exponentText)
	if err != nil || exponent < -len(text)-32 || exponent > len(text)+32 {
		return 0, false
	}
	trimmed := strings.TrimRight(digits, "0")
	power := exponent - fractional + len(digits) - len(trimmed)
	if power < 0 || power > 16 || len(trimmed)+power > 16 {
		return 0, false
	}
	number, err := strconv.ParseInt(trimmed+strings.Repeat("0", power), 10, 64)
	if err != nil || number > rpcapi.MaxMhsInt {
		return 0, false
	}
	if negative {
		number = -number
	}
	return number, true
}

// validateValue checks the manifest type and enum membership. Range and step
// constraints bound what a caller may write; values a device reports reflect
// real hardware state and are checked only for type (ranges false).
func validateValue(state apitypes.MhsV0State, value *rpcpb.MhsValue, ranges bool) error {
	if err := rpcapi.ValidateMhsValue(value); err != nil {
		return err
	}
	var number float64
	switch state.Type {
	case "bool":
		if _, ok := value.Value.(*rpcpb.MhsValue_BoolValue); !ok {
			return fmt.Errorf("value must be bool")
		}
		return nil
	case "string", "enum":
		v, ok := value.Value.(*rpcpb.MhsValue_StringValue)
		if !ok {
			return fmt.Errorf("value must be string")
		}
		if state.Type == "enum" && (state.EnumValues == nil || !slices.Contains(*state.EnumValues, v.StringValue)) {
			return fmt.Errorf("value is not listed in enum_values")
		}
		return nil
	case "int":
		v, ok := value.Value.(*rpcpb.MhsValue_IntValue)
		if !ok {
			return fmt.Errorf("value must be int")
		}
		number = float64(v.IntValue)
	case "double":
		v, ok := value.Value.(*rpcpb.MhsValue_DoubleValue)
		if !ok {
			return fmt.Errorf("value must be double")
		}
		number = v.DoubleValue
	default:
		return fmt.Errorf("unknown state type")
	}
	if !ranges {
		return nil
	}
	if state.Min != nil && number < *state.Min {
		return fmt.Errorf("value is below min")
	}
	if state.Max != nil && number > *state.Max {
		return fmt.Errorf("value is above max")
	}
	if state.Step != nil {
		origin := 0.0
		if state.Min != nil {
			origin = *state.Min
		}
		// Decimal JSON values define the grid; rational arithmetic avoids both
		// rejecting 0.3 / 0.1 and accepting off-grid large values through rounding.
		decimal := func(v float64) *big.Rat {
			n, _ := new(big.Rat).SetString(strconv.FormatFloat(v, 'g', -1, 64))
			return n
		}
		q := new(big.Rat).Sub(decimal(number), decimal(origin))
		if !q.Quo(q, decimal(*state.Step)).IsInt() {
			return fmt.Errorf("value is off the step grid (origin min or zero)")
		}
	}
	return nil
}

// Response verifies exact key coverage, manifest types and enum membership
// before exposing device-controlled values through HTTP. It does not apply
// min/max/step: a device may report clamped or off-grid real state. Order is
// not significant.
func Response(manifest apitypes.MhsV0Manifest, requested []*rpcpb.MhsStateRef, states []*rpcpb.MhsStateValue) (apitypes.MhsV0States, error) {
	if err := rpcapi.ValidateMhsStates(states); err != nil {
		return apitypes.MhsV0States{}, err
	}
	if len(states) != len(requested) {
		return apitypes.MhsV0States{}, fmt.Errorf("device returned incomplete states")
	}
	wanted := make(map[[2]string]bool, len(requested))
	for _, ref := range requested {
		wanted[[2]string{ref.DeviceId, ref.State}] = true
	}
	out := apitypes.MhsV0States{States: make([]apitypes.MhsV0StateValue, len(states))}
	for i, entry := range states {
		if !wanted[[2]string{entry.DeviceId, entry.State}] {
			return apitypes.MhsV0States{}, fmt.Errorf("device returned an unrequested state")
		}
		state, err := findState(manifest, entry.DeviceId, entry.State)
		if err != nil {
			return apitypes.MhsV0States{}, err
		}
		if err := validateValue(state, entry.Value, false); err != nil {
			return apitypes.MhsV0States{}, err
		}
		var value any
		switch v := entry.Value.Value.(type) {
		case *rpcpb.MhsValue_BoolValue:
			value = v.BoolValue
		case *rpcpb.MhsValue_IntValue:
			value = v.IntValue
		case *rpcpb.MhsValue_DoubleValue:
			value = v.DoubleValue
		case *rpcpb.MhsValue_StringValue:
			value = v.StringValue
		}
		data, err := json.Marshal(value)
		if err != nil {
			return apitypes.MhsV0States{}, err
		}
		out.States[i] = apitypes.MhsV0StateValue{DeviceId: entry.DeviceId, State: entry.State}
		if err := out.States[i].Value.UnmarshalJSON(data); err != nil {
			return apitypes.MhsV0States{}, err
		}
	}
	return out, nil
}
