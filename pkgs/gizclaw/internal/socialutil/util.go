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

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	DefaultListLimit        = 50
	MaxListLimit            = 200
	DefaultInviteTokenTTL   = 5 * time.Minute
	DefaultMessageTTL       = 24 * time.Hour
	DefaultMessageMaxTTL    = 7 * 24 * time.Hour
	DefaultCleanupInterval  = 5 * time.Minute
	DefaultMaxAudioBytes    = 2 * 1024 * 1024
	DefaultAudioContentType = "audio/opus"
)

var (
	ContactsRoot           = kv.Key{"contacts"}
	FriendsRoot            = kv.Key{"friends"}
	FriendInviteTokensRoot = kv.Key{"friend-invite-tokens"}
	GroupsRoot             = kv.Key{"friend-groups"}
	GroupInviteTokensRoot  = kv.Key{"friend-group-invite-tokens"}
	GroupMembersRoot       = kv.Key{"friend-group-members"}
	GroupBelongsRoot       = kv.Key{"friend-group-belongs"}
	GroupMessagesRoot      = kv.Key{"friend-group-messages"}
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

func ListPage(ctx context.Context, store kv.Store, prefix kv.Key, cursor string, limit int) (EntryPage, error) {
	cursor, limit = NormalizeListParams(cursor, limit)
	entries, err := kv.ListAfter(ctx, store, prefix, CursorAfterKey(prefix, cursor), limit+1)
	if err != nil {
		return EntryPage{}, err
	}
	hasNext := len(entries) > limit
	if hasNext {
		entries = entries[:limit]
	}
	var next *string
	if hasNext && len(entries) > 0 {
		v := UnescapeStoreSegment(entries[len(entries)-1].Key[len(entries[len(entries)-1].Key)-1])
		next = &v
	}
	return EntryPage{Items: entries, HasNext: hasNext, NextCursor: next}, nil
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

func CursorAfterKey(prefix kv.Key, cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	return append(append(kv.Key{}, prefix...), cursor)
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

func DeletePrefix(ctx context.Context, store kv.Store, prefix kv.Key) error {
	var keys []kv.Key
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return err
		}
		keys = append(keys, entry.Key)
	}
	if len(keys) == 0 {
		return nil
	}
	return store.BatchDelete(ctx, keys)
}

func OwnerPrefix(root kv.Key, owner string) kv.Key {
	return append(append(kv.Key{}, root...), EscapeStoreSegment(strings.TrimSpace(owner)))
}

func ContactKey(owner, id string) kv.Key {
	return append(OwnerPrefix(ContactsRoot, owner), EscapeStoreSegment(id))
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

func GroupMessageKey(friendGroupID, id string) kv.Key {
	return append(append(kv.Key{}, GroupMessagesRoot...), EscapeStoreSegment(friendGroupID), EscapeStoreSegment(id))
}

func RelationID(a, b string) string {
	parts := []string{strings.TrimSpace(a), strings.TrimSpace(b)}
	sort.Strings(parts)
	return parts[0] + ":" + parts[1]
}

func DirectWorkspaceName(relationID string) string {
	return "social-direct-" + shortHash(strings.TrimSpace(relationID))
}

func GroupWorkspaceName(friendGroupID string) string {
	return "social-group-" + shortHash(strings.TrimSpace(friendGroupID))
}

func ChatRoomWorkspaceParameters(mode apitypes.ChatRoomMode) *apitypes.WorkspaceParameters {
	input := apitypes.WorkspaceInputModePushToTalk
	var params apitypes.WorkspaceParameters
	_ = params.FromChatRoomWorkspaceParameters(apitypes.ChatRoomWorkspaceParameters{
		Input: &input,
		Mode:  &mode,
	})
	return &params
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

func MessageExpired(item rpcapi.FriendGroupMessageObject, now time.Time) bool {
	return item.ExpiresAt != nil && !item.ExpiresAt.After(now)
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

func IntPtr(v int) *int {
	return &v
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
