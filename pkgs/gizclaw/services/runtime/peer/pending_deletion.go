package peer

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const PendingDeletionSourceName = "peer"

func PendingDeletionSource(store kv.Store) pendingdeletion.KVSource {
	return pendingdeletion.KVSource{
		Store: store, SourceName: PendingDeletionSourceName,
		OwnedKinds: []pendingdeletion.Kind{pendingdeletion.KindPeer},
	}
}
