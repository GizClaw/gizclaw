package resourcemanager

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
)

func (m *Manager) applyACLRole(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ACL == nil {
		return apitypes.ApplyResult{}, missingService("acl")
	}
	item, err := resource.AsACLRoleResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_ACL_ROLE_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getACLRole(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(aclRoleSpec(existing), normalizeACLRoleSpec(item.Spec))
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindACLRole, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.ACL.PutRole(ctx, name, item.Spec.Permissions); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindACLRole, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindACLRole, item.Metadata.Name), nil
}

func (m *Manager) getACLRole(ctx context.Context, name string) (apitypes.ACLRole, bool, error) {
	role, err := m.services.ACL.GetRole(ctx, name)
	if errors.Is(err, acl.ErrRoleNotFound) {
		return apitypes.ACLRole{}, false, nil
	}
	if err != nil {
		return apitypes.ACLRole{}, false, err
	}
	return role, true, nil
}

func aclRoleSpec(role apitypes.ACLRole) apitypes.ACLRoleSpec {
	return normalizeACLRoleSpec(apitypes.ACLRoleSpec{
		Permissions: role.Permissions,
	})
}

func normalizeACLRoleSpec(spec apitypes.ACLRoleSpec) apitypes.ACLRoleSpec {
	return apitypes.ACLRoleSpec{
		Permissions: spec.Permissions,
	}
}

func resourceFromACLRole(item apitypes.ACLRole) (apitypes.Resource, error) {
	return marshalResource(apitypes.ACLRoleResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ACLRoleResourceKind(apitypes.ResourceKindACLRole),
		Metadata:   apitypes.ResourceMetadata{Name: item.Name},
		Spec:       aclRoleSpec(item),
	})
}

func (m *Manager) applyACLPolicyBinding(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	if m.services.ACL == nil {
		return apitypes.ApplyResult{}, missingService("acl")
	}
	item, err := resource.AsACLPolicyBindingResource()
	if err != nil {
		return apitypes.ApplyResult{}, applyError(400, "INVALID_ACL_POLICY_BINDING_RESOURCE", err.Error())
	}
	if err := validateResourceHeader(item.ApiVersion, item.Metadata.Name); err != nil {
		return apitypes.ApplyResult{}, err
	}
	name := string(pathParam(item.Metadata.Name))
	existing, exists, err := m.getACLPolicyBinding(ctx, name)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		same, err := semanticEqual(aclPolicyBindingSpec(existing), item.Spec)
		if err != nil {
			return apitypes.ApplyResult{}, applyError(500, "RESOURCE_COMPARE_FAILED", err.Error())
		}
		if same {
			return applyResult(apitypes.ApplyActionUnchanged, apitypes.ResourceKindACLPolicyBinding, item.Metadata.Name), nil
		}
	}
	if _, err := m.services.ACL.PutPolicyBinding(ctx, name, 0, item.Spec); err != nil {
		return apitypes.ApplyResult{}, err
	}
	if exists {
		return applyResult(apitypes.ApplyActionUpdated, apitypes.ResourceKindACLPolicyBinding, item.Metadata.Name), nil
	}
	return applyResult(apitypes.ApplyActionCreated, apitypes.ResourceKindACLPolicyBinding, item.Metadata.Name), nil
}

func (m *Manager) getACLPolicyBinding(ctx context.Context, name string) (apitypes.ACLPolicyBinding, bool, error) {
	binding, err := m.services.ACL.GetPolicyBinding(ctx, name)
	if errors.Is(err, acl.ErrPolicyBindingNotFound) {
		return apitypes.ACLPolicyBinding{}, false, nil
	}
	if err != nil {
		return apitypes.ACLPolicyBinding{}, false, err
	}
	return binding, true, nil
}

func aclPolicyBindingSpec(binding apitypes.ACLPolicyBinding) apitypes.ACLPolicy {
	return binding.Policy
}

func resourceFromACLPolicyBinding(item apitypes.ACLPolicyBinding) (apitypes.Resource, error) {
	return marshalResource(apitypes.ACLPolicyBindingResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ACLPolicyBindingResourceKind(apitypes.ResourceKindACLPolicyBinding),
		Metadata:   apitypes.ResourceMetadata{Name: item.Id},
		Spec:       aclPolicyBindingSpec(item),
	})
}
