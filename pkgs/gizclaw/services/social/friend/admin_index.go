package friend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"golang.org/x/sync/errgroup"
)

var adminFriendBucketsKey = kv.Key{"admin-friend-index", "buckets"}

func adminFriendBucket(member string) string {
	sum := sha256.Sum256([]byte(member))
	return hex.EncodeToString(sum[:1])
}

func adminFriendBucketKey(bucket string) kv.Key {
	return kv.Key{"admin-friend-index", "members", bucket}
}

func adminFriendMembership(owner, relationID string) kv.SetMembers {
	member := adminFriendCursor(socialutil.FriendKey(owner, relationID))
	return kv.SetMembers{Key: adminFriendBucketKey(adminFriendBucket(member)), Members: []string{member}}
}

func adminFriendDirectoryMembership(owner, relationID string) kv.SetMembers {
	membership := adminFriendMembership(owner, relationID)
	return kv.SetMembers{Key: adminFriendBucketsKey, Members: []string{membership.Key[2]}}
}

func adminFriendIndexCursor(key kv.Key) string {
	member := adminFriendCursor(key)
	return adminFriendBucket(member) + "/" + member
}

// adminFriendPageKeys reads only buckets at or after the opaque cursor, stopping
// at the requested lookahead limit. Ordering is bucket then owner/relation ID.
func adminFriendPageKeys(ctx context.Context, store kv.Store, cursor string, limit int) ([]kv.Key, error) {
	startBucket, after := "", ""
	parts := strings.Split(strings.TrimSpace(cursor), "/")
	if len(parts) == 3 && parts[1] != "" && parts[2] != "" {
		member := parts[1] + "/" + parts[2]
		if parts[0] == adminFriendBucket(member) {
			startBucket, after = parts[0], member
		}
	}
	buckets, err := store.ListMembers(ctx, adminFriendBucketsKey)
	if err != nil {
		return nil, err
	}
	if len(buckets) > 256 {
		return nil, errors.New("social: invalid Admin Friend bucket count")
	}
	slices.Sort(buckets)
	keys := make([]kv.Key, 0, limit)
	for _, bucket := range buckets {
		decoded, err := hex.DecodeString(bucket)
		if err != nil || len(decoded) != 1 || hex.EncodeToString(decoded) != bucket {
			return nil, errors.New("social: invalid Admin Friend bucket")
		}
		if bucket < startBucket {
			continue
		}
		query := kv.OrderedRange{Limit: limit - len(keys)}
		if bucket == startBucket {
			query.After = &after
		}
		members, err := store.RangeOrderedMembers(ctx, adminFriendBucketKey(bucket), query)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			if bucket == startBucket && member <= after {
				continue
			}
			parts := strings.Split(member, "/")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" || adminFriendBucket(member) != bucket {
				return nil, errors.New("social: invalid Admin Friend index member")
			}
			keys = append(keys, append(append(kv.Key{}, socialutil.FriendsRoot...), parts...))
			if len(keys) == limit {
				return keys, nil
			}
		}
	}
	return keys, nil
}

// loadAdminFriendRows bounds network concurrency and deduplicates binding reads.
// Each worker owns one result slot; no mutex is held across database I/O.
func loadAdminFriendRows(ctx context.Context, store kv.Store, keys []kv.Key) ([]adminhttp.AdminFriendObject, error) {
	records := make([]*friendRecord, len(keys))
	workers, workerCtx := errgroup.WithContext(ctx)
	workers.SetLimit(8)
	for i, key := range keys {
		workers.Go(func() error {
			data, err := store.Get(workerCtx, key)
			if errors.Is(err, kv.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			var record friendRecord
			if err := json.Unmarshal(data, &record); err != nil {
				return err
			}
			if err := record.validate(); err != nil {
				return err
			}
			owner, _ := adminFriendOwner(key)
			if record.RelationID != socialutil.UnescapeStoreSegment(key[2]) || record.RelationID != socialutil.RelationID(owner, record.PeerPublicKey) {
				return errors.New("social: Admin Friend index identity mismatch")
			}
			records[i] = &record
			return nil
		})
	}
	if err := workers.Wait(); err != nil {
		return nil, err
	}
	positions := make(map[string]int)
	var ids []string
	for _, record := range records {
		if record == nil {
			continue
		}
		if _, ok := positions[record.RelationID]; !ok {
			positions[record.RelationID] = len(ids)
			ids = append(ids, record.RelationID)
		}
	}
	bindings := make([]workspaceBinding, len(ids))
	workers, workerCtx = errgroup.WithContext(ctx)
	workers.SetLimit(8)
	for i, id := range ids {
		workers.Go(func() error {
			binding, err := readWorkspaceBinding(workerCtx, store, id)
			if errors.Is(err, kv.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			bindings[i] = binding
			return nil
		})
	}
	if err := workers.Wait(); err != nil {
		return nil, err
	}
	items := make([]adminhttp.AdminFriendObject, 0, len(keys))
	for i, record := range records {
		if record == nil {
			continue
		}
		binding := bindings[positions[record.RelationID]]
		if binding.WorkspaceID == "" {
			continue
		}
		if binding.WorkspaceName != record.WorkspaceName {
			return nil, errors.New("social: Admin Friend binding changed during listing")
		}
		owner, _ := adminFriendOwner(keys[i])
		items = append(items, adminhttp.AdminFriendObject{
			OwnerPublicKey: owner, Id: record.RelationID, PeerPublicKey: record.PeerPublicKey,
			WorkspaceId: binding.WorkspaceID, CreatedAt: &record.CreatedAt, UpdatedAt: &record.UpdatedAt,
		})
	}
	return items, nil
}
