package apitypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeProfileSystemWorkflowsRejectsRemovedChatroomRoles(t *testing.T) {
	for _, raw := range []string{
		`{"pet":"pet-care","friend_chatroom":"sfu"}`,
		`{"pet":"pet-care","group_chatroom":"sfu"}`,
	} {
		var workflows RuntimeProfileSystemWorkflows
		err := json.Unmarshal([]byte(raw), &workflows)
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("json.Unmarshal(%s) error = %v, want unknown field", raw, err)
		}
	}
}

func TestRuntimeProfileSystemWorkflowsRejectsUnknownRole(t *testing.T) {
	var workflows RuntimeProfileSystemWorkflows
	err := json.Unmarshal([]byte(`{
		"pet":"pet-care",
		"shared":"pet-care"
	}`), &workflows)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("json.Unmarshal() error = %v, want unknown field", err)
	}
}
