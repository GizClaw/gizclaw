package gizedge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

var errRouteAssignmentNotFound = errors.New("edge: peer assignment not found")

var (
	errAPIKeyUnauthorized     = errors.New("edge: API key is unauthorized")
	errAPIKeyOwnerUnavailable = errors.New("edge: API key owner is unavailable")
)

func (g *Gateway) acquirePeerUpstream(ctx context.Context, peerKey giznet.PublicKey) (*gatewayUpstream, func(), error) {
	if g.resolvePeerRoute != nil {
		assignment, err := g.resolvePeerRoute(ctx, peerKey)
		if errors.Is(err, errRouteAssignmentNotFound) {
			if assignment != nil {
				target, targetErr := g.configuredPoolForAssignment(assignment)
				if targetErr != nil {
					return nil, nil, targetErr
				}
				return target.acquire(ctx)
			}
			return g.poolOrder[0].acquire(ctx)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("edge: resolve peer route: %w", err)
		}
		target, err := g.configuredPoolForAssignment(assignment)
		if err != nil {
			return nil, nil, err
		}
		return target.acquire(ctx)
	}
	var errs []error
	for _, pool := range g.poolOrder {
		entry, release, err := pool.acquire(ctx)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		assignment, resolveErr := resolvePeerAssignment(ctx, entry.conn, peerKey)
		if errors.Is(resolveErr, errRouteAssignmentNotFound) {
			return entry, release, nil
		}
		if resolveErr != nil {
			release()
			errs = append(errs, resolveErr)
			continue
		}
		target, targetErr := g.configuredPoolForAssignment(assignment)
		if targetErr != nil {
			release()
			return nil, nil, targetErr
		}
		if target == pool {
			return entry, release, nil
		}
		release()
		return target.acquire(ctx)
	}
	return nil, nil, fmt.Errorf("edge: resolve peer route: %w", errors.Join(errs...))
}

func (g *Gateway) configuredPoolForAssignment(assignment *rpcpb.PeerAssignment) (*gatewayPool, error) {
	if assignment == nil {
		return nil, errors.New("edge: route returned no assignment")
	}
	var owner giznet.PublicKey
	if err := owner.UnmarshalText([]byte(assignment.GetServerPublicKey())); err != nil || owner.IsZero() {
		return nil, errors.New("edge: route returned invalid server identity")
	}
	target := g.pools[owner]
	if target == nil {
		return nil, errors.New("edge: route returned unconfigured server identity")
	}
	return target, nil
}

func resolvePeerAssignment(ctx context.Context, conn giznet.Conn, peerKey giznet.PublicKey) (*rpcpb.PeerAssignment, error) {
	stream, err := conn.Dial(gizclaw.ServiceEdgeRPC)
	if err != nil {
		return nil, fmt.Errorf("edge: open route rpc: %w", err)
	}
	defer func() { _ = stream.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetDeadline(deadline)
	} else {
		_ = stream.SetDeadline(time.Now().Add(upstreamDialTimeout))
	}
	var params rpcapi.RPCPayload
	if err := params.FromServerRouteResolveRequest(rpcpb.ServerRouteResolveRequest{TargetPeerPublicKey: peerKey.String()}); err != nil {
		return nil, fmt.Errorf("edge: encode route request: %w", err)
	}
	request := &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: "edge-route", Method: rpcapi.RPCMethodServerRouteResolve, Params: &params,
	}
	if err := rpcapi.WriteRequest(stream, request); err != nil {
		return nil, fmt.Errorf("edge: write route request: %w", err)
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		return nil, fmt.Errorf("edge: finish route request: %w", err)
	}
	response, err := rpcapi.ReadResponseForMethod(stream, rpcapi.RPCMethodServerRouteResolve)
	if err != nil {
		return nil, fmt.Errorf("edge: read route response: %w", err)
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		return nil, fmt.Errorf("edge: finish route response: %w", err)
	}
	if response.Error != nil {
		if response.Error.Code == rpcapi.StatusCodeNotFound {
			return nil, errRouteAssignmentNotFound
		}
		return nil, fmt.Errorf("edge: route rpc failed with code %d", response.Error.Code)
	}
	if response.Result == nil {
		return nil, errors.New("edge: route rpc returned no result")
	}
	result, err := response.Result.AsServerRouteResolveResponse()
	if err != nil {
		return nil, fmt.Errorf("edge: decode route response: %w", err)
	}
	if result.Assignment == nil {
		return nil, errors.New("edge: route rpc returned no assignment")
	}
	return result.Assignment, nil
}

func resolveAPIKeyAssignment(ctx context.Context, conn giznet.Conn, apiKey string) (*rpcpb.PeerAssignment, error) {
	stream, err := conn.Dial(gizclaw.ServiceEdgeRPC)
	if err != nil {
		return nil, fmt.Errorf("edge: open API route RPC: %w", err)
	}
	defer func() { _ = stream.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetDeadline(deadline)
	} else {
		_ = stream.SetDeadline(time.Now().Add(upstreamDialTimeout))
	}
	var params rpcapi.RPCPayload
	if err := params.FromServerAPIKeyResolveRequest(rpcpb.ServerAPIKeyResolveRequest{ApiKey: apiKey}); err != nil {
		return nil, errors.New("edge: encode API route request")
	}
	request := &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: "edge-api-route", Method: rpcapi.RPCMethodServerAPIKeyResolve, Params: &params,
	}
	if err := rpcapi.WriteRequest(stream, request); err != nil {
		return nil, fmt.Errorf("edge: write API route request: %w", err)
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		return nil, fmt.Errorf("edge: finish API route request: %w", err)
	}
	response, err := rpcapi.ReadResponseForMethod(stream, rpcapi.RPCMethodServerAPIKeyResolve)
	if err != nil {
		return nil, fmt.Errorf("edge: read API route response: %w", err)
	}
	if err := rpcapi.ReadEOS(stream); err != nil {
		return nil, fmt.Errorf("edge: finish API route response: %w", err)
	}
	if response.Error != nil {
		switch response.Error.Code {
		case rpcapi.StatusCodePermissionDenied:
			return nil, errAPIKeyUnauthorized
		case rpcapi.StatusCodeNotFound:
			return nil, errAPIKeyOwnerUnavailable
		default:
			return nil, fmt.Errorf("edge: API route RPC failed with code %d", response.Error.Code)
		}
	}
	if response.Result == nil {
		return nil, errors.New("edge: API route RPC returned no result")
	}
	result, err := response.Result.AsServerAPIKeyResolveResponse()
	if err != nil {
		return nil, fmt.Errorf("edge: decode API route response: %w", err)
	}
	if result.Assignment == nil {
		return nil, errors.New("edge: API route RPC returned no assignment")
	}
	return result.Assignment, nil
}
