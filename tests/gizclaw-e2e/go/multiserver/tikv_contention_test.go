//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"
)

type contentionResult struct {
	client *gizcli.Client
	err    error
}

type contentionEvidence struct {
	Peer    string                         `json:"peer"`
	Winner  string                         `json:"winner"`
	StartTS uint64                         `json:"abandoned_start_ts"`
	Gates   [2]tikvfixture.ClientGateState `json:"gates"`
}

// The observer only pauses and forwards original RPCs. Both real Servers must
// reach the same production Peer-creation transaction before either can win.
func TestConcurrentServersResolveResidualSecondary(t *testing.T) {
	if os.Getenv("GIZCLAW_E2E_TIKV_CONTENTION_ONLY") != "1" {
		t.Skip("requires the isolated Docker TiKV contention runner")
	}
	servers := [2]gizcli.ServerInfoMetadata{
		fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A")),
		fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B")),
	}
	gates := [2]string{requiredEnv(t, "GIZCLAW_E2E_TIKV_GATE_A"), requiredEnv(t, "GIZCLAW_E2E_TIKV_GATE_B")}
	states := [2]string{requiredEnv(t, "GIZCLAW_E2E_SERVER_A_STATE"), requiredEnv(t, "GIZCLAW_E2E_SERVER_B_STATE")}
	client := openSharedTiKV(t)
	root, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	peers := kv.Prefixed(root, kv.Key{"peers", "records"})
	routes := &peerroute.Server{Store: kv.Prefixed(root, kv.Key{"peers", "routes"})}
	var evidence []contentionEvidence
	for _, winner := range []int{0, 1} {
		if !t.Run(fmt.Sprintf("server-%c-wins", 'a'+winner), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			peer, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			key := []byte("gizclaw-kv\x00\x01\x01peers:records:by-pubkey:" + peer.Public.String())
			startTS, err := tikvfixture.Timestamp(ctx, client)
			if err != nil {
				t.Fatal(err)
			}
			var workers sync.WaitGroup
			t.Cleanup(func() {
				cancel()
				cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				for _, gate := range gates {
					if err := tikvfixture.HTTP(cleanup, http.MethodPost, gate+"/reset", nil, nil); err != nil {
						t.Errorf("release fixture: %v", err)
					}
				}
				workers.Wait()
			})
			results := [2]chan contentionResult{make(chan contentionResult, 1), make(chan contentionResult, 1)}
			for i, gate := range gates {
				gateRequest(t, ctx, gate, "/arm", tikvfixture.ClientGateArm{Key: key, StartTS: startTS})
				workers.Go(func() {
					connected, err := connect(peer, servers[i], servers[i].PublicKey)
					if err != nil {
						results[i] <- contentionResult{err: err}
						return
					}
					served := make(chan struct{})
					go func() { defer close(served); _ = connected.Serve() }()
					defer func() { _ = connected.Close(); <-served }()
					pingCtx, stop := context.WithTimeout(ctx, 10*time.Second)
					_, err = connected.Ping(pingCtx, "concurrent-peer-creation")
					stop()
					results[i] <- contentionResult{client: connected, err: err}
					<-ctx.Done()
				})
			}
			for _, gate := range gates {
				waitClientGate(t, ctx, gate, func(s tikvfixture.ClientGateState) bool { return s.Reads > 0 })
			}
			// Earlier point reads saw absence. Install the abandoned transaction
			// only now, so both production mutation-prefetch snapshots hit it.
			if err := tikvfixture.LeaveSecondaryLock(ctx, client, append([]byte("contention-primary/"), key...), key, startTS); err != nil {
				t.Fatal(err)
			}
			for _, gate := range gates {
				gateRequest(t, ctx, gate, "/release/reads", nil)
			}
			for _, gate := range gates {
				waitClientGate(t, ctx, gate, func(s tikvfixture.ClientGateState) bool { return s.Resolves > 0 })
			}
			for _, gate := range gates {
				gateRequest(t, ctx, gate, "/release/resolves", nil)
			}
			for _, gate := range gates {
				waitClientGate(t, ctx, gate, func(s tikvfixture.ClientGateState) bool { return s.Resolved > 0 && s.Prewrites > 0 })
			}
			gateRequest(t, ctx, gates[winner], "/release/prewrites", nil)
			accepted := awaitContender(t, ctx, results[winner])
			if accepted.err != nil {
				t.Fatalf("winning Server admission: %v", accepted.err)
			}
			loser := 1 - winner
			gateRequest(t, ctx, gates[loser], "/release/prewrites", nil)
			if rejected := awaitContender(t, ctx, results[loser]); rejected.err == nil {
				t.Fatal("both Servers admitted the same Peer")
			}
			report := contentionEvidence{Peer: peer.Public.String(), Winner: servers[winner].PublicKey.String(), StartTS: startTS}
			for i, gate := range gates {
				report.Gates[i] = waitClientGate(t, ctx, gate, func(s tikvfixture.ClientGateState) bool { return s.InFlight == 0 })
				if err := tikvfixture.CheckClientGate(report.Gates[i]); err != nil {
					t.Fatal(err)
				}
				if report.Gates[i].PrewriteStartTS <= startTS {
					t.Fatal("competing transaction did not follow the abandoned transaction")
				}
			}
			if report.Gates[loser].WriteConflicts == 0 {
				t.Fatal("losing Server did not encounter a real TiKV write conflict")
			}
			if err := tikvfixture.CheckSecondaryRemoved(ctx, client, key, startTS); err != nil {
				t.Fatal(err)
			}
			stored, err := peers.Get(ctx, kv.Key{"by-pubkey", peer.Public.String()})
			var record apitypes.Peer
			if err != nil || json.Unmarshal(stored, &record) != nil || record.PublicKey != peer.Public.String() || record.Status != apitypes.PeerRegistrationStatusActive {
				t.Fatalf("Peer record after conflict: %q, %v", stored, err)
			}
			assignment, err := routes.Lookup(ctx, peer.Public)
			if err != nil || assignment.ServerPublicKey != servers[winner].PublicKey.String() || assignment.Version != 1 {
				t.Fatalf("fixed assignment after conflict: %+v, %v", assignment, err)
			}
			if err := accepted.client.SendBatteryTelemetry(73, false); err != nil {
				t.Fatal(err)
			}
			waitPeerRunStatus(t, states[winner], peer.Public, 73)
			assertPeerRunAbsent(t, states[loser], peer.Public)
			if _, err := accepted.client.Ping(ctx, "same-connection-after-conflict"); err != nil {
				t.Fatalf("winning connection after cleanup: %v", err)
			}
			target, err := tikvfixture.Timestamp(ctx, client)
			if err != nil {
				t.Fatal(err)
			}
			if err := tikvfixture.WaitHealthySafeTS(ctx, client, requiredEnv(t, "GIZCLAW_E2E_TIKV_PD"), target); err != nil {
				t.Fatal(err)
			}
			evidence = append(evidence, report)
			t.Logf("real Server contention, secondary rollback and safe-ts passed: %+v", report)
		}) {
			break
		}
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requiredEnv(t, "GIZCLAW_E2E_CONTENTION_REPORT"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func gateRequest(t *testing.T, ctx context.Context, gate, path string, body any) {
	t.Helper()
	if err := tikvfixture.HTTP(ctx, http.MethodPost, gate+path, body, nil); err != nil {
		t.Fatal(err)
	}
}

func waitClientGate(t *testing.T, ctx context.Context, gate string, ready func(tikvfixture.ClientGateState) bool) tikvfixture.ClientGateState {
	t.Helper()
	deadline, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var state tikvfixture.ClientGateState
	for {
		if err := tikvfixture.HTTP(deadline, http.MethodGet, gate+"/status", nil, &state); err != nil {
			t.Fatal(err)
		}
		if ready(state) {
			return state
		}
		select {
		case <-deadline.Done():
			t.Fatalf("TiKV rendezvous %s: %+v: %v", gate, state, deadline.Err())
		case <-ticker.C:
		}
	}
}

func awaitContender(t *testing.T, ctx context.Context, result <-chan contentionResult) contentionResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		return contentionResult{}
	}
}
