CREATE UNIQUE INDEX IF NOT EXISTS idx_acl_policy_bindings_subject_resource_role
ON acl_policy_bindings(subject_kind, subject_id, resource_kind, resource_id, role);

CREATE INDEX IF NOT EXISTS idx_acl_policy_bindings_subject_resource
ON acl_policy_bindings(subject_kind, subject_id, resource_kind, resource_id);

CREATE INDEX IF NOT EXISTS idx_acl_policy_bindings_role
ON acl_policy_bindings(role);

CREATE INDEX IF NOT EXISTS idx_acl_policy_bindings_subject_display_order
ON acl_policy_bindings(subject_kind, subject_id, display_order, id);

CREATE INDEX IF NOT EXISTS idx_acl_policy_bindings_resource_display_order
ON acl_policy_bindings(resource_kind, resource_id, display_order, id);

CREATE INDEX IF NOT EXISTS idx_acl_policy_bindings_expires_at
ON acl_policy_bindings(expires_at);

CREATE INDEX IF NOT EXISTS idx_acl_binding_permissions_subject_resource_permission
ON acl_binding_permissions(subject_kind, subject_id, resource_kind, permission, resource_id);

CREATE INDEX IF NOT EXISTS idx_acl_binding_permissions_subject_resource
ON acl_binding_permissions(subject_kind, subject_id, resource_kind, resource_id);

CREATE INDEX IF NOT EXISTS idx_acl_binding_permissions_expires_at
ON acl_binding_permissions(expires_at);

CREATE INDEX IF NOT EXISTS idx_acl_binding_permissions_not_before
ON acl_binding_permissions(not_before);
