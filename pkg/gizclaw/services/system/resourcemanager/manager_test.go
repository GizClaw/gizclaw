package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/badge"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/gameplay/petspecies"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

func TestGetRejectsUnknownKind(t *testing.T) {
	manager := New(Services{})

	_, err := manager.Get(context.Background(), apitypes.ResourceKind("Unknown"), "example")
	assertResourceError(t, err, 400, "UNKNOWN_RESOURCE_KIND")
}

func TestGetRejectsEmptyName(t *testing.T) {
	manager := New(Services{})

	_, err := manager.Get(context.Background(), apitypes.ResourceKindCredential, "")
	assertResourceError(t, err, 400, "INVALID_RESOURCE")
}

func TestGetRejectsResourceList(t *testing.T) {
	manager := New(Services{})

	_, err := manager.Get(context.Background(), apitypes.ResourceKindResourceList, "bundle")
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_GET")
}

func TestGetRejectsMissingService(t *testing.T) {
	manager := New(Services{})

	_, err := manager.Get(context.Background(), apitypes.ResourceKindCredential, "example")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")
}

func TestGetReturnsNotFoundByKind(t *testing.T) {
	tests := []struct {
		name     string
		kind     apitypes.ResourceKind
		manager  *Manager
		wantCode string
	}{
		{name: "acl policy binding", kind: apitypes.ResourceKindACLPolicyBinding, manager: newACLResourceManager(t), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "acl role", kind: apitypes.ResourceKindACLRole, manager: newACLResourceManager(t), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "credential", kind: apitypes.ResourceKindCredential, manager: New(Services{Credentials: newFakeCredentials()}), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "firmware", kind: apitypes.ResourceKindFirmware, manager: New(Services{Firmwares: &firmware.Server{Store: kv.NewMemory(nil)}}), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "badge", kind: apitypes.ResourceKindBadge, manager: New(Services{Badges: &badge.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())}}), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "peer config", kind: apitypes.ResourceKindPeerConfig, manager: New(Services{Peers: newFakePeers()}), wantCode: "GEAR_NOT_FOUND"},
		{name: "model", kind: apitypes.ResourceKindModel, manager: newModelManager(), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "minimax tenant", kind: apitypes.ResourceKindMiniMaxTenant, manager: New(Services{ProviderTenants: newFakeMiniMax(),
			Voices: newFakeMiniMax()}), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "voice", kind: apitypes.ResourceKindVoice, manager: New(Services{ProviderTenants: newFakeMiniMax(),
			Voices: newFakeMiniMax()}), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "pet species", kind: apitypes.ResourceKindPetSpecies, manager: New(Services{PetSpecies: &petspecies.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())}}), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "workspace", kind: apitypes.ResourceKindWorkspace, manager: New(Services{Workspaces: newFakeWorkspaces()}), wantCode: "RESOURCE_NOT_FOUND"},
		{name: "workflow", kind: apitypes.ResourceKindWorkflow, manager: New(Services{Workflows: newFakeWorkflows()}), wantCode: "RESOURCE_NOT_FOUND"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.manager.Get(context.Background(), tc.kind, "missing")
			assertResourceError(t, err, 404, tc.wantCode)
		})
	}
}

func TestPutRejectsUnknownKind(t *testing.T) {
	manager := New(Services{})

	_, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Unknown",
		"metadata": {"name": "example"},
		"spec": {}
	}`))
	assertResourceError(t, err, 400, "UNKNOWN_RESOURCE_KIND")
}

func TestPutRejectsNilManager(t *testing.T) {
	var manager *Manager

	_, err := manager.Put(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "example"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_MANAGER_NOT_CONFIGURED")
}

func TestPutRejectsMissingServicesByKind(t *testing.T) {
	manager := New(Services{})
	tests := []struct {
		name     string
		resource string
	}{
		{name: "acl policy binding", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"ACLPolicyBinding","metadata":{"name":"binding"},"spec":{"subject":{"kind":"pk","id":"peer"},"resource":{"kind":"workspace","id":"workspace"},"role":"role"}}`},
		{name: "acl role", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"ACLRole","metadata":{"name":"role"},"spec":{"permissions":["workspace.read"]}}`},
		{name: "credential", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Credential","metadata":{"name":"name"},"spec":{"provider":"minimax","body":{"api_key":"secret"}}}`},
		{name: "firmware", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Firmware","metadata":{"name":"firmware"},"spec":{"slots":{"stable":{"description":"stable firmware"}}}}`},
		{name: "badge", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Badge","metadata":{"name":"badge"},"spec":{"name":"Badge"}}`},
		{name: "peer config", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"PeerConfig","metadata":{"name":"peer"},"spec":{}}`},
		{name: "model", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"name":"model"},"spec":{"kind":"llm","provider":{"kind":"openai-tenant","name":"main"},"source":"manual"}}`},
		{name: "dashscope tenant", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"DashScopeTenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "gemini tenant", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"GeminiTenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "openai tenant", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"OpenAITenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "minimax tenant", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"MiniMaxTenant","metadata":{"name":"tenant"},"spec":{"app_id":"app","group_id":"group","credential_name":"credential"}}`},
		{name: "voice", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Voice","metadata":{"name":"voice"},"spec":{"provider":{"kind":"minimax","name":"tenant"},"source":"manual"}}`},
		{name: "pet species", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"PetSpecies","metadata":{"name":"species"},"spec":{"name":"Species"}}`},
		{name: "workspace", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workspace","metadata":{"name":"workspace"},"spec":{"workflow_name":"workflow"}}`},
		{name: "workflow", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workflow","metadata":{"name":"workflow"},"spec":{"driver":"flowcraft","flowcraft":{}}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manager.Put(context.Background(), mustResource(t, tc.resource))
			assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")
		})
	}
}

func TestPutRejectsUnsupportedVersionByKind(t *testing.T) {
	aclManager := newACLResourceManager(t)
	manager := New(Services{
		ACL:             aclManager.services.ACL,
		Credentials:     newFakeCredentials(),
		Firmwares:       &firmware.Server{Store: kv.NewMemory(nil)},
		Badges:          &badge.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())},
		Peers:           newFakePeers(),
		Models:          newModelManager().services.Models,
		PetSpecies:      &petspecies.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())},
		ProviderTenants: newFakeMiniMax(),
		Voices:          newFakeMiniMax(),
		Workspaces:      newFakeWorkspaces(),
		Workflows:       newFakeWorkflows(),
	})
	tests := []struct {
		name     string
		resource string
	}{
		{name: "acl policy binding", resource: `{"apiVersion":"unsupported","kind":"ACLPolicyBinding","metadata":{"name":"binding"},"spec":{"subject":{"kind":"pk","id":"peer"},"resource":{"kind":"workspace","id":"workspace"},"role":"role"}}`},
		{name: "acl role", resource: `{"apiVersion":"unsupported","kind":"ACLRole","metadata":{"name":"role"},"spec":{"permissions":["workspace.read"]}}`},
		{name: "credential", resource: `{"apiVersion":"unsupported","kind":"Credential","metadata":{"name":"name"},"spec":{"provider":"minimax","body":{"api_key":"secret"}}}`},
		{name: "firmware", resource: `{"apiVersion":"unsupported","kind":"Firmware","metadata":{"name":"firmware"},"spec":{"slots":{"stable":{"description":"stable firmware"}}}}`},
		{name: "badge", resource: `{"apiVersion":"unsupported","kind":"Badge","metadata":{"name":"badge"},"spec":{"name":"Badge"}}`},
		{name: "peer config", resource: `{"apiVersion":"unsupported","kind":"PeerConfig","metadata":{"name":"peer"},"spec":{}}`},
		{name: "model", resource: `{"apiVersion":"unsupported","kind":"Model","metadata":{"name":"model"},"spec":{"kind":"llm","provider":{"kind":"openai-tenant","name":"main"},"source":"manual"}}`},
		{name: "dashscope tenant", resource: `{"apiVersion":"unsupported","kind":"DashScopeTenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "gemini tenant", resource: `{"apiVersion":"unsupported","kind":"GeminiTenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "openai tenant", resource: `{"apiVersion":"unsupported","kind":"OpenAITenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "minimax tenant", resource: `{"apiVersion":"unsupported","kind":"MiniMaxTenant","metadata":{"name":"tenant"},"spec":{"app_id":"app","group_id":"group","credential_name":"credential"}}`},
		{name: "resource list", resource: `{"apiVersion":"unsupported","kind":"ResourceList","metadata":{"name":"bundle"},"spec":{"items":[]}}`},
		{name: "voice", resource: `{"apiVersion":"unsupported","kind":"Voice","metadata":{"name":"voice"},"spec":{"provider":{"kind":"minimax","name":"tenant"},"source":"manual"}}`},
		{name: "pet species", resource: `{"apiVersion":"unsupported","kind":"PetSpecies","metadata":{"name":"species"},"spec":{"name":"Species"}}`},
		{name: "workspace", resource: `{"apiVersion":"unsupported","kind":"Workspace","metadata":{"name":"workspace"},"spec":{"workflow_name":"workflow"}}`},
		{name: "workflow", resource: `{"apiVersion":"unsupported","kind":"Workflow","metadata":{"name":"workflow"},"spec":{"driver":"flowcraft","flowcraft":{}}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manager.Put(context.Background(), mustResource(t, tc.resource))
			assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_VERSION")
		})
	}
}

func TestDeleteRejectsUnsupportedInputs(t *testing.T) {
	var nilManager *Manager
	_, err := nilManager.Delete(context.Background(), apitypes.ResourceKindCredential, "example")
	assertResourceError(t, err, 500, "RESOURCE_MANAGER_NOT_CONFIGURED")

	manager := New(Services{})
	_, err = manager.Delete(context.Background(), apitypes.ResourceKindCredential, "")
	assertResourceError(t, err, 400, "INVALID_RESOURCE")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKind("Unknown"), "example")
	assertResourceError(t, err, 400, "UNKNOWN_RESOURCE_KIND")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindResourceList, "bundle")
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_DELETE")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindPeerConfig, "peer")
	assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_DELETE")

	_, err = manager.Delete(context.Background(), apitypes.ResourceKindCredential, "example")
	assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")
}

func TestDeleteRejectsMissingServicesByKind(t *testing.T) {
	manager := New(Services{})
	tests := []struct {
		name string
		kind apitypes.ResourceKind
	}{
		{name: "acl policy binding", kind: apitypes.ResourceKindACLPolicyBinding},
		{name: "acl role", kind: apitypes.ResourceKindACLRole},
		{name: "credential", kind: apitypes.ResourceKindCredential},
		{name: "firmware", kind: apitypes.ResourceKindFirmware},
		{name: "badge", kind: apitypes.ResourceKindBadge},
		{name: "model", kind: apitypes.ResourceKindModel},
		{name: "dashscope tenant", kind: apitypes.ResourceKindDashScopeTenant},
		{name: "gemini tenant", kind: apitypes.ResourceKindGeminiTenant},
		{name: "openai tenant", kind: apitypes.ResourceKindOpenAITenant},
		{name: "minimax tenant", kind: apitypes.ResourceKindMiniMaxTenant},
		{name: "voice", kind: apitypes.ResourceKindVoice},
		{name: "pet species", kind: apitypes.ResourceKindPetSpecies},
		{name: "workspace", kind: apitypes.ResourceKindWorkspace},
		{name: "workflow", kind: apitypes.ResourceKindWorkflow},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manager.Delete(context.Background(), tc.kind, "example")
			assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")
		})
	}
}

func TestDeleteReturnsNotFoundByKind(t *testing.T) {
	tests := []struct {
		name    string
		kind    apitypes.ResourceKind
		manager *Manager
	}{
		{name: "acl policy binding", kind: apitypes.ResourceKindACLPolicyBinding, manager: newACLResourceManager(t)},
		{name: "acl role", kind: apitypes.ResourceKindACLRole, manager: newACLResourceManager(t)},
		{name: "credential", kind: apitypes.ResourceKindCredential, manager: New(Services{Credentials: newFakeCredentials()})},
		{name: "firmware", kind: apitypes.ResourceKindFirmware, manager: New(Services{Firmwares: &firmware.Server{Store: kv.NewMemory(nil)}})},
		{name: "badge", kind: apitypes.ResourceKindBadge, manager: New(Services{Badges: &badge.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())}})},
		{name: "model", kind: apitypes.ResourceKindModel, manager: newModelManager()},
		{name: "minimax tenant", kind: apitypes.ResourceKindMiniMaxTenant, manager: New(Services{ProviderTenants: newFakeMiniMax(),
			Voices: newFakeMiniMax()})},
		{name: "voice", kind: apitypes.ResourceKindVoice, manager: New(Services{ProviderTenants: newFakeMiniMax(),
			Voices: newFakeMiniMax()})},
		{name: "pet species", kind: apitypes.ResourceKindPetSpecies, manager: New(Services{PetSpecies: &petspecies.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())}})},
		{name: "workspace", kind: apitypes.ResourceKindWorkspace, manager: New(Services{Workspaces: newFakeWorkspaces()})},
		{name: "workflow", kind: apitypes.ResourceKindWorkflow, manager: New(Services{Workflows: newFakeWorkflows()})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.manager.Delete(context.Background(), tc.kind, "missing")
			assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
		})
	}
}

func TestDeleteRemovesResourcesByKind(t *testing.T) {
	credentials := newFakeCredentials()
	models := newModelManager().services.Models
	minimax := newFakeMiniMax()
	workspaces := newFakeWorkspaces()
	workflows := newFakeWorkflows()
	manager := New(Services{
		Credentials:     credentials,
		Firmwares:       &firmware.Server{Store: kv.NewMemory(nil)},
		Badges:          &badge.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())},
		Models:          models,
		PetSpecies:      &petspecies.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())},
		ProviderTenants: minimax,
		Voices:          minimax,
		Workspaces:      workspaces,
		Workflows:       workflows,
	})

	for _, resource := range []apitypes.Resource{
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Credential","metadata":{"name":"credential"},"spec":{"provider":"minimax","body":{"api_key":"secret"}}}`),
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Firmware","metadata":{"name":"firmware"},"spec":{"slots":{"stable":{"description":"stable firmware"}}}}`),
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Badge","metadata":{"name":"badge"},"spec":{"name":"Badge"}}`),
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"name":"model"},"spec":{"kind":"llm","provider":{"kind":"openai-tenant","name":"main"},"source":"manual"}}`),
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"MiniMaxTenant","metadata":{"name":"tenant"},"spec":{"app_id":"app","group_id":"group","credential_name":"credential"}}`),
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Voice","metadata":{"name":"voice"},"spec":{"provider":{"kind":"minimax","name":"tenant"},"source":"manual"}}`),
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"PetSpecies","metadata":{"name":"species"},"spec":{"name":"Species"}}`),
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workflow","metadata":{"name":"workflow"},"spec":{"driver":"flowcraft","flowcraft":{}}}`),
		mustResource(t, `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workspace","metadata":{"name":"workspace"},"spec":{"workflow_name":"workflow"}}`),
	} {
		if _, err := manager.Put(context.Background(), resource); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}

	tests := []struct {
		kind apitypes.ResourceKind
		name string
	}{
		{apitypes.ResourceKindCredential, "credential"},
		{apitypes.ResourceKindFirmware, "firmware"},
		{apitypes.ResourceKindBadge, "badge"},
		{apitypes.ResourceKindModel, "model"},
		{apitypes.ResourceKindMiniMaxTenant, "tenant"},
		{apitypes.ResourceKindVoice, "voice"},
		{apitypes.ResourceKindPetSpecies, "species"},
		{apitypes.ResourceKindWorkspace, "workspace"},
		{apitypes.ResourceKindWorkflow, "workflow"},
	}
	for _, tc := range tests {
		t.Run(string(tc.kind), func(t *testing.T) {
			if _, err := manager.Delete(context.Background(), tc.kind, tc.name); err != nil {
				t.Fatalf("Delete() error = %v", err)
			}
			_, err := manager.Get(context.Background(), tc.kind, tc.name)
			assertResourceError(t, err, 404, "RESOURCE_NOT_FOUND")
		})
	}
}

func TestApplyRejectsNilManager(t *testing.T) {
	var manager *Manager

	_, err := manager.Apply(context.Background(), mustResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "example"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`))
	assertResourceError(t, err, 500, "RESOURCE_MANAGER_NOT_CONFIGURED")
}

func TestApplyRejectsMissingServicesByKind(t *testing.T) {
	manager := New(Services{})
	tests := []struct {
		name     string
		resource string
	}{
		{name: "acl policy binding", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"ACLPolicyBinding","metadata":{"name":"binding"},"spec":{"subject":{"kind":"pk","id":"peer"},"resource":{"kind":"workspace","id":"workspace"},"role":"role"}}`},
		{name: "acl role", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"ACLRole","metadata":{"name":"role"},"spec":{"permissions":["workspace.read"]}}`},
		{name: "credential", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Credential","metadata":{"name":"name"},"spec":{"provider":"minimax","body":{"api_key":"secret"}}}`},
		{name: "firmware", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Firmware","metadata":{"name":"firmware"},"spec":{"slots":{"stable":{"description":"stable firmware"}}}}`},
		{name: "badge", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Badge","metadata":{"name":"badge"},"spec":{"name":"Badge"}}`},
		{name: "peer config", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"PeerConfig","metadata":{"name":"peer"},"spec":{}}`},
		{name: "model", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"name":"model"},"spec":{"kind":"llm","provider":{"kind":"openai-tenant","name":"main"},"source":"manual"}}`},
		{name: "dashscope tenant", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"DashScopeTenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "gemini tenant", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"GeminiTenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "openai tenant", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"OpenAITenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "minimax tenant", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"MiniMaxTenant","metadata":{"name":"tenant"},"spec":{"app_id":"app","group_id":"group","credential_name":"credential"}}`},
		{name: "voice", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Voice","metadata":{"name":"voice"},"spec":{"provider":{"kind":"minimax","name":"tenant"},"source":"manual"}}`},
		{name: "pet species", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"PetSpecies","metadata":{"name":"species"},"spec":{"name":"Species"}}`},
		{name: "workspace", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workspace","metadata":{"name":"workspace"},"spec":{"workflow_name":"workflow"}}`},
		{name: "workflow", resource: `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workflow","metadata":{"name":"workflow"},"spec":{"driver":"flowcraft","flowcraft":{}}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manager.Apply(context.Background(), mustResource(t, tc.resource))
			assertResourceError(t, err, 500, "RESOURCE_SERVICE_NOT_CONFIGURED")
		})
	}
}

func TestApplyRejectsUnsupportedVersionByKind(t *testing.T) {
	aclManager := newACLResourceManager(t)
	manager := New(Services{
		ACL:             aclManager.services.ACL,
		Credentials:     newFakeCredentials(),
		Firmwares:       &firmware.Server{Store: kv.NewMemory(nil)},
		Badges:          &badge.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())},
		Peers:           newFakePeers(),
		Models:          newModelManager().services.Models,
		PetSpecies:      &petspecies.Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())},
		ProviderTenants: newFakeMiniMax(),
		Voices:          newFakeMiniMax(),
		Workspaces:      newFakeWorkspaces(),
		Workflows:       newFakeWorkflows(),
	})
	tests := []struct {
		name     string
		resource string
	}{
		{name: "acl policy binding", resource: `{"apiVersion":"unsupported","kind":"ACLPolicyBinding","metadata":{"name":"binding"},"spec":{"subject":{"kind":"pk","id":"peer"},"resource":{"kind":"workspace","id":"workspace"},"role":"role"}}`},
		{name: "acl role", resource: `{"apiVersion":"unsupported","kind":"ACLRole","metadata":{"name":"role"},"spec":{"permissions":["workspace.read"]}}`},
		{name: "credential", resource: `{"apiVersion":"unsupported","kind":"Credential","metadata":{"name":"name"},"spec":{"provider":"minimax","body":{"api_key":"secret"}}}`},
		{name: "firmware", resource: `{"apiVersion":"unsupported","kind":"Firmware","metadata":{"name":"firmware"},"spec":{"slots":{"stable":{"description":"stable firmware"}}}}`},
		{name: "badge", resource: `{"apiVersion":"unsupported","kind":"Badge","metadata":{"name":"badge"},"spec":{"name":"Badge"}}`},
		{name: "peer config", resource: `{"apiVersion":"unsupported","kind":"PeerConfig","metadata":{"name":"peer"},"spec":{}}`},
		{name: "model", resource: `{"apiVersion":"unsupported","kind":"Model","metadata":{"name":"model"},"spec":{"kind":"llm","provider":{"kind":"openai-tenant","name":"main"},"source":"manual"}}`},
		{name: "dashscope tenant", resource: `{"apiVersion":"unsupported","kind":"DashScopeTenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "gemini tenant", resource: `{"apiVersion":"unsupported","kind":"GeminiTenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "openai tenant", resource: `{"apiVersion":"unsupported","kind":"OpenAITenant","metadata":{"name":"tenant"},"spec":{"credential_name":"credential"}}`},
		{name: "minimax tenant", resource: `{"apiVersion":"unsupported","kind":"MiniMaxTenant","metadata":{"name":"tenant"},"spec":{"app_id":"app","group_id":"group","credential_name":"credential"}}`},
		{name: "resource list", resource: `{"apiVersion":"unsupported","kind":"ResourceList","metadata":{"name":"bundle"},"spec":{"items":[]}}`},
		{name: "voice", resource: `{"apiVersion":"unsupported","kind":"Voice","metadata":{"name":"voice"},"spec":{"provider":{"kind":"minimax","name":"tenant"},"source":"manual"}}`},
		{name: "pet species", resource: `{"apiVersion":"unsupported","kind":"PetSpecies","metadata":{"name":"species"},"spec":{"name":"Species"}}`},
		{name: "workspace", resource: `{"apiVersion":"unsupported","kind":"Workspace","metadata":{"name":"workspace"},"spec":{"workflow_name":"workflow"}}`},
		{name: "workflow", resource: `{"apiVersion":"unsupported","kind":"Workflow","metadata":{"name":"workflow"},"spec":{"driver":"flowcraft","flowcraft":{}}}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := manager.Apply(context.Background(), mustResource(t, tc.resource))
			assertResourceError(t, err, 400, "UNSUPPORTED_RESOURCE_VERSION")
		})
	}
}

func assertResourceError(t *testing.T, err error, statusCode int, code string) {
	t.Helper()
	resourceErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("error = %T %v, want *Error", err, err)
	}
	if resourceErr.StatusCode != statusCode {
		t.Fatalf("StatusCode = %d, want %d", resourceErr.StatusCode, statusCode)
	}
	if resourceErr.Code != code {
		t.Fatalf("Code = %q, want %q", resourceErr.Code, code)
	}
}
