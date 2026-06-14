package gizcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/serverpublic"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizhttp"
	"golang.org/x/sync/errgroup"
)

// Client holds device-side peer client configuration.
type Client struct {
	KeyPair *giznet.KeyPair
	// CipherMode must match the server's low-level giznet cipher mode.
	CipherMode giznet.CipherMode

	Device apitypes.DeviceInfo

	mu       sync.RWMutex
	listener *giznet.Listener
	conn     *giznet.Conn
	serverPK giznet.PublicKey
	rpc      *rpcClient

	packetMu          sync.RWMutex
	packetSubscribers map[byte]map[chan []byte]struct{}
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

	l, err := (&giznet.ListenConfig{
		Addr:           ":0",
		CipherMode:     c.CipherMode,
		SecurityPolicy: clientSecurityPolicy{},
	}).Listen(c.KeyPair)
	if err != nil {
		return fmt.Errorf("gizclaw: listen: %w", err)
	}

	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		_ = l.Close()
		return fmt.Errorf("gizclaw: resolve addr: %w", err)
	}

	conn, err := l.Dial(serverPK, udpAddr)
	if err != nil {
		_ = l.Close()
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
	g.Go(c.serveRPC)
	g.Go(c.servePackets)
	if err := g.Wait(); err != nil {
		_ = c.Close()
		return err
	}
	return nil
}

func (c *Client) init(listener *giznet.Listener, conn *giznet.Conn, serverPK giznet.PublicKey) {
	c.listener = listener
	c.conn = conn
	c.serverPK = serverPK
	c.rpc = &rpcClient{peer: c}
}

// Close releases all resources including the underlying UDP socket.
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
	return gizhttp.NewClient(c.PeerConn(), service)
}

func (c *Client) ServerAdminClient() (*adminservice.ClientWithResponses, error) {
	return adminservice.NewClientWithResponses(
		"http://gizclaw",
		adminservice.WithHTTPClient(c.HTTPClient(ServiceAdmin)),
	)
}

func (c *Client) ServerPublicClient() (*serverpublic.ClientWithResponses, error) {
	return serverpublic.NewClientWithResponses(
		"http://gizclaw",
		serverpublic.WithHTTPClient(c.HTTPClient(ServiceServerPublic)),
	)
}

// Ping opens a fresh RPC stream, sends one ping, and closes it.
//
// Our current RPC transport uses one KCP stream per round trip so multiple RPC
// requests can run concurrently on separate streams. This is closer to
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

func (c *Client) DownloadFirmware(ctx context.Context, id string, request rpcapi.FirmwareDownloadRequest, out io.Writer) (FirmwareDownloadResult, error) {
	stream, err := c.rpcConn()
	if err != nil {
		return FirmwareDownloadResult{}, err
	}
	defer func() { _ = stream.Close() }()
	return c.rpcClient().DownloadFirmware(ctx, stream, id, request, out)
}

func callClientRPC[T any](c *Client, call func(*rpcClient, net.Conn) (*T, error)) (*T, error) {
	stream, err := c.rpcConn()
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	return call(c.rpcClient(), stream)
}

func (c *Client) rpcConn() (net.Conn, error) {
	conn := c.PeerConn()
	if conn == nil {
		return nil, fmt.Errorf("gizclaw: client is not connected")
	}
	stream, err := conn.Dial(ServiceRPC)
	if err != nil {
		return nil, fmt.Errorf("gizclaw: dial rpc stream: %w", err)
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
func (c *Client) PeerConn() *giznet.Conn {
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
	listener := c.conn.ListenService(ServiceRPC)
	defer func() {
		_ = listener.Close()
	}()
	client := c.rpcClient()
	for {
		stream, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
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
		errors.Is(err, giznet.ErrUDPClosed) ||
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

// ProxyHandler exposes the local reverse-proxy routes for remote server APIs.
func (c *Client) ProxyHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/admin/", http.StripPrefix("/api/admin", c.proxyService(ServiceAdmin)))
	mux.Handle("/api/public/", http.StripPrefix("/api/public", c.proxyService(ServiceServerPublic)))
	mux.HandleFunc("/api/admin", redirectProxyPrefix("/api/admin/"))
	mux.HandleFunc("/api/public", redirectProxyPrefix("/api/public/"))
	mux.HandleFunc("/api", redirectProxyPrefix("/api/"))
	return mux
}

func (c *Client) proxyService(service uint64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c == nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		conn := c.PeerConn()
		if conn == nil {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		newServiceProxy(conn, service).ServeHTTP(w, r)
	})
}

func newServiceProxy(conn *giznet.Conn, service uint64) *httputil.ReverseProxy {
	target := &url.URL{
		Scheme: "http",
		Host:   "gizclaw",
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = gizhttp.NewRoundTripper(conn, service)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
	}
	return proxy
}

func redirectProxyPrefix(target string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
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
