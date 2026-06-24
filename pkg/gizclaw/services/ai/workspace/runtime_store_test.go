package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

func TestObjectRuntimeStorePrepareWorkspaceCreatesLocalDir(t *testing.T) {
	root := t.TempDir()
	store := NewObjectRuntimeStore(objectstore.Dir(root))

	rt, err := store.PrepareWorkspace(context.Background(), "demo ws")
	if err != nil {
		t.Fatalf("PrepareWorkspace() error = %v", err)
	}
	if rt.ObjectPrefix != "workspaces/demo%20ws" {
		t.Fatalf("ObjectPrefix = %q, want escaped workspace prefix", rt.ObjectPrefix)
	}
	wantDir := filepath.Join(root, "workspaces", "demo%20ws")
	if rt.LocalDir != wantDir {
		t.Fatalf("LocalDir = %q, want %q", rt.LocalDir, wantDir)
	}
	if info, err := os.Stat(wantDir); err != nil || !info.IsDir() {
		t.Fatalf("workspace dir not created: info=%v err=%v", info, err)
	}
}

func TestObjectRuntimeStoreDeleteWorkspaceRuntimeRemovesPrefix(t *testing.T) {
	root := t.TempDir()
	objects := objectstore.Dir(root)
	store := NewObjectRuntimeStore(objects)

	if err := objects.Put("workspaces/demo/history/item.json", strings.NewReader("{}")); err != nil {
		t.Fatalf("Put history: %v", err)
	}
	if err := store.DeleteWorkspaceRuntime(context.Background(), "demo"); err != nil {
		t.Fatalf("DeleteWorkspaceRuntime() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspaces", "demo")); !os.IsNotExist(err) {
		t.Fatalf("workspace dir after delete err = %v, want not exist", err)
	}
}

func TestObjectRuntimeStoreValidation(t *testing.T) {
	if _, err := (ObjectRuntimeStore{}).PrepareWorkspace(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "runtime store") {
		t.Fatalf("PrepareWorkspace(nil store) error = %v", err)
	}

	store := NewObjectRuntimeStore(objectstore.Dir(t.TempDir()))
	if _, err := store.PrepareWorkspace(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("PrepareWorkspace(empty workspace) error = %v", err)
	}
	if err := store.DeleteWorkspaceRuntime(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("DeleteWorkspaceRuntime(empty workspace) error = %v", err)
	}
}
