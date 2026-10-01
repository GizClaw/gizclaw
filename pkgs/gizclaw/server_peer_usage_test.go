package gizclaw

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerusage"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestManagerUsageRecorderRetainsPeerAndBillingModel(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	defer db.Close()
	store, err := peerusage.NewStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{PeerUsage: peerusage.NewRecorder(store)}
	peer := giznet.PublicKey{11}
	callback := m.usageRecorder(peer)
	if callback == nil {
		t.Fatal("missing recorder")
	}
	callback(genx.UsageRecord{Provider: "volc", Model: "provider-resource", Input: 4, CachedInput: 3, Output: 2})
	if err := m.PeerUsage.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Truncate(time.Hour)
	rows, err := store.Query(t.Context(), peer, "provider-resource", hour, hour.Add(time.Hour))
	if err != nil || len(rows) != 1 || rows[0].Quantity != 9 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if (&Manager{}).usageRecorder(peer) != nil {
		t.Fatal("unconfigured manager enabled metering")
	}
}
