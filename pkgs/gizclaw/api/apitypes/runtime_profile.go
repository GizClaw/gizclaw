package apitypes

// UnmarshalJSON keeps the closed set of system Workflow roles strict at JSON
// boundaries. Required non-empty values are normalized and validated by the
// RuntimeProfile service.
func (s *RuntimeProfileSystemWorkflows) UnmarshalJSON(data []byte) error {
	type runtimeProfileSystemWorkflows RuntimeProfileSystemWorkflows
	var decoded runtimeProfileSystemWorkflows
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return err
	}
	*s = RuntimeProfileSystemWorkflows(decoded)
	return nil
}
