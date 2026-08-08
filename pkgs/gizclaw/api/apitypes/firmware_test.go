package apitypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFirmwareSlotsJSONContract(t *testing.T) {
	testFirmwareSlotsJSONContract[FirmwareSlots](t)
}

func TestFirmwareSpecSlotsJSONContract(t *testing.T) {
	testFirmwareSlotsJSONContract[FirmwareSpecSlots](t)
}

func TestFirmwareStoredJSONRejectsLegacySlots(t *testing.T) {
	var firmware Firmware
	err := json.Unmarshal([]byte(`{"slots":{"stable":{},"beta":{},"develop":{},"pending":{}}}`), &firmware)
	if err == nil || !strings.Contains(err.Error(), `unknown field "pending"`) {
		t.Fatalf("json.Unmarshal() error = %v, want stored pending rejection", err)
	}
}

func TestFirmwareSlotsJSONErrorDoesNotMutateReceiver(t *testing.T) {
	stableDescription := "keep stable"
	betaDescription := "keep beta"
	original := FirmwareSlots{
		Stable: FirmwareSlot{Description: &stableDescription},
		Beta:   FirmwareSlot{Description: &betaDescription},
	}
	value := original
	if err := json.Unmarshal([]byte(`{"stable":{},"beta":{},"develop":{},"unknown":{}}`), &value); err == nil {
		t.Fatal("json.Unmarshal() succeeded, want error")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal(original) error = %v", err)
	}
	if string(encoded) != string(want) {
		t.Fatalf("receiver changed after error: got %s, want %s", encoded, want)
	}
}

func testFirmwareSlotsJSONContract[T any](t *testing.T) {
	t.Helper()
	valid := `{"stable":{},"beta":{},"develop":{}}`
	var decoded T
	if err := json.Unmarshal([]byte(valid), &decoded); err != nil {
		t.Fatalf("json.Unmarshal(valid) error = %v", err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("json.Unmarshal(encoded) error = %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("json.Marshal() fields = %v, want exactly three channels", fields)
	}
	for _, name := range []string{"stable", "beta", "develop"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("json.Marshal() fields = %v, missing %q", fields, name)
		}
	}

	for name, raw := range map[string]string{
		"missing stable":  `{"beta":{},"develop":{}}`,
		"missing beta":    `{"stable":{},"develop":{}}`,
		"missing develop": `{"stable":{},"beta":{}}`,
		"legacy pending":  `{"stable":{},"beta":{},"develop":{},"pending":{}}`,
		"unknown slot":    `{"stable":{},"beta":{},"develop":{},"canary":{}}`,
		"null slot":       `{"stable":null,"beta":{},"develop":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var value T
			err := json.Unmarshal([]byte(raw), &value)
			if err == nil {
				t.Fatal("json.Unmarshal() succeeded, want error")
			}
			if name == "legacy pending" && !strings.Contains(err.Error(), "unknown field \"pending\"") {
				t.Fatalf("json.Unmarshal() error = %v, want pending unknown-field error", err)
			}
		})
	}
}
