package gizwebrtc

import (
	"context"
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

type signalingTiming struct {
	peerConnection time.Duration
	setRemote      time.Duration
	createAnswer   time.Duration
	setLocal       time.Duration
	iceGathering   time.Duration
	rewriteSDP     time.Duration
}

func (t signalingTiming) serverTiming() string {
	metrics := []struct {
		name     string
		duration time.Duration
	}{
		{name: "giz_peer_connection", duration: t.peerConnection},
		{name: "giz_set_remote", duration: t.setRemote},
		{name: "giz_create_answer", duration: t.createAnswer},
		{name: "giz_set_local", duration: t.setLocal},
		{name: "giz_ice_gathering", duration: t.iceGathering},
		{name: "giz_rewrite_sdp", duration: t.rewriteSDP},
	}
	parts := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		parts = append(parts, fmt.Sprintf("%s;dur=%.3f", metric.name, float64(metric.duration)/float64(time.Millisecond)))
	}
	return strings.Join(parts, ", ")
}

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

	answerSDP, conn, timing, err := l.acceptOffer(r.Context(), clientPK, string(offerSDP))
	if err != nil {
		writeSignalingError(w, http.StatusInternalServerError, "answer_failed")
		return
	}
	sealed := respAEAD.Seal(nil, respNonce, []byte(answerSDP), responseAAD(clientPK, ts, nonce))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Server-Timing", timing.serverTiming())
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

func (l *Listener) acceptOffer(
	ctx context.Context,
	clientPK giznet.PublicKey,
	offerSDP string,
) (string, *Conn, signalingTiming, error) {
	timing := signalingTiming{}
	started := time.Now()
	pc, err := l.api.NewPeerConnection(peerConnectionConfiguration(l.cfg.ICEServers, l.cfg.ICETransportPolicy))
	timing.peerConnection = time.Since(started)
	if err != nil {
		return "", nil, timing, err
	}
	conn, err := newConn(clientPK, pc, l.cfg.SecurityPolicy, "server")
	if err != nil {
		_ = pc.Close()
		return "", nil, timing, err
	}
	if l.cfg.AggregateServices {
		conn.EnableServiceAccept()
	}
	started = time.Now()
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSDP}); err != nil {
		timing.setRemote = time.Since(started)
		_ = conn.Close()
		return "", nil, timing, err
	}
	timing.setRemote = time.Since(started)
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	started = time.Now()
	answer, err := pc.CreateAnswer(nil)
	timing.createAnswer = time.Since(started)
	if err != nil {
		_ = conn.Close()
		return "", nil, timing, err
	}
	started = time.Now()
	if err := pc.SetLocalDescription(answer); err != nil {
		timing.setLocal = time.Since(started)
		_ = conn.Close()
		return "", nil, timing, err
	}
	timing.setLocal = time.Since(started)
	started = time.Now()
	if err := waitForGathering(ctx, gatherComplete); err != nil {
		timing.iceGathering = time.Since(started)
		_ = conn.Close()
		return "", nil, timing, err
	}
	timing.iceGathering = time.Since(started)
	if pc.LocalDescription() == nil {
		_ = conn.Close()
		return "", nil, timing, fmt.Errorf("gizwebrtc: missing local answer")
	}
	started = time.Now()
	answerSDP, err := rewriteSDPHostCandidates(pc.LocalDescription().SDP, l.cfg.PublicICEUDPAddr, l.cfg.PublicICETCPAddr)
	timing.rewriteSDP = time.Since(started)
	if err != nil {
		_ = conn.Close()
		return "", nil, timing, err
	}
	return answerSDP, conn, timing, nil
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
	hasOpusAudio, hasDataChannel := offerHasMandatoryMedia(lower)
	if !hasOpusAudio || !hasDataChannel {
		return fmt.Errorf("%w: missing bidirectional opus audio or data channel", ErrUnsupportedCodec)
	}
	return nil
}

func offerHasMandatoryMedia(sdp string) (hasOpusAudio, hasDataChannel bool) {
	sessionDirection := "sendrecv"
	media := ""
	mediaDirection := sessionDirection
	mediaHasOpus := false
	finishMedia := func() {
		if media == "audio" && mediaHasOpus && mediaDirection == "sendrecv" {
			hasOpusAudio = true
		}
	}
	for line := range strings.SplitSeq(strings.ReplaceAll(sdp, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "m=") {
			finishMedia()
			fields := strings.Fields(strings.TrimPrefix(line, "m="))
			media = ""
			if len(fields) != 0 {
				media = fields[0]
			}
			mediaDirection = sessionDirection
			mediaHasOpus = false
			if media == "application" && strings.Contains(line, "webrtc-datachannel") {
				hasDataChannel = true
			}
			continue
		}
		switch line {
		case "a=sendrecv", "a=sendonly", "a=recvonly", "a=inactive":
			direction := strings.TrimPrefix(line, "a=")
			if media == "" {
				sessionDirection = direction
			} else {
				mediaDirection = direction
			}
		default:
			if media == "audio" &&
				strings.HasPrefix(line, "a=rtpmap:") &&
				strings.Contains(line, " opus/48000") {
				mediaHasOpus = true
			}
		}
	}
	finishMedia()
	return hasOpusAudio, hasDataChannel
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
