//go:build gizclaw_e2e

package clitest

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
)

// UpsertRuntimeProfile applies an exact caller-supplied RuntimeProfile ID and
// assumes every nested Admin reference already contains its canonical ID.
func UpsertRuntimeProfile(ctx context.Context, api *adminhttp.ClientWithResponses, body adminhttp.RuntimeProfileUpsert) (apitypes.RuntimeProfile, error) {
	item, found, err := RuntimeProfileByID(ctx, api, body.Id)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if found {
		response, err := api.PutRuntimeProfileWithResponse(ctx, item.Id, body)
		if err != nil {
			return apitypes.RuntimeProfile{}, err
		}
		if response.JSON200 == nil {
			return apitypes.RuntimeProfile{}, fmt.Errorf("put RuntimeProfile %q status %d: %s", body.Id, response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		return *response.JSON200, nil
	}
	response, err := api.CreateRuntimeProfileWithResponse(ctx, body)
	if err != nil {
		return apitypes.RuntimeProfile{}, err
	}
	if response.JSON200 == nil {
		return apitypes.RuntimeProfile{}, fmt.Errorf("create RuntimeProfile %q status %d: %s", body.Id, response.StatusCode(), strings.TrimSpace(string(response.Body)))
	}
	return *response.JSON200, nil
}

type SocialFixtureIDs struct {
	FriendID           string
	FriendGroupID      string
	FriendGroupName    string
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
		created, err := api.CreateFriendWithResponse(ctx, adminhttp.AdminSocialFriendCreateRequest{Id: socialRelationID(ownerPublicKey, clientPublicKey), OwnerPublicKey: ownerPublicKey, PeerPublicKey: clientPublicKey})
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
			ids.FriendGroupName = item.Name
			break
		}
	}
	if ids.FriendGroupID == "" {
		created, err := api.CreateFriendGroupWithResponse(ctx, adminhttp.AdminFriendGroupCreateRequest{
			Id: "family-circle", Name: "family-circle", DisplayName: &displayName, Description: &description, OwnerPublicKey: ownerPublicKey,
		})
		if err != nil || created.JSON200 == nil {
			return ids, listResponseError("create FriendGroup", created, err)
		}
		ids.FriendGroupID = created.JSON200.Id
		ids.FriendGroupName = created.JSON200.Name
	} else {
		updated, err := api.PutFriendGroupWithResponse(ctx, ids.FriendGroupID, adminhttp.AdminFriendGroupPutRequest{Id: ids.FriendGroupID, DisplayName: &displayName, Description: &description})
		if err != nil || updated.JSON200 == nil {
			return ids, listResponseError("put FriendGroup", updated, err)
		}
	}
	if ids.FriendGroupName == "" {
		ids.FriendGroupName = "family-circle"
	}

	members, err := api.ListFriendGroupMembersWithResponse(ctx, ids.FriendGroupID, &adminhttp.ListFriendGroupMembersParams{Limit: &limit})
	if err != nil || members.JSON200 == nil {
		return ids, listResponseError("FriendGroupMembers", members, err)
	}
	for _, item := range members.JSON200.Items {
		if item.PeerPublicKey == clientPublicKey {
			ids.ClientMembershipID = customid.MembershipName(ids.FriendGroupID, item.PeerPublicKey)
			break
		}
	}
	if ids.ClientMembershipID == "" {
		created, err := api.CreateFriendGroupMemberWithResponse(ctx, ids.FriendGroupID, adminhttp.AdminFriendGroupMemberCreateRequest{
			Id: customid.MembershipName(ids.FriendGroupID, clientPublicKey), Name: "family-circle", PeerPublicKey: clientPublicKey, Role: rpcapi.FriendGroupMemberRoleMember,
		})
		if err != nil || created.JSON200 == nil {
			return ids, listResponseError("create FriendGroupMember", created, err)
		}
		ids.ClientMembershipID = customid.MembershipName(ids.FriendGroupID, created.JSON200.PeerPublicKey)
	} else {
		updated, err := api.PutFriendGroupMemberWithResponse(ctx, ids.FriendGroupID, clientPublicKey, adminhttp.AdminFriendGroupMemberPutRequest{Id: ids.ClientMembershipID, Role: rpcapi.FriendGroupMemberRoleMember})
		if err != nil || updated.JSON200 == nil {
			return ids, listResponseError("put FriendGroupMember", updated, err)
		}
	}

	expiresAt := time.Date(2099, time.January, 1, 0, 0, 0, 0, time.UTC)
	token, err := api.PutFriendGroupInviteTokenWithResponse(ctx, ids.FriendGroupID, adminhttp.AdminFriendGroupInviteTokenPutRequest{
		Id:          ids.FriendGroupID,
		InviteToken: "family-circle-token", ExpiresAt: expiresAt,
	})
	if err != nil || token.JSON200 == nil {
		return ids, listResponseError("put FriendGroupInviteToken", token, err)
	}
	for _, contact := range []adminhttp.AdminContactCreateRequest{
		{Id: "living-room", Name: "living-room", OwnerPublicKey: ownerPublicKey, DisplayName: stringPointer("Living Room Device"), PhoneNumber: stringPointer("+15550100001")},
		{Id: "family-admin", Name: "family-admin", OwnerPublicKey: clientPublicKey, DisplayName: stringPointer("Family Admin Device"), PhoneNumber: stringPointer("+15550100002")},
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
		updated, err := api.PutContactWithResponse(ctx, body.OwnerPublicKey, item.Id, adminhttp.AdminContactPutRequest{Id: item.Id, DisplayName: body.DisplayName, PhoneNumber: body.PhoneNumber})
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

func socialRelationID(a, b string) string {
	keys := []string{strings.TrimSpace(a), strings.TrimSpace(b)}
	sort.Strings(keys)
	return keys[0] + ":" + keys[1]
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

func RuntimeProfileByID(ctx context.Context, api *adminhttp.ClientWithResponses, id string) (apitypes.RuntimeProfile, bool, error) {
	response, err := api.GetRuntimeProfileWithResponse(ctx, id)
	if err != nil {
		return apitypes.RuntimeProfile{}, false, err
	}
	if response.JSON200 != nil {
		return *response.JSON200, true, nil
	}
	if response.JSON404 != nil {
		return apitypes.RuntimeProfile{}, false, nil
	}
	return apitypes.RuntimeProfile{}, false, fmt.Errorf("get RuntimeProfile %q status %d: %s", id, response.StatusCode(), strings.TrimSpace(string(response.Body)))
}

func FirmwareByID(ctx context.Context, api *adminhttp.ClientWithResponses, id string) (apitypes.Firmware, bool, error) {
	response, err := api.GetFirmwareWithResponse(ctx, id)
	if err != nil {
		return apitypes.Firmware{}, false, err
	}
	if response.JSON200 != nil {
		return *response.JSON200, true, nil
	}
	if response.JSON404 != nil {
		return apitypes.Firmware{}, false, nil
	}
	return apitypes.Firmware{}, false, fmt.Errorf("get Firmware %q status %d: %s", id, response.StatusCode(), strings.TrimSpace(string(response.Body)))
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

func VolcTenantByID(ctx context.Context, api *adminhttp.ClientWithResponses, id string) (apitypes.VolcTenant, bool, error) {
	response, err := api.GetVolcTenantWithResponse(ctx, id)
	if err != nil {
		return apitypes.VolcTenant{}, false, err
	}
	if response.JSON200 != nil {
		return *response.JSON200, true, nil
	}
	if response.JSON404 != nil {
		return apitypes.VolcTenant{}, false, nil
	}
	return apitypes.VolcTenant{}, false, fmt.Errorf("get VolcTenant %q status %d: %s", id, response.StatusCode(), strings.TrimSpace(string(response.Body)))
}

func DeleteRegistrationTokenByID(ctx context.Context, api *adminhttp.ClientWithResponses, id string) error {
	deleted, err := api.DeleteRegistrationTokenWithResponse(ctx, id)
	if err != nil {
		return err
	}
	if deleted.StatusCode() == 404 {
		return nil
	}
	if deleted.StatusCode() != 200 {
		return fmt.Errorf("delete RegistrationToken %q status %d: %s", id, deleted.StatusCode(), strings.TrimSpace(string(deleted.Body)))
	}
	return nil
}
