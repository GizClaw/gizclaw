package gizwebrtc

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestOfferEnvelope(t *testing.T) {
	// This byte vector is shared by the Go, JS, Dart and C signaling tests.
	const vector = "475a4f4601000300ff01763d300d0a"
	for _, credential := range [][]byte{nil, {}, {0, 255, 1}, bytes.Repeat([]byte{9}, MaxCredentialBytes)} {
		encoded, err := encodeOfferEnvelope("v=0\r\n", credential)
		if err != nil {
			t.Fatal(err)
		}
		if len(credential) == 0 && string(encoded) != "v=0\r\n" {
			t.Fatal("empty credential changed legacy SDP")
		}
		if len(credential) == 3 && hex.EncodeToString(encoded) != vector {
			t.Fatalf("wire vector = %x", encoded)
		}
		sdp, got, err := decodeOfferEnvelope(encoded)
		if err != nil || string(sdp) != "v=0\r\n" || !bytes.Equal(got, credential) {
			t.Fatalf("decode: %q %x %v", sdp, got, err)
		}
	}
	if _, err := encodeOfferEnvelope("v=0", make([]byte, MaxCredentialBytes+1)); err == nil {
		t.Fatal("oversized credential accepted")
	}
	for _, encoded := range [][]byte{
		[]byte("GZOF"), []byte("GZOF\x02\x00\x00v=0"),
		[]byte("GZOF\x01\x00\x03x"), []byte("GZOF\x01\x10\x01"),
	} {
		if _, _, err := decodeOfferEnvelope(encoded); err == nil {
			t.Fatalf("invalid envelope accepted: %x", encoded)
		}
	}
}

type admissionPolicyFunc func(context.Context, giznet.PeerAdmission) bool

func (f admissionPolicyFunc) AllowPeer(ctx context.Context, admission giznet.PeerAdmission) bool {
	return f(ctx, admission)
}
func (admissionPolicyFunc) AllowService(giznet.PublicKey, uint64) bool { return true }

func TestSignalingAdmissionReceivesRequestContext(t *testing.T) {
	server, client := mustKeyPair(t), mustKeyPair(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	credential := []byte{0, 255, 1}
	calls := 0
	listener := newTestSignalingListener(t, server, CipherModeChaChaPoly, admissionPolicyFunc(func(got context.Context, admission giznet.PeerAdmission) bool {
		calls++
		if got != ctx || admission.PublicKey != client.Public || !bytes.Equal(admission.Credential, credential) {
			t.Error("admission lost request context, key or credential")
		}
		return false
	}))
	t.Cleanup(func() { _ = listener.Close() })
	plaintext, err := encodeOfferEnvelope(minimalValidSignalingSDP, credential)
	if err != nil {
		t.Fatal(err)
	}
	req := newSealedOfferRequest(t, server, client, CipherModeChaChaPoly, time.Now(), fixedNonce(20), plaintext).WithContext(ctx)
	rec := httptest.NewRecorder()
	listener.SignalingHandler().ServeHTTP(rec, req)
	assertSignalingStatus(t, rec, http.StatusForbidden, "peer_forbidden")
	if calls != 1 {
		t.Fatalf("policy calls = %d", calls)
	}
	for i, bad := range [][]byte{[]byte("GZOF"), []byte("GZOF\x02\x00\x00"), []byte("GZOF\x01\x10\x01"), []byte("GZOF\x01\x00\x03x")} {
		req = newSealedOfferRequest(t, server, client, CipherModeChaChaPoly, time.Now(), fixedNonce(byte(21+i)), bad)
		rec = httptest.NewRecorder()
		listener.SignalingHandler().ServeHTTP(rec, req)
		assertSignalingStatus(t, rec, http.StatusBadRequest, "invalid_credential")
	}
	req = newSealedOfferRequest(t, server, client, CipherModeChaChaPoly, time.Now(), fixedNonce(30), plaintext)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	body[8] ^= 1
	req.Body = io.NopCloser(bytes.NewReader(body))
	rec = httptest.NewRecorder()
	listener.SignalingHandler().ServeHTTP(rec, req)
	assertSignalingStatus(t, rec, http.StatusUnauthorized, "unauthorized")
	if calls != 1 {
		t.Fatal("invalid envelope or tampered ciphertext reached policy")
	}
}

func TestDialEncryptedAdmissionAndLegacySDP(t *testing.T) {
	for _, credential := range [][]byte{nil, {0, 255, 1}} {
		t.Run(hex.EncodeToString(credential), func(t *testing.T) {
			server, client := mustKeyPair(t), mustKeyPair(t)
			observed := make(chan giznet.PeerAdmission, 1)
			listener := newTestSignalingListener(t, server, CipherModeChaChaPoly, admissionPolicyFunc(func(_ context.Context, a giznet.PeerAdmission) bool {
				observed <- giznet.PeerAdmission{PublicKey: a.PublicKey, Credential: bytes.Clone(a.Credential)}
				return bytes.Equal(a.Credential, credential)
			}))
			defer listener.Close()
			httpServer := httptest.NewServer(listener.SignalingHandler())
			defer httpServer.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			local, conn, err := Dial(ctx, client, server.Public, DialConfig{
				SignalingURL: httpServer.URL + SignalingPath, Credential: credential,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer local.Close()
			defer conn.Close()
			accepted := acceptConn(t, listener)
			defer accepted.Close()
			got := <-observed
			if got.PublicKey != client.Public || !bytes.Equal(got.Credential, credential) {
				t.Fatal("incorrect admission")
			}
		})
	}
}

func TestPlaintextAdmissionCredentialRejected(t *testing.T) {
	server, client := mustKeyPair(t), mustKeyPair(t)
	if _, err := postOffer(t.Context(), client, server.Public, minimalValidSignalingSDP, DialConfig{CipherMode: CipherModePlaintext, Credential: []byte{1}}); err == nil {
		t.Fatal("plaintext sender accepted credential")
	}
	listener := newTestSignalingListener(t, server, CipherModePlaintext, admissionPolicyFunc(func(context.Context, giznet.PeerAdmission) bool {
		t.Fatal("plaintext credential reached policy")
		return true
	}))
	defer listener.Close()
	plaintext, err := encodeOfferEnvelope(minimalValidSignalingSDP, []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	listener.SignalingHandler().ServeHTTP(rec, newSealedOfferRequest(t, server, client, CipherModePlaintext, time.Now(), fixedNonce(31), plaintext))
	assertSignalingStatus(t, rec, http.StatusBadRequest, "invalid_credential")
}
