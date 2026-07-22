package peerresource

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestOrderedUniqueKeepsProfileBeforeOwner(t *testing.T) {
	got := orderedUnique(
		[]string{"profile-a", "shared", "missing", "profile-a"},
		[]string{"owner-a", "shared", "owner-b"},
	)
	want := []string{"profile-a", "shared", "missing", "owner-a", "owner-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orderedUnique() = %#v, want %#v", got, want)
	}
}

func TestProfileNamesUsesImmutableSnapshotAndUnregisteredHasNone(t *testing.T) {
	models := map[string]apitypes.RuntimeProfileBinding{
		"a": {ResourceId: "profile-a"}, "b": {ResourceId: "profile-b"},
		"duplicate": {ResourceId: "profile-a"}, "empty": {ResourceId: " "},
	}
	profile := apitypes.RuntimeProfile{
		Name: "device",
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &models}},
	}
	server := &Server{RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
	got := server.profileNames(profileModels)
	models["a"] = apitypes.RuntimeProfileBinding{ResourceId: "changed"}
	if !reflect.DeepEqual(got, []string{"profile-a", "profile-b"}) {
		t.Fatalf("profileNames() = %#v", got)
	}
	if got := (&Server{}).profileNames(profileModels); got != nil {
		t.Fatalf("unregistered profileNames() = %#v, want nil", got)
	}
}

func TestPageModelsUsesEffectiveOrder(t *testing.T) {
	items := []apitypes.Model{{Id: "profile-a"}, {Id: "profile-b"}, {Id: "owner-a"}}
	limit := 2
	page, hasNext, cursor := pageModels(items, nil, &limit)
	if !reflect.DeepEqual(page, items[:2]) || !hasNext || cursor == nil || *cursor != "profile-b" {
		t.Fatalf("first page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
	}
	page, hasNext, cursor = pageModels(items, cursor, &limit)
	if !reflect.DeepEqual(page, items[2:]) || hasNext || cursor != nil {
		t.Fatalf("second page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
	}
}

func TestPageAliasesBindsCursorToRuntimeProfileRevision(t *testing.T) {
	aliases := []string{"profile-a", "profile-b", "profile-c"}
	limit := 1
	page, hasNext, cursor, conflict := pageAliases(aliases, nil, &limit, "revision-1")
	if !reflect.DeepEqual(page, aliases[:1]) || !hasNext || cursor == nil || conflict {
		t.Fatalf("first page = %#v, hasNext=%v cursor=%v conflict=%v", page, hasNext, cursor, conflict)
	}
	page, hasNext, nextCursor, conflict := pageAliases(aliases, cursor, &limit, "revision-1")
	if !reflect.DeepEqual(page, aliases[1:2]) || !hasNext || nextCursor == nil || conflict {
		t.Fatalf("second page = %#v, hasNext=%v cursor=%v conflict=%v", page, hasNext, nextCursor, conflict)
	}
	if page, _, _, conflict := pageAliases(aliases, cursor, &limit, "revision-2"); len(page) != 0 || !conflict {
		t.Fatalf("stale cursor page = %#v, conflict=%v", page, conflict)
	}
}

func TestPageVoicesUsesProfileOrder(t *testing.T) {
	items := []apitypes.Voice{{Id: "profile-a"}, {Id: "profile-b"}, {Id: "profile-c"}}
	limit := 2
	page, hasNext, cursor := pageVoices(items, nil, &limit)
	if !reflect.DeepEqual(page, items[:2]) || !hasNext || cursor == nil || *cursor != "profile-b" {
		t.Fatalf("first page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
	}
	page, hasNext, cursor = pageVoices(items, cursor, &limit)
	if !reflect.DeepEqual(page, items[2:]) || hasNext || cursor != nil {
		t.Fatalf("second page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
	}
}

func TestDomainWorkspaceNamesSkipsGameplayWithoutDatabase(t *testing.T) {
	server := &Server{Gameplay: &gameplay.Runtime{}}
	names, err := server.domainWorkspaceNames(context.Background())
	if err != nil {
		t.Fatalf("domainWorkspaceNames() error = %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("domainWorkspaceNames() = %#v, want empty", names)
	}
}

func TestDomainWorkspaceNamesRetainsDeletedPetWorkspaceWithinRuntimeProfile(t *testing.T) {
	ctx := context.Background()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	runtime := &gameplay.Runtime{DB: db}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	caller := giznet.PublicKey{1}
	now := time.Date(2026, 7, 19, 7, 45, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for _, profileName := range []string{"profile-a", "profile-b"} {
		_, err := db.ExecContext(ctx, `INSERT INTO gameplay_pets (owner_public_key, id, runtime_profile_name, petdef_id, display_name, workspace_name, stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			caller.String(), profileName+"-pet", profileName, "petdef-basic", profileName, profileName+"-workspace", `{"life":100,"health":100,"satiety":100,"hygiene":100,"mood":100,"energy":100}`, `{"experience":0,"level":1}`, "alive", nil, now, now, now, now)
		if err != nil {
			t.Fatalf("insert pet for %s: %v", profileName, err)
		}
	}
	profileCtx := gameplay.WithRuntimeProfile(ctx, apitypes.RuntimeProfile{Name: "profile-a"})
	if _, err := runtime.DeletePet(profileCtx, caller.String(), "profile-a-pet"); err != nil {
		t.Fatalf("DeletePet(profile-a) error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM gameplay_pending_deletions WHERE kind = 'pet' AND owner_public_key = ? AND resource_id = ?`, caller.String(), "profile-a-pet"); err != nil {
		t.Fatalf("simulate completed pending cleanup: %v", err)
	}
	profile := apitypes.RuntimeProfile{Name: "profile-a"}
	server := &Server{
		Caller:   caller,
		Gameplay: runtime,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
	names, err := server.domainWorkspaceNames(ctx)
	if err != nil {
		t.Fatalf("domainWorkspaceNames() error = %v", err)
	}
	if !reflect.DeepEqual(names, []string{"profile-a-workspace"}) {
		t.Fatalf("domainWorkspaceNames() = %#v", names)
	}
}
