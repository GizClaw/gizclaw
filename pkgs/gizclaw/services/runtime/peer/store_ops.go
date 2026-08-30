package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const peerTombstoneVersion = 1

type tombstone struct {
	Version int    `json:"version"`
	State   string `json:"state"`
}

var encodedPeerTombstone = []byte(`{"version":1,"state":"deleted"}`)

func isPeerTombstone(data []byte) bool {
	return bytes.Equal(data, encodedPeerTombstone)
}

// BindFirmware persists the Server-assigned Firmware ID for a Peer.
// Firmware channel selection remains device-owned and is not stored here.
func (s *Server) BindFirmware(ctx context.Context, publicKey giznet.PublicKey, firmwareID string) (apitypes.Peer, error) {
	if publicKey.IsZero() {
		return apitypes.Peer{}, fmt.Errorf("peer: empty public key")
	}
	if err := customid.ValidateResourceID(firmwareID); err != nil {
		return apitypes.Peer{}, fmt.Errorf("peer: invalid firmware id: %w", err)
	}
	unlock := s.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	peer.FirmwareId = &firmwareID
	return s.putRecord(ctx, peer)
}

// EnsureConnectedPeer creates a default active peer record for a connected peer
// when the peer has not been registered yet. Existing records are preserved.
func (s *Server) EnsureConnectedPeer(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	return s.EnsureConnectedPeerGuarded(ctx, publicKey, nil)
}

// EnsureConnectedPeerGuarded runs guard while holding the per-Peer record lock
// and creates the connected Peer only when the guard still accepts it.
func (s *Server) EnsureConnectedPeerGuarded(ctx context.Context, publicKey giznet.PublicKey, guard func() error) (apitypes.Peer, error) {
	if publicKey.IsZero() {
		return apitypes.Peer{}, fmt.Errorf("peer: empty public key")
	}
	recordUnlock := s.IconLocks.LockRecord(publicKey.String())
	defer recordUnlock()
	if guard != nil {
		if err := guard(); err != nil {
			return apitypes.Peer{}, err
		}
	}
	if err := s.EnsureAvailable(ctx, publicKey); err != nil && !errors.Is(err, ErrPeerNotFound) {
		return apitypes.Peer{}, err
	}
	existing, err := s.get(ctx, publicKey)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrPeerNotFound) {
		return apitypes.Peer{}, err
	}

	autoRegistered := true
	created, err := s.createLocked(ctx, publicKey, apitypes.Peer{
		PublicKey:      publicKey.String(),
		Role:           apitypes.PeerRoleClient,
		Status:         apitypes.PeerRegistrationStatusActive,
		Device:         apitypes.DeviceInfo{},
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
	if info.Hardware != nil || info.Identifiers != nil {
		return apitypes.Peer{}, fmt.Errorf("%w: hardware and identifiers are read-only", ErrInvalidInfo)
	}
	if info.Name != nil && (!utf8.ValidString(*info.Name) || len(*info.Name) > 256) {
		return apitypes.Peer{}, fmt.Errorf("%w: name must be valid UTF-8 and at most 256 bytes", ErrInvalidInfo)
	}
	if info.Emoji != nil && (!utf8.ValidString(*info.Emoji) || len(*info.Emoji) > 64) {
		return apitypes.Peer{}, fmt.Errorf("%w: emoji must be valid UTF-8 and at most 64 bytes", ErrInvalidInfo)
	}
	unlock := s.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	peer.Device.Name = info.Name
	peer.Device.Emoji = info.Emoji
	return s.putRecord(ctx, peer)
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
		if err := func() error {
			unlock := s.IconLocks.LockRecord(publicKey.String())
			defer unlock()
			peer, err := s.get(ctx, publicKey)
			if err != nil {
				if !errors.Is(err, ErrPeerNotFound) {
					return err
				}
				peer = apitypes.Peer{
					PublicKey: publicKey.String(),
					Device:    apitypes.DeviceInfo{},
				}
			}
			peer.Role = apitypes.PeerRoleEdgeNode
			peer.Status = apitypes.PeerRegistrationStatusActive
			if _, err := s.putRecord(ctx, peer); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			return err
		}
	}
	return nil
}

// SavePeer stores a full peer record and returns the persisted value.
func (s *Server) SavePeer(ctx context.Context, peer apitypes.Peer) (apitypes.Peer, error) {
	return s.put(ctx, peer)
}

// SaveRefreshedDeviceFields atomically merges device-reported fields into the
// current Peer record without replacing concurrently updated profile fields.
func (s *Server) SaveRefreshedDeviceFields(
	ctx context.Context,
	publicKey giznet.PublicKey,
	device apitypes.DeviceInfo,
	fields []string,
) (apitypes.Peer, error) {
	unlock := s.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	for _, field := range fields {
		switch field {
		case "device.hardware.manufacturer":
			if device.Hardware == nil {
				return apitypes.Peer{}, fmt.Errorf("peer: refreshed device field %q has no hardware value", field)
			}
			ensurePeerHardware(&peer.Device).Manufacturer = device.Hardware.Manufacturer
		case "device.hardware.model":
			if device.Hardware == nil {
				return apitypes.Peer{}, fmt.Errorf("peer: refreshed device field %q has no hardware value", field)
			}
			ensurePeerHardware(&peer.Device).Model = device.Hardware.Model
		case "device.hardware.hardware_revision":
			if device.Hardware == nil {
				return apitypes.Peer{}, fmt.Errorf("peer: refreshed device field %q has no hardware value", field)
			}
			ensurePeerHardware(&peer.Device).HardwareRevision = device.Hardware.HardwareRevision
		case "device.identifiers.sn":
			if device.Identifiers == nil {
				return apitypes.Peer{}, fmt.Errorf("peer: refreshed device field %q has no identifiers value", field)
			}
			ensurePeerIdentifiers(&peer.Device).Sn = device.Identifiers.Sn
		case "device.identifiers.imeis":
			if device.Identifiers == nil {
				return apitypes.Peer{}, fmt.Errorf("peer: refreshed device field %q has no identifiers value", field)
			}
			ensurePeerIdentifiers(&peer.Device).Imeis = device.Identifiers.Imeis
		case "device.identifiers.labels":
			if device.Identifiers == nil {
				return apitypes.Peer{}, fmt.Errorf("peer: refreshed device field %q has no identifiers value", field)
			}
			ensurePeerIdentifiers(&peer.Device).Labels = device.Identifiers.Labels
		default:
			return apitypes.Peer{}, fmt.Errorf("peer: unsupported refreshed device field %q", field)
		}
	}
	return s.putRecord(ctx, peer)
}

func ensurePeerHardware(device *apitypes.DeviceInfo) *apitypes.HardwareInfo {
	if device.Hardware == nil {
		device.Hardware = &apitypes.HardwareInfo{}
	}
	return device.Hardware
}

func ensurePeerIdentifiers(device *apitypes.DeviceInfo) *apitypes.DeviceIdentifiers {
	if device.Identifiers == nil {
		device.Identifiers = &apitypes.DeviceIdentifiers{}
	}
	return device.Identifiers
}

func (s *Server) approve(ctx context.Context, publicKey giznet.PublicKey, role apitypes.PeerRole) (apitypes.Peer, error) {
	if role == apitypes.PeerRoleUnspecified || !role.Valid() {
		return apitypes.Peer{}, fmt.Errorf("peer: invalid role %q", role)
	}
	unlock := s.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	approvedAt := time.Now()
	peer.Role = role
	peer.Status = apitypes.PeerRegistrationStatusActive
	peer.ApprovedAt = &approvedAt
	return s.putRecord(ctx, peer)
}

func (s *Server) block(ctx context.Context, publicKey giznet.PublicKey) (apitypes.Peer, error) {
	unlock := s.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	peer.Status = apitypes.PeerRegistrationStatusBlocked
	return s.putRecord(ctx, peer)
}

func (s *Server) delete(ctx context.Context, publicKey giznet.PublicKey, reason pendingdeletion.Reason) (apitypes.Peer, error) {
	unlock := s.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	item, err := s.deleteLocked(ctx, publicKey, reason)
	if quiescer, ok := s.PeerManager.(interface {
		QuiescePeer(context.Context, giznet.PublicKey) error
	}); err == nil && ok {
		err = quiescer.QuiescePeer(ctx, publicKey)
	}
	return item, err
}

func (s *Server) deleteLocked(ctx context.Context, publicKey giznet.PublicKey, reason pendingdeletion.Reason) (apitypes.Peer, error) {
	peer, err := s.get(ctx, publicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	store, err := s.store()
	if err != nil {
		return apitypes.Peer{}, err
	}
	record, err := pendingdeletion.New(pendingdeletion.KindPeer, peer.PublicKey, &peer.PublicKey, reason, struct {
		PublicKey string `json:"public_key"`
	}{PublicKey: peer.PublicKey}, time.Now())
	if err != nil {
		return apitypes.Peer{}, err
	}
	if _, _, err := pendingdeletion.CreateOrGet(ctx, store, record); err != nil {
		return apitypes.Peer{}, fmt.Errorf("peer: delete %s: %w", peer.PublicKey, err)
	}
	return peer, nil
}

// DeleteSelf records a deletion request for the authenticated Peer. A retry
// after a lost response reuses the durable pending record for the public key.
func (s *Server) DeleteSelf(ctx context.Context, publicKey giznet.PublicKey) error {
	unlock := s.IconLocks.LockRecord(publicKey.String())
	defer unlock()
	if _, err := s.deleteLocked(ctx, publicKey, pendingdeletion.ReasonPeerDelete); err == nil {
		return nil
	} else if !errors.Is(err, ErrPeerNotFound) {
		return err
	}
	store, err := s.store()
	if err != nil {
		return err
	}
	exists, err := pendingdeletion.HasLocator(ctx, store, pendingdeletion.KindPeer, publicKey.String())
	if err != nil {
		return err
	}
	if !exists {
		return ErrPeerNotFound
	}
	return nil
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
	if isPeerTombstone(data) {
		return apitypes.Peer{}, ErrPeerDeleted
	}
	peer, err := decodePeer(data)
	if err != nil {
		return apitypes.Peer{}, fmt.Errorf("peer: decode %s: %w", publicKeyText, err)
	}
	return peer, nil
}

func decodePeer(data []byte) (apitypes.Peer, error) {
	if isPeerTombstone(data) {
		return apitypes.Peer{}, ErrPeerDeleted
	}
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

// create is reserved for a newly authenticated Client connection. A pending
// marker or permanent tombstone makes the public key unavailable here too.
func (s *Server) create(ctx context.Context, peer apitypes.Peer) (apitypes.Peer, error) {
	if err := validatePeer(peer); err != nil {
		return apitypes.Peer{}, err
	}
	publicKey, err := publicKeyFromText(peer.PublicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	recordUnlock := s.IconLocks.LockRecord(publicKey.String())
	defer recordUnlock()
	return s.createLocked(ctx, publicKey, peer)
}

func (s *Server) createLocked(ctx context.Context, publicKey giznet.PublicKey, peer apitypes.Peer) (apitypes.Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.EnsureAvailable(ctx, publicKey); err != nil && !errors.Is(err, ErrPeerNotFound) {
		return apitypes.Peer{}, err
	}

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
	publicKey, err := publicKeyFromText(peer.PublicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}
	recordUnlock := s.IconLocks.LockRecord(publicKey.String())
	defer recordUnlock()

	return s.putRecord(ctx, peer)
}

func (s *Server) putRecord(ctx context.Context, peer apitypes.Peer) (apitypes.Peer, error) {
	if err := validatePeer(peer); err != nil {
		return apitypes.Peer{}, err
	}
	publicKey, err := publicKeyFromText(peer.PublicKey)
	if err != nil {
		return apitypes.Peer{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.EnsureAvailable(ctx, publicKey); err != nil && !errors.Is(err, ErrPeerNotFound) {
		return apitypes.Peer{}, err
	}

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

// EnsureAvailable rejects marker-time and permanent-tombstone activation.
func (s *Server) EnsureAvailable(ctx context.Context, publicKey giznet.PublicKey) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	data, err := store.Get(ctx, peerKey(publicKey.String()))
	if errors.Is(err, kv.ErrNotFound) {
		return ErrPeerNotFound
	}
	if err != nil {
		return err
	}
	if isPeerTombstone(data) {
		return ErrPeerDeleted
	}
	pending, err := pendingdeletion.HasLocator(ctx, store, pendingdeletion.KindPeer, publicKey.String())
	if err != nil {
		return err
	}
	if pending {
		return ErrPeerPendingDeletion
	}
	return nil
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
		if isPeerTombstone(entry.Value) {
			continue
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

func (s *Server) listAdminPage(ctx context.Context, cursor string, limit int) ([]adminhttp.PeerRegistrationResult, bool, *string, error) {
	store, err := s.store()
	if err != nil {
		return nil, false, nil, err
	}
	type item struct {
		publicKey string
		createdAt time.Time
		result    adminhttp.PeerRegistrationResult
	}
	items := make([]item, 0)
	for entry, err := range store.List(ctx, peersPrefix()) {
		if err != nil {
			return nil, false, nil, fmt.Errorf("peer: list: %w", err)
		}
		if len(entry.Key) != 2 {
			return nil, false, nil, fmt.Errorf("peer: malformed public-key record %v", entry.Key)
		}
		publicKey := entry.Key[1]
		if isPeerTombstone(entry.Value) {
			items = append(items, item{publicKey: publicKey, result: toAdminTombstoneResult(publicKey)})
			continue
		}
		peer, err := decodePeer(entry.Value)
		if err != nil || peer.PublicKey != publicKey {
			return nil, false, nil, fmt.Errorf("peer: decode list %s: %w", entry.Key.String(), err)
		}
		items = append(items, item{publicKey: publicKey, createdAt: peer.CreatedAt, result: toAdminRegistrationResult(peer)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].createdAt.Equal(items[j].createdAt) {
			return items[i].publicKey < items[j].publicKey
		}
		return items[i].createdAt.Before(items[j].createdAt)
	})
	start := 0
	if cursor != "" {
		start = len(items)
		for index, item := range items {
			if item.publicKey == cursor {
				start = index + 1
				break
			}
		}
	}
	if start >= len(items) {
		return nil, false, nil, nil
	}
	end := min(start+limit, len(items))
	page := make([]adminhttp.PeerRegistrationResult, 0, end-start)
	for _, item := range items[start:end] {
		page = append(page, item.result)
	}
	if end >= len(items) {
		return page, false, nil, nil
	}
	next := items[end-1].publicKey
	return page, true, &next, nil
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
	end := min(start+limit, len(items))
	page := items[start:end]
	if end >= len(items) {
		return page, false, nil, nil
	}
	nextCursor := page[len(page)-1].PublicKey
	return page, true, &nextCursor, nil
}

func (s *Server) listBySN(ctx context.Context, sn string) ([]adminhttp.PeerRegistrationResult, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	publicKeys := make(map[string]struct{})
	_, legacyErr := store.Get(ctx, snPrefix(sn))
	if legacyErr == nil {
		// The former one-to-one index stored a public key directly at the
		// prefix. Scan records so collisions hidden by that index are restored.
		for entry, err := range store.List(ctx, peersPrefix()) {
			if err != nil {
				return nil, fmt.Errorf("peer: list legacy sn %q: %w", sn, err)
			}
			if isPeerTombstone(entry.Value) {
				continue
			}
			peer, err := decodePeer(entry.Value)
			if err != nil {
				return nil, fmt.Errorf("peer: decode legacy sn %q: %w", sn, err)
			}
			if peerSN(peer) == sn {
				publicKeys[peer.PublicKey] = struct{}{}
			}
		}
	} else if !errors.Is(legacyErr, kv.ErrNotFound) {
		return nil, fmt.Errorf("peer: get legacy sn %q: %w", sn, legacyErr)
	}
	for entry, err := range store.List(ctx, snPrefix(sn)) {
		if err != nil {
			return nil, fmt.Errorf("peer: list sn %q: %w", sn, err)
		}
		if len(entry.Key) != 3 {
			return nil, fmt.Errorf("peer: malformed sn index %v", entry.Key)
		}
		publicKeys[entry.Key[2]] = struct{}{}
	}

	peers := make([]apitypes.Peer, 0, len(publicKeys))
	for publicKey := range publicKeys {
		peer, err := s.getByPublicKeyText(ctx, store, publicKey)
		if errors.Is(err, ErrPeerNotFound) || errors.Is(err, ErrPeerDeleted) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if peerSN(peer) == sn {
			peers = append(peers, peer)
		}
	}
	sort.Slice(peers, func(i, j int) bool {
		if peers[i].CreatedAt.Equal(peers[j].CreatedAt) {
			return peers[i].PublicKey < peers[j].PublicKey
		}
		return peers[i].CreatedAt.Before(peers[j].CreatedAt)
	})
	items := make([]adminhttp.PeerRegistrationResult, 0, len(peers))
	for _, peer := range peers {
		items = append(items, toAdminRegistrationResult(peer))
	}
	return items, nil
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
