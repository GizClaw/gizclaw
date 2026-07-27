package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giztunnel"
)

const (
	edgeTunnelEnvelopeTTL       = 30 * time.Second
	edgeTunnelFutureSkew        = 5 * time.Second
	edgeTunnelMaxRemoteAddrSize = 256
)

func (h *PeerConn) tunnelPacketMux() *giztunnel.PacketMux {
	if h == nil {
		return nil
	}
	h.tunnelMuxOnce.Do(func() {
		h.tunnelMux = giztunnel.NewPacketMux(h.Conn)
	})
	return h.tunnelMux
}

func (h *PeerConn) serveEdgeTunnel() error {
	if h == nil || h.Conn == nil || h.Service == nil || h.Service.manager == nil {
		return nil
	}
	listener := h.Conn.ListenService(ServiceEdgeTunnel)
	if listener == nil {
		return giznet.ErrNilConn
	}
	defer func() { _ = listener.Close() }()
	for {
		stream, err := listener.Accept()
		if err != nil {
			if isPeerServiceClosed(err) {
				return nil
			}
			return err
		}
		if !h.Service.manager.allowService(context.Background(), h.Conn.PublicKey(), ServiceEdgeTunnel) {
			_ = stream.Close()
			continue
		}
		go h.acceptEdgeTunnel(stream)
	}
}

func (h *PeerConn) acceptEdgeTunnel(stream net.Conn) {
	if stream == nil {
		return
	}
	var clientPublicKey giznet.PublicKey
	peerInfo := &giznet.PeerInfo{}
	logical, _, err := giztunnel.Accept(
		context.Background(),
		stream,
		h.tunnelPacketMux(),
		func(open giztunnel.OpenRequest) error {
			if err := h.Service.validateEdgeTunnelOpen(
				time.Now(),
				h.Conn.PublicKey(),
				h.ServerPublicKey,
				open,
			); err != nil {
				return err
			}
			clientPublicKey = open.ClientPublicKey
			if open.RemoteAddr != "" {
				peerInfo.Endpoint = edgeTunnelAddr(open.RemoteAddr)
			}
			return nil
		},
		giztunnel.Config{
			PeerInfo: peerInfo,
			AllowRemoteService: func(service uint64) bool {
				return h.Service.manager.allowService(context.Background(), clientPublicKey, service)
			},
		},
	)
	if err != nil {
		_ = stream.Close()
		return
	}
	host := &PeerConn{
		Conn:            logical,
		Service:         h.Service,
		ServerPublicKey: h.ServerPublicKey,
	}
	if err := host.serve(); err != nil {
		_ = logical.Close()
	}
}

type edgeTunnelAddr string

func (a edgeTunnelAddr) Network() string { return "edge-gateway" }
func (a edgeTunnelAddr) String() string  { return string(a) }

func (s *PeerService) validateEdgeTunnelOpen(
	now time.Time,
	physicalEdge giznet.PublicKey,
	serverPublicKey giznet.PublicKey,
	open giztunnel.OpenRequest,
) error {
	if s == nil || s.manager == nil {
		return errors.New("gizclaw: tunnel service is not configured")
	}
	if physicalEdge.IsZero() || !open.EdgePublicKey.Equal(physicalEdge) {
		return errors.New("gizclaw: tunnel edge identity mismatch")
	}
	if serverPublicKey.IsZero() || !open.ServerPublicKey.Equal(serverPublicKey) {
		return errors.New("gizclaw: tunnel server identity mismatch")
	}
	if open.ClientPublicKey.IsZero() {
		return errors.New("gizclaw: tunnel client identity is zero")
	}
	nowUnix := now.Unix()
	if open.IssuedAtUnix > now.Add(edgeTunnelFutureSkew).Unix() {
		return errors.New("gizclaw: tunnel envelope issued in the future")
	}
	if open.ExpiresAtUnix <= nowUnix {
		return errors.New("gizclaw: tunnel envelope expired")
	}
	ttl := time.Duration(open.ExpiresAtUnix-open.IssuedAtUnix) * time.Second
	if ttl <= 0 || ttl > edgeTunnelEnvelopeTTL {
		return fmt.Errorf("gizclaw: tunnel envelope validity %s exceeds limit", ttl)
	}
	if len(strings.TrimSpace(open.RemoteAddr)) > edgeTunnelMaxRemoteAddrSize {
		return errors.New("gizclaw: tunnel remote address is too long")
	}
	s.tunnelReplayMu.Lock()
	defer s.tunnelReplayMu.Unlock()
	if s.tunnelReplay == nil {
		s.tunnelReplay = make(map[giztunnel.SessionID]int64)
	}
	for sessionID, expiry := range s.tunnelReplay {
		if expiry <= nowUnix {
			delete(s.tunnelReplay, sessionID)
		}
	}
	if _, exists := s.tunnelReplay[open.SessionID]; exists {
		return errors.New("gizclaw: tunnel session replayed")
	}
	s.tunnelReplay[open.SessionID] = open.ExpiresAtUnix
	return nil
}
