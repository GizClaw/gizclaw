package peerrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
)

var (
	ErrNilServer             = errors.New("peerrun: nil server")
	ErrNilStore              = errors.New("peerrun: nil database")
	ErrInvalidPublicKey      = errors.New("peerrun: invalid public key")
	ErrInvalidStatus         = errors.New("peerrun: invalid status")
	ErrRunAgentNotConfigured = errors.New("peerrun: run agent not configured")
	ErrRunAgentChanged       = errors.New("peerrun: run agent selection changed")
)

// Server owns this Server's durable peer runtime rows and borrows its SQL pool.
// Initialize must run before serving requests. Server never closes the pool.
type Server struct{ DB *sqlx.DB }

// Initialize creates the runtime schema at startup.
func (s *Server) Initialize(ctx context.Context) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS peer_runs (
 public_key TEXT PRIMARY KEY,
 registered_at TEXT,
 status_json TEXT,
 ota_json TEXT,
 pending_workspace TEXT,
 active_workspace TEXT,
 debug_mode TEXT NOT NULL DEFAULT 'off' CHECK (debug_mode IN ('off','readonly','fullcontrol')),
 CHECK (pending_workspace IS NULL OR length(pending_workspace) > 0),
 CHECK (active_workspace IS NULL OR length(active_workspace) > 0)
 )`)
	if err != nil {
		return fmt.Errorf("peerrun: initialize schema: %w", err)
	}
	_, err = db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS peer_runs_directory ON peer_runs(registered_at,public_key) WHERE registered_at IS NOT NULL`)
	return err
}

func (s *Server) database() (*sqlx.DB, error) {
	if s == nil {
		return nil, ErrNilServer
	}
	if s.DB == nil {
		return nil, ErrNilStore
	}
	switch s.DB.DriverName() {
	case "sqlite", "postgres":
	default:
		return nil, fmt.Errorf("peerrun: unsupported SQL driver %q", s.DB.DriverName())
	}
	return s.DB, nil
}

func (s *Server) GetStatus(ctx context.Context, publicKey giznet.PublicKey) (apitypes.PeerStatus, error) {
	db, err := s.database()
	if err != nil {
		return apitypes.PeerStatus{}, err
	}
	if publicKey.IsZero() {
		return apitypes.PeerStatus{}, ErrInvalidPublicKey
	}
	var data, ota sql.NullString
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT status_json, ota_json FROM peer_runs WHERE public_key = ?`), publicKey.String()).Scan(&data, &ota)
	if errors.Is(err, sql.ErrNoRows) {
		return apitypes.PeerStatus{}, nil
	}
	if err != nil {
		return apitypes.PeerStatus{}, fmt.Errorf("peerrun: get status: %w", err)
	}
	var status apitypes.PeerStatus
	if data.Valid {
		if err = json.Unmarshal([]byte(data.String), &status); err != nil {
			return status, fmt.Errorf("peerrun: decode status: %w", err)
		}
	}
	if ota.Valid {
		if err = json.Unmarshal([]byte(ota.String), &status.Ota); err != nil {
			return status, fmt.Errorf("peerrun: decode ota: %w", err)
		}
	}
	if status.Ota != nil && (status.ReportedAt == nil || status.Ota.ObservedAt.After(*status.ReportedAt)) {
		status.ReportedAt = new(status.Ota.ObservedAt)
	}
	return status, nil
}

func (s *Server) PutStatus(ctx context.Context, publicKey giznet.PublicKey, status apitypes.PeerStatus) (apitypes.PeerStatus, error) {
	if err := validateStatus(status); err != nil {
		return apitypes.PeerStatus{}, err
	}
	db, err := s.database()
	if err != nil {
		return apitypes.PeerStatus{}, err
	}
	if publicKey.IsZero() {
		return apitypes.PeerStatus{}, ErrInvalidPublicKey
	}
	// Independent columns prevent status writes from replacing newer OTA telemetry.
	status.Ota = nil
	data, err := json.Marshal(status)
	if err != nil {
		return status, err
	}
	_, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO peer_runs(public_key,status_json) VALUES (?,?) ON CONFLICT(public_key) DO UPDATE SET status_json=excluded.status_json`), publicKey.String(), string(data))
	if err != nil {
		return status, fmt.Errorf("peerrun: put status: %w", err)
	}
	return s.GetStatus(ctx, publicKey)
}

func scanRunAgent(row interface{ Scan(...any) error }) (apitypes.PeerRunAgent, error) {
	var pending, active sql.NullString
	if err := row.Scan(&pending, &active); err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	var agent apitypes.PeerRunAgent
	if pending.Valid {
		agent.Pending = &apitypes.AgentSelection{WorkspaceName: pending.String}
	}
	if active.Valid {
		agent.Active = &apitypes.AgentSelection{WorkspaceName: active.String}
	}
	return agent, nil
}

func (s *Server) GetRunAgent(ctx context.Context, publicKey giznet.PublicKey) (apitypes.PeerRunAgent, error) {
	db, err := s.database()
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	if publicKey.IsZero() {
		return apitypes.PeerRunAgent{}, ErrInvalidPublicKey
	}
	agent, err := scanRunAgent(db.QueryRowContext(ctx, db.Rebind(`SELECT pending_workspace,active_workspace FROM peer_runs WHERE public_key=?`), publicKey.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return apitypes.PeerRunAgent{}, nil
	}
	return agent, err
}

func (s *Server) SetRunAgent(ctx context.Context, publicKey giznet.PublicKey, selection apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	if err := validateAgentSelection(selection); err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	db, err := s.database()
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	if publicKey.IsZero() {
		return apitypes.PeerRunAgent{}, ErrInvalidPublicKey
	}
	return scanRunAgent(db.QueryRowContext(ctx, db.Rebind(`INSERT INTO peer_runs(public_key,pending_workspace) VALUES (?,?) ON CONFLICT(public_key) DO UPDATE SET pending_workspace=excluded.pending_workspace RETURNING pending_workspace,active_workspace`), publicKey.String(), selection.WorkspaceName))
}

func (s *Server) ResolveRunAgent(ctx context.Context, publicKey giznet.PublicKey) (apitypes.AgentSelection, error) {
	agent, err := s.GetRunAgent(ctx, publicKey)
	if err != nil {
		return apitypes.AgentSelection{}, err
	}
	if agent.Pending != nil {
		return *agent.Pending, nil
	}
	if agent.Active != nil {
		return *agent.Active, nil
	}
	return apitypes.AgentSelection{}, ErrRunAgentNotConfigured
}

func (s *Server) ActivateRunAgent(ctx context.Context, publicKey giznet.PublicKey, selection apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	if err := validateAgentSelection(selection); err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	db, err := s.database()
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	if publicKey.IsZero() {
		return apitypes.PeerRunAgent{}, ErrInvalidPublicKey
	}
	// Compare and activate in one statement: an older activation cannot erase a
	// newly selected pending workspace.
	agent, err := scanRunAgent(db.QueryRowContext(ctx, db.Rebind(`UPDATE peer_runs SET pending_workspace=NULL,active_workspace=? WHERE public_key=? AND (pending_workspace=? OR (pending_workspace IS NULL AND active_workspace=?)) RETURNING pending_workspace,active_workspace`), selection.WorkspaceName, publicKey.String(), selection.WorkspaceName, selection.WorkspaceName))
	if !errors.Is(err, sql.ErrNoRows) {
		return agent, err
	}
	current, err := s.GetRunAgent(ctx, publicKey)
	if err != nil {
		return current, err
	}
	if current.Pending == nil && current.Active == nil {
		return current, ErrRunAgentNotConfigured
	}
	return apitypes.PeerRunAgent{}, ErrRunAgentChanged
}

func validateStatus(status apitypes.PeerStatus) error {
	if status.Volume != nil && (*status.Volume < 0 || *status.Volume > 100) {
		return fmt.Errorf("%w: volume must be between 0 and 100", ErrInvalidStatus)
	}
	if status.BatteryPercent != nil && (*status.BatteryPercent < 0 || *status.BatteryPercent > 100) {
		return fmt.Errorf("%w: battery_percent must be between 0 and 100", ErrInvalidStatus)
	}
	return nil
}

func validateRunAgent(agent apitypes.PeerRunAgent) error {
	if agent.Active != nil {
		if err := validateAgentSelection(*agent.Active); err != nil {
			return err
		}
	}
	if agent.Pending != nil {
		if err := validateAgentSelection(*agent.Pending); err != nil {
			return err
		}
	}
	return nil
}

func validateAgentSelection(selection apitypes.AgentSelection) error {
	if strings.TrimSpace(selection.WorkspaceName) == "" {
		return fmt.Errorf("peerrun: workspace_name is required")
	}
	if selection.WorkspaceName != strings.TrimSpace(selection.WorkspaceName) {
		return fmt.Errorf("peerrun: workspace_name must not have surrounding whitespace")
	}
	return nil
}

func sameAgentSelection(a, b apitypes.AgentSelection) bool {
	return a.WorkspaceName == b.WorkspaceName
}
