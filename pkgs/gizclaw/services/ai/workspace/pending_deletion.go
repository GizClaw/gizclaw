package workspace

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const pendingDeletionSourceName = "workspace"

// NewPendingDeletionSource binds Workspace tasks to the exact Workspace KV
// transaction boundary.
func NewPendingDeletionSource(store kv.Store) pendingdeletion.KVSource {
	return pendingdeletion.KVSource{
		Store:      store,
		SourceName: pendingDeletionSourceName,
		OwnedKinds: []pendingdeletion.Kind{pendingdeletion.KindWorkspace},
	}
}
