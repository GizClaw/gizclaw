package peerapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type peerHTTPAPI interface {
	GetServerInfoWithResponse(ctx context.Context, reqEditors ...peerhttp.RequestEditorFn) (*peerhttp.GetServerInfoResponse, error)
}

var defaultPeerHTTPClientFrom = func(c *gizcli.Client) (peerHTTPAPI, error) {
	return c.PeerHTTPClient()
}

var peerHTTPClientFrom = defaultPeerHTTPClientFrom

func GetServerInfo(ctx context.Context, c *gizcli.Client) (apitypes.ServerInfo, error) {
	api, err := peerHTTPClientFrom(c)
	if err != nil {
		return apitypes.ServerInfo{}, err
	}
	resp, err := api.GetServerInfoWithResponse(ctx)
	if err != nil {
		return apitypes.ServerInfo{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.ServerInfo{}, responseError(resp.StatusCode(), resp.Body, resp.JSON400)
}

func responseError(status int, body []byte, errs ...interface{}) error {
	for _, errResp := range errs {
		switch e := errResp.(type) {
		case *apitypes.ErrorResponse:
			if e != nil {
				return fmt.Errorf("%s: %s", e.Error.Code, e.Error.Message)
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if text != "" {
		return fmt.Errorf("unexpected status %d: %s", status, text)
	}
	if status != 0 {
		return fmt.Errorf("unexpected status %d", status)
	}
	return fmt.Errorf("unexpected empty response")
}
