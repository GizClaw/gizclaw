package memorystore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// ScopeForRequest maps the selected MemoryLayout implementation to its
// product-owned memory identity. An omitted mapping keeps Workspace scope.
func ScopeForRequest(request Request) (memory.Scope, error) {
	var selected string
	switch request.Binding.Driver {
	case apitypes.RuntimeProfileMemoryDriverFlowcraft:
		if request.Layout.Spec.Flowcraft.Scope != nil {
			selected = string(*request.Layout.Spec.Flowcraft.Scope)
		}
	case apitypes.RuntimeProfileMemoryDriverMem0:
		if request.Layout.Spec.Mem0.Scope != nil {
			selected = string(*request.Layout.Spec.Mem0.Scope)
		}
	case apitypes.RuntimeProfileMemoryDriverVolcMem0:
		if request.Layout.Spec.VolcMem0.Scope != nil {
			selected = string(*request.Layout.Spec.VolcMem0.Scope)
		}
	default:
		return memory.Scope{}, fmt.Errorf("memory store: unsupported scope driver %q", request.Binding.Driver)
	}
	switch selected {
	case "", "workspace":
		if strings.TrimSpace(request.WorkspaceID) == "" {
			return memory.Scope{}, fmt.Errorf("memory store: Workspace ID is required")
		}
		return memory.Scope{AppID: request.WorkspaceID}, nil
	case "peer":
		owner := strings.TrimSpace(request.OwnerPublicKey)
		if owner == "" {
			return memory.Scope{}, fmt.Errorf("memory store: owner Peer is required for peer scope")
		}
		// ':' cannot occur in a canonical Workspace resource ID. The hash keeps
		// the provider identity short and avoids exposing the public key in it.
		digest := sha256.Sum256([]byte(owner))
		return memory.Scope{AppID: "peer:" + hex.EncodeToString(digest[:16])}, nil
	default:
		return memory.Scope{}, fmt.Errorf("memory store: unsupported memory scope %q", selected)
	}
}

// PeerScoped reports whether the selected implementation writes to the owner
// Peer's shared scope. Invalid policy is rejected by ScopeForRequest.
func PeerScoped(request Request) (bool, error) {
	scope, err := ScopeForRequest(request)
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(scope.AppID, "peer:"), nil
}
