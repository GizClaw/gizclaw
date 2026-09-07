package friendgroup

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"golang.org/x/sync/errgroup"
)

const adminGroupShards = 16

func adminGroupIndexKey(shard int) kv.Key {
	return kv.Key{"admin-group-index", fmt.Sprint(shard)}
}

func adminGroupMembership(id string) kv.SetMembers {
	hash := sha256.Sum256([]byte(id))
	return kv.SetMembers{Key: adminGroupIndexKey(int(hash[0]) % adminGroupShards), Members: []string{id}}
}

func listAdminGroupRecords(ctx context.Context, store kv.Store, cursor string, limit int) (socialutil.EntryPage, error) {
	cursor, limit = socialutil.NormalizeListParams(cursor, limit)
	cursor = socialutil.UnescapeStoreSegment(cursor)
	query := kv.OrderedRange{Limit: limit + 1}
	if cursor != "" {
		query.After = &cursor
	}
	parts := make([][]string, adminGroupShards)
	workers, workerCtx := errgroup.WithContext(ctx)
	workers.SetLimit(8)
	for shard := range adminGroupShards {
		workers.Go(func() error {
			ids, err := store.RangeOrderedMembers(workerCtx, adminGroupIndexKey(shard), query)
			parts[shard] = ids
			return err
		})
	}
	if err := workers.Wait(); err != nil {
		return socialutil.EntryPage{}, err
	}
	var ids []string
	for _, part := range parts {
		ids = append(ids, part...)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	page := socialutil.EntryPage{HasNext: len(ids) > limit}
	if page.HasNext {
		ids = ids[:limit]
		page.NextCursor = new(ids[len(ids)-1])
	}
	entries := make([]*kv.Entry, len(ids))
	workers, workerCtx = errgroup.WithContext(ctx)
	workers.SetLimit(8)
	for i, id := range ids {
		workers.Go(func() error {
			if err := customid.ValidateFriendGroupID(id); err != nil {
				return err
			}
			key := socialutil.GroupKey(id)
			data, err := store.Get(workerCtx, key)
			if errors.Is(err, kv.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			entries[i] = &kv.Entry{Key: key, Value: data}
			return nil
		})
	}
	if err := workers.Wait(); err != nil {
		return socialutil.EntryPage{}, err
	}
	page.Items = make([]kv.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			page.Items = append(page.Items, *entry)
		}
	}
	return page, nil
}
