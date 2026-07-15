package apitypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowMetadataRejectsLegacyDescription(t *testing.T) {
	var metadata WorkflowMetadata
	err := json.Unmarshal([]byte(`{"name":"legacy","description":"old"}`), &metadata)
	if err == nil || !strings.Contains(err.Error(), `unknown field "description"`) {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
}

func TestWorkflowMetadataAcceptsStableName(t *testing.T) {
	var metadata WorkflowMetadata
	if err := json.Unmarshal([]byte(`{"name":"workflow"}`), &metadata); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if metadata.Name != "workflow" {
		t.Fatalf("WorkflowMetadata.Name = %q", metadata.Name)
	}
}
