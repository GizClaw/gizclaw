package apitypes

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowSpecJSONOneOf(t *testing.T) {
	for name, tc := range map[string]struct {
		raw     string
		wantErr string
	}{
		"chatroom": {
			raw: `{"driver":"chatroom","chatroom":{"history":{}}}`,
		},
		"nested chatroom": {
			raw: `{"driver":"pet","pet":{"driver":"chatroom","chatroom":{"history":{}}}}`,
		},
		"missing payload": {
			raw:     `{"driver":"chatroom"}`,
			wantErr: "chatroom is required",
		},
		"mismatched payload": {
			raw:     `{"driver":"ast-translate","chatroom":{"history":{}}}`,
			wantErr: "ast_translate is required",
		},
		"multiple payloads": {
			raw:     `{"driver":"chatroom","chatroom":{"history":{}},"ast_translate":{"translation_model":"translation"}}`,
			wantErr: "does not match",
		},
		"recursive pet": {
			raw:     `{"driver":"pet","pet":{"driver":"pet"}}`,
			wantErr: "unsupported reusable driver",
		},
		"unknown field": {
			raw:     `{"driver":"chatroom","chatroom":{"history":{}},"config":{}}`,
			wantErr: "unknown field",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var spec WorkflowSpec
			err := json.Unmarshal([]byte(tc.raw), &spec)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("json.Unmarshal() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("json.Unmarshal() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
