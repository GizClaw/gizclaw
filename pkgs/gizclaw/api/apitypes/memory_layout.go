package apitypes

// UnmarshalJSON rejects unsupported policy fields at Go JSON boundaries.
// Persisted archived policies remain SQL metadata, outside the public contract.
func (s *MemoryLayoutSpec) UnmarshalJSON(data []byte) error {
	type memoryLayoutSpec MemoryLayoutSpec
	var decoded memoryLayoutSpec
	if err := decodeStrictJSON(data, &decoded); err != nil {
		return err
	}
	*s = MemoryLayoutSpec(decoded)
	return nil
}
