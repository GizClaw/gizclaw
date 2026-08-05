package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestResourceIdentitiesUsesCallerSuppliedIDs(t *testing.T) {
	document := map[string]any{
		"kind": "ResourceList",
		"spec": map[string]any{
			"items": []any{
				map[string]any{"kind": "Credential", "metadata": map[string]any{"id": "credential-id"}},
				map[string]any{"kind": "Workflow", "metadata": map[string]any{"id": "workflow-id"}},
			},
		},
	}
	got, isList, err := resourceIdentities(document)
	if err != nil {
		t.Fatal(err)
	}
	want := []applyResult{{ID: "credential-id", Kind: "Credential"}, {ID: "workflow-id", Kind: "Workflow"}}
	if !isList || !reflect.DeepEqual(got, want) {
		t.Fatalf("resourceIdentities = %#v, %t; want %#v, true", got, isList, want)
	}
}

func TestResourceIdentitiesRejectsLegacyName(t *testing.T) {
	document := map[string]any{
		"kind":     "Workflow",
		"metadata": map[string]any{"name": "legacy-workflow"},
	}
	if _, _, err := resourceIdentities(document); err == nil {
		t.Fatal("resourceIdentities accepted legacy metadata.name")
	}
}

func TestValidateApplyResultsRequiresExactCallerIDs(t *testing.T) {
	expected := []applyResult{{ID: "caller-id", Kind: "Workflow"}}
	if err := validateApplyResults(expected, expected); err != nil {
		t.Fatalf("validateApplyResults exact result: %v", err)
	}
	if err := validateApplyResults(expected, []applyResult{{ID: "server-id", Kind: "Workflow"}}); err == nil {
		t.Fatal("validateApplyResults accepted a rewritten ID")
	}
}

func TestDocumentContainsKind(t *testing.T) {
	document := map[string]any{
		"kind": "ResourceList",
		"spec": map[string]any{"items": []any{
			map[string]any{"kind": "Workflow"},
			map[string]any{"kind": "RuntimeProfile"},
		}},
	}
	if !documentContainsKind(document, "RuntimeProfile") {
		t.Fatal("documentContainsKind did not find RuntimeProfile")
	}
	if documentContainsKind(document, "Firmware") {
		t.Fatal("documentContainsKind found absent Firmware")
	}
}

func TestIsTransientCommandError(t *testing.T) {
	for _, detail := range []string{
		"gizclaw: timeout waiting for client readiness",
		"gizclaw: dial: connection reset by peer",
		"gizclaw: dial: unexpected EOF",
	} {
		if !isTransientCommandError(errors.New(detail)) {
			t.Fatalf("isTransientCommandError(%q) = false", detail)
		}
	}
	if isTransientCommandError(errors.New("INVALID_MODEL: provider.id is required")) {
		t.Fatal("isTransientCommandError accepted a resource validation error")
	}
}
