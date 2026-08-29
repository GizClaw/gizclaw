package gizwebrtc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

const (
	dialICEUDPAddr            = "0.0.0.0:0"
	dialICEUDPReadBufferSize  = 256 * 1024
	dialICEUDPWriteBufferSize = 256 * 1024
	// Pion sends 25 binding requests over about five seconds. One additional
	// second lets the final response and DataChannel open before changing tuple.
	dialInitialAttemptTimeout = 6 * time.Second
	dialMaxAttempts           = 2
)

var errDialICEAttempt = errors.New("gizwebrtc: ICE establishment attempt")

type DialConfig struct {
	// MetricsNodeRole identifies the process initiating this dial. Values other
	// than "edge" use the application default.
	MetricsNodeRole    string
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
	// Attempts is the number of fresh PeerConnections and local UDP tuples used.
	Attempts                   int
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
	SelectedCandidatePair      *ICECandidatePairObservation
}

func (t *DialTiming) add(attempt DialTiming) {
	t.PeerConnectionConstruction += attempt.PeerConnectionConstruction
	t.OfferCreation += attempt.OfferCreation
	t.SetLocalDescription += attempt.SetLocalDescription
	t.ICEGathering += attempt.ICEGathering
	t.HTTPSignaling += attempt.HTTPSignaling
	t.SetRemoteDescription += attempt.SetRemoteDescription
	t.ICEConnected += attempt.ICEConnected
	t.DTLSConnected += attempt.DTLSConnected
	t.DataChannelReady += attempt.DataChannelReady
	if attempt.SelectedCandidatePair != nil {
		t.SelectedCandidatePair = attempt.SelectedCandidatePair
	}
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

type dialAttemptFunc func(
	context.Context,
	*giznet.KeyPair,
	giznet.PublicKey,
	DialConfig,
	time.Duration,
) (*Listener, *Conn, error)

func Dial(ctx context.Context, key *giznet.KeyPair, serverPK giznet.PublicKey, cfg DialConfig) (*Listener, *Conn, error) {
	return dialWithAttempts(ctx, key, serverPK, cfg, dialInitialAttemptTimeout, dialAttempt)
}

func dialWithAttempts(
	ctx context.Context,
	key *giznet.KeyPair,
	serverPK giznet.PublicKey,
	cfg DialConfig,
	initialAttemptTimeout time.Duration,
	attempt dialAttemptFunc,
) (*Listener, *Conn, error) {
	started := time.Now()
	combined := DialTiming{}
	var finalErr error
	defer func() {
		combined.Total = time.Since(started)
		recordDial(ctx, cfg.MetricsNodeRole, combined, finalErr)
	}()
	callback := cfg.OnTiming
	cfg.OnTiming = func(observation DialTiming) {
		combined.add(observation)
	}
	if key == nil {
		combined.Total = time.Since(started)
		if callback != nil {
			callback(combined)
		}
		finalErr = fmt.Errorf("gizwebrtc: nil key pair")
		return nil, nil, finalErr
	}
	maxAttempts := dialMaxAttempts
	if cfg.API != nil {
		maxAttempts = 1
	}
	var firstErr error
	for attemptIndex := range maxAttempts {
		packetChannelTimeout := time.Duration(0)
		if attemptIndex == 0 && maxAttempts > 1 {
			packetChannelTimeout = initialAttemptTimeout
		}
		listener, conn, err := attempt(ctx, key, serverPK, cfg, packetChannelTimeout)
		combined.Attempts = attemptIndex + 1
		if err == nil {
			combined.Total = time.Since(started)
			if callback != nil {
				callback(combined)
			}
			return listener, conn, nil
		}
		if attemptIndex == 0 {
			firstErr = err
		}
		if attemptIndex+1 == maxAttempts || ctx.Err() != nil || !errors.Is(err, errDialICEAttempt) {
			combined.Total = time.Since(started)
			if callback != nil {
				callback(combined)
			}
			if attemptIndex > 0 {
				finalErr = fmt.Errorf("gizwebrtc: dial failed after %d ICE attempts: first: %v; last: %w", attemptIndex+1, firstErr, err)
				return nil, nil, finalErr
			}
			finalErr = err
			return nil, nil, finalErr
		}
	}
	finalErr = errors.New("gizwebrtc: no ICE dial attempt configured")
	return nil, nil, finalErr
}

func dialAttempt(
	ctx context.Context,
	key *giznet.KeyPair,
	serverPK giznet.PublicKey,
	cfg DialConfig,
	packetChannelTimeout time.Duration,
) (*Listener, *Conn, error) {
	timing := newDialTimingRecorder()
	if cfg.OnTiming != nil {
		defer func() { cfg.OnTiming(timing.snapshot()) }()
	}
	api := cfg.API
	var closers []func() error
	if api != nil && cfg.SCTPReceiveBufferSize != 0 {
		return nil, nil, fmt.Errorf("gizwebrtc: SCTP receive override requires the default Pion API")
	}
	if api == nil {
		var err error
		api, closers, err = newDialPionAPI(cfg.SCTPReceiveBufferSize)
		if err != nil {
			return nil, nil, err
		}
	}
	l := &Listener{
		key: key,
		cfg: ListenConfig{
			MetricsNodeRole:    cfg.MetricsNodeRole,
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
	conn.nodeRole = normalizedMetricsNodeRole(cfg.MetricsNodeRole)
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
	packetCtx := ctx
	cancelPacketWait := func() {}
	if packetChannelTimeout > 0 {
		packetCtx, cancelPacketWait = context.WithTimeout(ctx, packetChannelTimeout)
	}
	defer cancelPacketWait()
	if err := waitForPacketChannel(packetCtx, conn.readyCh); err != nil {
		err = fmt.Errorf("%w: %w: %s", errDialICEAttempt, err, peerConnectionStateDetails(pc))
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
	timing.update(func(t *DialTiming) {
		t.DataChannelReady = dataChannelReady
		t.SelectedCandidatePair = selectedICEObservation(pc)
	})
	l.enqueueConn(conn)
	return l, conn, nil
}

func newDialPionAPI(sctpReceiveBufferSize uint32) (*webrtc.API, []func() error, error) {
	// Bind one wildcard UDP socket per PeerConnection. All host candidates for
	// that connection then share one OS-unique source port instead of allocating
	// the same ephemeral port independently on different local interfaces. This
	// prevents a NAT from collapsing those candidates onto an indistinguishable
	// remote tuple while keeping mux address ownership request-scoped. A private
	// client socket uses bounded 256 KiB buffers instead of the 4 MiB buffers
	// reserved for shared listener sockets.
	api, _, closers, err := newPionAPIsWithICEUDPBuffers(&ListenConfig{
		ICEUDPAddr:            dialICEUDPAddr,
		SCTPReceiveBufferSize: sctpReceiveBufferSize,
	}, false, dialICEUDPReadBufferSize, dialICEUDPWriteBufferSize)
	return api, closers, err
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

func peerConnectionStateDetails(pc *webrtc.PeerConnection) string {
	if pc == nil {
		return "peer_connection_state=unavailable"
	}
	dtlsState := "unavailable"
	sctpState := "unavailable"
	if sctp := pc.SCTP(); sctp != nil {
		sctpState = sctp.State().String()
		if dtls := sctp.Transport(); dtls != nil {
			dtlsState = dtls.State().String()
		}
	}
	detail := fmt.Sprintf(
		"peer_connection_state=%s ice_state=%s ice_gathering_state=%s signaling_state=%s dtls_state=%s sctp_state=%s",
		pc.ConnectionState(),
		pc.ICEConnectionState(),
		pc.ICEGatheringState(),
		pc.SignalingState(),
		dtlsState,
		sctpState,
	)
	pair, ok := selectedICECandidatePair(pc.GetStats())
	if !ok {
		return detail + " ice_pair=unavailable"
	}
	return fmt.Sprintf(
		"%s ice_pair_state=%s nominated=%t packets_sent=%d packets_received=%d requests_sent=%d responses_received=%d requests_received=%d responses_sent=%d packets_discarded_on_send=%d",
		detail,
		pair.State,
		pair.Nominated,
		pair.PacketsSent,
		pair.PacketsReceived,
		pair.RequestsSent,
		pair.ResponsesReceived,
		pair.RequestsReceived,
		pair.ResponsesSent,
		pair.PacketsDiscardedOnSend,
	)
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
