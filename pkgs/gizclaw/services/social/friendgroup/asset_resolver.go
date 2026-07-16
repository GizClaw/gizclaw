package friendgroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// ResolveAssetOwner loads a FriendGroup message without caller authorization
// for internal reverse-reference validation.
func (s *Server) ResolveAssetOwner(ctx context.Context, owner asset.Owner) (asset.OwnerSnapshot, error) {
	if owner.Kind != asset.OwnerKindFriendGroupMessage {
		return asset.OwnerSnapshot{}, fmt.Errorf("unsupported friend-group owner kind %q", owner.Kind)
	}
	groupID, messageID, ok := strings.Cut(owner.ID, "/")
	if !ok || groupID == "" || messageID == "" {
		return asset.OwnerSnapshot{}, fmt.Errorf("invalid friend-group message owner id %q", owner.ID)
	}
	store, err := s.messagesStore()
	if err != nil {
		return asset.OwnerSnapshot{}, err
	}
	message, err := socialutil.ReadJSONValue[rpcapi.FriendGroupMessageObject](ctx, store, socialutil.GroupMessageKey(groupID, messageID))
	if errors.Is(err, kv.ErrNotFound) {
		return asset.OwnerSnapshot{Exists: false}, nil
	}
	if err != nil {
		return asset.OwnerSnapshot{}, err
	}
	if socialutil.MessageExpired(message, s.now()) {
		return asset.OwnerSnapshot{Exists: false}, nil
	}
	data, err := json.Marshal(message)
	if err != nil {
		return asset.OwnerSnapshot{}, err
	}
	return asset.OwnerSnapshot{Exists: true, Refs: friendGroupAssetRefs(data)}, nil
}

func friendGroupAssetRefs(data []byte) []asset.Ref {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	refs := make([]asset.Ref, 0)
	var visit func(any)
	visit = func(current any) {
		switch typed := current.(type) {
		case string:
			if ref, err := asset.ParseRef(typed); err == nil {
				refs = append(refs, ref)
			}
		case []any:
			for _, item := range typed {
				visit(item)
			}
		case map[string]any:
			for _, item := range typed {
				visit(item)
			}
		}
	}
	visit(value)
	return refs
}
