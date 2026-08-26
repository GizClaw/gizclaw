package workspace

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type PeerRetirementWorkspace struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	HasIcon bool   `json:"has_icon"`
}

type PeerRetirementSnapshot struct {
	PublicKey     string                    `json:"public_key"`
	Workspaces    []PeerRetirementWorkspace `json:"workspaces"`
	PetWorkspaces []PeerRetirementWorkspace `json:"pet_workspaces"`
}

func (s *Server) SnapshotPeerWorkspaces(ctx context.Context, publicKey string, petWorkspaceIDs []string) (PeerRetirementSnapshot, error) {
	if publicKey == "" || publicKey != strings.TrimSpace(publicKey) {
		return PeerRetirementSnapshot{}, errors.New("workspace: Peer public key is required and must be canonical")
	}
	release, err := s.ownerCreateLocks.Acquire(ctx, publicKey)
	if err != nil {
		return PeerRetirementSnapshot{}, err
	}
	defer release()
	store, err := s.store()
	if err != nil {
		return PeerRetirementSnapshot{}, err
	}
	result := PeerRetirementSnapshot{PublicKey: publicKey}
	for entry, err := range store.List(ctx, workspaceByOwnerPrefix(publicKey)) {
		if err != nil {
			return PeerRetirementSnapshot{}, err
		}
		item, err := getWorkspaceByID(ctx, store, string(entry.Value))
		if err != nil {
			return PeerRetirementSnapshot{}, err
		}
		if item.OwnerPublicKey == nil || *item.OwnerPublicKey != publicKey || workspaceIsSystem(item) {
			return PeerRetirementSnapshot{}, errors.New("workspace: owner index contains a foreign or system Workspace")
		}
		result.Workspaces = append(result.Workspaces, PeerRetirementWorkspace{ID: item.Id, Name: item.Name, HasIcon: item.Icon != nil})
	}
	sort.Slice(result.Workspaces, func(i, j int) bool { return result.Workspaces[i].ID < result.Workspaces[j].ID })
	seen := make(map[string]struct{}, len(petWorkspaceIDs))
	for _, id := range petWorkspaceIDs {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		item, err := getWorkspaceByID(ctx, store, id)
		if err != nil {
			return PeerRetirementSnapshot{}, err
		}
		if item.OwnerPublicKey == nil || *item.OwnerPublicKey != publicKey || !workspaceIsSystem(item) {
			return PeerRetirementSnapshot{}, errors.New("workspace: Pet Workspace is foreign or is not a system Workspace")
		}
		result.PetWorkspaces = append(result.PetWorkspaces, PeerRetirementWorkspace{ID: item.Id, Name: item.Name, HasIcon: item.Icon != nil})
	}
	sort.Slice(result.PetWorkspaces, func(i, j int) bool { return result.PetWorkspaces[i].ID < result.PetWorkspaces[j].ID })
	return result, nil
}

// RetirePeerWorkspaces creates exact Workspace child markers. The independent
// Workspace source performs artifact cleanup and physical finalization.
func (s *Server) RetirePeerWorkspaces(ctx context.Context, snapshot PeerRetirementSnapshot) ([]string, error) {
	if snapshot.PublicKey == "" {
		return nil, errors.New("workspace: Peer retirement public key is required")
	}
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(snapshot.Workspaces))
	for _, expected := range snapshot.Workspaces {
		unlock := s.IconLocks.LockOwner(expected.ID)
		item, getErr := getWorkspaceByID(ctx, store, expected.ID)
		if getErr == nil {
			if item.OwnerPublicKey == nil || *item.OwnerPublicKey != snapshot.PublicKey || workspaceIsSystem(item) ||
				item.Name != expected.Name || (item.Icon != nil) != expected.HasIcon {
				unlock()
				return nil, errors.New("workspace: Workspace no longer matches Peer retirement snapshot")
			}
			getErr = s.fastDeleteWorkspaceRecord(ctx, store, item)
		}
		unlock()
		if getErr != nil && !errors.Is(getErr, kv.ErrNotFound) {
			return nil, getErr
		}
		ids = append(ids, expected.ID)
	}
	sort.Strings(ids)
	return ids, nil
}

// RetirePeerPetWorkspaces runs only after every Pet child has finalized, so a
// Pet row can never reference a Workspace that the Workspace handler already
// removed.
func (s *Server) RetirePeerPetWorkspaces(ctx context.Context, snapshot PeerRetirementSnapshot) ([]string, error) {
	if snapshot.PublicKey == "" {
		return nil, errors.New("workspace: Peer retirement public key is required")
	}
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(snapshot.PetWorkspaces))
	for _, expected := range snapshot.PetWorkspaces {
		unlock := s.IconLocks.LockOwner(expected.ID)
		item, getErr := getWorkspaceByID(ctx, store, expected.ID)
		if getErr == nil {
			if item.OwnerPublicKey == nil || *item.OwnerPublicKey != snapshot.PublicKey || !workspaceIsSystem(item) ||
				item.Name != expected.Name || (item.Icon != nil) != expected.HasIcon {
				unlock()
				return nil, errors.New("workspace: Pet Workspace no longer matches Peer retirement snapshot")
			}
			getErr = s.retirePeerPetWorkspaceRecord(ctx, store, item, snapshot.PublicKey)
		}
		unlock()
		if getErr != nil && !errors.Is(getErr, kv.ErrNotFound) {
			return nil, getErr
		}
		ids = append(ids, expected.ID)
	}
	sort.Strings(ids)
	return ids, nil
}
