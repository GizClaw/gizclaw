package resourcemanager

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestToolResourceLifecycleUsesCallerID(t *testing.T) {
	tools := &toolkit.Server{Store: kv.NewMemory(nil)}
	manager := New(Services{Tools: tools})
	resource := mustResource(t, `{
		"apiVersion":"gizclaw.admin/v1alpha1",
		"kind":"Tool",
		"metadata":{"id":"volume_set"},
		"spec":{
			"type":"client_rpc",
			"invoke_name":"volume_set",
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
	if err != nil || created.Action != apitypes.ApplyActionCreated || created.Id == nil || *created.Id != "volume_set" {
		t.Fatalf("Apply(create) = %#v, %v", created, err)
	}
	id := *created.Id
	got, err := manager.Get(t.Context(), apitypes.ResourceKindTool, id)
	if err != nil {
		t.Fatal(err)
	}
	typed, err := got.AsToolResource()
	if err != nil {
		t.Fatal(err)
	}
	if metadataID(t, typed.Metadata) != "volume_set" {
		t.Fatalf("metadata.id = %q", metadataID(t, typed.Metadata))
	}
	if discriminator, err := typed.Spec.Discriminator(); err != nil || discriminator != "client_rpc" {
		t.Fatalf("spec type = %q, %v", discriminator, err)
	}
	deleted, err := manager.Delete(t.Context(), apitypes.ResourceKindTool, id)
	if err != nil {
		t.Fatal(err)
	}
	deletedTool, err := deleted.AsToolResource()
	if err != nil || metadataID(t, deletedTool.Metadata) != "volume_set" {
		t.Fatalf("Delete() = %#v, %v", deletedTool, err)
	}
	if _, err := manager.Get(t.Context(), apitypes.ResourceKindTool, id); err == nil {
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
			"metadata":{"id":"weather"},
			"spec":{
				"type":"http_request",
				"invoke_name":"weather",
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
	toolID := *created.Id
	assertToolSecretRedacted(t, manager, toolID)
	unchanged, err := manager.Apply(t.Context(), withResourceID(t, resourceWithSecret(""), toolID))
	if err != nil || unchanged.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("Apply(retain) = %#v, %v", unchanged, err)
	}
	stored, err := tools.GetToolByID(t.Context(), toolID)
	if err != nil || stored.HTTP.Auth.BearerToken == nil || *stored.HTTP.Auth.BearerToken != "first" {
		t.Fatalf("retained secret = %#v, %v", stored.HTTP, err)
	}
	rotated, err := manager.Apply(t.Context(), withResourceID(t, resourceWithSecret(`,"bearer_token":"second"`), toolID))
	if err != nil || rotated.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("Apply(rotate) = %#v, %v", rotated, err)
	}
	stored, err = tools.GetToolByID(context.Background(), toolID)
	if err != nil || stored.HTTP.Auth.BearerToken == nil || *stored.HTTP.Auth.BearerToken != "second" {
		t.Fatalf("rotated secret = %#v, %v", stored.HTTP, err)
	}
	assertToolSecretRedacted(t, manager, toolID)
}

func TestToolResourceIdentityConflictsReturnConflict(t *testing.T) {
	t.Parallel()
	tools := &toolkit.Server{Store: kv.NewMemory(nil)}
	manager := New(Services{Tools: tools})
	resource := func(id, invokeName string) apitypes.Resource {
		return mustResource(t, `{
			"apiVersion":"gizclaw.admin/v1alpha1",
			"kind":"Tool",
			"metadata":{"id":"`+id+`"},
			"spec":{
				"type":"client_rpc",
				"invoke_name":"`+invokeName+`",
				"input_schema":{"type":"object"}
			}
		}`)
	}
	if _, err := manager.Apply(t.Context(), resource("volume-tool", "volume_set")); err != nil {
		t.Fatalf("Apply(create) error = %v", err)
	}

	_, err := manager.Apply(t.Context(), resource("volume-tool", "volume_get"))
	assertResourceError(t, err, 409, "TOOL_CONFLICT")

	_, err = manager.Apply(t.Context(), resource("other-tool", "volume_set"))
	assertResourceError(t, err, 409, "TOOL_CONFLICT")
}

func assertToolSecretRedacted(t *testing.T, manager *Manager, id string) {
	t.Helper()
	got, err := manager.Get(t.Context(), apitypes.ResourceKindTool, id)
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
