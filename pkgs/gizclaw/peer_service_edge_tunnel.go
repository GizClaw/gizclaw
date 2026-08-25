package gizclaw

import (
	"context"
	"errors"
	"log/slog"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

const (
	edgeTunnelMaxChannelsPerSession = 32
	edgeTunnelMaxChannels           = 8192
	edgeTunnelMaxPendingSessions    = 2048
	edgeTunnelSessionBufferBytes    = 1 << 20
)

func (h *PeerConn) initEdgeTunnelRouter() (*giztunnel.Router, error) {
	if h == nil || h.Conn == nil || h.Service == nil || h.Service.manager == nil {
		return nil, errors.New("gizclaw: edge tunnel is not configured")
	}
	if h.tunnelRouter != nil {
		return h.tunnelRouter, nil
	}
	transport, ok := h.Conn.(*gizwebrtc.Conn)
	if !ok {
		return nil, errors.New("gizclaw: edge tunnel requires WebRTC transport")
	}
	router, err := giztunnel.NewRouter(transport, giztunnel.Config{
		AcceptSessions:        true,
		MaxChannelsPerSession: edgeTunnelMaxChannelsPerSession,
		MaxChannels:           edgeTunnelMaxChannels,
		MaxPendingSessions:    edgeTunnelMaxPendingSessions,
		MaxBufferedBytes:      edgeTunnelSessionBufferBytes,
		AllowRemoteService: func(client giznet.PublicKey, service uint64) bool {
			return h.Service.manager.allowService(context.Background(), client, service)
		},
	})
	if err != nil {
		return nil, err
	}
	h.tunnelRouter = router
	return router, nil
}

func (h *PeerConn) serveEdgeTunnel() error {
	router, err := h.initEdgeTunnelRouter()
	if err != nil {
		return err
	}
	for {
		logical, declaration, err := router.Accept(context.Background())
		if err != nil {
			if isPeerServiceClosed(err) {
				return nil
			}
			return err
		}
		lifecycle := newPeerStreamLifecycle(
			slog.Default(),
			declaration.SessionID.String(),
			declaration.ClientPublicKey.String(),
		)
		lifecycle.accepted()
		host := &PeerConn{
			Conn:            logical,
			Service:         h.Service,
			ServerPublicKey: h.ServerPublicKey,
			streamLifecycle: lifecycle,
		}
		go func() {
			err := host.serve()
			lifecycle.finish("server_tunnel", err)
			if err != nil {
				_ = logical.Close()
			}
		}()
	}
}
