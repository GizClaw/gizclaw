package resourcemanager

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestResourceAssetBindingsFollowOwnerCommit(t *testing.T) {
	ctx := context.Background()
	workflows := newFakeWorkflows()
	manager := New(Services{Workflows: workflows})
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := assets.RegisterOwnerResolver(asset.OwnerKindResource, manager); err != nil {
		t.Fatal(err)
	}
	manager.services.Assets = assets
	first := putResourceAsset(t, assets, "first")
	second := putResourceAsset(t, assets, "second")

	if _, err := manager.Apply(ctx, workflowResourceWithAsset(t, "asset-workflow", first)); err != nil {
		t.Fatalf("Apply(first) error = %v", err)
	}
	assertResourceAssetBinding(t, assets, first, 1)

	if _, err := manager.Apply(ctx, workflowResourceWithAsset(t, "asset-workflow", second)); err != nil {
		t.Fatalf("Apply(second) error = %v", err)
	}
	assertResourceAssetBinding(t, assets, first, 0)
	assertResourceAssetBinding(t, assets, second, 1)

	if _, err := manager.Delete(ctx, apitypes.ResourceKindWorkflow, "asset-workflow"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	assertResourceAssetBinding(t, assets, second, 0)
}

func TestResourceAssetBindingRollsBackFailedOwnerWrite(t *testing.T) {
	ctx := context.Background()
	workflows := newFakeWorkflows()
	manager := New(Services{Workflows: workflows})
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := assets.RegisterOwnerResolver(asset.OwnerKindResource, manager); err != nil {
		t.Fatal(err)
	}
	manager.services.Assets = assets
	ref := putResourceAsset(t, assets, "rollback")
	workflows.putStatus = 500
	if _, err := manager.Apply(ctx, workflowResourceWithAsset(t, "asset-workflow", ref)); err == nil {
		t.Fatal("Apply() error = nil")
	}
	assertResourceAssetBinding(t, assets, ref, 0)
}

func TestResourceAssetBindingRetryReactivatesCommittedRef(t *testing.T) {
	ctx := context.Background()
	workflows := newFakeWorkflows()
	manager := New(Services{Workflows: workflows})
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := assets.RegisterOwnerResolver(asset.OwnerKindResource, manager); err != nil {
		t.Fatal(err)
	}
	manager.services.Assets = assets
	ref := putResourceAsset(t, assets, "retry")
	resource := workflowResourceWithAsset(t, "asset-workflow", ref)
	if _, err := manager.Apply(ctx, resource); err != nil {
		t.Fatalf("Apply(initial) error = %v", err)
	}
	owner := asset.Owner{Kind: asset.OwnerKindResource, ID: "Workflow/asset-workflow"}
	if err := assets.Protect(ctx, ref, asset.Binding{Owner: owner}); err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	if live, err := assets.LiveBindings(ctx, ref); err != nil || len(live) != 0 {
		t.Fatalf("LiveBindings(pending) = %#v, %v", live, err)
	}
	if _, err := manager.Apply(ctx, resource); err != nil {
		t.Fatalf("Apply(retry) error = %v", err)
	}
	if live, err := assets.LiveBindings(ctx, ref); err != nil || len(live) != 1 || live[0].Owner != owner {
		t.Fatalf("LiveBindings(reactivated) = %#v, %v", live, err)
	}
}

func TestResourceAssetBindingsAcceptGeneratedDiscriminator(t *testing.T) {
	ctx := context.Background()
	workflows := newFakeWorkflows()
	manager := New(Services{Workflows: workflows})
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := assets.RegisterOwnerResolver(asset.OwnerKindResource, manager); err != nil {
		t.Fatal(err)
	}
	manager.services.Assets = assets
	ref := putResourceAsset(t, assets, "generated")
	workflow, err := workflowResourceWithAsset(t, "generated-workflow", ref).AsWorkflowResource()
	if err != nil {
		t.Fatal(err)
	}
	var resource apitypes.Resource
	if err := resource.FromWorkflowResource(workflow); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Apply(ctx, resource); err != nil {
		t.Fatalf("Apply(generated discriminator) error = %v", err)
	}
	assertResourceAssetBinding(t, assets, ref, 1)
}

func putResourceAsset(t *testing.T, assets *asset.Service, body string) asset.Ref {
	t.Helper()
	stored, err := assets.Put(context.Background(), asset.PutRequest{
		MediaType: "application/octet-stream",
		MaxBytes:  1024,
	}, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	return stored.Metadata.Ref
}

func workflowResourceWithAsset(t *testing.T, name string, ref asset.Ref) apitypes.Resource {
	t.Helper()
	return mustResource(t, fmt.Sprintf(`{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Workflow",
		"metadata":{"name":%q},
		"i18n":{"default_locale":"en","en":{"description":%q}},
		"spec":{"driver":"flowcraft","flowcraft":{"prompt":"asset test"}}
	}`, name, ref.String()))
}

func assertResourceAssetBinding(t *testing.T, assets *asset.Service, ref asset.Ref, want int) {
	t.Helper()
	bindings, err := assets.Bindings(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != want {
		t.Fatalf("Bindings(%s) = %#v, want %d", ref, bindings, want)
	}
}
