package flowcraft

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"testing"

	flowworkspace "github.com/GizClaw/flowcraft/sdk/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestObjectWorkspaceCRUDIsolationAndStableList(t *testing.T) {
	ctx := context.Background()
	objects := objectstore.Dir(t.TempDir())
	first, err := newObjectWorkspace(objects, "flowcraft-memory/workspace-a/assistant")
	if err != nil {
		t.Fatal(err)
	}
	second, err := newObjectWorkspace(objects, "flowcraft-memory/workspace-b/assistant")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Write(ctx, "facts/z.json", []byte("z")); err != nil {
		t.Fatal(err)
	}
	if err := first.Write(ctx, "facts/a.json", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := second.Write(ctx, "facts/a.json", []byte("other")); err != nil {
		t.Fatal(err)
	}
	entries, err := first.List(ctx, "facts")
	if err != nil {
		t.Fatal(err)
	}
	got := []string{entries[0].Name(), entries[1].Name()}
	if !reflect.DeepEqual(got, []string{"a.json", "z.json"}) {
		t.Fatalf("List() = %#v", got)
	}
	data, err := first.Read(ctx, "facts/a.json")
	if err != nil || string(data) != "a" {
		t.Fatalf("first Read() = %q, %v", data, err)
	}
	data, err = second.Read(ctx, "facts/a.json")
	if err != nil || string(data) != "other" {
		t.Fatalf("second Read() = %q, %v", data, err)
	}
	if err := first.RemoveAll(ctx, "facts"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Read(ctx, "facts/a.json"); !errors.Is(err, flowworkspace.ErrNotFound) {
		t.Fatalf("removed Read() error = %v", err)
	}
	if data, err := second.Read(ctx, "facts/a.json"); err != nil || string(data) != "other" {
		t.Fatalf("second scope after removal = %q, %v", data, err)
	}
}

func TestObjectWorkspaceRejectsPathTraversal(t *testing.T) {
	workspace, err := newObjectWorkspace(objectstore.Dir(t.TempDir()), "memory/workspace")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../secret", "/absolute", `..\\secret`} {
		if _, err := workspace.Read(context.Background(), name); !errors.Is(err, flowworkspace.ErrPathTraversal) {
			t.Fatalf("Read(%q) error = %v", name, err)
		}
	}
	if _, err := workspace.Stat(context.Background(), "missing"); !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, flowworkspace.ErrNotFound) {
		t.Fatalf("Stat(missing) error = %v", err)
	}
}
