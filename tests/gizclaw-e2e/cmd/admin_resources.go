//go:build gizclaw_e2e

package clitest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

// UpsertRuntimeProfileByName keeps E2E setup name-oriented while exercising
// the Admin API's canonical-ID update contract.
func UpsertRuntimeProfileByName(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.RuntimeProfileUpsert) (apitypes.RuntimeProfile, error) {
	if err := canonicalizeRuntimeProfileReferences(ctx, api, &body.Spec); err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	item, found, err := RuntimeProfileByName(ctx, api, body.Name)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if found {
		response, err := api.PutRuntimeProfileWithResponse(ctx, item.Id, body)
		if err != nil {
			return apitypes.RuntimeProfile{}, err
		}
		if response.JSON200 == nil {
			return apitypes.RuntimeProfile{}, fmt.Errorf("put RuntimeProfile %q status %d: %s", body.Name, response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		return *response.JSON200, nil
	}
	response, err := api.CreateRuntimeProfileWithResponse(ctx, body)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if response.JSON200 == nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("create RuntimeProfile %q status %d: %s", body.Name, response.StatusCode(), strings.TrimSpace(string(response.Body)))
	}
	return *response.JSON200, nil
}

type SocialFixtureIDs struct {
	FriendID           string
	FriendGroupID      string
	ClientMembershipID string
}

// EnsureSocialFixture creates or updates the shared Social E2E graph while
// resolving every Admin mutation through canonical IDs.
func EnsureSocialFixture(ctx context.Context, api *adminhttp.ClientWithResponses, ownerPublicKey, clientPublicKey string) (SocialFixtureIDs, error) {
	ids := SocialFixtureIDs{}
	limit := 200
	friends, err := api.ListFriendsWithResponse(ctx, &adminhttp.ListFriendsParams{Limit: &limit})
	if err != nil || friends.JSON200 == nil {
		return ids, listResponseError("Friends", friends, err)
	}
	for _, item := range friends.JSON200.Items {
		if item.OwnerPublicKey == ownerPublicKey && item.PeerPublicKey == clientPublicKey {
			ids.FriendID = item.Id
			break
		}
	}
	if ids.FriendID == "" {
		created, err := api.CreateFriendWithResponse(ctx, adminhttp.AdminSocialFriendCreateRequest{OwnerPublicKey: ownerPublicKey, PeerPublicKey: clientPublicKey})
		if err != nil || created.JSON200 == nil {
			return ids, listResponseError("create Friend", created, err)
		}
		ids.FriendID = created.JSON200.Id
	}

	displayName := "Family Circle"
	description := "Shared family chat group."
	groups, err := api.ListFriendGroupsWithResponse(ctx, &adminhttp.ListFriendGroupsParams{Limit: &limit})
	if err != nil || groups.JSON200 == nil {
		return ids, listResponseError("FriendGroups", groups, err)
	}
	for _, item := range groups.JSON200.Items {
		if item.CreatedByPeerPublicKey == ownerPublicKey && item.Name == "family-circle" {
			ids.FriendGroupID = item.Id
			break
		}
	}
	if ids.FriendGroupID == "" {
		created, err := api.CreateFriendGroupWithResponse(ctx, adminhttp.AdminFriendGroupCreateRequest{
			Name: "family-circle", DisplayName: &displayName, Description: &description, OwnerPublicKey: ownerPublicKey,
		})
		if err != nil || created.JSON200 == nil {
			return ids, listResponseError("create FriendGroup", created, err)
		}
		ids.FriendGroupID = created.JSON200.Id
	} else {
		updated, err := api.PutFriendGroupWithResponse(ctx, ids.FriendGroupID, adminhttp.AdminFriendGroupPutRequest{DisplayName: &displayName, Description: &description})
		if err != nil || updated.JSON200 == nil {
			return ids, listResponseError("put FriendGroup", updated, err)
		}
	}

	members, err := api.ListFriendGroupMembersWithResponse(ctx, ids.FriendGroupID, &adminhttp.ListFriendGroupMembersParams{Limit: &limit})
	if err != nil || members.JSON200 == nil {
		return ids, listResponseError("FriendGroupMembers", members, err)
	}
	for _, item := range members.JSON200.Items {
		if item.PeerPublicKey == clientPublicKey {
			ids.ClientMembershipID = item.Id
			break
		}
	}
	if ids.ClientMembershipID == "" {
		created, err := api.CreateFriendGroupMemberWithResponse(ctx, ids.FriendGroupID, adminhttp.AdminFriendGroupMemberCreateRequest{
			Name: "family-circle", PeerPublicKey: clientPublicKey, Role: rpcapi.FriendGroupMemberRoleMember,
		})
		if err != nil || created.JSON200 == nil {
			return ids, listResponseError("create FriendGroupMember", created, err)
		}
		ids.ClientMembershipID = created.JSON200.Id
	} else {
		updated, err := api.PutFriendGroupMemberWithResponse(ctx, ids.FriendGroupID, clientPublicKey, adminhttp.AdminFriendGroupMemberPutRequest{Role: rpcapi.FriendGroupMemberRoleMember})
		if err != nil || updated.JSON200 == nil {
			return ids, listResponseError("put FriendGroupMember", updated, err)
		}
	}

	expiresAt := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	token, err := api.PutFriendGroupInviteTokenWithResponse(ctx, ids.FriendGroupID, adminhttp.AdminFriendGroupInviteTokenPutRequest{
		InviteToken: "family-circle-token", ExpiresAt: expiresAt,
	})
	if err != nil || token.JSON200 == nil {
		return ids, listResponseError("put FriendGroupInviteToken", token, err)
	}
	for _, contact := range []adminhttp.AdminContactCreateRequest{
		{Name: "living-room", OwnerPublicKey: ownerPublicKey, DisplayName: stringPointer("Living Room Device"), PhoneNumber: stringPointer("+15550100001")},
		{Name: "family-admin", OwnerPublicKey: clientPublicKey, DisplayName: stringPointer("Family Admin Device"), PhoneNumber: stringPointer("+15550100002")},
	} {
		if err := ensureAdminContact(ctx, api, contact); err != nil {
			return ids, err
		}
	}
	return ids, nil
}

func ensureAdminContact(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.AdminContactCreateRequest) error {
	limit := 200
	response, err := api.ListContactsWithResponse(ctx, &adminhttp.ListContactsParams{OwnerPublicKey: &body.OwnerPublicKey, Limit: &limit})
	if err != nil || response.JSON200 == nil {
		return listResponseError("Contacts", response, err)
	}
	for _, item := range response.JSON200.Items {
		if item.Name != body.Name {
			continue
		}
		updated, err := api.PutContactWithResponse(ctx, body.OwnerPublicKey, item.Id, adminhttp.AdminContactPutRequest{DisplayName: body.DisplayName, PhoneNumber: body.PhoneNumber})
		if err != nil || updated.JSON200 == nil {
			return listResponseError("put Contact", updated, err)
		}
		return nil
	}
	created, err := api.CreateContactWithResponse(ctx, body)
	if err != nil || created.JSON200 == nil {
		return listResponseError("create Contact", created, err)
	}
	return nil
}

func stringPointer(value string) *string { return &value }

func canonicalizeRuntimeProfileReferences(ctx context.Context, api *adminhttp.ClientWithResponses, spec *apitypes.RuntimeProfileSpec) error {
	if spec == nil {
		return nil
	}
	for kind, bindings := range map[string]*map[string]apitypes.RuntimeProfileBinding{
		"BadgeDef": spec.Resources.BadgeDefs,
		"GameDef":  spec.Resources.GameDefs,
		"Model":    spec.Resources.Models,
		"PetDef":   spec.Resources.PetDefs,
		"Voice":    spec.Resources.Voices,
	} {
		if bindings == nil {
			continue
		}
		for alias, binding := range *bindings {
			id, err := runtimeResourceID(ctx, api, kind, binding.ResourceId)
			if err != nil {
				return fmt.Errorf("resolve RuntimeProfile %s alias %q: %w", kind, alias, err)
			}
			binding.ResourceId = id
			(*bindings)[alias] = binding
		}
	}
	if spec.Resources.Memories != nil {
		for alias, binding := range *spec.Resources.Memories {
			id, err := runtimeResourceID(ctx, api, "MemoryLayout", binding.LayoutId)
			if err != nil {
				return fmt.Errorf("resolve RuntimeProfile Memory alias %q: %w", alias, err)
			}
			binding.LayoutId = id
			(*spec.Resources.Memories)[alias] = binding
		}
	}
	for collection, bindings := range spec.Workflows.Collections {
		for alias, binding := range bindings {
			id, err := runtimeResourceID(ctx, api, "Workflow", binding.ResourceId)
			if err != nil {
				return fmt.Errorf("resolve RuntimeProfile Workflow %s/%s: %w", collection, alias, err)
			}
			binding.ResourceId = id
			bindings[alias] = binding
		}
	}
	for label, reference := range map[string]*string{
		"friend_chatroom": &spec.Workflows.System.FriendChatroom,
		"group_chatroom":  &spec.Workflows.System.GroupChatroom,
		"pet":             &spec.Workflows.System.Pet,
	} {
		id, err := runtimeResourceID(ctx, api, "Workflow", *reference)
		if err != nil {
			return fmt.Errorf("resolve RuntimeProfile system Workflow %s: %w", label, err)
		}
		*reference = id
	}
	return nil
}

func runtimeResourceID(ctx context.Context, api *adminhttp.ClientWithResponses, kind, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return "", fmt.Errorf("empty reference")
	}
	limit := int32(200)
	switch kind {
	case "Workflow":
		return findAdminResourceID(ctx, reference, func(cursor *string) ([]apitypes.Workflow, bool, *string, error) {
			response, err := api.ListWorkflowsWithResponse(ctx, &adminhttp.ListWorkflowsParams{Cursor: cursor, Limit: &limit})
			if err != nil || response.JSON200 == nil {
				return nil, false, nil, listResponseError("Workflows", response, err)
			}
			return response.JSON200.Items, response.JSON200.HasNext, response.JSON200.NextCursor, nil
		}, func(item apitypes.Workflow) (string, string) { return item.Id, item.Name })
	case "Model":
		return findAdminResourceID(ctx, reference, func(cursor *string) ([]apitypes.Model, bool, *string, error) {
			response, err := api.ListModelsWithResponse(ctx, &adminhttp.ListModelsParams{Cursor: cursor, Limit: &limit})
			if err != nil || response.JSON200 == nil {
				return nil, false, nil, listResponseError("Models", response, err)
			}
			return response.JSON200.Items, response.JSON200.HasNext, response.JSON200.NextCursor, nil
		}, func(item apitypes.Model) (string, string) { return item.Id, item.Name })
	case "Voice":
		return findAdminResourceID(ctx, reference, func(cursor *string) ([]apitypes.Voice, bool, *string, error) {
			response, err := api.ListVoicesWithResponse(ctx, &adminhttp.ListVoicesParams{Cursor: cursor, Limit: &limit})
			if err != nil || response.JSON200 == nil {
				return nil, false, nil, listResponseError("Voices", response, err)
			}
			return response.JSON200.Items, response.JSON200.HasNext, response.JSON200.NextCursor, nil
		}, func(item apitypes.Voice) (string, string) { return item.Id, item.Name })
	case "MemoryLayout":
		return findAdminResourceID(ctx, reference, func(cursor *string) ([]apitypes.MemoryLayout, bool, *string, error) {
			response, err := api.ListMemoryLayoutsWithResponse(ctx, &adminhttp.ListMemoryLayoutsParams{Cursor: cursor, Limit: &limit})
			if err != nil || response.JSON200 == nil {
				return nil, false, nil, listResponseError("MemoryLayouts", response, err)
			}
			return response.JSON200.Items, response.JSON200.HasNext, response.JSON200.NextCursor, nil
		}, func(item apitypes.MemoryLayout) (string, string) { return item.Id, item.Name })
	case "PetDef":
		return findAdminResourceID(ctx, reference, func(cursor *string) ([]apitypes.PetDef, bool, *string, error) {
			response, err := api.ListPetDefsWithResponse(ctx, &adminhttp.ListPetDefsParams{Cursor: cursor, Limit: &limit})
			if err != nil || response.JSON200 == nil {
				return nil, false, nil, listResponseError("PetDefs", response, err)
			}
			return response.JSON200.Items, response.JSON200.HasNext, response.JSON200.NextCursor, nil
		}, func(item apitypes.PetDef) (string, string) { return item.Id, item.Name })
	case "BadgeDef":
		return findAdminResourceID(ctx, reference, func(cursor *string) ([]apitypes.BadgeDef, bool, *string, error) {
			response, err := api.ListBadgeDefsWithResponse(ctx, &adminhttp.ListBadgeDefsParams{Cursor: cursor, Limit: &limit})
			if err != nil || response.JSON200 == nil {
				return nil, false, nil, listResponseError("BadgeDefs", response, err)
			}
			return response.JSON200.Items, response.JSON200.HasNext, response.JSON200.NextCursor, nil
		}, func(item apitypes.BadgeDef) (string, string) { return item.Id, item.Name })
	case "GameDef":
		return findAdminResourceID(ctx, reference, func(cursor *string) ([]apitypes.GameDef, bool, *string, error) {
			response, err := api.ListGameDefsWithResponse(ctx, &adminhttp.ListGameDefsParams{Cursor: cursor, Limit: &limit})
			if err != nil || response.JSON200 == nil {
				return nil, false, nil, listResponseError("GameDefs", response, err)
			}
			return response.JSON200.Items, response.JSON200.HasNext, response.JSON200.NextCursor, nil
		}, func(item apitypes.GameDef) (string, string) { return item.Id, item.Name })
	default:
		return "", fmt.Errorf("unsupported resource kind %q", kind)
	}
}

// ResourceIDByName resolves a typed Admin resource name to its canonical ID.
func ResourceIDByName(ctx context.Context, api *adminhttp.ClientWithResponses, kind, name string) (string, error) {
	return runtimeResourceID(ctx, api, kind, name)
}

func findAdminResourceID[T any](ctx context.Context, reference string, list func(*string) ([]T, bool, *string, error), identity func(T) (string, string)) (string, error) {
	_ = ctx
	var cursor *string
	for {
		items, hasNext, nextCursor, err := list(cursor)
		if err != nil {
			return "", err
		}
		for _, item := range items {
			id, name := identity(item)
			if id == reference || name == reference {
				return id, nil
			}
		}
		if !hasNext || nextCursor == nil {
			return "", fmt.Errorf("resource %q not found", reference)
		}
		cursor = nextCursor
	}
}

func listResponseError(kind string, response interface{ StatusCode() int }, err error) error {
	if err != nil {
		return err
	}
	if response == nil {
		return fmt.Errorf("list %s returned no response", kind)
	}
	return fmt.Errorf("list %s status %d", kind, response.StatusCode())
}

func RuntimeProfileByName(ctx context.Context, api *adminhttp.ClientWithResponses, name string) (apitypes.RuntimeProfile, bool, error) {
	limit := int32(200)
	var cursor *string
	for {
		response, err := api.ListRuntimeProfilesWithResponse(ctx, &adminhttp.ListRuntimeProfilesParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			return apitypes.RuntimeProfile{}, false, err
		}
		if response.JSON200 == nil {
			return apitypes.RuntimeProfile{}, false, fmt.Errorf("list RuntimeProfiles status %d: %s", response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		for _, item := range response.JSON200.Items {
			if item.Name == name {
				return item, true, nil
			}
		}
		if !response.JSON200.HasNext || response.JSON200.NextCursor == nil {
			return apitypes.RuntimeProfile{}, false, nil
		}
		cursor = response.JSON200.NextCursor
	}
}

func FirmwareByName(ctx context.Context, api *adminhttp.ClientWithResponses, name string) (apitypes.Firmware, bool, error) {
	limit := int32(200)
	var cursor *string
	for {
		response, err := api.ListFirmwaresWithResponse(ctx, &adminhttp.ListFirmwaresParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			return apitypes.Firmware{}, false, err
		}
		if response.JSON200 == nil {
			return apitypes.Firmware{}, false, fmt.Errorf("list Firmwares status %d: %s", response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		for _, item := range response.JSON200.Items {
			if item.Name == name {
				return item, true, nil
			}
		}
		if !response.JSON200.HasNext || response.JSON200.NextCursor == nil {
			return apitypes.Firmware{}, false, nil
		}
		cursor = response.JSON200.NextCursor
	}
}

func WorkspaceByName(ctx context.Context, api *adminhttp.ClientWithResponses, name string) (apitypes.Workspace, bool, error) {
	limit := int32(200)
	var cursor *string
	for {
		response, err := api.ListWorkspacesWithResponse(ctx, &adminhttp.ListWorkspacesParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			return apitypes.Workspace{}, false, err
		}
		if response.JSON200 == nil {
			return apitypes.Workspace{}, false, fmt.Errorf("list Workspaces status %d: %s", response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		for _, item := range response.JSON200.Items {
			if item.Name == name {
				return item, true, nil
			}
		}
		if !response.JSON200.HasNext || response.JSON200.NextCursor == nil {
			return apitypes.Workspace{}, false, nil
		}
		cursor = response.JSON200.NextCursor
	}
}

func VolcTenantByName(ctx context.Context, api *adminhttp.ClientWithResponses, name string) (apitypes.VolcTenant, bool, error) {
	limit := int32(200)
	var cursor *string
	for {
		response, err := api.ListVolcTenantsWithResponse(ctx, &adminhttp.ListVolcTenantsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			return apitypes.VolcTenant{}, false, err
		}
		if response.JSON200 == nil {
			return apitypes.VolcTenant{}, false, fmt.Errorf("list VolcTenants status %d: %s", response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		for _, item := range response.JSON200.Items {
			if item.Name == name {
				return item, true, nil
			}
		}
		if !response.JSON200.HasNext || response.JSON200.NextCursor == nil {
			return apitypes.VolcTenant{}, false, nil
		}
		cursor = response.JSON200.NextCursor
	}
}

func MiniMaxTenantByName(ctx context.Context, api *adminhttp.ClientWithResponses, name string) (apitypes.MiniMaxTenant, bool, error) {
	limit := int32(200)
	var cursor *string
	for {
		response, err := api.ListMiniMaxTenantsWithResponse(ctx, &adminhttp.ListMiniMaxTenantsParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			return apitypes.MiniMaxTenant{}, false, err
		}
		if response.JSON200 == nil {
			return apitypes.MiniMaxTenant{}, false, fmt.Errorf("list MiniMaxTenants status %d: %s", response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		for _, item := range response.JSON200.Items {
			if item.Name == name {
				return item, true, nil
			}
		}
		if !response.JSON200.HasNext || response.JSON200.NextCursor == nil {
			return apitypes.MiniMaxTenant{}, false, nil
		}
		cursor = response.JSON200.NextCursor
	}
}

func DeleteRegistrationTokenByName(ctx context.Context, api *adminhttp.ClientWithResponses, name string) error {
	limit := int32(200)
	var cursor *string
	for {
		response, err := api.ListRegistrationTokensWithResponse(ctx, &adminhttp.ListRegistrationTokensParams{Cursor: cursor, Limit: &limit})
		if err != nil {
			return err
		}
		if response.JSON200 == nil {
			return fmt.Errorf("list RegistrationTokens status %d: %s", response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		for _, item := range response.JSON200.Items {
			if item.Name == name {
				deleted, err := api.DeleteRegistrationTokenWithResponse(ctx, item.Id)
				if err != nil {
					return err
				}
				if deleted.StatusCode() != 204 {
					return fmt.Errorf("delete RegistrationToken %q status %d: %s", name, deleted.StatusCode(), strings.TrimSpace(string(deleted.Body)))
				}
				return nil
			}
		}
		if !response.JSON200.HasNext || response.JSON200.NextCursor == nil {
			return nil
		}
		cursor = response.JSON200.NextCursor
	}
}
