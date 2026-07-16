package resourcemanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
)

// ResolveAssetOwner loads a complete declarative Resource and enumerates every
// canonical AssetRef in its current structure.
func (m *Manager) ResolveAssetOwner(ctx context.Context, owner asset.Owner) (asset.OwnerSnapshot, error) {
	resource, exists, err := m.assetOwnerResource(ctx, owner)
	if err != nil || !exists {
		return asset.OwnerSnapshot{Exists: exists}, err
	}
	data, err := json.Marshal(resource)
	if err != nil {
		return asset.OwnerSnapshot{}, fmt.Errorf("encode resource owner %s: %w", owner.ID, err)
	}
	return asset.OwnerSnapshot{Exists: true, Refs: assetRefsInJSON(data)}, nil
}

// ResourceHasDisplayAsset reports whether the current Resource displays
// structure contains ref. It deliberately excludes other string fields so the
// generic asset download RPC cannot expose domain-internal assets.
func (m *Manager) ResourceHasDisplayAsset(ctx context.Context, owner asset.Owner, ref asset.Ref) (bool, error) {
	resource, exists, err := m.assetOwnerResource(ctx, owner)
	if err != nil || !exists {
		return false, err
	}
	refs, err := resourceDisplayAssetRefs(resource)
	if err != nil {
		return false, fmt.Errorf("resolve display assets for resource owner %s: %w", owner.ID, err)
	}
	return slices.Contains(refs, ref), nil
}

// resourceDisplayAssetRefs enumerates public display AssetRefs through the
// generated Resource variants. Fields absent from the generated contract must
// not become downloadable merely because they appear in raw JSON.
func resourceDisplayAssetRefs(resource apitypes.Resource) ([]asset.Ref, error) {
	kind, err := resource.Discriminator()
	if err != nil {
		return nil, err
	}
	switch kind {
	case string(apitypes.ResourceKindACLPolicyBinding), "ACLPolicyBindingResource":
		return noDisplayAssetRefs(resource.AsACLPolicyBindingResource())
	case string(apitypes.ResourceKindACLRole), "ACLRoleResource":
		return noDisplayAssetRefs(resource.AsACLRoleResource())
	case string(apitypes.ResourceKindACLView), "ACLViewResource":
		return noDisplayAssetRefs(resource.AsACLViewResource())
	case string(apitypes.ResourceKindBadgeDef), "BadgeDefResource":
		return noDisplayAssetRefs(resource.AsBadgeDefResource())
	case string(apitypes.ResourceKindContact), "ContactResource":
		return noDisplayAssetRefs(resource.AsContactResource())
	case string(apitypes.ResourceKindCredential), "CredentialResource":
		return noDisplayAssetRefs(resource.AsCredentialResource())
	case string(apitypes.ResourceKindDashScopeTenant), "DashScopeTenantResource":
		return noDisplayAssetRefs(resource.AsDashScopeTenantResource())
	case string(apitypes.ResourceKindFirmware), "FirmwareResource":
		return noDisplayAssetRefs(resource.AsFirmwareResource())
	case string(apitypes.ResourceKindFriend), "FriendResource":
		return noDisplayAssetRefs(resource.AsFriendResource())
	case string(apitypes.ResourceKindFriendGroup), "FriendGroupResource":
		return noDisplayAssetRefs(resource.AsFriendGroupResource())
	case string(apitypes.ResourceKindFriendGroupInviteToken), "FriendGroupInviteTokenResource":
		return noDisplayAssetRefs(resource.AsFriendGroupInviteTokenResource())
	case string(apitypes.ResourceKindFriendGroupMember), "FriendGroupMemberResource":
		return noDisplayAssetRefs(resource.AsFriendGroupMemberResource())
	case string(apitypes.ResourceKindGameDef), "GameDefResource":
		return noDisplayAssetRefs(resource.AsGameDefResource())
	case string(apitypes.ResourceKindGameRuleset), "GameRulesetResource":
		return noDisplayAssetRefs(resource.AsGameRulesetResource())
	case string(apitypes.ResourceKindGeminiTenant), "GeminiTenantResource":
		return noDisplayAssetRefs(resource.AsGeminiTenantResource())
	case string(apitypes.ResourceKindMiniMaxTenant), "MiniMaxTenantResource":
		return noDisplayAssetRefs(resource.AsMiniMaxTenantResource())
	case string(apitypes.ResourceKindModel), "ModelResource":
		return noDisplayAssetRefs(resource.AsModelResource())
	case string(apitypes.ResourceKindOpenAITenant), "OpenAITenantResource":
		return noDisplayAssetRefs(resource.AsOpenAITenantResource())
	case string(apitypes.ResourceKindPeerConfig), "PeerConfigResource":
		return noDisplayAssetRefs(resource.AsPeerConfigResource())
	case string(apitypes.ResourceKindPetDef), "PetDefResource":
		return noDisplayAssetRefs(resource.AsPetDefResource())
	case string(apitypes.ResourceKindResourceList), "ResourceListResource":
		return noDisplayAssetRefs(resource.AsResourceListResource())
	case string(apitypes.ResourceKindTool), "ToolResource":
		return noDisplayAssetRefs(resource.AsToolResource())
	case string(apitypes.ResourceKindVoice), "VoiceResource":
		return noDisplayAssetRefs(resource.AsVoiceResource())
	case string(apitypes.ResourceKindVolcTenant), "VolcTenantResource":
		return noDisplayAssetRefs(resource.AsVolcTenantResource())
	case string(apitypes.ResourceKindWorkflow), "WorkflowResource":
		return noDisplayAssetRefs(resource.AsWorkflowResource())
	case string(apitypes.ResourceKindWorkspace), "WorkspaceResource":
		return noDisplayAssetRefs(resource.AsWorkspaceResource())
	default:
		return nil, fmt.Errorf("unsupported resource kind %q", kind)
	}
}

func noDisplayAssetRefs[T any](_ T, err error) ([]asset.Ref, error) {
	return nil, err
}

func (m *Manager) assetOwnerResource(ctx context.Context, owner asset.Owner) (apitypes.Resource, bool, error) {
	if owner.Kind != asset.OwnerKindResource {
		return apitypes.Resource{}, false, fmt.Errorf("unsupported resource owner kind %q", owner.Kind)
	}
	kindValue, name, ok := strings.Cut(owner.ID, "/")
	kind := apitypes.ResourceKind(kindValue)
	if !ok || !kind.Valid() || name == "" {
		return apitypes.Resource{}, false, fmt.Errorf("invalid resource owner id %q", owner.ID)
	}
	resource, err := m.Get(ctx, kind, name)
	if err != nil {
		var resourceErr *Error
		if errors.As(err, &resourceErr) && resourceErr.StatusCode == 404 {
			return apitypes.Resource{}, false, nil
		}
		return apitypes.Resource{}, false, err
	}
	return resource, true, nil
}

func assetRefsInJSON(data []byte) []asset.Ref {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	refs := make([]asset.Ref, 0)
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case string:
			if ref, err := asset.ParseRef(typed); err == nil {
				refs = append(refs, ref)
			}
		case []any:
			for _, item := range typed {
				visit(item)
			}
		case map[string]any:
			for _, item := range typed {
				visit(item)
			}
		}
	}
	visit(value)
	return refs
}
