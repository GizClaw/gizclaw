package resourcemanager

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

func (m *Manager) applyResourceList(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	list, err := resource.AsResourceListResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE_LIST", err.Error())
	}
	if err := validateResourceHeader(list.ApiVersion, list.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	items := make([]apitypes.ApplyResult, 0, len(list.Spec.Items))
	action := apitypes.ApplyActionUnchanged
	for _, item := range list.Spec.Items {
		result, err := m.Apply(ctx, item)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		items = append(items, result)
		if result.Action != apitypes.ApplyActionUnchanged {
			action = apitypes.ApplyActionApplied
		}
	}
	result := applyResult(action, apitypes.ResourceKindResourceList, list.Metadata.Name)
	result.Items = &items
	return result, nil
}

func resourceFromResourceList(name string, items []apitypes.Resource) (apitypes.Resource, error) {
	return marshalResource(apitypes.ResourceListResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ResourceListResourceKind(apitypes.ResourceKindResourceList),
		Metadata:   apitypes.ResourceMetadata{Name: name},
		Spec:       apitypes.ResourceListSpec{Items: items},
	})
}
