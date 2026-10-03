package apitypes

import (
	"encoding/json"
	"testing"
)

func TestRuntimeProfileSelfHostedMem0ResourceContract(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		connection string
		valid      bool
	}{
		{"local without key", `{"type":"mem0_self_hosted","endpoint":"http://127.0.0.1:18000"}`, true},
		{"authenticated", `{"type":"mem0_self_hosted","endpoint":"https://mem0.example","api_key":"key"}`, true},
		{"missing endpoint", `{"type":"mem0_self_hosted"}`, false},
		{"empty key", `{"type":"mem0_self_hosted","endpoint":"https://mem0.example","api_key":""}`, false},
		{"Platform project", `{"type":"mem0_self_hosted","endpoint":"https://mem0.example","project_id":"project"}`, false},
		{"database owned by service", `{"type":"mem0_self_hosted","endpoint":"https://mem0.example","dsn":"postgres://db/mem0"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := []byte(`{"apiVersion":"gizclaw.admin/v1alpha1","kind":"RuntimeProfile",
				"metadata":{"id":"self-hosted"},"spec":{"quota":{"endpoint":"http://quota.example.test/v1/quota","api_key":"test-quota-key"},"workflows":{},"resources":{"memories":{
				"assistant":{"layout_id":"assistant-memory","driver":"mem0","connection":` + test.connection + `}}}}}`)
			err := ValidateResourceJSON(raw)
			if (err == nil) != test.valid {
				t.Fatalf("ValidateResourceJSON() error = %v, valid = %v", err, test.valid)
			}
			if !test.valid {
				return
			}
			var resource Resource
			if err := json.Unmarshal(raw, &resource); err != nil {
				t.Fatal(err)
			}
			profile, err := resource.AsRuntimeProfileResource()
			if err != nil {
				t.Fatal(err)
			}
			connection := (*profile.Spec.Resources.Memories)["assistant"].Connection
			kind, err := connection.Discriminator()
			if err != nil || kind != "mem0_self_hosted" {
				t.Fatalf("connection discriminator = %q, %v", kind, err)
			}
			if _, err := connection.AsRuntimeProfileMem0SelfHostedConnection(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
