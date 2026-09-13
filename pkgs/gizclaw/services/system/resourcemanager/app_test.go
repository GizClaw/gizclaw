package resourcemanager

import (
	"archive/tar"
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/app"
)

func TestAppResourceLifecycle(t *testing.T) {
	var archive bytes.Buffer
	zw := zlib.NewWriter(&archive)
	tw := tar.NewWriter(zw)
	files := map[string]string{"app.json": `{"app_name":"clock","runtime":"runtime.lua.gizos","entry":"main.lua","methods":[]}`, "main.lua": "return {}"}
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	data := archive.Bytes()
	digest := sha256.Sum256(data)
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer source.Close()
	apps := &app.Server{DB: toolkittest.New(t).DB, HTTP: source.Client()}
	if err := apps.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	manager := New(Services{Apps: apps})
	item := apitypes.AppResource{ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1, Kind: apitypes.AppResourceKindApp, Metadata: apitypes.ResourceMetadata{Id: "clock-package"}, Spec: apitypes.AppSpec{Package: apitypes.FirmwarePackage{Url: source.URL, Sha256: hex.EncodeToString(digest[:]), Size: int64(len(data))}}}
	resource, err := marshalResource(item)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Apply(t.Context(), resource)
	if err != nil || result.Action != apitypes.ApplyActionCreated {
		t.Fatalf("apply: %v %v", result, err)
	}
	result, err = manager.Apply(t.Context(), resource)
	if err != nil || result.Action != apitypes.ApplyActionUnchanged {
		t.Fatalf("repeat: %v %v", result, err)
	}
	value, err := apps.Get(t.Context(), item.Metadata.Id)
	if err != nil || value.AppName != "clock" || value.Runtime != "runtime.lua.gizos" {
		t.Fatalf("manifest: %v %v", value, err)
	}
	if _, err := manager.Get(t.Context(), apitypes.ResourceKindApp, item.Metadata.Id); err != nil {
		t.Fatal(err)
	}
	item.Spec.Package.Url = source.URL + "/clock.tar.zlib"
	resource, err = marshalResource(item)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Put(t.Context(), resource); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Delete(t.Context(), apitypes.ResourceKindApp, item.Metadata.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(t.Context(), apitypes.ResourceKindApp, item.Metadata.Id); err == nil {
		t.Fatal("deleted App remains available")
	}
}

func TestAppResourceRejectsDisplayMetadata(t *testing.T) {
	spec := map[string]any{"package": map[string]any{"url": "https://example.com/clock.tar.zlib", "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "size": 123}}
	resource := map[string]any{"apiVersion": "gizclaw.admin/v1alpha1", "kind": "App", "metadata": map[string]any{"id": "clock"}, "spec": spec}
	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	if err := apitypes.ValidateResourceJSON(data); err != nil {
		t.Fatalf("package-only App: %v", err)
	}
	spec["description"] = "Clock"
	data, err = json.Marshal(resource)
	if err != nil {
		t.Fatal(err)
	}
	if err := apitypes.ValidateResourceJSON(data); err == nil {
		t.Fatal("App accepted display metadata")
	}
}
