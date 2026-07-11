package gizcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"golang.org/x/sync/errgroup"
)

var _ genx.Transformer = (*Client)(nil)

var (
	defaultRPCStreamTimeout       = 30 * time.Second
	defaultHTTPClientTimeout      = 30 * time.Second
	defaultAdminHTTPClientTimeout = 2 * time.Minute
)

// Client holds device-side peer client configuration.
type Client struct {
	KeyPair       *giznet.KeyPair
	DialTransport DialTransportFunc

	Device      apitypes.DeviceInfo
	ToolInvoker func(context.Context, rpcapi.ToolInvokeRequest) (rpcapi.ToolInvokeResponse, error)

	mu       sync.RWMutex
	listener giznet.Listener
	conn     giznet.Conn
	serverPK giznet.PublicKey
	rpc      *rpcClient

	packetMu          sync.RWMutex
	packetSubscribers map[byte]map[chan []byte]struct{}
	openPeerStream    func(int) (*PeerStream, error)
}

type DialTransportFunc func(key *giznet.KeyPair, serverPK giznet.PublicKey, serverAddr string, securityPolicy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error)

// Transform bridges a local genx stream to the connected peer workspace stream.
func (c *Client) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if c == nil {
		return nil, fmt.Errorf("gizclaw: nil client")
	}
	if input == nil {
		return nil, fmt.Errorf("gizclaw: input stream is required")
	}
	openPeerStream := c.OpenPeerStream
	if c.openPeerStream != nil {
		openPeerStream = c.openPeerStream
	}
	stream, err := openPeerStream(64)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			chunk, err := input.Next()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
					return
				}
				_ = stream.CloseWithError(err)
				return
			}
			if err := stream.Push(ctx, chunk); err != nil {
				_ = stream.CloseWithError(err)
				return
			}
		}
	}()
	return stream, nil
}

// Dial establishes the peer connection and initializes client runtime state.
func (c *Client) Dial(serverPK giznet.PublicKey, serverAddr string) error {
	if c == nil {
		return fmt.Errorf("gizclaw: nil client")
	}
	if c.KeyPair == nil {
		return fmt.Errorf("gizclaw: nil key pair")
	}
	if serverAddr == "" {
		return fmt.Errorf("gizclaw: empty server addr")
	}
	c.mu.RLock()
	alreadyStarted := c.listener != nil || c.conn != nil
	c.mu.RUnlock()
	if alreadyStarted {
		return fmt.Errorf("gizclaw: client already started")
	}
	if c.DialTransport == nil {
		return fmt.Errorf("gizclaw: nil dial transport")
	}

	l, conn, err := c.DialTransport(c.KeyPair, serverPK, serverAddr, clientSecurityPolicy{})
	if err != nil {
		return fmt.Errorf("gizclaw: dial: %w", err)
	}
	c.init(l, conn, serverPK)
	return nil
}

// Serve blocks serving client-side peer services on a connection created by Dial.
func (c *Client) Serve() error {
	if c == nil {
		return fmt.Errorf("gizclaw: nil client")
	}
	if c.PeerConn() == nil {
		return fmt.Errorf("gizclaw: client is not connected")
	}
	var g errgroup.Group
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			_ = c.Close()
		})
	}
	defer stop()
	g.Go(func() error {
		defer stop()
		return c.serveRPC()
	})
	g.Go(func() error {
		defer stop()
		return c.servePackets()
	})
	return g.Wait()
}

func (c *Client) init(listener giznet.Listener, conn giznet.Conn, serverPK giznet.PublicKey) {
	c.listener = listener
	c.conn = conn
	c.serverPK = serverPK
	c.rpc = &rpcClient{peer: c}
}

// Close releases all transport resources.
func (c *Client) Close() error {
	c.mu.Lock()
	conn := c.conn
	listener := c.listener
	c.conn = nil
	c.listener = nil
	c.serverPK = giznet.PublicKey{}
	c.rpc = nil
	c.mu.Unlock()

	var err error
	if conn != nil {
		if closeErr := conn.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if listener != nil {
		if closeErr := listener.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

// HTTPClient returns an HTTP client bound to a peer service.
func (c *Client) HTTPClient(service uint64) *http.Client {
	return c.HTTPClientWithTimeout(service, defaultHTTPClientTimeout)
}

// HTTPClientWithTimeout returns an HTTP client bound to a peer service with the
// provided end-to-end request timeout.
func (c *Client) HTTPClientWithTimeout(service uint64, timeout time.Duration) *http.Client {
	client := gizhttp.NewClient(c.PeerConn(), service)
	client.Timeout = timeout
	return client
}

func (c *Client) ServerAdminClient() (*adminhttp.ClientWithResponses, error) {
	return adminhttp.NewClientWithResponses(
		"http://gizclaw",
		adminhttp.WithHTTPClient(c.HTTPClientWithTimeout(ServiceAdminHTTP, defaultAdminHTTPClientTimeout)),
	)
}

func (c *Client) PeerHTTPClient() (*peerhttp.ClientWithResponses, error) {
	return peerhttp.NewClientWithResponses(
		"http://gizclaw",
		peerhttp.WithHTTPClient(c.HTTPClient(ServicePeerHTTP)),
	)
}

// Ping opens a fresh RPC stream, sends one ping, and closes it.
//
// Our current RPC transport uses one giznet service stream per round trip so
// multiple RPC requests can run concurrently on separate streams. This is closer to
// HTTP/1.0-style request lifecycles; HTTP/1.1-style stream reuse is not
// supported yet.
func (c *Client) Ping(ctx context.Context, id string) (*rpcapi.PingResponse, error) {
	stream, err := c.rpcConn()
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	return c.rpcClient().Ping(ctx, stream, id)
}

func (c *Client) GetServerInfo(ctx context.Context, id string) (*rpcapi.ServerGetInfoResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGetInfoResponse, error) {
		return client.GetServerInfo(ctx, conn, id)
	})
}

func (c *Client) PutServerInfo(ctx context.Context, id string, request rpcapi.ServerPutInfoRequest) (*rpcapi.ServerPutInfoResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPutInfoResponse, error) {
		return client.PutServerInfo(ctx, conn, id, request)
	})
}

func (c *Client) GetServerRuntime(ctx context.Context, id string) (*rpcapi.ServerGetRuntimeResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGetRuntimeResponse, error) {
		return client.GetServerRuntime(ctx, conn, id)
	})
}

func (c *Client) GetServerStatus(ctx context.Context, id string) (*rpcapi.ServerGetStatusResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGetStatusResponse, error) {
		return client.GetServerStatus(ctx, conn, id)
	})
}

func (c *Client) GetServerRunAgent(ctx context.Context, id string) (*rpcapi.ServerGetRunAgentResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGetRunAgentResponse, error) {
		return client.GetServerRunAgent(ctx, conn, id)
	})
}

func (c *Client) SetServerRunAgent(ctx context.Context, id string, request rpcapi.ServerSetRunAgentRequest) (*rpcapi.ServerSetRunAgentResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerSetRunAgentResponse, error) {
		return client.SetServerRunAgent(ctx, conn, id, request)
	})
}

func (c *Client) GetServerRunWorkspace(ctx context.Context, id string) (*rpcapi.ServerGetRunWorkspaceResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGetRunWorkspaceResponse, error) {
		return client.GetServerRunWorkspace(ctx, conn, id)
	})
}

func (c *Client) SetServerRunWorkspace(ctx context.Context, id string, request rpcapi.ServerSetRunWorkspaceRequest) (*rpcapi.ServerSetRunWorkspaceResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerSetRunWorkspaceResponse, error) {
		return client.SetServerRunWorkspace(ctx, conn, id, request)
	})
}

func (c *Client) ReloadServerRunWorkspace(ctx context.Context, id string) (*rpcapi.ServerReloadRunWorkspaceResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerReloadRunWorkspaceResponse, error) {
		return client.ReloadServerRunWorkspace(ctx, conn, id)
	})
}

func (c *Client) ListServerRunWorkspaceHistory(ctx context.Context, id string, request rpcapi.ServerListRunWorkspaceHistoryRequest) (*rpcapi.ServerListRunWorkspaceHistoryResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerListRunWorkspaceHistoryResponse, error) {
		return client.ListServerRunWorkspaceHistory(ctx, conn, id, request)
	})
}

func (c *Client) PlayServerRunWorkspaceHistory(ctx context.Context, id string, request rpcapi.ServerPlayRunWorkspaceHistoryRequest) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPlayRunWorkspaceHistoryResponse, error) {
		return client.PlayServerRunWorkspaceHistory(ctx, conn, id, request)
	})
}

func (c *Client) GetServerRunWorkspaceMemoryStats(ctx context.Context, id string, request rpcapi.ServerGetRunWorkspaceMemoryStatsRequest) (*rpcapi.ServerGetRunWorkspaceMemoryStatsResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGetRunWorkspaceMemoryStatsResponse, error) {
		return client.GetServerRunWorkspaceMemoryStats(ctx, conn, id, request)
	})
}

func (c *Client) ServerRunWorkspaceRecall(ctx context.Context, id string, request rpcapi.ServerRunWorkspaceRecallRequest) (*rpcapi.ServerRunWorkspaceRecallResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerRunWorkspaceRecallResponse, error) {
		return client.ServerRunWorkspaceRecall(ctx, conn, id, request)
	})
}

func (c *Client) ReloadServerRun(ctx context.Context, id string) (*rpcapi.ServerReloadRunResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerReloadRunResponse, error) {
		return client.ReloadServerRun(ctx, conn, id)
	})
}

func (c *Client) GetServerRunStatus(ctx context.Context, id string, request ...rpcapi.ServerGetRunStatusRequest) (*rpcapi.ServerGetRunStatusResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerGetRunStatusResponse, error) {
		return client.GetServerRunStatus(ctx, conn, id, request...)
	})
}

func (c *Client) StopServerRun(ctx context.Context, id string) (*rpcapi.ServerStopRunResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerStopRunResponse, error) {
		return client.StopServerRun(ctx, conn, id)
	})
}

func (c *Client) ServerRunSay(ctx context.Context, id string, request rpcapi.ServerRunSayRequest) (*rpcapi.ServerRunSayResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerRunSayResponse, error) {
		return client.ServerRunSay(ctx, conn, id, request)
	})
}

func (c *Client) ListFirmwares(ctx context.Context, id string, request rpcapi.FirmwareListRequest) (*rpcapi.FirmwareListResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FirmwareListResponse, error) {
		return client.ListFirmwares(ctx, conn, id, request)
	})
}

func (c *Client) GetFirmware(ctx context.Context, id string, request rpcapi.FirmwareGetRequest) (*rpcapi.FirmwareGetResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.FirmwareGetResponse, error) {
		return client.GetFirmware(ctx, conn, id, request)
	})
}

func (c *Client) DownloadFirmware(ctx context.Context, id string, request rpcapi.FirmwareFilesDownloadRequest, out io.Writer) (FirmwareDownloadResult, error) {
	stream, err := c.rpcConn()
	if err != nil {
		return FirmwareDownloadResult{}, err
	}
	defer func() { _ = stream.Close() }()
	return c.rpcClient().DownloadFirmware(ctx, stream, id, request, out)
}

func (c *Client) DownloadPetDefPixa(ctx context.Context, id string, request rpcapi.PetDefPixaDownloadRequest, out io.Writer) (PetDefPixaDownloadResult, error) {
	stream, err := c.rpcConn()
	if err != nil {
		return PetDefPixaDownloadResult{}, err
	}
	defer func() { _ = stream.Close() }()
	return c.rpcClient().DownloadPetDefPixa(ctx, stream, id, request, out)
}

func (c *Client) DownloadBadgeDefPixa(ctx context.Context, id string, request rpcapi.BadgeDefPixaDownloadRequest, out io.Writer) (BadgeDefPixaDownloadResult, error) {
	stream, err := c.rpcConn()
	if err != nil {
		return BadgeDefPixaDownloadResult{}, err
	}
	defer func() { _ = stream.Close() }()
	return c.rpcClient().DownloadBadgeDefPixa(ctx, stream, id, request, out)
}

func (c *Client) GetWorkspaceHistoryAudio(ctx context.Context, id string, request rpcapi.WorkspaceHistoryAudioGetRequest, out io.Writer) (WorkspaceHistoryAudioGetResult, error) {
	stream, err := c.rpcConn()
	if err != nil {
		return WorkspaceHistoryAudioGetResult{}, err
	}
	defer func() { _ = stream.Close() }()
	return c.rpcClient().GetWorkspaceHistoryAudio(ctx, stream, id, request, out)
}

func (c *Client) ServerPeerLookup(ctx context.Context, id string, request rpcapi.ServerPeerLookupRequest) (*rpcapi.ServerPeerLookupResponse, error) {
	return callClientServiceRPC(c, ServiceEdgeRPC, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPeerLookupResponse, error) {
		return client.ServerPeerLookup(ctx, conn, id, request)
	})
}

func (c *Client) ServerPeerAssign(ctx context.Context, id string, request rpcapi.ServerPeerAssignRequest) (*rpcapi.ServerPeerAssignResponse, error) {
	return callClientServiceRPC(c, ServiceEdgeRPC, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerPeerAssignResponse, error) {
		return client.ServerPeerAssign(ctx, conn, id, request)
	})
}

func (c *Client) ServerRouteResolve(ctx context.Context, id string, request rpcapi.ServerRouteResolveRequest) (*rpcapi.ServerRouteResolveResponse, error) {
	return callClientServiceRPC(c, ServiceEdgeRPC, func(client *rpcClient, conn net.Conn) (*rpcapi.ServerRouteResolveResponse, error) {
		return client.ServerRouteResolve(ctx, conn, id, request)
	})
}

func callClientRPC[T any](c *Client, call func(*rpcClient, net.Conn) (*T, error)) (*T, error) {
	return callClientServiceRPC(c, ServicePeerRPC, call)
}

func callClientServiceRPC[T any](c *Client, service uint64, call func(*rpcClient, net.Conn) (*T, error)) (*T, error) {
	stream, err := c.rpcConnForService(service)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	return call(c.rpcClient(), stream)
}

func (c *Client) rpcConn() (net.Conn, error) {
	return c.rpcConnForService(ServicePeerRPC)
}

func (c *Client) rpcConnForService(service uint64) (net.Conn, error) {
	conn := c.PeerConn()
	if conn == nil {
		return nil, fmt.Errorf("gizclaw: client is not connected")
	}
	stream, err := conn.Dial(service)
	if err != nil {
		return nil, fmt.Errorf("gizclaw: dial rpc stream: %w", err)
	}
	if err := stream.SetDeadline(time.Now().Add(defaultRPCStreamTimeout)); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("gizclaw: set rpc stream deadline: %w", err)
	}
	return stream, nil
}

func (c *Client) rpcClient() *rpcClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rpc == nil {
		c.rpc = &rpcClient{peer: c}
	}
	if c.rpc.peer == nil {
		c.rpc.peer = c
	}
	return c.rpc
}

// PeerConn returns the underlying peer connection.
func (c *Client) PeerConn() giznet.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// ServerPublicKey returns the expected remote server public key.
func (c *Client) ServerPublicKey() giznet.PublicKey {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serverPK
}

func (c *Client) serveRPC() error {
	conn := c.PeerConn()
	if conn == nil {
		return nil
	}
	listener := conn.ListenService(ServicePeerRPC)
	defer func() {
		_ = listener.Close()
	}()
	client := c.rpcClient()
	for {
		stream, err := listener.Accept()
		if err != nil {
			if isPeerPacketReadClosed(err) {
				return nil
			}
			return err
		}

		go func(stream net.Conn) {
			if err := client.Handle(stream); err != nil {
				_ = stream.Close()
			}
		}(stream)
	}
}

func (c *Client) servePackets() error {
	if c == nil {
		return fmt.Errorf("gizclaw: nil client")
	}
	buf := make([]byte, 64*1024)
	for {
		conn := c.PeerConn()
		if conn == nil {
			return nil
		}
		protocol, n, err := conn.Read(buf)
		if err != nil {
			if isPeerPacketReadClosed(err) {
				return nil
			}
			return err
		}
		c.dispatchPeerPacket(protocol, buf[:n])
	}
}

func isPeerPacketReadClosed(err error) bool {
	return errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, giznet.ErrConnClosed) ||
		errors.Is(err, giznet.ErrClosed) ||
		errors.Is(err, giznet.ErrServiceMuxClosed)
}

func (c *Client) subscribePeerPackets(protocol byte, buffer int) (<-chan []byte, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan []byte, buffer)

	c.packetMu.Lock()
	if c.packetSubscribers == nil {
		c.packetSubscribers = make(map[byte]map[chan []byte]struct{})
	}
	if c.packetSubscribers[protocol] == nil {
		c.packetSubscribers[protocol] = make(map[chan []byte]struct{})
	}
	c.packetSubscribers[protocol][ch] = struct{}{}
	c.packetMu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			c.packetMu.Lock()
			if subscribers := c.packetSubscribers[protocol]; subscribers != nil {
				delete(subscribers, ch)
				if len(subscribers) == 0 {
					delete(c.packetSubscribers, protocol)
				}
			}
			c.packetMu.Unlock()
		})
	}
	return ch, unsubscribe
}

func (c *Client) dispatchPeerPacket(protocol byte, payload []byte) {
	c.packetMu.RLock()
	subscribers := c.packetSubscribers[protocol]
	if len(subscribers) == 0 {
		c.packetMu.RUnlock()
		return
	}
	targets := make([]chan []byte, 0, len(subscribers))
	for ch := range subscribers {
		targets = append(targets, ch)
	}
	c.packetMu.RUnlock()

	packet := append([]byte(nil), payload...)
	for _, ch := range targets {
		select {
		case ch <- packet:
		default:
			// Direct media packets are realtime; stale consumers should drop
			// rather than backpressure the peer packet loop.
		}
	}
}

func peerDeviceToPeerRefreshInfo(in apitypes.DeviceInfo) apitypes.RefreshInfo {
	out := apitypes.RefreshInfo{}
	if in.Name != nil {
		out.Name = in.Name
	}
	if in.Hardware != nil {
		out.Manufacturer = in.Hardware.Manufacturer
		out.Model = in.Hardware.Model
		out.HardwareRevision = in.Hardware.HardwareRevision
	}
	return out
}

func peerToPeerPeerIMEI(in apitypes.PeerIMEI) apitypes.PeerIMEI {
	out := apitypes.PeerIMEI{
		Tac:    in.Tac,
		Serial: in.Serial,
	}
	out.Name = in.Name
	return out
}

func peerToPeerPeerLabel(in apitypes.PeerLabel) apitypes.PeerLabel {
	return apitypes.PeerLabel{
		Key:   in.Key,
		Value: in.Value,
	}
}

func peerDeviceToPeerRefreshIdentifiers(in apitypes.DeviceInfo) apitypes.RefreshIdentifiers {
	out := apitypes.RefreshIdentifiers{}
	out.Sn = in.Sn
	if in.Hardware != nil {
		if in.Hardware.Imeis != nil {
			items := make([]apitypes.PeerIMEI, len(*in.Hardware.Imeis))
			for i := range *in.Hardware.Imeis {
				items[i] = peerToPeerPeerIMEI((*in.Hardware.Imeis)[i])
			}
			out.Imeis = &items
		}
		if in.Hardware.Labels != nil {
			items := make([]apitypes.PeerLabel, len(*in.Hardware.Labels))
			for i := range *in.Hardware.Labels {
				items[i] = peerToPeerPeerLabel((*in.Hardware.Labels)[i])
			}
			out.Labels = &items
		}
	}
	return out
}
