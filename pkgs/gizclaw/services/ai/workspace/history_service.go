package workspace

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *Server) AppendWorkspaceHistory(ctx context.Context, workspaceName string, req AppendHistoryRequest) (HistoryEntry, error) {
	workspaceName = strings.TrimSpace(workspaceName)
	metadataStore, history, err := s.historyStoreWithMetadata(ctx, workspaceName)
	if err != nil {
		return HistoryEntry{}, err
	}
	entry, err := history.Append(ctx, req)
	if err != nil {
		return HistoryEntry{}, err
	}
	if err := bumpWorkspaceLastActive(ctx, metadataStore, workspaceName, entry.CreatedAt); err != nil {
		return HistoryEntry{}, err
	}
	return entry, nil
}

func (s *Server) ListWorkspaceHistory(ctx context.Context, workspaceName string, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	store, err := s.historyStore(ctx, workspaceName)
	if err != nil {
		return apitypes.PeerRunHistoryListResponse{}, err
	}
	return store.List(ctx, req)
}

func (s *Server) ListWorkspaceHistoryByID(ctx context.Context, workspaceID string, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	store, err := s.historyStoreByID(ctx, workspaceID)
	if err != nil {
		return apitypes.PeerRunHistoryListResponse{}, err
	}
	return store.List(ctx, req)
}

// ListWorkspaceHistoryPage returns one internal History page for Server-owned
// projections that need authoritative entry and asset metadata.
func (s *Server) ListWorkspaceHistoryPage(ctx context.Context, workspaceName string, req apitypes.PeerRunHistoryListRequest) (HistoryEntryPage, error) {
	store, err := s.historyStore(ctx, workspaceName)
	if err != nil {
		return HistoryEntryPage{}, err
	}
	return store.ListPage(ctx, req)
}

func (s *Server) ListWorkspaceHistoryPageByID(ctx context.Context, workspaceID string, req apitypes.PeerRunHistoryListRequest) (HistoryEntryPage, error) {
	store, err := s.historyStoreByID(ctx, workspaceID)
	if err != nil {
		return HistoryEntryPage{}, err
	}
	return store.ListPage(ctx, req)
}

// ListWorkspaceHistoryEntries returns the internal persisted shape used by
// Server-owned post-processors. Public History DTOs intentionally omit origin.
func (s *Server) ListWorkspaceHistoryEntries(ctx context.Context, workspaceName, after, through string, limit int) (HistoryEntryPage, error) {
	store, err := s.historyStore(ctx, workspaceName)
	if err != nil {
		return HistoryEntryPage{}, err
	}
	return store.ListEntries(ctx, after, through, limit)
}

func (s *Server) ListWorkspaceHistoryEntriesByID(ctx context.Context, workspaceID, after, through string, limit int) (HistoryEntryPage, error) {
	store, err := s.historyStoreByID(ctx, workspaceID)
	if err != nil {
		return HistoryEntryPage{}, err
	}
	return store.ListEntries(ctx, after, through, limit)
}

// LatestWorkspaceHistoryEntry returns the newest retained internal entry.
func (s *Server) LatestWorkspaceHistoryEntry(ctx context.Context, workspaceName string) (HistoryEntry, bool, error) {
	store, err := s.historyStore(ctx, workspaceName)
	if err != nil {
		return HistoryEntry{}, false, err
	}
	return store.LatestEntry(ctx)
}

func (s *Server) LatestWorkspaceHistoryEntryByID(ctx context.Context, workspaceID string) (HistoryEntry, bool, error) {
	store, err := s.historyStoreByID(ctx, workspaceID)
	if err != nil {
		return HistoryEntry{}, false, err
	}
	return store.LatestEntry(ctx)
}

// LatestWorkspaceHistoryEntryBefore returns the newest retained entry strictly
// before a non-retroactive activation boundary.
func (s *Server) LatestWorkspaceHistoryEntryBefore(
	ctx context.Context,
	workspaceName string,
	before time.Time,
) (HistoryEntry, bool, error) {
	store, err := s.historyStore(ctx, workspaceName)
	if err != nil {
		return HistoryEntry{}, false, err
	}
	return store.LatestEntryBefore(ctx, before)
}

func (s *Server) LatestWorkspaceHistoryEntryBeforeByID(ctx context.Context, workspaceID string, before time.Time) (HistoryEntry, bool, error) {
	store, err := s.historyStoreByID(ctx, workspaceID)
	if err != nil {
		return HistoryEntry{}, false, err
	}
	return store.LatestEntryBefore(ctx, before)
}

func (s *Server) AdminListWorkspaceHistory(ctx context.Context, workspaceName string, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	store, err := s.historyStoreByID(ctx, workspaceName)
	if err != nil {
		return apitypes.PeerRunHistoryListResponse{}, err
	}
	return store.List(ctx, req)
}

func (s *Server) GetWorkspaceHistory(ctx context.Context, workspaceName, historyID string) (HistoryEntry, error) {
	store, err := s.historyStore(ctx, workspaceName)
	if err != nil {
		return HistoryEntry{}, err
	}
	return store.Get(ctx, historyID)
}

func (s *Server) GetWorkspaceHistoryByID(ctx context.Context, workspaceID, historyID string) (HistoryEntry, error) {
	store, err := s.historyStoreByID(ctx, workspaceID)
	if err != nil {
		return HistoryEntry{}, err
	}
	return store.Get(ctx, historyID)
}

func (s *Server) AdminGetWorkspaceHistory(ctx context.Context, workspaceName, historyID string) (HistoryEntry, error) {
	store, err := s.historyStoreByID(ctx, workspaceName)
	if err != nil {
		return HistoryEntry{}, err
	}
	return store.Get(ctx, historyID)
}

func (s *Server) ReadWorkspaceHistoryAsset(ctx context.Context, workspaceName, assetName string) (io.ReadCloser, error) {
	store, err := s.historyStore(ctx, workspaceName)
	if err != nil {
		return nil, err
	}
	return store.ReadAsset(ctx, assetName)
}

func (s *Server) ReadWorkspaceHistoryAssetByID(ctx context.Context, workspaceID, assetName string) (io.ReadCloser, error) {
	store, err := s.historyStoreByID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return store.ReadAsset(ctx, assetName)
}

func (s *Server) AdminReadWorkspaceHistoryAudio(ctx context.Context, workspaceName, historyID string) (io.ReadCloser, int64, error) {
	store, err := s.historyStoreByID(ctx, workspaceName)
	if err != nil {
		return nil, 0, err
	}
	entry, err := store.Get(ctx, historyID)
	if err != nil {
		return nil, 0, err
	}
	for _, asset := range entry.Assets {
		if strings.EqualFold(strings.TrimSpace(asset.MIMEType), "audio/ogg") || strings.EqualFold(strings.TrimSpace(asset.MIMEType), "audio/ogg; codecs=opus") {
			r, err := store.ReadAsset(ctx, asset.Name)
			if err != nil {
				return nil, 0, err
			}
			return r, asset.Bytes, nil
		}
	}
	return nil, 0, fs.ErrNotExist
}

func (s *Server) historyStore(ctx context.Context, workspaceName string) (*HistoryStore, error) {
	_, history, err := s.historyStoreWithMetadata(ctx, workspaceName)
	return history, err
}

func (s *Server) historyStoreWithMetadata(ctx context.Context, workspaceName string) (kv.Store, *HistoryStore, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("workspace: nil server")
	}
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		return nil, nil, fmt.Errorf("workspace: name is required")
	}
	store, err := s.store()
	if err != nil {
		return nil, nil, err
	}
	workspace, err := getWorkspace(ctx, store, workspaceName)
	if err != nil {
		return nil, nil, err
	}
	if s.RuntimeStore == nil {
		return nil, nil, fmt.Errorf("workspace: runtime store is required")
	}
	rt, err := s.RuntimeStore.GetWorkspaceRuntime(ctx, workspace.Id)
	if err != nil {
		return nil, nil, err
	}
	if rt.History == nil {
		return nil, nil, fmt.Errorf("workspace: history store is required")
	}
	return store, rt.History, nil
}

func (s *Server) historyStoreByID(ctx context.Context, workspaceID string) (*HistoryStore, error) {
	if s == nil {
		return nil, fmt.Errorf("workspace: nil server")
	}
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return nil, fmt.Errorf("workspace: invalid id: %w", err)
	}
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	workspace, err := getWorkspaceByID(ctx, store, workspaceID)
	if err != nil {
		return nil, err
	}
	if s.RuntimeStore == nil {
		return nil, fmt.Errorf("workspace: runtime store is required")
	}
	rt, err := s.RuntimeStore.GetWorkspaceRuntime(ctx, workspace.Id)
	if err != nil {
		return nil, err
	}
	if rt.History == nil {
		return nil, fmt.Errorf("workspace: history store is required")
	}
	return rt.History, nil
}

func bumpWorkspaceLastActive(ctx context.Context, store kv.Store, workspaceName string, lastActiveAt time.Time) error {
	if lastActiveAt.IsZero() {
		lastActiveAt = time.Now().UTC()
	}
	workspace, err := getWorkspace(ctx, store, workspaceName)
	if err != nil {
		return err
	}
	lastActiveAt = lastActiveAt.UTC()
	if !lastActiveAt.After(workspace.LastActiveAt) {
		return nil
	}
	workspace.LastActiveAt = lastActiveAt
	return writeWorkspace(ctx, store, workspace)
}
