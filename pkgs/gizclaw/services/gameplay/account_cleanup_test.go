package gameplay

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestPeerRetirementWithoutGameplayDatabase(t *testing.T) {
	adapter := PeerRetirement{}
	snapshot, err := adapter.SnapshotPeerGameplay(t.Context(), "peer-a")
	if err != nil || snapshot.PublicKey != "peer-a" || len(snapshot.Pets) != 0 {
		t.Fatalf("SnapshotPeerGameplay() = %#v, %v", snapshot, err)
	}
	ready, err := adapter.RetirePeerGameplay(t.Context(), snapshot)
	if err != nil || !ready {
		t.Fatalf("RetirePeerGameplay() = %v, %v", ready, err)
	}
	if _, err := adapter.SnapshotPeerGameplay(t.Context(), " peer-a"); err == nil {
		t.Fatal("SnapshotPeerGameplay() accepted non-canonical public key")
	}
	ready, err = adapter.RetirePeerGameplay(t.Context(), PeerGameplaySnapshot{PublicKey: "peer-a", Pets: []PeerGameplayPet{{ID: "pet-a"}}})
	if err == nil || ready {
		t.Fatalf("RetirePeerGameplay(Pets without DB) = %v, %v", ready, err)
	}
}

func TestPeerGameplayRetirementWaitsForPetThenPurgesAccount(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	adopted, err := runtime.AdoptPet(ctx, "peer-a", apitypes.PetAdoptRequest{Name: "pet-main", DisplayName: "Pet"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.SnapshotPeerGameplay(ctx, "peer-a")
	if err != nil || len(snapshot.Pets) != 1 || snapshot.Pets[0].ID != adopted.Pet.Id {
		t.Fatalf("SnapshotPeerGameplay() = %#v, %v", snapshot, err)
	}
	ready, err := runtime.RetirePeerGameplay(ctx, snapshot)
	if err != nil || ready {
		t.Fatalf("RetirePeerGameplay(first) = %v, %v; want deferred", ready, err)
	}
	source := PendingDeletionSource{DB: runtime.DB}
	refs, _, err := source.ScanDue(ctx, now.Add(time.Second), 10, "")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ScanDue() = %#v, %v", refs, err)
	}
	claim, claimed, err := source.Claim(ctx, refs[0], now.Add(time.Second), time.Minute)
	if err != nil || !claimed {
		t.Fatalf("Claim() = %#v, %v, %v", claim, claimed, err)
	}
	if err := (PetDeletionHandler{DB: runtime.DB, Now: func() time.Time { return now.Add(time.Second) }}).Handle(ctx, claim); err != nil {
		t.Fatal(err)
	}
	ready, err = runtime.RetirePeerGameplay(ctx, snapshot)
	if err != nil || !ready {
		t.Fatalf("RetirePeerGameplay(final) = %v, %v", ready, err)
	}
	count, err := peerGameplayRowCount(ctx, runtime.DB, "peer-a")
	if err != nil || count != 0 {
		t.Fatalf("Peer Gameplay rows = %d, %v", count, err)
	}
}

func TestPeerGameplayPurgeCoversInventoryAndPreservesForeignRows(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	seedPeerGameplayRows(t, runtime, "peer-a", "a", *now)
	seedPeerGameplayRows(t, runtime, "peer-b", "b", *now)
	if err := runtime.purgePeerGameplay(ctx, "peer-a"); err != nil {
		t.Fatal(err)
	}
	if count, err := peerGameplayRowCount(ctx, runtime.DB, "peer-a"); err != nil || count != 0 {
		t.Fatalf("target row count = %d, %v", count, err)
	}
	if count, err := peerGameplayRowCount(ctx, runtime.DB, "peer-b"); err != nil || count != 10 {
		t.Fatalf("foreign row count = %d, %v; want 10", count, err)
	}
	var sources int
	if err := runtime.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_workspace_reward_sources`).Scan(&sources); err != nil || sources != 2 {
		t.Fatalf("Workspace reward sources = %d, %v; want preserved", sources, err)
	}
}

func TestPeerGameplayPurgeRollsBackAllTables(t *testing.T) {
	ctx, runtime, now := newPetRuntime(t)
	if err := runtime.Migration(ctx); err != nil {
		t.Fatal(err)
	}
	seedPeerGameplayRows(t, runtime, "peer-a", "a", *now)
	if _, err := runtime.DB.ExecContext(ctx, `CREATE TRIGGER fail_account_purge
		BEFORE DELETE ON gameplay_points_accounts
		WHEN OLD.owner_public_key = 'peer-a'
		BEGIN SELECT RAISE(ABORT, 'forced purge failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := runtime.purgePeerGameplay(ctx, "peer-a"); err == nil {
		t.Fatal("purgePeerGameplay() error = nil")
	}
	if count, err := peerGameplayRowCount(ctx, runtime.DB, "peer-a"); err != nil || count != 10 {
		t.Fatalf("row count after rollback = %d, %v; want 10", count, err)
	}
}

func seedPeerGameplayRows(t *testing.T, runtime *Runtime, owner, suffix string, now time.Time) {
	t.Helper()
	ctx := t.Context()
	stamp := formatTime(now)
	workspaceID := "workspace-" + suffix
	petID := "pet-" + suffix
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO gameplay_pet_adoption_reservations (owner_public_key, pet_id, name, runtime_profile_id, pet_def_id, display_name, workspace_name, workflow_id, adoption_cost, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`, []any{owner, petID, "name-" + suffix, "profile-" + suffix, "petdef-" + suffix, "Pet", workspaceID, "workflow-" + suffix, stamp}},
		{`INSERT INTO gameplay_pet_workspace_bindings (owner_public_key, pet_id, runtime_profile_id, workspace_id, created_at) VALUES (?, ?, ?, ?, ?)`, []any{owner, petID, "profile-" + suffix, workspaceID, stamp}},
		{`INSERT INTO gameplay_pet_drive_ticks (owner_public_key, runtime_profile_id, idempotency_key, pet_id, created_at) VALUES (?, ?, ?, ?, ?)`, []any{owner, "profile-" + suffix, "tick-" + suffix, petID, stamp}},
		{`INSERT INTO gameplay_points_accounts (owner_public_key, runtime_profile_id, balance, created_at, updated_at) VALUES (?, ?, 1, ?, ?)`, []any{owner, "profile-" + suffix, stamp, stamp}},
		{`INSERT INTO gameplay_points_transactions (owner_public_key, id, runtime_profile_id, delta, balance_after, reason, created_at) VALUES (?, ?, ?, 1, 1, 'test', ?)`, []any{owner, "transaction-" + suffix, "profile-" + suffix, stamp}},
		{`INSERT INTO gameplay_badges (owner_public_key, id, badge_def_id, exp, level, active, progress, created_at, updated_at) VALUES (?, ?, ?, 1, 1, 1, 1, ?, ?)`, []any{owner, "badge-" + suffix, "badgedef-" + suffix, stamp, stamp}},
		{`INSERT INTO gameplay_game_results (owner_public_key, id, runtime_profile_id, pet_id, game_def_id, occurred_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, []any{owner, "result-" + suffix, "profile-" + suffix, petID, "gamedef-" + suffix, stamp, stamp}},
		{`INSERT INTO gameplay_reward_grants (owner_public_key, id, runtime_profile_id, points_delta, pet_exp_delta, badge_exp_delta_json, created_at) VALUES (?, ?, ?, 1, 1, '{}', ?)`, []any{owner, "grant-" + suffix, "profile-" + suffix, stamp}},
		{`INSERT INTO gameplay_workspace_reward_windows (id, workspace_id, workspace_kind, beneficiary_public_key, runtime_profile_id, runtime_profile_revision, policy_json, policy_digest, start_history_id, high_water_history_id, start_history_at, high_water_history_at, opened_at, last_activity_at, evaluate_after, state, attempt_count, next_attempt_at, claim_token, claim_until, transcript_digest, outcome, last_error, created_at, updated_at) VALUES (?, ?, 'workflow', ?, ?, 'revision', '{}', 'digest', 'start', 'end', ?, ?, ?, ?, ?, 'completed', 0, ?, '', '', '', '', '', ?, ?)`, []any{"window-" + suffix, workspaceID, owner, "profile-" + suffix, stamp, stamp, stamp, stamp, stamp, stamp, stamp, stamp}},
		{`INSERT INTO gameplay_workspace_reward_sources (workspace_id, scheduled_checkpoint, completed_checkpoint, created_at, updated_at) VALUES (?, '', '', ?, ?)`, []any{workspaceID, stamp, stamp}},
	}
	for _, statement := range statements {
		if _, err := runtime.DB.ExecContext(ctx, runtime.DB.Rebind(statement.query), statement.args...); err != nil {
			t.Fatalf("seed %s: %v", statement.query, err)
		}
	}
	tx, err := runtime.DB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := insertDriveFactOutbox(ctx, tx, driveFactOutbox{
		ObservationID: "observation-" + suffix, PayloadDigest: "digest-" + suffix,
		OwnerPublicKey: owner, RuntimeProfile: "profile-" + suffix, PetID: petID,
		Target:  DriveFactTarget{WorkspaceID: workspaceID, ProfileID: "profile-" + suffix, ProfileRevision: "revision", BindingName: "memory", BindingIdentity: "binding"},
		Payload: driveFactPayload{ID: "fact-" + suffix, Text: "fact", ObservedAt: now}, State: driveFactPending,
		NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
