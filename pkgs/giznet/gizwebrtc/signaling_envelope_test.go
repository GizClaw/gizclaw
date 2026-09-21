package gizwebrtc

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"
	"google.golang.org/protobuf/proto"
)

func TestOfferEnvelope(t *testing.T) {
	// These protobuf vectors are shared by Go, JS, Dart and C signaling tests.
	for _, tc := range []struct {
		credential *giznetpb.AdmissionCredential
		hex        string
	}{
		{&giznetpb.AdmissionCredential{Version: 1, Type: "registration_token", Value: "token"}, "08011212726567697374726174696f6e5f746f6b656e1a05746f6b656e"},
		{&giznetpb.AdmissionCredential{Version: 2, Type: "custom", Value: "令牌"}, "08021206637573746f6d1a06e4bba4e7898c"},
		{&giznetpb.AdmissionCredential{Type: "x"}, "120178"},
	} {
		encoded, err := encodeOfferEnvelope("v=0\r\n", tc.credential)
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString(encoded[7:len(encoded)-5]) != tc.hex {
			t.Fatalf("wire vector = %x", encoded)
		}
		sdp, got, err := decodeOfferEnvelope(encoded)
		if err != nil || string(sdp) != "v=0\r\n" || !proto.Equal(got, tc.credential) {
			t.Fatalf("incorrect decoded fields: %v", err)
		}
	}
	for _, credential := range []*giznetpb.AdmissionCredential{nil, {Version: 1, Type: "x", Value: strings.Repeat("x", 4088)}} {
		encoded, err := encodeOfferEnvelope("v=0\r\n", credential)
		if err != nil {
			t.Fatal(err)
		}
		if credential == nil && string(encoded) != "v=0\r\n" {
			t.Fatal("absent credential changed legacy SDP")
		}
		if credential != nil && len(encoded) != 7+4096+5 {
			t.Fatal("maximum encoded boundary changed")
		}
		_, got, err := decodeOfferEnvelope(encoded)
		if err != nil || !proto.Equal(got, credential) {
			t.Fatal("boundary round trip failed")
		}
	}
	for _, credential := range []*giznetpb.AdmissionCredential{
		{}, {Version: 1, Type: "x", Value: strings.Repeat("x", 4089)},
		{Version: 1, Type: strings.Repeat("x", 129)}, {Version: 1, Value: string([]byte{255})},
	} {
		if _, err := encodeOfferEnvelope("v=0", credential); err == nil {
			t.Fatal("invalid credential encoded")
		}
	}
	for _, encoded := range invalidAdmissionEnvelopes() {
		if _, _, err := decodeOfferEnvelope(encoded); err == nil {
			t.Fatalf("invalid credential accepted: %x", encoded)
		}
	}
	// A GZOF envelope with zero credential bytes also means absent.
	if _, credential, err := decodeOfferEnvelope([]byte("GZOF\x01\x00\x00v=0")); err != nil || credential != nil {
		t.Fatal("zero-length envelope credential is not absent")
	}
}

func invalidAdmissionEnvelopes() [][]byte {
	return [][]byte{
		[]byte("GZOF"), []byte("GZOF\x02\x00\x00v=0"),
		[]byte("GZOF\x01\x00\x03x"), []byte("GZOF\x01\x10\x01"),
		[]byte("GZOF\x01\x00\x01\x00v=0"),                                         // illegal protobuf tag
		[]byte("GZOF\x01\x00\x02\x08\x80v=0"),                                     // truncated varint
		[]byte("GZOF\x01\x00\x03\x1a\x02xv=0"),                                    // truncated string
		[]byte("GZOF\x01\x00\x03\x1a\x01\xffv=0"),                                 // invalid UTF-8
		[]byte("GZOF\x01\x00\x84\x12\x81\x01" + strings.Repeat("x", 129) + "v=0"), // type bound
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
	credential := &giznetpb.AdmissionCredential{Version: 7, Type: "custom", Value: "opaque"}
	calls := 0
	listener := newTestSignalingListener(t, server, CipherModeChaChaPoly, admissionPolicyFunc(func(got context.Context, admission giznet.PeerAdmission) bool {
		calls++
		if got != ctx || admission.PublicKey != client.Public || !proto.Equal(admission.Credential, credential) {
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
	for i, bad := range invalidAdmissionEnvelopes() {
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
	for _, credential := range []*giznetpb.AdmissionCredential{nil, {Version: 7, Type: "custom", Value: "opaque"}} {
		t.Run(fmt.Sprint(credential == nil), func(t *testing.T) {
			server, client := mustKeyPair(t), mustKeyPair(t)
			observed := make(chan giznet.PeerAdmission, 1)
			listener := newTestSignalingListener(t, server, CipherModeChaChaPoly, admissionPolicyFunc(func(_ context.Context, a giznet.PeerAdmission) bool {
				observed <- giznet.PeerAdmission{PublicKey: a.PublicKey, Credential: proto.CloneOf(a.Credential)}
				return proto.Equal(a.Credential, credential)
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
			if got.PublicKey != client.Public || !proto.Equal(got.Credential, credential) {
				t.Fatal("incorrect admission")
			}
		})
	}
}

func TestPlaintextAdmissionCredentialRejected(t *testing.T) {
	server, client := mustKeyPair(t), mustKeyPair(t)
	if _, err := postOffer(t.Context(), client, server.Public, minimalValidSignalingSDP, DialConfig{CipherMode: CipherModePlaintext, Credential: &giznetpb.AdmissionCredential{Version: 1}}); err == nil {
		t.Fatal("plaintext sender accepted credential")
	}
	listener := newTestSignalingListener(t, server, CipherModePlaintext, admissionPolicyFunc(func(context.Context, giznet.PeerAdmission) bool {
		t.Fatal("plaintext credential reached policy")
		return true
	}))
	defer listener.Close()
	plaintext, err := encodeOfferEnvelope(minimalValidSignalingSDP, &giznetpb.AdmissionCredential{Version: 1})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	listener.SignalingHandler().ServeHTTP(rec, newSealedOfferRequest(t, server, client, CipherModePlaintext, time.Now(), fixedNonce(31), plaintext))
	assertSignalingStatus(t, rec, http.StatusBadRequest, "invalid_credential")
}

func TestDialRejectsInvalidCredentialBeforeSignaling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("invalid credential made a signaling request")
	}))
	defer server.Close()
	for _, credential := range []*giznetpb.AdmissionCredential{
		{}, {Version: 1, Type: "x", Value: strings.Repeat("x", 4089)},
		{Version: 1, Type: strings.Repeat("x", 129)},
	} {
		timings := 0
		local, conn, err := Dial(t.Context(), mustKeyPair(t), mustKeyPair(t).Public, DialConfig{
			SignalingURL: server.URL, Credential: credential,
			OnTiming: func(timing DialTiming) {
				timings++
				if timing.Attempts != 0 {
					t.Error("invalid credential began ICE")
				}
			},
		})
		if err != errInvalidCredential || local != nil || conn != nil || timings != 1 {
			t.Fatalf("invalid credential result: local=%v conn=%v err=%v timings=%d", local, conn, err, timings)
		}
	}
}
