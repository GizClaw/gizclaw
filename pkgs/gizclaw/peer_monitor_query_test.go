package gizclaw

import (
	"context"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
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
	workspaces := workspacetest.New(t)
	now := time.Now().UTC()
	foreign := apitypes.Workspace{Id: "foreign", Name: "foreign", WorkflowId: "flow", OwnerPublicKey: new("another-peer"), System: new(false), CreatedAt: now, UpdatedAt: now, LastActiveAt: now}
	workspacetest.Seed(t, workspaces, foreign)
	s := &peerHTTP{Workspaces: workspaces}
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

func TestMonitorHistoryHonorsOrderAndTimeRange(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	workspaces := workspacetest.New(t)
	workspaces.RuntimeStore = newTestWorkspaceRuntimeStore(t, newTestObjectStore(t))
	now := time.Now().UTC()
	owned := apitypes.Workspace{Id: "owned", Name: "owned", WorkflowId: "flow", OwnerPublicKey: new(key.Public.String()), System: new(false), CreatedAt: now, UpdatedAt: now, LastActiveAt: now}
	workspacetest.Seed(t, workspaces, owned)
	runtime, err := workspaces.RuntimeStore.GetWorkspaceRuntime(t.Context(), "owned")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for day := range 3 {
		entry, err := runtime.History.Append(t.Context(), workspace.AppendHistoryRequest{Type: "agent", Name: "assistant", Text: "hello", CreatedAt: base.AddDate(0, 0, day)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, entry.ID)
	}
	s := &peerHTTP{Workspaces: workspaces}
	ctx := peerhttp.WithCallerPublicKey(t.Context(), key.Public)
	list := func(params peerhttp.ListDeviceWorkspaceHistoryParams) peerhttp.ListDeviceWorkspaceHistoryResponseObject {
		t.Helper()
		response, err := s.ListDeviceWorkspaceHistory(ctx, peerhttp.ListDeviceWorkspaceHistoryRequestObject{WorkspaceId: "owned", Params: params})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	names := func(response peerhttp.ListDeviceWorkspaceHistoryResponseObject) []string {
		t.Helper()
		page, ok := response.(peerhttp.ListDeviceWorkspaceHistory200JSONResponse)
		if !ok {
			t.Fatalf("response=%T", response)
		}
		out := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			out = append(out, item.Name)
		}
		return out
	}

	if got := names(list(peerhttp.ListDeviceWorkspaceHistoryParams{})); !slices.Equal(got, []string{ids[2], ids[1], ids[0]}) {
		t.Fatalf("default order=%v", got)
	}
	asc := peerhttp.Asc
	if got := names(list(peerhttp.ListDeviceWorkspaceHistoryParams{Order: &asc})); !slices.Equal(got, []string{ids[0], ids[1], ids[2]}) {
		t.Fatalf("asc order=%v", got)
	}
	end := base.AddDate(0, 0, 2).UnixMilli()
	if got := names(list(peerhttp.ListDeviceWorkspaceHistoryParams{EndTimeMs: &end})); !slices.Equal(got, []string{ids[1], ids[0]}) {
		t.Fatalf("before end=%v", got)
	}
	start := base.AddDate(0, 0, 1).UnixMilli()
	if got := names(list(peerhttp.ListDeviceWorkspaceHistoryParams{Order: &asc, StartTimeMs: &start})); !slices.Equal(got, []string{ids[1], ids[2]}) {
		t.Fatalf("from start=%v", got)
	}
	if got := names(list(peerhttp.ListDeviceWorkspaceHistoryParams{Order: &asc, Cursor: &ids[0]})); !slices.Equal(got, []string{ids[1], ids[2]}) {
		t.Fatalf("newer than cursor=%v", got)
	}
	for name, params := range map[string]peerhttp.ListDeviceWorkspaceHistoryParams{
		"inverted range": {StartTimeMs: &end, EndTimeMs: &start},
		"negative start": {StartTimeMs: new(int64(-1))},
		"unknown order":  {Order: new(peerhttp.ListDeviceWorkspaceHistoryParamsOrder("sideways"))},
	} {
		if _, ok := list(params).(peerhttp.ListDeviceWorkspaceHistory400JSONResponse); !ok {
			t.Fatalf("%s was accepted", name)
		}
	}
}
