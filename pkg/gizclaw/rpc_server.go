package gizclaw

import (
	"context"
	"fmt"
	"net"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peergenx"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

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

type rpcPeerRunRuntime interface {
	Reload(context.Context) (apitypes.PeerRunStatus, error)
	Status(context.Context) (apitypes.PeerRunStatus, error)
	Stop(context.Context) (apitypes.PeerRunStatus, error)
}

type rpcServerResourceService interface {
	Dispatch(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, bool, error)
}

type rpcFriendOTPReporter interface {
	ReportFriendOTP(context.Context, string, string) error
}

type rpcServerGenXService interface {
	Say(context.Context, peergenx.SayRequest) (peergenx.SayResponse, error)
}

type rpcServer struct {
	peer            rpcPeerService
	peerRun         rpcPeerRunService
	peerRunRuntime  rpcPeerRunRuntime
	serverResources rpcServerResourceService
	friendOTPs      rpcFriendOTPReporter
	serverGenX      rpcServerGenXService
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
	case rpcapi.RPCMethodAllSpeedTestRun:
		return true, s.handleSpeedTest(ctx, stream, req)
	case rpcapi.RPCMethodServerFirmwareDownload:
		return true, s.handleFirmwareBinDownload(ctx, stream, req)
	default:
		return false, nil
	}
}

func (s *rpcServer) dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return rpcapi.Error{Code: rpcapi.RPCErrorCodeInvalidRequest, Message: "nil request"}.RPCResponse(), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodAllPing:
		return handleRPCPing(ctx, req)
	case rpcapi.RPCMethodServerInfoGet:
		return s.handleGetInfo(ctx, req)
	case rpcapi.RPCMethodServerInfoPut:
		return s.handlePutInfo(ctx, req)
	case rpcapi.RPCMethodServerRuntimeGet:
		return s.handleGetRuntime(ctx, req)
	case rpcapi.RPCMethodServerStatusGet:
		return s.handleGetStatus(ctx, req)
	case rpcapi.RPCMethodServerStatusPut:
		return s.handlePutStatus(ctx, req)
	case rpcapi.RPCMethodServerRunAgentGet:
		return s.handleGetRunAgent(ctx, req)
	case rpcapi.RPCMethodServerRunAgentSet:
		return s.handleSetRunAgent(ctx, req)
	case rpcapi.RPCMethodServerRunReload:
		return s.handleReloadRun(ctx, req)
	case rpcapi.RPCMethodServerRunStatus:
		return s.handleGetRunStatus(ctx, req)
	case rpcapi.RPCMethodServerRunStop:
		return s.handleStopRun(ctx, req)
	case rpcapi.RPCMethodServerRunSay:
		return s.handleServerRunSay(ctx, req)
	default:
		if s.serverResources != nil {
			resp, handled, err := s.serverResources.Dispatch(ctx, req)
			if handled || err != nil {
				return resp, err
			}
		}
		if isPlannedServerMethod(req.Method) {
			return rpcNotImplemented(req.Id, req.Method), nil
		}
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: fmt.Sprintf("unknown method: %s", req.Method)}.RPCResponse(), nil
	}
}
