package resourcemanager

import (
	"context"
	"errors"

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
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindTool, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.Tools.PutTool(ctx, desired); err != nil {
		return apitypes.ApplyResult{}, toolServiceError(err)
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
	stored, err := m.services.Tools.PutTool(ctx, tool)
	if err != nil {
		return apitypes.Resource{}, toolServiceError(err)
	}
	return resourceFromTool(stored)
}

func (m *Manager) deleteTool(ctx context.Context, id string) (toolkit.Tool, bool, error) {
	item, exists, err := m.getTool(ctx, id)
	if err != nil || !exists {
		return item, exists, err
	}
	if err := m.services.Tools.DeleteTool(ctx, id); err != nil {
		return toolkit.Tool{}, false, toolServiceError(err)
	}
	return item, true, nil
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
