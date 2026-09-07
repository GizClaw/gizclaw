package gameplay

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

const catalogLookupBatchSize = 256

// existingCatalogIDs checks only requested IDs and bounds each database request.
func existingCatalogIDs(ctx context.Context, db *sqlx.DB, table string, ids []string) (map[string]bool, error) {
	switch table {
	case "pet_definitions", "badge_definitions", "game_definitions":
	default:
		return nil, fmt.Errorf("gameplay: unknown catalog table %q", table)
	}
	found := make(map[string]bool, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := pathID(id); err != nil {
			return nil, err
		}
		if _, seen := found[id]; seen {
			continue
		}
		found[id] = false
		unique = append(unique, id)
	}
	for start := 0; start < len(unique); start += catalogLookupBatchSize {
		end := min(start+catalogLookupBatchSize, len(unique))
		args := make([]any, end-start)
		for i, id := range unique[start:end] {
			args[i] = id
		}
		query := "SELECT id FROM " + table + " WHERE id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + ")"
		rows, err := db.QueryContext(ctx, db.Rebind(query), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			found[id] = true
		}
		scanErr := rows.Err()
		closeErr := rows.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	return found, nil
}
