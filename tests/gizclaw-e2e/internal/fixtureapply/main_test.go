package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveResourceIDs(t *testing.T) {
	ids := resourceIDs{
		"Credential":     {"credential": "credential-id"},
		"OpenAITenant":   {"tenant": "tenant-id"},
		"Workflow":       {"workflow": "workflow-id"},
		"Model":          {"model": "model-id"},
		"Voice":          {"voice": "voice-id"},
		"Tool":           {"tool": "tool-id"},
		"PetDef":         {"pet": "pet-id"},
		"BadgeDef":       {"badge": "badge-id"},
		"GameDef":        {"game": "game-id"},
		"MemoryLayout":   {"memory": "memory-id"},
		"RuntimeProfile": {"runtime": "runtime-id"},
	}
	tests := []struct {
		name     string
		kind     string
		document map[string]any
		wantSpec map[string]any
	}{
		{
			name:     "tenant credential",
			kind:     "OpenAITenant",
			document: map[string]any{"spec": map[string]any{"credential_name": "credential"}},
			wantSpec: map[string]any{"credential_id": "credential-id"},
		},
		{
			name: "model provider",
			kind: "Model",
			document: map[string]any{"spec": map[string]any{
				"provider": map[string]any{"kind": "openai-tenant", "name": "tenant"},
			}},
			wantSpec: map[string]any{"provider": map[string]any{"kind": "openai-tenant", "id": "tenant-id"}},
		},
		{
			name: "workspace workflow",
			kind: "Workspace",
			document: map[string]any{"spec": map[string]any{
				"workflow_name": "workflow",
				"toolkit":       map[string]any{"tool_ids": []any{"tool"}},
			}},
			wantSpec: map[string]any{
				"workflow_id": "workflow-id",
				"toolkit":     map[string]any{"tool_ids": []any{"tool-id"}},
			},
		},
		{
			name: "workflow tool policies",
			kind: "Workflow",
			document: map[string]any{"spec": map[string]any{
				"toolkit": map[string]any{"tool_ids": []any{"tool"}},
				"pet":     map[string]any{"toolkit": map[string]any{"tool_ids": []any{"tool"}}},
			}},
			wantSpec: map[string]any{
				"toolkit": map[string]any{"tool_ids": []any{"tool-id"}},
				"pet":     map[string]any{"toolkit": map[string]any{"tool_ids": []any{"tool-id"}}},
			},
		},
		{
			name: "runtime profile",
			kind: "RuntimeProfile",
			document: map[string]any{"spec": map[string]any{
				"workflows": map[string]any{
					"system": map[string]any{"friend_chatroom": "workflow", "group_chatroom": "workflow", "pet": "workflow"},
					"collections": map[string]any{
						"default": map[string]any{"chat": map[string]any{"resource_id": "workflow"}},
					},
				},
				"resources": map[string]any{
					"models":     map[string]any{"chat": map[string]any{"resource_id": "model"}},
					"voices":     map[string]any{"chat": map[string]any{"resource_id": "voice"}},
					"tools":      map[string]any{"clock": map[string]any{"resource_id": "tool"}},
					"pet_defs":   map[string]any{"starter": map[string]any{"resource_id": "pet"}},
					"badge_defs": map[string]any{"starter": map[string]any{"resource_id": "badge"}},
					"game_defs":  map[string]any{"starter": map[string]any{"resource_id": "game"}},
					"memories":   map[string]any{"default": map[string]any{"layout_id": "memory"}},
				},
			}},
			wantSpec: map[string]any{
				"workflows": map[string]any{
					"system": map[string]any{"friend_chatroom": "workflow-id", "group_chatroom": "workflow-id", "pet": "workflow-id"},
					"collections": map[string]any{
						"default": map[string]any{"chat": map[string]any{"resource_id": "workflow-id"}},
					},
				},
				"resources": map[string]any{
					"models":     map[string]any{"chat": map[string]any{"resource_id": "model-id"}},
					"voices":     map[string]any{"chat": map[string]any{"resource_id": "voice-id"}},
					"tools":      map[string]any{"clock": map[string]any{"resource_id": "tool-id"}},
					"pet_defs":   map[string]any{"starter": map[string]any{"resource_id": "pet-id"}},
					"badge_defs": map[string]any{"starter": map[string]any{"resource_id": "badge-id"}},
					"game_defs":  map[string]any{"starter": map[string]any{"resource_id": "game-id"}},
					"memories":   map[string]any{"default": map[string]any{"layout_id": "memory-id"}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := resolveResourceIDs(tt.document, tt.kind, ids); err != nil {
				t.Fatal(err)
			}
			if got := tt.document["spec"]; !reflect.DeepEqual(got, tt.wantSpec) {
				t.Fatalf("spec = %#v, want %#v", got, tt.wantSpec)
			}
		})
	}
}

func TestResolveResourceIDsRejectsUnknownReference(t *testing.T) {
	document := map[string]any{"spec": map[string]any{"workflow_name": "missing"}}
	if err := resolveResourceIDs(document, "Workspace", resourceIDs{}); err == nil {
		t.Fatal("resolveResourceIDs accepted an unknown Workflow name")
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

func TestRecordSyncedVoicesIndexesFixtureTenantName(t *testing.T) {
	ids := resourceIDs{}
	voices := []applyResult{{
		ID:   "voice-id",
		Name: "volc-tenant:tenant-id:provider-voice-id",
		ProviderData: map[string]any{
			"voice_id": "provider-voice-id",
		},
	}}
	if err := recordSyncedVoices(ids, "tenant-name", voices); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"volc-tenant:tenant-id:provider-voice-id",
		"volc-tenant:tenant-name:provider-voice-id",
	} {
		if got, err := ids.require("Voice", name); err != nil || got != "voice-id" {
			t.Fatalf("Voice/%s = %q, %v; want voice-id", name, got, err)
		}
	}
}

func TestRecordSyncedVoicesRejectsMissingProviderVoiceID(t *testing.T) {
	err := recordSyncedVoices(resourceIDs{}, "tenant-name", []applyResult{{
		ID:   "voice-id",
		Name: "volc-tenant:tenant-id:provider-voice-id",
	}})
	if err == nil {
		t.Fatal("recordSyncedVoices accepted a voice without provider_data.voice_id")
	}
}
