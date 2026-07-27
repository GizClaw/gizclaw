package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestToolResourceLifecycleUsesMetadataName(t *testing.T) {
	tools := &toolkit.Server{Store: kv.NewMemory(nil)}
	manager := New(Services{Tools: tools})
	resource := mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Tool",
		"metadata":{"name":"volume_set"},
		"spec":{
			"type":"client_rpc",
			"description":"Set the current device volume",
			"input_schema":{
				"type":"object",
				"required":["level"],
				"properties":{"level":{"type":"integer","minimum":0,"maximum":10}},
				"additionalProperties":false
			}
		}
	}`)
	created, err := manager.Apply(t.Context(), resource)
	if err != nil || created.Action != apitypes.ApplyActionCreated || created.Name != "volume_set" {
		t.Fatalf("Apply(create) = %#v, %v", created, err)
	}
	got, err := manager.Get(t.Context(), apitypes.ResourceKindTool, "volume_set")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := got.AsToolResource()
	if err != nil {
		t.Fatal(err)
	}
	if typed.Metadata.Name != "volume_set" {
		t.Fatalf("metadata.name = %q", typed.Metadata.Name)
	}
	if discriminator, err := typed.Spec.Discriminator(); err != nil || discriminator != "client_rpc" {
		t.Fatalf("spec type = %q, %v", discriminator, err)
	}
	deleted, err := manager.Delete(t.Context(), apitypes.ResourceKindTool, "volume_set")
	if err != nil {
		t.Fatal(err)
	}
	deletedTool, err := deleted.AsToolResource()
	if err != nil || deletedTool.Metadata.Name != "volume_set" {
		t.Fatalf("Delete() = %#v, %v", deletedTool, err)
	}
	if _, err := manager.Get(t.Context(), apitypes.ResourceKindTool, "volume_set"); err == nil {
		t.Fatal("Get(deleted Tool) succeeded")
	}
}

func TestToolResourceDirectSecretIsWriteOnlyRetainedAndRotated(t *testing.T) {
	tools := &toolkit.Server{Store: kv.NewMemory(nil)}
	manager := New(Services{Tools: tools})
	resourceWithSecret := func(secretField string) apitypes.Resource {
		return mustResource(t, `{
			"apiVersion":"gizclaw.admin/v1alpha1",
			"kind":"Tool",
			"metadata":{"name":"weather"},
			"spec":{
				"type":"http_request",
				"input_schema":{"type":"object"},
				"http":{
					"url":"https://weather.example/v1",
					"method":"GET",
					"auth":{"method":"bearer"`+secretField+`},
					"timeout":"3s",
					"max_response_bytes":4096
				}
			}
		}`)
	}
	created, err := manager.Apply(t.Context(), resourceWithSecret(`,"bearer_token":"first"`))
	if err != nil || created.Action != apitypes.ApplyActionCreated {
		t.Fatalf("Apply(create) = %#v, %v", created, err)
	}
	assertToolSecretRedacted(t, manager)
	unchanged, err := manager.Apply(t.Context(), resourceWithSecret(""))
	if err != nil || unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(retain) = %#v, %v", unchanged, err)
	}
	stored, err := tools.GetTool(t.Context(), "weather")
	if err != nil || stored.HTTP.Auth.BearerToken == nil || *stored.HTTP.Auth.BearerToken != "first" {
		t.Fatalf("retained secret = %#v, %v", stored.HTTP, err)
	}
	rotated, err := manager.Apply(t.Context(), resourceWithSecret(`,"bearer_token":"second"`))
	if err != nil || rotated.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(rotate) = %#v, %v", rotated, err)
	}
	stored, err = tools.GetTool(context.Background(), "weather")
	if err != nil || stored.HTTP.Auth.BearerToken == nil || *stored.HTTP.Auth.BearerToken != "second" {
		t.Fatalf("rotated secret = %#v, %v", stored.HTTP, err)
	}
	assertToolSecretRedacted(t, manager)
}

func assertToolSecretRedacted(t *testing.T, manager *Manager) {
	t.Helper()
	got, err := manager.Get(t.Context(), apitypes.ResourceKindTool, "weather")
	if err != nil {
		t.Fatal(err)
	}
	typed, err := got.AsToolResource()
	if err != nil {
		t.Fatal(err)
	}
	httpSpec, err := typed.Spec.AsHTTPToolSpec()
	if err != nil {
		t.Fatal(err)
	}
	auth, err := httpSpec.Http.Auth.AsToolHTTPAuthBearer()
	if err != nil {
		t.Fatal(err)
	}
	if auth.BearerToken != nil {
		t.Fatalf("read response leaked bearer_token = %q", *auth.BearerToken)
	}
}
