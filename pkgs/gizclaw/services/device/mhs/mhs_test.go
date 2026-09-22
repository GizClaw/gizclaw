package mhs

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func testManifest() apitypes.MhsV0Manifest {
	return apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "display.main", Kind: "display", States: []apitypes.MhsV0State{
		{Name: "brightness", Type: "int", Access: "read_write", Min: new(0.0), Max: new(100.0), Step: new(5.0)},
		{Name: "enabled", Type: "bool", Access: "read_write"},
		{Name: "mode", Type: "enum", Access: "read_write", EnumValues: new([]string{"auto", "off"})},
		{Name: "label", Type: "string", Access: "read_write"},
		{Name: "voltage", Type: "double", Access: "read", Min: new(0.0), Max: new(5.0)},
		{Name: "ratio", Type: "double", Access: "read_write", Step: new(0.1)},
	}}}}
}

func TestValidateManifest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*apitypes.MhsV0Manifest)
	}{
		{"missing devices", func(m *apitypes.MhsV0Manifest) { m.Devices = nil }},
		{"duplicate device", func(m *apitypes.MhsV0Manifest) { m.Devices = append(m.Devices, m.Devices[0]) }},
		{"duplicate state", func(m *apitypes.MhsV0Manifest) {
			m.Devices[0].States = append(m.Devices[0].States, m.Devices[0].States[0])
		}},
		{"bad device syntax", func(m *apitypes.MhsV0Manifest) { m.Devices[0].Id = "Display" }},
		{"long id", func(m *apitypes.MhsV0Manifest) { m.Devices[0].Id = strings.Repeat("x", 65) }},
		{"long name", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[0].Name = strings.Repeat("x", 65) }},
		{"kind required", func(m *apitypes.MhsV0Manifest) { m.Devices[0].Kind = "" }},
		{"states required", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States = nil }},
		{"type", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[0].Type = "float" }},
		{"access", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[0].Access = "write" }},
		{"bool constraint", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[1].Min = new(0.0) }},
		{"string constraint", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[3].Max = new(10.0) }},
		{"enum constraint", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[2].Step = new(1.0) }},
		{"inverted bounds", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[0].Max = new(-1.0) }},
		{"zero step", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[0].Step = new(0.0) }},
		{"negative step", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[0].Step = new(-1.0) }},
		{"non-finite", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[4].Max = new(math.Inf(1)) }},
		{"int fraction", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[0].Min = new(0.5) }},
		{"int overflow", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[0].Max = new(9007199254740992.0) }},
		{"enum missing", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[2].EnumValues = nil }},
		{"enum empty", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[2].EnumValues = new([]string{}) }},
		{"enum on bool", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[1].EnumValues = new([]string{}) }},
		{"duplicate enum", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[2].EnumValues = new([]string{"a", "a"}) }},
		{"enum long bytes", func(m *apitypes.MhsV0Manifest) {
			m.Devices[0].States[2].EnumValues = new([]string{strings.Repeat("中", 86)})
		}},
		{"enum NUL", func(m *apitypes.MhsV0Manifest) { m.Devices[0].States[2].EnumValues = new([]string{"a\x00"}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testManifest()
			tc.mutate(&m)
			if err := ValidateManifest(m); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	m := testManifest()
	m.Devices[0].Kind = "product-specific"
	m.Devices[0].Id = strings.Repeat("x", 64)
	if err := ValidateManifest(m); err != nil {
		t.Fatal(err)
	}
	if err := ValidateManifest(apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{}}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteValidation(t *testing.T) {
	for _, tc := range []struct {
		state, value string
		valid        bool
	}{
		{"brightness", "0", true}, {"brightness", "100", true}, {"brightness", "1e1", true}, {"brightness", "5.0", true},
		{"brightness", "101", false}, {"brightness", "-5", false}, {"brightness", "2", false}, {"brightness", "1.5", false}, {"brightness", "5.00000000000000001", false},
		{"brightness", "9007199254740992", false}, {"brightness", `"5"`, false}, {"brightness", "true", false},
		{"enabled", "false", true}, {"enabled", "0", false}, {"mode", `"auto"`, true}, {"mode", `"other"`, false},
		{"label", `""`, true}, {"label", `null`, false}, {"label", `{}`, false}, {"label", `[]`, false}, {"label", `"\u0000"`, false},
		{"label", strconvJSON(strings.Repeat("中", 85)), true}, {"label", strconvJSON(strings.Repeat("中", 86)), false},
		{"voltage", "1.0", false}, {"ratio", "0.3", true}, {"ratio", "0.31", false}, {"ratio", "1e999", false}, {"unknown", "1", false},
	} {
		t.Run(tc.state+tc.value, func(t *testing.T) {
			var request apitypes.MhsV0States
			if err := json.Unmarshal([]byte(`{"states":[{"device_id":"display.main","state":"`+tc.state+`","value":`+tc.value+`}]}`), &request); err != nil {
				t.Fatal(err)
			}
			_, err := WriteRequest(testManifest(), request)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
		})
	}
}
func strconvJSON(value string) string { data, _ := json.Marshal(value); return string(data) }

func TestIntegralJSONNumber(t *testing.T) {
	for text, want := range map[string]int64{
		"0": 0, "-0.000e99999999999999999999": 0,
		"1e1": 10, "0.1e2": 10, "1.000": 1, "1000e-3": 1,
		"-9007199254740991": -rpcapi.MaxMhsInt, "9007199254740991.0": rpcapi.MaxMhsInt,
	} {
		if got, ok := integralJSONNumber(text); !ok || got != want {
			t.Fatalf("%s: got %d, %v, want %d", text, got, ok, want)
		}
	}
	for _, text := range []string{"1e999999999", "1e-999999999", "1e-999999999999999999999", "5.00000000000000001", "1.5", "9007199254740992", "-9007199254740992"} {
		if got, ok := integralJSONNumber(text); ok {
			t.Fatalf("accepted %s as %d", text, got)
		}
	}
}

func TestStepOriginAndMaximumBatch(t *testing.T) {
	manifest := testManifest()
	manifest.Devices[0].States[5].Min = new(0.05)
	var request apitypes.MhsV0States
	if err := json.Unmarshal([]byte(`{"states":[{"device_id":"display.main","state":"ratio","value":0.35}]}`), &request); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteRequest(manifest, request); err != nil {
		t.Fatalf("min-based decimal grid: %v", err)
	}
	refs := apitypes.MhsV0ReadRequest{}
	manifest.Devices[0].States = nil
	for i := range rpcapi.MaxMhsStates {
		name := "state" + string(rune('a'+i/26)) + string(rune('a'+i%26))
		manifest.Devices[0].States = append(manifest.Devices[0].States, apitypes.MhsV0State{Name: name, Type: "bool", Access: "read"})
		refs.States = append(refs.States, apitypes.MhsV0StateRef{DeviceId: "display.main", State: name})
	}
	if _, err := ReadRequest(manifest, refs); err != nil {
		t.Fatalf("maximum legal batch: %v", err)
	}
}

func TestReadBatchValidation(t *testing.T) {
	key := apitypes.MhsV0StateRef{DeviceId: "display.main", State: "enabled"}
	for _, states := range [][]apitypes.MhsV0StateRef{nil, {}, {key, key}, make([]apitypes.MhsV0StateRef, 33), {{DeviceId: "missing", State: "enabled"}}, {{DeviceId: "display.main", State: "missing"}}, {{DeviceId: "Display", State: "enabled"}}} {
		if _, err := ReadRequest(testManifest(), apitypes.MhsV0ReadRequest{States: states}); err == nil {
			t.Fatalf("accepted %+v", states)
		}
	}
	if _, err := ReadRequest(testManifest(), apitypes.MhsV0ReadRequest{States: []apitypes.MhsV0StateRef{key}}); err != nil {
		t.Fatal(err)
	}
}

func TestResponseValidatesDeviceValues(t *testing.T) {
	refs := []*rpcpb.MhsStateRef{{DeviceId: "display.main", State: "brightness"}}
	good := &rpcpb.MhsStateValue{DeviceId: "display.main", State: "brightness", Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_IntValue{IntValue: 40}}}
	for _, states := range [][]*rpcpb.MhsStateValue{
		nil, {nil}, {good, good}, {{DeviceId: "display.main", State: "enabled", Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_BoolValue{BoolValue: false}}}},
		{{DeviceId: "display.main", State: "brightness"}}, {{DeviceId: "display.main", State: "brightness", Value: &rpcpb.MhsValue{}}},
		{{DeviceId: "display.main", State: "brightness", Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_DoubleValue{DoubleValue: 40}}}},
	} {
		if _, err := Response(testManifest(), refs, states); err == nil {
			t.Fatalf("accepted %+v", states)
		}
	}
	got, err := Response(testManifest(), refs, []*rpcpb.MhsStateValue{good})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(got)
	if !strings.Contains(string(data), `"value":40`) {
		t.Fatal(string(data))
	}
	// Reported values are real hardware state: out-of-range and off-grid values
	// pass through, while enum membership is still enforced.
	for _, value := range []int64{101, 37, -3} {
		reported := &rpcpb.MhsStateValue{DeviceId: "display.main", State: "brightness", Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_IntValue{IntValue: value}}}
		if _, err := Response(testManifest(), refs, []*rpcpb.MhsStateValue{reported}); err != nil {
			t.Fatalf("rejected reported brightness %d: %v", value, err)
		}
	}
	modeRefs := []*rpcpb.MhsStateRef{{DeviceId: "display.main", State: "mode"}}
	unlisted := &rpcpb.MhsStateValue{DeviceId: "display.main", State: "mode", Value: &rpcpb.MhsValue{Value: &rpcpb.MhsValue_StringValue{StringValue: "eco"}}}
	if _, err := Response(testManifest(), modeRefs, []*rpcpb.MhsStateValue{unlisted}); err == nil {
		t.Fatal("accepted an enum value missing from enum_values")
	}
	if err := rpcapi.ValidateMhsValue(&rpcpb.MhsValue{Value: &rpcpb.MhsValue_DoubleValue{DoubleValue: math.NaN()}}); err == nil {
		t.Fatal("accepted NaN")
	}
}
