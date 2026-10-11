package apitypes

import "encoding/json"

// UnmarshalJSON discards the legacy starts field at every JSON boundary.
func (conversation *EinoConversation) UnmarshalJSON(data []byte) error {
	type fields EinoConversation
	var decoded struct {
		fields
		Starts json.RawMessage `json:"starts"`
	}
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return err
	}
	decoded.fields.Starts = nil
	*conversation = EinoConversation(decoded.fields)
	return nil
}

// MarshalJSON emits only the effective conversation settings.
func (conversation EinoConversation) MarshalJSON() ([]byte, error) {
	type fields EinoConversation
	encoded := fields(conversation)
	encoded.Starts = nil
	return json.Marshal(encoded)
}
