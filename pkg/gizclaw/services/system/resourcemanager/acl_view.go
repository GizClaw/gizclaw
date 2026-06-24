package resourcemanager

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
)

func (m *Manager) applyACLView(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ACL == nil {
		return apitypes.ApplyResult{}, missingService("acl")
	}
	item, err := resource.AsACLViewResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_ACL_VIEW_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getACLView(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(aclViewSpec(existing), normalizeACLViewSpec(item.Spec))
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindACLView, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.ACL.PutView(ctx, name, item.Spec); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindACLView, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindACLView, item.Metadata.Name), nil
}

func (m *Manager) getACLView(ctx context.Context, name string) (apitypes.ACLView, bool, error) {
	view, err := m.services.ACL.GetView(ctx, name)
	if errors.Is(err, acl.ErrViewNotFound) {
		return apitypes.ACLView{}, false, nil
	}
	if err != nil {
		return apitypes.ACLView{}, false, err
	}
	return view, true, nil
}

func aclViewSpec(view apitypes.ACLView) apitypes.ACLViewSpec {
	return normalizeACLViewSpec(apitypes.ACLViewSpec{
		Description: view.Description,
	})
}

func normalizeACLViewSpec(spec apitypes.ACLViewSpec) apitypes.ACLViewSpec {
	return apitypes.ACLViewSpec{
		Description: spec.Description,
	}
}

func resourceFromACLView(item apitypes.ACLView) (apitypes.Resource, error) {
	return marshalResource(apitypes.ACLViewResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ACLViewResourceKind(apitypes.ResourceKindACLView),
		Metadata:   apitypes.ResourceMetadata{Name: item.Name},
		Spec:       aclViewSpec(item),
	})
}
