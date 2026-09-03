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
		"sfu": {
			raw: `{"driver":"sfu","sfu":{}}`,
		},
		"nested dashscope realtime": {
			raw: `{"driver":"pet","pet":{"driver":"dashscope-realtime","dashscope_realtime":{"model":"realtime"}}}`,
		},
		"dashscope realtime": {
			raw: `{"driver":"dashscope-realtime","dashscope_realtime":{"model":"realtime"}}`,
		},
		"doubao realtime duplex": {
			raw: `{"driver":"doubao-realtime-duplex","doubao_realtime_duplex":{"model":"duplex"}}`,
		},
		"eino": {
			raw: `{
				"driver":"eino",
				"eino":{"graph":{
					"name":"strict",
					"compile":{"node_trigger_mode":"any_predecessor"},
					"state":{"fields":[{"name":"answer","type":"string","merge":"replace"}]},
					"nodes":[{"id":"answer","type":"passthrough","inputs":{"value":{"from":"input.text"}},"outputs":{"value":"answer"}}],
					"edges":[{"from":"start","to":"answer"},{"from":"answer","to":"end"}],
					"branches":[],
					"outputs":[{"node":"answer","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]
				}}
			}`,
		},
		"missing payload": {
			raw:     `{"driver":"sfu"}`,
			wantErr: "sfu is required",
		},
		"non-empty sfu payload": {
			raw:     `{"driver":"sfu","sfu":{"room":"main"}}`,
			wantErr: "sfu must be an empty object",
		},
		"mismatched payload": {
			raw:     `{"driver":"ast-translate","sfu":{}}`,
			wantErr: "ast_translate is required",
		},
		"multiple payloads": {
			raw:     `{"driver":"sfu","sfu":{},"ast_translate":{"translation_model":"translation"}}`,
			wantErr: "does not match",
		},
		"recursive pet": {
			raw:     `{"driver":"pet","pet":{"driver":"pet"}}`,
			wantErr: "unsupported reusable driver",
		},
		"nested sfu": {
			raw:     `{"driver":"pet","pet":{"driver":"sfu","sfu":{}}}`,
			wantErr: `unknown field "sfu"`,
		},
		"unknown field": {
			raw:     `{"driver":"sfu","sfu":{},"config":{}}`,
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
