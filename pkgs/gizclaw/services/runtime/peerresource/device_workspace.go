package peerresource

import (
	"context"
	"database/sql"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

// ErrDeviceWorkspaceNotFound reports a Workspace ID that is absent, foreign,
// or already pending deletion for the caller.
var ErrDeviceWorkspaceNotFound = errors.New("peerresource: device workspace not found")

// DeviceWorkspaceConflictError reports a Workspace deletion the Workspace
// service refused, carrying its stable error code.
type DeviceWorkspaceConflictError struct {
	Code    string
	Message string
}

func (e *DeviceWorkspaceConflictError) Error() string {
	return "peerresource: device workspace conflict: " + e.Code
}

type deviceWorkspaceService interface {
	ListOwnedHistoryWorkspaces(context.Context, string) ([]apitypes.Workspace, error)
	DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error)
}

// DeviceWorkspaceFilter narrows DeviceWorkspaces by exact workflow name.
type DeviceWorkspaceFilter struct {
	WorkflowName string
}

// DeviceWorkspaces returns the Workspaces explicitly owned by the caller,
// excluding those pending deletion. Workflows are identified by the
// alias it resolves to in the caller's current
// RuntimeProfile, exactly like the Peer RPC Workspace projection; the Admin
// Workflow ID never leaves the Server.
func (r DeviceReads) DeviceWorkspaces(ctx context.Context, filter DeviceWorkspaceFilter) ([]peerhttp.DeviceWorkspace, error) {
	profile, err := r.deviceWorkspaceProfile(ctx)
	if err != nil {
		return nil, err
	}
	items, err := r.Workspaces.ListOwnedHistoryWorkspaces(ctx, r.Caller.String())
	if err != nil {
		return nil, deviceWorkspaceOwnerError(err)
	}
	result := make([]peerhttp.DeviceWorkspace, 0, len(items))
	for _, item := range items {
		projected := deviceWorkspaceProjection(item, &profile)
		if filter.WorkflowName != "" && (projected.WorkflowName == nil || *projected.WorkflowName != filter.WorkflowName) {
			continue
		}
		result = append(result, projected)
	}
	return result, nil
}

// DeleteDeviceWorkspace starts the asynchronous deletion of one Workspace
// owned by the caller through the shared Workspace deletion lifecycle. It
// never contacts the device.
func (r DeviceReads) DeleteDeviceWorkspace(ctx context.Context, workspaceID string) error {
	// A binding removed after owner validation must refuse the deletion the
	// same way the list refuses the read.
	if _, err := r.deviceWorkspaceProfile(ctx); err != nil {
		return err
	}
	items, err := r.Workspaces.ListOwnedHistoryWorkspaces(ctx, r.Caller.String())
	if err != nil {
		return deviceWorkspaceOwnerError(err)
	}
	owned := false
	for _, item := range items {
		if item.Id == workspaceID {
			owned = true
			break
		}
	}
	if !owned {
		return ErrDeviceWorkspaceNotFound
	}
	response, err := r.Workspaces.DeleteWorkspace(ownership.WithOwner(ctx, r.Caller.String()), adminhttp.DeleteWorkspaceRequestObject{Id: workspaceID})
	if err != nil {
		return err
	}
	switch response := response.(type) {
	case adminhttp.DeleteWorkspace200JSONResponse:
		return nil
	case adminhttp.DeleteWorkspace404JSONResponse:
		return ErrDeviceWorkspaceNotFound
	case adminhttp.DeleteWorkspace409JSONResponse:
		return &DeviceWorkspaceConflictError{Code: response.Error.Code, Message: response.Error.Message}
	default:
		return errors.New("peerresource: workspace deletion failed")
	}
}

func (r DeviceReads) deviceWorkspaceProfile(ctx context.Context) (apitypes.RuntimeProfile, error) {
	if r.Workspaces == nil || r.Profiles == nil {
		return apitypes.RuntimeProfile{}, ErrDeviceServiceNotConfigured
	}
	profile, err := r.Profiles.ResolveOwnerProfile(ctx, r.Caller.String())
	if errors.Is(err, sql.ErrNoRows) {
		return apitypes.RuntimeProfile{}, ErrDeviceRuntimeProfileNotBound
	}
	return profile, err
}

// deviceWorkspaceOwnerError classifies an owner that became unavailable after
// the request passed owner validation with the Workspace service's own codes.
func deviceWorkspaceOwnerError(err error) error {
	switch {
	case errors.Is(err, workspace.ErrPeerPendingDeletion):
		return &DeviceWorkspaceConflictError{Code: workspace.PeerPendingDeletionCode, Message: err.Error()}
	case errors.Is(err, workspace.ErrPeerDeleted):
		return &DeviceWorkspaceConflictError{Code: workspace.PeerDeletedCode, Message: err.Error()}
	default:
		return err
	}
}

func deviceWorkspaceProjection(item apitypes.Workspace, profile *apitypes.RuntimeProfile) peerhttp.DeviceWorkspace {
	out := peerhttp.DeviceWorkspace{
		Id: item.Id, Name: item.Name, System: item.System != nil && *item.System,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, LastActiveAt: item.LastActiveAt,
	}
	workflowName, available := workspaceWorkflowName(profile, item)
	out.Available = available
	if available && workflowName != "" {
		out.WorkflowName = &workflowName
	}
	return out
}

var _ deviceWorkspaceService = (*workspace.Server)(nil)

// ResolveRunWorkspace resolves a control-app Workspace switch target to the
// one Workspace name the device reloads, because
// server.run.workspace.reload-with-options takes a name only. Exactly one of
// name or workflowName is set.
//
// A name must be an available, non-system Workspace the caller owns. A workflow target
// selects among the caller's available Workspaces of that workflow the most
// recently active one, ties broken by ascending name, so
// several Workspaces of one workflow resolve deterministically. No match, or
// no RuntimeProfile bound, is ErrDeviceWorkspaceNotFound; the control app
// cannot create a Workspace, which stays the device's decision.
func (r DeviceReads) ResolveRunWorkspace(ctx context.Context, name, workflowName string) (string, error) {
	filter := DeviceWorkspaceFilter{WorkflowName: workflowName}
	if name != "" {
		filter = DeviceWorkspaceFilter{}
	}
	items, err := r.DeviceWorkspaces(ctx, filter)
	if errors.Is(err, ErrDeviceRuntimeProfileNotBound) {
		return "", ErrDeviceWorkspaceNotFound
	}
	if err != nil {
		return "", err
	}
	var best *peerhttp.DeviceWorkspace
	for i := range items {
		item := &items[i]
		// A system Workspace, such as a Friend Group SFU room, is the Server's
		// and never a switch target, even when it resolves as available.
		if item.System || !item.Available || (name != "" && item.Name != name) {
			continue
		}
		if best == nil || item.LastActiveAt.After(best.LastActiveAt) ||
			(item.LastActiveAt.Equal(best.LastActiveAt) && item.Name < best.Name) {
			best = item
		}
	}
	if best == nil {
		return "", ErrDeviceWorkspaceNotFound
	}
	return best.Name, nil
}
