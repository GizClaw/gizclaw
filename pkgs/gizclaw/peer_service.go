package gizclaw

import (
	"context"
	"errors"
	"net"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/publiclogin"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	ServiceRPC          uint64 = 0x00
	ServiceServerPublic uint64 = 0x01
	ServiceOpenAI       uint64 = 0x02
	ServiceAdmin        uint64 = 0x10
	ServiceEvent        uint64 = 0x20

	ProtocolEvent       byte = 0x03
	ProtocolStampedOpus byte = 0x10
)

type serverPublic struct {
	peer.ServerPublicService
	publiclogin.ServerPublic
	WebRTCSignalingHandler func() http.Handler
}

// PeerService serves one peer connection.
type PeerService struct {
	admin   *adminService
	public  *serverPublic
	manager *Manager
}

var _ serverpublic.StrictServerInterface = (*serverPublic)(nil)

func (s *PeerService) ServeConn(conn giznet.Conn) error {
	if s == nil {
		return errors.New("gizclaw: nil peer service")
	}
	if conn == nil {
		return errors.New("gizclaw: nil conn")
	}
	defer func() {
		_ = conn.Close()
	}()
	if err := s.validateServices(); err != nil {
		return err
	}
	if err := s.ensureConnectedPeer(context.Background(), conn); err != nil {
		return err
	}
	publicKey := conn.PublicKey()
	s.manager.SetPeerUp(publicKey, conn)
	defer s.manager.SetPeerDown(publicKey)

	var g errgroup.Group
	g.Go(func() error { return s.serveAdmin(conn) })
	g.Go(func() error { return s.servePublic(conn) })

	if err := g.Wait(); err != nil && !isPeerServiceClosed(err) {
		return err
	}
	return nil
}

func isPeerServiceClosed(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, giznet.ErrClosed) ||
		errors.Is(err, giznet.ErrConnClosed) ||
		errors.Is(err, giznet.ErrServiceMuxClosed)
}

func (s *PeerService) ensureConnectedPeer(ctx context.Context, conn giznet.Conn) error {
	if s == nil || s.manager == nil {
		return nil
	}
	_, err := s.manager.EnsurePeer(ctx, conn.PublicKey())
	return err
}

func (s *PeerService) validateServices() error {
	switch {
	case s.admin == nil:
		return errors.New("gizclaw: nil admin service")
	case s.manager == nil:
		return errors.New("gizclaw: nil manager")
	case s.public == nil:
		return errors.New("gizclaw: nil public service")
	default:
		return nil
	}
}
