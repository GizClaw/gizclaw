package peerresource

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

type profileResourceKind string

type ownedModelLister interface {
	ListModelsByOwner(context.Context, string) ([]apitypes.Model, error)
}

type ownedCredentialLister interface {
	ListCredentialsByOwner(context.Context, string) ([]apitypes.Credential, error)
}

type ownedWorkspaceLister interface {
	ListWorkspacesByOwner(context.Context, string) ([]apitypes.Workspace, error)
}

const (
	profileWorkflows profileResourceKind = "workflows"
	profileModels    profileResourceKind = "models"
	profileVoices    profileResourceKind = "voices"
	profileTools     profileResourceKind = "tools"
	profilePetDefs   profileResourceKind = "pet_defs"
	profileGameDefs  profileResourceKind = "game_defs"
	profileBadgeDefs profileResourceKind = "badge_defs"
)

func (s *Server) ownerContext(ctx context.Context) context.Context {
	if s == nil {
		return ctx
	}
	return ownership.WithOwner(ctx, s.Caller.String())
}

func (s *Server) profileNames(kind profileResourceKind) []string {
	if s == nil || s.RuntimeProfile == nil {
		return nil
	}
	profile := s.RuntimeProfile()
	if profile == nil {
		return nil
	}
	resources := profile.Spec.Resources
	var values *map[string]string
	switch kind {
	case profileWorkflows:
		values = resources.Workflows
	case profileModels:
		values = resources.Models
	case profileVoices:
		values = resources.Voices
	case profileTools:
		values = resources.Tools
	case profilePetDefs:
		values = resources.PetDefs
	case profileGameDefs:
		values = resources.GameDefs
	case profileBadgeDefs:
		values = resources.BadgeDefs
	}
	if values == nil {
		return nil
	}
	aliases := make([]string, 0, len(*values))
	for alias := range *values {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	out := make([]string, 0, len(aliases))
	seen := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		value := strings.TrimSpace((*values)[alias])
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (s *Server) profileAllows(kind profileResourceKind, name string) bool {
	return slices.Contains(s.profileNames(kind), name)
}

func (s *Server) ownedModels(ctx context.Context) ([]apitypes.Model, error) {
	if lister, ok := s.Models.(ownedModelLister); ok {
		return lister.ListModelsByOwner(ctx, s.Caller.String())
	}
	items := make([]apitypes.Model, 0)
	limit := int32(200)
	var cursor *string
	for {
		response, err := s.Models.ListModels(ctx, adminhttp.ListModelsRequestObject{
			Params: adminhttp.ListModelsParams{Cursor: cursor, Limit: &limit},
		})
		if err != nil {
			return nil, err
		}
		page, rpcResponse, err := adminResult[adminhttp.ModelList](response.VisitListModelsResponse)
		if err != nil {
			return nil, err
		}
		if rpcResponse != nil {
			return nil, fmt.Errorf("list Models: %s", rpcResponse.Error.Message)
		}
		for _, item := range page.Items {
			if s.owns(item.OwnerPublicKey) {
				items = append(items, item)
			}
		}
		if !page.HasNext || page.NextCursor == nil || *page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
}

func (s *Server) effectiveModels(ctx context.Context) ([]apitypes.Model, error) {
	ownedItems, err := s.ownedModels(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]apitypes.Model, len(ownedItems))
	owned := make([]string, 0, len(ownedItems))
	for _, item := range ownedItems {
		byID[item.Id] = item
		owned = append(owned, item.Id)
	}
	ordered := orderedUnique(s.profileNames(profileModels), owned)
	items := make([]apitypes.Model, 0, len(ordered))
	for _, id := range ordered {
		item, ok := byID[id]
		if !ok {
			value, response := s.getModelValue(ctx, id)
			if isNotFoundResponse(response) {
				continue
			}
			if response != nil {
				return nil, fmt.Errorf("get profile Model %q: %s", id, response.Error.Message)
			}
			item = value
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Server) ownedCredentials(ctx context.Context) ([]apitypes.Credential, error) {
	if lister, ok := s.Credentials.(ownedCredentialLister); ok {
		return lister.ListCredentialsByOwner(ctx, s.Caller.String())
	}
	items := make([]apitypes.Credential, 0)
	limit := int32(200)
	var cursor *string
	for {
		response, err := s.Credentials.ListCredentials(ctx, adminhttp.ListCredentialsRequestObject{
			Params: adminhttp.ListCredentialsParams{Cursor: cursor, Limit: &limit},
		})
		if err != nil {
			return nil, err
		}
		page, rpcResponse, err := adminResult[adminhttp.CredentialList](response.VisitListCredentialsResponse)
		if err != nil {
			return nil, err
		}
		if rpcResponse != nil {
			return nil, fmt.Errorf("list Credentials: %s", rpcResponse.Error.Message)
		}
		for _, item := range page.Items {
			if s.owns(item.OwnerPublicKey) {
				items = append(items, item)
			}
		}
		if !page.HasNext || page.NextCursor == nil || *page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
}

func (s *Server) ownedWorkspaces(ctx context.Context) ([]apitypes.Workspace, error) {
	if lister, ok := s.Workspaces.(ownedWorkspaceLister); ok {
		return lister.ListWorkspacesByOwner(ctx, s.Caller.String())
	}
	items := make([]apitypes.Workspace, 0)
	limit := int32(200)
	var cursor *string
	for {
		response, err := s.Workspaces.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{
			Params: adminhttp.ListWorkspacesParams{Cursor: cursor, Limit: &limit},
		})
		if err != nil {
			return nil, err
		}
		page, rpcResponse, err := adminResult[adminhttp.WorkspaceList](response.VisitListWorkspacesResponse)
		if err != nil {
			return nil, err
		}
		if rpcResponse != nil {
			return nil, fmt.Errorf("list Workspaces: %s", rpcResponse.Error.Message)
		}
		for _, item := range page.Items {
			if s.owns(item.OwnerPublicKey) {
				items = append(items, item)
			}
		}
		if !page.HasNext || page.NextCursor == nil || *page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
}

func (s *Server) effectiveWorkspaces(ctx context.Context) ([]apitypes.Workspace, error) {
	ownedItems, err := s.ownedWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]apitypes.Workspace, len(ownedItems))
	ownedNames := make([]string, 0, len(ownedItems))
	for _, item := range ownedItems {
		byName[item.Name] = item
		ownedNames = append(ownedNames, item.Name)
	}
	domainNames, err := s.domainWorkspaceNames(ctx)
	if err != nil {
		return nil, err
	}
	for _, name := range domainNames {
		if _, exists := byName[name]; exists {
			continue
		}
		item, response, err := s.getWorkspaceForList(ctx, "", name)
		if err != nil {
			return nil, err
		}
		if isNotFoundResponse(response) {
			continue
		}
		if response != nil {
			return nil, fmt.Errorf("get domain Workspace %q: %s", name, response.Error.Message)
		}
		byName[name] = item
	}
	ordered := orderedUnique(ownedNames, domainNames)
	items := make([]apitypes.Workspace, 0, len(ordered))
	for _, name := range ordered {
		if item, ok := byName[name]; ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *Server) domainWorkspaceNames(ctx context.Context) ([]string, error) {
	owner := s.Caller.String()
	names := make([]string, 0)
	limit := 200
	if s.Friends != nil {
		var cursor *string
		for {
			page, err := s.Friends.ListFriends(ctx, owner, rpcapi.FriendListRequest{Cursor: cursor, Limit: &limit})
			if err != nil {
				return nil, err
			}
			for _, item := range page.Items {
				names = append(names, strings.TrimSpace(valueOrZero(item.WorkspaceName)))
			}
			if !page.HasNext || page.NextCursor == nil {
				break
			}
			cursor = page.NextCursor
		}
	}
	if s.FriendGroups != nil {
		var cursor *string
		for {
			page, err := s.FriendGroups.ListFriendGroups(ctx, owner, rpcapi.FriendGroupListRequest{Cursor: cursor, Limit: &limit})
			if err != nil {
				return nil, err
			}
			for _, item := range page.Items {
				names = append(names, strings.TrimSpace(valueOrZero(item.WorkspaceName)))
			}
			if !page.HasNext || page.NextCursor == nil {
				break
			}
			cursor = page.NextCursor
		}
	}
	if s.Gameplay != nil {
		var cursor *string
		for {
			page, err := s.Gameplay.ListPets(ctx, owner, apitypes.GameplayListRequest{Cursor: cursor, Limit: &limit})
			if err != nil {
				return nil, err
			}
			for _, item := range page.Items {
				names = append(names, strings.TrimSpace(item.WorkspaceName))
			}
			if !page.HasNext || page.NextCursor == nil {
				break
			}
			cursor = page.NextCursor
		}
	}
	return orderedUnique(names, nil), nil
}

func (s *Server) owns(owner *string) bool {
	return s != nil && owner != nil && *owner == s.Caller.String()
}

func (s *Server) requireOwner(requestID string, owner *string) *rpcapi.RPCResponse {
	if s.owns(owner) {
		return nil
	}
	return statusError(requestID, http.StatusForbidden, "resource is not owned by the authenticated peer")
}

func (s *Server) canAccessWorkspace(ctx context.Context, item apitypes.Workspace) (bool, error) {
	if s.owns(item.OwnerPublicKey) {
		return true, nil
	}
	workspaceName := strings.TrimSpace(item.Name)
	owner := s.Caller.String()
	if s.Friends != nil {
		limit := 200
		var cursor *string
		for {
			list, err := s.Friends.ListFriends(ctx, owner, rpcapi.FriendListRequest{Cursor: cursor, Limit: &limit})
			if err != nil {
				return false, err
			}
			for _, friend := range list.Items {
				if strings.TrimSpace(valueOrZero(friend.WorkspaceName)) == workspaceName {
					return true, nil
				}
			}
			if !list.HasNext || list.NextCursor == nil {
				break
			}
			cursor = list.NextCursor
		}
	}
	if s.FriendGroups != nil {
		limit := 200
		var cursor *string
		for {
			list, err := s.FriendGroups.ListFriendGroups(ctx, owner, rpcapi.FriendGroupListRequest{Cursor: cursor, Limit: &limit})
			if err != nil {
				return false, err
			}
			for _, group := range list.Items {
				if strings.TrimSpace(valueOrZero(group.WorkspaceName)) == workspaceName {
					return true, nil
				}
			}
			if !list.HasNext || list.NextCursor == nil {
				break
			}
			cursor = list.NextCursor
		}
	}
	if s.Gameplay != nil {
		allowed, err := s.Gameplay.OwnerHasPetWorkspace(ctx, owner, workspaceName)
		if err != nil {
			return false, err
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

func (s *Server) requireWorkspaceAccess(ctx context.Context, requestID, name string) *rpcapi.RPCResponse {
	response, err := s.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: name})
	if err != nil {
		return internalError(requestID, err.Error())
	}
	item, rpcResponse, err := adminResult[apitypes.Workspace](response.VisitGetWorkspaceResponse)
	if err != nil {
		return internalError(requestID, err.Error())
	}
	if rpcResponse != nil {
		return withRequestID(requestID, rpcResponse)
	}
	allowed, err := s.canAccessWorkspace(ctx, item)
	if err != nil {
		return internalError(requestID, err.Error())
	}
	if !allowed {
		return statusError(requestID, http.StatusForbidden, "workspace is not accessible to the authenticated peer")
	}
	return nil
}

func orderedUnique(profile []string, owned []string) []string {
	seen := make(map[string]struct{}, len(profile)+len(owned))
	out := make([]string, 0, len(profile)+len(owned))
	for _, values := range [][]string{profile, owned} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func (s *Server) getModelValue(ctx context.Context, id string) (apitypes.Model, *rpcapi.RPCResponse) {
	response, err := s.Models.GetModel(ctx, adminhttp.GetModelRequestObject{Id: id})
	if err != nil {
		return apitypes.Model{}, internalError("", err.Error())
	}
	item, rpcResponse, err := adminResult[apitypes.Model](response.VisitGetModelResponse)
	if err != nil {
		return apitypes.Model{}, internalError("", err.Error())
	}
	return item, rpcResponse
}

func (s *Server) canUseModel(ctx context.Context, id string) bool {
	if s.profileAllows(profileModels, id) {
		return true
	}
	item, rpcResponse := s.getModelValue(ctx, id)
	return rpcResponse == nil && s.owns(item.OwnerPublicKey)
}

func isNotFoundResponse(response *rpcapi.RPCResponse) bool {
	return response != nil && response.Error != nil && response.Error.Code == rpcapi.RPCErrorCodeNotFound
}

func pageModels(items []apitypes.Model, cursor *string, requested *int) ([]apitypes.Model, bool, *string) {
	limit := 50
	if requested != nil && *requested > 0 {
		limit = min(*requested, 200)
	}
	start := 0
	if cursor != nil && *cursor != "" {
		for i := range items {
			if items[i].Id == *cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(items))
	page := items[start:end]
	if end == len(items) || len(page) == 0 {
		return page, false, nil
	}
	next := page[len(page)-1].Id
	return page, true, &next
}

func pageCredentials(items []apitypes.Credential, cursor *string, requested *int) ([]apitypes.Credential, bool, *string) {
	limit := 50
	if requested != nil && *requested > 0 {
		limit = min(*requested, 200)
	}
	start := 0
	if cursor != nil && *cursor != "" {
		for i := range items {
			if items[i].Name == *cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(items))
	page := items[start:end]
	if end == len(items) || len(page) == 0 {
		return page, false, nil
	}
	next := page[len(page)-1].Name
	return page, true, &next
}

func pageWorkspaces(items []apitypes.Workspace, cursor *string, requested *int) ([]apitypes.Workspace, bool, *string) {
	limit := 50
	if requested != nil && *requested > 0 {
		limit = min(*requested, 200)
	}
	start := 0
	if cursor != nil && *cursor != "" {
		for i := range items {
			if items[i].Name == *cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(items))
	page := items[start:end]
	if end == len(items) || len(page) == 0 {
		return page, false, nil
	}
	next := page[len(page)-1].Name
	return page, true, &next
}

func pageWorkflows(items []rpcapi.Workflow, cursor *string, requested *int) ([]rpcapi.Workflow, bool, *string) {
	limit := 50
	if requested != nil && *requested > 0 {
		limit = min(*requested, 200)
	}
	start := 0
	if cursor != nil && *cursor != "" {
		for i := range items {
			if items[i].Name == *cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(items))
	page := items[start:end]
	if end == len(items) || len(page) == 0 {
		return page, false, nil
	}
	next := page[len(page)-1].Name
	return page, true, &next
}

func pageVoices(items []apitypes.Voice, cursor *string, requested *int) ([]apitypes.Voice, bool, *string) {
	limit := 50
	if requested != nil && *requested > 0 {
		limit = min(*requested, 200)
	}
	start := 0
	if cursor != nil && *cursor != "" {
		for i := range items {
			if items[i].Id == *cursor {
				start = i + 1
				break
			}
		}
	}
	end := min(start+limit, len(items))
	page := items[start:end]
	if end == len(items) || len(page) == 0 {
		return page, false, nil
	}
	next := string(page[len(page)-1].Id)
	return page, true, &next
}

func ptr[T any](value T) *T { return &value }
