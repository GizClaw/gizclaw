package gizwebrtc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

var defaultDialAPI = sync.OnceValues(func() (*webrtc.API, error) {
	api, _, err := newPionAPI(nil)
	return api, err
})

type DialConfig struct {
	API                *webrtc.API
	HTTPClient         *http.Client
	SignalingURL       string
	ICEServers         []ICEServer
	ICETransportPolicy webrtc.ICETransportPolicy
	CipherMode         CipherMode
	SecurityPolicy     giznet.SecurityPolicy
	// SCTPReceiveBufferSize overrides Pion's default association receive
	// window. Gateway upstream callers use GatewaySCTPReceiveBufferSize; public
	// client associations leave this at zero.
	SCTPReceiveBufferSize uint32
	// OnTiming receives one snapshot before Dial returns. Milestones after
	// SetRemoteDescription are measured from the start of that call.
	OnTiming func(DialTiming)
}

// DialTiming reports client-side establishment phases and readiness
// milestones without exposing Pion objects to callers.
type DialTiming struct {
	PeerConnectionConstruction time.Duration
	OfferCreation              time.Duration
	SetLocalDescription        time.Duration
	ICEGathering               time.Duration
	HTTPSignaling              time.Duration
	SetRemoteDescription       time.Duration
	ICEConnected               time.Duration
	DTLSConnected              time.Duration
	DataChannelReady           time.Duration
	Total                      time.Duration
}

type dialTimingRecorder struct {
	started       time.Time
	remoteStarted time.Time

	mu     sync.Mutex
	timing DialTiming
}

func newDialTimingRecorder() *dialTimingRecorder {
	return &dialTimingRecorder{started: time.Now()}
}

func (r *dialTimingRecorder) update(update func(*DialTiming)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	update(&r.timing)
}

func (r *dialTimingRecorder) startRemoteDescription() {
	r.mu.Lock()
	r.remoteStarted = time.Now()
	r.mu.Unlock()
}

func (r *dialTimingRecorder) sinceRemote() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.remoteStarted.IsZero() {
		return 0
	}
	return time.Since(r.remoteStarted)
}

func (r *dialTimingRecorder) markICEConnected() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timing.ICEConnected == 0 && !r.remoteStarted.IsZero() {
		r.timing.ICEConnected = time.Since(r.remoteStarted)
	}
}

func (r *dialTimingRecorder) markDTLSConnected() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timing.DTLSConnected == 0 && !r.remoteStarted.IsZero() {
		r.timing.DTLSConnected = time.Since(r.remoteStarted)
	}
}

func (r *dialTimingRecorder) snapshot() DialTiming {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timing.Total = time.Since(r.started)
	return r.timing
}

func Dial(ctx context.Context, key *giznet.KeyPair, serverPK giznet.PublicKey, cfg DialConfig) (*Listener, *Conn, error) {
	timing := newDialTimingRecorder()
	if cfg.OnTiming != nil {
		defer func() { cfg.OnTiming(timing.snapshot()) }()
	}
	if key == nil {
		return nil, nil, fmt.Errorf("gizwebrtc: nil key pair")
	}
	api := cfg.API
	var closers []func() error
	if api != nil && cfg.SCTPReceiveBufferSize != 0 {
		return nil, nil, fmt.Errorf("gizwebrtc: SCTP receive override requires the default Pion API")
	}
	if api == nil {
		if cfg.SCTPReceiveBufferSize == 0 {
			var err error
			api, err = defaultDialAPI()
			if err != nil {
				return nil, nil, err
			}
		} else {
			var err error
			api, closers, err = newPionAPI(&ListenConfig{SCTPReceiveBufferSize: cfg.SCTPReceiveBufferSize})
			if err != nil {
				return nil, nil, err
			}
		}
	}
	l := &Listener{
		key: key,
		cfg: ListenConfig{
			CipherMode:         cfg.CipherMode,
			ICEServers:         cfg.ICEServers,
			ICETransportPolicy: cfg.ICETransportPolicy,
			SecurityPolicy:     cfg.SecurityPolicy,
		},
		api:        api,
		closers:    closers,
		acceptCh:   make(chan giznet.Conn, 1),
		closeCh:    make(chan struct{}),
		replaySeen: make(map[string]int64),
	}
	if l.cfg.CipherMode == "" {
		l.cfg.CipherMode = CipherModeChaChaPoly
	}
	if err := validateICEServers(cfg.ICEServers); err != nil {
		_ = l.Close()
		return nil, nil, err
	}
	started := time.Now()
	pc, err := api.NewPeerConnection(peerConnectionConfiguration(cfg.ICEServers, cfg.ICETransportPolicy))
	timing.update(func(t *DialTiming) {
		t.PeerConnectionConstruction = time.Since(started)
	})
	if err != nil {
		_ = l.Close()
		return nil, nil, err
	}
	conn, err := newConn(serverPK, pc, cfg.SecurityPolicy, "client")
	if err != nil {
		_ = pc.Close()
		_ = l.Close()
		return nil, nil, err
	}
	ordered := false
	maxRetransmits := uint16(0)
	packetDC, err := pc.CreateDataChannel(packetLabel, &webrtc.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: &maxRetransmits,
	})
	if err != nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	if !conn.reservePacketDataChannel(packetDC) {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, ErrPacketChannel
	}
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
			timing.markICEConnected()
		}
	})
	if sctp := pc.SCTP(); sctp != nil && sctp.Transport() != nil {
		sctp.Transport().OnStateChange(func(state webrtc.DTLSTransportState) {
			if state == webrtc.DTLSTransportStateConnected {
				timing.markDTLSConnected()
			}
		})
	}
	packetDC.OnOpen(func() {
		raw, err := packetDC.DetachWithDeadline()
		if err != nil {
			_ = conn.Close()
			return
		}
		conn.setPacket(packetDC, raw)
	})

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	started = time.Now()
	offer, err := pc.CreateOffer(nil)
	timing.update(func(t *DialTiming) {
		t.OfferCreation = time.Since(started)
	})
	if err != nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	started = time.Now()
	if err := pc.SetLocalDescription(offer); err != nil {
		timing.update(func(t *DialTiming) {
			t.SetLocalDescription = time.Since(started)
		})
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	timing.update(func(t *DialTiming) {
		t.SetLocalDescription = time.Since(started)
	})
	started = time.Now()
	if err := waitForGathering(ctx, gatherComplete); err != nil {
		timing.update(func(t *DialTiming) {
			t.ICEGathering = time.Since(started)
		})
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	timing.update(func(t *DialTiming) {
		t.ICEGathering = time.Since(started)
	})
	if pc.LocalDescription() == nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, fmt.Errorf("gizwebrtc: missing local offer")
	}
	started = time.Now()
	answerSDP, err := postOffer(ctx, key, serverPK, pc.LocalDescription().SDP, cfg)
	timing.update(func(t *DialTiming) {
		t.HTTPSignaling = time.Since(started)
	})
	if err != nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	timing.startRemoteDescription()
	started = time.Now()
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answerSDP}); err != nil {
		timing.update(func(t *DialTiming) {
			t.SetRemoteDescription = time.Since(started)
		})
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	timing.update(func(t *DialTiming) {
		t.SetRemoteDescription = time.Since(started)
	})
	if err := waitForPacketChannel(ctx, conn.readyCh); err != nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	if state := pc.ICEConnectionState(); state == webrtc.ICEConnectionStateConnected || state == webrtc.ICEConnectionStateCompleted {
		timing.markICEConnected()
	}
	if sctp := pc.SCTP(); sctp != nil && sctp.Transport() != nil &&
		sctp.Transport().State() == webrtc.DTLSTransportStateConnected {
		timing.markDTLSConnected()
	}
	dataChannelReady := timing.sinceRemote()
	timing.update(func(t *DialTiming) { t.DataChannelReady = dataChannelReady })
	l.enqueueConn(conn)
	return l, conn, nil
}

func waitForGathering(ctx context.Context, gatherComplete <-chan struct{}) error {
	select {
	case <-gatherComplete:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitForPacketChannel(ctx context.Context, ready <-chan struct{}) error {
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("gizwebrtc: wait for packet channel: %w", ctx.Err())
	}
}

func postOffer(ctx context.Context, key *giznet.KeyPair, serverPK giznet.PublicKey, offerSDP string, cfg DialConfig) (string, error) {
	if cfg.SignalingURL == "" {
		return "", fmt.Errorf("gizwebrtc: empty signaling URL")
	}
	var nonceRaw [signalingNonceBytes]byte
	if _, err := rand.Read(nonceRaw[:]); err != nil {
		return "", err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceRaw[:])
	ts := time.Now().Unix()
	reqAEAD, reqNonce, respAEAD, respNonce, err := deriveSignaling(key, serverPK, nonce, ts, cfg.CipherMode)
	if err != nil {
		return "", err
	}
	body := reqAEAD.Seal(nil, reqNonce, []byte(offerSDP), requestAAD(key.Public, ts, nonce))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.SignalingURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Giznet-Public-Key", key.Public.String())
	req.Header.Set("X-Giznet-Timestamp", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Giznet-Nonce", nonce)
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gizwebrtc: signaling failed: %s: %s", resp.Status, string(respBody))
	}
	answer, err := respAEAD.Open(nil, respNonce, respBody, responseAAD(key.Public, ts, nonce))
	if err != nil {
		return "", err
	}
	return string(answer), nil
}
