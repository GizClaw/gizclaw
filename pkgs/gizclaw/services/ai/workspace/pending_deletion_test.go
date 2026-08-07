package workspace

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPendingDeletionSourceOwnsOnlyWorkspaces(t *testing.T) {
	source := NewPendingDeletionSource(kv.NewMemory(nil))
	if err := source.Validate(); err != nil {
		t.Fatal(err)
	}
	if source.Name() != pendingDeletionSourceName {
		t.Fatalf("Name() = %q", source.Name())
	}
	kinds := source.Kinds()
	if len(kinds) != 1 || kinds[0] != pendingdeletion.KindWorkspace {
		t.Fatalf("Kinds() = %#v", kinds)
	}
}
