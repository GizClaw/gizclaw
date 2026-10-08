package peer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestAdminRefreshReportsIdentifierLimits(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{name: "peer", err: ErrPeerIdentifierLimit, code: "DEVICE_IDENTIFIER_LIMIT_REACHED"},
		{name: "reverse index", err: ErrIdentifierIndexPeerLimit, code: "DEVICE_IDENTIFIER_INDEX_FULL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &Server{PeerManager: stubPeerManager{refreshOnline: true, refreshErr: tc.err}}
			response, err := server.RefreshPeer(t.Context(), adminhttp.RefreshPeerRequestObject{PublicKey: giznet.PublicKey{1}.String()})
			conflict, ok := response.(adminhttp.RefreshPeer409JSONResponse)
			if err != nil || !ok || conflict.Error.Code != tc.code {
				t.Fatalf("refresh response=%+v err=%v", response, err)
			}
		})
	}
}

func identifierLimitPeer(key byte, sn string, imeis ...apitypes.PeerIMEI) apitypes.Peer {
	return apitypes.Peer{PublicKey: giznet.PublicKey{key}.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive,
		Device: apitypes.DeviceInfo{Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn, Imeis: &imeis}}}
}

func TestPeerSNAndIMEICombinedLimit(t *testing.T) {
	server := &Server{Store: kv.NewMemory(nil)}
	imeis := make([]apitypes.PeerIMEI, PeerIdentifierLimit)
	for i := range imeis {
		imeis[i] = apitypes.PeerIMEI{Tac: "12345678", Serial: fmt.Sprint(i)}
	}
	key := giznet.PublicKey{1}
	allowed := identifierLimitPeer(1, "serial", imeis[:PeerIdentifierLimit-1]...)
	if _, err := server.SavePeer(t.Context(), allowed); err != nil {
		t.Fatalf("SN + 9 IMEI: %v", err)
	}
	before, err := server.Store.Get(t.Context(), peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.SaveRefreshedDeviceFields(t.Context(), key, identifierLimitPeer(1, "serial", imeis...).Device, []string{"device.identifiers.imeis"}); !errors.Is(err, ErrPeerIdentifierLimit) {
		t.Fatalf("SN + 10 IMEI: %v", err)
	}
	after, err := server.Store.Get(t.Context(), peerKey(key.String()))
	if err != nil || string(after) != string(before) {
		t.Fatalf("overflow changed record: %v", err)
	}
	if present, err := server.Store.HasMember(t.Context(), imeiPrefix(imeis[9].Tac, imeis[9].Serial), key.String()); err != nil || present {
		t.Fatalf("overflow added index: %v, %v", present, err)
	}
	allowed.Device.Identifiers.Sn = nil
	allowed.Device.Identifiers.Imeis = &imeis
	if _, err := server.SavePeer(t.Context(), allowed); err != nil {
		t.Fatalf("10 IMEI without SN: %v", err)
	}
	duplicates := append(append([]apitypes.PeerIMEI(nil), imeis[:9]...), imeis[0], apitypes.PeerIMEI{})
	if _, err := server.SavePeer(t.Context(), identifierLimitPeer(2, "other", duplicates...)); err != nil {
		t.Fatalf("duplicate or incomplete IMEI must not create extra indexes: %v", err)
	}
}

func TestIdentifierIndexLimitIsAtomicAndReleasesCapacity(t *testing.T) {
	store := kv.NewMemory(nil)
	server := &Server{Store: store}
	imei := apitypes.PeerIMEI{Tac: "12345678", Serial: "shared"}
	for i := 1; i <= IdentifierIndexPeerLimit; i++ {
		if _, err := server.SavePeer(t.Context(), identifierLimitPeer(byte(i), "shared", imei)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := server.SavePeer(t.Context(), identifierLimitPeer(1, "shared", imei)); err != nil {
		t.Fatalf("existing member at capacity: %v", err)
	}
	key := giznet.PublicKey{11}
	old := identifierLimitPeer(11, "old", apitypes.PeerIMEI{Tac: "87654321", Serial: "old"})
	if _, err := server.SavePeer(t.Context(), old); err != nil {
		t.Fatal(err)
	}
	before, err := store.Get(t.Context(), peerKey(key.String()))
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []apitypes.Peer{identifierLimitPeer(11, "shared"), identifierLimitPeer(11, "new", imei)} {
		if _, err := server.SavePeer(t.Context(), next); !errors.Is(err, ErrIdentifierIndexPeerLimit) {
			t.Fatalf("full index: %v", err)
		}
		after, err := store.Get(t.Context(), peerKey(key.String()))
		if err != nil || string(after) != string(before) {
			t.Fatalf("full index changed source record: %v", err)
		}
		if present, err := store.HasMember(t.Context(), snPrefix("old"), key.String()); err != nil || !present {
			t.Fatalf("full index removed old membership: %v, %v", present, err)
		}
		if present, err := store.HasMember(t.Context(), snPrefix("new"), key.String()); err != nil || present {
			t.Fatalf("full index left partial new membership: %v, %v", present, err)
		}
	}
	if _, err := server.SavePeer(t.Context(), identifierLimitPeer(1, "replacement")); err != nil {
		t.Fatal(err)
	}
	if _, err := server.SavePeer(t.Context(), identifierLimitPeer(11, "shared", imei)); err != nil {
		t.Fatalf("released SN and IMEI capacity: %v", err)
	}
}

type concurrentIdentifierLimitStore struct {
	kv.Store
	entered chan struct{}
	release chan struct{}
}

func (s concurrentIdentifierLimitStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.entered <- struct{}{}
	select {
	case <-s.release:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestIdentifierIndexLimitAcrossConcurrentServers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	store := kv.Prefixed(kv.NewMemory(nil), kv.Key{"peer-records"})
	seed := &Server{Store: store}
	imei := apitypes.PeerIMEI{Tac: "12345678", Serial: "shared"}
	for i := 1; i < IdentifierIndexPeerLimit; i++ {
		if _, err := seed.SavePeer(t.Context(), identifierLimitPeer(byte(i), "shared", imei)); err != nil {
			t.Fatal(err)
		}
	}
	hook := concurrentIdentifierLimitStore{Store: store, entered: make(chan struct{}, 2), release: make(chan struct{})}
	results := make(chan error, 2)
	for i := 10; i <= 11; i++ {
		server := &Server{Store: hook}
		go func() {
			_, err := server.SavePeer(ctx, identifierLimitPeer(byte(i), "shared", imei))
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-hook.entered:
		case <-ctx.Done():
			t.Fatal("both Servers did not reach the shared capacity check")
		}
	}
	close(hook.release)
	winners, rejected := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrIdentifierIndexPeerLimit):
			rejected++
		default:
			t.Fatalf("SavePeer: %v", err)
		}
	}
	if winners != 1 || rejected != 1 {
		t.Fatalf("winners=%d rejected=%d", winners, rejected)
	}
	for _, index := range []kv.Key{snPrefix("shared"), imeiPrefix(imei.Tac, imei.Serial)} {
		members, err := store.ListMembers(t.Context(), index)
		if err != nil || len(members) != IdentifierIndexPeerLimit {
			t.Fatalf("index count=%d, %v", len(members), err)
		}
	}
}
