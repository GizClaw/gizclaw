package tikvfixture

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

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
			usageRecordRead(ctx, req, &tikvrpc.Response{Resp: test.response}, nil)
			if receipt.learner.Load() != test.want {
				t.Fatalf("actual response readiness = %t, want %t", receipt.learner.Load(), test.want)
			}
		})
	}
}
