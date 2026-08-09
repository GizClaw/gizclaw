package apitypes

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// UnmarshalJSON enforces the closed, complete Firmware channel set at every
// JSON boundary, including Admin requests and stored Firmware records.
func (s *FirmwareSlots) UnmarshalJSON(data []byte) error {
	var decoded FirmwareSlots
	if err := unmarshalFirmwareSlots(data, map[string]*FirmwareSlot{
		"stable":  &decoded.Stable,
		"beta":    &decoded.Beta,
		"develop": &decoded.Develop,
	}); err != nil {
		return err
	}
	*s = decoded
	return nil
}

// UnmarshalJSON enforces the closed, complete Firmware Resource channel set.
func (s *FirmwareSpecSlots) UnmarshalJSON(data []byte) error {
	var decoded FirmwareSpecSlots
	if err := unmarshalFirmwareSlots(data, map[string]*FirmwareSpecSlot{
		"stable":  &decoded.Stable,
		"beta":    &decoded.Beta,
		"develop": &decoded.Develop,
	}); err != nil {
		return err
	}
	*s = decoded
	return nil
}

func unmarshalFirmwareSlots[T any](data []byte, slots map[string]*T) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("firmware slots must be an object")
	}
	for name, raw := range fields {
		slot, ok := slots[name]
		if !ok {
			return fmt.Errorf("json: unknown field %q", name)
		}
		if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
			return fmt.Errorf("firmware slot %q must be an object", name)
		}
		if err := json.Unmarshal(raw, slot); err != nil {
			return fmt.Errorf("firmware slot %q: %w", name, err)
		}
	}
	for _, name := range []string{"stable", "beta", "develop"} {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("firmware slots missing required field %q", name)
		}
	}
	return nil
}
