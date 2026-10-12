package logstore

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

var sqlLogIndexes = []struct {
	purpose string
	columns string
	want    []string
}{
	{"page_idx", "timestamp_unix_nano, stream, id", []string{"timestamp_unix_nano", "stream", "id"}},
	{"selector_idx", "stream, kind, severity", []string{"stream", "kind", "severity"}},
	{"stream_page_idx", "stream, timestamp_unix_nano, id", []string{"stream", "timestamp_unix_nano", "id"}},
	{"expiry_idx", "expires_at_unix_nano", []string{"expires_at_unix_nano"}},
}

func ensureSQLSchema(ctx context.Context, db *sqlx.DB, table storage.SQLTable) error {
	statements := []string{"CREATE TABLE IF NOT EXISTS " + table.Quoted() + " (stream TEXT NOT NULL,id TEXT NOT NULL,timestamp_unix_nano BIGINT NOT NULL,expires_at_unix_nano BIGINT,kind TEXT NOT NULL,severity TEXT NOT NULL,message TEXT NOT NULL,attributes_json TEXT NOT NULL,payload_json BLOB NOT NULL,PRIMARY KEY(stream,id))"}
	for _, index := range sqlLogIndexes {
		name, err := storage.SQLIndexName(table, index.purpose)
		if err != nil {
			return err
		}
		statements = append(statements, "CREATE INDEX IF NOT EXISTS \""+name+"\" ON "+table.Quoted()+" ("+index.columns+")")
	}
	return storage.EnsureSQLTable(ctx, db, table, statements...)
}
