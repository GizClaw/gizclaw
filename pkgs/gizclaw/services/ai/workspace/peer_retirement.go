package workspace

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

type PeerRetirementWorkspace struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	HasIcon bool   `json:"has_icon"`
}

type PeerRetirementSnapshot struct {
	PublicKey  string                    `json:"public_key"`
	Workspaces []PeerRetirementWorkspace `json:"workspaces"`
}

func (s *Server) SnapshotPeerWorkspaces(ctx context.Context, publicKey string) (PeerRetirementSnapshot, error) {
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
	items, err := listAllSQLWorkspaces(ctx, store, workspaceSQLFilter{owner: &publicKey, ordinaryOnly: true})
	if err != nil {
		return PeerRetirementSnapshot{}, err
	}
	for _, item := range items {
		result.Workspaces = append(result.Workspaces, PeerRetirementWorkspace{ID: item.Id, Name: item.Name, HasIcon: item.Icon != nil})
	}
	sort.Slice(result.Workspaces, func(i, j int) bool { return result.Workspaces[i].ID < result.Workspaces[j].ID })
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
		if getErr != nil && !errors.Is(getErr, sql.ErrNoRows) {
			return nil, getErr
		}
		ids = append(ids, expected.ID)
	}
	sort.Strings(ids)
	return ids, nil
}
