package peer

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const PendingDeletionSourceName = "peer"

// PendingDeletionSource reads Peer deletion tasks through a strong Store view.
func PendingDeletionSource(store kv.Store) pendingdeletion.KVSource {
	return pendingdeletion.KVSource{
		Store: kv.WithStrongReads(store), SourceName: PendingDeletionSourceName,
		OwnedKinds: []pendingdeletion.Kind{pendingdeletion.KindPeer},
	}
}
