package logstore

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSQLLogAttributeIsolationAndPagination(t *testing.T) {
	for _, backend := range []string{"sqlite"} {
		t.Run(backend, func(t *testing.T) {
			store, _ := newSQLiteLog(t)
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
