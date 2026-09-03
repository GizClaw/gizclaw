package sfu

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

const (
	// DefaultRecheckInterval is used when Config.RecheckInterval is zero.
	DefaultRecheckInterval = 5 * time.Second
	// DefaultReconnectTimeout is used when Config.ReconnectTimeout is zero.
	DefaultReconnectTimeout = 30 * time.Second
	// DefaultTalkHangover is used when Config.TalkHangover is zero.
	DefaultTalkHangover = 500 * time.Millisecond
	// DefaultFloorIdle is used when Config.FloorIdle is zero.
	DefaultFloorIdle = 300 * time.Millisecond
)

// Factory builds the shared per-Workspace SFU Agent. Config carries the
// Server-held connector credentials; Bindings is the authoritative Social
// membership check every attachment must pass.
type Factory struct {
	Config   Config
	Bindings BindingResolver
	Logger   *slog.Logger

	// connector overrides the LiveKit connector in tests.
	connector roomConnector
}

// NewAgent returns the Agent for one SFU Workspace. The Agent is shared by
// every Peer attached to that Workspace on this Server; each Transform call
// owns one participant connection.
func (f Factory) NewAgent(_ context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if f.Bindings == nil {
		return nil, errors.New("sfu: binding resolver is required")
	}
	workspaceID := strings.TrimSpace(spec.Workspace.Id)
	if workspaceID == "" {
		return nil, errors.New("sfu: workspace id is required")
	}
	config := f.Config
	if config.RecheckInterval <= 0 {
		config.RecheckInterval = DefaultRecheckInterval
	}
	if config.ReconnectTimeout <= 0 {
		config.ReconnectTimeout = DefaultReconnectTimeout
	}
	if config.TalkHangover <= 0 {
		config.TalkHangover = DefaultTalkHangover
	}
	if config.FloorIdle <= 0 {
		config.FloorIdle = DefaultFloorIdle
	}
	connector := f.connector
	if connector == nil {
		if strings.TrimSpace(config.URL) == "" || config.APIKey == "" || config.APISecret == "" {
			return nil, errors.New("sfu: services.sfu is not configured on this Server")
		}
		connector = livekitConnector{apiKey: config.APIKey, apiSecret: config.APISecret}
	}
	logger := f.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Agent{
		workspaceID:   workspaceID,
		workspaceName: spec.Workspace.Name,
		config:        config,
		bindings:      f.Bindings,
		connector:     connector,
		logger:        logger.With("workspace", workspaceID),
		sessions:      make(map[string]*session),
	}, nil
}

// Agent is the shared SFU Workspace runtime. Every Transform call attaches
// the calling Peer as one SFU participant for the lifetime of its context.
type Agent struct {
	workspaceID   string
	workspaceName string
	config        Config
	bindings      BindingResolver
	connector     roomConnector
	logger        *slog.Logger

	mu       sync.Mutex
	sessions map[string]*session
}

var _ agenthost.Agent = (*Agent)(nil)

// Transform attaches the Peer identified by the context to the Room. The
// returned stream carries the floor holder's raw Opus packets as passthrough
// audio (see agenthost.OpusPassthroughMIME) and closes when the context is
// cancelled, the binding is revoked, the participant is replaced by a newer
// connection or reconnection gives up.
func (a *Agent) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	if input == nil {
		return nil, errors.New("sfu: input stream is required")
	}
	peer := strings.TrimSpace(gizlog.PeerPublicKey(ctx))
	if peer == "" {
		return nil, errors.New("sfu: peer identity is required")
	}
	binding, err := a.bindings.ResolveSFUWorkspaceBinding(ctx, a.workspaceID, peer)
	if err != nil {
		return nil, fmt.Errorf("sfu: resolve workspace %s binding: %w", a.workspaceID, err)
	}
	if err := binding.SFU.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotBound, err)
	}
	if err := a.retire(ctx, peer); err != nil {
		return nil, err
	}
	s := newSession(ctx, a, peer, binding)
	if err := s.connect(ctx); err != nil {
		s.cancel(context.Canceled)
		return nil, err
	}
	a.mu.Lock()
	if previous := a.sessions[peer]; previous != nil {
		// A concurrent Transform for the same peer won the race; keep the
		// newest participant and drop this one.
		a.mu.Unlock()
		s.finish(nil)
		return nil, fmt.Errorf("sfu: peer %s attached concurrently", peer)
	}
	a.sessions[peer] = s
	a.mu.Unlock()
	s.start(input)
	return s.out, nil
}

// retire cancels any previous attachment for the peer and waits for its
// participant to leave so the Room never sees two of them from this Server.
func (a *Agent) retire(ctx context.Context, peer string) error {
	a.mu.Lock()
	previous := a.sessions[peer]
	a.mu.Unlock()
	if previous == nil {
		return nil
	}
	previous.cancel(context.Canceled)
	select {
	case <-previous.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("sfu: replace previous attachment: %w", ctx.Err())
	}
}

func (a *Agent) removeSession(peer string, s *session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sessions[peer] == s {
		delete(a.sessions, peer)
	}
}

// SessionStatus reports the attachment state of one Peer. It reports false
// when the Peer is not attached.
func (a *Agent) SessionStatus(peer string) (SessionStatus, bool) {
	a.mu.Lock()
	s := a.sessions[strings.TrimSpace(peer)]
	a.mu.Unlock()
	if s == nil {
		return SessionStatus{}, false
	}
	return s.status(), true
}

// Status maps the calling Peer's attachment onto the Workspace state. When
// the context carries no Peer identity the shared Agent reports running.
func (a *Agent) Status(ctx context.Context) (apitypes.PeerRunWorkspaceState, error) {
	available := false
	agentType := Type
	state := apitypes.PeerRunWorkspaceState{
		WorkspaceName:        a.workspaceName,
		AgentType:            &agentType,
		RuntimeState:         apitypes.PeerRunStatusStateRunning,
		HistoryAvailable:     &available,
		MemoryStatsAvailable: &available,
		RecallAvailable:      &available,
	}
	status, attached := a.SessionStatus(gizlog.PeerPublicKey(ctx))
	if !attached {
		return state, nil
	}
	var message string
	switch status.State {
	case StateReconnecting:
		state.RuntimeState = apitypes.PeerRunStatusStateStarting
		message = "sfu: reconnecting to room"
	case StateClosed:
		if status.Err != nil {
			state.RuntimeState = apitypes.PeerRunStatusStateError
			message = status.Err.Error()
		} else {
			state.RuntimeState = apitypes.PeerRunStatusStateStopped
		}
	}
	if message != "" {
		state.Message = &message
	}
	return state, nil
}

// ListHistory always reports no history: SFU Workspaces own none.
func (a *Agent) ListHistory(context.Context, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return apitypes.PeerRunHistoryListResponse{Available: false, Items: []apitypes.PeerRunHistoryEntry{}}, nil
}

// PlayHistory always rejects playback: SFU Workspaces own no history.
func (a *Agent) PlayHistory(_ context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryName: req.HistoryName, State: "unsupported"}, nil
}

// MemoryStats always reports no memory.
func (a *Agent) MemoryStats(context.Context, apitypes.PeerRunMemoryStatsRequest) (apitypes.PeerRunMemoryStatsResponse, error) {
	return apitypes.PeerRunMemoryStatsResponse{Available: false}, nil
}

// Recall always reports no memory.
func (a *Agent) Recall(context.Context, apitypes.PeerRunRecallRequest) (apitypes.PeerRunRecallResponse, error) {
	return apitypes.PeerRunRecallResponse{Available: false, Hits: []apitypes.PeerRunRecallHit{}}, nil
}
