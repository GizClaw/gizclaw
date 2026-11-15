package tikvfixture

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/tikv/client-go/v2/tikvrpc"
)

func TestUsageMutationGateDistinguishesGuardDeleteAndHoldsAllPublishingPuts(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	native := &gateTransport{response: &tikvrpc.Response{Resp: &kvrpcpb.PrewriteResponse{}}, requests: make(chan *tikvrpc.Request, 3)}
	gate, err := StartUsageGate(native, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	rule := UsageRule{ID: "publish", Key: []string{"bindings", "pair"}, Commands: []string{"Prewrite"}, MutationOp: "Put", HoldAll: true, Stage: "request"}
	if err := HTTP(ctx, http.MethodPost, gate.URL()+"/arm", rule, nil); err != nil {
		t.Fatal(err)
	}
	guard := tikvrpc.NewRequest(tikvrpc.CmdPrewrite, &kvrpcpb.PrewriteRequest{Mutations: []*kvrpcpb.Mutation{{Op: kvrpcpb.Op_Del, Key: []byte("bindings:pair")}}})
	if response, err := gate.SendRequest(ctx, "center:20160", guard, time.Second); err != nil || response != native.response {
		t.Fatalf("actual guard delete changed: %p %v", response, err)
	}
	if <-native.requests != guard {
		t.Fatal("guard request replaced")
	}
	publish := tikvrpc.NewRequest(tikvrpc.CmdPrewrite, &kvrpcpb.PrewriteRequest{Mutations: []*kvrpcpb.Mutation{{Op: kvrpcpb.Op_Put, Key: []byte("bindings:pair"), Value: []byte("original-native-value")}}})
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := gate.SendRequest(ctx, "center:20160", publish, time.Second); done <- err }()
	}
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		state := gate.rule("publish").observation()
		if state.HeldRequests == 2 {
			if state.Hits != 2 || state.LastMutation == nil || state.LastMutation.Operation != "Put" || state.LastMutation.ValueBytes != len(publish.Prewrite().Mutations[0].Value) {
				t.Fatalf("semantic boundary evidence: %+v", state)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if len(native.requests) != 0 {
		t.Fatal("a publishing Put bypassed hold_all")
	}
	if err := HTTP(ctx, http.MethodPost, gate.URL()+"/release", map[string]string{"id": "publish"}, nil); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for range 2 {
		if <-native.requests != publish {
			t.Fatal("publishing request replaced")
		}
	}
	if gate.rule("publish").observation().HeldRequests != 0 {
		t.Fatal("released publishing requests remained held")
	}
}
