package giztest

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed giztest.schema.json
var schemaData []byte

var schemaOnce = sync.OnceValues(func() (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		return nil, err
	}
	if err := c.AddResource("giztest.schema.json", resource); err != nil {
		return nil, err
	}
	return c.Compile("giztest.schema.json")
})

func validateSchema(r io.Reader) error {
	schema, err := schemaOnce()
	if err != nil {
		return fmt.Errorf("compile embedded schema: %w", err)
	}
	value, err := jsonschema.UnmarshalJSON(r)
	if err != nil {
		return fmt.Errorf("decode JSON for schema: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	return nil
}
