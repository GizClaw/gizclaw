package pendingdeletion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"golang.org/x/sync/errgroup"
)

const kvTaskIndexShards = 16
const kvTaskIndexTimeLayout = "2006-01-02T15:04:05.000000000Z"

func kvTaskShard(id string) int {
	sum := sha256.Sum256([]byte(id))
	return int(sum[0]) % kvTaskIndexShards
}

func kvDueIndexKey(kind Kind, shard int) kv.Key {
	return kv.Key{"pending-deletion", "due-index", string(kind), fmt.Sprint(shard)}
}

func kvCreatedIndexKey(kind Kind, status Status, shard int) kv.Key {
	return kv.Key{"pending-deletion", "created-index", string(kind), string(status), fmt.Sprint(shard)}
}

func kvIndexMember(at time.Time, task Task) string {
	return at.UTC().Format(kvTaskIndexTimeLayout) + "/" + task.Record.DeletionID + "/" + task.MarkerFingerprint
}

func parseKVIndexMember(member string) (at time.Time, id, fingerprint string, err error) {
	parts := strings.Split(member, "/")
	if len(parts) != 3 || parts[1] == "" {
		return time.Time{}, "", "", fmt.Errorf("%w: invalid task index member", ErrInvalid)
	}
	at, err = time.Parse(kvTaskIndexTimeLayout, parts[0])
	if err != nil || at.UTC().Format(kvTaskIndexTimeLayout) != parts[0] {
		return time.Time{}, "", "", fmt.Errorf("%w: invalid task index time", ErrInvalid)
	}
	digest, err := hex.DecodeString(parts[2])
	if err != nil || len(digest) != sha256.Size || hex.EncodeToString(digest) != parts[2] {
		return time.Time{}, "", "", fmt.Errorf("%w: invalid task index fingerprint", ErrInvalid)
	}
	return at, parts[1], parts[2], nil
}

func kvTaskIndexes(task *Task) []kv.SetMembers {
	if task == nil {
		return nil
	}
	shard := kvTaskShard(task.Record.DeletionID)
	indexes := []kv.SetMembers{{Key: kvCreatedIndexKey(task.Record.Kind, task.Status, shard), Members: []string{kvIndexMember(task.Record.DeletedAt, *task)}}}
	var due time.Time
	switch task.Status {
	case StatusQueued, StatusRetryWait:
		due = task.NextAttemptAt
	case StatusRunning:
		due = task.LeaseDeadline
	}
	if !due.IsZero() {
		indexes = append(indexes, kv.SetMembers{Key: kvDueIndexKey(task.Record.Kind, shard), Members: []string{kvIndexMember(due, *task)}})
	}
	return indexes
}

// kvTaskIndexChanges omits unchanged memberships: adding then removing the same
// member in one mutation would otherwise erase an unchanged creation index.
func kvTaskIndexChanges(before, after *Task) (add, remove []kv.SetMembers) {
	old, next := kvTaskIndexes(before), kvTaskIndexes(after)
	contains := func(indexes []kv.SetMembers, candidate kv.SetMembers) bool {
		return slices.ContainsFunc(indexes, func(item kv.SetMembers) bool {
			return slices.Equal(item.Key, candidate.Key) && item.Members[0] == candidate.Members[0]
		})
	}
	for _, item := range next {
		if !contains(old, item) {
			add = append(add, item)
		}
	}
	for _, item := range old {
		if !contains(next, item) {
			remove = append(remove, item)
		}
	}
	return add, remove
}

// readKVTaskIndexes bounds each shard query and overlaps at most eight remote
// reads. Merging shard-local prefixes yields the global ordered prefix without
// loading records outside the requested page.
func readKVTaskIndexes(ctx context.Context, store kv.Store, keys []kv.Key, query kv.OrderedRange) ([]string, error) {
	parts := make([][]string, len(keys))
	workers, workerCtx := errgroup.WithContext(ctx)
	workers.SetLimit(8)
	for i, key := range keys {
		workers.Go(func() error {
			members, err := store.RangeOrderedMembers(workerCtx, key, query)
			if err == nil {
				parts[i] = members
			}
			return err
		})
	}
	if err := workers.Wait(); err != nil {
		return nil, err
	}
	var members []string
	for _, part := range parts {
		members = append(members, part...)
	}
	slices.Sort(members)
	return slices.Compact(members), nil
}

func (s KVSource) scanIndexedDue(ctx context.Context, now time.Time, limit int, cursor string) ([]Reference, string, error) {
	if err := s.validateTaskSource(); err != nil {
		return nil, "", err
	}
	if limit <= 0 {
		return nil, "", fmt.Errorf("%w: scan limit must be positive", ErrInvalid)
	}
	query := kv.OrderedRange{Before: new(now.UTC().Format(kvTaskIndexTimeLayout) + "\xff"), Limit: limit + 1}
	if cursor != "" {
		if _, _, _, err := parseKVIndexMember(cursor); err != nil {
			return nil, "", err
		}
		query.After = &cursor
	}
	var keys []kv.Key
	for _, kind := range s.OwnedKinds {
		for shard := range kvTaskIndexShards {
			keys = append(keys, kvDueIndexKey(kind, shard))
		}
	}
	members, err := readKVTaskIndexes(ctx, s.Store, keys, query)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(members) > limit {
		members = members[:limit]
		next = members[len(members)-1]
	}
	refs := make([]Reference, 0, len(members))
	for _, member := range members {
		_, id, fingerprint, err := parseKVIndexMember(member)
		if err != nil {
			return nil, "", err
		}
		refs = append(refs, Reference{Source: s.Name(), DeletionID: id, MarkerFingerprint: fingerprint})
	}
	return refs, next, nil
}

func (s KVSource) listIndexedTasks(ctx context.Context, options SourceListOptions) ([]Task, error) {
	if err := s.validateTaskSource(); err != nil {
		return nil, err
	}
	if options.Limit <= 0 {
		return nil, fmt.Errorf("%w: list limit must be positive", ErrInvalid)
	}
	query := kv.OrderedRange{Limit: options.Limit + 1}
	if options.StartTime != nil {
		query.After = new(options.StartTime.UTC().Format(kvTaskIndexTimeLayout))
	}
	if options.EndTime != nil {
		query.Before = new(options.EndTime.UTC().Format(kvTaskIndexTimeLayout))
	}
	if options.AfterCreatedAt != nil {
		after := options.AfterCreatedAt.UTC().Format(kvTaskIndexTimeLayout)
		if s.Name() < options.AfterSource {
			after += "\xff"
		} else if s.Name() == options.AfterSource {
			after += "/" + options.AfterDeletionID + "/\xff"
		}
		if query.After == nil || after > *query.After {
			query.After = &after
		}
	}
	var keys []kv.Key
	for _, kind := range s.OwnedKinds {
		if len(options.Kinds) > 0 && !options.Kinds[kind] {
			continue
		}
		for _, status := range []Status{StatusQueued, StatusRunning, StatusRetryWait, StatusFailed} {
			if len(options.Statuses) > 0 && !options.Statuses[status] {
				continue
			}
			for shard := range kvTaskIndexShards {
				keys = append(keys, kvCreatedIndexKey(kind, status, shard))
			}
		}
	}
	members, err := readKVTaskIndexes(ctx, s.Store, keys, query)
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, options.Limit)
	for _, member := range members {
		_, id, fingerprint, err := parseKVIndexMember(member)
		if err != nil {
			return nil, err
		}
		task, _, err := s.loadTask(ctx, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if task.MarkerFingerprint != fingerprint || !taskMatches(task, options) {
			continue
		}
		tasks = append(tasks, task)
		if len(tasks) == options.Limit {
			break
		}
	}
	return tasks, nil
}
