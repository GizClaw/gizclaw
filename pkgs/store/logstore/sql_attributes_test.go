package logstore

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

func newPostgresAttributeLog(t *testing.T, ttl time.Duration) (*SQLStore, *sqlx.DB) {
	t.Helper()
	dsn := os.Getenv("GIZCLAW_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GIZCLAW_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	db, err := sqlx.ConnectContext(t.Context(), "postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	table := "log_attributes_" + strings.ToLower(rand.Text())
	store, err := NewSQLStoreWithDBAndTTL(db, table, ttl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
		_, _ = db.Exec(`DROP TABLE "` + table + `" CASCADE`)
		_, _ = db.Exec(`DROP TABLE IF EXISTS "` + table + `_keys"`)
	})
	return store, db
}

func TestSQLLogAttributeIsolationAndPagination(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var store *SQLStore
			if backend == "sqlite" {
				store, _ = newSQLiteLog(t)
			} else {
				store, _ = newPostgresAttributeLog(t, 0)
			}
			at := time.Now().UTC().Truncate(time.Millisecond)
			value := "quote'\"\\中文 1 true \\u0000"
			attributes := map[string]string{"scope": "workspace", "agent_id": "assistant", "context_id": "conversation", "request.id": value, "empty": ""}
			base := Record{ID: "a", Time: at, Stream: "eino.history", Kind: "message", Attributes: attributes}
			records := []Record{base, cloneRecord(base), cloneRecord(base), cloneRecord(base), cloneRecord(base), cloneRecord(base), cloneRecord(base), cloneRecord(base)}
			records[1].ID = "b"
			records[2].ID = "older"
			records[2].Time = at.Add(-time.Millisecond)
			records[3].ID = "other-scope"
			records[3].Attributes["scope"] = "other"
			records[4].ID = "other-agent"
			records[4].Attributes["agent_id"] = "other"
			records[5].ID = "other-context"
			records[5].Attributes["context_id"] = "other"
			records[6].ID = "other-kind"
			records[6].Kind = "event"
			records[7].ID = "other-stream"
			records[7].Stream = "other"
			if _, err := store.Append(t.Context(), records); err != nil {
				t.Fatal(err)
			}
			query := Query{Start: at.Add(-time.Second), End: at.Add(time.Second), Streams: []string{"eino.history"}, Kinds: []string{"message"}, Limit: 1, Order: OrderDesc, Matchers: []AttributeMatcher{
				{Name: "scope", Op: MatchEqual, Value: "workspace"}, {Name: "agent_id", Op: MatchEqual, Value: "assistant"}, {Name: "context_id", Op: MatchEqual, Value: "conversation"},
			}}
			for _, order := range []Order{OrderDesc, OrderAsc} {
				query.Order = order
				query.Cursor = ""
				var ids []string
				for {
					page, err := store.Query(t.Context(), query)
					if err != nil {
						t.Fatal(err)
					}
					for _, record := range page.Records {
						ids = append(ids, record.ID)
					}
					if !page.HasNext {
						break
					}
					query.Cursor = page.NextCursor
					query.Limit = 2
				}
				want := []string{"b", "a", "older"}
				if order == OrderAsc {
					slices.Reverse(want)
				}
				if !slices.Equal(ids, want) {
					t.Fatalf("%s IDs=%v, want %v", order, ids, want)
				}
			}
			query.Cursor = ""
			query.Limit = 20
			for _, test := range []struct {
				name     string
				matchers []AttributeMatcher
				want     int
			}{
				{"literal dotted key", []AttributeMatcher{{Name: "request.id", Op: MatchEqual, Value: value}}, 7},
				{"case sensitive", []AttributeMatcher{{Name: "request.id", Op: MatchEqual, Value: strings.ToUpper(value)}}, 0},
				{"missing inequality", []AttributeMatcher{{Name: "missing", Op: MatchNotEqual, Value: "anything"}}, 0},
				{"present inequality", []AttributeMatcher{{Name: "request.id", Op: MatchNotEqual, Value: "other"}}, 7},
				{"empty exists", []AttributeMatcher{{Name: "empty", Op: MatchExists}}, 7},
				{"empty not exists", []AttributeMatcher{{Name: "empty", Op: MatchNotExists}}, 0},
				{"missing not exists", []AttributeMatcher{{Name: "missing", Op: MatchNotExists}}, 7},
				{"conflicting equalities", []AttributeMatcher{{Name: "scope", Op: MatchEqual, Value: "workspace"}, {Name: "scope", Op: MatchEqual, Value: "other"}}, 0},
				{"duplicate equalities", []AttributeMatcher{{Name: "scope", Op: MatchEqual, Value: "workspace"}, {Name: "scope", Op: MatchEqual, Value: "workspace"}}, 6},
			} {
				t.Run(test.name, func(t *testing.T) {
					query.Kinds = nil
					query.Matchers = test.matchers
					page, err := store.Query(t.Context(), query)
					if err != nil || len(page.Records) != test.want {
						t.Fatalf("records=%d, want %d: %v", len(page.Records), test.want, err)
					}
				})
			}
			// Updating an attribute must also update the lookup index.
			changed := cloneRecord(base)
			changed.Attributes["request.id"] = "changed"
			if err := store.Replace(t.Context(), changed); err != nil {
				t.Fatal(err)
			}
			query.Matchers = []AttributeMatcher{{Name: "request.id", Op: MatchEqual, Value: "changed"}}
			page, err := store.Query(t.Context(), query)
			if err != nil || len(page.Records) != 1 || page.Records[0].ID != "a" {
				t.Fatalf("replaced attribute: %+v, %v", page, err)
			}
		})
	}
}

func TestPostgreSQLLogAttributesIndexUpgradeAndPartitions(t *testing.T) {
	for _, ttl := range []time.Duration{0, 30 * 24 * time.Hour} {
		t.Run(ttl.String(), func(t *testing.T) {
			store, db := newPostgresAttributeLog(t, ttl)
			ctx := t.Context()
			name, err := storage.SQLIndexName(store.table, postgresAttributesIndexPurpose)
			if err != nil {
				t.Fatal(err)
			}
			// Reproduce a pre-index table with existing records and all its other
			// indexes, rather than validating only an empty new database.
			if _, err := db.ExecContext(ctx, `DROP INDEX "`+name+`"`); err != nil {
				t.Fatal(err)
			}
			record := Record{ID: "legacy", Stream: "eino.history", Kind: "message", Time: time.Now().UTC().Truncate(time.Millisecond), Attributes: map[string]string{"scope": "existing", "agent_id": "assistant", "context_id": "conversation"}, Payload: []byte(` {"version":1} `)}
			if _, err := store.Append(ctx, []Record{record}); err != nil {
				t.Fatal(err)
			}
			query := Query{Start: record.Time.Add(-time.Second), End: record.Time.Add(time.Second), Streams: []string{record.Stream}, Kinds: []string{record.Kind}, Matchers: []AttributeMatcher{{Name: "scope", Op: MatchEqual, Value: "existing"}}, Limit: 1, Order: OrderDesc}
			bound := normalizeSQLQuery(query)
			// Version-1 cursors from the original builder retain their selector
			// and ordering identity after the database index is introduced.
			cursor, err := encodeSQLCursor(sqlCursor{Version: 1, Query: bound, Position: sqlPosition{TimeUnixNano: record.Time.UnixNano(), Stream: record.Stream, ID: record.ID}})
			if err != nil {
				t.Fatal(err)
			}
			upgraded, err := NewSQLStoreWithDBAndTTL(db, store.table.Name(), ttl)
			if err != nil {
				t.Fatal(err)
			}
			defer upgraded.Close()
			got, err := upgraded.Get(ctx, record.Key())
			if err != nil || string(got.Payload) != string(record.Payload) || !got.Time.Equal(record.Time) || got.Attributes["scope"] != "existing" {
				t.Fatalf("upgrade changed legacy record: %+v, %v", got, err)
			}
			query.Cursor = cursor
			if page, err := upgraded.Query(ctx, query); err != nil || len(page.Records) != 0 {
				t.Fatalf("old continuation: %+v, %v", page, err)
			}
			if ttl > 0 {
				// A changed retention horizon makes a genuinely new expiration
				// partition. Force Append's cold partition-preparation path too.
				later, err := NewSQLStoreWithDBAndTTL(db, store.table.Name(), ttl+7*24*time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				defer later.Close()
				day := time.Now().UTC().Add(ttl + 7*24*time.Hour).Truncate(24 * time.Hour)
				child := store.table.Name() + "_p" + day.Format("20060102")
				if _, err := db.ExecContext(ctx, `DROP TABLE "`+child+`"`); err != nil {
					t.Fatal(err)
				}
				fresh := cloneRecord(record)
				fresh.ID = "new-partition"
				fresh.Attributes["scope"] = "new"
				if _, err := later.Append(ctx, []Record{fresh}); err != nil {
					t.Fatal(err)
				}
				var children, indexes int
				if err := db.GetContext(ctx, &children, `SELECT count(*) FROM pg_inherits WHERE inhparent=$1::regclass`, store.quoted); err != nil {
					t.Fatal(err)
				}
				if err := db.GetContext(ctx, &indexes, `SELECT count(*) FROM pg_inherits i JOIN pg_index x ON x.indexrelid=i.inhrelid WHERE i.inhparent=$1::regclass AND x.indisvalid AND x.indisready`, `"`+name+`"`); err != nil {
					t.Fatal(err)
				}
				if children != indexes || children < 3 {
					t.Fatalf("partitions=%d, valid attached attribute indexes=%d", children, indexes)
				}
				query.Cursor = ""
				query.Matchers[0].Value = "new"
				page, err := later.Query(ctx, query)
				if err != nil || len(page.Records) != 1 || page.Records[0].ID != fresh.ID {
					t.Fatalf("new partition lookup: %+v, %v", page, err)
				}
			}
		})
	}
}

func TestPostgreSQLLogRejectsIncompatibleAttributesIndex(t *testing.T) {
	for _, definition := range []string{
		"USING BTREE ((attributes_json::jsonb))",
		"USING GIN ((attributes_json::jsonb) jsonb_ops)",
		"USING GIN ((attributes_json::jsonb -> 'scope') jsonb_path_ops)",
		"USING GIN ((attributes_json::jsonb) jsonb_path_ops) WHERE stream='eino.history'",
	} {
		t.Run(definition, func(t *testing.T) {
			store, db := newPostgresAttributeLog(t, 0)
			name, err := storage.SQLIndexName(store.table, postgresAttributesIndexPurpose)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`DROP INDEX "` + name + `"`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(fmt.Sprintf(`CREATE INDEX "%s" ON %s %s`, name, store.quoted, definition)); err != nil {
				t.Fatal(err)
			}
			if _, err := NewSQLStoreWithDB(db, store.table.Name()); err == nil {
				t.Fatal("accepted incompatible attribute index")
			}
		})
	}
	t.Run("invalid partition parent", func(t *testing.T) {
		store, db := newPostgresAttributeLog(t, 30*24*time.Hour)
		name, err := storage.SQLIndexName(store.table, postgresAttributesIndexPurpose)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DROP INDEX "` + name + `"`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE INDEX "` + name + `" ON ONLY ` + store.quoted + ` USING GIN ((attributes_json::jsonb) jsonb_path_ops)`); err != nil {
			t.Fatal(err)
		}
		if _, err := NewSQLStoreWithDBAndTTL(db, store.table.Name(), 30*24*time.Hour); err == nil {
			t.Fatal("accepted invalid parent without child attribute indexes")
		}
	})
}

func TestPostgreSQLLogAttributeIndexSelectivity(t *testing.T) {
	store, db := newPostgresAttributeLog(t, 0)
	at := time.Now().UTC().Truncate(time.Millisecond)
	// A shared stream contains mostly unrelated conversations. The target is
	// old, so seeking by stream/time and filtering JSON must inspect the tail.
	_, err := db.ExecContext(t.Context(), `INSERT INTO `+store.quoted+` (stream,id,timestamp_unix_nano,kind,severity,message,attributes_json,payload_json)
		SELECT 'eino.history', n::text, $1::bigint+n, 'message', '', '',
		jsonb_build_object('scope',CASE WHEN n<10 THEN 'target' ELSE 'other-'||n::text END,
		'agent_id','assistant','context_id','conversation')::text, ''::bytea
		FROM generate_series(0,4999) n`, at.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "VACUUM ANALYZE "+store.quoted); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"target", "missing"} {
		query := Query{Start: at.Add(-time.Second), End: at.Add(time.Second), Streams: []string{"eino.history"}, Kinds: []string{"message"}, Limit: 2, Order: OrderDesc, Matchers: []AttributeMatcher{
			{Name: "scope", Op: MatchEqual, Value: scope}, {Name: "agent_id", Op: MatchEqual, Value: "assistant"}, {Name: "context_id", Op: MatchEqual, Value: "conversation"},
		}}
		statement, args, err := store.buildQuery(normalizeSQLQuery(query), nil, time.Now().UnixNano(), query.Limit+1)
		if err != nil {
			t.Fatal(err)
		}
		var data []byte
		if err := db.GetContext(t.Context(), &data, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+db.Rebind(statement), args...); err != nil {
			t.Fatal(err)
		}
		var plans []struct{ Plan attributeQueryPlan }
		if err := json.Unmarshal(data, &plans); err != nil {
			t.Fatal(err)
		}
		name, err := storage.SQLIndexName(store.table, postgresAttributesIndexPurpose)
		if err != nil {
			t.Fatal(err)
		}
		var indexUsed bool
		var scanned float64
		var visit func(attributeQueryPlan)
		visit = func(plan attributeQueryPlan) {
			if plan.IndexName == name {
				indexUsed = true
			}
			if plan.NodeType == "Function Scan" {
				t.Fatal("per-record JSON function scan returned")
			}
			if plan.NodeType == "Index Scan" || plan.NodeType == "Bitmap Heap Scan" || plan.NodeType == "Seq Scan" {
				scanned += (plan.Rows + plan.Removed) * plan.Loops
			}
			for _, child := range plan.Plans {
				visit(child)
			}
		}
		visit(plans[0].Plan)
		if !indexUsed || scanned > 10 {
			t.Fatalf("scope=%s attribute index used=%t, scanned=%g, want at most 10; plan=%s", scope, indexUsed, scanned, data)
		}
	}
}

type attributeQueryPlan struct {
	NodeType  string  `json:"Node Type"`
	IndexName string  `json:"Index Name"`
	Rows      float64 `json:"Actual Rows"`
	Removed   float64 `json:"Rows Removed by Filter"`
	Loops     float64 `json:"Actual Loops"`
	Plans     []attributeQueryPlan
}
