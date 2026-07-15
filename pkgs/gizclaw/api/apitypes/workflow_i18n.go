package apitypes

import (
	"bytes"
	"encoding/json"
)

// UnmarshalJSON enforces WorkflowI18n's closed locale set so unsupported
// catalogs are rejected at the HTTP and persisted-data boundary.
func (i18n *WorkflowI18n) UnmarshalJSON(data []byte) error {
	type workflowI18n WorkflowI18n
	var decoded workflowI18n
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*i18n = WorkflowI18n(decoded)
	return nil
}

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
