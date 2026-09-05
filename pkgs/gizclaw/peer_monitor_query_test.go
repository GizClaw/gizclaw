package gizclaw

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
)

type monitorLogQuery struct {
	request ServerLogStreamRequest
	foreign bool
}

func (q *monitorLogQuery) StreamServerLogs(_ context.Context, req ServerLogStreamRequest, emit func(apitypes.ServerLogEntry) error) (apitypes.ServerLogStreamEnd, error) {
	q.request = req
	if q.foreign {
		if err := emit(apitypes.ServerLogEntry{Fields: map[string]string{"peer_public_key": "foreign"}}); err != nil {
			return apitypes.ServerLogStreamEnd{}, err
		}
	}
	return apitypes.ServerLogStreamEnd{}, nil
}
func TestMonitorLogsBindOwnerOnContinuationAndRejectForeignOutput(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ctx := peerhttp.WithCallerPublicKey(t.Context(), key.Public)
	q := &monitorLogQuery{}
	s := &peerHTTP{ServerLogs: q}
	cursor := "foreign-cursor"
	query := `hello" AND peer_public_key:"foreign`
	req := peerhttp.SearchDeviceLogsRequestObject{Params: peerhttp.SearchDeviceLogsParams{StartTimeMs: 1, EndTimeMs: 2, Cursor: &cursor, Query: &query}}
	response, err := s.SearchDeviceLogs(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(peerhttp.SearchDeviceLogs200JSONResponse); !ok {
		t.Fatalf("response=%T", response)
	}
	filter, err := parseServerLogFilter(q.request.Filter)
	if err != nil {
		t.Fatal(err)
	}
	if !q.request.FilterSet || len(filter.Matchers) != 1 || filter.Matchers[0].Name != "peer_public_key" || filter.Matchers[0].Value != key.Public.String() || filter.Text != query {
		t.Fatalf("owner filter not enforced: %+v", q.request)
	}
	q.foreign = true
	response, err = s.SearchDeviceLogs(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(peerhttp.SearchDeviceLogs500JSONResponse); !ok {
		t.Fatalf("foreign records exposed: %T", response)
	}
}
func TestMonitorHistoryForeignWorkspaceIsNotFound(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := kv.NewMemory(nil)
	now := time.Now().UTC()
	foreign := apitypes.Workspace{Id: "foreign", Name: "foreign", WorkflowId: "flow", OwnerPublicKey: new("another-peer"), System: new(false), CreatedAt: now, UpdatedAt: now, LastActiveAt: now}
	data, err := json.Marshal(foreign)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(t.Context(), kv.Key{"by-id", "foreign"}, data); err != nil {
		t.Fatal(err)
	}
	s := &peerHTTP{Workspaces: &workspace.Server{Store: store}}
	response, err := s.ListDeviceWorkspaceHistory(peerhttp.WithCallerPublicKey(t.Context(), key.Public), peerhttp.ListDeviceWorkspaceHistoryRequestObject{WorkspaceId: "foreign"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(peerhttp.ListDeviceWorkspaceHistory404JSONResponse); !ok {
		t.Fatalf("response=%T", response)
	}
}

func TestMonitorLogCursorCannotCrossPeer(t *testing.T) {
	first, _ := giznet.GenerateKeyPair()
	second, _ := giznet.GenerateKeyPair()
	backend := &fakeLogQuerier{page: logstore.Page{HasNext: true, NextCursor: "backend-page"}}
	s := &peerHTTP{ServerLogs: newTestServerLogQueryService(t, backend)}
	req := peerhttp.SearchDeviceLogsRequestObject{Params: peerhttp.SearchDeviceLogsParams{StartTimeMs: 1000, EndTimeMs: 2000}}
	response, err := s.SearchDeviceLogs(peerhttp.WithCallerPublicKey(t.Context(), first.Public), req)
	if err != nil {
		t.Fatal(err)
	}
	page, ok := response.(peerhttp.SearchDeviceLogs200JSONResponse)
	if !ok || page.End.NextCursor == nil {
		t.Fatalf("response=%+v", response)
	}
	req.Params.Cursor = page.End.NextCursor
	response, err = s.SearchDeviceLogs(peerhttp.WithCallerPublicKey(t.Context(), second.Public), req)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(peerhttp.SearchDeviceLogs400JSONResponse); !ok {
		t.Fatalf("cross-peer cursor accepted: %T", response)
	}
	if len(backend.queries) != 1 {
		t.Fatal("foreign continuation reached store")
	}
}

func TestMonitorLogsRejectUnsupportedLevel(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"TRACE", "info", "", "WARN,ERROR"} {
		q := &monitorLogQuery{}
		s := &peerHTTP{ServerLogs: q}
		level := peerhttp.SearchDeviceLogsParamsLevel(value)
		response, err := s.SearchDeviceLogs(peerhttp.WithCallerPublicKey(t.Context(), key.Public), peerhttp.SearchDeviceLogsRequestObject{Params: peerhttp.SearchDeviceLogsParams{Level: &level, StartTimeMs: 1000, EndTimeMs: 2000}})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := response.(peerhttp.SearchDeviceLogs400JSONResponse); !ok {
			t.Fatalf("level %q returned %T", value, response)
		}
		if q.request.FilterSet {
			t.Fatal("invalid level reached Log Store")
		}
	}
}
