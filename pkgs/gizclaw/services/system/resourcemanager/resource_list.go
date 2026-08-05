package resourcemanager

import (
	"context"
	"encoding/json"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func (m *Manager) applyResourceList(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE_LIST", err.Error())
	}
	var envelope struct {
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE_LIST", err.Error())
	}
	if len(envelope.Metadata) != 0 && string(envelope.Metadata) != "null" {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE_LIST", "ResourceList must not contain metadata")
	}
	list, err := resource.AsResourceListResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE_LIST", err.Error())
	}
	if list.ApiVersion != apitypes.ResourceAPIVersionGizclawAdminv1alpha1 {
		return apitypes.ApplyResult{}, applyError(400, "UNSUPPORTED_RESOURCE_VERSION", "unsupported ResourceList apiVersion")
	}
	items := make([]apitypes.ApplyResult, 0, len(list.Spec.Items))
	action := apitypes.ApplyActionUnchanged
	for _, concrete := range list.Spec.Items {
		item, err := resourceFromConcrete(concrete)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE_LIST", err.Error())
		}
		kind, err := resourceKind(item)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		if kind == apitypes.ResourceKindResourceList {
			return apitypes.ApplyResult{}, applyError(400, "INVALID_RESOURCE_LIST", "ResourceList items must be concrete resources")
		}
		result, err := m.Apply(ctx, item)
		if err != nil {
			return apitypes.ApplyResult{}, err
		}
		items = append(items, result)
		if result.Action != apitypes.ApplyActionUnchanged {
			action = apitypes.ApplyActionApplied
		}
	}
	result := applyResult(action, apitypes.ResourceKindResourceList)
	result.Items = &items
	return result, nil
}

func resourceFromResourceList(items []apitypes.Resource) (apitypes.Resource, error) {
	concrete := make([]apitypes.ConcreteResource, 0, len(items))
	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			return apitypes.Resource{}, err
		}
		var converted apitypes.ConcreteResource
		if err := json.Unmarshal(data, &converted); err != nil {
			return apitypes.Resource{}, err
		}
		concrete = append(concrete, converted)
	}
	return marshalResource(apitypes.ResourceListResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ResourceListResourceKind(apitypes.ResourceKindResourceList),
		Spec:       apitypes.ResourceListSpec{Items: concrete},
	})
}

func resourceFromConcrete(item apitypes.ConcreteResource) (apitypes.Resource, error) {
	data, err := json.Marshal(item)
	if err != nil {
		return apitypes.Resource{}, err
	}
	var resource apitypes.Resource
	if err := json.Unmarshal(data, &resource); err != nil {
		return apitypes.Resource{}, err
	}
	return resource, nil
}
