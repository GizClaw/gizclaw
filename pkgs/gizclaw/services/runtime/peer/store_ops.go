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
	"golang.org/x/sync/errgroup"
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
	record, err := s.EnsureConnectedPeerGuarded(ctx, publicKey, nil)
	if err != nil {
		return apitypes.Peer{}, err
	}
	return record, s.rememberPeer(ctx, record)
}

// EnsureConnectedPeerGuarded runs guard while holding the per-Peer record lock
// and creates the connected Peer only when the guard still accepts it.
// It does not publish the Peer in the local directory; admission must first
// verify shared routing ownership before recording the local PeerRun entry.
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

// putInfo applies a partial device profile update. Only the fields present in
// info are written; absent fields keep their stored value.
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
	if info.Name != nil {
		peer.Device.Name = info.Name
	}
	if info.Emoji != nil {
		peer.Device.Emoji = info.Emoji
	}
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
	created, err := s.createLocked(ctx, publicKey, peer)
	if err != nil {
		return apitypes.Peer{}, err
	}
	return created, s.rememberPeer(ctx, created)
}

func (s *Server) createLocked(ctx context.Context, publicKey giznet.PublicKey, peer apitypes.Peer) (apitypes.Peer, error) {
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
	if err := s.rememberPeer(ctx, peer); err != nil {
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

func (s *Server) listAdminPage(ctx context.Context, cursor string, limit int) ([]adminhttp.PeerRegistrationResult, bool, *string, error) {
	if s.LocalRuns == nil {
		return nil, false, nil, errors.New("peer: local runtime directory is not configured")
	}
	keys, more, err := s.LocalRuns.ListPeerPublicKeys(ctx, cursor, limit)
	if err != nil {
		return nil, false, nil, err
	}
	store, err := s.store()
	if err != nil {
		return nil, false, nil, err
	}
	items := make([]adminhttp.PeerRegistrationResult, len(keys))
	workers, workerCtx := errgroup.WithContext(ctx)
	workers.SetLimit(8)
	for i, key := range keys {
		workers.Go(func() error {
			data, err := store.Get(workerCtx, peerKey(key))
			if err != nil {
				return err
			}
			if isPeerTombstone(data) {
				items[i] = toAdminTombstoneResult(key)
				return nil
			}
			record, err := decodePeer(data)
			if err != nil {
				return err
			}
			if record.PublicKey != key {
				return errors.New("peer: local directory registration identity mismatch")
			}
			items[i] = toAdminRegistrationResult(record)
			return nil
		})
	}
	if err := workers.Wait(); err != nil {
		return nil, false, nil, err
	}
	if more {
		return items, true, new(keys[len(keys)-1]), nil
	}
	return items, false, nil, nil
}

func (s *Server) listBySN(ctx context.Context, sn string) ([]adminhttp.PeerRegistrationResult, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	publicKeys, err := store.ListMembers(ctx, snPrefix(sn))
	if err != nil {
		return nil, fmt.Errorf("peer: list serial number index: %w", err)
	}

	candidates := make([]*apitypes.Peer, len(publicKeys))
	workers, workerCtx := errgroup.WithContext(ctx)
	workers.SetLimit(8)
	for i, publicKey := range publicKeys {
		workers.Go(func() error {
			peer, err := s.getByPublicKeyText(workerCtx, store, publicKey)
			if errors.Is(err, ErrPeerNotFound) || errors.Is(err, ErrPeerDeleted) {
				return nil
			}
			if err != nil {
				return err
			}
			if peerSN(peer) == sn {
				candidates[i] = &peer
			}
			return nil
		})
	}
	if err := workers.Wait(); err != nil {
		return nil, err
	}
	peers := make([]apitypes.Peer, 0, len(publicKeys))
	for _, peer := range candidates {
		if peer != nil {
			peers = append(peers, *peer)
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

// ListPublicKeysByIMEI returns all current peers declaring the TAC and serial.
func (s *Server) ListPublicKeysByIMEI(ctx context.Context, tac, serial string) ([]string, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	keys, err := store.ListMembers(ctx, imeiPrefix(tac, serial))
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(keys))
	for _, key := range keys {
		peer, err := s.getByPublicKeyText(ctx, store, key)
		if errors.Is(err, ErrPeerNotFound) || errors.Is(err, ErrPeerDeleted) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, item := range peerIMEIs(peer) {
			if item.Tac == tac && item.Serial == serial {
				result = append(result, key)
				break
			}
		}
	}
	sort.Strings(result)
	return result, nil
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

	// Preserve indexes still present in the new record: mutation deletes run
	// after record writes, and member removals run after additions.
	retained := make(map[string]bool, len(entries))
	for _, entry := range entries {
		retained[entry.Key.String()] = true
	}
	filtered := deletes[:0]
	for _, key := range deletes {
		if !retained[key.String()] {
			filtered = append(filtered, key)
		}
	}
	additions := identifierSets(peer)
	var removals []kv.SetMembers
	if previous != nil {
		current := make(map[string]bool, len(additions))
		for _, group := range additions {
			current[group.Key.String()] = true
		}
		for _, group := range identifierSets(*previous) {
			if previous.PublicKey != peer.PublicKey || !current[group.Key.String()] {
				removals = append(removals, group)
			}
		}
	}
	var expected []byte
	if previous != nil {
		expected, err = json.Marshal(previous)
		if err != nil {
			return fmt.Errorf("peer: encode previous record: %w", err)
		}
	}
	// A process-local lock cannot protect shared identifier indexes. Compare
	// the source record in the same transaction as all index changes so a
	// competing Server cannot publish an index derived from a stale record.
	applied, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{
			{Key: peerKey(peer.PublicKey), Expected: expected},
			pendingdeletion.AbsentLocatorCondition(pendingdeletion.KindPeer, peer.PublicKey),
		},
		Entries: entries, DeleteKeys: filtered, AddMembers: additions, RemoveMembers: removals,
	})
	if err != nil {
		return fmt.Errorf("peer: write record and identifier indexes: %w", err)
	}
	if !applied {
		pending, err := pendingdeletion.HasLocator(ctx, store, pendingdeletion.KindPeer, peer.PublicKey)
		if err != nil {
			return err
		}
		if pending {
			return ErrPeerPendingDeletion
		}
		if previous == nil {
			return ErrPeerAlreadyExists
		}
		return ErrPeerConcurrentUpdate
	}

	return nil
}

func (s *Server) rememberPeer(ctx context.Context, record apitypes.Peer) error {
	if s.LocalRuns == nil {
		return nil
	}
	key, err := publicKeyFromText(record.PublicKey)
	if err != nil {
		return err
	}
	return s.LocalRuns.RememberPeer(ctx, key, record.CreatedAt)
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
