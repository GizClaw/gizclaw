// Package apikey manages long-lived, device-bound API keys.
package apikey

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	secretPrefix       = "gizclaw_sk_v1_"
	namePrefix         = "key_"
	maxDisplayNameSize = 80
	defaultListLimit   = 50
	maxListLimit       = 100
)

var (
	ErrInvalidAPIKey      = errors.New("api key: invalid credential")
	ErrInvalidOwner       = errors.New("api key: invalid owner")
	ErrInvalidDisplayName = errors.New("api key: display name must be non-empty UTF-8 and at most 80 bytes")
	ErrInvalidCursor      = errors.New("api key: invalid cursor")
	ErrInvalidName        = errors.New("api key: invalid name")
	ErrOwnerRetired       = errors.New("api key: owner is retired")
	ErrForbidden          = errors.New("api key: management permission required")
	ErrNotFound           = errors.New("api key: not found")
)

// Key is the stored representation of an API key. APIKey is intentionally
// recoverable and is returned by the API key management surfaces.
type Key struct {
	Name          string    `json:"name"`
	DisplayName   string    `json:"display_name"`
	Prefix        string    `json:"prefix"`
	APIKey        string    `json:"api_key"`
	ManageAPIKeys bool      `json:"manage_api_keys"`
	CreatedAt     time.Time `json:"created_at"`
	Owner         string    `json:"owner"`
}

// Created contains the stored key plus the complete credential.
type Created struct {
	Key    Key
	Secret string
}

// Principal is an authenticated API key.
type Principal struct {
	Key Key
}

// ListResult is one page of API keys owned by a device.
type ListResult struct {
	Items      []Key
	NextCursor string
}

// Server persists API keys and their credential and owner indexes.
type Server struct {
	Store kv.Store

	mu     sync.Mutex
	now    func() time.Time
	random io.Reader
}

// NewServer creates an API key service over store.
func NewServer(store kv.Store) *Server {
	return &Server{Store: store, now: time.Now, random: rand.Reader}
}

// Create issues one API key for a canonical device owner.
func (s *Server) Create(ctx context.Context, owner, displayName string, manageAPIKeys bool) (Created, error) {
	if s == nil || s.Store == nil {
		return Created{}, errors.New("api key: store is not configured")
	}
	if err := validateOwner(owner); err != nil {
		return Created{}, err
	}
	displayName, err := normalizeDisplayName(displayName)
	if err != nil {
		return Created{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Store.Get(ctx, retiredKey(owner)); err == nil {
		return Created{}, ErrOwnerRetired
	} else if !errors.Is(err, kv.ErrNotFound) {
		return Created{}, err
	}
	for range 8 {
		nameRandom, err := randomText(s.random, 16)
		if err != nil {
			return Created{}, err
		}
		secretRandom, err := randomText(s.random, 32)
		if err != nil {
			return Created{}, err
		}
		secret := secretPrefix + secretRandom
		item := Key{
			Name:          namePrefix + nameRandom,
			DisplayName:   displayName,
			Prefix:        secretPrefix + secretRandom[:8] + "…",
			APIKey:        secret,
			ManageAPIKeys: manageAPIKeys,
			CreatedAt:     s.nowOrDefault().UTC(),
			Owner:         owner,
		}
		data, err := json.Marshal(item)
		if err != nil {
			return Created{}, err
		}
		guards := []kv.Entry{
			{Key: recordKey(item.Name), Value: data},
			{Key: secretKey(secret), Value: []byte(item.Name)},
		}
		_, _, created, err := kv.CreateIfAllAbsent(ctx, s.Store, guards, []kv.Entry{
			{Key: ownerKey(owner, item.Name), Value: []byte(item.Name)},
		})
		if err != nil {
			return Created{}, err
		}
		if created {
			return Created{Key: item, Secret: secret}, nil
		}
	}
	return Created{}, errors.New("api key: random identifier collision limit reached")
}

// Authenticate resolves a bearer credential using its plaintext index.
func (s *Server) Authenticate(ctx context.Context, secret string) (Principal, error) {
	if s == nil || s.Store == nil || !validSecret(secret) {
		return Principal{}, ErrInvalidAPIKey
	}
	name, err := s.Store.Get(ctx, secretKey(secret))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return Principal{}, ErrInvalidAPIKey
		}
		return Principal{}, err
	}
	item, err := s.load(ctx, string(name))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Principal{}, ErrInvalidAPIKey
		}
		return Principal{}, err
	}
	if item.APIKey != secret {
		return Principal{}, ErrInvalidAPIKey
	}
	return Principal{Key: item}, nil
}

// GetSelf returns the complete credential used by the caller.
func (s *Server) GetSelf(principal Principal) Key { return principal.Key }

// List returns API keys for the caller's device.
func (s *Server) List(ctx context.Context, principal Principal, cursor string, limit int) (ListResult, error) {
	if !principal.Key.ManageAPIKeys {
		return ListResult{}, ErrForbidden
	}
	return s.ListOwner(ctx, principal.Key.Owner, cursor, limit)
}

// ListOwner returns API keys for a canonical device owner. It is intended for
// the authenticated Peer RPC root management surface.
func (s *Server) ListOwner(ctx context.Context, owner, cursor string, limit int) (ListResult, error) {
	if s == nil || s.Store == nil {
		return ListResult{}, errors.New("api key: store is not configured")
	}
	if err := validateOwner(owner); err != nil {
		return ListResult{}, err
	}
	if cursor != "" && !validName(cursor) {
		return ListResult{}, ErrInvalidCursor
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []Key
	for entry, err := range s.Store.List(ctx, ownerPrefix(owner)) {
		if err != nil {
			return ListResult{}, err
		}
		if len(entry.Key) != 3 {
			return ListResult{}, fmt.Errorf("api key: malformed owner index %v", entry.Key)
		}
		name := entry.Key[2]
		if name <= cursor {
			continue
		}
		item, err := s.load(ctx, name)
		if err != nil {
			return ListResult{}, err
		}
		if item.Owner != owner {
			return ListResult{}, fmt.Errorf("api key: cross-owned index %v", entry.Key)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	result := ListResult{Items: items}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.NextCursor = result.Items[len(result.Items)-1].Name
	}
	return result, nil
}

// Get returns one same-owner API key to a management credential.
func (s *Server) Get(ctx context.Context, principal Principal, name string) (Key, error) {
	if !principal.Key.ManageAPIKeys {
		return Key{}, ErrForbidden
	}
	item, err := s.load(ctx, name)
	if err != nil {
		return Key{}, err
	}
	if item.Owner != principal.Key.Owner {
		return Key{}, ErrNotFound
	}
	return item, nil
}

// RevokeSelf revokes the credential used by the caller.
func (s *Server) RevokeSelf(ctx context.Context, principal Principal) error {
	return s.revoke(ctx, principal.Key.Owner, principal.Key.Name)
}

// Revoke revokes one same-owner API key using a management credential.
func (s *Server) Revoke(ctx context.Context, principal Principal, name string) error {
	if !principal.Key.ManageAPIKeys {
		return ErrForbidden
	}
	return s.revoke(ctx, principal.Key.Owner, name)
}

// RevokeOwner revokes one API key for a canonical device owner. It is intended
// for the authenticated Peer RPC root management surface.
func (s *Server) RevokeOwner(ctx context.Context, owner, name string) error {
	if s == nil || s.Store == nil {
		return errors.New("api key: store is not configured")
	}
	if err := validateOwner(owner); err != nil {
		return err
	}
	if !validName(name) {
		return ErrInvalidName
	}
	return s.revoke(ctx, owner, name)
}

// CleanupPeer revokes all API keys owned by a canonical device. It is safe to replay.
func (s *Server) CleanupPeer(ctx context.Context, owner string) error {
	if err := validateOwner(owner); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var deletes []kv.Key
	for entry, err := range s.Store.List(ctx, ownerPrefix(owner)) {
		if err != nil {
			return err
		}
		if len(entry.Key) != 3 {
			return fmt.Errorf("api key: malformed owner index %v", entry.Key)
		}
		item, err := s.load(ctx, entry.Key[2])
		if err != nil {
			return err
		}
		if item.Owner != owner {
			return fmt.Errorf("api key: cross-owned index %v", entry.Key)
		}
		deletes = append(deletes, entry.Key, recordKey(item.Name), secretKey(item.APIKey))
	}
	return s.Store.BatchMutate(ctx, []kv.Entry{{Key: retiredKey(owner), Value: []byte("retired")}}, deletes)
}

func (s *Server) revoke(ctx context.Context, owner, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.load(ctx, name)
	if err != nil {
		return err
	}
	if item.Owner != owner {
		return ErrNotFound
	}
	return s.Store.BatchDelete(ctx, []kv.Key{recordKey(name), secretKey(item.APIKey), ownerKey(owner, name)})
}

func (s *Server) load(ctx context.Context, name string) (Key, error) {
	data, err := s.Store.Get(ctx, recordKey(name))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return Key{}, ErrNotFound
		}
		return Key{}, err
	}
	var item Key
	if err := json.Unmarshal(data, &item); err != nil {
		return Key{}, fmt.Errorf("api key: malformed record %q: %w", name, err)
	}
	if item.Name != name || item.Owner == "" || !validSecret(item.APIKey) {
		return Key{}, fmt.Errorf("api key: malformed record %q", name)
	}
	return item, nil
}

func (s *Server) nowOrDefault() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func validateOwner(owner string) error {
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(owner)); err != nil || publicKey.IsZero() || publicKey.String() != owner {
		return ErrInvalidOwner
	}
	return nil
}

func normalizeDisplayName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || len(value) > maxDisplayNameSize {
		return "", ErrInvalidDisplayName
	}
	return value, nil
}

func randomText(reader io.Reader, size int) (string, error) {
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func validSecret(secret string) bool {
	if len(secret) != len(secretPrefix)+43 || !strings.HasPrefix(secret, secretPrefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(secret, secretPrefix))
	return err == nil && len(decoded) == 32
}

func validName(name string) bool {
	if len(name) != len(namePrefix)+22 || !strings.HasPrefix(name, namePrefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(name, namePrefix))
	return err == nil && len(decoded) == 16
}

func recordKey(name string) kv.Key       { return kv.Key{"records", name} }
func secretKey(secret string) kv.Key     { return kv.Key{"secrets", secret} }
func ownerPrefix(owner string) kv.Key    { return kv.Key{"owners", owner} }
func ownerKey(owner, name string) kv.Key { return kv.Key{"owners", owner, name} }
func retiredKey(owner string) kv.Key     { return kv.Key{"retired", owner} }
