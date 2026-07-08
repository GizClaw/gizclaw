package peerapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type fakePeerHTTPAPI struct {
	resp *peerhttp.GetServerInfoResponse
	err  error
}

func (f fakePeerHTTPAPI) GetServerInfoWithResponse(context.Context, ...peerhttp.RequestEditorFn) (*peerhttp.GetServerInfoResponse, error) {
	return f.resp, f.err
}

func TestGetServerInfoReturnsJSON200(t *testing.T) {
	t.Cleanup(func() { peerHTTPClientFrom = defaultPeerHTTPClientFrom })
	want := apitypes.ServerInfo{PublicKey: "pk"}
	peerHTTPClientFrom = func(*gizcli.Client) (peerHTTPAPI, error) {
		return fakePeerHTTPAPI{resp: &peerhttp.GetServerInfoResponse{JSON200: &want}}, nil
	}
	got, err := GetServerInfo(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetServerInfo error = %v", err)
	}
	if got.PublicKey != want.PublicKey {
		t.Fatalf("PublicKey = %q, want %q", got.PublicKey, want.PublicKey)
	}
}

func TestGetServerInfoPropagatesClientError(t *testing.T) {
	t.Cleanup(func() { peerHTTPClientFrom = defaultPeerHTTPClientFrom })
	peerHTTPClientFrom = func(*gizcli.Client) (peerHTTPAPI, error) {
		return nil, errors.New("offline")
	}
	_, err := GetServerInfo(context.Background(), nil)
	if err == nil || err.Error() != "offline" {
		t.Fatalf("GetServerInfo error = %v", err)
	}
}

func TestGetServerInfoPropagatesRequestError(t *testing.T) {
	t.Cleanup(func() { peerHTTPClientFrom = defaultPeerHTTPClientFrom })
	peerHTTPClientFrom = func(*gizcli.Client) (peerHTTPAPI, error) {
		return fakePeerHTTPAPI{err: errors.New("request failed")}, nil
	}
	_, err := GetServerInfo(context.Background(), nil)
	if err == nil || err.Error() != "request failed" {
		t.Fatalf("GetServerInfo error = %v", err)
	}
}

func TestGetServerInfoConvertsResponseError(t *testing.T) {
	t.Cleanup(func() { peerHTTPClientFrom = defaultPeerHTTPClientFrom })
	peerHTTPClientFrom = func(*gizcli.Client) (peerHTTPAPI, error) {
		return fakePeerHTTPAPI{resp: &peerhttp.GetServerInfoResponse{Body: []byte("bad"), HTTPResponse: nil}}, nil
	}
	_, err := GetServerInfo(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("GetServerInfo error = %v", err)
	}
}

func TestResponseErrorUsesStructuredError(t *testing.T) {
	err := responseError(400, nil, &apitypes.ErrorResponse{
		Error: apitypes.ErrorPayload{Code: "bad_request", Message: "missing field"},
	})
	if err == nil || err.Error() != "bad_request: missing field" {
		t.Fatalf("responseError() = %v", err)
	}
}

func TestResponseErrorUsesResponseBody(t *testing.T) {
	err := responseError(502, []byte(" upstream failed \n"))
	if err == nil || !strings.Contains(err.Error(), "unexpected status 502: upstream failed") {
		t.Fatalf("responseError() = %v", err)
	}
}

func TestResponseErrorUsesFallbackStatus(t *testing.T) {
	err := responseError(500, nil)
	if err == nil || err.Error() != "unexpected status 500" {
		t.Fatalf("responseError() = %v", err)
	}
}

func TestResponseErrorHandlesEmptyResponse(t *testing.T) {
	err := responseError(0, nil)
	if err == nil || err.Error() != "unexpected empty response" {
		t.Fatalf("responseError() = %v", err)
	}
}
