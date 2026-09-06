package socialutil

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	DefaultListLimit      = 50
	MaxListLimit          = 200
	DefaultInviteTokenTTL = 5 * time.Minute
)

var (
	ErrResourceAlreadyExists = errors.New("social: resource already exists")

	ContactsRoot           = kv.Key{"contacts"}
	ContactNamesRoot       = kv.Key{"contact-names"}
	ContactIDsRoot         = kv.Key{"contact-ids"}
	FriendsRoot            = kv.Key{"friends"}
	FriendInviteTokensRoot = kv.Key{"friend-invite-tokens"}
	GroupsRoot             = kv.Key{"friend-groups"}
	GroupInviteTokensRoot  = kv.Key{"friend-group-invite-tokens"}
	GroupMembersRoot       = kv.Key{"friend-group-members"}
	GroupBelongsRoot       = kv.Key{"friend-group-belongs"}
	GroupNamesRoot         = kv.Key{"friend-group-names"}
)

type EntryPage struct {
	Items      []kv.Entry
	HasNext    bool
	NextCursor *string
}

type ItemPage[T any] struct {
	Items      []T
	HasNext    bool
	NextCursor *string
}

func PageItems[T any](items []T, cursor string, limit int, id func(T) string) ItemPage[T] {
	cursor = strings.TrimSpace(cursor)
	_, limit = NormalizeListParams("", limit)
	start := 0
	if cursor != "" {
		for i, item := range items {
			if id(item) == cursor {
				start = i + 1
				break
			}
		}
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	hasNext := end < len(items)
	if end > len(items) {
		end = len(items)
	}
	var next *string
	if hasNext && end > start {
		v := id(items[end-1])
		next = &v
	}
	return ItemPage[T]{Items: items[start:end], HasNext: hasNext, NextCursor: next}
}

func RequireOwner(owner string) error {
	if strings.TrimSpace(owner) == "" {
		return errors.New("social: owner is required")
	}
	return nil
}

func NormalizeListParams(cursor string, limit int) (string, int) {
	normalizedCursor := EscapeStoreSegment(strings.TrimSpace(cursor))
	normalizedLimit := DefaultListLimit
	if limit > 0 {
		normalizedLimit = limit
	}
	if normalizedLimit > MaxListLimit {
		normalizedLimit = MaxListLimit
	}
	return normalizedCursor, normalizedLimit
}

func ReadJSONValue[T any](ctx context.Context, store kv.Store, key kv.Key) (T, error) {
	var out T
	data, err := store.Get(ctx, key)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}

func WriteJSON(ctx context.Context, store kv.Store, key kv.Key, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return store.Set(ctx, key, data)
}

func OwnerPrefix(root kv.Key, owner string) kv.Key {
	return append(append(kv.Key{}, root...), EscapeStoreSegment(strings.TrimSpace(owner)))
}

func ContactKey(owner, id string) kv.Key {
	return append(OwnerPrefix(ContactsRoot, owner), EscapeStoreSegment(id))
}

func ContactNameKey(owner, name string) kv.Key {
	return append(OwnerPrefix(ContactNamesRoot, owner), EscapeStoreSegment(name))
}

func ContactIDKey(id string) kv.Key {
	return append(append(kv.Key{}, ContactIDsRoot...), EscapeStoreSegment(id))
}

func FriendKey(owner, id string) kv.Key {
	return append(OwnerPrefix(FriendsRoot, owner), EscapeStoreSegment(id))
}

func FriendInviteTokenKey(peerPublicKey string) kv.Key {
	return append(append(kv.Key{}, FriendInviteTokensRoot...), EscapeStoreSegment(peerPublicKey))
}

func GroupKey(id string) kv.Key {
	return append(append(kv.Key{}, GroupsRoot...), EscapeStoreSegment(id))
}

func GroupInviteTokenKey(friendGroupID string) kv.Key {
	return append(append(kv.Key{}, GroupInviteTokensRoot...), EscapeStoreSegment(friendGroupID))
}

func GroupMemberKey(friendGroupID, peerID string) kv.Key {
	return append(append(kv.Key{}, GroupMembersRoot...), EscapeStoreSegment(friendGroupID), EscapeStoreSegment(peerID))
}

func GroupBelongKey(peerID, friendGroupID string) kv.Key {
	return append(append(kv.Key{}, GroupBelongsRoot...), EscapeStoreSegment(peerID), EscapeStoreSegment(friendGroupID))
}

func GroupNameKey(peerID, name string) kv.Key {
	return append(append(kv.Key{}, GroupNamesRoot...), EscapeStoreSegment(peerID), EscapeStoreSegment(name))
}

func RelationID(a, b string) string {
	parts := []string{strings.TrimSpace(a), strings.TrimSpace(b)}
	sort.Strings(parts)
	return parts[0] + ":" + parts[1]
}

func DirectWorkspaceName(relationID string) string {
	return "social-direct-" + shortHash(strings.TrimSpace(relationID))
}

// DirectWorkspaceIncarnationName returns the system Workspace name for one
// lifecycle of a direct-friend relationship.
func DirectWorkspaceIncarnationName(relationID, incarnationID string) string {
	return "social-direct-" + shortHash(
		strings.TrimSpace(relationID)+"\x00"+strings.TrimSpace(incarnationID),
	)
}

func GroupWorkspaceName(friendGroupID string) string {
	return "social-group-" + shortHash(strings.TrimSpace(friendGroupID))
}

func GroupRole(member rpcapi.FriendGroupMemberObject) rpcapi.FriendGroupMemberRole {
	if member.Role == nil {
		return ""
	}
	return *member.Role
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:20]
}

func TimeValue(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return *v
}

func CompareByCreatedAtAsc(aTime time.Time, aID string, bTime time.Time, bID string) bool {
	if aTime.Equal(bTime) {
		return aID < bID
	}
	return aTime.Before(bTime)
}

func CompareByCreatedAtDesc(aTime time.Time, aID string, bTime time.Time, bID string) bool {
	if aTime.Equal(bTime) {
		return aID > bID
	}
	return aTime.After(bTime)
}

func NewID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func StringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func IntValue(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

//go:fix inline
func IntPtr(v int) *int {
	return new(v)
}

func OptionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func NormalizePhone(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func EscapeStoreSegment(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}

func UnescapeStoreSegment(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

// InviteTokenIndexKey addresses one token within its invitation domain.
// Tokens are hashed so bearer credentials do not appear in storage key names.
func InviteTokenIndexKey(root kv.Key, token string) kv.Key {
	digest := sha256.Sum256([]byte(token))
	return append(append(kv.Key(nil), root...), "by-token", hex.EncodeToString(digest[:]))
}

// WriteInviteToken atomically replaces an invitation and its unique token index.
func WriteInviteToken(ctx context.Context, store kv.Store, key kv.Key, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var next struct {
		Token string `json:"invite_token"`
	}
	if err := json.Unmarshal(data, &next); err != nil {
		return err
	}
	if strings.TrimSpace(next.Token) == "" || len(key) < 2 {
		return errors.New("social: invalid invite token")
	}
	locator, err := json.Marshal(key)
	if err != nil {
		return err
	}
	index := InviteTokenIndexKey(key[:len(key)-1], next.Token)
	for range 16 {
		old, err := store.Get(ctx, key)
		if err != nil && !errors.Is(err, kv.ErrNotFound) {
			return err
		}
		var previous struct {
			Token string `json:"invite_token"`
		}
		if old != nil {
			if err := json.Unmarshal(old, &previous); err != nil {
				return err
			}
		}
		indexed, err := store.Get(ctx, index)
		if err != nil && !errors.Is(err, kv.ErrNotFound) {
			return err
		}
		if indexed != nil && string(indexed) != string(locator) {
			return errors.New("social: invite token already belongs to another resource")
		}
		mutation := kv.Mutation{
			Conditions: []kv.Condition{{Key: key, Expected: old}, {Key: index, Expected: indexed}},
			Entries:    []kv.Entry{{Key: key, Value: data}, {Key: index, Value: locator}},
		}
		if previous.Token != "" && previous.Token != next.Token {
			mutation.DeleteKeys = []kv.Key{InviteTokenIndexKey(key[:len(key)-1], previous.Token)}
		}
		ok, err := store.ApplyMutation(ctx, mutation)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return errors.New("social: invite token changed concurrently")
}

// DeleteInviteToken atomically removes the current invitation and its index.
func DeleteInviteToken(ctx context.Context, store kv.Store, key kv.Key) error {
	for range 16 {
		data, err := store.Get(ctx, key)
		if errors.Is(err, kv.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var record struct {
			Token string `json:"invite_token"`
		}
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		keys := []kv.Key{key}
		if record.Token != "" {
			keys = append(keys, InviteTokenIndexKey(key[:len(key)-1], record.Token))
		}
		ok, err := store.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{{Key: key, Expected: data}}, DeleteKeys: keys})
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return errors.New("social: invite token changed concurrently")
}

// ReadInviteToken resolves a token with two exact reads and verifies that the
// authoritative invitation still names it, including during concurrent rotation.
func ReadInviteToken(ctx context.Context, store kv.Store, root kv.Key, token string) ([]byte, error) {
	locator, err := store.Get(ctx, InviteTokenIndexKey(root, token))
	if err != nil {
		return nil, err
	}
	var key kv.Key
	if err := json.Unmarshal(locator, &key); err != nil {
		return nil, err
	}
	if len(key) != len(root)+1 {
		return nil, errors.New("social: invalid invite token locator")
	}
	for i := range root {
		if key[i] != root[i] {
			return nil, errors.New("social: invite token locator domain mismatch")
		}
	}
	data, err := store.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	var record struct {
		Token string `json:"invite_token"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	if record.Token != token {
		return nil, kv.ErrNotFound
	}
	return data, nil
}
