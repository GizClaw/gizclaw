package apitypes

import (
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const validCredentialResource = `{
  "apiVersion":"gizclaw.admin/v1alpha1",
  "kind":"Credential",
  "metadata":{"id":"main"},
  "spec":{"provider":"openai","body":{"api_key":"top-secret-value"}}
}`

func TestValidateResourceJSON(t *testing.T) {
	if err := ValidateResourceJSON([]byte(validCredentialResource)); err != nil {
		t.Fatalf("ValidateResourceJSON() error = %v", err)
	}
}

func TestValidateResourceJSONReportsSortedRedactedIssues(t *testing.T) {
	const secret = "top-secret-value"
	input := `{
  "apiVersion":"unsupported",
  "kind":"Credential",
  "metadata":{"id":" main","secret":"` + secret + `"},
  "spec":{"provider":"openai","body":{"api_key":"` + secret + `"}}
}`
	err := ValidateResourceJSON([]byte(input))
	if err == nil {
		t.Fatal("ValidateResourceJSON() error = nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("validation error leaked secret: %v", err)
	}
	var validationError *ResourceValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("error type = %T, want *ResourceValidationError", err)
	}
	if len(validationError.Issues) < 3 {
		t.Fatalf("issues = %#v, want at least three", validationError.Issues)
	}
	for i := 1; i < len(validationError.Issues); i++ {
		previous := validationError.Issues[i-1]
		current := validationError.Issues[i]
		if previous.Pointer > current.Pointer {
			t.Fatalf("issues are not pointer-sorted: %#v", validationError.Issues)
		}
	}
	joined := err.Error()
	for _, want := range []string{"/apiVersion", "/metadata [properties]", "/metadata/id"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("validation error %q does not contain %q", joined, want)
		}
	}
	second := ValidateResourceJSON([]byte(input))
	if second == nil || second.Error() != err.Error() {
		t.Fatalf("validation errors are not deterministic:\nfirst: %v\nsecond: %v", err, second)
	}
}

func TestValidateResourceJSONCredentialBodyUsesWireShapeBoundary(t *testing.T) {
	input := `{
  "apiVersion":"gizclaw.admin/v1alpha1",
  "kind":"Credential",
  "metadata":{"id":"provider-boundary"},
  "spec":{"provider":"openai","body":{"speech_api_key":"server-validates-provider-combination"}}
}`
	if err := ValidateResourceJSON([]byte(input)); err != nil {
		t.Fatalf("ValidateResourceJSON() error = %v", err)
	}
}

func TestValidateResourceJSONReportsResourceListItemPointer(t *testing.T) {
	input := `{
  "apiVersion":"gizclaw.admin/v1alpha1",
  "kind":"ResourceList",
  "spec":{"items":[{
    "apiVersion":"gizclaw.admin/v1alpha1",
    "kind":"Credential",
    "metadata":{},
    "spec":{"provider":"openai","body":{"api_key":"secret"}}
  }]}
}`
	err := ValidateResourceJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "/spec/items/0/metadata/id") {
		t.Fatalf("ValidateResourceJSON() error = %v, want indexed metadata pointer", err)
	}
}

func TestValidateResourceJSONRejectsNestedResourceList(t *testing.T) {
	input := `{
  "apiVersion":"gizclaw.admin/v1alpha1",
  "kind":"ResourceList",
  "spec":{"items":[{
    "apiVersion":"gizclaw.admin/v1alpha1",
    "kind":"ResourceList",
    "spec":{"items":[]}
  }]}
}`
	err := ValidateResourceJSON([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "/spec/items/0") {
		t.Fatalf("ValidateResourceJSON() error = %v, want nested list pointer", err)
	}
}

func TestValidateResourceJSONRejectsMalformedAndMultipleValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "malformed", input: `{"kind":`, want: "invalid resource JSON"},
		{name: "multiple", input: `{}` + `{}`, want: "exactly one value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateResourceJSON([]byte(tc.input))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateResourceJSON() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestResourceValidatorInitializesOnceConcurrently(t *testing.T) {
	var loads atomic.Int32
	validator := &resourceValidator{load: func() (*openapi3.Schema, error) {
		loads.Add(1)
		return loadBundledResourceSchema()
	}}

	const callers = 32
	var wait sync.WaitGroup
	wait.Add(callers)
	for range callers {
		go func() {
			defer wait.Done()
			if err := validator.validate([]byte(validCredentialResource)); err != nil {
				t.Errorf("validate() error = %v", err)
			}
		}()
	}
	wait.Wait()
	if got := loads.Load(); got != 1 {
		t.Fatalf("schema loads = %d, want 1", got)
	}
}

func TestResourceValidatorCachesInitializationError(t *testing.T) {
	var loads atomic.Int32
	want := errors.New("broken schema")
	validator := &resourceValidator{load: func() (*openapi3.Schema, error) {
		loads.Add(1)
		return nil, want
	}}
	for range 2 {
		if err := validator.validate([]byte(validCredentialResource)); !errors.Is(err, want) {
			t.Fatalf("validate() error = %v, want %v", err, want)
		}
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("schema loads = %d, want 1", got)
	}
}

func TestResourceValidatorRejectsMissingInitializedSchema(t *testing.T) {
	validator := &resourceValidator{load: func() (*openapi3.Schema, error) {
		return nil, nil
	}}
	if err := validator.validate([]byte(validCredentialResource)); err == nil || !strings.Contains(err.Error(), "returned no schema") {
		t.Fatalf("validate() error = %v, want missing schema error", err)
	}
}

func TestReadEmbeddedAPIFile(t *testing.T) {
	data, err := readEmbeddedAPIFile(nil, &url.URL{Path: "http/resources/resource.json"})
	if err != nil {
		t.Fatalf("readEmbeddedAPIFile() error = %v", err)
	}
	if !strings.Contains(string(data), `"Resource"`) {
		t.Fatal("embedded Resource schema does not contain Resource")
	}

	for _, location := range []*url.URL{
		nil,
		{Scheme: "https", Host: "example.com", Path: "/schema.json"},
		{Path: "../outside.json"},
	} {
		if _, err := readEmbeddedAPIFile(nil, location); !errors.Is(err, openapi3.ErrURINotSupported) {
			t.Fatalf("readEmbeddedAPIFile(%v) error = %v, want ErrURINotSupported", location, err)
		}
	}
	if _, err := readEmbeddedAPIFile(nil, &url.URL{Path: "http/missing.json"}); err == nil {
		t.Fatal("readEmbeddedAPIFile() missing file error = nil")
	}
}

func TestResourceJSONPointerEscapesRFC6901Segments(t *testing.T) {
	path := []string{"spec", "a/b~c"}
	if got, want := resourceJSONPointer(path), "/spec/a~1b~0c"; got != want {
		t.Fatalf("resourceJSONPointer() = %q, want %q", got, want)
	}
	if got, want := strings.Join(path, "/"), "spec/a/b~c"; got != want {
		t.Fatalf("resourceJSONPointer() mutated input to %q", got)
	}
}
