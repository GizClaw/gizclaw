package apitypes

import (
	"fmt"
)

// UnmarshalJSON preserves the WorkflowSpec one-of contract at Go JSON
// boundaries. The generated Go representation remains a convenient struct,
// while the canonical OpenAPI and JavaScript surfaces expose typed variants.
func (s *WorkflowSpecObject) UnmarshalJSON(data []byte) error {
	type workflowSpecObject WorkflowSpecObject
	var decoded workflowSpecObject
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return err
	}
	value := WorkflowSpecObject(decoded)
	if !value.Driver.Valid() {
		return fmt.Errorf("unsupported driver %q", value.Driver)
	}
	if err := validateWorkflowDriverPayloads(
		string(value.Driver),
		value.Flowcraft != nil,
		value.DoubaoRealtime != nil,
		value.AstTranslate != nil,
		value.Chatroom != nil,
		value.Pet != nil,
	); err != nil {
		return err
	}
	*s = value
	return nil
}

// UnmarshalJSON enforces the same driver/payload shape for the reusable
// non-Pet union and rejects recursive Pet nesting through its driver enum.
func (s *ReusableWorkflowSpecObject) UnmarshalJSON(data []byte) error {
	type reusableWorkflowSpecObject ReusableWorkflowSpecObject
	var decoded reusableWorkflowSpecObject
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return err
	}
	value := ReusableWorkflowSpecObject(decoded)
	if !value.Driver.Valid() {
		return fmt.Errorf("unsupported reusable driver %q", value.Driver)
	}
	if err := validateWorkflowDriverPayloads(
		string(value.Driver),
		value.Flowcraft != nil,
		value.DoubaoRealtime != nil,
		value.AstTranslate != nil,
		value.Chatroom != nil,
		false,
	); err != nil {
		return err
	}
	*s = value
	return nil
}

func validateWorkflowDriverPayloads(driver string, flowcraft, doubaoRealtime, astTranslate, chatroom, pet bool) error {
	payloads := []struct {
		driver  string
		field   string
		present bool
	}{
		{driver: "flowcraft", field: "flowcraft", present: flowcraft},
		{driver: "doubao-realtime", field: "doubao_realtime", present: doubaoRealtime},
		{driver: "ast-translate", field: "ast_translate", present: astTranslate},
		{driver: "chatroom", field: "chatroom", present: chatroom},
		{driver: "pet", field: "pet", present: pet},
	}
	for _, payload := range payloads {
		if payload.driver == driver {
			if !payload.present {
				return fmt.Errorf("%s is required for driver %q", payload.field, driver)
			}
			continue
		}
		if payload.present {
			return fmt.Errorf("%s does not match driver %q", payload.field, driver)
		}
	}
	return nil
}
