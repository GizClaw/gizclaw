package resourcemanager

import (
	"context"
	"errors"
	"fmt"
	"sync"

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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Services groups the admin services that own concrete resource writes.
type Services struct {
	Assets          *asset.Service
	ACL             *acl.Server
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
}

// Manager applies declarative admin resources by delegating to owner services.
type Manager struct {
	services     Services
	assetWriteMu sync.Mutex
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
	case apitypes.ResourceKindACLPolicyBinding:
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, exists, err := m.getACLPolicyBinding(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromACLPolicyBinding(item)
	case apitypes.ResourceKindACLRole:
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, exists, err := m.getACLRole(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromACLRole(item)
	case apitypes.ResourceKindACLView:
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, exists, err := m.getACLView(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		return resourceFromACLView(item)
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
		resource, err := resourceFromCredential(item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return m.withOwnedResourceOwner(ctx, apitypes.ACLResourceKindCredential, name, resource)
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
		resource, err := resourceFromFirmware(item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return m.withOwnedResourceOwner(ctx, apitypes.ACLResourceKindFirmware, name, resource)
	case apitypes.ResourceKindPeerConfig:
		if m.services.Peers == nil {
			return apitypes.Resource{}, missingService("peers")
		}
		item, err := m.getPeerConfig(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		return resourceFromPeerConfig(name, item)
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
		resource, err := resourceFromModel(item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return m.withOwnedResourceOwner(ctx, apitypes.ACLResourceKindModel, name, resource)
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
		resource, err := resourceFromTool(item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return m.withOwnedResourceOwner(ctx, apitypes.ACLResourceKindTool, name, resource)
	case apitypes.ResourceKindGameRuleset:
		item, exists, err := m.getGameRuleset(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if !exists {
			return apitypes.Resource{}, notFound(kind, name)
		}
		resource, err := resourceFromGameRuleset(item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return m.withOwnedResourceOwner(ctx, apitypes.ACLResourceKindGameruleset, name, resource)
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
		resource, err := resourceFromVoice(item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return m.withOwnedResourceOwner(ctx, apitypes.ACLResourceKindVoice, name, resource)
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
		resource, err := resourceFromWorkspace(item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return m.withOwnedResourceOwner(ctx, apitypes.ACLResourceKindWorkspace, name, resource)
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
		resource, err := resourceFromWorkflow(name, item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return m.withOwnedResourceOwner(ctx, apitypes.ACLResourceKindWorkflow, name, resource)
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

func (m *Manager) put(ctx context.Context, resource apitypes.Resource) (apitypes.Resource, error) {
	if m == nil {
		return apitypes.Resource{}, applyError(500, "RESOURCE_MANAGER_NOT_CONFIGURED", "resource manager is not configured")
	}
	kind, err := resource.Discriminator()
	if err != nil {
		return apitypes.Resource{}, applyError(400, "INVALID_RESOURCE", err.Error())
	}
	switch kind {
	case string(apitypes.ResourceKindACLPolicyBinding), "ACLPolicyBindingResource":
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, err := resource.AsACLPolicyBindingResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_ACL_POLICY_BINDING_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if _, err := m.services.ACL.PutPolicyBinding(ctx, string(pathParam(item.Metadata.Name)), 0, item.Spec); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindACLPolicyBinding, item.Metadata.Name)
	case string(apitypes.ResourceKindACLRole), "ACLRoleResource":
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, err := resource.AsACLRoleResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_ACL_ROLE_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if _, err := m.services.ACL.PutRole(ctx, string(pathParam(item.Metadata.Name)), item.Spec.Permissions); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindACLRole, item.Metadata.Name)
	case string(apitypes.ResourceKindACLView), "ACLViewResource":
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, err := resource.AsACLViewResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_ACL_VIEW_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if _, err := m.services.ACL.PutView(ctx, string(pathParam(item.Metadata.Name)), item.Spec); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindACLView, item.Metadata.Name)
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
		_, exists, err := m.getCredential(ctx, string(pathParam(item.Metadata.Name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindCredential, item.Metadata.Name, item.Metadata, exists); err != nil {
			return apitypes.Resource{}, err
		}
		ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindCredential, item.Metadata.Name, item.Metadata)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putCredential(ctx, string(pathParam(item.Metadata.Name)), credentialUpsert(item)); err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
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
		_, exists, err := m.getFirmware(ctx, string(pathParam(item.Metadata.Name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindFirmware, item.Metadata.Name, item.Metadata, exists); err != nil {
			return apitypes.Resource{}, err
		}
		ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindFirmware, item.Metadata.Name, item.Metadata)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putFirmware(ctx, string(pathParam(item.Metadata.Name)), firmwareUpsert(item)); err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		return m.Get(ctx, apitypes.ResourceKindFirmware, item.Metadata.Name)
	case string(apitypes.ResourceKindPeerConfig), "PeerConfigResource":
		if m.services.Peers == nil {
			return apitypes.Resource{}, missingService("peers")
		}
		item, err := resource.AsPeerConfigResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_PEER_CONFIG_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putPeerConfig(ctx, string(pathParam(item.Metadata.Name)), item.Spec); err != nil {
			return apitypes.Resource{}, err
		}
		return m.Get(ctx, apitypes.ResourceKindPeerConfig, item.Metadata.Name)
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
		_, exists, err := m.getModel(ctx, string(pathParam(item.Metadata.Name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindModel, item.Metadata.Name, item.Metadata, exists); err != nil {
			return apitypes.Resource{}, err
		}
		ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindModel, item.Metadata.Name, item.Metadata)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putModel(ctx, string(pathParam(item.Metadata.Name)), modelUpsert(item)); err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
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
	case string(apitypes.ResourceKindGameRuleset), "GameRulesetResource":
		item, err := resource.AsGameRulesetResource()
		if err != nil {
			return apitypes.Resource{}, applyError(400, "INVALID_GAME_RULESET_RESOURCE", err.Error())
		}
		if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
			return apitypes.Resource{}, err
		}
		_, exists, err := m.getGameRuleset(ctx, string(pathParam(item.Metadata.Name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindGameruleset, item.Metadata.Name, item.Metadata, exists); err != nil {
			return apitypes.Resource{}, err
		}
		ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindGameruleset, item.Metadata.Name, item.Metadata)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putGameRuleset(ctx, string(pathParam(item.Metadata.Name)), gameRulesetUpsert(item)); err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		return m.Get(ctx, apitypes.ResourceKindGameRuleset, item.Metadata.Name)
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
		_, exists, err := m.getVoice(ctx, string(pathParam(item.Metadata.Name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindVoice, item.Metadata.Name, item.Metadata, exists); err != nil {
			return apitypes.Resource{}, err
		}
		ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindVoice, item.Metadata.Name, item.Metadata)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putVoice(ctx, string(pathParam(item.Metadata.Name)), voiceUpsert(item)); err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
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
		_, exists, err := m.getWorkspace(ctx, string(pathParam(item.Metadata.Name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindWorkspace, item.Metadata.Name, item.Metadata, exists); err != nil {
			return apitypes.Resource{}, err
		}
		ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindWorkspace, item.Metadata.Name, item.Metadata)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putWorkspace(ctx, string(pathParam(item.Metadata.Name)), workspaceUpsert(item)); err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
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
		_, exists, err := m.getWorkflow(ctx, string(pathParam(item.Metadata.Name)))
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindWorkflow, item.Metadata.Name, item.Metadata, exists); err != nil {
			return apitypes.Resource{}, err
		}
		ownerRollback, err := m.ensureOwnedResourceOwnerBeforeWrite(ctx, apitypes.ACLResourceKindWorkflow, item.Metadata.Name, item.Metadata)
		if err != nil {
			return apitypes.Resource{}, err
		}
		if err := m.putWorkflow(ctx, string(pathParam(item.Metadata.Name)), workflowFromResource(item)); err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
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

func (m *Manager) delete(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
	if m == nil {
		return apitypes.Resource{}, applyError(500, "RESOURCE_MANAGER_NOT_CONFIGURED", "resource manager is not configured")
	}
	if name == "" {
		return apitypes.Resource{}, applyError(400, "INVALID_RESOURCE", "metadata.name is required")
	}
	switch kind {
	case apitypes.ResourceKindACLPolicyBinding:
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, err := m.services.ACL.DeletePolicyBinding(ctx, string(pathParam(name)))
		if err != nil {
			if errors.Is(err, acl.ErrPolicyBindingNotFound) {
				return apitypes.Resource{}, notFound(kind, name)
			}
			return apitypes.Resource{}, err
		}
		return resourceFromACLPolicyBinding(item)
	case apitypes.ResourceKindACLRole:
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, err := m.services.ACL.DeleteRole(ctx, string(pathParam(name)))
		if err != nil {
			if errors.Is(err, acl.ErrRoleNotFound) {
				return apitypes.Resource{}, notFound(kind, name)
			}
			return apitypes.Resource{}, err
		}
		return resourceFromACLRole(item)
	case apitypes.ResourceKindACLView:
		if m.services.ACL == nil {
			return apitypes.Resource{}, missingService("acl")
		}
		item, err := m.services.ACL.DeleteView(ctx, string(pathParam(name)))
		if err != nil {
			if errors.Is(err, acl.ErrViewNotFound) {
				return apitypes.Resource{}, notFound(kind, name)
			}
			return apitypes.Resource{}, err
		}
		return resourceFromACLView(item)
	case apitypes.ResourceKindCredential:
		if m.services.Credentials == nil {
			return apitypes.Resource{}, missingService("credentials")
		}
		ownerRollback, err := m.removeOwnedResourceOwnerBeforeDelete(ctx, apitypes.ACLResourceKindCredential, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, exists, err := m.deleteCredential(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		if !exists {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, notFound(kind, name))
		}
		return resourceFromCredential(item)
	case apitypes.ResourceKindFirmware:
		if m.services.Firmwares == nil {
			return apitypes.Resource{}, missingService("firmwares")
		}
		ownerRollback, err := m.removeOwnedResourceOwnerBeforeDelete(ctx, apitypes.ACLResourceKindFirmware, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, exists, err := m.deleteFirmware(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		if !exists {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, notFound(kind, name))
		}
		return resourceFromFirmware(item)
	case apitypes.ResourceKindPeerConfig:
		return apitypes.Resource{}, applyError(400, "UNSUPPORTED_RESOURCE_DELETE", "PeerConfig cannot be deleted independently")
	case apitypes.ResourceKindModel:
		if m.services.Models == nil {
			return apitypes.Resource{}, missingService("models")
		}
		ownerRollback, err := m.removeOwnedResourceOwnerBeforeDelete(ctx, apitypes.ACLResourceKindModel, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, exists, err := m.deleteModel(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		if !exists {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, notFound(kind, name))
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
	case apitypes.ResourceKindGameRuleset:
		ownerRollback, err := m.removeOwnedResourceOwnerBeforeDelete(ctx, apitypes.ACLResourceKindGameruleset, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, exists, err := m.deleteGameRuleset(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		if !exists {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, notFound(kind, name))
		}
		return resourceFromGameRuleset(item)
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
		ownerRollback, err := m.removeOwnedResourceOwnerBeforeDelete(ctx, apitypes.ACLResourceKindVoice, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, exists, err := m.deleteVoice(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		if !exists {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, notFound(kind, name))
		}
		return resourceFromVoice(item)
	case apitypes.ResourceKindWorkspace:
		if m.services.Workspaces == nil {
			return apitypes.Resource{}, missingService("workspaces")
		}
		ownerRollback, err := m.removeOwnedResourceOwnerBeforeDelete(ctx, apitypes.ACLResourceKindWorkspace, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, exists, err := m.deleteWorkspace(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		if !exists {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, notFound(kind, name))
		}
		return resourceFromWorkspace(item)
	case apitypes.ResourceKindWorkflow:
		if m.services.Workflows == nil {
			return apitypes.Resource{}, missingService("workflows")
		}
		ownerRollback, err := m.removeOwnedResourceOwnerBeforeDelete(ctx, apitypes.ACLResourceKindWorkflow, name)
		if err != nil {
			return apitypes.Resource{}, err
		}
		item, exists, err := m.deleteWorkflow(ctx, string(pathParam(name)))
		if err != nil {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, err)
		}
		if !exists {
			return apitypes.Resource{}, m.rollbackOwnedResourceOwner(ctx, ownerRollback, notFound(kind, name))
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

func (m *Manager) apply(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m == nil {
		return apitypes.ApplyResult{}, applyError(500, "RESOURCE_MANAGER_NOT_CONFIGURED", "resource manager is not configured")
	}
	kind, err := resource.Discriminator()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE", err.Error())
	}
	switch kind {
	case string(apitypes.ResourceKindACLPolicyBinding), "ACLPolicyBindingResource":
		return m.applyACLPolicyBinding(ctx, resource)
	case string(apitypes.ResourceKindACLRole), "ACLRoleResource":
		return m.applyACLRole(ctx, resource)
	case string(apitypes.ResourceKindACLView), "ACLViewResource":
		return m.applyACLView(ctx, resource)
	case string(apitypes.ResourceKindCredential), "CredentialResource":
		return m.applyCredential(ctx, resource)
	case string(apitypes.ResourceKindFirmware), "FirmwareResource":
		return m.applyFirmware(ctx, resource)
	case string(apitypes.ResourceKindPeerConfig), "PeerConfigResource":
		return m.applyPeerConfig(ctx, resource)
	case string(apitypes.ResourceKindDashScopeTenant), "DashScopeTenantResource":
		return m.applyDashScopeTenant(ctx, resource)
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
	case string(apitypes.ResourceKindGameRuleset), "GameRulesetResource":
		return m.applyGameRuleset(ctx, resource)
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
