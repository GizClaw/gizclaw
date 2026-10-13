package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// HistoryReader reads ordered messages from one explicitly selected conversation.
// It does not record input or output on behalf of its caller.
type HistoryReader interface {
	Query(context.Context, HistoryQuery) (HistoryPage, error)
}

// HistoryStore owns durable conversation messages and their asset references.
// Implementations borrow their connection and do not start background workers.
type HistoryStore interface {
	HistoryReader
	// QueryCleanup reads identities and assets for maintenance, including
	// expired messages whose content partitions have already been removed.
	QueryCleanup(context.Context, HistoryCleanupQuery) (HistoryCleanupPage, error)
	Append(context.Context, []HistoryEntry) error
	Get(context.Context, HistoryKey) (HistoryEntry, error)
	Delete(context.Context, HistoryKey) error
}

// HistoryKey identifies one message within a conversation.
type HistoryKey struct {
	ConversationID string
	ID             string
}

// HistoryAsset references a binary object; the History Store does not own its bytes.
type HistoryAsset struct {
	Name      string
	MIMEType  string
	Bytes     int64
	ExpiresAt *time.Time
}

// HistoryEntry is a delivered user or assistant message. TurnID groups input and
// output routes when the producer has that information; it is not a routing scope.
type HistoryEntry struct {
	ConversationID string
	ID             string
	TurnID         string
	Role           Role
	ActorID        string
	Origin         string
	Name           string
	Text           string
	CreatedAt      time.Time
	ExpiresAt      *time.Time
	Interrupted    bool
	Assets         []HistoryAsset
}

// Key returns the persisted identity of the message.
func (entry HistoryEntry) Key() HistoryKey {
	return HistoryKey{ConversationID: entry.ConversationID, ID: entry.ID}
}

// HistoryPosition is an exclusive, stable (creation time, message ID) boundary.
// It remains valid after the boundary message has been removed.
type HistoryPosition struct {
	CreatedAt time.Time
	ID        string
}

// HistoryQuery selects one conversation. Start/End are optional [start,end)
// creation-time bounds. Cursor seeks in the selected direction. ExcludeTurnID
// prevents a model from reading an input that it will add to its current request.
type HistoryQuery struct {
	ConversationID string
	Start, End     time.Time
	Text           string
	IDs            []string
	ExcludeTurnID  string
	Cursor         *HistoryPosition
	Descending     bool
	Limit          int
}

// HistoryPage contains messages in query order and the next exclusive boundary.
type HistoryPage struct {
	Entries []HistoryEntry
	Next    *HistoryPosition
}

// HistoryCleanupQuery selects maintenance records in (conversation, ID) order.
// Before includes only records expiring at or before that time. An empty
// conversation selects all conversations and requires a nonzero Before.
type HistoryCleanupQuery struct {
	ConversationID string
	Before         time.Time
	Cursor         *HistoryKey
	Limit          int
}

// HistoryCleanupEntry keeps only the identity and caller-owned binary assets.
type HistoryCleanupEntry struct {
	Key    HistoryKey
	Assets []HistoryAsset
}

// HistoryCleanupPage is a stable page even when previously visited keys vanish.
type HistoryCleanupPage struct {
	Entries []HistoryCleanupEntry
	Next    *HistoryKey
}

// CleanupHistory deletes binary assets before acknowledging each identity's
// removal. Failed deletion leaves references available for an idempotent retry.
// Store retention may remove content independently, but must keep these references.
func CleanupHistory(ctx context.Context, store HistoryStore, query HistoryCleanupQuery, deleteAsset func(HistoryKey, HistoryAsset) error) error {
	query.Limit = 200
	for {
		page, err := store.QueryCleanup(ctx, query)
		if err != nil {
			return err
		}
		for _, entry := range page.Entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, asset := range entry.Assets {
				if deleteAsset == nil {
					return fmt.Errorf("history: binary asset cleanup is required")
				}
				if err := deleteAsset(entry.Key, asset); err != nil {
					return err
				}
			}
			if err := store.Delete(ctx, entry.Key); err != nil && !errors.Is(err, ErrNotFound) {
				return err
			}
		}
		if page.Next == nil {
			return nil
		}
		query.Cursor = page.Next
	}
}

// MaxHistoryQueryLimit bounds allocations and individual history reads.
const MaxHistoryQueryLimit = 10_000

func validateHistoryKey(key HistoryKey) error {
	if strings.TrimSpace(key.ConversationID) == "" || strings.TrimSpace(key.ID) == "" {
		return fmt.Errorf("%w: history conversation and message id are required", ErrInvalidInput)
	}
	return nil
}

func validateHistoryEntry(entry HistoryEntry) error {
	if err := validateHistoryKey(entry.Key()); err != nil {
		return err
	}
	if entry.Role != RoleUser && entry.Role != RoleAssistant {
		return fmt.Errorf("%w: history role must be user or assistant", ErrInvalidInput)
	}
	if entry.CreatedAt.IsZero() {
		return fmt.Errorf("%w: history creation time is required", ErrInvalidInput)
	}
	for _, asset := range entry.Assets {
		if strings.TrimSpace(asset.Name) == "" || strings.TrimSpace(asset.MIMEType) == "" || asset.Bytes < 0 {
			return fmt.Errorf("%w: history asset name, MIME type and nonnegative size are required", ErrInvalidInput)
		}
	}
	return nil
}
