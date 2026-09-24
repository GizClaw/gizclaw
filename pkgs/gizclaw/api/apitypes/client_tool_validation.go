package apitypes

import (
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
)

var clientToolValidator = &resourceValidator{load: func() (*openapi3.Schema, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = readEmbeddedAPIFile
	doc, err := loader.LoadFromFile("http/peer.json")
	if err != nil {
		return nil, fmt.Errorf("load client tool schema: %w", err)
	}
	return doc.Components.Schemas["ClientToolV0InvokeRequest"].Value, nil
}}

// ValidateClientToolJSON checks the discriminated tool arguments before RPC.
// Errors redact input values, including Wi-Fi credentials.
func ValidateClientToolJSON(data []byte) error {
	return clientToolValidator.validate(data)
}
