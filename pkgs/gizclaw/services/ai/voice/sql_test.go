package voice

import (
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/sqltest"
)

func syncTestVoice(index int) apitypes.Voice {
	kind := apitypes.VoiceProviderKindMinimaxTenant
	return apitypes.Voice{Id: fmt.Sprintf("voice-%04d", index), Source: apitypes.VoiceSourceSync, Provider: apitypes.VoiceProvider{Kind: kind, Id: "tenant"}, ProviderData: ProviderData(kind, map[string]any{"voice_id": fmt.Sprintf("upstream-%04d", index)}), CreatedAt: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)}
}

func TestSQLVoiceSyncUsesBatchesWithDatabaseLatency(t *testing.T) {
	db, observer := sqltest.New(t)
	s := &Server{DB: db}
	ctx := t.Context()
	if err := s.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	desired := make([]apitypes.Voice, 1000)
	for i := range desired {
		desired[i] = syncTestVoice(i)
	}
	observer.Delay.Store(int64(5 * time.Millisecond))
	observer.Reset()
	created, updated, deleted, err := s.ReconcileProviderVoices(ctx, apitypes.VoiceProviderKindMinimaxTenant, "tenant", desired)
	if err != nil || created != 1000 || updated != 0 || deleted != 0 {
		t.Fatalf("sync counts=%d/%d/%d error=%v", created, updated, deleted, err)
	}
	// Two provider-lock statements, one scoped read, sixteen batches and one cleanup.
	if got := observer.Statements.Load(); got != 20 {
		t.Fatalf("SQL statements=%d, want 20 for 1000 voices", got)
	}
	if got := observer.Rows.Load(); got != 0 {
		t.Fatalf("initial sync read %d rows", got)
	}
	observer.Reset()
	created, updated, deleted, err = s.ReconcileProviderVoices(ctx, apitypes.VoiceProviderKindMinimaxTenant, "tenant", desired[:500])
	if err != nil || created != 0 || updated != 0 || deleted != 500 {
		t.Fatalf("resync counts=%d/%d/%d error=%v", created, updated, deleted, err)
	}
	if got := observer.Statements.Load(); got != 12 {
		t.Fatalf("resync SQL statements=%d, want 12", got)
	}
	if got := observer.Rows.Load(); got != 1000 {
		t.Fatalf("resync rows=%d, want exactly the provider's 1000 rows", got)
	}
	observer.Reset()
	if err := s.DeleteProviderVoices(ctx, apitypes.VoiceProviderKindMinimaxTenant, "tenant"); err != nil {
		t.Fatal(err)
	}
	if observer.Statements.Load() != 3 || observer.Rows.Load() != 0 {
		t.Fatalf("provider deletion statements=%d rows=%d", observer.Statements.Load(), observer.Rows.Load())
	}
}

func TestSQLVoiceSyncRollsBackEarlierBatchesOnCollision(t *testing.T) {
	s := &Server{DB: newTestDB(t)}
	ctx := t.Context()
	kind := apitypes.VoiceProviderKindMinimaxTenant
	original := syncTestVoice(0)
	if _, _, _, err := s.ReconcileProviderVoices(ctx, kind, "tenant", []apitypes.Voice{original}); err != nil {
		t.Fatal(err)
	}
	manual := syncTestVoice(129)
	manual.Source = apitypes.VoiceSourceManual
	if err := Write(ctx, s.DB, manual, nil); err != nil {
		t.Fatal(err)
	}
	desired := make([]apitypes.Voice, 130)
	for i := range desired {
		desired[i] = syncTestVoice(i)
	}
	desired[0].DisplayName = new("changed")
	if _, _, _, err := s.ReconcileProviderVoices(ctx, kind, "tenant", desired); err == nil {
		t.Fatal("sync replaced a manual record")
	}
	items, err := ListProvider(ctx, s.DB, kind, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("failed sync leaked writes: %d records", len(items))
	}
	saved, err := Get(ctx, s.DB, original.Id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DisplayName != nil {
		t.Fatal("failed sync retained an earlier batch update")
	}
	kept, err := Get(ctx, s.DB, manual.Id)
	if err != nil {
		t.Fatal(err)
	}
	if kept.Source != apitypes.VoiceSourceManual {
		t.Fatal("failed sync changed manual voice")
	}
}
