package gizwebrtc

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/pion/logging"
	"github.com/pion/webrtc/v4"
)

const (
	// stalledOpenMessageSize spans several SCTP DATA fragments, so the filter
	// can deliver the first fragment and drop the rest.
	stalledOpenMessageSize = 4000
	// stalledOpenFragmentDatagram separates fragment-carrying DTLS records
	// (about 1.25 KB) from DCEP, SACK, FORWARD TSN, and heartbeat records.
	stalledOpenFragmentDatagram = 1000
	stalledOpenStreamID         = 100
	// stalledOpenAcceptDeadline stays below the 10 s default DCEP OPEN read
	// deadline, so the later channel must be accepted while the stalled stream
	// is still waiting rather than after its OPEN read times out.
	stalledOpenAcceptDeadline = 8 * time.Second
)

// TestServerAcceptsDataChannelBehindStalledOpen reproduces the Server-side
// wedge from the 2026-09-10 Edge incident. The client starts a message on a
// stream the Server has no channel for, and the network delivers only its first
// fragment: SCTP admits the stream, but the Server never receives a complete
// DCEP OPEN on it. A channel opened afterwards must still reach the Server's
// OnDataChannel and carry data.
func TestServerAcceptsDataChannelBehindStalledOpen(t *testing.T) {
	serverAPI, closers, err := newPionAPI(&ListenConfig{ICEAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("newPionAPI error = %v", err)
	}
	t.Cleanup(func() {
		for _, closeFn := range closers {
			_ = closeFn()
		}
	})
	filter, clientAPI := newStalledOpenClientAPI(t)

	server, err := serverAPI.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("server NewPeerConnection error = %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	client, err := clientAPI.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	accepted := make(chan string, 4)
	received := make(chan []byte, 4)
	server.OnDataChannel(func(dc *webrtc.DataChannel) {
		accepted <- dc.Label()
		dc.OnOpen(func() {
			raw, err := dc.Detach()
			if err != nil {
				return
			}
			buf := make([]byte, 64)
			n, err := raw.Read(buf)
			if err != nil {
				return
			}
			received <- append([]byte(nil), buf[:n]...)
		})
	})

	warmup, err := client.CreateDataChannel("warmup", nil)
	if err != nil {
		t.Fatalf("CreateDataChannel(warmup) error = %v", err)
	}
	connectStalledOpenPeers(t, client, server)
	sendWhenOpen(t, warmup, []byte("warmup"))
	expectAcceptedChannel(t, accepted, received, "warmup", []byte("warmup"), stalledOpenAcceptDeadline)

	filter.arm()
	ordered := false
	maxRetransmits := uint16(0)
	negotiated := true
	streamID := uint16(stalledOpenStreamID)
	stalled, err := client.CreateDataChannel("stalled", &webrtc.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
		Negotiated:     &negotiated,
		ID:             &streamID,
	})
	if err != nil {
		t.Fatalf("CreateDataChannel(stalled) error = %v", err)
	}
	sendWhenOpen(t, stalled, bytes.Repeat([]byte{0x5a}, stalledOpenMessageSize))
	waitForCondition(t, stalledOpenAcceptDeadline, func() bool {
		passed, dropped := filter.counts()
		return passed == 1 && dropped > 0
	}, "the first stalled fragment to pass and a later one to drop")

	after, err := client.CreateDataChannel("after-stalled", nil)
	if err != nil {
		t.Fatalf("CreateDataChannel(after-stalled) error = %v", err)
	}
	sendWhenOpen(t, after, []byte("after"))
	expectAcceptedChannel(t, accepted, received, "after-stalled", []byte("after"), stalledOpenAcceptDeadline)
}

func newStalledOpenClientAPI(t *testing.T) (*fragmentStallPacketConn, *webrtc.API) {
	t.Helper()
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("ListenUDP error = %v", err)
	}
	filter := &fragmentStallPacketConn{UDPConn: udp}
	loggerFactory := logging.NewDefaultLoggerFactory()

	settings := webrtc.SettingEngine{LoggerFactory: loggerFactory}
	settings.SetIncludeLoopbackCandidate(true)
	settings.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	udpMux := webrtc.NewICEUDPMux(loggerFactory.NewLogger("gizwebrtc"), filter)
	settings.SetICEUDPMux(udpMux)
	t.Cleanup(func() { _ = udpMux.Close() })

	return filter, webrtc.NewAPI(webrtc.WithSettingEngine(settings))
}

func connectStalledOpenPeers(t *testing.T, client, server *webrtc.PeerConnection) {
	t.Helper()
	offer, err := client.CreateOffer(nil)
	if err != nil {
		t.Fatalf("CreateOffer error = %v", err)
	}
	clientGathered := webrtc.GatheringCompletePromise(client)
	if err := client.SetLocalDescription(offer); err != nil {
		t.Fatalf("client SetLocalDescription error = %v", err)
	}
	<-clientGathered
	if err := server.SetRemoteDescription(*client.LocalDescription()); err != nil {
		t.Fatalf("server SetRemoteDescription error = %v", err)
	}
	answer, err := server.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("CreateAnswer error = %v", err)
	}
	serverGathered := webrtc.GatheringCompletePromise(server)
	if err := server.SetLocalDescription(answer); err != nil {
		t.Fatalf("server SetLocalDescription error = %v", err)
	}
	<-serverGathered
	if err := client.SetRemoteDescription(*server.LocalDescription()); err != nil {
		t.Fatalf("client SetRemoteDescription error = %v", err)
	}
}

func sendWhenOpen(t *testing.T, dc *webrtc.DataChannel, payload []byte) {
	t.Helper()
	opened := make(chan struct{})
	var once sync.Once
	dc.OnOpen(func() { once.Do(func() { close(opened) }) })
	if dc.ReadyState() == webrtc.DataChannelStateOpen {
		once.Do(func() { close(opened) })
	}
	select {
	case <-opened:
	case <-time.After(stalledOpenAcceptDeadline):
		t.Fatalf("DataChannel %q did not open on the client", dc.Label())
	}
	if err := dc.Send(payload); err != nil {
		t.Fatalf("DataChannel %q Send error = %v", dc.Label(), err)
	}
}

func expectAcceptedChannel(
	t *testing.T,
	accepted <-chan string,
	received <-chan []byte,
	label string,
	payload []byte,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.After(timeout)
	select {
	case got := <-accepted:
		if got != label {
			t.Fatalf("server accepted DataChannel %q, want %q", got, label)
		}
	case <-deadline:
		t.Fatalf("server did not accept DataChannel %q within %s", label, timeout)
	}
	select {
	case got := <-received:
		if !bytes.Equal(got, payload) {
			t.Fatalf("server read %q on DataChannel %q, want %q", got, label, payload)
		}
	case <-deadline:
		t.Fatalf("server did not read from DataChannel %q within %s", label, timeout)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fragmentStallPacketConn passes the first fragment-sized DTLS application
// record written after arm and drops every later one. Only the stalled
// message's fragments are that large while armed.
type fragmentStallPacketConn struct {
	*net.UDPConn

	mu      sync.Mutex
	armed   bool
	passed  int
	dropped int
}

func (c *fragmentStallPacketConn) arm() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.armed = true
}

func (c *fragmentStallPacketConn) counts() (passed, dropped int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.passed, c.dropped
}

func (c *fragmentStallPacketConn) WriteTo(payload []byte, addr net.Addr) (int, error) {
	if len(payload) < stalledOpenFragmentDatagram || payload[0] != 0x17 {
		return c.UDPConn.WriteTo(payload, addr)
	}
	c.mu.Lock()
	drop := false
	if c.armed {
		if c.passed == 0 {
			c.passed++
		} else {
			c.dropped++
			drop = true
		}
	}
	c.mu.Unlock()
	if drop {
		return len(payload), nil
	}
	return c.UDPConn.WriteTo(payload, addr)
}
