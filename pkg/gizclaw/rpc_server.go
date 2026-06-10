package gizclaw

import (
	"context"
	"fmt"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

type rpcServerInfoService interface {
	GetServerInfo(context.Context, serverpublic.GetServerInfoRequestObject) (serverpublic.GetServerInfoResponseObject, error)
}

type rpcPeerService interface {
	GetSelfInfo(context.Context, giznet.PublicKey) (apitypes.DeviceInfo, error)
	PutSelfInfo(context.Context, giznet.PublicKey, apitypes.DeviceInfo) (apitypes.DeviceInfo, error)
	GetSelfRuntime(context.Context, giznet.PublicKey) apitypes.Runtime
}

type rpcPeerRunService interface {
	GetStatus(context.Context, giznet.PublicKey) (apitypes.PeerStatus, error)
	PutStatus(context.Context, giznet.PublicKey, apitypes.PeerStatus) (apitypes.PeerStatus, error)
	GetRunAgent(context.Context, giznet.PublicKey) (apitypes.PeerRunAgent, error)
	SetRunAgent(context.Context, giznet.PublicKey, apitypes.AgentSelection) (apitypes.PeerRunAgent, error)
}

type rpcServer struct {
	peer            rpcPeerService
	peerRun         rpcPeerRunService
	serverInfo      rpcServerInfoService
	callerPublicKey giznet.PublicKey
}

func (s *rpcServer) Handle(conn net.Conn) error {
	return handleRPCWithStream(conn, s.dispatch, s.dispatchStream)
}

func (s *rpcServer) dispatchStream(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) (bool, error) {
	if req == nil {
		return false, nil
	}
	switch req.Method {
	case rpcapi.RPCMethodPeerSpeedTestRun:
		return true, s.handleSpeedTest(ctx, stream, req)
	default:
		return false, nil
	}
}

func (s *rpcServer) dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return rpcapi.Error{Code: rpcapi.RPCErrorCodeInvalidRequest, Message: "nil request"}.RPCResponse(), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodPeerPing:
		return handleRPCPing(ctx, req)
	case rpcapi.RPCMethodServerInfoGet:
		return s.handleGetServerInfo(ctx, req)
	case rpcapi.RPCMethodPeerInfoGet:
		return s.handleGetInfo(ctx, req)
	case rpcapi.RPCMethodPeerInfoPut:
		return s.handlePutInfo(ctx, req)
	case rpcapi.RPCMethodPeerRuntimeGet:
		return s.handleGetRuntime(ctx, req)
	case rpcapi.RPCMethodPeerStatusGet:
		return s.handleGetStatus(ctx, req)
	case rpcapi.RPCMethodPeerStatusPut:
		return s.handlePutStatus(ctx, req)
	case rpcapi.RPCMethodPeerRunAgentGet:
		return s.handleGetRunAgent(ctx, req)
	case rpcapi.RPCMethodPeerRunAgentSet:
		return s.handleSetRunAgent(ctx, req)
	case rpcapi.RPCMethodPeerRunReload, rpcapi.RPCMethodPeerRunStatus, rpcapi.RPCMethodPeerRunStop:
		return rpcNotImplemented(req.Id, req.Method), nil
	default:
		if isPlannedGearMethod(req.Method) {
			return rpcNotImplemented(req.Id, req.Method), nil
		}
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: fmt.Sprintf("unknown method: %s", req.Method)}.RPCResponse(), nil
	}
}
