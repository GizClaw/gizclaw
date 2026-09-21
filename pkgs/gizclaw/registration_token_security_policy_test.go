package gizclaw

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type admissionStore struct {
	kv.Store
	reads      atomic.Int64
	beforeRead func(context.Context) error
}

func (s *admissionStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.reads.Add(1)
	if s.beforeRead != nil {
		if err := s.beforeRead(ctx); err != nil {
			return nil, err
		}
	}
	return s.Store.Get(ctx, key)
}

func newAdmissionTestPolicy(t *testing.T) (*RegistrationTokenSecurityPolicy, *admissionStore, string) {
	t.Helper()
	store := &admissionStore{Store: mustBadgerInMemory(t, nil)}
	registrations, token := registrationServerAndToken(t, "admission")
	server := &Server{manager: NewManager(&peer.Server{Store: store})}
	server.manager.RuntimeProfiles = registrations
	return NewRegistrationTokenSecurityPolicy(server), store, token
}

func TestRegistrationTokenAdmissionIsReadOnly(t *testing.T) {
	p, store, token := newAdmissionTestPolicy(t)
	unknown := giznet.PublicKey{1}
	for _, tc := range []struct {
		name       string
		credential *giznetpb.AdmissionCredential
		want       bool
	}{
		{"unknown without credential", nil, false},
		{"valid token", testRegistrationCredential(token), true},
		{"reusable token", testRegistrationCredential(token), true},
		{"invalid token", testRegistrationCredential("invalid"), false},
		{"whitespace is nonempty credential", testRegistrationCredential("  "), false},
		{"oversized", testRegistrationCredential(strings.Repeat("x", 4097)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: unknown, Credential: tc.credential}); got != tc.want {
				t.Fatalf("AllowPeer = %v, want %v", got, tc.want)
			}
			if _, err := p.server.manager.Peers.LoadPeer(t.Context(), unknown); !errors.Is(err, peer.ErrPeerNotFound) {
				t.Fatalf("admission created a Peer: %v", err)
			}
			if _, err := p.server.manager.RuntimeProfiles.ResolveOwnerProfile(t.Context(), unknown.String()); err == nil {
				t.Fatal("admission bound a RuntimeProfile owner")
			}
		})
	}
	for i, status := range []apitypes.PeerRegistrationStatus{
		apitypes.PeerRegistrationStatusActive, apitypes.PeerRegistrationStatusUnspecified, apitypes.PeerRegistrationStatusBlocked,
	} {
		key := giznet.PublicKey{byte(i + 2)}
		record := apitypes.Peer{PublicKey: key.String(), Status: status, Role: apitypes.PeerRoleClient}
		if _, err := p.server.manager.Peers.SavePeer(t.Context(), record); err != nil {
			t.Fatal(err)
		}
		for _, credential := range []*giznetpb.AdmissionCredential{nil, testRegistrationCredential(token)} {
			if got := p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: key, Credential: credential}); got != (status != apitypes.PeerRegistrationStatusBlocked) {
				t.Fatalf("status %q, credential=%t: %v", status, credential != nil, got)
			}
		}
		if p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: key, Credential: testRegistrationCredential("invalid-known")}) {
			t.Fatal("known key bypassed invalid credential")
		}
	}
	deleted, deleting := giznet.PublicKey{10}, giznet.PublicKey{11}
	if err := store.Set(t.Context(), kv.Key{"by-pubkey", deleted.String()}, []byte(`{"version":1,"state":"deleted"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.server.manager.Peers.EnsureConnectedPeer(t.Context(), deleting); err != nil {
		t.Fatal(err)
	}
	marker, err := pendingdeletion.New(pendingdeletion.KindPeer, deleting.String(), nil, pendingdeletion.ReasonPeerDelete, struct{}{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pendingdeletion.CreateOrGet(t.Context(), store, marker); err != nil {
		t.Fatal(err)
	}
	for _, key := range []giznet.PublicKey{deleted, deleting} {
		for _, credential := range []*giznetpb.AdmissionCredential{nil, testRegistrationCredential(token)} {
			if p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: key, Credential: credential}) {
				t.Fatal("deleted/deleting Peer admitted")
			}
		}
	}
}

func TestAdmissionNegativeCacheAndGlobalFailureBudget(t *testing.T) {
	p, store, token := newAdmissionTestPolicy(t)
	now := time.Now()
	p.now = func() time.Time { return now }
	deny := func(key byte, token string) {
		t.Helper()
		if p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: giznet.PublicKey{key}, Credential: testRegistrationCredential(token)}) {
			t.Fatal("invalid token admitted")
		}
	}
	deny(1, "bad")
	if _, ok := p.negative[sha256.Sum256([]byte(RegistrationTokenCredentialType+"\x00bad"))]; !ok {
		t.Fatal("negative cache does not key the canonical type/value pair")
	}
	reads := store.reads.Load()
	deny(2, "  bad\n")
	if store.reads.Load() != reads {
		t.Fatal("key/padding evaded negative cache")
	}
	now = now.Add(admissionNegativeTTL)
	deny(2, "bad")
	if store.reads.Load() != reads+1 {
		t.Fatal("negative cache did not expire")
	}
	for i := 2; i < admissionFailureLimit; i++ {
		deny(byte(i+1), fmt.Sprintf("bad-%d", i))
	}
	reads = store.reads.Load()
	deny(100, "new-random-token")
	deny(101, token)
	if store.reads.Load() != reads {
		t.Fatal("failure limit performed storage I/O")
	}
	now = now.Add(admissionFailureWindow)
	if !p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: giznet.PublicKey{102}, Credential: testRegistrationCredential(token)}) {
		t.Fatal("failure budget did not recover")
	}
	if len(p.negative) != 0 {
		t.Fatal("expired cache entries retained")
	}
}

func TestAdmissionConcurrentLookupsAreBoundedAndCancelable(t *testing.T) {
	p, store, _ := newAdmissionTestPolicy(t)
	entered := make(chan struct{}, admissionInFlightLimit)
	store.beforeRead = func(ctx context.Context) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan bool, admissionInFlightLimit)
	for i := range admissionInFlightLimit {
		go func() {
			done <- p.AllowPeer(ctx, giznet.PeerAdmission{PublicKey: giznet.PublicKey{byte(i + 1)}, Credential: testRegistrationCredential(fmt.Sprintf("token-%d", i))})
		}()
	}
	for range admissionInFlightLimit {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("lookups serialized behind I/O")
		}
	}
	for _, token := range []string{"token-0", "new-token"} {
		if p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: giznet.PublicKey{99}, Credential: testRegistrationCredential(token)}) {
			t.Fatal("excess lookup admitted")
		}
	}
	if store.reads.Load() != admissionInFlightLimit {
		t.Fatal("concurrency bound bypassed")
	}
	cancel()
	for range admissionInFlightLimit {
		if <-done {
			t.Fatal("canceled lookup admitted")
		}
	}
	if len(p.inFlight) != 0 || p.failures != admissionInFlightLimit {
		t.Fatal("lookup reservations leaked")
	}
}

func TestAdmissionCacheCapacityAndInFlightFailureReservation(t *testing.T) {
	p, _, _ := newAdmissionTestPolicy(t)
	now := time.Now()
	p.now = func() time.Time { return now }
	for i := range admissionCacheLimit + 1 {
		digest := sha256.Sum256(fmt.Appendf(nil, "token-%d", i))
		p.finishLookup(digest, false, true)
	}
	if len(p.negative) != admissionCacheLimit {
		t.Fatal("negative cache is unbounded")
	}
	p.windowEnd = now.Add(time.Minute)
	p.failures = admissionFailureLimit - 1
	first, second := sha256.Sum256([]byte("one")), sha256.Sum256([]byte("two"))
	if !p.beginLookup(first) || p.beginLookup(second) {
		t.Fatal("in-flight failures exceed budget")
	}
	p.finishLookup(first, true, false)
	if !p.beginLookup(second) {
		t.Fatal("successful lookup consumed failure budget")
	}
	p.finishLookup(second, false, false)
	if p.beginLookup(first) {
		t.Fatal("failure budget exceeded")
	}
}

func TestAdmissionFailsClosedOnContextAndStoreErrors(t *testing.T) {
	p, store, token := newAdmissionTestPolicy(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if p.AllowPeer(ctx, giznet.PeerAdmission{PublicKey: giznet.PublicKey{1}, Credential: testRegistrationCredential(token)}) || store.reads.Load() != 0 {
		t.Fatal("canceled admission touched store")
	}
	store.beforeRead = func(context.Context) error { return errors.New("storage unavailable") }
	for _, credential := range []*giznetpb.AdmissionCredential{nil, testRegistrationCredential(token)} {
		if p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: giznet.PublicKey{1}, Credential: credential}) {
			t.Fatal("store failure admitted")
		}
	}
}

func TestRegistrationTokenSignalingAdmission(t *testing.T) {
	for _, tc := range []struct {
		name                                       string
		open, known, valid, invalid, blocked, want bool
	}{
		{name: "default legacy unknown", open: true, want: true},
		{name: "enabled legacy unknown"},
		{name: "enabled legacy known", known: true, want: true},
		{name: "enabled valid token", valid: true, want: true},
		{name: "enabled invalid token", invalid: true},
		{name: "enabled blocked", known: true, blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, token := newAdmissionTestPolicy(t)
			serverKey, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			clientKey, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			if tc.known {
				record, err := p.server.manager.Peers.EnsureConnectedPeer(t.Context(), clientKey.Public)
				if err != nil {
					t.Fatal(err)
				}
				if tc.blocked {
					record.Status = apitypes.PeerRegistrationStatusBlocked
					if _, err := p.server.manager.Peers.SavePeer(t.Context(), record); err != nil {
						t.Fatal(err)
					}
				}
			}
			if !tc.open {
				p.server.SecurityPolicy = p
			}
			listener, err := (&gizwebrtc.ListenConfig{SecurityPolicy: (*ServerSecurityPolicy)(p.server)}).Listen(serverKey)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			httpServer := httptest.NewServer(listener.SignalingHandler())
			defer httpServer.Close()
			cfg := gizwebrtc.DialConfig{SignalingURL: httpServer.URL + gizwebrtc.SignalingPath}
			if tc.valid {
				cfg.Credential = testRegistrationCredential(token)
			}
			if tc.invalid {
				cfg.Credential = testRegistrationCredential("invalid-token")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			local, conn, err := gizwebrtc.Dial(ctx, clientKey, serverKey.Public, cfg)
			if tc.want {
				if err != nil {
					t.Fatal(err)
				}
				defer local.Close()
				defer conn.Close()
			} else if err == nil {
				local.Close()
				conn.Close()
				t.Fatal("forbidden peer connected")
			} else if !strings.Contains(err.Error(), "peer_forbidden") {
				t.Fatalf("unexpected failure: %v", err)
			}
			if !tc.known {
				if _, err := p.server.manager.Peers.LoadPeer(t.Context(), clientKey.Public); !errors.Is(err, peer.ErrPeerNotFound) {
					t.Fatalf("signaling persisted Peer: %v", err)
				}
			}
		})
	}
}

func testRegistrationCredential(value string) *giznetpb.AdmissionCredential {
	return &giznetpb.AdmissionCredential{Version: 1, Type: RegistrationTokenCredentialType, Value: value}
}

func TestAdmissionUnsupportedCredentialDoesNotReadStorage(t *testing.T) {
	p, store, token := newAdmissionTestPolicy(t)
	// The valid token would be admitted if version/type checks were bypassed.
	// Reserve the catalog's sole SQL connection. Any attempted SQL lookup would
	// wait for it and increment WaitCount, even if admission ultimately denied.
	db := p.server.manager.RuntimeProfiles.DB
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	waits := db.Stats().WaitCount
	for _, credential := range []*giznetpb.AdmissionCredential{
		{Version: 0, Type: RegistrationTokenCredentialType, Value: token},
		{Version: 2, Type: RegistrationTokenCredentialType, Value: token},
		{Version: 1, Type: "unknown", Value: token},
		{Version: 1, Type: " registration_token ", Value: token},
		{},
	} {
		if p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: giznet.PublicKey{1}, Credential: credential}) {
			t.Fatal("unsupported credential admitted")
		}
	}
	if store.reads.Load() != 0 || db.Stats().WaitCount != waits {
		t.Fatal("unsupported credential accessed storage")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if !p.AllowPeer(t.Context(), giznet.PeerAdmission{PublicKey: giznet.PublicKey{1}, Credential: testRegistrationCredential(" " + token + "\n")}) {
		t.Fatal("valid normalized token rejected")
	}
}
