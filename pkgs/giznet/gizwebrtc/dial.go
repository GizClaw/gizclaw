package gizwebrtc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

type DialConfig struct {
	API            *webrtc.API
	HTTPClient     *http.Client
	SignalingURL   string
	ICEServers     []ICEServer
	CipherMode     CipherMode
	SecurityPolicy giznet.SecurityPolicy
}

func Dial(ctx context.Context, key *giznet.KeyPair, serverPK giznet.PublicKey, cfg DialConfig) (*Listener, *Conn, error) {
	if key == nil {
		return nil, nil, fmt.Errorf("gizwebrtc: nil key pair")
	}
	api := cfg.API
	var closers []func() error
	if api == nil {
		var err error
		api, closers, err = newPionAPI(nil)
		if err != nil {
			return nil, nil, err
		}
	}
	l := &Listener{
		key:        key,
		cfg:        ListenConfig{CipherMode: cfg.CipherMode, SecurityPolicy: cfg.SecurityPolicy},
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
	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: webrtcICEServers(cfg.ICEServers)})
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
	packetDC.OnClose(func() {
		_ = conn.Close()
	})
	packetDC.OnOpen(func() {
		raw, err := packetDC.DetachWithDeadline()
		if err != nil {
			_ = conn.Close()
			return
		}
		conn.setPacket(packetDC, raw)
	})

	gatherComplete := webrtc.GatheringCompletePromise(pc)
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	<-gatherComplete
	if pc.LocalDescription() == nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, fmt.Errorf("gizwebrtc: missing local offer")
	}
	answerSDP, err := postOffer(ctx, key, serverPK, pc.LocalDescription().SDP, cfg)
	if err != nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answerSDP}); err != nil {
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, err
	}
	select {
	case <-conn.readyCh:
		l.enqueueConn(conn)
		return l, conn, nil
	case <-ctx.Done():
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, ctx.Err()
	case <-time.After(10 * time.Second):
		_ = conn.Close()
		_ = l.Close()
		return nil, nil, fmt.Errorf("gizwebrtc: timeout waiting for packet channel")
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
