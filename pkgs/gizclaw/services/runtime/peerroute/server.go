package peerroute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	ErrAssignmentNotFound = errors.New("peerroute: assignment not found")
	ErrInvalidPublicKey   = errors.New("peerroute: invalid public key")
	ErrPeerStoreNil       = errors.New("peerroute: peer store not configured")
	ErrStoreNil           = errors.New("peerroute: store not configured")
	ErrMissingRoute       = errors.New("peerroute: server route not configured")
	ErrVersionConflict    = errors.New("peerroute: assignment version conflict")
	ErrPeerInactive       = errors.New("peerroute: peer is not active")
	ErrPeerNotAssignable  = errors.New("peerroute: peer role is not assignable")
)

type PeerStore interface {
	LoadPeer(context.Context, giznet.PublicKey) (apitypes.Peer, error)
}

type Server struct {
	Store           kv.Store
	Peers           PeerStore
	ServerPublicKey giznet.PublicKey
	ServerEndpoint  string
}

func (s *Server) Lookup(ctx context.Context, publicKey giznet.PublicKey) (apitypes.PeerAssignment, error) {
	if publicKey.IsZero() {
		return apitypes.PeerAssignment{}, ErrInvalidPublicKey
	}
	return s.get(ctx, publicKey)
}

func (s *Server) Resolve(ctx context.Context, target giznet.PublicKey) (apitypes.PeerAssignment, error) {
	if target.IsZero() {
		return apitypes.PeerAssignment{}, ErrInvalidPublicKey
	}
	if err := s.validateRoute(); err != nil {
		return apitypes.PeerAssignment{}, err
	}
	if s.Peers == nil {
		return apitypes.PeerAssignment{}, ErrPeerStoreNil
	}
	peer, err := s.Peers.LoadPeer(ctx, target)
	if err != nil {
		return apitypes.PeerAssignment{}, err
	}
	if err := validateAssignablePeer(peer); err != nil {
		return apitypes.PeerAssignment{}, err
	}

	return s.get(ctx, target)
}

func (s *Server) Assign(ctx context.Context, publicKey giznet.PublicKey, expectedVersion *int64) (apitypes.PeerAssignment, error) {
	if publicKey.IsZero() {
		return apitypes.PeerAssignment{}, ErrInvalidPublicKey
	}
	if err := s.validateRoute(); err != nil {
		return apitypes.PeerAssignment{}, err
	}
	if s.Peers == nil {
		return apitypes.PeerAssignment{}, ErrPeerStoreNil
	}
	peer, err := s.Peers.LoadPeer(ctx, publicKey)
	if err != nil {
		return apitypes.PeerAssignment{}, err
	}
	if err := validateAssignablePeer(peer); err != nil {
		return apitypes.PeerAssignment{}, err
	}

	assignment := apitypes.PeerAssignment{
		PeerPublicKey:   publicKey.String(),
		ServerPublicKey: s.ServerPublicKey.String(),
		ServerEndpoint:  strings.TrimSpace(s.ServerEndpoint),
		Role:            peer.Role,
		Version:         1,
		UpdatedAt:       time.Now(),
	}
	encoded, err := encodeAssignment(assignment)
	if err != nil {
		return apitypes.PeerAssignment{}, err
	}
	store, err := s.store()
	if err != nil {
		return apitypes.PeerAssignment{}, err
	}
	var existing []byte
	if expectedVersion != nil {
		existing, err = store.Get(ctx, assignmentKey(publicKey.String()))
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.PeerAssignment{}, ErrVersionConflict
		}
		if err != nil {
			return apitypes.PeerAssignment{}, fmt.Errorf("peerroute: get %s: %w", publicKey.String(), err)
		}
	} else {
		var created bool
		existing, created, err = kv.CreateIfAbsent(ctx, store, kv.Entry{Key: assignmentKey(publicKey.String()), Value: encoded}, nil)
		if err != nil {
			return apitypes.PeerAssignment{}, fmt.Errorf("peerroute: claim %s: %w", publicKey.String(), err)
		}
		if created {
			return assignment, nil
		}
	}
	for {
		current, decodeErr := decodeAssignment(publicKey, existing)
		if decodeErr != nil {
			return apitypes.PeerAssignment{}, decodeErr
		}
		if current.ServerPublicKey != s.ServerPublicKey.String() {
			return apitypes.PeerAssignment{}, ErrVersionConflict
		}
		if expectedVersion != nil && current.Version != *expectedVersion {
			return apitypes.PeerAssignment{}, ErrVersionConflict
		}
		if s.assignmentCurrent(current, peer) {
			return current, nil
		}
		next := current
		next.ServerEndpoint = strings.TrimSpace(s.ServerEndpoint)
		next.Role = peer.Role
		next.Version++
		next.UpdatedAt = time.Now()
		nextEncoded, encodeErr := encodeAssignment(next)
		if encodeErr != nil {
			return apitypes.PeerAssignment{}, encodeErr
		}
		matched, mutateErr := kv.CompareAndMutate(ctx, store, assignmentKey(publicKey.String()), existing, []kv.Entry{{Key: assignmentKey(publicKey.String()), Value: nextEncoded}}, nil)
		if mutateErr != nil {
			return apitypes.PeerAssignment{}, fmt.Errorf("peerroute: refresh %s: %w", publicKey.String(), mutateErr)
		}
		if matched {
			return next, nil
		}
		existing, err = store.Get(ctx, assignmentKey(publicKey.String()))
		if err != nil {
			if errors.Is(err, kv.ErrNotFound) {
				return apitypes.PeerAssignment{}, ErrAssignmentNotFound
			}
			return apitypes.PeerAssignment{}, fmt.Errorf("peerroute: reload %s: %w", publicKey.String(), err)
		}
	}
}

func validateAssignablePeer(peer apitypes.Peer) error {
	if peer.Status != apitypes.PeerRegistrationStatusActive {
		return ErrPeerInactive
	}
	if peer.Role != apitypes.PeerRoleClient {
		return ErrPeerNotAssignable
	}
	return nil
}

func (s *Server) assignmentCurrent(assignment apitypes.PeerAssignment, peer apitypes.Peer) bool {
	return assignment.ServerPublicKey == s.ServerPublicKey.String() &&
		assignment.ServerEndpoint == strings.TrimSpace(s.ServerEndpoint) &&
		assignment.Role == peer.Role
}

func (s *Server) validateRoute() error {
	if s == nil || s.ServerPublicKey.IsZero() || strings.TrimSpace(s.ServerEndpoint) == "" {
		return ErrMissingRoute
	}
	return nil
}

func (s *Server) get(ctx context.Context, publicKey giznet.PublicKey) (apitypes.PeerAssignment, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.PeerAssignment{}, err
	}
	data, err := store.Get(ctx, assignmentKey(publicKey.String()))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.PeerAssignment{}, ErrAssignmentNotFound
		}
		return apitypes.PeerAssignment{}, fmt.Errorf("peerroute: get %s: %w", publicKey.String(), err)
	}
	return decodeAssignment(publicKey, data)
}

func decodeAssignment(publicKey giznet.PublicKey, data []byte) (apitypes.PeerAssignment, error) {
	var assignment apitypes.PeerAssignment
	if err := json.Unmarshal(data, &assignment); err != nil {
		return apitypes.PeerAssignment{}, fmt.Errorf("peerroute: decode %s: %w", publicKey.String(), err)
	}
	return assignment, nil
}

func encodeAssignment(assignment apitypes.PeerAssignment) ([]byte, error) {
	data, err := json.Marshal(assignment)
	if err != nil {
		return nil, fmt.Errorf("peerroute: encode %s: %w", assignment.PeerPublicKey, err)
	}
	return data, nil
}

func (s *Server) put(ctx context.Context, assignment apitypes.PeerAssignment) error {
	publicKey, err := ParsePublicKey(assignment.PeerPublicKey)
	if err != nil {
		return err
	}
	if assignment.ServerPublicKey == "" || assignment.ServerEndpoint == "" || assignment.Version <= 0 || assignment.UpdatedAt.IsZero() || assignment.Role != apitypes.PeerRoleClient {
		return fmt.Errorf("peerroute: invalid assignment")
	}
	data, err := encodeAssignment(assignment)
	if err != nil {
		return err
	}
	store, err := s.store()
	if err != nil {
		return err
	}
	return store.Set(ctx, assignmentKey(publicKey.String()), data)
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, ErrStoreNil
	}
	return s.Store, nil
}

func ParsePublicKey(value string) (giznet.PublicKey, error) {
	var key giznet.PublicKey
	if err := key.UnmarshalText([]byte(strings.TrimSpace(value))); err != nil {
		return giznet.PublicKey{}, fmt.Errorf("%w: %v", ErrInvalidPublicKey, err)
	}
	if key.IsZero() {
		return giznet.PublicKey{}, ErrInvalidPublicKey
	}
	return key, nil
}

func assignmentKey(publicKey string) kv.Key {
	return kv.Key{"by-peer", publicKey}
}
