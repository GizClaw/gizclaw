package workspace

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

// GetSystemWorkspaceForCleanup reads an exact owner-scoped SQL identity for
// lifecycle recovery, including when the Workspace or owner is retiring. It
// does not authorize ordinary business access or infer completion from absence.
func (s *Server) GetSystemWorkspaceForCleanup(ctx context.Context, name string) (apitypes.Workspace, error) {
	owner, ok := ownership.FromContext(ctx)
	if !ok || owner == "" || owner != strings.TrimSpace(owner) || name == "" || name != strings.TrimSpace(name) {
		return apitypes.Workspace{}, errors.New("workspace: cleanup requires canonical owner and name")
	}
	store, err := s.store()
	if err != nil {
		return apitypes.Workspace{}, err
	}
	item, _, err := getSQLWorkspaceByName(ctx, store, &owner, name)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if !workspaceIsSystem(item) || item.OwnerPublicKey == nil || *item.OwnerPublicKey != owner {
		return apitypes.Workspace{}, errors.New("workspace: cleanup identity is not an owned system Workspace")
	}
	return item, nil
}
