package gizclaw

import (
	"context"
	"errors"
	"log/slog"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	edgeTunnelMaxChannelsPerSession = 32
	edgeTunnelMaxChannels           = 8192
	edgeTunnelMaxPendingSessions    = 2048
	edgeTunnelAdmissionWorkers      = 32
	edgeTunnelSessionBufferBytes    = 1 << 20
)

func (h *PeerConn) initEdgeTunnelRouter() (*giztunnel.Router, error) {
	if h == nil || h.Conn == nil || h.Service == nil || h.Service.manager == nil {
		return nil, errors.New("gizclaw: edge tunnel is not configured")
	}
	if h.tunnelRouter != nil {
		return h.tunnelRouter, nil
	}
	transport, ok := h.Conn.(giznet.ChannelConn)
	if !ok {
		return nil, errors.New("gizclaw: edge tunnel requires a native-channel transport")
	}
	router, err := giztunnel.NewRouter(transport, giztunnel.Config{
		AcceptSessions:              true,
		AdmitSession:                h.admitEdgeTunnelSession,
		MaxChannelsPerSession:       edgeTunnelMaxChannelsPerSession,
		MaxChannels:                 edgeTunnelMaxChannels,
		MaxPendingSessions:          edgeTunnelMaxPendingSessions,
		MaxBufferedBytes:            edgeTunnelSessionBufferBytes,
		MaxAssociationBufferedBytes: gizwebrtc.GatewaySCTPWriteBudgetSize,
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

// admitEdgeTunnelSession establishes only durable Peer identity and fixed home.
// It does not publish a connection, Presence, PeerRun, address or RuntimeProfile.
// Those remain owned by activation after the mandatory Event Stream is open.
func (h *PeerConn) admitEdgeTunnelSession(ctx context.Context, declaration giztunnel.SessionDeclaration) error {
	manager := h.Service.manager
	if manager.Peers == nil {
		return errors.New("gizclaw: peers service not configured")
	}
	record, err := manager.Peers.EnsureConnectedPeerGuarded(ctx, declaration.ClientPublicKey, nil)
	if err != nil {
		return err
	}
	if record.Role != apitypes.PeerRoleClient || manager.PeerRoutes == nil {
		return nil
	}
	assignment, err := manager.PeerRoutes.Assign(ctx, declaration.ClientPublicKey, nil)
	if errors.Is(err, peer.ErrPeerNotFound) {
		// Match activation's existing bootstrap update recovery after a confirmed
		// Peer creation. Ordinary route queries retain their configured reader.
		assignment, err = manager.PeerRoutes.Assign(kv.WithPrimaryReads(ctx), declaration.ClientPublicKey, nil)
	}
	if !errors.Is(err, peerroute.ErrVersionConflict) {
		return err
	}
	var owner giznet.PublicKey
	if parseErr := owner.UnmarshalText([]byte(assignment.ServerPublicKey)); parseErr != nil || owner.IsZero() || owner == manager.PeerRoutes.ServerPublicKey || assignment.PeerPublicKey != declaration.ClientPublicKey.String() {
		return err
	}
	return &giztunnel.SessionRedirectError{ServerPublicKey: owner}
}

func (h *PeerConn) serveEdgeTunnel() error {
	router, err := h.initEdgeTunnelRouter()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan error, edgeTunnelAdmissionWorkers)
	for range edgeTunnelAdmissionWorkers {
		go func() { results <- h.acceptEdgeTunnelSessions(ctx, router) }()
	}
	err = <-results
	cancel()
	for range edgeTunnelAdmissionWorkers - 1 {
		<-results
	}
	return err
}

// A bounded worker group isolates storage-backed admission from sibling sessions.
// The Router continues to own the handshake deadline and admission/channel quotas.
func (h *PeerConn) acceptEdgeTunnelSessions(ctx context.Context, router *giztunnel.Router) error {
	for {
		logical, declaration, err := router.Accept(ctx)
		if err != nil {
			if isPeerServiceClosed(err) || errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		lifecycle := newAcceptedPeerStreamLifecycle(slog.Default(), declaration)
		lifecycle.accepted()
		host := &PeerConn{
			Conn:                    logical,
			Service:                 h.Service,
			ServerPublicKey:         h.ServerPublicKey,
			streamLifecycle:         lifecycle,
			streamLifecycleDisabled: lifecycle == nil,
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

func newAcceptedPeerStreamLifecycle(
	logger *slog.Logger,
	declaration giztunnel.SessionDeclaration,
) *peerStreamLifecycle {
	return newPeerStreamLifecycle(
		logger,
		declaration.SessionID.String(),
		declaration.ClientPublicKey.String(),
	)
}
