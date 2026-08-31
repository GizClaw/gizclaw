package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectRuntimeStorePrepareWorkspaceCreatesLocalDir(t *testing.T) {
	root := t.TempDir()
	store := newTestRuntimeStore(t, newTestObjectStoreAt(t, root))

	rt, err := store.PrepareWorkspace(context.Background(), "demo ws")
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	wantPrefix := ObjectPrefix("demo ws")
	if rt.ObjectPrefix != wantPrefix {
		t.Fatalf("ObjectPrefix = %q, want escaped workspace prefix", rt.ObjectPrefix)
	}
	wantDir := filepath.Join(root, filepath.FromSlash(wantPrefix))
	if rt.LocalDir != wantDir {
		t.Fatalf("LocalDir = %q, want %q", rt.LocalDir, wantDir)
	}
	if info, err := os.Stat(wantDir); err != nil || !info.IsDir() {
		t.Fatalf("workspace dir not created: info=%v err=%v", info, err)
	}
}

func TestObjectPrefixIsolatesOpaqueWorkspaceIDs(t *testing.T) {
	for _, id := range []string{".", "..", "team", "team/blue", "a:b"} {
		prefix := ObjectPrefix(id)
		if prefix == "workspaces" || prefix == "workspaces/." || prefix == "workspaces/.." {
			t.Fatalf("ObjectPrefix(%q) = %q", id, prefix)
		}
	}
	if ObjectPrefix("team") == ObjectPrefix("team/blue") {
		t.Fatal("distinct workspace IDs share an object prefix")
	}
}

func TestObjectRuntimeStoreIgnoresLegacyRuntimeMetadata(t *testing.T) {
	root := t.TempDir()
	objects := newTestObjectStoreAt(t, root)
	store := newTestRuntimeStore(t, objects)
	prefix := ObjectPrefix("demo")
	if err := objects.Put(prefix+"/runtime.json", strings.NewReader(`{not-json`)); err != nil {
		t.Fatalf("Put legacy runtime metadata: %v", err)
	}

	if _, err := store.PrepareWorkspace(context.Background(), "demo"); err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	if _, err := store.GetWorkspaceRuntime(context.Background(), "demo"); err != nil {
		t.Fatalf("GetWorkspaceRuntime() error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(prefix), "runtime.json"))
	if err != nil {
		t.Fatalf("read legacy runtime metadata: %v", err)
	}
	if got := string(data); got != `{not-json` {
		t.Fatalf("legacy runtime metadata = %q, want unchanged", got)
	}
}

func TestObjectRuntimeStoreDoesNotCreateRuntimeMetadata(t *testing.T) {
	root := t.TempDir()
	store := newTestRuntimeStore(t, newTestObjectStoreAt(t, root))

	if _, err := store.PrepareWorkspace(context.Background(), "demo"); err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	metadata := filepath.Join(root, filepath.FromSlash(ObjectPrefix("demo")), "runtime.json")
	if _, err := os.Stat(metadata); !os.IsNotExist(err) {
		t.Fatalf("runtime metadata after prepare err = %v, want not exist", err)
	}
}

func TestObjectRuntimeStoreKeepsHistoryAssetsSeparate(t *testing.T) {
	runtimeObjects := newTestObjectStore(t)
	historyAssets := newTestObjectStore(t)
	store := NewObjectRuntimeStore(runtimeObjects, newTestLogStore(t), historyAssets)
	runtime, err := store.GetWorkspaceRuntime(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := runtime.History.Append(context.Background(), AppendHistoryRequest{
		Type: "agent", Name: "agent", Text: "hello",
		Asset: &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Assets) != 1 {
		t.Fatalf("history assets = %#v", entry.Assets)
	}
	if items, err := runtimeObjects.List(""); err != nil || len(items) != 0 {
		t.Fatalf("runtime ObjectStore items = %#v, %v, want none", items, err)
	}
	items, err := historyAssets.List("")
	if err != nil || len(items) != 1 || items[0].Name != entry.Assets[0].Name {
		t.Fatalf("history asset ObjectStore items = %#v, %v", items, err)
	}
}

func TestObjectRuntimeStoreDeleteWorkspaceRuntimeRemovesPrefix(t *testing.T) {
	root := t.TempDir()
	objects := newTestObjectStoreAt(t, root)
	store := newTestRuntimeStore(t, objects)

	prefix := ObjectPrefix("demo")
	if err := objects.Put(prefix+"/history/item.json", strings.NewReader("{}")); err != nil {
		t.Fatalf("Put history: %v", err)
	}
	runtime, err := store.GetWorkspaceRuntime(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.History.Append(context.Background(), AppendHistoryRequest{Type: "agent", Name: "agent", Text: "hello"}); err != nil {
		t.Fatalf("Append history: %v", err)
	}
	if absent, err := store.WorkspaceRuntimeAbsent(context.Background(), "demo"); err != nil || absent {
		t.Fatalf("WorkspaceRuntimeAbsent() before delete = %v, %v", absent, err)
	}
	if err := store.DeleteWorkspaceRuntime(context.Background(), "demo"); err != nil {
		t.Fatalf("DeleteWorkspaceRuntime() error = %v", err)
	}
	if absent, err := store.WorkspaceRuntimeAbsent(context.Background(), "demo"); err != nil || !absent {
		t.Fatalf("WorkspaceRuntimeAbsent() after delete = %v, %v", absent, err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(prefix))); !os.IsNotExist(err) {
		t.Fatalf("workspace dir after delete err = %v, want not exist", err)
	}
}

func TestObjectRuntimeStoreValidation(t *testing.T) {
	if _, err := (ObjectRuntimeStore{}).PrepareWorkspace(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "runtime store") {
		t.Fatalf("PrepareWorkspace(nil store) error = %v", err)
	}

	store := newTestRuntimeStore(t, newTestObjectStore(t))
	if _, err := store.PrepareWorkspace(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("PrepareWorkspace(empty workspace) error = %v", err)
	}
	if err := store.DeleteWorkspaceRuntime(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("DeleteWorkspaceRuntime(empty workspace) error = %v", err)
	}
	if _, err := store.PrepareWorkspace(context.Background(), " demo "); err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("PrepareWorkspace(padded workspace) error = %v", err)
	}
	if err := store.DeleteWorkspaceRuntime(context.Background(), " demo "); err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("DeleteWorkspaceRuntime(padded workspace) error = %v", err)
	}
}
