package gizwebrtc

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

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

	t.Run("forbidden peer", func(t *testing.T) {
		listener := newTestSignalingListener(t, serverKey, CipherModePlaintext, denyAllPolicy{})
		defer listener.Close()
		req := newSealedOfferRequest(t, serverKey, clientKey, CipherModePlaintext, time.Now(), fixedNonce(5), []byte(minimalValidSignalingSDP))
		rec := httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusForbidden, "peer_forbidden")
	})
}

const minimalValidSignalingSDP = "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=rtpmap:111 opus/48000/2\r\na=fingerprint:sha-256 AA\r\n"

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
