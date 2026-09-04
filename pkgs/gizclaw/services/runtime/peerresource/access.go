package peerresource

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type profileResourceKind string

type ownedWorkspaceLister interface {
	ListWorkspacesByOwner(context.Context, string) ([]apitypes.Workspace, error)
}

type ownedWorkspaceLabelLister interface {
	ListWorkspacesByOwnerAndLabels(context.Context, string, map[string]string) ([]apitypes.Workspace, error)
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

func (s *Server) currentRuntimeProfile() *apitypes.RuntimeProfile {
	if s == nil || s.RuntimeProfile == nil {
		return nil
	}
	return s.RuntimeProfile()
}

func bindingMap(values *map[string]apitypes.RuntimeProfileBinding) map[string]apitypes.RuntimeProfileBinding {
	if values == nil {
		return map[string]apitypes.RuntimeProfileBinding{}
	}
	return *values
}

func bindingI18n(binding apitypes.RuntimeProfileBinding) map[string]rpcapi.ResourceI18nText {
	out := make(map[string]rpcapi.ResourceI18nText, len(binding.I18n))
	for locale, text := range binding.I18n {
		out[locale] = rpcapi.ResourceI18nText{DisplayName: text.DisplayName, Description: text.Description}
	}
	return out
}

func workflowBinding(profile *apitypes.RuntimeProfile, alias string) (string, apitypes.RuntimeProfileBinding, bool) {
	if profile == nil {
		return "", apitypes.RuntimeProfileBinding{}, false
	}
	alias = strings.TrimSpace(alias)
	for collection, bindings := range profile.Spec.Workflows.Collections {
		if binding, ok := bindings[alias]; ok {
			return collection, binding, true
		}
	}
	return "", apitypes.RuntimeProfileBinding{}, false
}

func pageAliases(aliases []string, cursor *string, requested *int, revision string) ([]string, bool, *string, bool) {
	limit := peerListLimit(requested)
	start := 0
	if cursor != nil && strings.TrimSpace(*cursor) != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(*cursor)
		if err != nil {
			return nil, false, nil, true
		}
		parts := strings.SplitN(string(decoded), "\x00", 2)
		if len(parts) != 2 || parts[0] != revision {
			return nil, false, nil, true
		}
		start = sort.SearchStrings(aliases, parts[1])
		if start < len(aliases) && aliases[start] == parts[1] {
			start++
		}
	}
	end := min(start+limit, len(aliases))
	page := aliases[start:end]
	if end == len(aliases) || len(page) == 0 {
		return page, false, nil, false
	}
	next := base64.RawURLEncoding.EncodeToString([]byte(revision + "\x00" + page[len(page)-1]))
	return page, true, &next, false
}

func (s *Server) profileNames(kind profileResourceKind) []string {
	bindings := s.profileBindings(kind)
	if len(bindings) == 0 {
		return nil
	}
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Server) profileBindings(kind profileResourceKind) map[string]string {
	if s == nil || s.RuntimeProfile == nil {
		return map[string]string{}
	}
	return profileBindingsFrom(s.RuntimeProfile(), kind)
}

func profileBindingsFrom(profile *apitypes.RuntimeProfile, kind profileResourceKind) map[string]string {
	if profile == nil {
		return map[string]string{}
	}
	resources := profile.Spec.Resources
	var values *map[string]apitypes.RuntimeProfileBinding
	switch kind {
	case profileWorkflows:
		out := make(map[string]string)
		for _, bindings := range profile.Spec.Workflows.Collections {
			for alias, binding := range bindings {
				alias = strings.TrimSpace(alias)
				value := binding.ResourceId
				if alias != "" && customid.ValidateResourceID(value) == nil {
					out[alias] = value
				}
			}
		}
		return out
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
		return map[string]string{}
	}
	out := make(map[string]string, len(*values))
	for alias, binding := range *values {
		alias = strings.TrimSpace(alias)
		value := binding.ResourceId
		if alias == "" || customid.ValidateResourceID(value) != nil {
			continue
		}
		out[alias] = value
	}
	return out
}

func (s *Server) profileAllows(kind profileResourceKind, name string) bool {
	return slices.Contains(s.profileNames(kind), name)
}

func (s *Server) resolveProfileResourceName(kind profileResourceKind, name string) (string, bool) {
	id, ok := s.profileBindings(kind)[strings.TrimSpace(name)]
	return id, ok && id != ""
}

func (s *Server) profileResourceName(kind profileResourceKind, id string) (string, bool) {
	for _, name := range s.profileNames(kind) {
		if s.profileBindings(kind)[name] == id {
			return name, true
		}
	}
	return "", false
}

// ResolveModelAlias resolves an allowed RuntimeProfile alias without exposing
// the canonical resource ID to the peer.
func (s *Server) ResolveModelAlias(alias string) (string, bool) {
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return "", false
	}
	binding, ok := bindingMap(profile.Spec.Resources.Models)[strings.TrimSpace(alias)]
	return binding.ResourceId, ok && customid.ValidateResourceID(binding.ResourceId) == nil
}

// ResolveVoiceAlias resolves an allowed RuntimeProfile alias without exposing
// the canonical resource ID to the peer.
func (s *Server) ResolveVoiceAlias(alias string) (string, bool) {
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return "", false
	}
	binding, ok := bindingMap(profile.Spec.Resources.Voices)[strings.TrimSpace(alias)]
	return binding.ResourceId, ok && customid.ValidateResourceID(binding.ResourceId) == nil
}

func (s *Server) effectiveModels(ctx context.Context) ([]apitypes.Model, error) {
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return nil, nil
	}
	bindings := bindingMap(profile.Spec.Resources.Models)
	aliases := sortedBindingAliases(bindings)
	items := make([]apitypes.Model, 0, len(aliases))
	for _, alias := range aliases {
		resourceID := bindings[alias].ResourceId
		if resourceID == "" {
			continue
		}
		item, response := s.getModelValue(ctx, resourceID)
		if isNotFoundResponse(response) {
			continue
		}
		if response != nil {
			return nil, fmt.Errorf("get profile Model alias %q: %s", alias, response.Error.Message)
		}
		item.Id = alias
		items = append(items, item)
	}
	return items, nil
}

func (s *Server) ownedWorkspaces(ctx context.Context) ([]apitypes.Workspace, error) {
	if lister, ok := s.Workspaces.(ownedWorkspaceLister); ok {
		items, err := lister.ListWorkspacesByOwner(ctx, s.Caller.String())
		if err != nil {
			return nil, err
		}
		return excludeSocialWorkspaces(items), nil
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
			if s.owns(item.OwnerPublicKey) && !isSocialWorkspace(item) {
				items = append(items, item)
			}
		}
		if !page.HasNext || page.NextCursor == nil || *page.NextCursor == "" {
			return items, nil
		}
		cursor = page.NextCursor
	}
}

func (s *Server) ownedWorkspacesByLabels(ctx context.Context, selector map[string]string) ([]apitypes.Workspace, error) {
	if lister, ok := s.Workspaces.(ownedWorkspaceLabelLister); ok {
		return lister.ListWorkspacesByOwnerAndLabels(ctx, s.Caller.String(), selector)
	}
	items, err := s.ownedWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]apitypes.Workspace, 0, len(items))
	for _, item := range items {
		if workspaceLabelsMatch(item.Labels, selector) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func workspaceLabelsMatch(labels *map[string]string, selector map[string]string) bool {
	if len(selector) == 0 {
		return true
	}
	if labels == nil {
		return false
	}
	for key, value := range selector {
		if (*labels)[key] != value {
			return false
		}
	}
	return true
}

func (s *Server) effectiveWorkspacesByLabels(ctx context.Context, selector map[string]string) ([]apitypes.Workspace, error) {
	ownedItems, err := s.ownedWorkspacesByLabels(ctx, selector)
	if err != nil {
		return nil, err
	}
	allItems, err := s.effectiveWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]apitypes.Workspace, len(ownedItems))
	for _, item := range ownedItems {
		byName[item.Name] = item
	}
	for _, item := range allItems {
		if _, owned := byName[item.Name]; owned {
			continue
		}
		if workspaceLabelsMatch(item.Labels, selector) {
			byName[item.Name] = item
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]apitypes.Workspace, 0, len(names))
	for _, name := range names {
		items = append(items, byName[name])
	}
	return items, nil
}

// effectiveWorkspaces merges the caller's own Workspaces with the shared
// Workspaces reachable through Friend, FriendGroup, and Pet relationships. A
// Workspace whose deletion is already pending is treated as absent, like a
// Workspace that no longer resolves, so an unfinished asynchronous deletion
// leaves the remaining Workspaces listable.
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
		if errors.Is(err, workspace.ErrWorkspacePendingDeletion) {
			continue
		}
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
	if s.Gameplay != nil && s.Gameplay.DB != nil && s.RuntimeProfile != nil {
		profile := s.RuntimeProfile()
		if profile == nil {
			return orderedUnique(names, nil), nil
		}
		profileCtx := gameplay.WithRuntimeProfile(ctx, *profile)
		petWorkspaceNames, err := s.Gameplay.ListPetWorkspaceNames(profileCtx, owner)
		if err != nil {
			return nil, err
		}
		for _, name := range petWorkspaceNames {
			if name = strings.TrimSpace(name); name != "" {
				names = append(names, name)
			}
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
	return statusError(requestID, rpcapi.StatusCodePermissionDenied, "resource is not owned by the authenticated peer")
}

// canAccessWorkspace decides Workspace access for the caller. Ordinary
// Workspaces are owner-only. Social SFU Workspaces are never owner-granted:
// the authoritative Social KV binding decides, and revoked or foreign
// memberships fail closed.
func (s *Server) canAccessWorkspace(ctx context.Context, item apitypes.Workspace) (bool, error) {
	if s.owns(item.OwnerPublicKey) && !isSocialWorkspace(item) {
		return true, nil
	}
	workspaceName := strings.TrimSpace(item.Name)
	workspaceID := item.Id
	if customid.ValidateResourceID(workspaceID) != nil {
		return false, nil
	}
	owner := s.Caller.String()
	binding, err := s.resolveSocialBinding(ctx, func(resolver sfu.BindingResolver) (socialutil.SFUWorkspaceBinding, error) {
		return resolver.ResolveSFUWorkspaceBinding(ctx, workspaceID, owner)
	})
	switch {
	case err == nil:
		return binding.WorkspaceName == workspaceName, nil
	case errors.Is(err, sfu.ErrNotMember), errors.Is(err, sfu.ErrRevoked):
		return false, nil
	case !errors.Is(err, kv.ErrNotFound):
		return false, err
	}
	if isSocialWorkspace(item) {
		return false, nil
	}
	if s.Gameplay != nil && s.RuntimeProfile != nil {
		profile := s.RuntimeProfile()
		if profile == nil {
			return false, nil
		}
		profileCtx := gameplay.WithRuntimeProfile(ctx, *profile)
		allowed, err := s.Gameplay.OwnerHasPetWorkspace(profileCtx, owner, workspaceName)
		if err != nil {
			return false, err
		}
		if allowed {
			return true, nil
		}
	}
	return false, nil
}

func excludeSocialWorkspaces(items []apitypes.Workspace) []apitypes.Workspace {
	filtered := make([]apitypes.Workspace, 0, len(items))
	for _, item := range items {
		if !isSocialWorkspace(item) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// isSocialWorkspace reports whether item is a Friend or Friend Group SFU
// Workspace: a system Workspace bound to the built-in SFU Workflow.
func isSocialWorkspace(item apitypes.Workspace) bool {
	return item.System != nil && *item.System && item.WorkflowId == socialutil.SFUWorkflowID
}

// resolveSocialBinding asks the Friend and Friend Group services in turn for
// the caller's SFU binding. kv.ErrNotFound means neither service binds the
// Workspace; membership and revocation errors surface unchanged.
func (s *Server) resolveSocialBinding(
	ctx context.Context,
	resolve func(sfu.BindingResolver) (socialutil.SFUWorkspaceBinding, error),
) (socialutil.SFUWorkspaceBinding, error) {
	resolvers := make([]sfu.BindingResolver, 0, 2)
	if s.Friends != nil {
		resolvers = append(resolvers, s.Friends)
	}
	if s.FriendGroups != nil {
		resolvers = append(resolvers, s.FriendGroups)
	}
	for _, resolver := range resolvers {
		binding, err := resolve(resolver)
		if err == nil || !errors.Is(err, kv.ErrNotFound) {
			return binding, err
		}
	}
	return socialutil.SFUWorkspaceBinding{}, kv.ErrNotFound
}

type systemWorkspaceCreator interface {
	CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error)
}

// materializeSocialWorkspace creates this Server's copy of a Social SFU
// Workspace when the shared Social KV binds name for the caller but the local
// catalog has no record yet. Every Server materializes the same identity,
// Workflow, and owner, so the copies stay interchangeable.
func (s *Server) materializeSocialWorkspace(ctx context.Context, name string) (apitypes.Workspace, error) {
	creator, ok := s.Workspaces.(systemWorkspaceCreator)
	if !ok {
		return apitypes.Workspace{}, kv.ErrNotFound
	}
	owner := s.Caller.String()
	binding, err := s.resolveSocialBinding(ctx, func(resolver sfu.BindingResolver) (socialutil.SFUWorkspaceBinding, error) {
		if byName, ok := resolver.(interface {
			ResolveSFUWorkspaceBindingByName(context.Context, string, string) (socialutil.SFUWorkspaceBinding, error)
		}); ok {
			return byName.ResolveSFUWorkspaceBindingByName(ctx, name, owner)
		}
		return socialutil.SFUWorkspaceBinding{}, kv.ErrNotFound
	})
	switch {
	case errors.Is(err, sfu.ErrNotMember), errors.Is(err, sfu.ErrRevoked):
		return apitypes.Workspace{}, kv.ErrNotFound
	case err != nil:
		return apitypes.Workspace{}, err
	}
	return s.materializeSocialWorkspaceBinding(ctx, creator, binding)
}

func (s *Server) materializeSocialWorkspaceBinding(ctx context.Context, creator systemWorkspaceCreator, binding socialutil.SFUWorkspaceBinding) (apitypes.Workspace, error) {
	if strings.TrimSpace(binding.Owner) == "" || strings.TrimSpace(binding.WorkspaceID) == "" || strings.TrimSpace(binding.WorkspaceName) == "" {
		return apitypes.Workspace{}, fmt.Errorf("social Workspace binding %q has no materializable identity", binding.WorkspaceName)
	}
	workspace, _, err := creator.CreateSystemWorkspace(ownership.WithOwner(ctx, binding.Owner), adminhttp.WorkspaceUpsert{
		Id:         binding.WorkspaceID,
		Name:       binding.WorkspaceName,
		WorkflowId: socialutil.SFUWorkflowID,
	})
	if err != nil {
		return apitypes.Workspace{}, fmt.Errorf("materialize social Workspace %q: %w", binding.WorkspaceName, err)
	}
	if workspace.Id != binding.WorkspaceID || workspace.Name != binding.WorkspaceName {
		return apitypes.Workspace{}, fmt.Errorf("materialized social Workspace %q has identity %q/%q", binding.WorkspaceName, workspace.Id, workspace.Name)
	}
	return workspace, nil
}

func (s *Server) requireWorkspaceAccess(ctx context.Context, requestID, name string) *rpcapi.RPCResponse {
	item, err := s.getWorkspaceByName(s.ownerContext(ctx), name)
	if err != nil {
		return statusError(requestID, rpcapi.StatusCodeNotFound, "workspace not found")
	}
	allowed, err := s.canAccessWorkspace(ctx, item)
	if err != nil {
		return internalError(requestID, err.Error())
	}
	if !allowed {
		return statusError(requestID, rpcapi.StatusCodePermissionDenied, "workspace is not accessible to the authenticated peer")
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

func isNotFoundResponse(response *rpcapi.RPCResponse) bool {
	return response != nil && response.Error != nil && response.Error.Code == rpcapi.StatusCodeNotFound
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

//go:fix inline
func ptr[T any](value T) *T { return new(value) }
