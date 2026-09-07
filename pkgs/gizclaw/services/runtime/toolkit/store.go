package toolkit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Server owns the SQL catalog of executable Tool declarations.
type Server struct {
	DB  *sqlx.DB
	Now func() time.Time
}

const toolColumns = "id,invoke_name,type,description,enabled,version,input_schema_json,triggers_json,metadata_json,http_json,created_at,updated_at,revision,incarnation"

// Initialize creates the Tool schema during Server startup.
func (s *Server) Initialize(ctx context.Context) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tools (
 id TEXT PRIMARY KEY CHECK(length(id)>0),
 invoke_name TEXT NOT NULL UNIQUE CHECK(length(invoke_name)>0),
 type TEXT NOT NULL CHECK(type IN ('http_request','client_rpc')),
 description TEXT,
 enabled BOOLEAN NOT NULL,
 version TEXT,
 input_schema_json TEXT NOT NULL,
 triggers_json TEXT NOT NULL,
 metadata_json TEXT NOT NULL,
 http_json TEXT NOT NULL,
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 incarnation TEXT NOT NULL
 )`)
	return err
}

func (s *Server) GetTool(ctx context.Context, name string) (Tool, error) {
	db, err := s.database()
	if err != nil {
		return Tool{}, err
	}
	name, err = normalizeToolName(name)
	if err != nil {
		return Tool{}, err
	}
	tool, _, err := scanTool(db.QueryRowContext(ctx, db.Rebind(`SELECT `+toolColumns+` FROM tools WHERE invoke_name=?`), name))
	return tool, err
}

func (s *Server) GetToolByID(ctx context.Context, id string) (Tool, error) {
	db, err := s.database()
	if err != nil {
		return Tool{}, err
	}
	if err := customid.ValidateResourceID(id); err != nil {
		return Tool{}, ErrToolNotFound
	}
	tool, _, err := scanTool(db.QueryRowContext(ctx, db.Rebind(`SELECT `+toolColumns+` FROM tools WHERE id=?`), id))
	return tool, err
}

// ListTools reads the catalog in bounded SQL pages ordered by canonical ID.
func (s *Server) ListTools(ctx context.Context) ([]Tool, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	var result []Tool
	cursor := ""
	for {
		page, err := listToolPage(ctx, db, cursor)
		if err != nil {
			return nil, err
		}
		result = append(result, page...)
		if len(page) < 256 {
			return result, nil
		}
		cursor = page[len(page)-1].ID
	}
}

func listToolPage(ctx context.Context, db *sqlx.DB, cursor string) ([]Tool, error) {
	rows, err := db.QueryContext(ctx, db.Rebind(`SELECT `+toolColumns+` FROM tools WHERE id>? ORDER BY id LIMIT 256`), cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := make([]Tool, 0, 256)
	for rows.Next() {
		tool, _, err := scanTool(rows)
		if err != nil {
			return nil, err
		}
		page = append(page, tool)
	}
	return page, rows.Err()
}

func (s *Server) CreateTool(ctx context.Context, tool Tool) (Tool, error) {
	db, err := s.database()
	if err != nil {
		return Tool{}, err
	}
	tool, err = normalizeToolDeclaration(tool)
	if err != nil {
		return Tool{}, err
	}
	if err := customid.ValidateResourceID(tool.ID); err != nil {
		return Tool{}, fmt.Errorf("%w: %v", ErrInvalidTool, err)
	}
	tool.CreatedAt = s.now().UTC()
	tool.UpdatedAt = tool.CreatedAt
	tool, err = NormalizeTool(tool)
	if err != nil {
		return Tool{}, err
	}
	config, err := toolConfigValues(tool)
	if err != nil {
		return Tool{}, err
	}
	args := []any{tool.ID, tool.InvokeName, string(tool.Type), tool.Description, tool.Enabled, tool.Version}
	args = append(args, config...)
	args = append(args, tool.CreatedAt.Format(time.RFC3339Nano), tool.UpdatedAt.Format(time.RFC3339Nano), uuid.NewString())
	result, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO tools(id,invoke_name,type,description,enabled,version,input_schema_json,triggers_json,metadata_json,http_json,created_at,updated_at,incarnation) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`), args...)
	if err != nil {
		return Tool{}, fmt.Errorf("toolkit: create tool: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Tool{}, err
	}
	if count == 0 {
		return Tool{}, fmt.Errorf("%w: tool ID or invoke_name already exists", ErrToolConflict)
	}
	return cloneTool(tool), nil
}

func (s *Server) PutTool(ctx context.Context, id string, desired Tool) (Tool, error) {
	db, err := s.database()
	if err != nil {
		return Tool{}, err
	}
	desired, err = normalizeToolDeclaration(desired)
	if err != nil {
		return Tool{}, err
	}
	if desired.ID != id {
		return Tool{}, fmt.Errorf("%w: id %q must match path id %q", ErrInvalidTool, desired.ID, id)
	}
	for range 16 {
		existing, revision, err := scanTool(db.QueryRowContext(ctx, db.Rebind(`SELECT `+toolColumns+` FROM tools WHERE id=?`), id))
		if err != nil {
			return Tool{}, err
		}
		if desired.InvokeName != existing.InvokeName {
			return Tool{}, fmt.Errorf("%w: invoke_name is immutable", ErrToolConflict)
		}
		tool := cloneTool(desired)
		tool.CreatedAt = existing.CreatedAt
		tool.UpdatedAt = s.now().UTC()
		retainDirectSecret(&tool, existing)
		tool, err = NormalizeTool(tool)
		if err != nil {
			return Tool{}, err
		}
		config, err := toolConfigValues(tool)
		if err != nil {
			return Tool{}, err
		}
		args := []any{string(tool.Type), tool.Description, tool.Enabled, tool.Version}
		args = append(args, config...)
		args = append(args, tool.UpdatedAt.Format(time.RFC3339Nano), id, revision.Version, revision.Incarnation)
		result, err := db.ExecContext(ctx, db.Rebind(`UPDATE tools SET type=?,description=?,enabled=?,version=?,input_schema_json=?,triggers_json=?,metadata_json=?,http_json=?,updated_at=?,revision=revision+1 WHERE id=? AND revision=? AND incarnation=?`), args...)
		if err != nil {
			return Tool{}, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return Tool{}, err
		}
		if count == 1 {
			return cloneTool(tool), nil
		}
	}
	return Tool{}, fmt.Errorf("%w: tool changed concurrently", ErrToolConflict)
}

func (s *Server) DeleteTool(ctx context.Context, id string) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM tools WHERE id=?`), id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrToolNotFound
	}
	return nil
}

type toolRevision struct {
	Version     int64
	Incarnation string
}

func scanTool(row interface{ Scan(...any) error }) (Tool, toolRevision, error) {
	var tool Tool
	var input, triggers, metadata, http, created, updated string
	var revision toolRevision
	err := row.Scan(&tool.ID, &tool.InvokeName, &tool.Type, &tool.Description, &tool.Enabled, &tool.Version, &input, &triggers, &metadata, &http, &created, &updated, &revision.Version, &revision.Incarnation)
	if errors.Is(err, sql.ErrNoRows) {
		return Tool{}, toolRevision{}, ErrToolNotFound
	}
	if err != nil {
		return Tool{}, toolRevision{}, err
	}
	for _, field := range []struct {
		raw    string
		target any
	}{{input, &tool.InputSchema}, {triggers, &tool.Triggers}, {metadata, &tool.Metadata}, {http, &tool.HTTP}} {
		if err := json.Unmarshal([]byte(field.raw), field.target); err != nil {
			return Tool{}, toolRevision{}, fmt.Errorf("toolkit: invalid stored configuration: %w", err)
		}
	}
	if metadata == "null" {
		tool.Metadata = nil
	}
	if tool.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Tool{}, toolRevision{}, err
	}
	if tool.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Tool{}, toolRevision{}, err
	}
	tool, err = NormalizeTool(tool)
	return tool, revision, err
}

func toolConfigValues(tool Tool) ([]any, error) {
	values := make([]any, 0, 4)
	for _, field := range []any{tool.InputSchema, tool.Triggers, tool.Metadata, tool.HTTP} {
		data, err := json.Marshal(field)
		if err != nil {
			return nil, err
		}
		values = append(values, string(data))
	}
	return values, nil
}

func (s *Server) database() (*sqlx.DB, error) {
	if s == nil || s.DB == nil {
		return nil, ErrNotConfigured
	}
	switch s.DB.DriverName() {
	case "sqlite", "postgres":
	default:
		return nil, fmt.Errorf("toolkit: unsupported SQL driver %q", s.DB.DriverName())
	}
	return s.DB, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func retainDirectSecret(desired *Tool, existing Tool) {
	if desired.HTTP == nil || existing.HTTP == nil || desired.HTTP.Auth.Method != existing.HTTP.Auth.Method {
		return
	}
	switch desired.HTTP.Auth.Method {
	case "bearer":
		if desired.HTTP.Auth.BearerToken == nil {
			desired.HTTP.Auth.BearerToken = cloneStringPtr(existing.HTTP.Auth.BearerToken)
		}
	case "header_api_key":
		if desired.HTTP.Auth.APIKey == nil {
			desired.HTTP.Auth.APIKey = cloneStringPtr(existing.HTTP.Auth.APIKey)
		}
	}
}

// MergeDirectSecrets retains omitted direct secrets only when the auth method
// is unchanged. It returns an independently owned, executable declaration.
func MergeDirectSecrets(desired, existing Tool) (Tool, error) {
	desired = cloneTool(desired)
	retainDirectSecret(&desired, existing)
	return NormalizeTool(desired)
}
