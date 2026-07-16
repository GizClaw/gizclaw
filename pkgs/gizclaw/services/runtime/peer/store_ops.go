package peer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// EnsureConnectedPeer creates a default active peer record for a connected peer
// when the peer has not been registered yet. Existing records are preserved.
func (s *Server) EnsureConnectedPeer(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	if publicKey.IsZero() {
		return apitypes.Peer{}, fmt.Errorf("peer: empty public key")
	}
	existing, err := s.get(ctx, publicKey)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrPeerNotFound) {
		return apitypes.Peer{}, err
	}

	autoRegistered := true
	configuration := apitypes.Configuration{}
	if view := strings.TrimSpace(s.DefaultPeerView); view != "" {
		configuration.View = &view
	}
	created, err := s.create(ctx, apitypes.Peer{
		PublicKey:      publicKey.String(),
		Role:           apitypes.PeerRoleClient,
		Status:         apitypes.PeerRegistrationStatusActive,
		Device:         apitypes.DeviceInfo{},
		Configuration:  configuration,
		AutoRegistered: &autoRegistered,
	})
	if errors.Is(err, ErrPeerAlreadyExists) {
		return s.get(ctx, publicKey)
	}
	return created, err
}

func isAutoConnectedPeer(peer apitypes.Peer) bool {
	return peer.AutoRegistered != nil &&
		*peer.AutoRegistered &&
		peer.ApprovedAt == nil &&
		peer.Role == apitypes.PeerRoleClient &&
		peer.Status == apitypes.PeerRegistrationStatusActive
}

func (s *Server) putInfo(ctx context.Context, publicKey giznet.PublicKey, info apitypes.DeviceInfo) (apitypes.Peer, error) {
	unlock := s.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	if err := iconasset.ValidateProjection(peer.Device.Icon, info.Icon); err != nil {
		return apitypes.Peer{}, err
	}
	info.Icon = peer.Device.Icon
	peer.Device = info
	return s.put(ctx, peer)
}

// LoadPeer returns the stored peer record for a public key.
func (s *Server) LoadPeer(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	return s.get(ctx, publicKey)
}

// BootstrapEdgeNodes inserts or updates configured edge-node peers while
// preserving existing peer metadata.
func (s *Server) BootstrapEdgeNodes(ctx context.Context, publicKeys []giznet.PublicKey) error {
	for _, publicKey := range publicKeys {
		if publicKey.IsZero() {
			return fmt.Errorf("peer: empty edge-node public key")
		}
		peer, err := s.get(ctx, publicKey)
		if err != nil {
			if !errors.Is(err, ErrPeerNotFound) {
				return err
			}
			peer = apitypes.Peer{
				PublicKey:     publicKey.String(),
				Device:        apitypes.DeviceInfo{},
				Configuration: apitypes.Configuration{},
			}
		}
		peer.Role = apitypes.PeerRoleEdgeNode
		peer.Status = apitypes.PeerRegistrationStatusActive
		if _, err := s.put(ctx, peer); err != nil {
			return err
		}
	}
	return nil
}

// SavePeer stores a full peer record and returns the persisted value.
func (s *Server) SavePeer(ctx context.Context, peer apitypes.Peer) (apitypes.Peer, error) {
	return s.put(ctx, peer)
}

func (s *Server) putConfig(ctx context.Context, publicKey giznet.PublicKey, cfg apitypes.Configuration) (apitypes.Peer, error) {
	if err := validateConfiguration(cfg); err != nil {
		return apitypes.Peer{}, err
	}
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	peer.Configuration = cfg
	return s.put(ctx, peer)
}

func (s *Server) approve(ctx context.Context, publicKey giznet.PublicKey, role apitypes.PeerRole) (apitypes.Peer, error) {
	if role == apitypes.PeerRoleUnspecified || !role.Valid() {
		return apitypes.Peer{}, fmt.Errorf("peer: invalid role %q", role)
	}
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	approvedAt := time.Now()
	peer.Role = role
	peer.Status = apitypes.PeerRegistrationStatusActive
	peer.ApprovedAt = &approvedAt
	return s.put(ctx, peer)
}

func (s *Server) block(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	peer.Status = apitypes.PeerRegistrationStatusBlocked
	return s.put(ctx, peer)
}

func (s *Server) delete(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	unlock := s.IconLocks.LockOwner(publicKey.String())
	defer unlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	if peer.Device.Icon != nil && s.Assets == nil {
		return apitypes.Peer{}, errors.New("peer: asset store not configured")
	}
	if s.Assets != nil {
		for _, format := range []iconasset.Format{iconasset.FormatPixa, iconasset.FormatPNG} {
			if err := s.Assets.Delete(iconasset.ObjectName(publicKey.String(), format)); err != nil {
				return apitypes.Peer{}, errors.New("peer: failed to delete icon")
			}
		}
	}
	store, err := s.store()
	if err != nil {
		return apitypes.Peer{}, err
	}
	deletes := append([]kv.Key{peerKey(peer.PublicKey)}, indexKeys(peer)...)
	if err := store.BatchDelete(ctx, deletes); err != nil {
		return apitypes.Peer{}, fmt.Errorf("peer: delete %s: %w", peer.PublicKey, err)
	}
	return peer, nil
}

func (s *Server) get(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Peer{}, err
	}
	publicKeyText := publicKey.String()
	peer, err := s.getByPublicKeyText(ctx, store, publicKeyText)
	if err != nil {
		return apitypes.Peer{}, err
	}
	return peer, nil
}

func (s *Server) getByPublicKeyText(ctx context.Context, store kv.Store, publicKeyText string) (apitypes.Peer, error) {
	data, err := store.Get(ctx, peerKey(publicKeyText))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return apitypes.Peer{}, ErrPeerNotFound
		}
		return apitypes.Peer{}, fmt.Errorf("peer: get %s: %w", publicKeyText, err)
	}
	peer, err := decodePeer(data)
	if err != nil {
		return apitypes.Peer{}, fmt.Errorf("peer: decode %s: %w", publicKeyText, err)
	}
	return peer, nil
}

func decodePeer(data []byte) (apitypes.Peer, error) {
	var peer apitypes.Peer
	if err := json.Unmarshal(data, &peer); err != nil {
		return apitypes.Peer{}, err
	}
	return peer, nil
}

func (s *Server) exists(ctx context.Context, publicKey giznet.PublicKey) (bool, error) {
	_, err := s.get(ctx, publicKey)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrPeerNotFound) {
		return false, nil
	}
	return false, err
}

func (s *Server) create(ctx context.Context, peer apitypes.Peer) (apitypes.Peer, error) {
	if err := validatePeer(peer); err != nil {
		return apitypes.Peer{}, err
	}
	publicKey, err := publicKeyFromText(peer.PublicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.get(ctx, publicKey); err == nil {
		return apitypes.Peer{}, ErrPeerAlreadyExists
	} else if !errors.Is(err, ErrPeerNotFound) {
		return apitypes.Peer{}, err
	}

	now := time.Now()
	peer.CreatedAt = now
	peer.UpdatedAt = now
	if err := s.writePeerLocked(ctx, peer, nil); err != nil {
		return apitypes.Peer{}, err
	}
	return s.get(ctx, publicKey)
}

func (s *Server) put(ctx context.Context, peer apitypes.Peer) (apitypes.Peer, error) {
	if err := validatePeer(peer); err != nil {
		return apitypes.Peer{}, err
	}
	publicKey, err := publicKeyFromText(peer.PublicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old, err := s.get(ctx, publicKey)
	if err != nil && !errors.Is(err, ErrPeerNotFound) {
		return apitypes.Peer{}, err
	}
	if peer.CreatedAt.IsZero() {
		if errors.Is(err, ErrPeerNotFound) {
			peer.CreatedAt = time.Now()
		} else {
			peer.CreatedAt = old.CreatedAt
		}
	}
	peer.UpdatedAt = time.Now()
	if err := s.writePeerLocked(ctx, peer, optionalPeer(old, err)); err != nil {
		return apitypes.Peer{}, err
	}
	return s.get(ctx, publicKey)
}

func (s *Server) list(ctx context.Context) ([]apitypes.Peer, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	items := make([]apitypes.Peer, 0)
	for entry, err := range store.List(ctx, peersPrefix()) {
		if err != nil {
			return nil, fmt.Errorf("peer: list: %w", err)
		}
		var peer apitypes.Peer
		if err := json.Unmarshal(entry.Value, &peer); err != nil {
			return nil, fmt.Errorf("peer: decode list %s: %w", entry.Key.String(), err)
		}
		items = append(items, peer)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].PublicKey < items[j].PublicKey
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s *Server) listPage(ctx context.Context, cursor string, limit int) ([]apitypes.Peer, bool, *string, error) {
	items, err := s.list(ctx)
	if err != nil {
		return nil, false, nil, err
	}
	start := 0
	if cursor != "" {
		start = len(items)
		for index, peer := range items {
			if peer.PublicKey == cursor {
				start = index + 1
				break
			}
		}
	}
	if start >= len(items) {
		return nil, false, nil, nil
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	page := items[start:end]
	if end >= len(items) {
		return page, false, nil, nil
	}
	nextCursor := page[len(page)-1].PublicKey
	return page, true, &nextCursor, nil
}

func (s *Server) resolveBySN(ctx context.Context, sn string) (giznet.PublicKey, error) {
	return s.resolveSingle(ctx, snKey(sn), ErrPeerNotFound)
}

func (s *Server) resolveByIMEI(ctx context.Context, tac, serial string) (giznet.PublicKey, error) {
	return s.resolveSingle(ctx, imeiKey(tac, serial), ErrPeerNotFound)
}

func (s *Server) writePeerLocked(ctx context.Context, peer apitypes.Peer, previous *apitypes.Peer) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	data, err := json.Marshal(peer)
	if err != nil {
		return fmt.Errorf("peer: encode %s: %w", peer.PublicKey, err)
	}

	var deletes []kv.Key
	if previous != nil {
		if previous.PublicKey != peer.PublicKey {
			deletes = append(deletes, peerKey(previous.PublicKey))
		}
		deletes = append(deletes, indexKeys(*previous)...)
	}

	entries := []kv.Entry{{Key: peerKey(peer.PublicKey), Value: data}}
	entries = append(entries, indexEntries(peer)...)

	if len(deletes) > 0 {
		if err := store.BatchDelete(ctx, deletes); err != nil {
			return fmt.Errorf("peer: delete stale indexes %s: %w", peer.PublicKey, err)
		}
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return fmt.Errorf("peer: write %s: %w", peer.PublicKey, err)
	}
	return nil
}

func (s *Server) resolveSingle(ctx context.Context, key kv.Key, notFound error) (giznet.PublicKey, error) {
	store, err := s.store()
	if err != nil {
		return giznet.PublicKey{}, err
	}
	data, err := store.Get(ctx, key)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return giznet.PublicKey{}, notFound
		}
		return giznet.PublicKey{}, err
	}
	publicKey, err := publicKeyFromText(string(data))
	if err != nil {
		return giznet.PublicKey{}, err
	}
	return publicKey, nil
}

func (s *Server) store() (kv.Store, error) {
	if s.Store == nil {
		return nil, errors.New("peer: store not configured")
	}
	return s.Store, nil
}

func (s *Server) peerRuntime(ctx context.Context, publicKey giznet.PublicKey) apitypes.Runtime {
	if s.PeerManager == nil {
		return apitypes.Runtime{}
	}
	if publicKey.IsZero() {
		return apitypes.Runtime{}
	}
	return s.PeerManager.PeerRuntime(ctx, publicKey)
}

func optionalPeer(peer apitypes.Peer, err error) *apitypes.Peer {
	if err != nil {
		return nil
	}
	cp := peer
	return &cp
}
