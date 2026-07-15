package apitypes

import (
	"encoding/json"
	"fmt"
	"sort"
)

// UnmarshalJSON enforces WorkflowMetadata's closed HTTP schema. In particular,
// legacy metadata.description writes must fail instead of succeeding after the
// removed field has been silently discarded.
func (metadata *WorkflowMetadata) UnmarshalJSON(data []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}

	rawName, hasName := object["name"]
	delete(object, "name")
	if len(object) != 0 {
		fields := make([]string, 0, len(object))
		for field := range object {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		return fmt.Errorf("workflow metadata contains unknown field %q", fields[0])
	}

	metadata.Name = ""
	if hasName {
		if err := json.Unmarshal(rawName, &metadata.Name); err != nil {
			return fmt.Errorf("unmarshal workflow metadata name: %w", err)
		}
	}
	return nil
}
