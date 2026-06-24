package peerrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

var (
	ErrNilServer             = errors.New("peerrun: nil server")
	ErrNilStore              = errors.New("peerrun: nil store")
	ErrInvalidPublicKey      = errors.New("peerrun: invalid public key")
	ErrRunAgentNotConfigured = errors.New("peerrun: run agent not configured")
	ErrRunAgentChanged       = errors.New("peerrun: run agent selection changed")
)

type Server struct {
	Store kv.Store
}

func (s *Server) GetStatus(ctx context.Context, publicKey giznet.PublicKey) (apitypes.PeerStatus, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.PeerStatus{}, err
	}
	key, err := statusKey(publicKey)
	if err != nil {
		return apitypes.PeerStatus{}, err
	}
	data, err := store.Get(ctx, key)
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.PeerStatus{}, nil
	}
	if err != nil {
		return apitypes.PeerStatus{}, fmt.Errorf("peerrun: get status: %w", err)
	}
	var status apitypes.PeerStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return apitypes.PeerStatus{}, fmt.Errorf("peerrun: decode status: %w", err)
	}
	return status, nil
}

func (s *Server) PutStatus(ctx context.Context, publicKey giznet.PublicKey, status apitypes.PeerStatus) (apitypes.PeerStatus, error) {
	if err := validateStatus(status); err != nil {
		return apitypes.PeerStatus{}, err
	}
	store, err := s.store()
	if err != nil {
		return apitypes.PeerStatus{}, err
	}
	key, err := statusKey(publicKey)
	if err != nil {
		return apitypes.PeerStatus{}, err
	}
	data, err := json.Marshal(status)
	if err != nil {
		return apitypes.PeerStatus{}, fmt.Errorf("peerrun: encode status: %w", err)
	}
	if err := store.Set(ctx, key, data); err != nil {
		return apitypes.PeerStatus{}, fmt.Errorf("peerrun: put status: %w", err)
	}
	return status, nil
}

func (s *Server) GetRunAgent(ctx context.Context, publicKey giznet.PublicKey) (apitypes.PeerRunAgent, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	key, err := runAgentKey(publicKey)
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	data, err := store.Get(ctx, key)
	if errors.Is(err, kv.ErrNotFound) {
		return apitypes.PeerRunAgent{}, nil
	}
	if err != nil {
		return apitypes.PeerRunAgent{}, fmt.Errorf("peerrun: get run agent: %w", err)
	}
	var agent apitypes.PeerRunAgent
	if err := json.Unmarshal(data, &agent); err != nil {
		return apitypes.PeerRunAgent{}, fmt.Errorf("peerrun: decode run agent: %w", err)
	}
	return agent, nil
}

func (s *Server) SetRunAgent(ctx context.Context, publicKey giznet.PublicKey, selection apitypes.AgentSelection) (apitypes.PeerRunAgent, error) {
	if err := validateAgentSelection(selection); err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	agent, err := s.GetRunAgent(ctx, publicKey)
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	agent.Pending = &selection
	if err := validateRunAgent(agent); err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	store, err := s.store()
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	key, err := runAgentKey(publicKey)
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	data, err := json.Marshal(agent)
	if err != nil {
		return apitypes.PeerRunAgent{}, fmt.Errorf("peerrun: encode run agent: %w", err)
	}
	if err := store.Set(ctx, key, data); err != nil {
		return apitypes.PeerRunAgent{}, fmt.Errorf("peerrun: set run agent: %w", err)
	}
	return agent, nil
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
	agent, err := s.GetRunAgent(ctx, publicKey)
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	switch {
	case agent.Pending != nil:
		if !sameAgentSelection(*agent.Pending, selection) {
			return apitypes.PeerRunAgent{}, ErrRunAgentChanged
		}
		agent.Pending = nil
	case agent.Active != nil:
		if !sameAgentSelection(*agent.Active, selection) {
			return apitypes.PeerRunAgent{}, ErrRunAgentChanged
		}
	default:
		return apitypes.PeerRunAgent{}, ErrRunAgentNotConfigured
	}
	agent.Active = &selection
	if err := validateRunAgent(agent); err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	store, err := s.store()
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	key, err := runAgentKey(publicKey)
	if err != nil {
		return apitypes.PeerRunAgent{}, err
	}
	data, err := json.Marshal(agent)
	if err != nil {
		return apitypes.PeerRunAgent{}, fmt.Errorf("peerrun: encode run agent: %w", err)
	}
	if err := store.Set(ctx, key, data); err != nil {
		return apitypes.PeerRunAgent{}, fmt.Errorf("peerrun: activate run agent: %w", err)
	}
	return agent, nil
}

func (s *Server) store() (kv.Store, error) {
	if s == nil {
		return nil, ErrNilServer
	}
	if s.Store == nil {
		return nil, ErrNilStore
	}
	return s.Store, nil
}

func statusKey(publicKey giznet.PublicKey) (kv.Key, error) {
	return key(publicKey, "status")
}

func runAgentKey(publicKey giznet.PublicKey) (kv.Key, error) {
	return key(publicKey, "run-agent")
}

func key(publicKey giznet.PublicKey, name string) (kv.Key, error) {
	if publicKey.IsZero() {
		return nil, ErrInvalidPublicKey
	}
	return kv.Key{"by-peer", publicKey.String(), name}, nil
}

func validateStatus(status apitypes.PeerStatus) error {
	if status.Volume != nil && (*status.Volume < 0 || *status.Volume > 100) {
		return fmt.Errorf("peerrun: volume must be between 0 and 100")
	}
	if status.BatteryPercent != nil && (*status.BatteryPercent < 0 || *status.BatteryPercent > 100) {
		return fmt.Errorf("peerrun: battery_percent must be between 0 and 100")
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
