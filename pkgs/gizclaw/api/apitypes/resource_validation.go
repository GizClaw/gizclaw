package apitypes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"

	apiassets "github.com/GizClaw/gizclaw-go/api"
	"github.com/getkin/kin-openapi/openapi3"
)

// ResourceValidationIssue describes one value-redacted OpenAPI schema failure.
type ResourceValidationIssue struct {
	Pointer string
	Keyword string
	Reason  string
}

// ResourceValidationError reports deterministic Resource schema failures.
// It never includes the rejected input value.
type ResourceValidationError struct {
	Issues []ResourceValidationIssue
}

func (e *ResourceValidationError) Error() string {
	var message strings.Builder
	message.WriteString("resource validation failed")
	for _, issue := range e.Issues {
		fmt.Fprintf(&message, "\n- %s [%s]: %s", issue.Pointer, issue.Keyword, issue.Reason)
	}
	return message.String()
}

type resourceValidator struct {
	once   sync.Once
	load   func() (*openapi3.Schema, error)
	schema *openapi3.Schema
	err    error
}

var bundledResourceValidator = &resourceValidator{load: loadBundledResourceSchema}

// ValidateResourceJSON validates one normalized Resource JSON document against
// the OpenAPI contract embedded in this package.
func ValidateResourceJSON(data []byte) error {
	return bundledResourceValidator.validate(data)
}

func (v *resourceValidator) validate(data []byte) error {
	v.once.Do(func() {
		v.schema, v.err = v.load()
	})
	if v.err != nil {
		return v.err
	}
	if v.schema == nil {
		return errors.New("resource schema initializer returned no schema")
	}

	value, err := decodeResourceJSONValue(data)
	if err != nil {
		return err
	}
	if err := v.schema.VisitJSON(value, openapi3.MultiErrors()); err != nil {
		return newResourceValidationError(err)
	}
	return nil
}

func loadBundledResourceSchema() (*openapi3.Schema, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	loader.ReadFromURIFunc = readEmbeddedAPIFile
	doc, err := loader.LoadFromFile("http/resources/resource.json")
	if err != nil {
		return nil, fmt.Errorf("load embedded Resource schema sources: %w", err)
	}
	resource, ok := doc.Components.Schemas["Resource"]
	if !ok || resource == nil || resource.Value == nil {
		return nil, errors.New("embedded OpenAPI document has no Resource schema")
	}
	// Examples are documentation samples rather than runtime validation rules.
	if err := resource.Value.Validate(context.Background(), openapi3.DisableExamplesValidation()); err != nil {
		return nil, fmt.Errorf("validate embedded Resource schema: %w", err)
	}
	return resource.Value, nil
}

func readEmbeddedAPIFile(_ *openapi3.Loader, location *url.URL) ([]byte, error) {
	if location == nil || location.Scheme != "" || location.Host != "" {
		return nil, openapi3.ErrURINotSupported
	}
	name := path.Clean(strings.TrimPrefix(location.Path, "/"))
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return nil, openapi3.ErrURINotSupported
	}
	data, err := apiassets.Files.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read embedded API source %q: %w", name, err)
	}
	return data, nil
}

func decodeResourceJSONValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		var syntaxError *json.SyntaxError
		if errors.As(err, &syntaxError) {
			return nil, fmt.Errorf("invalid resource JSON at byte %d", syntaxError.Offset)
		}
		return nil, errors.New("invalid resource JSON")
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("resource JSON must contain exactly one value")
	}
	return value, nil
}

func newResourceValidationError(err error) error {
	issues := make([]ResourceValidationIssue, 0, 1)
	collectResourceValidationIssues(err, &issues)
	if len(issues) == 0 {
		issues = append(issues, ResourceValidationIssue{
			Pointer: "/",
			Keyword: "schema",
			Reason:  "resource does not match the bundled schema",
		})
	}

	slices.SortFunc(issues, func(a, b ResourceValidationIssue) int {
		if cmp := strings.Compare(a.Pointer, b.Pointer); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Keyword, b.Keyword); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Reason, b.Reason)
	})
	issues = slices.Compact(issues)
	return &ResourceValidationError{Issues: issues}
}

func collectResourceValidationIssues(err error, issues *[]ResourceValidationIssue) {
	if err == nil {
		return
	}
	if multi, ok := err.(openapi3.MultiError); ok {
		for _, nested := range multi {
			collectResourceValidationIssues(nested, issues)
		}
		return
	}
	if schemaError, ok := err.(*openapi3.SchemaError); ok {
		if schemaError.Origin != nil {
			before := len(*issues)
			collectResourceValidationIssues(schemaError.Origin, issues)
			if len(*issues) > before {
				return
			}
		}
		reason := schemaError.Reason
		if reason == "" {
			reason = "value does not match the schema"
		}
		keyword := schemaError.SchemaField
		if keyword == "" {
			keyword = "schema"
		}
		*issues = append(*issues, ResourceValidationIssue{
			Pointer: resourceJSONPointer(schemaError.JSONPointer()),
			Keyword: keyword,
			Reason:  reason,
		})
		return
	}
	if unwrapper, ok := err.(interface{ Unwrap() []error }); ok {
		for _, nested := range unwrapper.Unwrap() {
			collectResourceValidationIssues(nested, issues)
		}
		return
	}
	if nested := errors.Unwrap(err); nested != nil {
		collectResourceValidationIssues(nested, issues)
	}
}

func resourceJSONPointer(path []string) string {
	if len(path) == 0 {
		return "/"
	}
	escaped := make([]string, len(path))
	for i, segment := range path {
		segment = strings.ReplaceAll(segment, "~", "~0")
		escaped[i] = strings.ReplaceAll(segment, "/", "~1")
	}
	return "/" + strings.Join(escaped, "/")
}
