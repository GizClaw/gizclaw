package friendgroup

import (
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
)

func TestPendingDeletionSourceOwnsOnlyFriendGroups(t *testing.T) {
	store := newTestServer(t).RelationshipStore
	source := NewPendingDeletionSource(store)
	if err := source.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if source.Name() != pendingDeletionSourceName {
		t.Fatalf("Name() = %q", source.Name())
	}
	kinds := source.Kinds()
	if len(kinds) != 1 || kinds[0] != pendingdeletion.KindFriendGroup {
		t.Fatalf("Kinds() = %#v", kinds)
	}

	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	var workspaceDeletionID string
	for _, kind := range []pendingdeletion.Kind{pendingdeletion.KindFriendGroup, pendingdeletion.KindWorkspace} {
		record, err := pendingdeletion.New(kind, string(kind)+"-a", nil, pendingdeletion.ReasonResourceDelete, map[string]string{"id": string(kind) + "-a"}, now)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := pendingdeletion.CreateOrGet(t.Context(), store, record); err != nil {
			t.Fatal(err)
		}
		if kind == pendingdeletion.KindWorkspace {
			workspaceDeletionID = record.DeletionID
		}
	}
	var found []pendingdeletion.Reference
	cursor := ""
	for {
		refs, next, err := source.ScanDue(t.Context(), now, 1, cursor)
		if err != nil {
			t.Fatalf("ScanDue(%q) error = %v", cursor, err)
		}
		found = append(found, refs...)
		if next == "" {
			break
		}
		cursor = next
	}
	if len(found) != 1 || found[0].Source != pendingDeletionSourceName {
		t.Fatalf("bounded ScanDue() references = %#v", found)
	}
	if _, err := source.GetTask(t.Context(), workspaceDeletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
		t.Fatalf("GetTask(foreign) error = %v, want ErrNotFound", err)
	}
}
