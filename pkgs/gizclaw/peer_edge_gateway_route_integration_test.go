package gizclaw_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func newEdgeTestKey(t *testing.T) *giznet.KeyPair {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// Observe the public response while passing the actual encrypted exchange through
// to the Go client. Never retain SDP, identity headers or credentials.
type edgeOfferResult struct {
	calls    int
	status   int
	body     string
	retry    string
	upstream string
}

func (o *edgeOfferResult) RoundTrip(r *http.Request) (*http.Response, error) {
	o.calls++
	if r.Method != http.MethodPost || r.URL.Path != gizwebrtc.SignalingPath || r.Header.Get("Authorization") != "" {
		return nil, errors.New("expected an offer without a bearer credential")
	}
	res, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	o.status, o.retry, o.upstream = res.StatusCode, res.Header.Get("Retry-After"), res.Header.Get("X-GizClaw-Gateway-Upstream")
	if res.StatusCode != http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(res.Body, 256))
		_ = res.Body.Close()
		if err != nil {
			return nil, err
		}
		o.body = string(body)
		res.Body = io.NopCloser(bytes.NewReader(body))
	}
	return res, nil
}

func requireEdgeOfferError(t *testing.T, url string, key *giznet.KeyPair, edge giznet.PublicKey, status int, code, retry string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var seen edgeOfferResult
	listener, conn, err := gizwebrtc.Dial(ctx, key, edge, gizwebrtc.DialConfig{
		SignalingURL: url, HTTPClient: &http.Client{Transport: &seen}, SecurityPolicy: allowAllEdgeGatewayPolicy{},
	})
	if conn != nil {
		_ = conn.Close()
	}
	if listener != nil {
		_ = listener.Close()
	}
	wantBody := fmt.Sprintf("{\"error\":%q}\n", code)
	t.Logf("encrypted offer: HTTP %d body=%q retry-after=%q requests=%d", seen.status, seen.body, seen.retry, seen.calls)
	if err == nil || seen.calls != 1 || seen.status != status || seen.body != wantBody || seen.retry != retry || seen.upstream != "" || !strings.Contains(err.Error(), fmt.Sprintf("%d %s: %s", status, http.StatusText(status), wantBody)) {
		t.Fatalf("expected Go dial to report HTTP %d %s, retry-after=%q", status, code, retry)
	}
}

func TestEdgeGatewayPeerRouteDenial(t *testing.T) {
	edge := newEdgeTestKey(t)
	upstream := startEdgeGatewayTestServer(t, edge.Public)
	url := "http://" + startEdgeGatewayTestIngress(t, edge, upstream, 4) + gizwebrtc.SignalingPath
	peers := upstream.server.Manager().Peers
	// A client with stale Edge discovery cannot authenticate its encrypted
	// offer. That failure must be independent of Peer state, including headers.
	staleEdgeKey := newEdgeTestKey(t).Public
	requireEdgeOfferError(t, url, newEdgeTestKey(t), staleEdgeKey, http.StatusUnauthorized, "unauthorized", "")
	for _, state := range []string{"pending", "deleted", "blocked"} {
		t.Run(state, func(t *testing.T) {
			ctx := t.Context()
			key := newEdgeTestKey(t)
			owner := key.Public.String()
			if _, err := peers.EnsureConnectedPeer(ctx, key.Public); err != nil {
				t.Fatal(err)
			}
			// Warm a real fixed assignment before transitioning this Peer.
			if _, err := upstream.server.Manager().PeerRoutes.Assign(ctx, key.Public, nil); err != nil {
				t.Fatal(err)
			}
			switch state {
			case "pending":
				// Legacy pending records can fence a still-present Peer before a
				// tombstone exists. Schedule its worker past the test lifetime.
				record, err := pendingdeletion.New(pendingdeletion.KindPeer, owner, &owner, pendingdeletion.ReasonPeerDelete,
					map[string]string{"public_key": owner}, time.Now().Add(time.Hour))
				if err != nil {
					t.Fatal(err)
				}
				if _, created, err := pendingdeletion.CreateOrGet(ctx, peers.Store, record); err != nil || !created {
					t.Fatalf("create pending locator: created=%t err=%v", created, err)
				}
				if err := peers.EnsureAvailable(ctx, key.Public); !errors.Is(err, peer.ErrPeerPendingDeletion) {
					t.Fatalf("pending fence: %v", err)
				}
			case "deleted":
				if err := peers.DeleteSelf(ctx, key.Public); err != nil {
					t.Fatal(err)
				}
				if err := peers.EnsureAvailable(ctx, key.Public); !errors.Is(err, peer.ErrPeerDeleted) {
					t.Fatalf("tombstone: %v", err)
				}
			case "blocked":
				res, err := peers.BlockPeer(ctx, adminhttp.BlockPeerRequestObject{PublicKey: owner})
				if _, ok := res.(adminhttp.BlockPeer200JSONResponse); !ok || err != nil {
					t.Fatalf("block Peer: %T %v", res, err)
				}
			}
			requireEdgeOfferError(t, url, key, staleEdgeKey, http.StatusUnauthorized, "unauthorized", "")
			requireEdgeOfferError(t, url, key, edge.Public, http.StatusForbidden, "peer_forbidden", "")
		})
	}
}

func TestEdgeGatewaySignalingControls(t *testing.T) {
	edge := newEdgeTestKey(t)
	upstream := startEdgeGatewayTestServer(t, edge.Public)
	base := "http://" + startEdgeGatewayTestIngress(t, edge, upstream, 1)
	url := base + gizwebrtc.SignalingPath
	t.Run("bootstrap_active_and_capacity", func(t *testing.T) {
		// An unassigned Peer must bootstrap on the reachable responder, reach
		// the Server through its tunnel and hold the sole real gateway slot.
		key := newEdgeTestKey(t)
		client := &gizcli.Client{KeyPair: key, DialTransport: testWebRTCDialTransport("")}
		t.Cleanup(func() { _ = client.Close() })
		startTestClient(t, client, edge.Public, url)
		if _, err := client.GetServerInfo(t.Context(), "active-peer-info"); err != nil {
			t.Fatalf("active Peer RPC: %v", err)
		}
		assignment, err := upstream.server.Manager().PeerRoutes.Resolve(t.Context(), key.Public)
		if err != nil || assignment.ServerPublicKey != upstream.server.PublicKey().String() {
			t.Fatalf("bootstrap ownership: %v", err)
		}
		t.Log("unassigned Peer completed signaling, tunnel, server.info and fixed ownership")
		requireEdgeOfferError(t, url, newEdgeTestKey(t), edge.Public, http.StatusServiceUnavailable, "gateway_over_capacity", "1")
	})
	t.Run("retired_bearer", func(t *testing.T) {
		// Issue and retire a distinct fixture's secret in the Server's real KV.
		keys := apikey.NewServer(upstream.server.APIKeyStore)
		owner := newEdgeTestKey(t).Public.String()
		created, err := keys.Create(t.Context(), owner, "retired fixture", false)
		if err != nil {
			t.Fatal(err)
		}
		if err := keys.RevokeOwner(t.Context(), owner, created.Key.Name); err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/gizclaw/v1/api-keys/self", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+created.Secret)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var failure apitypes.ErrorResponse
		if err := json.NewDecoder(res.Body).Decode(&failure); err != nil || res.StatusCode != http.StatusUnauthorized || failure.Error.Code != "INVALID_API_KEY" || res.Header.Get("Retry-After") != "" {
			t.Fatalf("retired API key self: status=%d error=%q", res.StatusCode, failure.Error.Code)
		}
		t.Log("retired bearer GET self: HTTP 401 INVALID_API_KEY without Retry-After")
	})
}

func TestEdgeGatewayUnavailableRouteStore(t *testing.T) {
	edge := newEdgeTestKey(t)
	upstream := startEdgeGatewayTestServer(t, edge.Public)
	url := "http://" + startEdgeGatewayTestIngress(t, edge, upstream, 4) + gizwebrtc.SignalingPath
	// Stop this Server's dedicated Peer KV backend while the Edge is still
	// serving offers. Both Edge role checks and route lookups use this store.
	if err := upstream.server.PeerStore.Close(); err != nil {
		t.Fatal(err)
	}
	requireEdgeOfferError(t, url, newEdgeTestKey(t), edge.Public, http.StatusServiceUnavailable, "gateway_over_capacity", "1")
}
