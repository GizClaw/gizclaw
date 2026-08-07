package friendgroup

import (
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const pendingDeletionSourceName = "friend_group"

// NewPendingDeletionSource binds Friend Group task state to the same atomic
// relationship store that owns its marker and retirement proof.
func NewPendingDeletionSource(store kv.Store) pendingdeletion.KVSource {
	return pendingdeletion.KVSource{
		Store:      store,
		SourceName: pendingDeletionSourceName,
		OwnedKinds: []pendingdeletion.Kind{pendingdeletion.KindFriendGroup},
	}
}
