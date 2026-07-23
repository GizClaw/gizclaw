package apitypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeProfileSystemWorkflowsRejectsUnknownRole(t *testing.T) {
	var workflows RuntimeProfileSystemWorkflows
	err := json.Unmarshal([]byte(`{
		"friend_chatroom":"chatroom",
		"group_chatroom":"chatroom",
		"pet":"pet-care",
		"shared":"chatroom"
	}`), &workflows)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("json.Unmarshal() error = %v, want unknown field", err)
	}
}
