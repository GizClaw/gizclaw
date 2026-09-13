package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"sort"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/app"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type peerAppClient struct {
	conn   giznet.Conn
	apps   *app.Server
	status *atomic.Pointer[peerAppStatus]
}

// peerAppStatus is immutable after publication. Only completed reconciliation
// exposes methods; network and catalog operations never hold a state lock.
type peerAppStatus struct {
	runtime   string
	installed map[string]string
	ready     bool
}

func (h *PeerConn) publishAppStatus(runtime string, installed map[string]string, ready bool) {
	h.appStatus.Store(&peerAppStatus{runtime: runtime, installed: maps.Clone(installed), ready: ready})
}

func appCall[T any](ctx context.Context, conn giznet.Conn, method rpcapi.RPCMethod, params rpcapi.RPCPayload, decode func(rpcapi.RPCPayload) (T, error)) (T, error) {
	var zero T
	if conn == nil {
		return zero, agenthost.ErrAppUnavailable
	}
	var stream net.Conn
	var err error
	if dialer, ok := conn.(giznet.ContextDialer); ok {
		stream, err = dialer.DialContext(ctx, 0)
	} else {
		stream, err = conn.Dial(0)
	}
	if err != nil {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		return zero, agenthost.ErrAppUnavailable
	}
	defer stream.Close()
	response, err := callRPC(ctx, stream, newRPCRequest("app", method, &params))
	if err != nil {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		return zero, fmt.Errorf("%w: %v", agenthost.ErrAppUnavailable, err)
	}
	if response.Id != "app" || response.Error != nil || response.Result == nil {
		return zero, agenthost.ErrAppUnavailable
	}
	return decode(*response.Result)
}

func (p peerAppClient) list(ctx context.Context) (*rpcpb.ClientAppListResponse, error) {
	var params rpcapi.RPCPayload
	if err := params.FromClientAppListRequest(&rpcpb.ClientAppListRequest{}); err != nil {
		return nil, err
	}
	return appCall(ctx, p.conn, rpcapi.RPCMethodClientAppList, params, rpcapi.RPCPayload.AsClientAppListResponse)
}

func (p peerAppClient) ResolveApps(ctx context.Context, ids []string) ([]apitypes.App, error) {
	if len(ids) == 0 || p.apps == nil {
		return nil, nil
	}
	if p.status == nil {
		return nil, nil
	}
	status := p.status.Load()
	if status == nil || !status.ready {
		return nil, nil
	}
	var result []apitypes.App
	for _, id := range ids {
		value, err := p.apps.Get(ctx, id)
		if errors.Is(err, app.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if value.Runtime == status.runtime && status.installed[value.AppName] == value.Sha256 {
			result = append(result, value)
		}
	}
	return result, nil
}

func (p peerAppClient) InvokeApp(ctx context.Context, value apitypes.App, method apitypes.AppMethod, args json.RawMessage) (json.RawMessage, error) {
	var params rpcapi.RPCPayload
	if method.Mode == "job" {
		if err := params.FromClientAppJobStartRequest(&rpcpb.ClientAppJobStartRequest{AppName: value.AppName, Method: method.Name, ArgsJson: string(args)}); err != nil {
			return nil, err
		}
		result, err := appCall(ctx, p.conn, rpcapi.RPCMethodClientAppJobStart, params, rpcapi.RPCPayload.AsClientAppJobStartResponse)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]uint32{"job_id": result.JobId})
	}
	if err := params.FromClientAppInvokeRequest(&rpcpb.ClientAppInvokeRequest{AppName: value.AppName, Method: method.Name, ArgsJson: string(args)}); err != nil {
		return nil, err
	}
	result, err := appCall(ctx, p.conn, rpcapi.RPCMethodClientAppInvoke, params, rpcapi.RPCPayload.AsClientAppInvokeResponse)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(result.ResultJson), nil
}

func (h *PeerConn) reconcileApps() error {
	h.appStatus.Store(nil)
	profile := h.currentRuntimeProfile()
	if profile == nil || profile.Spec.Resources.Apps == nil || h.Service.manager.Apps == nil {
		return nil
	}
	client := peerAppClient{conn: h.Conn, apps: h.Service.manager.Apps, status: &h.appStatus}
	reconcileCtx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	ctx, cancel := context.WithTimeout(reconcileCtx, 10*time.Second)
	status, err := client.list(ctx)
	cancel()
	if err != nil {
		slog.Warn("peer App list failed", "error", err)
		return nil
	}
	installed := map[string]string{}
	for _, value := range status.Apps {
		if value != nil {
			installed[value.AppName] = value.Sha256
		}
	}
	h.publishAppStatus(status.Runtime, installed, false)
	complete := true
	var aliases []string
	for alias := range *profile.Spec.Resources.Apps {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	for _, alias := range aliases {
		if reconcileCtx.Err() != nil {
			complete = false
			slog.Warn("peer App reconciliation timed out")
			break
		}
		id := (*profile.Spec.Resources.Apps)[alias].ResourceId
		ctx, cancel := context.WithTimeout(reconcileCtx, 10*time.Second)
		value, err := client.apps.Get(ctx, id)
		if err == nil && value.Runtime == status.Runtime && installed[value.AppName] != value.Sha256 {
			var params rpcapi.RPCPayload
			err = params.FromClientAppInstallRequest(&rpcpb.ClientAppInstallRequest{AppName: value.AppName, Url: value.Package.Url, Sha256: value.Sha256, Size: value.Package.Size})
			if err == nil {
				_, err = appCall(ctx, client.conn, rpcapi.RPCMethodClientAppInstall, params, rpcapi.RPCPayload.AsClientAppInstallResponse)
			}
			if err == nil {
				installed[value.AppName] = value.Sha256
				h.publishAppStatus(status.Runtime, installed, false)
			}
		}
		cancel()
		if err != nil {
			complete = false
			slog.Warn("peer App reconciliation failed", "app_id", id, "error", err)
		}
	}
	if complete {
		h.publishAppStatus(status.Runtime, installed, true)
	}
	return nil
}
