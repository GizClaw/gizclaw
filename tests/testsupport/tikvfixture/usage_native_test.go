package tikvfixture

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/pingcap/kvproto/pkg/errorpb"
	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

func TestUsageNativeBudgetBoundsPauseWithBackgroundContext(t *testing.T) {
	for _, asynchronous := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync-timeout-argument", true: "async-native-execution-budget"}[asynchronous], func(t *testing.T) {
			native := &gateTransport{requests: make(chan *tikvrpc.Request, 1)}
			gate, err := StartUsageGate(native, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Close()
			if err := HTTP(t.Context(), http.MethodPost, gate.URL()+"/block-center", map[string]bool{"blocked": true}, nil); err != nil {
				t.Fatal(err)
			}
			request := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Key: []byte("peers:records:by-pubkey:public-subject"), Version: 97}, kvrpcpb.Context{MaxExecutionDurationMs: 50})
			var returned error
			started := time.Now()
			if asynchronous {
				done := make(chan error, 1)
				gate.SendRequestAsync(context.Background(), "center:20160", request, async.NewCallback(nil, func(_ *tikvrpc.Response, err error) { done <- err }))
				select {
				case returned = <-done:
				case <-time.After(time.Second):
					t.Fatal("async native budget ignored")
				}
			} else {
				_, returned = gate.SendRequest(context.Background(), "center:20160", request, 50*time.Millisecond)
			}
			if !errors.Is(returned, context.DeadlineExceeded) || time.Since(started) > time.Second {
				t.Fatalf("native budget did not end pause: %v", returned)
			}
			if len(native.requests) != 0 {
				t.Fatal("paused request reached center")
			}
			gate.mu.Lock()
			requests := append([]usageBlockedRequest(nil), gate.blockedRequests...)
			gate.mu.Unlock()
			if len(requests) != 1 || requests[0].Version != 97 || requests[0].Command != "Get" || requests[0].KeySHA256 == "" {
				t.Fatalf("bounded metadata missing: %+v", requests)
			}
		})
	}
}

func TestUsageReadinessRequiresSuccessfulActualLearnerResponse(t *testing.T) {
	for _, test := range []struct {
		name     string
		role     metapb.PeerRole
		response *kvrpcpb.GetResponse
		want     bool
	}{
		{name: "center-fallback-is-not-Learner-readiness", response: &kvrpcpb.GetResponse{Value: []byte{1}}, role: metapb.PeerRole_Voter},
		{name: "Learner-positive", response: &kvrpcpb.GetResponse{Value: []byte{1}}, role: metapb.PeerRole_Learner, want: true},
		{name: "Learner-negative", response: &kvrpcpb.GetResponse{}, role: metapb.PeerRole_Learner, want: true},
		{name: "Learner-error-is-not-readiness", response: &kvrpcpb.GetResponse{RegionError: &errorpb.Error{Message: "not ready"}}, role: metapb.PeerRole_Learner},
	} {
		t.Run(test.name, func(t *testing.T) {
			receipt := new(usageReadReceipt)
			ctx := context.WithValue(t.Context(), usageReadReceiptKey{}, receipt)
			req := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{}, kvrpcpb.Context{Peer: &metapb.Peer{Role: test.role}})
			gate := new(UsageGate)
			gate.recordNativeRead(ctx, req, &tikvrpc.Response{Resp: test.response}, nil)
			if receipt.learner.Load() != test.want {
				t.Fatalf("actual response readiness = %t, want %t", receipt.learner.Load(), test.want)
			}
		})
	}
}

// ListMembers first reads collection metadata through the native gate, like the
// TiKV collection reader. The actual gate records readiness from its response;
// this adapter never creates or sets a learner receipt itself.
type usageReceiptMembersStore struct {
	kv.Store
	gate    *UsageGate
	request *tikvrpc.Request
}

func (s *usageReceiptMembersStore) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	if _, err := s.gate.SendRequest(ctx, "native:20160", s.request, time.Second); err != nil {
		return nil, err
	}
	return s.Store.ListMembers(ctx, key)
}

func TestUsageMembersReadinessUsesActualNativeReceipt(t *testing.T) {
	for _, test := range []struct {
		name string
		role metapb.PeerRole
		wait string
		want bool
	}{
		{name: "learner", role: metapb.PeerRole_Learner, want: true},
		{name: "learner-with-wait", role: metapb.PeerRole_Learner, wait: "25ms", want: true},
		{name: "center-is-not-readiness", role: metapb.PeerRole_Voter},
		{name: "center-wait-is-bounded", role: metapb.PeerRole_Voter, wait: "25ms"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			store := kv.NewMemory(nil)
			defer store.Close()
			if err := store.AddMembers(ctx, kv.Key{"groups", "a%3Ab"}, "second", "first"); err != nil {
				t.Fatal(err)
			}
			native := &gateTransport{response: &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{Value: []byte("actual metadata")}}, requests: make(chan *tikvrpc.Request, 2)}
			gate, err := StartUsageGate(native, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Close()
			request := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Key: []byte("groups:a%3Ab"), Version: 97}, kvrpcpb.Context{Peer: &metapb.Peer{Role: test.role}})
			services := &UsageServices{Stores: map[string]kv.Store{"group": &usageReceiptMembersStore{Store: store, gate: gate, request: request}}}
			value, err := services.read(ctx, UsageAction{Operation: "members", Store: "group", Key: []string{"groups", "a:b"}, RequireLearner: true, Wait: test.wait})
			if test.want {
				if err != nil {
					t.Fatal(err)
				}
				result := value.(map[string]any)
				if result["native_learner"] != true || result["found"] != true || !slices.Equal(result["data"].([]string), []string{"first", "second"}) {
					t.Fatalf("actual membership/receipt lost: %#v", result)
				}
			} else if err == nil {
				t.Fatal("center response became learner readiness")
			} else if test.wait != "" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("membership wait deadline lost: %v", err)
			}
			if len(native.requests) != 1 || <-native.requests != request || request.Get().Version != 97 {
				t.Fatal("membership observation replaced the native request")
			}
		})
	}
}

func TestUsageDedicatedScenarioDocuments(t *testing.T) {
	paths, err := filepath.Glob("../../gizclaw-e2e/giztest/tikv.usage.*.giztest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 7 {
		t.Fatalf("dedicated usage suite contains %d documents, want 7", len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := giztest.LoadDocument(path, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}
