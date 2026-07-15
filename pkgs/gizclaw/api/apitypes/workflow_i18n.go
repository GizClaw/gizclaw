package apitypes

import (
	"bytes"
	"encoding/json"
)

// UnmarshalJSON enforces WorkflowI18nCatalog's closed HTTP schema so locale
// text is not silently discarded when a catalog field is misspelled.
func (catalog *WorkflowI18nCatalog) UnmarshalJSON(data []byte) error {
	type workflowI18nCatalog WorkflowI18nCatalog
	var decoded workflowI18nCatalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*catalog = WorkflowI18nCatalog(decoded)
	return nil
}
