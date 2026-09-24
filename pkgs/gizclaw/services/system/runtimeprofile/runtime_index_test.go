package runtimeprofile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	runtimeindex "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/runtimeprofile"
	"github.com/jmoiron/sqlx"
)

type failingProfileSource struct {
	db   *sqlx.DB
	fail bool
}

func (source *failingProfileSource) ForEachProfile(ctx context.Context, consume func(apitypes.RuntimeProfile) error) error {
	if source.fail {
		return errors.New("source temporarily unavailable")
	}
	return profileSource{source.db}.ForEachProfile(ctx, consume)
}

func TestCommittedProfileWritesReturnSuccessWhenIndexRefreshFails(t *testing.T) {
	ctx := t.Context()
	s := &Server{DB: profileSQLTestDB(t)}
	source := &failingProfileSource{db: s.DB}
	s.runtimeIndex = runtimeindex.New(source, time.Hour)
	if err := s.runtimeIndex.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	source.fail = true
	request := adminhttp.RuntimeProfileUpsert{Id: "durable", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{}}}
	created, err := s.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{Body: &request})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := created.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("committed create = %#v", created)
	}
	if _, err := s.ResolveProfile(ctx, "durable"); err != nil {
		t.Fatalf("committed profile is absent: %v", err)
	}
	request.Spec.AppConfig = new(apitypes.RuntimeProfileAppConfig{"theme": "dark"})
	updated, err := s.PutRuntimeProfile(ctx, adminhttp.PutRuntimeProfileRequestObject{Id: "durable", Body: &request})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("committed update = %#v", updated)
	}
	profile, err := s.ResolveProfile(ctx, "durable")
	if err != nil || profile.Spec.AppConfig == nil || (*profile.Spec.AppConfig)["theme"] != "dark" {
		t.Fatalf("committed profile update = %#v, %v", profile, err)
	}
	deleted, err := s.DeleteRuntimeProfile(ctx, adminhttp.DeleteRuntimeProfileRequestObject{Id: "durable"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := deleted.(adminhttp.DeleteRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("committed delete = %#v", deleted)
	}
	if _, err := s.ResolveProfile(ctx, "durable"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted profile still resolves: %v", err)
	}
}

func TestMemoryIndexDecomposesAllProfilesAndFiltersTags(t *testing.T) {
	ctx := context.Background()
	s := &Server{DB: profileSQLTestDB(t)}
	if err := s.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, id := range []string{"alpha", "beta"} {
		binding := runtimeProfileTestBinding(id + "-workflow")
		binding.Tags = &[]string{"6-8", "stories"}
		models := map[string]apitypes.RuntimeProfileBinding{"chat": runtimeProfileTestBinding(id + "-model")}
		voices := map[string]apitypes.RuntimeProfileBinding{"narrator": runtimeProfileTestBinding(id + "-voice")}
		config := apitypes.RuntimeProfileAppConfig{"theme": "dark"}
		response, err := s.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
			Id: id, Spec: apitypes.RuntimeProfileSpec{
				Workflows: apitypes.RuntimeProfileWorkflows{"journey": binding},
				Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices},
				AppConfig: &config,
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
			t.Fatalf("create %s = %#v", id, response)
		}
	}
	for _, kind := range []string{"workflow", "model", "voice", "app_config"} {
		entries, err := s.RuntimeIndex().ListEntries(ctx, kind)
		if err != nil || len(entries) != 2 || entries[0].RuntimeProfileID != "alpha" || entries[1].RuntimeProfileID != "beta" {
			t.Fatalf("%s entries = %#v, %v", kind, entries, err)
		}
	}
	if entry, err := s.RuntimeIndex().GetEntry(ctx, "alpha", "workflow", "journey"); err != nil || entry.RuntimeProfileID != "alpha" || entry.Name != "journey" {
		t.Fatalf("exact entry = %#v, %v", entry, err)
	}
	if ids, err := s.RuntimeIndex().ListProfileIDs(ctx); err != nil || !slices.Equal(ids, []string{"alpha", "beta"}) {
		t.Fatalf("indexed profile IDs = %#v, %v", ids, err)
	}
	previousGeneration := s.runtimeIndex.Generation()
	matched, err := s.RuntimeIndex().ListWorkflowsByTags(ctx, []string{"stories", "6-8"})
	if err != nil || len(matched) != 2 {
		t.Fatalf("AND matches = %#v, %v", matched, err)
	}
	alpha, err := s.ResolveProfile(ctx, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	profileMatches, err := s.RuntimeIndex().ListProfileWorkflowsByTags(ctx, "alpha", alpha.Revision, []string{"stories", "6-8"})
	if err != nil || len(profileMatches) != 1 || profileMatches[0].RuntimeProfileID != "alpha" {
		t.Fatalf("profile AND matches = %#v, %v", profileMatches, err)
	}
	for _, entry := range matched {
		var binding apitypes.RuntimeProfileBinding
		if err := json.Unmarshal(entry.Value, &binding); err != nil || binding.ResourceId != entry.RuntimeProfileID+"-workflow" {
			t.Fatalf("indexed binding = %#v, %v", binding, err)
		}
	}
	if matched, err := s.RuntimeIndex().ListWorkflowsByTags(ctx, []string{"stories", "9-12"}); err != nil || len(matched) != 0 {
		t.Fatalf("nonmatching AND = %#v, %v", matched, err)
	}
	beta, err := s.ResolveProfile(ctx, "beta")
	if err != nil {
		t.Fatal(err)
	}
	binding := beta.Spec.Workflows["journey"]
	binding.Tags = &[]string{"9-12", "stories"}
	beta.Spec.Workflows["journey"] = binding
	updated, err := s.PutRuntimeProfile(ctx, adminhttp.PutRuntimeProfileRequestObject{Id: "beta", Body: &adminhttp.RuntimeProfileUpsert{Id: "beta", Spec: beta.Spec}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("update = %#v", updated)
	}
	if s.runtimeIndex.Generation() <= previousGeneration {
		t.Fatal("profile update did not rotate the SQLite instance")
	}
	matched, err = s.RuntimeIndex().ListWorkflowsByTags(ctx, []string{"stories", "6-8"})
	if err != nil || len(matched) != 1 || matched[0].RuntimeProfileID != "alpha" {
		t.Fatalf("after tag update = %#v, %v", matched, err)
	}
	response, err := s.DeleteRuntimeProfile(ctx, adminhttp.DeleteRuntimeProfileRequestObject{Id: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.DeleteRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("delete = %#v", response)
	}
	matched, err = s.RuntimeIndex().ListWorkflowsByTags(ctx, nil)
	if err != nil || len(matched) != 1 || matched[0].RuntimeProfileID != "beta" {
		t.Fatalf("after delete = %#v, %v", matched, err)
	}
	if ids, err := s.RuntimeIndex().ListProfileIDs(ctx); err != nil || !slices.Equal(ids, []string{"beta"}) {
		t.Fatalf("profile IDs after delete = %#v, %v", ids, err)
	}
}

func TestMemoryIndexRotatesOnInterval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s := &Server{DB: profileSQLTestDB(t), indexRefreshInterval: 10 * time.Millisecond}
	if err := s.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	item := apitypes.RuntimeProfile{
		Id: "external", Revision: "rev", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"chat": runtimeProfileTestBinding("chat")}},
	}
	if created, err := insertRuntimeProfileSQL(ctx, s.DB, item); err != nil || !created {
		t.Fatalf("external insert = %v, %v", created, err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		entries, err := s.RuntimeIndex().ListEntries(ctx, "workflow")
		if err == nil && len(entries) == 1 && entries[0].RuntimeProfileID == "external" {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("interval refresh did not publish the external profile: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestMemoryIndexRefreshesExternalProfileWrites(t *testing.T) {
	ctx := context.Background()
	s := &Server{DB: profileSQLTestDB(t)}
	if err := s.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	item := apitypes.RuntimeProfile{
		Id: "external", Revision: "rev", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"chat": runtimeProfileTestBinding("chat")}},
	}
	if created, err := insertRuntimeProfileSQL(ctx, s.DB, item); err != nil || !created {
		t.Fatalf("external insert = %v, %v", created, err)
	}
	if entries, err := s.RuntimeIndex().ListEntries(ctx, "workflow"); err != nil || len(entries) != 0 {
		t.Fatalf("before refresh = %#v, %v", entries, err)
	}
	if err := s.RefreshMemoryIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if entries, err := s.RuntimeIndex().ListEntries(ctx, "workflow"); err != nil || len(entries) != 1 || entries[0].RuntimeProfileID != "external" {
		t.Fatalf("after refresh = %#v, %v", entries, err)
	}
	stored, version, err := getRuntimeProfileSQL(ctx, s.DB, "external")
	if err != nil {
		t.Fatal(err)
	}
	binding := stored.Spec.Workflows["chat"]
	binding.Tags = &[]string{"new"}
	stored.Spec.Workflows["chat"] = binding
	stored.Revision = "rev2"
	if _, _, err := updateRuntimeProfileSQL(ctx, s.DB, stored, version); err != nil {
		t.Fatal(err)
	}
	entries, err := s.RuntimeIndex().ListProfileWorkflowsByTags(ctx, "external", "rev2", []string{"new"})
	if err != nil || len(entries) != 1 || entries[0].Name != "chat" {
		t.Fatalf("revision-triggered refresh = %#v, %v", entries, err)
	}
}

func TestMemoryIndexSplitsEveryConfigurationKind(t *testing.T) {
	ctx := context.Background()
	s := &Server{DB: profileSQLTestDB(t)}
	if err := s.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	tools := map[string]apitypes.RuntimeProfileBinding{"echo": runtimeProfileTestBinding("echo-tool")}
	var connection apitypes.RuntimeProfileMemoryConnection
	if err := connection.FromRuntimeProfileFlowcraftObjectStoreConnection(apitypes.RuntimeProfileFlowcraftObjectStoreConnection{
		Directory: "/tmp/memory", Type: apitypes.RuntimeProfileFlowcraftObjectStoreConnectionTypeFlowcraftObjectStore,
	}); err != nil {
		t.Fatal(err)
	}
	memories := map[string]apitypes.RuntimeProfileMemoryBinding{"history": {LayoutId: "layout", Driver: apitypes.RuntimeProfileMemoryDriverFlowcraft, Connection: connection}}
	profile := apitypes.RuntimeProfile{Id: "all-kinds", Revision: "rev", Spec: apitypes.RuntimeProfileSpec{
		Resources:    apitypes.RuntimeProfileResources{Tools: &tools, Memories: &memories},
		SafetyFences: &apitypes.RuntimeProfileSafetyFences{General: &apitypes.RuntimeProfileSafetyFence{Prompt: "safe"}},
		Mhs:          &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "display", Kind: "display", States: []apitypes.MhsV0State{}}}}},
	}}
	profile.CreatedAt, profile.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if created, err := insertRuntimeProfileSQL(ctx, s.DB, profile); err != nil || !created {
		t.Fatalf("insert source profile = %v, %v", created, err)
	}
	if err := s.RefreshMemoryIndex(ctx); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"tool", "memory", "safety_fence", "mhs.v0.device"} {
		entries, err := s.RuntimeIndex().ListEntries(ctx, kind)
		if err != nil || len(entries) != 1 || entries[0].RuntimeProfileID != "all-kinds" {
			t.Fatalf("%s = %#v, %v", kind, entries, err)
		}
	}
}

func TestMemoryIndexRefreshAndWritesMakeProgress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s := &Server{DB: profileSQLTestDB(t)}
	if err := s.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	initial := apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"chat": runtimeProfileTestBinding("chat")}}
	if response, err := s.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{Id: "shared", Spec: initial}}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("create = %#v", response)
	}
	var group sync.WaitGroup
	errors := make(chan error, 3)
	group.Add(3)
	go func() {
		defer group.Done()
		for range 10 {
			if err := s.RefreshMemoryIndex(ctx); err != nil {
				errors <- err
				return
			}
		}
	}()
	go func() {
		defer group.Done()
		for i := range 10 {
			spec := apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"chat": runtimeProfileTestBinding("chat")}}
			binding := spec.Workflows["chat"]
			binding.Tags = &[]string{fmt.Sprintf("tag-%d", i)}
			spec.Workflows["chat"] = binding
			response, err := s.PutRuntimeProfile(ctx, adminhttp.PutRuntimeProfileRequestObject{Id: "shared", Body: &adminhttp.RuntimeProfileUpsert{Id: "shared", Spec: spec}})
			if err != nil {
				errors <- err
				return
			}
			if _, ok := response.(adminhttp.PutRuntimeProfile200JSONResponse); !ok {
				errors <- fmt.Errorf("put = %#v", response)
				return
			}
		}
	}()
	go func() {
		defer group.Done()
		for range 10 {
			if _, err := s.RuntimeIndex().ListWorkflowsByTags(ctx, nil); err != nil {
				errors <- err
				return
			}
		}
	}()
	group.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	entries, err := s.RuntimeIndex().ListWorkflowsByTags(ctx, []string{"tag-9"})
	if err != nil || len(entries) != 1 {
		t.Fatalf("final tag = %#v, %v", entries, err)
	}
}

func TestMemoryIndexRemainsReadableWithoutPersistentDB(t *testing.T) {
	ctx := context.Background()
	s := &Server{DB: profileSQLTestDB(t)}
	if err := s.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	response, err := s.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Id: "cached", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"chat": runtimeProfileTestBinding("chat")}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("create = %#v", response)
	}
	if err := s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := s.RuntimeIndex().ListEntries(ctx, "workflow")
	if err != nil || len(entries) != 1 || entries[0].RuntimeProfileID != "cached" {
		t.Fatalf("memory-only read = %#v, %v", entries, err)
	}
}
