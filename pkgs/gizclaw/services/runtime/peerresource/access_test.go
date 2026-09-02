package peerresource

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
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
		Id:   "device",
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &models}},
	}
	server := &Server{RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
	got := server.profileNames(profileModels)
	models["a"] = apitypes.RuntimeProfileBinding{ResourceId: "changed"}
	if !reflect.DeepEqual(got, []string{"a", "b", "duplicate"}) {
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
	workspaces := workspaceNameService{names: map[string]string{
		"profile-a-workspace-id": "  profile-a-workspace  ",
		"profile-b-workspace-id": "profile-b-workspace",
		"empty-workspace-id":     " ",
	}}
	runtime := &gameplay.Runtime{DB: db, Workspaces: workspaces}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	caller := giznet.PublicKey{1}
	now := time.Date(2026, 7, 19, 7, 45, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for _, profileName := range []string{"profile-a", "profile-b"} {
		workspaceID := profileName + "-workspace-id"
		_, err := db.ExecContext(ctx, `INSERT INTO gameplay_pets (owner_public_key, id, name, runtime_profile_id, pet_def_id, display_name, workspace_id, stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			caller.String(), profileName+"-pet", profileName+"-pet", profileName, "petdef-basic", profileName, workspaceID, `{"life":100,"health":100,"satiety":100,"hygiene":100,"mood":100,"energy":100}`, `{"experience":0,"level":1}`, "alive", nil, now, now, now, now)
		if err != nil {
			t.Fatalf("insert pet for %s: %v", profileName, err)
		}
	}
	_, err = db.ExecContext(ctx, `INSERT INTO gameplay_pets (owner_public_key, id, name, runtime_profile_id, pet_def_id, display_name, workspace_id, stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		caller.String(), "profile-a-empty-pet", "profile-a-empty-pet", "profile-a", "petdef-basic", "empty", "empty-workspace-id", `{"life":100,"health":100,"satiety":100,"hygiene":100,"mood":100,"energy":100}`, `{"experience":0,"level":1}`, "alive", nil, now, now, now, now)
	if err != nil {
		t.Fatalf("insert pet with empty workspace: %v", err)
	}
	profileCtx := gameplay.WithRuntimeProfile(ctx, apitypes.RuntimeProfile{Id: "profile-a"})
	if _, err := runtime.DeletePet(profileCtx, caller.String(), "profile-a-pet"); err != nil {
		t.Fatalf("DeletePet(profile-a) error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM gameplay_pending_deletions WHERE kind = 'pet' AND owner_public_key = ? AND resource_id = ?`, caller.String(), "profile-a-pet"); err != nil {
		t.Fatalf("simulate completed pending cleanup: %v", err)
	}
	profile := apitypes.RuntimeProfile{Id: "profile-a"}
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

type workspaceNameService struct {
	names map[string]string
}

func (workspaceNameService) CreateSystemWorkspace(context.Context, adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	return apitypes.Workspace{}, false, errors.New("unexpected CreateSystemWorkspace")
}

func (workspaceNameService) DeleteSystemWorkspace(context.Context, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, errors.New("unexpected DeleteSystemWorkspace")
}

func (s workspaceNameService) GetWorkspace(_ context.Context, request adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	name, ok := s.names[request.Id]
	if !ok {
		return adminhttp.GetWorkspace404JSONResponse(apitypes.NewErrorResponse("WORKSPACE_NOT_FOUND", "not found")), nil
	}
	return adminhttp.GetWorkspace200JSONResponse(apitypes.Workspace{Id: request.Id, Name: name}), nil
}

func (workspaceNameService) GetWorkspaceByName(context.Context, string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, errors.New("unexpected GetWorkspaceByName")
}

type deviceReadInfoStub struct {
	info    map[giznet.PublicKey]apitypes.DeviceInfo
	runtime map[giznet.PublicKey]apitypes.Runtime
	calls   []giznet.PublicKey
}

func (s *deviceReadInfoStub) GetSelfInfo(_ context.Context, key giznet.PublicKey) (apitypes.DeviceInfo, error) {
	s.calls = append(s.calls, key)
	info, ok := s.info[key]
	if !ok {
		return apitypes.DeviceInfo{}, peer.ErrPeerNotFound
	}
	return info, nil
}

func (s *deviceReadInfoStub) GetSelfRuntime(_ context.Context, key giznet.PublicKey) apitypes.Runtime {
	s.calls = append(s.calls, key)
	return s.runtime[key]
}

type deviceReadStatusStub struct {
	status map[giznet.PublicKey]apitypes.PeerStatus
}

func (s *deviceReadStatusStub) GetStatus(_ context.Context, key giznet.PublicKey) (apitypes.PeerStatus, error) {
	return s.status[key], nil
}

func TestDeviceReadsArePinnedToCaller(t *testing.T) {
	owner := giznet.PublicKey{7}
	other := giznet.PublicKey{8}
	info := &deviceReadInfoStub{
		info:    map[giznet.PublicKey]apitypes.DeviceInfo{owner: {Name: new("owner")}, other: {Name: new("other")}},
		runtime: map[giznet.PublicKey]apitypes.Runtime{other: {Online: true}},
	}
	status := &deviceReadStatusStub{status: map[giznet.PublicKey]apitypes.PeerStatus{owner: {Volume: new(30)}, other: {Volume: new(90)}}}
	reads := DeviceReads{Caller: owner, Info: info, Status: status}

	got, err := reads.DeviceInfo(context.Background())
	if err != nil || got.Name == nil || *got.Name != "owner" {
		t.Fatalf("DeviceInfo = %+v, %v", got, err)
	}
	runtime, err := reads.DeviceRuntime(context.Background())
	if err != nil || runtime.Online {
		t.Fatalf("DeviceRuntime = %+v, %v; must not read another Peer", runtime, err)
	}
	peerStatus, err := reads.DeviceStatus(context.Background())
	if err != nil || peerStatus.Volume == nil || *peerStatus.Volume != 30 {
		t.Fatalf("DeviceStatus = %+v, %v", peerStatus, err)
	}
	for _, call := range info.calls {
		if call != owner {
			t.Fatalf("device read used Peer %v, want caller %v", call, owner)
		}
	}
}

func TestDeviceReadsReportMissingServices(t *testing.T) {
	reads := DeviceReads{Caller: giznet.PublicKey{9}}
	if _, err := reads.DeviceInfo(context.Background()); !errors.Is(err, ErrDeviceServiceNotConfigured) {
		t.Fatalf("DeviceInfo error = %v", err)
	}
	if _, err := reads.DeviceRuntime(context.Background()); !errors.Is(err, ErrDeviceServiceNotConfigured) {
		t.Fatalf("DeviceRuntime error = %v", err)
	}
	if _, err := reads.DeviceStatus(context.Background()); !errors.Is(err, ErrDeviceServiceNotConfigured) {
		t.Fatalf("DeviceStatus error = %v", err)
	}
	if _, err := reads.DeviceTelemetryLatest(context.Background(), nil); !errors.Is(err, ErrDeviceServiceNotConfigured) {
		t.Fatalf("DeviceTelemetryLatest error = %v", err)
	}
	if _, err := reads.DeviceTelemetryRange(context.Background(), apitypes.PeerTelemetryFieldBatteryPercent, time.Time{}, time.Time{}, 0, 0, apitypes.PeerTelemetryOrderAsc); !errors.Is(err, ErrDeviceServiceNotConfigured) {
		t.Fatalf("DeviceTelemetryRange error = %v", err)
	}
	if _, err := reads.DeviceTelemetryAggregate(context.Background(), apitypes.PeerTelemetryFieldBatteryPercent, time.Time{}, time.Time{}, 0, apitypes.PeerTelemetryAggregateAvg); !errors.Is(err, ErrDeviceServiceNotConfigured) {
		t.Fatalf("DeviceTelemetryAggregate error = %v", err)
	}
}

func TestDeviceReadsTelemetryUsesCallerPeer(t *testing.T) {
	owner := giznet.PublicKey{10}
	other := giznet.PublicKey{11}
	store := metrics.NewMemoryStore()
	at := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	for key, value := range map[giznet.PublicKey]float64{owner: 41, other: 99} {
		if err := store.Append(context.Background(), []metrics.Sample{{
			Name: peertelemetry.MetricBatteryPercent, Labels: map[string]string{"peer_id": key.String()}, Timestamp: at, Value: value,
		}}); err != nil {
			t.Fatal(err)
		}
	}
	reads := DeviceReads{Caller: owner, Telemetry: &peertelemetry.AdminService{Metrics: store, Now: func() time.Time { return at.Add(time.Second) }}}
	latest, err := reads.DeviceTelemetryLatest(context.Background(), []apitypes.PeerTelemetryField{apitypes.PeerTelemetryFieldBatteryPercent})
	if err != nil {
		t.Fatal(err)
	}
	if latest.PeerPublicKey != owner.String() || len(latest.Values) != 1 || latest.Values[0].Value != 41 {
		t.Fatalf("latest = %+v", latest)
	}
}

func TestDeviceReadsRangeAndAggregateUseCallerPeer(t *testing.T) {
	owner := giznet.PublicKey{12}
	other := giznet.PublicKey{13}
	store := metrics.NewMemoryStore()
	at := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	for key, value := range map[giznet.PublicKey]float64{owner: 40, other: 90} {
		for i := range 3 {
			if err := store.Append(context.Background(), []metrics.Sample{{
				Name: peertelemetry.MetricBatteryPercent, Labels: map[string]string{"peer_id": key.String()},
				Timestamp: at.Add(time.Duration(i) * time.Minute), Value: value + float64(i),
			}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	reads := DeviceReads{Caller: owner, Telemetry: &peertelemetry.AdminService{Metrics: store, Now: func() time.Time { return at.Add(time.Hour) }}}
	rng, err := reads.DeviceTelemetryRange(context.Background(), apitypes.PeerTelemetryFieldBatteryPercent, at, at.Add(2*time.Minute), time.Minute, 10, apitypes.PeerTelemetryOrderDesc)
	if err != nil {
		t.Fatal(err)
	}
	if rng.PeerPublicKey != owner.String() || len(rng.Points) != 3 || rng.Points[0].Value != 42 || rng.Points[2].Value != 40 {
		t.Fatalf("range = %+v", rng)
	}
	agg, err := reads.DeviceTelemetryAggregate(context.Background(), apitypes.PeerTelemetryFieldBatteryPercent, at, at.Add(3*time.Minute), 3*time.Minute, apitypes.PeerTelemetryAggregateMax)
	if err != nil {
		t.Fatal(err)
	}
	if agg.PeerPublicKey != owner.String() || len(agg.Points) == 0 || agg.Points[0].Value != 42 {
		t.Fatalf("aggregate = %+v", agg)
	}
	if _, err := reads.DeviceTelemetryRange(context.Background(), apitypes.PeerTelemetryFieldBatteryPercent, at.Add(time.Hour), at, 0, 0, apitypes.PeerTelemetryOrderAsc); !errors.Is(err, peertelemetry.ErrInvalidQuery) {
		t.Fatalf("inverted range error = %v", err)
	}
}
