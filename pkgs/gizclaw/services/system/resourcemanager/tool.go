package resourcemanager

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

func (m *Manager) applyTool(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.Tools == nil {
		return apitypes.ApplyResult{}, missingService("tools")
	}
	item, err := resource.AsToolResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_TOOL_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	desired, err := toolkit.FromSpec(item.Metadata.Name, item.Spec)
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_TOOL_RESOURCE", err.Error())
	}
	existing, exists, err := m.getTool(ctx, desired.ID)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindTool, item.Metadata.Name, item.Metadata, exists); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		left, err := toolkit.ToSpec(existing)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		right, err := toolkit.ToSpec(desired)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		same, err := semanticEqual(left, right)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			ownerChanged, err := m.ensureOwnedResourceOwnerFromMetadata(ctx, apitypes.ACLResourceKindTool, item.Metadata.Name, item.Metadata)
			if err != nil {
				return apitypes.ApplyResult{}, err
			}
			if ownerChanged {
				return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindTool, item.Metadata.Name), nil
			}
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindTool, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.Tools.PutTool(ctx, desired); err != nil {
		return apitypes.ApplyResult{}, toolServiceError(err)
	}
	if exists && toolOwnerBindingChanged(existing, desired) {
		if err := m.removeToolOwnerBinding(ctx, existing); err != nil {
			return apitypes.ApplyResult{}, m.rollbackTool(ctx, existing, err)
		}
	}
	if _, err := m.ensureOwnedResourceOwnerFromMetadata(ctx, apitypes.ACLResourceKindTool, item.Metadata.Name, item.Metadata); err != nil {
		return apitypes.ApplyResult{}, m.rollbackToolWrite(ctx, existing, exists, desired.ID, err)
	}
	action := apitypes.ApplyActionCreated
	if exists {
		action = apitypes.ApplyActionUpdated
	}
	return applyResult(action, apitypes.ResourceKindTool, item.Metadata.Name), nil
}

func (m *Manager) getTool(ctx context.Context, id string) (toolkit.Tool, bool, error) {
	item, err := m.services.Tools.GetTool(ctx, id)
	if errors.Is(err, toolkit.ErrToolNotFound) {
		return toolkit.Tool{}, false, nil
	}
	if err != nil {
		return toolkit.Tool{}, false, toolServiceError(err)
	}
	return item, true, nil
}

func (m *Manager) putToolResource(ctx context.Context, item apitypes.ToolResource) (apitypes.Resource, error) {
	tool, err := toolkit.FromSpec(item.Metadata.Name, item.Spec)
	if err != nil {
		return apitypes.Resource{}, applyError(400, "INVALID_TOOL_RESOURCE", err.Error())
	}
	existing, exists, err := m.getTool(ctx, tool.ID)
	if err != nil {
		return apitypes.Resource{}, err
	}
	if err := m.validateOwnedResourceOwner(apitypes.ACLResourceKindTool, item.Metadata.Name, item.Metadata, exists); err != nil {
		return apitypes.Resource{}, err
	}
	stored, err := m.services.Tools.PutTool(ctx, tool)
	if err != nil {
		return apitypes.Resource{}, toolServiceError(err)
	}
	if exists && toolOwnerBindingChanged(existing, stored) {
		if err := m.removeToolOwnerBinding(ctx, existing); err != nil {
			return apitypes.Resource{}, m.rollbackTool(ctx, existing, err)
		}
	}
	if _, err := m.ensureOwnedResourceOwnerFromMetadata(ctx, apitypes.ACLResourceKindTool, item.Metadata.Name, item.Metadata); err != nil {
		return apitypes.Resource{}, m.rollbackToolWrite(ctx, existing, exists, stored.ID, err)
	}
	return m.Get(ctx, apitypes.ResourceKindTool, stored.ID)
}

func (m *Manager) deleteTool(ctx context.Context, id string) (toolkit.Tool, bool, error) {
	item, exists, err := m.getTool(ctx, id)
	if err != nil || !exists {
		return item, exists, err
	}
	if err := m.services.Tools.DeleteTool(ctx, id); err != nil {
		return toolkit.Tool{}, false, toolServiceError(err)
	}
	if err := m.removeToolOwnerBinding(ctx, item); err != nil {
		return toolkit.Tool{}, false, m.rollbackTool(ctx, item, err)
	}
	return item, true, nil
}

func toolOwnerBindingChanged(existing, desired toolkit.Tool) bool {
	existingOwner, existingHasOwner := deviceToolOwner(existing)
	if !existingHasOwner {
		return false
	}
	desiredOwner, desiredHasOwner := deviceToolOwner(desired)
	return !desiredHasOwner || desiredOwner != existingOwner
}

func deviceToolOwner(tool toolkit.Tool) (string, bool) {
	if tool.Source != toolkit.ToolSourceDevice || tool.OwnerPeer == nil {
		return "", false
	}
	owner := strings.TrimSpace(*tool.OwnerPeer)
	return owner, owner != ""
}

func (m *Manager) removeToolOwnerBinding(ctx context.Context, tool toolkit.Tool) error {
	owner, ok := deviceToolOwner(tool)
	extraIDs := []string{}
	if ok {
		extraIDs = append(extraIDs, toolkit.LegacyToolOwnerPolicyBindingID(tool.ID, owner))
	}
	if err := m.removeOwnedResourceOwner(ctx, apitypes.ACLResourceKindTool, tool.ID, extraIDs...); err != nil {
		return applyError(500, "TOOL_OWNER_ACL_CLEANUP_FAILED", err.Error())
	}
	return nil
}

func (m *Manager) rollbackToolWrite(ctx context.Context, existing toolkit.Tool, exists bool, writtenID string, cause error) error {
	if exists {
		return m.rollbackTool(ctx, existing, cause)
	}
	if err := m.services.Tools.DeleteTool(context.WithoutCancel(ctx), writtenID); err != nil {
		return applyError(500, "TOOL_OWNER_ACL_ROLLBACK_FAILED", fmt.Sprintf("%v; rollback failed: %v", cause, err))
	}
	return cause
}

func (m *Manager) rollbackTool(ctx context.Context, tool toolkit.Tool, cause error) error {
	if _, err := m.services.Tools.PutTool(context.WithoutCancel(ctx), tool); err != nil {
		return applyError(500, "TOOL_OWNER_ACL_ROLLBACK_FAILED", fmt.Sprintf("%v; rollback failed: %v", cause, err))
	}
	return cause
}

func resourceFromTool(item toolkit.Tool) (apitypes.Resource, error) {
	spec, err := toolkit.ToSpec(item)
	if err != nil {
		return apitypes.Resource{}, err
	}
	return marshalResource(apitypes.ToolResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ToolResourceKindTool,
		Metadata:   apitypes.ResourceMetadata{Name: item.ID},
		Spec:       spec,
	})
}

func toolServiceError(err error) error {
	if errors.Is(err, toolkit.ErrInvalidTool) {
		return applyError(400, "INVALID_TOOL", err.Error())
	}
	return err
}
