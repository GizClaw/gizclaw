package gizwebrtc

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

const signalingBodyLimit = 256 * 1024

func (l *Listener) SignalingHandler() http.Handler {
	return http.HandlerFunc(l.handleOffer)
}

func (l *Listener) handleOffer(w http.ResponseWriter, r *http.Request) {
	if l == nil || l.closed.Load() {
		writeSignalingError(w, http.StatusServiceUnavailable, "listener_closed")
		return
	}
	if r.Method != http.MethodPost {
		writeSignalingError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/octet-stream" {
		writeSignalingError(w, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}
	clientPK, err := parseHeaderPublicKey(r.Header.Get("X-Giznet-Public-Key"))
	if err != nil {
		writeSignalingError(w, http.StatusBadRequest, "invalid_public_key")
		return
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get("X-Giznet-Timestamp")), 10, 64)
	if err != nil {
		writeSignalingError(w, http.StatusBadRequest, "invalid_timestamp")
		return
	}
	now := time.Now().Unix()
	if ts < now-120 || ts > now+120 {
		writeSignalingError(w, http.StatusBadRequest, "expired_request")
		return
	}
	nonce := strings.TrimSpace(r.Header.Get("X-Giznet-Nonce"))
	if nonce == "" {
		writeSignalingError(w, http.StatusBadRequest, "invalid_nonce")
		return
	}
	if err := l.checkReplay(clientPK, nonce, now); err != nil {
		writeSignalingError(w, http.StatusConflict, "replayed_nonce")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, signalingBodyLimit))
	if err != nil {
		writeSignalingError(w, http.StatusBadRequest, "body_too_large")
		return
	}
	reqAEAD, reqNonce, respAEAD, respNonce, err := deriveSignaling(l.key, clientPK, nonce, ts, l.cfg.CipherMode)
	if err != nil {
		writeSignalingError(w, http.StatusBadRequest, "invalid_crypto")
		return
	}
	offerSDP, err := reqAEAD.Open(nil, reqNonce, body, requestAAD(clientPK, ts, nonce))
	if err != nil {
		writeSignalingError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := validateOfferSDP(string(offerSDP)); err != nil {
		writeSignalingError(w, http.StatusBadRequest, signalingSDPErrorCode(err))
		return
	}
	if l.cfg.SecurityPolicy != nil && !l.cfg.SecurityPolicy.AllowPeer(clientPK) {
		writeSignalingError(w, http.StatusForbidden, "peer_forbidden")
		return
	}

	answerSDP, conn, err := l.acceptOffer(clientPK, string(offerSDP))
	if err != nil {
		writeSignalingError(w, http.StatusInternalServerError, "answer_failed")
		return
	}
	sealed := respAEAD.Seal(nil, respNonce, []byte(answerSDP), responseAAD(clientPK, ts, nonce))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(sealed)

	go func() {
		select {
		case <-conn.readyCh:
			l.enqueueConn(conn)
		case <-conn.closeCh:
		case <-l.closeCh:
			_ = conn.Close()
		}
	}()
}

func (l *Listener) acceptOffer(clientPK giznet.PublicKey, offerSDP string) (string, *Conn, error) {
	pc, err := l.api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return "", nil, err
	}
	conn, err := newConn(clientPK, pc, l.cfg.SecurityPolicy, "server")
	if err != nil {
		_ = pc.Close()
		return "", nil, err
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		_ = conn.Close()
		return "", nil, err
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = conn.Close()
		return "", nil, err
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = conn.Close()
		return "", nil, err
	}
	<-gatherComplete
	if pc.LocalDescription() == nil {
		_ = conn.Close()
		return "", nil, fmt.Errorf("gizwebrtc: missing local answer")
	}
	answerSDP, err := rewriteSDPUDPHostCandidates(pc.LocalDescription().SDP, l.cfg.PublicICEUDPAddr)
	if err != nil {
		_ = conn.Close()
		return "", nil, err
	}
	return answerSDP, conn, nil
}

func parseHeaderPublicKey(text string) (giznet.PublicKey, error) {
	var pk giznet.PublicKey
	if err := pk.UnmarshalText([]byte(text)); err != nil {
		return pk, err
	}
	if pk.IsZero() {
		return pk, giznet.ErrInvalidPublicKey
	}
	return pk, nil
}

func validateOfferSDP(sdp string) error {
	lower := strings.ToLower(sdp)
	if !strings.Contains(lower, "a=fingerprint:") {
		return fmt.Errorf("%w: missing fingerprint", ErrInvalidSDP)
	}
	hasOpusAudio := strings.Contains(lower, "m=audio") && strings.Contains(lower, "opus/48000")
	hasDataChannel := strings.Contains(lower, "m=application") && strings.Contains(lower, "webrtc-datachannel")
	if !hasOpusAudio && !hasDataChannel {
		return fmt.Errorf("%w: missing opus audio or data channel", ErrUnsupportedCodec)
	}
	return nil
}

func signalingSDPErrorCode(err error) string {
	if errors.Is(err, ErrUnsupportedCodec) {
		return "missing_opus_audio"
	}
	return "invalid_sdp"
}

func writeSignalingError(w http.ResponseWriter, code int, name string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"error":%q}`+"\n", name)
}
