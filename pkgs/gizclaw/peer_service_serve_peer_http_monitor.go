package gizclaw

import (
	"context"
	"errors"
	"io/fs"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *peerHTTP) ListDeviceWorkspaces(ctx context.Context, _ peerhttp.ListDeviceWorkspacesRequestObject) (peerhttp.ListDeviceWorkspacesResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListDeviceWorkspaces401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s.Workspaces == nil {
		return peerhttp.ListDeviceWorkspaces500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	items, err := s.Workspaces.ListOwnedHistoryWorkspaces(ctx, owner.String())
	if err != nil {
		return peerhttp.ListDeviceWorkspaces500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	result := make(peerhttp.ListDeviceWorkspaces200JSONResponse, 0, len(items))
	for _, item := range items {
		result = append(result, peerhttp.DeviceWorkspace{Id: item.Id, Name: item.Name, WorkflowId: item.WorkflowId, LastActiveAt: item.LastActiveAt})
	}
	return result, nil
}

func (s *peerHTTP) ListDeviceWorkspaceHistory(ctx context.Context, req peerhttp.ListDeviceWorkspaceHistoryRequestObject) (peerhttp.ListDeviceWorkspaceHistoryResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListDeviceWorkspaceHistory401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s.Workspaces == nil {
		return peerhttp.ListDeviceWorkspaceHistory500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if (req.Params.Limit != nil && (*req.Params.Limit < 1 || *req.Params.Limit > 200)) || (req.Params.Query != nil && len(*req.Params.Query) > 512) {
		return peerhttp.ListDeviceWorkspaceHistory400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError("INVALID_REQUEST", "invalid history query"))}, nil
	}
	items, err := s.Workspaces.ListOwnedHistoryWorkspaces(ctx, owner.String())
	if err != nil {
		return peerhttp.ListDeviceWorkspaceHistory500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	allowed := false
	for _, item := range items {
		if item.Id == req.WorkspaceId {
			allowed = true
			break
		}
	}
	if !allowed {
		return peerhttp.ListDeviceWorkspaceHistory404JSONResponse{NotFoundJSONResponse: peerhttp.NotFoundJSONResponse(apiError("WORKSPACE_NOT_FOUND", "workspace not found"))}, nil
	}
	query := ""
	if req.Params.Query != nil {
		query = strings.TrimSpace(*req.Params.Query)
	}
	order := apitypes.PeerRunHistoryListRequestOrderDesc
	result, err := s.Workspaces.SearchWorkspaceHistoryByID(ctx, req.WorkspaceId, apitypes.PeerRunHistoryListRequest{Cursor: req.Params.Cursor, Limit: req.Params.Limit, Order: &order}, query)
	if err != nil {
		return peerhttp.ListDeviceWorkspaceHistory500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.ListDeviceWorkspaceHistory200JSONResponse(result), nil
}

func (s *peerHTTP) SearchDeviceLogs(ctx context.Context, req peerhttp.SearchDeviceLogsRequestObject) (peerhttp.SearchDeviceLogsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.SearchDeviceLogs401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s.ServerLogs == nil {
		return peerhttp.SearchDeviceLogs500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(apiError("LOG_QUERY_NOT_CONFIGURED", "persistent log query is not configured"))}, nil
	}
	limit := 100
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	if limit < 1 || limit > 500 || (req.Params.Query != nil && len(*req.Params.Query) > 512) {
		return peerhttp.SearchDeviceLogs400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError("INVALID_REQUEST", "invalid log query"))}, nil
	}
	// Always bind the filter, including continuation requests: an opaque cursor
	// from another Peer must never replace this authoritative owner condition.
	filter := "peer_public_key:" + strconv.Quote(owner.String())
	if req.Params.Query != nil && *req.Params.Query != "" {
		filter += " AND text:" + strconv.Quote(*req.Params.Query)
	}
	if req.Params.Level != nil {
		filter += " AND level:" + strconv.Quote(string(*req.Params.Level))
	}
	cursor := ""
	if req.Params.Cursor != nil {
		cursor = *req.Params.Cursor
	}
	request := ServerLogStreamRequest{Filter: filter, FilterSet: true, StartTimeMs: req.Params.StartTimeMs, StartTimeSet: true, EndTimeMs: req.Params.EndTimeMs, EndTimeSet: true, Limit: limit, Order: ServerLogOrderDesc, OrderSet: true, Cursor: cursor}
	items := make([]apitypes.ServerLogEntry, 0, limit)
	end, err := s.ServerLogs.StreamServerLogs(ctx, request, func(entry apitypes.ServerLogEntry) error {
		if entry.Fields["peer_public_key"] != owner.String() {
			return errors.New("log backend returned a foreign peer record")
		}
		items = append(items, entry)
		return nil
	})
	if err != nil {
		var queryErr *ServerLogQueryError
		if errors.As(err, &queryErr) && queryErr.StatusCode == 400 {
			return peerhttp.SearchDeviceLogs400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(queryErr.Code, queryErr.Message))}, nil
		}
		return peerhttp.SearchDeviceLogs500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.SearchDeviceLogs200JSONResponse{Items: items, End: end}, nil
}

func (s *peerHTTP) DownloadDeviceHistoryAudio(ctx context.Context, req peerhttp.DownloadDeviceHistoryAudioRequestObject) (peerhttp.DownloadDeviceHistoryAudioResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.DownloadDeviceHistoryAudio401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s.Workspaces == nil {
		return peerhttp.DownloadDeviceHistoryAudio500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	items, err := s.Workspaces.ListOwnedHistoryWorkspaces(ctx, owner.String())
	if err != nil {
		return peerhttp.DownloadDeviceHistoryAudio500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	allowed := false
	for _, item := range items {
		if item.Id == req.WorkspaceId {
			allowed = true
			break
		}
	}
	if !allowed {
		return peerhttp.DownloadDeviceHistoryAudio404JSONResponse{NotFoundJSONResponse: peerhttp.NotFoundJSONResponse(apiError("HISTORY_NOT_FOUND", "history audio not found"))}, nil
	}
	r, size, err := s.Workspaces.ReadWorkspaceHistoryAudioByID(ctx, req.WorkspaceId, req.HistoryId)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, kv.ErrNotFound) {
			return peerhttp.DownloadDeviceHistoryAudio404JSONResponse{NotFoundJSONResponse: peerhttp.NotFoundJSONResponse(apiError("HISTORY_NOT_FOUND", "history audio not found"))}, nil
		}
		return peerhttp.DownloadDeviceHistoryAudio500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.DownloadDeviceHistoryAudio200AudiooggResponse{Body: r, ContentLength: size}, nil
}
