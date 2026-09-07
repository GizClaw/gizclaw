package peerresource

import (
	"context"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

// handleAppConfigList pages the opaque app_config keys of the selected
// RuntimeProfile. Only keys are returned; values are read one at a time so a
// page always fits one RPC frame.
func (s *Server) handleAppConfigList(_ context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsAppConfigListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	keys := sortedAppConfigKeys(profile.Spec.AppConfig)
	page, hasNext, nextCursor, conflict := pageAliases(keys, params.Cursor, params.Limit, profile.Revision)
	if conflict {
		return statusError(req.Id, rpcapi.StatusCodeAborted, "runtime profile revision changed")
	}
	return resultResponse(req.Id, rpcapi.AppConfigListResponse{
		Keys: page, HasNext: hasNext, NextCursor: nextCursor,
		RuntimeProfileName: profile.Id, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromAppConfigListResponse)
}

// handleAppConfigGet returns one app_config value verbatim. The Server never
// parses the value and never exposes a write path for it.
func (s *Server) handleAppConfigGet(_ context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsAppConfigGetRequest)
	key := strings.TrimSpace(params.Key)
	if !ok || key == "" {
		return invalidParams(req.Id)
	}
	profile := s.currentRuntimeProfile()
	if profile == nil {
		return internalError(req.Id, "runtime profile not configured")
	}
	if profile.Spec.AppConfig == nil {
		return statusError(req.Id, rpcapi.StatusCodeNotFound, "app config key not found")
	}
	value, found := (*profile.Spec.AppConfig)[key]
	if !found {
		return statusError(req.Id, rpcapi.StatusCodeNotFound, "app config key not found")
	}
	return resultResponse(req.Id, rpcapi.AppConfigGetResponse{
		Value: value, RuntimeProfileName: profile.Id, RuntimeProfileRevision: profile.Revision,
	}, (*rpcapi.RPCPayload).FromAppConfigGetResponse)
}

func sortedAppConfigKeys(config *apitypes.RuntimeProfileAppConfig) []string {
	if config == nil {
		return nil
	}
	keys := make([]string, 0, len(*config))
	for key := range *config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
