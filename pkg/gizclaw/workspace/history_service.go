package workspace

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

type Authorizer interface {
	Authorize(context.Context, acl.AuthorizeRequest) error
}

func (s *Server) AppendWorkspaceHistory(ctx context.Context, workspaceName string, req AppendHistoryRequest) (HistoryEntry, error) {
	store, err := s.historyStore(ctx, workspaceName)
	if err != nil {
		return HistoryEntry{}, err
	}
	return store.Append(ctx, req)
}

func (s *Server) ListWorkspaceHistory(ctx context.Context, subject apitypes.ACLSubject, workspaceName string, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	store, err := s.authorizedHistoryStore(ctx, subject, workspaceName)
	if err != nil {
		return apitypes.PeerRunHistoryListResponse{}, err
	}
	return store.List(ctx, req)
}

func (s *Server) GetWorkspaceHistory(ctx context.Context, subject apitypes.ACLSubject, workspaceName, historyID string) (HistoryEntry, error) {
	store, err := s.authorizedHistoryStore(ctx, subject, workspaceName)
	if err != nil {
		return HistoryEntry{}, err
	}
	return store.Get(ctx, historyID)
}

func (s *Server) ReadWorkspaceHistoryAsset(ctx context.Context, subject apitypes.ACLSubject, workspaceName, assetName string) (io.ReadCloser, error) {
	store, err := s.authorizedHistoryStore(ctx, subject, workspaceName)
	if err != nil {
		return nil, err
	}
	return store.ReadAsset(ctx, assetName)
}

func (s *Server) authorizedHistoryStore(ctx context.Context, subject apitypes.ACLSubject, workspaceName string) (*HistoryStore, error) {
	workspaceName = strings.TrimSpace(workspaceName)
	if err := s.authorizeHistoryRead(ctx, subject, workspaceName); err != nil {
		return nil, err
	}
	return s.historyStore(ctx, workspaceName)
}

func (s *Server) authorizeHistoryRead(ctx context.Context, subject apitypes.ACLSubject, workspaceName string) error {
	if s == nil {
		return fmt.Errorf("workspace: nil server")
	}
	if s.Authorizer == nil {
		return nil
	}
	return s.Authorizer.Authorize(ctx, acl.AuthorizeRequest{
		Subject:    subject,
		Resource:   acl.WorkspaceResource(workspaceName),
		Permission: apitypes.ACLPermissionWorkspaceRead,
	})
}

func (s *Server) historyStore(ctx context.Context, workspaceName string) (*HistoryStore, error) {
	if s == nil {
		return nil, fmt.Errorf("workspace: nil server")
	}
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		return nil, fmt.Errorf("workspace: name is required")
	}
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	if _, err := getWorkspace(ctx, store, workspaceName); err != nil {
		return nil, err
	}
	if s.RuntimeStore == nil {
		return nil, fmt.Errorf("workspace: runtime store is required")
	}
	rt, err := s.RuntimeStore.GetWorkspaceRuntime(ctx, workspaceName)
	if err != nil {
		return nil, err
	}
	if rt.History == nil {
		return nil, fmt.Errorf("workspace: history store is required")
	}
	return rt.History, nil
}
