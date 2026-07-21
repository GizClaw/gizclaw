package resourcemanager

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/credential"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/providertenants"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/firmware"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Services groups the admin services that own concrete resource writes.
type Services struct {
	Credentials     credential.CredentialAdminService
	Firmwares       firmware.FirmwareAdminService
	Peers           peer.PeerAdminService
	Models          model.ModelAdminService
	ProviderTenants providertenants.ProviderTenantsAdminService
	Voices          voice.VoiceAdminService
	Workspaces      workspace.WorkspaceAdminService
	Workflows       workflow.WorkflowAdminService
	Contacts        *contact.Server
	Friends         *friend.Server
	FriendGroups    *friendgroup.Server
	GameplayCatalog gameplay.CatalogAdminService
	Tools           *toolkit.Server
	RuntimeProfiles *runtimeprofile.Server
}

// Manager applies declarative admin resources by delegating to owner services.
type Manager struct {
	services Services
}

// Error is returned for apply failures that should map cleanly to HTTP later.
type Error struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// New creates a resource manager using the provided owner services.
func New(services Services) *Manager {
	return &Manager{services: services}
}

// Get loads the current state of a named resource and returns it as a declarative resource.
func (m *Manager) Get(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
	if m == nil {
		return apitypes.Resource{}, applyError(500, "RESOURCE_MANAGER_NOT_CONFIGURED", "resource manager is not configured")
	}
	if name == "" {
		return apitypes.Resource{}, applyError(400, "INVALID_RESOURCE", "metadata.name is required")
	}
	switch kind {
	case apitypes.ResourceKindCredential:
		if m.services.Credentials == nil {
			return apitypes.Resource{}, missingService("credentials")
		}
		item, exists, err := m.getCredential(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromCredential(item)
	case apitypes.ResourceKindFirmware:
		if m.services.Firmwares == nil {
			return apitypes.Resource{}, missingService("firmwares")
		}
		item, exists, err := m.getFirmware(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromFirmware(item)
	case apitypes.ResourceKindRuntimeProfile:
		item, exists, err := m.getRuntimeProfile(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromRuntimeProfile(item)
	case apitypes.ResourceKindRegistrationToken:
		item, exists, err := m.getRegistrationToken(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromRegistrationToken(item, nil)
	case apitypes.ResourceKindModel:
		if m.services.Models == nil {
			return apitypes.Resource{}, missingService("models")
		}
		item, exists, err := m.getModel(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromModel(item)
	case apitypes.ResourceKindTool:
		if m.services.Tools == nil {
			return apitypes.Resource{}, missingService("tools")
		}
		item, exists, err := m.getTool(ctx, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromTool(item)
	case apitypes.ResourceKindPetDef:
		item, exists, err := m.getPetDef(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromPetDef(item)
	case apitypes.ResourceKindBadgeDef:
		item, exists, err := m.getBadgeDef(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromBadgeDef(item)
	case apitypes.ResourceKindGameDef:
		item, exists, err := m.getGameDef(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromGameDef(item)
	case apitypes.ResourceKindDashScopeTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.getDashScopeTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromDashScopeTenant(item)
	case apitypes.ResourceKindDeepSeekTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.getDeepSeekTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromDeepSeekTenant(item)
	case apitypes.ResourceKindMiniMaxTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.getMiniMaxTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromMiniMaxTenant(item)
	case apitypes.ResourceKindGeminiTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.getGeminiTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromGeminiTenant(item)
	case apitypes.ResourceKindOpenAITenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.getOpenAITenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromOpenAITenant(item)
	case apitypes.ResourceKindVolcTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.getVolcTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromVolcTenant(item)
	case apitypes.ResourceKindVoice:
		if m.services.Voices == nil {
			return apitypes.Resource{}, missingService("voices")
		}
		item, exists, err := m.getVoice(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromVoice(item)
	case apitypes.ResourceKindWorkspace:
		if m.services.Workspaces == nil {
			return apitypes.Resource{}, missingService("workspaces")
		}
		item, exists, err := m.getWorkspace(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromWorkspace(item)
	case apitypes.ResourceKindWorkflow:
		if m.services.Workflows == nil {
			return apitypes.Resource{}, missingService("workflows")
		}
		item, exists, err := m.getWorkflow(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromWorkflow(name, item)
	case apitypes.ResourceKindResourceList:
		return apitypes.Resource{}, applyError(400, "UNSUPPORTED_RESOURCE_GET", "ResourceList is not stored as a named resource")
	case apitypes.ResourceKindFriend:
		if m.services.Friends == nil {
			return apitypes.Resource{}, missingService("friends")
		}
		item, exists, err := m.getFriend(ctx, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromFriend(item)
	case apitypes.ResourceKindContact:
		if m.services.Contacts == nil {
			return apitypes.Resource{}, missingService("contacts")
		}
		item, exists, err := m.getContact(ctx, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromContact(item)
	case apitypes.ResourceKindFriendGroup:
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		item, exists, err := m.getFriendGroup(ctx, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromFriendGroup(item)
	case apitypes.ResourceKindFriendGroupInviteToken:
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		item, exists, err := m.getFriendGroupInviteToken(ctx, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromFriendGroupInviteToken(name, item)
	case apitypes.ResourceKindFriendGroupMember:
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		item, exists, err := m.getFriendGroupMember(ctx, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromFriendGroupMember(item)
	default:
		return apitypes.Resource{}, applyError(400, "UNKNOWN_RESOURCE_KIND", fmt.Sprintf("unknown resource kind %q", kind))
	}
}

// Put writes the provided resource and returns the stored resource state.
func (m *Manager) Put(ctx context.Context, resource apitypes.Resource) (apitypes.Resource, error) {
	if m == nil {
		return apitypes.Resource{}, applyError(500, "RESOURCE_MANAGER_NOT_CONFIGURED", "resource manager is not configured")
	}
	kind, err := resource.Discriminator()
	if err != nil {
		return apitypes.Resource{}, applyError(400, "INVALID_RESOURCE", err.Error())
	}
	switch kind {
	case string(apitypes.ResourceKindCredential), "CredentialResource":
		if m.services.Credentials == nil {
			return apitypes.Resource{}, missingService("credentials")
		}
		item, err := resource.AsCredentialResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_CREDENTIAL_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putCredential(ctx, string(pathParam(item.Metadata.Name)), credentialUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindCredential, item.Metadata.Name)
	case string(apitypes.ResourceKindFirmware), "FirmwareResource":
		if m.services.Firmwares == nil {
			return apitypes.Resource{}, missingService("firmwares")
		}
		item, err := resource.AsFirmwareResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_FIRMWARE_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putFirmware(ctx, string(pathParam(item.Metadata.Name)), firmwareUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindFirmware, item.Metadata.Name)
	case string(apitypes.ResourceKindRuntimeProfile), "RuntimeProfileResource":
		item, err := resource.AsRuntimeProfileResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_RUNTIME_PROFILE_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putRuntimeProfile(ctx, item.Metadata.Name, item.Spec); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindRuntimeProfile, item.Metadata.Name)
	case string(apitypes.ResourceKindRegistrationToken), "RegistrationTokenResource":
		item, err := resource.AsRegistrationTokenResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_REGISTRATION_TOKEN_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		return m.putRegistrationToken(ctx, item)
	case string(apitypes.ResourceKindDashScopeTenant), "DashScopeTenantResource":
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, err := resource.AsDashScopeTenantResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_DASHSCOPE_TENANT_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putDashScopeTenant(ctx, string(pathParam(item.Metadata.Name)), dashScopeTenantUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindDashScopeTenant, item.Metadata.Name)
	case string(apitypes.ResourceKindDeepSeekTenant), "DeepSeekTenantResource":
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, err := resource.AsDeepSeekTenantResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_DEEPSEEK_TENANT_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putDeepSeekTenant(ctx, string(pathParam(item.Metadata.Name)), deepSeekTenantUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindDeepSeekTenant, item.Metadata.Name)
	case string(apitypes.ResourceKindMiniMaxTenant), "MiniMaxTenantResource":
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, err := resource.AsMiniMaxTenantResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_MINIMAX_TENANT_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putMiniMaxTenant(ctx, string(pathParam(item.Metadata.Name)), miniMaxTenantUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindMiniMaxTenant, item.Metadata.Name)
	case string(apitypes.ResourceKindGeminiTenant), "GeminiTenantResource":
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, err := resource.AsGeminiTenantResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_GEMINI_TENANT_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putGeminiTenant(ctx, string(pathParam(item.Metadata.Name)), geminiTenantUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindGeminiTenant, item.Metadata.Name)
	case string(apitypes.ResourceKindOpenAITenant), "OpenAITenantResource":
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, err := resource.AsOpenAITenantResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_OPENAI_TENANT_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putOpenAITenant(ctx, string(pathParam(item.Metadata.Name)), openAITenantUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindOpenAITenant, item.Metadata.Name)
	case string(apitypes.ResourceKindModel), "ModelResource":
		if m.services.Models == nil {
			return apitypes.Resource{}, missingService("models")
		}
		item, err := resource.AsModelResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_MODEL_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putModel(ctx, string(pathParam(item.Metadata.Name)), modelUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindModel, item.Metadata.Name)
	case string(apitypes.ResourceKindTool), "ToolResource":
		if m.services.Tools == nil {
			return apitypes.Resource{}, missingService("tools")
		}
		item, err := resource.AsToolResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_TOOL_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		return m.putToolResource(ctx, item)
	case string(apitypes.ResourceKindPetDef), "PetDefResource":
		item, err := resource.AsPetDefResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_PET_DEF_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putPetDef(ctx, string(pathParam(item.Metadata.Name)), petDefUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindPetDef, item.Metadata.Name)
	case string(apitypes.ResourceKindBadgeDef), "BadgeDefResource":
		item, err := resource.AsBadgeDefResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_BADGE_DEF_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putBadgeDef(ctx, string(pathParam(item.Metadata.Name)), badgeDefUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindBadgeDef, item.Metadata.Name)
	case string(apitypes.ResourceKindGameDef), "GameDefResource":
		item, err := resource.AsGameDefResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_GAME_DEF_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putGameDef(ctx, string(pathParam(item.Metadata.Name)), gameDefUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindGameDef, item.Metadata.Name)
	case string(apitypes.ResourceKindVolcTenant), "VolcTenantResource":
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, err := resource.AsVolcTenantResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_VOLC_TENANT_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putVolcTenant(ctx, string(pathParam(item.Metadata.Name)), volcTenantUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindVolcTenant, item.Metadata.Name)
	case string(apitypes.ResourceKindResourceList), "ResourceListResource":
		list, err := resource.AsResourceListResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_RESOURCE_LIST", err.Error())
		}
		if err := validateResourceHeader(list.ApiVersion, list.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		items := make([]apitypes.Resource, 0, len(list.Spec.Items))
		for _, item := range list.Spec.Items {
			stored, err := m.Put(ctx, item)
			if err != nil {
				return apitypes.Resource{}, err
			}
			items = append(items, stored)
		}
		return resourceFromResourceList(list.Metadata.Name, items)
	case string(apitypes.ResourceKindVoice), "VoiceResource":
		if m.services.Voices == nil {
			return apitypes.Resource{}, missingService("voices")
		}
		item, err := resource.AsVoiceResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_VOICE_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putVoice(ctx, string(pathParam(item.Metadata.Name)), voiceUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindVoice, item.Metadata.Name)
	case string(apitypes.ResourceKindWorkspace), "WorkspaceResource":
		if m.services.Workspaces == nil {
			return apitypes.Resource{}, missingService("workspaces")
		}
		item, err := resource.AsWorkspaceResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_WORKSPACE_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putWorkspace(ctx, string(pathParam(item.Metadata.Name)), workspaceUpsert(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindWorkspace, item.Metadata.Name)
	case string(apitypes.ResourceKindWorkflow), "WorkflowResource":
		if m.services.Workflows == nil {
			return apitypes.Resource{}, missingService("workflows")
		}
		item, err := resource.AsWorkflowResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_WORKFLOW_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putWorkflow(ctx, string(pathParam(item.Metadata.Name)), workflowFromResource(item)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindWorkflow, item.Metadata.Name)
	case string(apitypes.ResourceKindFriend), "FriendResource":
		if m.services.Friends == nil {
			return apitypes.Resource{}, missingService("friends")
		}
		item, err := resource.AsFriendResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_FRIEND_RESOURCE", err.Error())
		}
		if err := validateFriendResource(item); err != nil {
			return apitypes.Resource{}, err
		}
		if _, err := m.services.Friends.AdminCreateFriendResource(ctx, item.Spec.OwnerPublicKey, item.Spec.PeerPublicKey); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindFriend, item.Metadata.Name)
	case string(apitypes.ResourceKindContact), "ContactResource":
		if m.services.Contacts == nil {
			return apitypes.Resource{}, missingService("contacts")
		}
		item, err := resource.AsContactResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_CONTACT_RESOURCE", err.Error())
		}
		if err := validateContactResource(item); err != nil {
			return apitypes.Resource{}, err
		}
		if _, err := m.services.Contacts.AdminApplyContact(ctx, item.Spec.OwnerPublicKey, item.Spec.Id, item.Spec.DisplayName, item.Spec.PhoneNumber); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindContact, item.Metadata.Name)
	case string(apitypes.ResourceKindFriendGroup), "FriendGroupResource":
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		item, err := resource.AsFriendGroupResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_FRIEND_GROUP_RESOURCE", err.Error())
		}
		if err := validateFriendGroupResource(item); err != nil {
			return apitypes.Resource{}, err
		}
		if _, err := m.services.FriendGroups.AdminApplyFriendGroup(ctx, item.Metadata.Name, item.Spec.Name, item.Spec.Description); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindFriendGroup, item.Metadata.Name)
	case string(apitypes.ResourceKindFriendGroupInviteToken), "FriendGroupInviteTokenResource":
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		item, err := resource.AsFriendGroupInviteTokenResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_FRIEND_GROUP_INVITE_TOKEN_RESOURCE", err.Error())
		}
		if err := validateFriendGroupInviteTokenResource(item); err != nil {
			return apitypes.Resource{}, err
		}
		if _, err := m.services.FriendGroups.AdminPutFriendGroupInviteToken(ctx, item.Spec.FriendGroupId, item.Spec.InviteToken, item.Spec.ExpiresAt); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindFriendGroupInviteToken, item.Metadata.Name)
	case string(apitypes.ResourceKindFriendGroupMember), "FriendGroupMemberResource":
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		item, err := resource.AsFriendGroupMemberResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_FRIEND_GROUP_MEMBER_RESOURCE", err.Error())
		}
		if err := validateFriendGroupMemberResource(item); err != nil {
			return apitypes.Resource{}, err
		}
		if _, err := m.services.FriendGroups.AdminPutFriendGroupMember(ctx, item.Spec.FriendGroupId, item.Spec.PeerPublicKey, rpcapi.FriendGroupMemberRole(item.Spec.Role)); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindFriendGroupMember, item.Metadata.Name)
	default:
		return apitypes.Resource{}, applyError(400, "UNKNOWN_RESOURCE_KIND", fmt.Sprintf("unknown resource kind %q", kind))
	}
}

// Delete removes a named concrete resource and returns the deleted resource state.
func (m *Manager) Delete(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
	if m == nil {
		return apitypes.Resource{}, applyError(500, "RESOURCE_MANAGER_NOT_CONFIGURED", "resource manager is not configured")
	}
	if name == "" {
		return apitypes.Resource{}, applyError(400, "INVALID_RESOURCE", "metadata.name is required")
	}
	switch kind {
	case apitypes.ResourceKindCredential:
		if m.services.Credentials == nil {
			return apitypes.Resource{}, missingService("credentials")
		}
		item, exists, err := m.deleteCredential(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromCredential(item)
	case apitypes.ResourceKindFirmware:
		if m.services.Firmwares == nil {
			return apitypes.Resource{}, missingService("firmwares")
		}
		item, exists, err := m.deleteFirmware(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromFirmware(item)
	case apitypes.ResourceKindRuntimeProfile:
		item, exists, err := m.deleteRuntimeProfile(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromRuntimeProfile(item)
	case apitypes.ResourceKindRegistrationToken:
		item, exists, err := m.deleteRegistrationToken(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromRegistrationToken(item, nil)
	case apitypes.ResourceKindModel:
		if m.services.Models == nil {
			return apitypes.Resource{}, missingService("models")
		}
		item, exists, err := m.deleteModel(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromModel(item)
	case apitypes.ResourceKindTool:
		if m.services.Tools == nil {
			return apitypes.Resource{}, missingService("tools")
		}
		item, exists, err := m.deleteTool(ctx, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromTool(item)
	case apitypes.ResourceKindPetDef:
		item, exists, err := m.deletePetDef(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromPetDef(item)
	case apitypes.ResourceKindBadgeDef:
		item, exists, err := m.deleteBadgeDef(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromBadgeDef(item)
	case apitypes.ResourceKindGameDef:
		item, exists, err := m.deleteGameDef(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromGameDef(item)
	case apitypes.ResourceKindDashScopeTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.deleteDashScopeTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromDashScopeTenant(item)
	case apitypes.ResourceKindDeepSeekTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.deleteDeepSeekTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromDeepSeekTenant(item)
	case apitypes.ResourceKindMiniMaxTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.deleteMiniMaxTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromMiniMaxTenant(item)
	case apitypes.ResourceKindGeminiTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.deleteGeminiTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromGeminiTenant(item)
	case apitypes.ResourceKindOpenAITenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.deleteOpenAITenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromOpenAITenant(item)
	case apitypes.ResourceKindVolcTenant:
		if m.services.ProviderTenants == nil {
			return apitypes.Resource{}, missingService("provider tenants")
		}
		item, exists, err := m.deleteVolcTenant(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromVolcTenant(item)
	case apitypes.ResourceKindVoice:
		if m.services.Voices == nil {
			return apitypes.Resource{}, missingService("voices")
		}
		item, exists, err := m.deleteVoice(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromVoice(item)
	case apitypes.ResourceKindWorkspace:
		if m.services.Workspaces == nil {
			return apitypes.Resource{}, missingService("workspaces")
		}
		item, exists, err := m.deleteWorkspace(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromWorkspace(item)
	case apitypes.ResourceKindWorkflow:
		if m.services.Workflows == nil {
			return apitypes.Resource{}, missingService("workflows")
		}
		item, exists, err := m.deleteWorkflow(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromWorkflow(name, item)
	case apitypes.ResourceKindResourceList:
		return apitypes.Resource{}, applyError(400, "UNSUPPORTED_RESOURCE_DELETE", "ResourceList is not stored as a named resource")
	case apitypes.ResourceKindFriend:
		if m.services.Friends == nil {
			return apitypes.Resource{}, missingService("friends")
		}
		owner, _, err := friendResourcePeers(name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, err := m.services.Friends.AdminDeleteFriend(ctx, owner, name)
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.Resource{}, notFound(kind, name)
		}
		if err != nil {
			return apitypes.Resource{}, err
		}
		return resourceFromFriend(item)
	case apitypes.ResourceKindContact:
		if m.services.Contacts == nil {
			return apitypes.Resource{}, missingService("contacts")
		}
		owner, id, err := contactResourceParts(name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, err := m.services.Contacts.AdminDeleteContact(ctx, owner, id)
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.Resource{}, notFound(kind, name)
		}
		if err != nil {
			return apitypes.Resource{}, err
		}
		return resourceFromContact(item)
	case apitypes.ResourceKindFriendGroup:
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		item, err := m.services.FriendGroups.AdminDeleteFriendGroup(ctx, name)
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.Resource{}, notFound(kind, name)
		}
		if err != nil {
			return apitypes.Resource{}, err
		}
		return resourceFromFriendGroup(item)
	case apitypes.ResourceKindFriendGroupInviteToken:
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		item, exists, err := m.getFriendGroupInviteToken(ctx, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		if _, err := m.services.FriendGroups.AdminDeleteFriendGroupInviteToken(ctx, name); err != nil {
			return apitypes.Resource{}, err
		}
		return resourceFromFriendGroupInviteToken(name, item)
	case apitypes.ResourceKindFriendGroupMember:
		if m.services.FriendGroups == nil {
			return apitypes.Resource{}, missingService("friend groups")
		}
		friendGroupID, peerID, err := friendGroupMemberResourceParts(name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, err := m.services.FriendGroups.AdminDeleteFriendGroupMember(ctx, friendGroupID, peerID)
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.Resource{}, notFound(kind, name)
		}
		if err != nil {
			return apitypes.Resource{}, err
		}
		return resourceFromFriendGroupMember(item)
	default:
		return apitypes.Resource{}, applyError(400, "UNKNOWN_RESOURCE_KIND", fmt.Sprintf("unknown resource kind %q", kind))
	}
}

// Apply creates, updates, or leaves unchanged the provided resource.
func (m *Manager) Apply(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m == nil {
		return apitypes.ApplyResult{}, applyError(500, "RESOURCE_MANAGER_NOT_CONFIGURED", "resource manager is not configured")
	}
	kind, err := resource.Discriminator()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE", err.Error())
	}
	switch kind {
	case string(apitypes.ResourceKindCredential), "CredentialResource":
		return m.applyCredential(ctx, resource)
	case string(apitypes.ResourceKindFirmware), "FirmwareResource":
		return m.applyFirmware(ctx, resource)
	case string(apitypes.ResourceKindRuntimeProfile), "RuntimeProfileResource":
		return m.applyRuntimeProfile(ctx, resource)
	case string(apitypes.ResourceKindRegistrationToken), "RegistrationTokenResource":
		return m.applyRegistrationToken(ctx, resource)
	case string(apitypes.ResourceKindDashScopeTenant), "DashScopeTenantResource":
		return m.applyDashScopeTenant(ctx, resource)
	case string(apitypes.ResourceKindDeepSeekTenant), "DeepSeekTenantResource":
		return m.applyDeepSeekTenant(ctx, resource)
	case string(apitypes.ResourceKindMiniMaxTenant), "MiniMaxTenantResource":
		return m.applyMiniMaxTenant(ctx, resource)
	case string(apitypes.ResourceKindGeminiTenant), "GeminiTenantResource":
		return m.applyGeminiTenant(ctx, resource)
	case string(apitypes.ResourceKindOpenAITenant), "OpenAITenantResource":
		return m.applyOpenAITenant(ctx, resource)
	case string(apitypes.ResourceKindModel), "ModelResource":
		return m.applyModel(ctx, resource)
	case string(apitypes.ResourceKindTool), "ToolResource":
		return m.applyTool(ctx, resource)
	case string(apitypes.ResourceKindPetDef), "PetDefResource":
		return m.applyPetDef(ctx, resource)
	case string(apitypes.ResourceKindBadgeDef), "BadgeDefResource":
		return m.applyBadgeDef(ctx, resource)
	case string(apitypes.ResourceKindGameDef), "GameDefResource":
		return m.applyGameDef(ctx, resource)
	case string(apitypes.ResourceKindVolcTenant), "VolcTenantResource":
		return m.applyVolcTenant(ctx, resource)
	case string(apitypes.ResourceKindResourceList), "ResourceListResource":
		return m.applyResourceList(ctx, resource)
	case string(apitypes.ResourceKindVoice), "VoiceResource":
		return m.applyVoice(ctx, resource)
	case string(apitypes.ResourceKindWorkspace), "WorkspaceResource":
		return m.applyWorkspace(ctx, resource)
	case string(apitypes.ResourceKindWorkflow), "WorkflowResource":
		return m.applyWorkflow(ctx, resource)
	case string(apitypes.ResourceKindFriend), "FriendResource":
		return m.applyFriend(ctx, resource)
	case string(apitypes.ResourceKindContact), "ContactResource":
		return m.applyContact(ctx, resource)
	case string(apitypes.ResourceKindFriendGroup), "FriendGroupResource":
		return m.applyFriendGroup(ctx, resource)
	case string(apitypes.ResourceKindFriendGroupInviteToken), "FriendGroupInviteTokenResource":
		return m.applyFriendGroupInviteToken(ctx, resource)
	case string(apitypes.ResourceKindFriendGroupMember), "FriendGroupMemberResource":
		return m.applyFriendGroupMember(ctx, resource)
	default:
		return apitypes.ApplyResult{}, applyError(400, "UNKNOWN_RESOURCE_KIND", fmt.Sprintf("unknown resource kind %q", kind))
	}
}
