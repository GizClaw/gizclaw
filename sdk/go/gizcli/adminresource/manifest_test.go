package adminresource

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestExpandJSONEnvSupportsJSONDefaults(t *testing.T) {
	data, err := expandJSONEnv([]byte(`{"resource_ids": ${GIZCLAW_TEST_IDS_JSON:-["a", "b"]}}`))
	if err != nil {
		t.Fatalf("expandJSONEnv() error = %v", err)
	}
	var decoded struct {
		ResourceIDs []string `json:"resource_ids"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("expanded JSON did not decode: %v; data=%s", err, data)
	}
	if len(decoded.ResourceIDs) != 2 || decoded.ResourceIDs[0] != "a" || decoded.ResourceIDs[1] != "b" {
		t.Fatalf("resource_ids = %#v", decoded.ResourceIDs)
	}
}

func TestFormatForPath(t *testing.T) {
	for path, want := range map[string]Format{"-": FormatJSON, "a.json": FormatJSON, "a.YAML": FormatYAML, "a.yml": FormatYAML} {
		if got, err := FormatForPath(path); err != nil || got != want {
			t.Fatalf("FormatForPath(%q) = %q, %v", path, got, err)
		}
	}
	_, err := FormatForPath("a.txt")
	if formatErr, ok := errors.AsType[*UnsupportedFormatError](err); !ok || formatErr.Extension != ".txt" {
		t.Fatalf("FormatForPath(a.txt) error = %v", err)
	}
}

func TestPrepareManifestExpandsEnvAndNormalizesKind(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_MANIFEST_SECRET", `quote"and\slash`)
	prepared, err := PrepareManifest(FormatJSON, []byte(`{"apiVersion":"gizclaw.admin/v1alpha1","kind":"CredentialResource","metadata":{"id":"c"},"spec":{"provider":"openai","body":{"api_key":"${GIZCLAW_TEST_MANIFEST_SECRET}"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Kind string
		Spec struct {
			Body map[string]string
		}
	}
	if err := json.Unmarshal(prepared, &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "Credential" || got.Spec.Body["api_key"] != `quote"and\slash` {
		t.Fatalf("prepared = %s", prepared)
	}
	if _, err := DecodeManifest(FormatJSON, prepared); err != nil {
		t.Fatalf("DecodeManifest error = %v", err)
	}

	yamlPrepared, err := PrepareManifest(FormatYAML, []byte("kind: Model\nmetadata:\n  id: m\nspec:\n  note: ${GIZCLAW_TEST_MANIFEST_SECRET}\n"))
	if err != nil || !strings.Contains(string(yamlPrepared), `quote\"and\\slash`) {
		t.Fatalf("YAML prepared = %s, %v", yamlPrepared, err)
	}
}

func TestPrepareManifestErrors(t *testing.T) {
	_, err := PrepareManifest(FormatJSON, []byte(`{"kind":"Model","spec":{"k":"${GIZCLAW_TEST_MANIFEST_MISSING}"}}`))
	if envErr, ok := errors.AsType[*MissingEnvError](err); !ok || envErr.Name != "GIZCLAW_TEST_MANIFEST_MISSING" {
		t.Fatalf("missing env error = %v", err)
	}
	_, err = PrepareManifest(FormatJSON, []byte(`{"kind":"Unknown"}`))
	if kindErr, ok := errors.AsType[*UnknownKindError](err); !ok || kindErr.Kind != "Unknown" {
		t.Fatalf("unknown kind error = %v", err)
	}
	if _, err := PrepareManifest(Format("toml"), []byte(`{}`)); err == nil {
		t.Fatal("unsupported format was accepted")
	}
	if _, err := expandYAMLValue(map[any]any{"key": "value"}); err != nil {
		t.Fatalf("string-key map: %v", err)
	}
	if _, err := expandYAMLValue(map[any]any{1: "value"}); err == nil {
		t.Fatal("non-string YAML key was accepted")
	}
}

func TestExpandEnvString(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_EXPAND_SET", "value")
	t.Setenv("GIZCLAW_TEST_EXPAND_EMPTY", "")
	got, err := ExpandEnvString("a-${GIZCLAW_TEST_EXPAND_SET}-${GIZCLAW_TEST_EXPAND_EMPTY:-d}-${GIZCLAW_TEST_EXPAND_UNSET:-u}")
	if err != nil || got != "a-value-d-u" {
		t.Fatalf("ExpandEnvString = %q, %v", got, err)
	}
	if _, err := ExpandEnvString("${GIZCLAW_TEST_EXPAND_EMPTY}"); err == nil {
		t.Fatal("empty variable without default was accepted")
	}
	if HasEnvReference("plain") || !HasEnvReference("${A:-b}") {
		t.Fatal("HasEnvReference mismatch")
	}
}
