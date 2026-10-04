package graphstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
)

// ErrConflict indicates that another turn replaced the loaded Graph state.
var ErrConflict = errors.New("graph state: revision conflict")

// Load returns the current revision and typed fields of an active Graph scope.
func (s *Store) Load(ctx context.Context, contextID string) (genxeino.StateSnapshot, error) {
	data, err := s.loadCheckpoint(ctx, contextID)
	if err != nil {
		return genxeino.StateSnapshot{}, err
	}
	if data == nil {
		var retired bool
		if err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT retired FROM graph_state_scopes WHERE owner_id=? AND workspace_id=?`), s.owner, s.workspace).Scan(&retired); err != nil {
			return genxeino.StateSnapshot{}, err
		}
		if retired {
			return genxeino.StateSnapshot{}, ErrRetired
		}
		data = s.initial
		if len(data) == 0 {
			return genxeino.StateSnapshot{}, nil
		}
	}
	return decodeSnapshot(data)
}

func decodeSnapshot(data []byte) (genxeino.StateSnapshot, error) {
	var record stateRecord
	var err error
	if err := json.Unmarshal(data, &record); err != nil {
		return genxeino.StateSnapshot{}, fmt.Errorf("graph state: decode checkpoint: %w", err)
	}
	if record.FormatVersion < 0 || record.FormatVersion > 1 {
		return genxeino.StateSnapshot{}, fmt.Errorf("graph state: unsupported snapshot format %d", record.FormatVersion)
	}
	fields := make(map[string]any, len(record.Fields))
	for name, field := range record.Fields {
		var value any
		switch field.Kind {
		case "messages":
			var typed []*schema.Message
			err = json.Unmarshal(field.Value, &typed)
			value = typed
		case "documents":
			var typed []*schema.Document
			err = json.Unmarshal(field.Value, &typed)
			value = typed
		case "blob":
			var typed []byte
			err = json.Unmarshal(field.Value, &typed)
			value = typed
		case "integer":
			var typed int64
			err = json.Unmarshal(field.Value, &typed)
			value = typed
		case "json":
			if record.FormatVersion == 0 {
				// Legacy JSON snapshots exposed numbers as float64. Do not
				// reinterpret existing records when numeric type hints are absent.
				err = json.Unmarshal(field.Value, &value)
			} else {
				value, err = decodeJSONState(field.Value, field.FloatPaths)
			}
		default:
			return genxeino.StateSnapshot{}, fmt.Errorf("graph state: unsupported field kind %q", field.Kind)
		}
		if err != nil {
			return genxeino.StateSnapshot{}, fmt.Errorf("graph state: decode field %q: %w", name, err)
		}
		fields[name] = value
	}
	return genxeino.StateSnapshot{Version: record.Version, Fields: fields}, nil
}

type stateRecord struct {
	FormatVersion int                   `json:"format_version,omitempty"`
	Version       string                `json:"version"`
	Fields        map[string]stateField `json:"fields"`
}

type stateField struct {
	Kind       string          `json:"kind"`
	Value      json.RawMessage `json:"value"`
	FloatPaths []string        `json:"float_paths,omitempty"`
}

// CompareAndSwap persists one completed turn only if its loaded revision is current.
// The SQL scope lock also fences concurrent Workspace retirement across processes.
func (s *Store) CompareAndSwap(ctx context.Context, contextID, expected string, values map[string]any) (genxeino.StateSnapshot, error) {
	if s == nil || s.db == nil {
		return genxeino.StateSnapshot{}, errors.New("graph state: store is not configured")
	}
	version := uuid.NewString()
	data, err := encodeSnapshot(values, version)
	if err != nil {
		return genxeino.StateSnapshot{}, err
	}
	lock := ""
	if s.db.DriverName() == "postgres" || s.db.DriverName() == "pgx" {
		lock = " FOR UPDATE"
	}
	query := `WITH active_scope AS (SELECT owner_id,workspace_id FROM graph_state_scopes WHERE owner_id=? AND workspace_id=? AND retired=0` + lock + `) INSERT INTO graph_states(owner_id,workspace_id,agent_id,context_id,state_json,revision) SELECT owner_id,workspace_id,?,?,?,? FROM active_scope WHERE ?='' OR EXISTS(SELECT 1 FROM graph_states WHERE owner_id=? AND workspace_id=? AND agent_id=? AND context_id=? AND revision=?) ON CONFLICT(owner_id,workspace_id,agent_id,context_id) DO UPDATE SET state_json=excluded.state_json,revision=excluded.revision WHERE graph_states.revision=?`
	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), s.owner, s.workspace, s.agent, contextID, string(data), version, expected, s.owner, s.workspace, s.agent, contextID, expected, expected)
	if err != nil {
		return genxeino.StateSnapshot{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return genxeino.StateSnapshot{}, err
	}
	if count == 0 {
		var retired bool
		if err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT retired FROM graph_state_scopes WHERE owner_id=? AND workspace_id=?`), s.owner, s.workspace).Scan(&retired); err != nil {
			return genxeino.StateSnapshot{}, err
		}
		if retired {
			return genxeino.StateSnapshot{}, ErrRetired
		}
		return genxeino.StateSnapshot{}, ErrConflict
	}
	return decodeSnapshot(data)
}

func encodeSnapshot(values map[string]any, version string) ([]byte, error) {
	record := stateRecord{FormatVersion: 1, Version: version, Fields: make(map[string]stateField, len(values))}
	for name, value := range values {
		kind := "json"
		switch value.(type) {
		case []*schema.Message:
			kind = "messages"
		case []*schema.Document:
			kind = "documents"
		case []byte:
			kind = "blob"
		case int, int8, int16, int32, int64:
			kind = "integer"
		}
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("graph state: encode field %q: %w", name, err)
		}
		field := stateField{Kind: kind, Value: data}
		if kind == "json" {
			field.FloatPaths = jsonStateFloatPaths(value)
		}
		record.Fields[name] = field
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return data, nil
}
