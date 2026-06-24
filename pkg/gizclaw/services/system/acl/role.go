package acl

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

type ListRolesRequest struct {
	Cursor string
	Limit  int
}

func (s *Server) ListRoles(ctx context.Context, request ListRolesRequest) ([]apitypes.ACLRole, bool, *string, error) {
	if err := s.validateStore(); err != nil {
		return nil, false, nil, err
	}
	cursor, limit := normalizeListParams(request.Cursor, request.Limit)
	rows, err := s.DB.QueryContext(ctx, `
SELECT name, permissions_json, created_at, updated_at
FROM acl_roles
WHERE name > ?
ORDER BY name
LIMIT ?`, cursor, limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	roles := make([]apitypes.ACLRole, 0, limit+1)
	for rows.Next() {
		role, err := scanRole(rows)
		if err != nil {
			return nil, false, nil, err
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	hasNext := len(roles) > limit
	if !hasNext {
		return roles, false, nil, nil
	}
	roles = roles[:limit]
	nextCursor := roles[len(roles)-1].Name
	return roles, true, &nextCursor, nil
}

func (s *Server) CreateRole(ctx context.Context, name string, permissions apitypes.ACLPermissionList) (apitypes.ACLRole, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLRole{}, err
	}
	role, err := newRole(name, permissions, s.now())
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	if _, err := s.GetRole(ctx, role.Name); err == nil {
		return apitypes.ACLRole{}, ErrRoleAlreadyExists
	} else if !errors.Is(err, ErrRoleNotFound) {
		return apitypes.ACLRole{}, err
	}
	if err := insertRole(ctx, s.DB, role); err != nil {
		return apitypes.ACLRole{}, err
	}
	return role, nil
}

func (s *Server) PutRole(ctx context.Context, name string, permissions apitypes.ACLPermissionList) (apitypes.ACLRole, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLRole{}, err
	}
	name, err := validateName(name, "role name")
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	permissions, err = normalizePermissions(permissions)
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	now := s.now()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	defer tx.Rollback()
	existing, err := getRole(ctx, tx, name)
	switch {
	case err == nil:
		existing.Permissions = permissions
		existing.UpdatedAt = now
	case errors.Is(err, ErrRoleNotFound):
		existing = apitypes.ACLRole{Name: name, Permissions: permissions, CreatedAt: now, UpdatedAt: now}
	default:
		return apitypes.ACLRole{}, err
	}
	data, err := json.Marshal(existing.Permissions)
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO acl_roles (name, permissions_json, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET permissions_json = excluded.permissions_json, updated_at = excluded.updated_at`,
		existing.Name, string(data), formatTime(existing.CreatedAt), formatTime(existing.UpdatedAt),
	); err != nil {
		return apitypes.ACLRole{}, err
	}
	if err := refreshRoleBindingPermissions(ctx, tx, existing.Name, existing.Permissions); err != nil {
		return apitypes.ACLRole{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.ACLRole{}, err
	}
	return existing, nil
}

func (s *Server) GetRole(ctx context.Context, name string) (apitypes.ACLRole, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLRole{}, err
	}
	name, err := validateName(name, "role name")
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	return getRole(ctx, s.DB, name)
}

func (s *Server) DeleteRole(ctx context.Context, name string) (apitypes.ACLRole, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLRole{}, err
	}
	name, err := validateName(name, "role name")
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	defer tx.Rollback()
	role, err := getRole(ctx, tx, name)
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM acl_roles WHERE name = ?`, name); err != nil {
		return apitypes.ACLRole{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM acl_binding_permissions WHERE binding_id IN (SELECT id FROM acl_policy_bindings WHERE role = ?)`, name); err != nil {
		return apitypes.ACLRole{}, err
	}
	if err := tx.Commit(); err != nil {
		return apitypes.ACLRole{}, err
	}
	return role, nil
}

type roleScanner interface {
	Scan(dest ...any) error
}

func newRole(name string, permissions apitypes.ACLPermissionList, now time.Time) (apitypes.ACLRole, error) {
	name, err := validateName(name, "role name")
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	permissions, err = normalizePermissions(permissions)
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	return apitypes.ACLRole{
		Name:        name,
		Permissions: permissions,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func normalizePermissions(permissions apitypes.ACLPermissionList) (apitypes.ACLPermissionList, error) {
	values := make(apitypes.ACLPermissionList, 0, len(permissions))
	seen := map[apitypes.ACLPermission]struct{}{}
	for _, permission := range permissions {
		permission = apitypes.ACLPermission(strings.TrimSpace(string(permission)))
		if string(permission) == "" {
			return nil, errors.New("acl: permission is required")
		}
		if !permission.Valid() {
			return nil, errors.New("acl: unsupported permission")
		}
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		values = append(values, permission)
	}
	return values, nil
}

func insertRole(ctx context.Context, exec sqlExecutor, role apitypes.ACLRole) error {
	data, err := json.Marshal(role.Permissions)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `
INSERT INTO acl_roles (name, permissions_json, created_at, updated_at)
VALUES (?, ?, ?, ?)`, role.Name, string(data), formatTime(role.CreatedAt), formatTime(role.UpdatedAt))
	return err
}

func getRole(ctx context.Context, query sqlQuerier, name string) (apitypes.ACLRole, error) {
	return scanRole(query.QueryRowContext(ctx, `
SELECT name, permissions_json, created_at, updated_at
FROM acl_roles
WHERE name = ?`, name))
}

func scanRole(row roleScanner) (apitypes.ACLRole, error) {
	var role apitypes.ACLRole
	var permissionsJSON, createdAt, updatedAt string
	if err := row.Scan(&role.Name, &permissionsJSON, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apitypes.ACLRole{}, ErrRoleNotFound
		}
		return apitypes.ACLRole{}, err
	}
	if err := json.Unmarshal([]byte(permissionsJSON), &role.Permissions); err != nil {
		return apitypes.ACLRole{}, err
	}
	var err error
	role.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	role.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return apitypes.ACLRole{}, err
	}
	return role, nil
}
