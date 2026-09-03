package giztest

import (
	"slices"
	"testing"
)

func TestVariablesPreserveTypedReferencesAndSingleAssignment(t *testing.T) {
	v, err := newVariables(map[string]VariableSpec{"count": {Direction: "input", Type: "integer", Value: 3}, "result": {Direction: "output", Type: "string"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.resolve("${count}")
	if err != nil || got != 3 {
		t.Fatalf("resolve = %#v, %v", got, err)
	}
	if err := v.assign("result", "ok"); err != nil {
		t.Fatal(err)
	}
	if err := v.assign("result", "again"); err == nil {
		t.Fatal("second assignment succeeded")
	}
}
func TestEnvironmentAudioVariablesDecodeBase64(t *testing.T) {
	t.Setenv("GIZTEST_TEST_AUDIO", "T2dnUw==")
	v, err := newVariables(map[string]VariableSpec{"tone": {Direction: "input", Type: "audio", Env: "GIZTEST_TEST_AUDIO", MaxBytes: 16}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.resolve("${tone}")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.([]byte)) != "OggS" {
		t.Fatalf("resolve = %#v, want decoded Ogg prefix", got)
	}
	t.Setenv("GIZTEST_TEST_AUDIO", "not base64!")
	if _, err := newVariables(map[string]VariableSpec{"tone": {Direction: "input", Type: "audio", Env: "GIZTEST_TEST_AUDIO"}}); err == nil {
		t.Fatal("invalid base64 audio environment was accepted")
	}
}

func TestGeneratedValuesAreIsolated(t *testing.T) {
	spec := map[string]VariableSpec{"id": {Direction: "input", Type: "string", Generate: "uuid"}}
	a, _ := newVariables(spec)
	b, _ := newVariables(spec)
	if a.values["id"].data == b.values["id"].data {
		t.Fatal("generated values are shared")
	}
}

func TestGeneratedTokenFitsResourceID(t *testing.T) {
	got, err := generateValue("token")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 33 || got[0] != 'g' {
		t.Fatalf("token = %q, want 33-character resource-safe token", got)
	}
}

func TestVariableRedactionsIncludeSecretsAndReportSelection(t *testing.T) {
	v, err := newVariables(map[string]VariableSpec{
		"credential": {Direction: "input", Type: "string", Value: "secret-value-long", Secret: true},
		"prefix":     {Direction: "input", Type: "string", Value: "secret-value", Secret: true},
		"selected":   {Direction: "input", Type: "string", Value: "selected-value"},
		"public":     {Direction: "input", Type: "string", Value: "public-value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := v.redactions([]string{"selected"})
	if !slices.Contains(got, "secret-value-long") || !slices.Contains(got, "selected-value") {
		t.Fatalf("redactions = %v", got)
	}
	if got[0] != "secret-value-long" {
		t.Fatalf("redactions are not longest-first: %v", got)
	}
	if slices.Contains(got, "public-value") {
		t.Fatalf("redactions contain public value: %v", got)
	}
}
