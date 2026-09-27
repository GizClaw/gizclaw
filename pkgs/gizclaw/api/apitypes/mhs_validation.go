package apitypes

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

var mhsWriteValidator = &resourceValidator{load: func() (*openapi3.Schema, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = readEmbeddedAPIFile
	doc, err := loader.LoadFromFile("http/shared/mhs_v0.json")
	if err != nil {
		return nil, fmt.Errorf("load MHS HWD write schema: %w", err)
	}
	return doc.Components.Schemas["MhsV0WriteRequest"].Value, nil
}}

// ValidateMhsV0WriteJSON checks the discriminated HWD write before RPC.
func ValidateMhsV0WriteJSON(data []byte) error {
	return mhsWriteValidator.validate(data)
}
