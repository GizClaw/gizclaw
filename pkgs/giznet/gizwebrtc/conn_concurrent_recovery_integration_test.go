package gizwebrtc

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v4"
)

const (
	recoveryIntegrationPeers   = 8
	recoveryIntegrationService = 100
	recoveryReadDeadline       = 2 * time.Second
	recoveryBurstWindow        = 10 * time.Millisecond
	recoveryLossWindow         = 1100 * time.Millisecond
)

// TestConcurrentRPCFrameRecoveryWithinDeadline keeps the application write
// path real while making a short shared-egress loss deterministic. The loss is
// armed only for the synchronized p8 write, so the p1 control proves that frame
// capacity and the RPC/DataChannel path remain healthy while SCTP recovers every
// concurrent response before the client deadline.
func TestConcurrentRPCFrameRecoveryWithinDeadline(t *testing.T) {
	lossConn, api, sctpLogs := newRecoveryIntegrationAPI(t)

	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	serverListener, err := (&ListenConfig{
		API:            api,
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	defer serverListener.Close()
	httpServer := httptest.NewServer(serverListener.SignalingHandler())
	defer httpServer.Close()

	type peer struct {
		clientListener *Listener
		clientConn     *Conn
		serverConn     *Conn
		clientStream   net.Conn
		serverStream   net.Conn
	}
	peers := make([]peer, recoveryIntegrationPeers)
	defer func() {
		for i := range peers {
			if peers[i].clientStream != nil {
				_ = peers[i].clientStream.Close()
			}
			if peers[i].serverStream != nil {
				_ = peers[i].serverStream.Close()
			}
			if peers[i].clientConn != nil {
				_ = peers[i].clientConn.Close()
			}
			if peers[i].serverConn != nil {
				_ = peers[i].serverConn.Close()
			}
			if peers[i].clientListener != nil {
				_ = peers[i].clientListener.Close()
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for i := range peers {
		clientKey, keyErr := giznet.GenerateKeyPair()
		if keyErr != nil {
			t.Fatalf("GenerateKeyPair(client %d) error = %v", i, keyErr)
		}
		peers[i].clientListener, peers[i].clientConn, err = Dial(ctx, clientKey, serverKey.Public, DialConfig{
			SignalingURL:   httpServer.URL + SignalingPath,
			CipherMode:     CipherModePlaintext,
			SecurityPolicy: allowAllPolicy{},
		})
		if err != nil {
			t.Fatalf("Dial(client %d) error = %v", i, err)
		}
		peers[i].serverConn = acceptConn(t, serverListener)
		service := peers[i].serverConn.ListenService(recoveryIntegrationService)
		peers[i].clientStream, err = peers[i].clientConn.Dial(recoveryIntegrationService)
		if err != nil {
			t.Fatalf("Dial(service, client %d) error = %v", i, err)
		}
		peers[i].serverStream, err = service.Accept()
		if err != nil {
			t.Fatalf("Accept(service, client %d) error = %v", i, err)
		}
		if err := service.Close(); err != nil {
			t.Fatalf("Close(service, client %d) error = %v", i, err)
		}
	}

	for i := range peers {
		assertRecoveryFrameRoundTrip(t, peers[i].serverStream, peers[i].clientStream, fmt.Sprintf("warm-%d", i))
	}

	for i := range peers {
		started := time.Now()
		assertRecoveryFrameRoundTrip(t, peers[i].serverStream, peers[i].clientStream, fmt.Sprintf("p1-%d", i))
		if elapsed := time.Since(started); elapsed >= recoveryReadDeadline {
			t.Fatalf("p1 peer %d took %v, want less than %v", i, elapsed, recoveryReadDeadline)
		}
	}

	lossConn.arm()
	start := make(chan struct{})
	type result struct {
		peer    int
		write   time.Duration
		read    time.Duration
		readErr error
	}
	results := make(chan result, recoveryIntegrationPeers)
	var writers sync.WaitGroup
	writers.Add(recoveryIntegrationPeers)
	for i := range peers {
		go func() {
			defer writers.Done()
			<-start
			writeStarted := time.Now()
			writeErr := rpcapi.WriteFrame(peers[i].serverStream, rpcapi.Frame{
				Type:    rpcapi.FrameTypeBinary,
				Payload: fmt.Appendf(nil, "p8-%d", i),
			})
			writeElapsed := time.Since(writeStarted)
			if writeErr != nil {
				results <- result{peer: i, write: writeElapsed, readErr: fmt.Errorf("write: %w", writeErr)}
				return
			}
			readStarted := time.Now()
			if deadlineErr := peers[i].clientStream.SetReadDeadline(readStarted.Add(recoveryReadDeadline)); deadlineErr != nil {
				results <- result{peer: i, write: writeElapsed, readErr: fmt.Errorf("set read deadline: %w", deadlineErr)}
				return
			}
			frame, readErr := rpcapi.ReadFrame(peers[i].clientStream)
			readElapsed := time.Since(readStarted)
			if readErr == nil && (frame.Type != rpcapi.FrameTypeBinary || string(frame.Payload) != fmt.Sprintf("p8-%d", i)) {
				readErr = fmt.Errorf("unexpected frame type=%v payload=%q", frame.Type, frame.Payload)
			}
			results <- result{peer: i, write: writeElapsed, read: readElapsed, readErr: readErr}
		}()
	}
	close(start)
	writers.Wait()
	close(results)

	maxWrite := time.Duration(0)
	maxRead := time.Duration(0)
	successes := 0
	for got := range results {
		if got.write > maxWrite {
			maxWrite = got.write
		}
		if got.read > maxRead {
			maxRead = got.read
		}
		if got.readErr != nil {
			t.Fatalf("p8 peer %d response error after %v = %v", got.peer, got.read, got.readErr)
		}
		successes++
		t.Logf("p8 peer=%d write=%v read=%v err=%v", got.peer, got.write, got.read, got.readErr)
	}

	dropped := lossConn.droppedPackets()
	logs := sctpLogs.String()
	t.Logf("p1=8/8 p8=%d/8 max_write=%v max_read=%v dropped_dtls_application=%d", successes, maxWrite, maxRead, dropped)
	if dropped == 0 {
		t.Fatal("p8 did not trigger the shared-egress loss fixture")
	}
	if maxWrite >= 100*time.Millisecond {
		t.Fatalf("p8 application writes took up to %v; expected accepted writes before transport recovery", maxWrite)
	}
	if successes != recoveryIntegrationPeers {
		t.Fatalf("p8 delivered %d/%d responses", successes, recoveryIntegrationPeers)
	}
	if maxRead >= recoveryReadDeadline {
		t.Fatalf("p8 recovery took %v, want less than %v", maxRead, recoveryReadDeadline)
	}
	if !strings.Contains(logs, "PTO fired") {
		t.Fatalf("p8 passed without exercising SCTP PTO recovery; SCTP logs:\n%s", logs)
	}
	if strings.Contains(logs, "T3-rtx timed out: nRtos=2") {
		t.Fatalf("p8 waited for the second T3 timeout despite PTO recovery; SCTP logs:\n%s", logs)
	}
	t.Logf("SCTP recovery evidence:\n%s", recoveryLogLines(logs))
}

func assertRecoveryFrameRoundTrip(t *testing.T, server, client net.Conn, payload string) {
	t.Helper()
	if err := rpcapi.WriteFrame(server, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte(payload)}); err != nil {
		t.Fatalf("WriteFrame(%q) error = %v", payload, err)
	}
	if err := client.SetReadDeadline(time.Now().Add(recoveryReadDeadline)); err != nil {
		t.Fatalf("SetReadDeadline(%q) error = %v", payload, err)
	}
	frame, err := rpcapi.ReadFrame(client)
	if err != nil {
		t.Fatalf("ReadFrame(%q) error = %v", payload, err)
	}
	if frame.Type != rpcapi.FrameTypeBinary || string(frame.Payload) != payload {
		t.Fatalf("ReadFrame(%q) = type %v payload %q", payload, frame.Type, frame.Payload)
	}
}

func newRecoveryIntegrationAPI(t *testing.T) (*burstLossPacketConn, *webrtc.API, *lockedLogBuffer) {
	t.Helper()
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("ListenUDP error = %v", err)
	}
	lossConn := &burstLossPacketConn{UDPConn: udp}
	logs := new(lockedLogBuffer)
	loggerFactory := logging.NewDefaultLoggerFactory()
	loggerFactory.Writer = logs
	loggerFactory.ScopeLevels["sctp"] = logging.LogLevelTrace

	var mediaEngine webrtc.MediaEngine
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    MediaStreamOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		_ = udp.Close()
		t.Fatalf("RegisterCodec error = %v", err)
	}

	settings := webrtc.SettingEngine{LoggerFactory: loggerFactory}
	settings.DetachDataChannels()
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	udpMux := webrtc.NewICEUDPMux(loggerFactory.NewLogger("gizwebrtc"), lossConn)
	settings.SetICEUDPMux(udpMux)
	t.Cleanup(func() { _ = udpMux.Close() })

	return lossConn, webrtc.NewAPI(
		webrtc.WithMediaEngine(&mediaEngine),
		webrtc.WithSettingEngine(settings),
	), logs
}

type burstLossPacketConn struct {
	*net.UDPConn

	mu          sync.Mutex
	armed       bool
	triggered   bool
	burstStart  time.Time
	burstWrites int
	lossUntil   time.Time
	dropped     int
}

func (c *burstLossPacketConn) arm() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.armed = true
	c.triggered = false
	c.burstStart = time.Time{}
	c.burstWrites = 0
	c.lossUntil = time.Time{}
	c.dropped = 0
}

func (c *burstLossPacketConn) WriteTo(payload []byte, addr net.Addr) (int, error) {
	if len(payload) == 0 || payload[0] != 0x17 {
		return c.UDPConn.WriteTo(payload, addr)
	}
	now := time.Now()
	c.mu.Lock()
	if c.armed && !c.triggered {
		if c.burstStart.IsZero() || now.Sub(c.burstStart) > recoveryBurstWindow {
			c.burstStart = now
			c.burstWrites = 1
		} else {
			c.burstWrites++
			if c.burstWrites == 2 {
				c.triggered = true
				c.lossUntil = now.Add(recoveryLossWindow)
			}
		}
	}
	drop := c.triggered && now.Before(c.lossUntil)
	if drop {
		c.dropped++
	}
	c.mu.Unlock()
	if drop {
		return len(payload), nil
	}
	return c.UDPConn.WriteTo(payload, addr)
}

func (c *burstLossPacketConn) droppedPackets() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

type lockedLogBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedLogBuffer) Write(payload []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(payload)
}

func (b *lockedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func recoveryLogLines(logs string) string {
	var selected []string
	for line := range strings.SplitSeq(logs, "\n") {
		if strings.Contains(line, "T3-rtx timed out") || strings.Contains(line, "PTO fired") {
			selected = append(selected, line)
		}
	}
	return strings.Join(selected, "\n")
}
