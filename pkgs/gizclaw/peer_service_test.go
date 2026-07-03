package gizclaw

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

const (
	testReadyTimeout = 10 * time.Second
	testPollInterval = 20 * time.Millisecond
)

func waitUntil(timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(testPollInterval)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("condition not satisfied before timeout")
}

func TestPeerServiceServeConnRequiresHandlers(t *testing.T) {
	service := &PeerService{}

	err := service.ServeConn(&testGiznetConn{})
	if err == nil {
		t.Fatal("ServeConn should fail when handlers are missing")
	}
	if err.Error() != "gizclaw: nil admin service" {
		t.Fatalf("ServeConn error = %v", err)
	}
}

func TestPeerServiceValidateServices(t *testing.T) {
	tests := []struct {
		name    string
		service *PeerService
		wantErr string
	}{
		{
			name:    "missing admin service",
			service: &PeerService{},
			wantErr: "nil admin service",
		},
		{
			name: "missing public service",
			service: &PeerService{
				admin:   &adminService{},
				manager: &Manager{},
			},
			wantErr: "nil public service",
		},
		{
			name: "missing manager",
			service: &PeerService{
				admin: &adminService{},
			},
			wantErr: "nil manager",
		},
		{
			name: "complete service bundle",
			service: &PeerService{
				admin:   &adminService{},
				manager: &Manager{},
				public:  &serverPublic{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.service.validateServices()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateServices() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateServices() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestIntegrationPeerServiceServeConnClientCloseUnblocksAndMarksPeerOffline(t *testing.T) {
	const closeTimeout = 2 * time.Second

	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}

	clientConn, serverConn := newTestWebRTCConnPair(t, serverKey, clientKey,
		testGiznetSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				switch service {
				case ServiceAdmin, ServiceServerPublic, ServiceRPC:
					return true
				default:
					return false
				}
			},
		},
		testGiznetSecurityPolicy{})
	defer clientConn.Close()
	defer serverConn.Close()

	server := &Server{
		LocalStatic: *serverKey,
		PeerStore:   mustBadgerInMemory(t, nil),
		BuildCommit: "test-build",
	}
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- server.peerService.ServeConn(serverConn)
	}()

	client := &http.Client{
		Transport: gizhttp.NewRoundTripper(clientConn, ServiceServerPublic),
		Timeout:   time.Second,
	}
	if err := waitUntil(testReadyTimeout, func() error {
		if _, ok := server.manager.Peer(clientKey.Public); !ok {
			return fmt.Errorf("peer not marked online yet")
		}
		peer, loadErr := server.manager.Peers.LoadPeer(context.Background(), clientKey.Public)
		if loadErr != nil {
			return fmt.Errorf("auto-created peer not ready: %w", loadErr)
		}
		if peer.Role != apitypes.PeerRoleClient || peer.Status != apitypes.PeerRegistrationStatusActive {
			return fmt.Errorf("auto-created peer = %+v", peer)
		}

		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://gizclaw/server-info", nil)
		if reqErr != nil {
			return reqErr
		}
		resp, doErr := client.Do(req)
		if doErr != nil {
			select {
			case serveErr := <-serveErrCh:
				return fmt.Errorf("ServeConn exited before ready: %w", serveErr)
			default:
			}
			return doErr
		}
		defer resp.Body.Close()

		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server-info status = %d body=%s", resp.StatusCode, string(body))
		}
		return nil
	}); err != nil {
		t.Fatalf("ServeConn did not become ready: %v", err)
	}

	start := time.Now()
	if err := clientConn.Close(); err != nil {
		t.Fatalf("clientConn.Close error = %v", err)
	}

	select {
	case serveErr := <-serveErrCh:
		if serveErr != nil {
			t.Fatalf("ServeConn error after client close = %v", serveErr)
		}
	case <-time.After(closeTimeout):
		t.Fatalf("ServeConn did not exit within %v after client close", closeTimeout)
	}

	if took := time.Since(start); took > closeTimeout {
		t.Fatalf("ServeConn close path took %v, want <= %v", took, closeTimeout)
	}

	if _, ok := server.manager.Peer(clientKey.Public); ok {
		t.Fatal("peer should be removed after client close")
	}
	if runtime := server.manager.PeerRuntime(context.Background(), clientKey.Public); runtime.Online || !runtime.LastSeenAt.IsZero() {
		t.Fatalf("peer runtime after client close = %+v", runtime)
	}
}

func TestIntegrationPeerServiceServeConnReplacesSameKeyConnection(t *testing.T) {
	const closeTimeout = 2 * time.Second

	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}

	server := &Server{
		LocalStatic: *serverKey,
		PeerStore:   mustBadgerInMemory(t, nil),
		BuildCommit: "test-build",
	}
	if err := server.init(); err != nil {
		t.Fatalf("init error = %v", err)
	}

	oldClientConn, oldServerConn := newTestWebRTCConnPair(t, serverKey, clientKey,
		testGiznetSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				switch service {
				case ServiceAdmin, ServiceServerPublic, ServiceRPC:
					return true
				default:
					return false
				}
			},
		},
		testGiznetSecurityPolicy{})
	defer oldClientConn.Close()

	oldServeErrCh := make(chan error, 1)
	go func() {
		oldServeErrCh <- server.peerService.ServeConn(oldServerConn)
	}()
	if err := waitUntil(testReadyTimeout, func() error {
		got, ok := server.manager.Peer(clientKey.Public)
		if !ok || got != oldServerConn {
			return fmt.Errorf("old conn not active yet: ok=%v", ok)
		}
		return nil
	}); err != nil {
		t.Fatalf("old ServeConn did not become active: %v", err)
	}

	newClientConn, newServerConn := newTestWebRTCConnPair(t, serverKey, clientKey,
		testGiznetSecurityPolicy{
			allowService: func(_ giznet.PublicKey, service uint64) bool {
				switch service {
				case ServiceAdmin, ServiceServerPublic, ServiceRPC:
					return true
				default:
					return false
				}
			},
		},
		testGiznetSecurityPolicy{})
	defer newClientConn.Close()

	newServeErrCh := make(chan error, 1)
	go func() {
		newServeErrCh <- server.peerService.ServeConn(newServerConn)
	}()
	if err := waitUntil(testReadyTimeout, func() error {
		got, ok := server.manager.Peer(clientKey.Public)
		if !ok || got != newServerConn {
			return fmt.Errorf("new conn not active yet: ok=%v", ok)
		}
		return nil
	}); err != nil {
		t.Fatalf("new ServeConn did not replace old active conn: %v", err)
	}

	select {
	case serveErr := <-oldServeErrCh:
		if serveErr != nil {
			t.Fatalf("old ServeConn error after replacement = %v", serveErr)
		}
	case <-time.After(closeTimeout):
		t.Fatalf("old ServeConn did not exit within %v after replacement", closeTimeout)
	}

	if got, ok := server.manager.Peer(clientKey.Public); !ok || got != newServerConn {
		t.Fatalf("old ServeConn teardown cleared new active conn: ok=%v", ok)
	}

	if err := newClientConn.Close(); err != nil {
		t.Fatalf("newClientConn.Close error = %v", err)
	}
	select {
	case serveErr := <-newServeErrCh:
		if serveErr != nil {
			t.Fatalf("new ServeConn error after client close = %v", serveErr)
		}
	case <-time.After(closeTimeout):
		t.Fatalf("new ServeConn did not exit within %v after client close", closeTimeout)
	}
	if _, ok := server.manager.Peer(clientKey.Public); ok {
		t.Fatal("peer should be offline after new active conn exits")
	}
}
