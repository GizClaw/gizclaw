package acl

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

type ListViewsRequest struct {
	Cursor string
	Limit  int
}

func (s *Server) ListViews(ctx context.Context, request ListViewsRequest) ([]apitypes.ACLView, bool, *string, error) {
	if err := s.validateStore(); err != nil {
		return nil, false, nil, err
	}
	cursor, limit := normalizeListParams(request.Cursor, request.Limit)
	query := `
SELECT name, description, created_at, updated_at
FROM acl_views`
	args := []any{}
	if cursor != "" {
		query += `
WHERE name > ?`
		args = append(args, cursor)
	}
	query += `
ORDER BY name
LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	views := make([]apitypes.ACLView, 0, limit+1)
	for rows.Next() {
		view, err := scanView(rows)
		if err != nil {
			return nil, false, nil, err
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	hasNext := len(views) > limit
	if !hasNext {
		return views, false, nil, nil
	}
	views = views[:limit]
	nextCursor := views[len(views)-1].Name
	return views, true, &nextCursor, nil
}

func (s *Server) CreateView(ctx context.Context, name string, spec apitypes.ACLViewSpec) (apitypes.ACLView, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLView{}, err
	}
	view, err := newView(name, spec, s.now())
	if err != nil {
		return apitypes.ACLView{}, err
	}
	if _, err := s.GetView(ctx, view.Name); err == nil {
		return apitypes.ACLView{}, ErrViewAlreadyExists
	} else if !errors.Is(err, ErrViewNotFound) {
		return apitypes.ACLView{}, err
	}
	if err := insertView(ctx, s.DB, view); err != nil {
		return apitypes.ACLView{}, err
	}
	return view, nil
}

func (s *Server) PutView(ctx context.Context, name string, spec apitypes.ACLViewSpec) (apitypes.ACLView, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLView{}, err
	}
	name, err := validateName(name, "view name")
	if err != nil {
		return apitypes.ACLView{}, err
	}
	now := s.now()
	existing, err := getView(ctx, s.DB, name)
	switch {
	case err == nil:
		existing.Description = normalizedViewDescription(spec.Description)
		existing.UpdatedAt = now
	case errors.Is(err, ErrViewNotFound):
		existing, err = newView(name, spec, now)
		if err != nil {
			return apitypes.ACLView{}, err
		}
	default:
		return apitypes.ACLView{}, err
	}
	if err := upsertView(ctx, s.DB, existing); err != nil {
		return apitypes.ACLView{}, err
	}
	return existing, nil
}

func (s *Server) GetView(ctx context.Context, name string) (apitypes.ACLView, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLView{}, err
	}
	name, err := validateName(name, "view name")
	if err != nil {
		return apitypes.ACLView{}, err
	}
	return getView(ctx, s.DB, name)
}

func (s *Server) DeleteView(ctx context.Context, name string) (apitypes.ACLView, error) {
	if err := s.validateStore(); err != nil {
		return apitypes.ACLView{}, err
	}
	name, err := validateName(name, "view name")
	if err != nil {
		return apitypes.ACLView{}, err
	}
	view, err := getView(ctx, s.DB, name)
	if err != nil {
		return apitypes.ACLView{}, err
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM acl_views WHERE name = ?`, name)
	return view, err
}

type viewScanner interface {
	Scan(dest ...any) error
}

func newView(name string, spec apitypes.ACLViewSpec, now time.Time) (apitypes.ACLView, error) {
	name, err := validateName(name, "view name")
	if err != nil {
		return apitypes.ACLView{}, err
	}
	return apitypes.ACLView{
		Name:        name,
		Description: normalizedViewDescription(spec.Description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func normalizedViewDescription(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func insertView(ctx context.Context, exec sqlExecutor, view apitypes.ACLView) error {
	_, err := exec.ExecContext(ctx, `
INSERT INTO acl_views (name, description, created_at, updated_at)
VALUES (?, ?, ?, ?)`,
		view.Name,
		nullableString(view.Description),
		formatTime(view.CreatedAt),
		formatTime(view.UpdatedAt),
	)
	return err
}

func upsertView(ctx context.Context, exec sqlExecutor, view apitypes.ACLView) error {
	_, err := exec.ExecContext(ctx, `
INSERT INTO acl_views (name, description, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
	description = excluded.description,
	updated_at = excluded.updated_at`,
		view.Name,
		nullableString(view.Description),
		formatTime(view.CreatedAt),
		formatTime(view.UpdatedAt),
	)
	return err
}

func getView(ctx context.Context, query sqlQuerier, name string) (apitypes.ACLView, error) {
	return scanView(query.QueryRowContext(ctx, `
SELECT name, description, created_at, updated_at
FROM acl_views
WHERE name = ?`, name))
}

func scanView(scanner viewScanner) (apitypes.ACLView, error) {
	var view apitypes.ACLView
	var description sql.NullString
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(&view.Name, &description, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apitypes.ACLView{}, ErrViewNotFound
		}
		return apitypes.ACLView{}, err
	}
	if description.Valid {
		view.Description = &description.String
	}
	var err error
	view.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return apitypes.ACLView{}, err
	}
	view.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return apitypes.ACLView{}, err
	}
	return view, nil
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
