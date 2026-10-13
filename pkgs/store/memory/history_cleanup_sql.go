package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

// QueryCleanup reads bounded maintenance identities independently of message
// partitions. It never changes the retained-message semantics of Query or Get.
func (store *SQLHistoryStore) QueryCleanup(ctx context.Context, query HistoryCleanupQuery) (HistoryCleanupPage, error) {
	if query.Limit <= 0 || query.Limit > MaxHistoryQueryLimit || query.ConversationID == "" && query.Before.IsZero() {
		return HistoryCleanupPage{}, fmt.Errorf("%w: cleanup requires a bounded limit and a conversation or expiration cutoff", ErrInvalidInput)
	}
	where := []string{"1=1"}
	var args []any
	if query.ConversationID != "" {
		if strings.TrimSpace(query.ConversationID) == "" {
			return HistoryCleanupPage{}, fmt.Errorf("%w: invalid cleanup conversation", ErrInvalidInput)
		}
		where = append(where, "conversation_id=?")
		args = append(args, query.ConversationID)
	}
	if !query.Before.IsZero() {
		nano, err := historyNano(query.Before)
		if err != nil {
			return HistoryCleanupPage{}, err
		}
		where = append(where, "expires_at_unix_nano <= ?")
		args = append(args, nano)
	}
	if query.Cursor != nil {
		if err := validateHistoryKey(*query.Cursor); err != nil {
			return HistoryCleanupPage{}, err
		}
		where = append(where, "(conversation_id,id) > (?,?)")
		args = append(args, query.Cursor.ConversationID, query.Cursor.ID)
	}
	args = append(args, query.Limit+1)
	executor := store.executor()
	rows, err := executor.QueryContext(ctx, store.db.Rebind("SELECT conversation_id,id FROM "+store.keys.Quoted()+" WHERE "+strings.Join(where, " AND ")+" ORDER BY conversation_id,id LIMIT ?"), args...)
	if err != nil {
		return HistoryCleanupPage{}, storage.ExternalSQLError("history: query cleanup identities", err)
	}
	var page HistoryCleanupPage
	for rows.Next() {
		var entry HistoryCleanupEntry
		if err := rows.Scan(&entry.Key.ConversationID, &entry.Key.ID); err != nil {
			rows.Close()
			return HistoryCleanupPage{}, storage.ExternalSQLError("history: decode cleanup identity", err)
		}
		page.Entries = append(page.Entries, entry)
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return HistoryCleanupPage{}, storage.ExternalSQLError("history: read cleanup identities", err)
	}
	if len(page.Entries) > query.Limit {
		page.Entries = page.Entries[:query.Limit]
		key := page.Entries[len(page.Entries)-1].Key
		page.Next = &key
	}
	if len(page.Entries) == 0 {
		return page, nil
	}
	args = nil
	positions := make(map[HistoryKey]int, len(page.Entries))
	for i, entry := range page.Entries {
		positions[entry.Key] = i
		args = append(args, entry.Key.ConversationID, entry.Key.ID)
	}
	pairs := strings.TrimSuffix(strings.Repeat("(?,?),", len(page.Entries)), ",")
	rows, err = executor.QueryContext(ctx, store.db.Rebind("SELECT conversation_id,id,name,mime_type,bytes,expires_at_unix_nano FROM "+store.assets.Quoted()+" WHERE (conversation_id,id) IN ("+pairs+") ORDER BY conversation_id,id,position"), args...)
	if err != nil {
		return HistoryCleanupPage{}, storage.ExternalSQLError("history: query cleanup assets", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key HistoryKey
		var asset HistoryAsset
		var expiration sql.NullInt64
		if err := rows.Scan(&key.ConversationID, &key.ID, &asset.Name, &asset.MIMEType, &asset.Bytes, &expiration); err != nil {
			return HistoryCleanupPage{}, storage.ExternalSQLError("history: decode cleanup asset", err)
		}
		asset.ExpiresAt = historyTime(expiration)
		i := positions[key]
		page.Entries[i].Assets = append(page.Entries[i].Assets, asset)
	}
	return page, storage.ExternalSQLError("history: read cleanup assets", rows.Err())
}
