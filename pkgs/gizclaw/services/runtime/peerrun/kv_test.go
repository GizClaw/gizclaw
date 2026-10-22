package peerrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
)

func TestKVRestartOrderingAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	open := func() *Server {
		db, err := sqlx.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		s := &Server{DB: db}
		if err = s.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := open()
	peer := testPublicKey(t)
	other := giznet.PublicKey{7}
	at := time.UnixMilli(10000).UTC()
	values := []string{`{"nested":[null,false,0,{"x":"ok"}]}`, `null`, `false`, `0`, `[1,2]`, `"text"`, `1e400`}
	for i, value := range values {
		if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: fmt.Sprint(i), Value: json.RawMessage(value), ObservedAt: at}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.PutStatus(t.Context(), peer, apitypes.PeerStatus{Volume: new(50), ReportedKv: &apitypes.PeerReportedKV{"injected": {Value: json.RawMessage(`true`), ObservedAt: at}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "1", ObservedAt: at.Add(time.Second)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s = open()
	t.Cleanup(func() { _ = s.DB.Close() })
	// Reconnection does not introduce a new sequence epoch or discard tombstones.
	for _, ts := range []time.Time{at, at.Add(time.Second)} {
		if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "1", Value: json.RawMessage(`"late"`), ObservedAt: ts}, {Key: "0", Value: json.RawMessage(`"replay"`), ObservedAt: at}}); err != nil {
			t.Fatal(err)
		}
	}
	status, err := s.GetStatus(t.Context(), peer)
	if err != nil {
		t.Fatal(err)
	}
	if status.ReportedKv == nil || len(*status.ReportedKv) != len(values)-1 {
		t.Fatalf("snapshot missing after restart: %+v", status)
	}
	for i, value := range values {
		if i == 1 {
			continue
		}
		entry, ok := (*status.ReportedKv)[fmt.Sprint(i)]
		if !ok || entry.Value == nil || string(entry.Value) != value || !entry.ObservedAt.Equal(at) {
			t.Fatalf("value/type/time lost for %d: %+v", i, entry)
		}
	}
	if _, ok := (*status.ReportedKv)["1"]; ok {
		t.Fatal("late report resurrected deleted key")
	}
	if status.Volume == nil || *status.Volume != 50 {
		t.Fatal("KV overwrote control state")
	}
	isolated, err := s.GetStatus(t.Context(), other)
	if err != nil || isolated.ReportedKv != nil {
		t.Fatal("cross-device state leakage")
	}
	if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "1", Value: json.RawMessage(`null`), ObservedAt: at.Add(2 * time.Second)}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.GetReportedKV(t.Context(), peer)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := (*snapshot)["1"]
	if !ok || entry.Value == nil || string(entry.Value) != "null" {
		t.Fatal("new null is not a present value")
	}
}

func TestKVAtomicLimitsAndOverwrite(t *testing.T) {
	s := newTestServer(t)
	peer := testPublicKey(t)
	at := time.Unix(20, 0).UTC()
	for i := range telemetrypb.MaxKVStoredKeys {
		if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: fmt.Sprint(i), Value: json.RawMessage(`0`), ObservedAt: at}}); err != nil {
			t.Fatal(err)
		}
	}
	// Replacing a known key at capacity does not charge another key.
	if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "0", Value: json.RawMessage(`false`), ObservedAt: at.Add(time.Second)}}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.GetReportedKV(t.Context(), peer)
	err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "0", Value: json.RawMessage(`true`), ObservedAt: at.Add(2 * time.Second)}, {Key: "overflow", Value: json.RawMessage(`null`), ObservedAt: at.Add(2 * time.Second)}})
	if !errors.Is(err, telemetrypb.ErrInvalidKV) {
		t.Fatalf("key cap: %v", err)
	}
	after, _ := s.GetReportedKV(t.Context(), peer)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("invalid batch partially replaced valid state")
	}
	if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "0", ObservedAt: at.Add(3 * time.Second)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "new-key", Value: json.RawMessage(`0`), ObservedAt: at.Add(4 * time.Second)}}); !errors.Is(err, telemetrypb.ErrInvalidKV) {
		t.Fatal("tombstone escaped storage budget")
	}
	// Complete encoded state (keys, JSON, timestamps) is bounded, atomically.
	bigPeer := giznet.PublicKey{8}
	value := json.RawMessage(`"` + strings.Repeat("x", 1022) + `"`)
	count := 0
	for i := range 64 {
		err := s.PutReportedKV(t.Context(), bigPeer, []KVUpdate{{Key: fmt.Sprint(i), Value: value, ObservedAt: at}})
		if errors.Is(err, telemetrypb.ErrInvalidKV) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
	}
	if count == 0 || count >= 64 {
		t.Fatal("byte cap was not reached")
	}
	var stored string
	if err := s.DB.GetContext(t.Context(), &stored, `SELECT state_json FROM peer_reported_kv WHERE public_key=?`, bigPeer.String()); err != nil || len(stored) > telemetrypb.MaxKVStoredBytes {
		t.Fatalf("stored byte cap %d: %v", len(stored), err)
	}
}

func TestKVConcurrentReportsKeepLatest(t *testing.T) {
	s := newTestServer(t)
	peer := testPublicKey(t)
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "x", Value: json.RawMessage(fmt.Sprint(i)), ObservedAt: time.UnixMilli(1000 + int64(i)).UTC()}})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	got, err := s.GetReportedKV(t.Context(), peer)
	if err != nil || got == nil {
		t.Fatal(err)
	}
	if string((*got)["x"].Value) != "11" {
		t.Fatal("concurrent report replaced a newer value")
	}
}

func TestKVEmptyEncodingCannotDelete(t *testing.T) {
	s := newTestServer(t)
	peer := testPublicKey(t)
	if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "x", Value: json.RawMessage(`null`), ObservedAt: time.Unix(1, 0)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutReportedKV(t.Context(), peer, []KVUpdate{{Key: "x", Value: json.RawMessage{}, ObservedAt: time.Unix(2, 0)}}); !errors.Is(err, telemetrypb.ErrInvalidKV) {
		t.Fatal("empty encoding accepted as deletion")
	}
	state, err := s.GetReportedKV(t.Context(), peer)
	if err != nil || state == nil || string((*state)["x"].Value) != "null" {
		t.Fatal("invalid update deleted a value")
	}
}
