package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectRuntimeStorePrepareWorkspaceCreatesLocalDir(t *testing.T) {
	root := t.TempDir()
	store := NewObjectRuntimeStore(newTestObjectStoreAt(t, root))

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

func TestObjectRuntimeStorePersistsDialogID(t *testing.T) {
	root := t.TempDir()
	store := NewObjectRuntimeStore(newTestObjectStoreAt(t, root))

	rt, err := store.PrepareWorkspace(context.Background(), "demo")
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	if rt.DialogID == "" {
		t.Fatal("DialogID is empty")
	}

	got, err := store.GetWorkspaceRuntime(context.Background(), "demo")
	if err != nil {
		t.Fatalf("GetWorkspaceRuntime() error = %v", err)
	}
	if got.DialogID != rt.DialogID {
		t.Fatalf("DialogID = %q, want %q", got.DialogID, rt.DialogID)
	}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ObjectPrefix("demo")), "runtime.json"))
	if err != nil {
		t.Fatalf("read runtime metadata: %v", err)
	}
	var metadata runtimeMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("decode runtime metadata: %v", err)
	}
	if metadata.DialogID != rt.DialogID {
		t.Fatalf("metadata DialogID = %q, want %q", metadata.DialogID, rt.DialogID)
	}
}

func TestObjectRuntimeStoreDeleteWorkspaceRuntimeRemovesPrefix(t *testing.T) {
	root := t.TempDir()
	objects := newTestObjectStoreAt(t, root)
	store := NewObjectRuntimeStore(objects)

	prefix := ObjectPrefix("demo")
	if err := objects.Put(prefix+"/history/item.json", strings.NewReader("{}")); err != nil {
		t.Fatalf("Put history: %v", err)
	}
	if err := store.DeleteWorkspaceRuntime(context.Background(), "demo"); err != nil {
		t.Fatalf("DeleteWorkspaceRuntime() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(prefix))); !os.IsNotExist(err) {
		t.Fatalf("workspace dir after delete err = %v, want not exist", err)
	}
}

func TestObjectRuntimeStoreValidation(t *testing.T) {
	if _, err := (ObjectRuntimeStore{}).PrepareWorkspace(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "runtime store") {
		t.Fatalf("PrepareWorkspace(nil store) error = %v", err)
	}

	store := NewObjectRuntimeStore(newTestObjectStore(t))
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
