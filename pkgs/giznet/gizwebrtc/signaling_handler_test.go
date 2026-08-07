package gizwebrtc

import (
	"bytes"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestSignalingTimingServerHeader(t *testing.T) {
	header := (signalingTiming{
		peerConnection: 1250 * time.Microsecond,
		iceGathering:   1000125 * time.Microsecond,
	}).serverTiming()
	if !strings.Contains(header, "giz_peer_connection;dur=1.250") ||
		!strings.Contains(header, "giz_ice_gathering;dur=1000.125") {
		t.Fatalf("Server-Timing = %q", header)
	}
}

func TestSignalingHandlerRejectsMalformedRequests(t *testing.T) {
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)
	listener := newTestSignalingListener(t, serverKey, CipherModePlaintext, allowAllPolicy{})
	defer listener.Close()

	t.Run("method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, SignalingPath, nil)
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
	})

	t.Run("content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, SignalingPath, nil)
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusUnsupportedMediaType, "unsupported_media_type")
	})

	t.Run("public key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, SignalingPath, nil)
		req.Header.Set("Content-Type", "application/octet-stream")
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusBadRequest, "invalid_public_key")
	})

	t.Run("timestamp", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, SignalingPath, nil)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Giznet-Public-Key", clientKey.Public.String())
		req.Header.Set("X-Giznet-Timestamp", "bad")
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusBadRequest, "invalid_timestamp")
	})

	t.Run("expired", func(t *testing.T) {
		req := newSealedOfferRequest(t, serverKey, clientKey, CipherModePlaintext, time.Now().Add(-3*time.Minute), fixedNonce(1), []byte("x"))
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusBadRequest, "expired_request")
	})
}

func TestSignalingHandlerRejectsUnauthorizedCiphertextAndReplay(t *testing.T) {
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)
	listener := newTestSignalingListener(t, serverKey, CipherModeChaChaPoly, allowAllPolicy{})
	defer listener.Close()

	nonce := fixedNonce(2)
	req := newBaseSignalingRequest(clientKey.Public, time.Now(), nonce, []byte("not ciphertext"))
	rec := httptest.NewRecorder()
	listener.SignalingHandler().ServeHTTP(rec, req)
	assertSignalingStatus(t, rec, http.StatusUnauthorized, "unauthorized")

	req = newBaseSignalingRequest(clientKey.Public, time.Now(), nonce, []byte("not ciphertext"))
	rec = httptest.NewRecorder()
	listener.SignalingHandler().ServeHTTP(rec, req)
	assertSignalingStatus(t, rec, http.StatusConflict, "replayed_nonce")
}

func TestCheckReplayPrunesPeriodicallyAndExpiresExactKey(t *testing.T) {
	key := mustKeyPair(t)
	listener := &Listener{replaySeen: make(map[string]int64)}
	const now = int64(10_000)
	if err := listener.checkReplay(key.Public, "nonce", now); err != nil {
		t.Fatalf("first checkReplay error = %v", err)
	}
	if err := listener.checkReplay(key.Public, "nonce", now+1); !errors.Is(err, ErrSignalingReplay) {
		t.Fatalf("duplicate checkReplay error = %v, want %v", err, ErrSignalingReplay)
	}
	if err := listener.checkReplay(key.Public, "nonce", now+signalingReplayTTL+1); err != nil {
		t.Fatalf("expired checkReplay error = %v", err)
	}

	listener.replaySeen["stale"] = now
	listener.replayNextPrune = now + 2*signalingReplayTTL
	if err := listener.checkReplay(key.Public, "another", now+signalingReplayTTL+2); err != nil {
		t.Fatalf("checkReplay before prune deadline error = %v", err)
	}
	if _, ok := listener.replaySeen["stale"]; !ok {
		t.Fatal("checkReplay pruned the map before its scheduled deadline")
	}
	if err := listener.checkReplay(key.Public, "third", now+2*signalingReplayTTL); err != nil {
		t.Fatalf("checkReplay at prune deadline error = %v", err)
	}
	if _, ok := listener.replaySeen["stale"]; ok {
		t.Fatal("checkReplay retained an expired entry at the prune deadline")
	}
}

func TestSignalingHandlerRejectsInvalidSDPAndForbiddenPeer(t *testing.T) {
	serverKey := mustKeyPair(t)
	clientKey := mustKeyPair(t)

	t.Run("missing fingerprint", func(t *testing.T) {
		listener := newTestSignalingListener(t, serverKey, CipherModePlaintext, allowAllPolicy{})
		defer listener.Close()
		req := newSealedOfferRequest(t, serverKey, clientKey, CipherModePlaintext, time.Now(), fixedNonce(3), []byte("m=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=rtpmap:111 opus/48000/2\r\n"))
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusBadRequest, "invalid_sdp")
	})

	t.Run("missing opus", func(t *testing.T) {
		listener := newTestSignalingListener(t, serverKey, CipherModePlaintext, allowAllPolicy{})
		defer listener.Close()
		req := newSealedOfferRequest(t, serverKey, clientKey, CipherModePlaintext, time.Now(), fixedNonce(4), []byte("m=audio 9 UDP/TLS/RTP/SAVPF 0\r\na=fingerprint:sha-256 AA\r\n"))
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusBadRequest, "missing_opus_audio")
	})

	t.Run("data channel without opus", func(t *testing.T) {
		listener := newTestSignalingListener(t, serverKey, CipherModePlaintext, denyAllPolicy{})
		defer listener.Close()
		req := newSealedOfferRequest(t, serverKey, clientKey, CipherModePlaintext, time.Now(), fixedNonce(6), []byte(minimalDataChannelSignalingSDP))
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusBadRequest, "missing_opus_audio")
	})

	t.Run("recvonly opus", func(t *testing.T) {
		listener := newTestSignalingListener(t, serverKey, CipherModePlaintext, allowAllPolicy{})
		defer listener.Close()
		req := newSealedOfferRequest(t, serverKey, clientKey, CipherModePlaintext, time.Now(), fixedNonce(7), []byte(minimalRecvOnlySignalingSDP))
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusBadRequest, "missing_opus_audio")
	})

	t.Run("forbidden peer", func(t *testing.T) {
		listener := newTestSignalingListener(t, serverKey, CipherModePlaintext, denyAllPolicy{})
		defer listener.Close()
		req := newSealedOfferRequest(t, serverKey, clientKey, CipherModePlaintext, time.Now(), fixedNonce(5), []byte(minimalValidSignalingSDP))
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusForbidden, "peer_forbidden")
	})
}

const minimalValidSignalingSDP = "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=rtpmap:111 opus/48000/2\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=sctp-port:5000\r\na=fingerprint:sha-256 AA\r\n"
const minimalDataChannelSignalingSDP = "v=0\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=sctp-port:5000\r\na=fingerprint:sha-256 AA\r\n"
const minimalRecvOnlySignalingSDP = "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=recvonly\r\na=rtpmap:111 opus/48000/2\r\nm=application 9 UDP/DTLS/SCTP webrtc-datachannel\r\na=sctp-port:5000\r\na=fingerprint:sha-256 AA\r\n"

type denyAllPolicy struct{}

func (denyAllPolicy) AllowPeer(giznet.PublicKey) bool {
	return false
}

func (denyAllPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return false
}

func mustKeyPair(t *testing.T) *giznet.KeyPair {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	return key
}

func newTestSignalingListener(t *testing.T, key *giznet.KeyPair, mode CipherMode, policy giznet.SecurityPolicy) *Listener {
	t.Helper()
	listener, err := (&ListenConfig{CipherMode: mode, SecurityPolicy: policy}).Listen(key)
	if err != nil {
		t.Fatalf("Listen error = %v", err)
	}
	return listener
}

func newSealedOfferRequest(t *testing.T, serverKey, clientKey *giznet.KeyPair, mode CipherMode, ts time.Time, nonce string, plaintext []byte) *http.Request {
	t.Helper()
	reqAEAD, reqNonce, _, _, err := deriveSignaling(clientKey, serverKey.Public, nonce, ts.Unix(), mode)
	if err != nil {
		t.Fatalf("deriveSignaling error = %v", err)
	}
	body := reqAEAD.Seal(nil, reqNonce, plaintext, requestAAD(clientKey.Public, ts.Unix(), nonce))
	return newBaseSignalingRequest(clientKey.Public, ts, nonce, body)
}

func newBaseSignalingRequest(client giznet.PublicKey, ts time.Time, nonce string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, SignalingPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Giznet-Public-Key", client.String())
	req.Header.Set("X-Giznet-Timestamp", timeFormatUnix(ts))
	req.Header.Set("X-Giznet-Nonce", nonce)
	return req
}

func timeFormatUnix(ts time.Time) string {
	return strconv.FormatInt(ts.Unix(), 10)
}

func fixedNonce(seed byte) string {
	var nonce [signalingNonceBytes]byte
	for i := range nonce {
		nonce[i] = seed
	}
	return base64.RawURLEncoding.EncodeToString(nonce[:])
}

func assertSignalingStatus(t *testing.T, rec *httptest.ResponseRecorder, want int, errorName string) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d body=%s, want %d", rec.Code, rec.Body.String(), want)
	}
	if errorName != "" && !strings.Contains(rec.Body.String(), errorName) {
		t.Fatalf("body = %s, want error %q", rec.Body.String(), errorName)
	}
}
