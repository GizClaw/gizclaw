package gizhttp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/pion/turn/v4"
	"github.com/pion/webrtc/v4"
)

// Exercise the real HTTP proxy and Pion DataChannels, retaining the parent
// association during cleanup assertions so parent Close cannot hide leaks.
func TestReverseProxyConcurrentStreamLifecycle(t *testing.T) {
	for _, mode := range []string{"direct", "relay"} {
		t.Run(mode, func(t *testing.T) { testReverseProxyConcurrentStreamLifecycle(t, mode == "relay") })
	}
}

func testReverseProxyConcurrentStreamLifecycle(t *testing.T, relay bool) {
	const concurrency = 16
	const rounds = 256 // 4096 requests on the same association, beyond admission capacity.
	var liveHTTP atomic.Int64
	var completed atomic.Int64
	baseline := gizwebrtc.ReadMonitorSnapshot()
	clientConn, serverConn := lifecycleConnections(t, relay)
	srv := NewServer(serverConn, 48, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
		completed.Add(1)
	}))
	srv.httpServer.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			liveHTTP.Add(1)
		case http.StateClosed, http.StateHijacked:
			liveHTTP.Add(-1)
		}
	}
	serveLifecycleServer(t, srv)
	proxy := httptest.NewServer(&httputil.ReverseProxy{
		Director:  func(r *http.Request) { r.URL.Scheme = "http"; r.URL.Host = "upstream" },
		Transport: NewRoundTripper(clientConn, 48),
	})
	t.Cleanup(proxy.Close)
	client := proxy.Client()
	client.Transport = &http.Transport{MaxIdleConnsPerHost: concurrency}
	t.Cleanup(client.CloseIdleConnections)
	client.Timeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	for round := range rounds {
		start := make(chan struct{})
		errs := make(chan error, concurrency)
		var wg sync.WaitGroup
		for range concurrency {
			wg.Go(func() {
				<-start
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy.URL+"/status", nil)
				if err != nil {
					errs <- err
					return
				}
				resp, err := client.Do(req)
				if err != nil {
					errs <- err
					return
				}
				data, readErr := io.ReadAll(resp.Body)
				closeErr := resp.Body.Close()
				if readErr != nil || closeErr != nil {
					errs <- errors.Join(readErr, closeErr)
					return
				}
				if resp.StatusCode != 200 || string(data) != `{"status":"ok"}` {
					errs <- fmt.Errorf("status=%d body=%q", resp.StatusCode, data)
				}
			})
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: %v (snapshot=%+v live_http=%d)", round, err, gizwebrtc.ReadMonitorSnapshot(), liveHTTP.Load())
		}
	}
	waitLifecycleRelease(t, baseline, &liveHTTP)
	if got := completed.Load(); got != concurrency*rounds {
		t.Fatalf("completed=%d", got)
	}
	t.Logf("completed %d requests, concurrency %d; service channels and HTTP connections returned to baseline", completed.Load(), concurrency)
}

func TestHTTPStreamTimeoutAndCancellationRelease(t *testing.T) {
	for _, mode := range []string{"direct", "relay"} {
		t.Run(mode, func(t *testing.T) { testHTTPStreamTimeoutAndCancellationRelease(t, mode == "relay") })
	}
}

func testHTTPStreamTimeoutAndCancellationRelease(t *testing.T, relay bool) {
	for _, phase := range []string{"headers", "body", "cancel", "disconnect_headers", "disconnect_body"} {
		t.Run(phase, func(t *testing.T) {
			baseline := gizwebrtc.ReadMonitorSnapshot()
			clientConn, serverConn := lifecycleConnections(t, relay)
			entered := make(chan struct{})
			exited := make(chan struct{})
			var liveHTTP atomic.Int64
			srv := NewServer(serverConn, 48, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/healthy" {
					_, _ = io.WriteString(w, "ok")
					return
				}
				defer close(exited)
				if phase == "body" || phase == "disconnect_body" {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Content-Length", "100")
					_, _ = io.WriteString(w, "partial")
					w.(http.Flusher).Flush()
				}
				close(entered)
				<-r.Context().Done()
			}))
			srv.httpServer.ConnState = func(_ net.Conn, state http.ConnState) {
				switch state {
				case http.StateNew:
					liveHTTP.Add(1)
				case http.StateClosed:
					liveHTTP.Add(-1)
				}
			}
			serveLifecycleServer(t, srv)
			transport := NewRoundTripper(clientConn, 48)
			proxy := httptest.NewServer(&httputil.ReverseProxy{
				Director:  func(r *http.Request) { r.URL.Scheme = "http"; r.URL.Host = "upstream" },
				Transport: transport,
			})
			t.Cleanup(proxy.Close)
			ctx, cancel := context.WithCancel(t.Context())
			if phase == "headers" || phase == "body" {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 500*time.Millisecond)
			}
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxy.URL+"/stall", nil)
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			var downstream net.Conn
			if strings.HasPrefix(phase, "disconnect_") {
				downstream, err = net.DialTimeout("tcp", proxy.Listener.Addr().String(), 5*time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer downstream.Close()
				if err := downstream.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
					t.Fatal(err)
				}
				if err := req.Write(downstream); err != nil {
					t.Fatal(err)
				}
			} else {
				go func() {
					resp, err := proxy.Client().Do(req)
					if err == nil {
						_, err = io.ReadAll(resp.Body)
						err = errors.Join(err, resp.Body.Close())
					}
					result <- err
				}()
			}
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("handler never entered")
			}
			switch phase {
			case "disconnect_headers", "disconnect_body":
				if phase == "disconnect_body" {
					_ = downstream.SetReadDeadline(time.Now().Add(5 * time.Second))
					resp, err := http.ReadResponse(bufio.NewReader(downstream), req)
					if err != nil {
						t.Fatal(err)
					}
					prefix := make([]byte, len("partial"))
					if _, err := io.ReadFull(resp.Body, prefix); err != nil {
						t.Fatal(err)
					}
				}
				if err := downstream.Close(); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				cancel()
			}
			if !strings.HasPrefix(phase, "disconnect_") {
				select {
				case err := <-result:
					if err == nil {
						t.Fatal("stalled request unexpectedly succeeded")
					}
					if (phase == "headers" || phase == "body") && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
						t.Fatalf("expected deadline expiry, got %v", ctx.Err())
					}
				case <-time.After(5 * time.Second):
					t.Fatal("request did not unblock")
				}
			}

			select {
			case <-exited:
			case <-time.After(5 * time.Second):
				t.Fatal("server handler did not observe cancellation")
			}
			waitLifecycleRelease(t, baseline, &liveHTTP)
			healthyCtx, healthyCancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer healthyCancel()
			healthy, _ := http.NewRequestWithContext(healthyCtx, http.MethodGet, "http://upstream/healthy", nil)
			resp, err := transport.RoundTrip(healthy)
			if err != nil {
				t.Fatalf("parent association unusable: %v", err)
			}
			body, err := io.ReadAll(resp.Body)
			closeErr := resp.Body.Close()
			if err != nil || closeErr != nil || strings.TrimSpace(string(body)) != "ok" {
				t.Fatalf("follow-up: body=%q err=%v close=%v", body, err, closeErr)
			}
			waitLifecycleRelease(t, baseline, &liveHTTP)
		})
	}
}

func lifecycleConnections(t *testing.T, relay bool) (giznet.Conn, giznet.Conn) {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	policy := testSecurityPolicy{allowService: func(_ giznet.PublicKey, service uint64) bool { return service == 48 }}
	server, err := (&gizwebrtc.ListenConfig{SecurityPolicy: policy, ICEUDPAddr: "0.0.0.0:0", SCTPReceiveBufferSize: gizwebrtc.GatewaySCTPReceiveBufferSize}).Listen(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	signaling := httptest.NewServer(server.SignalingHandler())
	t.Cleanup(signaling.Close)
	config := gizwebrtc.DialConfig{SecurityPolicy: policy, SignalingURL: signaling.URL + gizwebrtc.SignalingPath, SCTPReceiveBufferSize: gizwebrtc.GatewaySCTPReceiveBufferSize}
	if relay {
		socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = socket.Close() })
		// Advertise a routable local address: production ICE excludes loopback
		// candidates, so a 127.0.0.1 relay cannot be reached by this peer.
		route, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 9})
		if err != nil {
			t.Fatal(err)
		}
		relayIP := route.LocalAddr().(*net.UDPAddr).IP
		_ = route.Close()
		relayServer, err := turn.NewServer(turn.ServerConfig{
			Realm: "http-lifecycle-test",
			AuthHandler: func(username, realm string, _ net.Addr) ([]byte, bool) {
				return turn.GenerateAuthKey(username, realm, "test-password"), username == "test-client" && realm == "http-lifecycle-test"
			},
			PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: socket, RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{RelayAddress: relayIP, Address: "0.0.0.0"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = relayServer.Close() })
		config.ICETransportPolicy = webrtc.ICETransportPolicyRelay
		config.ICEServers = []gizwebrtc.ICEServer{{URLs: []string{"turn:" + socket.LocalAddr().String() + "?transport=udp"}, Username: "test-client", Credential: "test-password"}}
	}
	var observation *gizwebrtc.ICECandidatePairObservation
	config.OnTiming = func(timing gizwebrtc.DialTiming) { observation = timing.SelectedCandidatePair }
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	listener, client, err := gizwebrtc.Dial(ctx, clientKey, serverKey.Public, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	t.Cleanup(func() { _ = client.Close() })
	if relay && (observation == nil || observation.Local.Type != "relay") {
		t.Fatalf("TURN was not selected: %+v", observation)
	}
	peer, err := server.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	return client, peer
}

func serveLifecycleServer(t *testing.T, srv *Server) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()
	t.Cleanup(func() {
		_ = srv.httpServer.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("HTTP serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("HTTP server did not exit")
		}
	})
}

func waitLifecycleRelease(t *testing.T, baseline gizwebrtc.MonitorSnapshot, liveHTTP *atomic.Int64) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := gizwebrtc.ReadMonitorSnapshot()
		if snapshot.Services == baseline.Services && snapshot.InboundServiceChannels == baseline.InboundServiceChannels && liveHTTP.Load() == 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("streams retained while parent remains open: before=%+v after=%+v live_http=%d", baseline, snapshot, liveHTTP.Load())
		}
	}
}
