package peerresource

import (
	"context"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
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
	models := map[string]string{"a": "profile-a", "b": "profile-b", "duplicate": "profile-a", "empty": " "}
	profile := apitypes.RuntimeProfile{
		Name: "device",
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &models}},
	}
	server := &Server{RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
	got := server.profileNames(profileModels)
	models["a"] = "changed"
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

func TestPageWorkspacesUsesEffectiveOrder(t *testing.T) {
	items := []apitypes.Workspace{{Name: "profile-a"}, {Name: "profile-b"}, {Name: "owner-a"}}
	limit := 2
	page, hasNext, cursor := pageWorkspaces(items, nil, &limit)
	if !reflect.DeepEqual(page, items[:2]) || !hasNext || cursor == nil || *cursor != "profile-b" {
		t.Fatalf("first page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
	}
	page, hasNext, cursor = pageWorkspaces(items, cursor, &limit)
	if !reflect.DeepEqual(page, items[2:]) || hasNext || cursor != nil {
		t.Fatalf("second page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
	}
}

func TestPageWorkflowsUsesProfileOrder(t *testing.T) {
	items := []rpcapi.Workflow{{Name: "profile-a"}, {Name: "profile-b"}, {Name: "profile-c"}}
	limit := 1
	page, hasNext, cursor := pageWorkflows(items, nil, &limit)
	if !reflect.DeepEqual(page, items[:1]) || !hasNext || cursor == nil || *cursor != "profile-a" {
		t.Fatalf("first page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
	}
	page, hasNext, cursor = pageWorkflows(items, cursor, &limit)
	if !reflect.DeepEqual(page, items[1:2]) || !hasNext || cursor == nil || *cursor != "profile-b" {
		t.Fatalf("second page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
	}
	page, hasNext, cursor = pageWorkflows(items, cursor, &limit)
	if !reflect.DeepEqual(page, items[2:]) || hasNext || cursor != nil {
		t.Fatalf("third page = %#v, hasNext=%v cursor=%v", page, hasNext, cursor)
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

func TestVoiceMatchesListParams(t *testing.T) {
	item := apitypes.Voice{
		Source: apitypes.VoiceSourceManual,
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKindOpenaiTenant,
			Name: "primary",
		},
	}
	source := adminhttp.VoiceSource(apitypes.VoiceSourceManual)
	kind := adminhttp.VoiceProviderKind(apitypes.VoiceProviderKindOpenaiTenant)
	name := "primary"
	if !voiceMatchesListParams(item, adminhttp.ListVoicesParams{Source: &source, ProviderKind: &kind, ProviderName: &name}) {
		t.Fatal("voiceMatchesListParams() rejected matching voice")
	}
	otherName := "other"
	if voiceMatchesListParams(item, adminhttp.ListVoicesParams{ProviderName: &otherName}) {
		t.Fatal("voiceMatchesListParams() accepted mismatched provider name")
	}
	otherSource := adminhttp.VoiceSource(apitypes.VoiceSourceSync)
	if voiceMatchesListParams(item, adminhttp.ListVoicesParams{Source: &otherSource}) {
		t.Fatal("voiceMatchesListParams() accepted mismatched source")
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
