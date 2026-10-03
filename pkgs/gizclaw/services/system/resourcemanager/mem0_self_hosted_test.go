package resourcemanager

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/memorylayouttest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/runtimeprofiletest"
)

func TestSelfHostedMem0RuntimeProfileResourceLifecycle(t *testing.T) {
	profiles := runtimeprofiletest.New(t)
	manager := New(Services{RuntimeProfiles: profiles, MemoryLayouts: memorylayouttest.New(t)})
	profiles.ResolveResource = manager.Get
	if _, err := manager.Apply(t.Context(), mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1","kind":"MemoryLayout",
		"metadata":{"id":"assistant-memory"},"spec":{
			"flowcraft":{"extraction":{"model":"extract","mode":"single_pass"},
				"embedding":{"model":"embedding"},"lanes":[{"name":"profile","kind":"note"}],
				"write":{"mode":"sync","tier":"general"}},
			"mem0":{"scope":"peer","custom_instructions":"Keep durable facts."},
			"volc_mem0":{"strategies":[{"name":"profile","type":"semantic","custom_instructions":"Keep durable facts."}]}
		}}`)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		key    string
		action apitypes.ApplyAction
	}{
		{"", apitypes.ApplyActionCreated},
		{`,"api_key":"test-key"`, apitypes.ApplyActionUpdated},
		{``, apitypes.ApplyActionUpdated},
	} {
		resource := mustResource(t, `{
			"apiVersion":"gizclaw.admin/v1alpha1","kind":"RuntimeProfile",
			"metadata":{"id":"local-memory"},"spec":{"quota":{"endpoint":"http://quota.example.test/v1/quota","api_key":"test-quota-key"},"workflows":{},"resources":{"memories":{
				"assistant":{"layout_id":"assistant-memory","driver":"mem0","connection":{
					"type":"mem0_self_hosted","endpoint":"http://127.0.0.1:18000"`+test.key+`}}
			}}}}`)
		applied, err := manager.Apply(t.Context(), resource)
		if err != nil || applied.Action != test.action {
			t.Fatalf("Apply() = %#v, %v; want %s", applied, err, test.action)
		}
		got, err := manager.Get(t.Context(), apitypes.ResourceKindRuntimeProfile, "local-memory")
		if err != nil {
			t.Fatal(err)
		}
		profile, err := got.AsRuntimeProfileResource()
		if err != nil {
			t.Fatal(err)
		}
		connection, err := (*profile.Spec.Resources.Memories)["assistant"].Connection.AsRuntimeProfileMem0SelfHostedConnection()
		if err != nil || connection.Endpoint != "http://127.0.0.1:18000" {
			t.Fatalf("persisted connection = %#v, %v", connection, err)
		}
		if test.key == "" && connection.ApiKey != nil || test.key != "" && (connection.ApiKey == nil || *connection.ApiKey != "test-key") {
			t.Fatalf("persisted API key = %v", connection.ApiKey)
		}
	}
}
