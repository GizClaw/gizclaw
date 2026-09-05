package gizclaw

import (
	"context"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

func (s *peerHTTP) GetDeviceLogs(ctx context.Context, _ peerhttp.GetDeviceLogsRequestObject) (peerhttp.GetDeviceLogsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetDeviceLogs401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	logs := gizlog.ReadMonitorLogs(owner.String())
	result := make(peerhttp.GetDeviceLogs200JSONResponse, 0, len(logs))
	for _, e := range logs {
		result = append(result, peerhttp.DeviceMonitorLog{Id: e.ID, Time: e.Time, Level: e.Level, Message: e.Message, Error: &e.Error, PeerPublicKey: &e.PeerPublicKey})
	}
	return result, nil
}
