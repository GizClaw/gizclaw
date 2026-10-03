package peerusage

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) (*Store, *sqlx.DB) {
	t.Helper()
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s, err := NewStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	return s, db
}

func TestSnapshotsAreIdempotentAndWritersAreAdditive(t *testing.T) {
	s, db := newTestStore(t)
	hour := time.Now().UTC().Truncate(time.Hour)
	peer := giznet.PublicKey{1}
	first := Snapshot{Peer: peer, ModelID: "model", Hour: hour, WriterID: "one", Quantity: 10}
	if err := s.Write(t.Context(), []Snapshot{first, first}); err != nil {
		t.Fatal(err)
	}
	first.Quantity = 7
	if err := s.Write(t.Context(), []Snapshot{first}); err != nil {
		t.Fatal(err)
	}
	first.Quantity = 15
	if err := s.Write(t.Context(), []Snapshot{first}); err != nil {
		t.Fatal(err)
	}
	second := first
	second.WriterID = "two"
	second.Quantity = 20
	if err := s.Write(t.Context(), []Snapshot{second}); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := reopened.Query(t.Context(), peer, "model", hour, hour.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Quantity != 35 {
		t.Fatalf("rows=%+v", rows)
	}
	other, err := s.Query(t.Context(), giznet.PublicKey{2}, "", hour, hour.Add(time.Hour))
	if err != nil || len(other) != 0 {
		t.Fatalf("other=%+v, %v", other, err)
	}
}

func TestHourlyRetentionAndAtomicValidation(t *testing.T) {
	s, db := newTestStore(t)
	now := time.Now().UTC().Truncate(time.Hour)
	s.Now = func() time.Time { return now }
	peer := giznet.PublicKey{2}
	old := Snapshot{Peer: peer, ModelID: "a", Hour: now.Add(-Retention), WriterID: "a", Quantity: 3}
	newer := old
	newer.Hour = now
	newer.ModelID = "b"
	newer.Quantity = 4
	if err := s.Write(t.Context(), []Snapshot{old, newer}); err != nil {
		t.Fatal(err)
	}
	invalid := newer
	invalid.Quantity = -1
	if err := s.Write(t.Context(), []Snapshot{newer, invalid}); err == nil {
		t.Fatal("negative snapshot accepted")
	}
	now = now.Add(time.Hour)
	rows, err := s.Query(t.Context(), peer, "", now.Add(-Retention-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ModelID != "b" {
		t.Fatalf("rows=%+v", rows)
	}
	if err := s.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.GetContext(t.Context(), &count, "SELECT COUNT(*) FROM "+TableName); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count=%d", count)
	}
	bad := newer
	bad.Hour = now.Add(time.Minute)
	if err := s.Write(t.Context(), []Snapshot{bad}); err == nil {
		t.Fatal("non-hour timestamp accepted")
	}
}

func TestStorePersistsAcrossDatabaseReopening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	db := sqlx.MustOpen("sqlite", path)
	db.SetMaxOpenConns(1)
	s, err := NewStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Truncate(time.Hour)
	peer := giznet.PublicKey{3}
	if err := s.Write(t.Context(), []Snapshot{{Peer: peer, ModelID: "m", Hour: hour, WriterID: "epoch", Quantity: 42}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = sqlx.MustOpen("sqlite", path)
	db.SetMaxOpenConns(1)
	defer db.Close()
	s, err = NewStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.Query(t.Context(), peer, "", hour, hour.Add(time.Hour))
	if err != nil || len(rows) != 1 || rows[0].Quantity != 42 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}

func TestConcurrentSnapshotsNeverRegress(t *testing.T) {
	s, _ := newTestStore(t)
	hour := time.Now().UTC().Truncate(time.Hour)
	peer := giznet.PublicKey{4}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			err := s.Write(t.Context(), []Snapshot{{Peer: peer, ModelID: "m", Hour: hour, WriterID: "same", Quantity: int64(i + 1)}})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	rows, err := s.Query(t.Context(), peer, "", hour, hour.Add(time.Hour))
	if err != nil || len(rows) != 1 || rows[0].Quantity != 20 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
}
