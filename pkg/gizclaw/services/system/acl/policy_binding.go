package acl

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

const (
	PolicyBindingOrderByID           = "id"
	PolicyBindingOrderByDisplayOrder = "display_order"
)

type ListPolicyBindingsRequest struct {
	Cursor           string
	Limit            int
	OrderBy          string
	SubjectKind      apitypes.ACLSubjectKind
	SubjectID        string
	ResourceKind     apitypes.ACLResourceKind
	ResourceID       string
	ResourceIDPrefix string
	Role             string
	Permission       apitypes.ACLPermission
}

func (s *Server) ListPolicyBindings(ctx context.Context, request ListPolicyBindingsRequest) ([]apitypes.ACLPolicyBinding, bool, *string, error) {
	if err := s.validateStore(); err != nil {
		return nil, false, nil, err
	}
	cursor, limit := normalizeListParams(request.Cursor, request.Limit)
	query, args, err := listPolicyBindingsQuery(request, cursor, limit)
	if err != nil {
		return nil, false, nil, err
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	bindings := make([]apitypes.ACLPolicyBinding, 0, limit+1)
	for rows.Next() {
		binding, err := scanPolicyBinding(rows)
		if err != nil {
			return nil, false, nil, err
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	hasNext := len(bindings) > limit
	if !hasNext {
		return bindings, false, nil, nil
	}
	bindings = bindings[:limit]
	nextCursor, err := policyBindingCursor(request.OrderBy, bindings[len(bindings)-1])
	if err != nil {
		return nil, false, nil, err
	}
	return bindings, true, &nextCursor, nil
}

func listPolicyBindingsQuery(request ListPolicyBindingsRequest, cursor string, limit int) (string, []any, error) {
	orderBy, err := policyBindingOrderBy(request.OrderBy)
	if err != nil {
		return "", nil, err
	}
	var query strings.Builder
	query.WriteString(`
SELECT id, display_order, subject_kind, subject_id, resource_kind, resource_id, role, not_before, expires_at, created_at, updated_at
FROM acl_policy_bindings
WHERE 1 = 1`)
	args := []any{}
	if cursor != "" {
		switch orderBy {
		case PolicyBindingOrderByDisplayOrder:
			query.WriteString(`
  AND (display_order > (SELECT display_order FROM acl_policy_bindings WHERE id = ?)
    OR (display_order = (SELECT display_order FROM acl_policy_bindings WHERE id = ?) AND id > ?))`)
			args = append(args, cursor, cursor, cursor)
		default:
			query.WriteString(`
  AND id > ?`)
			args = append(args, cursor)
		}
	}
	if request.SubjectKind != "" {
		if !request.SubjectKind.Valid() {
			return "", nil, errors.New("acl: unsupported subject kind")
		}
		query.WriteString(`
  AND subject_kind = ?`)
		args = append(args, request.SubjectKind)
	}
	if strings.TrimSpace(request.SubjectID) != "" {
		query.WriteString(`
  AND subject_id = ?`)
		args = append(args, strings.TrimSpace(request.SubjectID))
	}
	if request.ResourceKind != "" {
		if !request.ResourceKind.Valid() {
			return "", nil, errors.New("acl: unsupported resource kind")
		}
		query.WriteString(`
  AND resource_kind = ?`)
		args = append(args, request.ResourceKind)
	}
	if strings.TrimSpace(request.ResourceID) != "" {
		query.WriteString(`
  AND resource_id = ?`)
		args = append(args, strings.TrimSpace(request.ResourceID))
	}
	if strings.TrimSpace(request.ResourceIDPrefix) != "" {
		query.WriteString(`
  AND resource_id LIKE ? ESCAPE '\'`)
		args = append(args, likePrefixPattern(strings.TrimSpace(request.ResourceIDPrefix)))
	}
	if strings.TrimSpace(request.Role) != "" {
		query.WriteString(`
  AND role = ?`)
		args = append(args, strings.TrimSpace(request.Role))
	}
	if strings.TrimSpace(string(request.Permission)) != "" {
		permission := apitypes.ACLPermission(strings.TrimSpace(string(request.Permission)))
		if !permission.Valid() {
			return "", nil, errors.New("acl: unsupported permission")
		}
		query.WriteString(`
  AND EXISTS (
    SELECT 1
    FROM acl_binding_permissions
    WHERE acl_binding_permissions.binding_id = acl_policy_bindings.id
      AND acl_binding_permissions.permission = ?
  )`)
		args = append(args, string(permission))
	}
	switch orderBy {
	case PolicyBindingOrderByDisplayOrder:
		query.WriteString(`
ORDER BY display_order, id`)
	default:
		query.WriteString(`
ORDER BY id`)
	}
	query.WriteString(`
LIMIT ?`)
	args = append(args, limit+1)
	return query.String(), args, nil
}

func likePrefixPattern(prefix string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(prefix) + "%"
}

func (s *Server) CreatePolicyBinding(ctx context.Context, id string, displayOrder float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	binding, err := s.newPolicyBinding(id, displayOrder, policy)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if _, err := s.GetPolicyBinding(ctx, binding.Id); err == nil {
		return apitypes.ACLPolicyBinding{}, ErrPolicyBindingAlreadyExists
	} else if !errors.Is(err, ErrPolicyBindingNotFound) {
		return apitypes.ACLPolicyBinding{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	defer tx.Rollback()
	role, err := getRole(ctx, tx, binding.Policy.Role)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if ok, err := policyBindingConflict(ctx, tx, binding.Id, binding.Policy); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	} else if ok {
		return apitypes.ACLPolicyBinding{}, ErrPolicyBindingAlreadyExists
	}
	if err := insertPolicyBinding(ctx, tx, binding); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if err := insertPolicyBindingPermissions(ctx, tx, binding, role.Permissions); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	return binding, nil
}

func (s *Server) PutPolicyBinding(ctx context.Context, id string, displayOrder float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	id, err := validateName(id, "policy binding id")
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	policy, err = normalizePolicy(policy)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	displayOrder, err = normalizeDisplayOrder(displayOrder)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	now := s.now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	defer tx.Rollback()
	role, err := getRole(ctx, tx, policy.Role)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	existing, err := getPolicyBinding(ctx, tx, id)
	switch {
	case err == nil:
		existing.DisplayOrder = displayOrder
		existing.Policy = policy
		existing.UpdatedAt = now
	case errors.Is(err, ErrPolicyBindingNotFound):
		existing = apitypes.ACLPolicyBinding{Id: id, DisplayOrder: displayOrder, Policy: policy, CreatedAt: now, UpdatedAt: now}
	default:
		return apitypes.ACLPolicyBinding{}, err
	}
	if ok, err := policyBindingConflict(ctx, tx, existing.Id, existing.Policy); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	} else if ok {
		return apitypes.ACLPolicyBinding{}, ErrPolicyBindingAlreadyExists
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO acl_policy_bindings (
	id, display_order, subject_kind, subject_id, resource_kind, resource_id, role, not_before, expires_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	display_order = excluded.display_order,
	subject_kind = excluded.subject_kind,
	subject_id = excluded.subject_id,
	resource_kind = excluded.resource_kind,
	resource_id = excluded.resource_id,
	role = excluded.role,
	not_before = excluded.not_before,
	expires_at = excluded.expires_at,
	updated_at = excluded.updated_at`,
		existing.Id,
		existing.DisplayOrder,
		existing.Policy.Subject.Kind,
		existing.Policy.Subject.Id,
		existing.Policy.Resource.Kind,
		existing.Policy.Resource.Id,
		existing.Policy.Role,
		nullableTime(existing.Policy.NotBefore),
		nullableTime(existing.Policy.ExpiresAt),
		formatTime(existing.CreatedAt),
		formatTime(existing.UpdatedAt),
	); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM acl_binding_permissions WHERE binding_id = ?`, existing.Id); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if err := insertPolicyBindingPermissions(ctx, tx, existing, role.Permissions); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	return existing, nil
}

func (s *Server) GetPolicyBinding(ctx context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	id, err := validateName(id, "policy binding id")
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	return getPolicyBinding(ctx, s.DB, id)
}

func (s *Server) DeletePolicyBinding(ctx context.Context, id string) (apitypes.ACLPolicyBinding, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	id, err := validateName(id, "policy binding id")
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	defer tx.Rollback()
	binding, err := getPolicyBinding(ctx, tx, id)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if err := deletePolicyBinding(ctx, tx, id); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	return binding, nil
}

type policyBindingScanner interface {
	Scan(dest ...any) error
}

func (s *Server) newPolicyBinding(id string, displayOrder float64, policy apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error) {
	displayOrder, err := normalizeDisplayOrder(displayOrder)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	policy, err = normalizePolicy(policy)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	id, err = policyBindingIDOrGenerate(id, policy)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	now := s.now()
	return apitypes.ACLPolicyBinding{
		Id:           id,
		DisplayOrder: displayOrder,
		Policy:       policy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func normalizePolicy(policy apitypes.ACLPolicy) (apitypes.ACLPolicy, error) {
	if _, err := CanonicalSubject(policy.Subject); err != nil {
		return apitypes.ACLPolicy{}, err
	}
	if _, err := CanonicalResource(policy.Resource); err != nil {
		return apitypes.ACLPolicy{}, err
	}
	role, err := validateName(policy.Role, "role name")
	if err != nil {
		return apitypes.ACLPolicy{}, err
	}
	policy.Subject.Id = strings.TrimSpace(policy.Subject.Id)
	policy.Resource.Id = strings.TrimSpace(policy.Resource.Id)
	policy.Role = role
	return policy, nil
}

func insertPolicyBinding(ctx context.Context, exec sqlExecutor, binding apitypes.ACLPolicyBinding) error {
	_, err := exec.ExecContext(ctx, `
INSERT INTO acl_policy_bindings (
	id, display_order, subject_kind, subject_id, resource_kind, resource_id, role, not_before, expires_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		binding.Id,
		binding.DisplayOrder,
		binding.Policy.Subject.Kind,
		binding.Policy.Subject.Id,
		binding.Policy.Resource.Kind,
		binding.Policy.Resource.Id,
		binding.Policy.Role,
		nullableTime(binding.Policy.NotBefore),
		nullableTime(binding.Policy.ExpiresAt),
		formatTime(binding.CreatedAt),
		formatTime(binding.UpdatedAt),
	)
	return err
}

func getPolicyBinding(ctx context.Context, query sqlQuerier, id string) (apitypes.ACLPolicyBinding, error) {
	return scanPolicyBinding(query.QueryRowContext(ctx, `
SELECT id, display_order, subject_kind, subject_id, resource_kind, resource_id, role, not_before, expires_at, created_at, updated_at
FROM acl_policy_bindings
WHERE id = ?`, id))
}

func scanPolicyBinding(row policyBindingScanner) (apitypes.ACLPolicyBinding, error) {
	var binding apitypes.ACLPolicyBinding
	var notBefore, expiresAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(
		&binding.Id,
		&binding.DisplayOrder,
		&binding.Policy.Subject.Kind,
		&binding.Policy.Subject.Id,
		&binding.Policy.Resource.Kind,
		&binding.Policy.Resource.Id,
		&binding.Policy.Role,
		&notBefore,
		&expiresAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apitypes.ACLPolicyBinding{}, ErrPolicyBindingNotFound
		}
		return apitypes.ACLPolicyBinding{}, err
	}
	var err error
	binding.Policy.NotBefore, err = parseNullableTime(notBefore)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	binding.Policy.ExpiresAt, err = parseNullableTime(expiresAt)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	binding.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	binding.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return apitypes.ACLPolicyBinding{}, err
	}
	return binding, nil
}

func insertPolicyBindingPermissions(ctx context.Context, exec sqlExecutor, binding apitypes.ACLPolicyBinding, permissions apitypes.ACLPermissionList) error {
	for _, permission := range permissions {
		if _, err := exec.ExecContext(ctx, `
INSERT INTO acl_binding_permissions (
	binding_id, subject_kind, subject_id, resource_kind, resource_id, permission, not_before, expires_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			binding.Id,
			binding.Policy.Subject.Kind,
			binding.Policy.Subject.Id,
			binding.Policy.Resource.Kind,
			binding.Policy.Resource.Id,
			string(permission),
			nullableTime(binding.Policy.NotBefore),
			nullableTime(binding.Policy.ExpiresAt),
		); err != nil {
			return err
		}
	}
	return nil
}

func refreshRoleBindingPermissions(ctx context.Context, tx *sql.Tx, roleName string, permissions apitypes.ACLPermissionList) error {
	rows, err := tx.QueryContext(ctx, `
SELECT id, display_order, subject_kind, subject_id, resource_kind, resource_id, role, not_before, expires_at, created_at, updated_at
FROM acl_policy_bindings
WHERE role = ?`, roleName)
	if err != nil {
		return err
	}
	defer rows.Close()
	bindings := []apitypes.ACLPolicyBinding{}
	for rows.Next() {
		binding, err := scanPolicyBinding(rows)
		if err != nil {
			return err
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM acl_binding_permissions WHERE binding_id IN (SELECT id FROM acl_policy_bindings WHERE role = ?)`, roleName); err != nil {
		return err
	}
	for _, binding := range bindings {
		if err := insertPolicyBindingPermissions(ctx, tx, binding, permissions); err != nil {
			return err
		}
	}
	return nil
}

func deletePolicyBinding(ctx context.Context, exec sqlExecutor, id string) error {
	if _, err := exec.ExecContext(ctx, `DELETE FROM acl_binding_permissions WHERE binding_id = ?`, id); err != nil {
		return err
	}
	_, err := exec.ExecContext(ctx, `DELETE FROM acl_policy_bindings WHERE id = ?`, id)
	return err
}

func policyBindingConflict(ctx context.Context, query sqlQuerier, id string, policy apitypes.ACLPolicy) (bool, error) {
	var count int
	err := query.QueryRowContext(ctx, `
SELECT count(*)
FROM acl_policy_bindings
WHERE id <> ?
  AND subject_kind = ?
  AND subject_id = ?
  AND resource_kind = ?
  AND resource_id = ?
  AND role = ?`,
		id,
		policy.Subject.Kind,
		policy.Subject.Id,
		policy.Resource.Kind,
		policy.Resource.Id,
		policy.Role,
	).Scan(&count)
	return count > 0, err
}

func policyBindingOrderBy(orderBy string) (string, error) {
	switch strings.TrimSpace(orderBy) {
	case "", PolicyBindingOrderByID:
		return PolicyBindingOrderByID, nil
	case PolicyBindingOrderByDisplayOrder:
		return PolicyBindingOrderByDisplayOrder, nil
	default:
		return "", errors.New("acl: unsupported policy binding order")
	}
}

func policyBindingCursor(orderBy string, binding apitypes.ACLPolicyBinding) (string, error) {
	if _, err := policyBindingOrderBy(orderBy); err != nil {
		return "", err
	}
	return binding.Id, nil
}

func normalizeDisplayOrder(value float64) (float64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("acl: display order must be finite")
	}
	return value, nil
}
