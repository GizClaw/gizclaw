//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/pingcap/kvproto/pkg/errorpb"
	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	tikvkv "github.com/tikv/client-go/v2/kv"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/tikvrpc/interceptor"
)

type learnerRequestSink struct {
	tikv.Client
	send func(string, *tikvrpc.Request) (*tikvrpc.Response, error)
}

func (s learnerRequestSink) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, _ time.Duration) (*tikvrpc.Response, error) {
	if chain := interceptor.GetRPCInterceptorFromCtx(ctx); chain != nil {
		return chain.Wrap(s.send)(address, req)
	}
	return s.send(address, req)
}

func TestLearnerControlObservesRequestsWithoutChangingReadSemantics(t *testing.T) {
	for _, command := range []tikvrpc.CmdType{tikvrpc.CmdGet, tikvrpc.CmdBatchGet, tikvrpc.CmdScan} {
		for _, stale := range []bool{true, false} {
			for _, replica := range []tikvkv.ReplicaReadType{tikvkv.ReplicaReadLeader, tikvkv.ReplicaReadLearner} {
				for _, outcome := range []string{"success", "transport-error", "region-error"} {
					t.Run(fmt.Sprintf("%s/stale=%t/replica=%d/%s", command, stale, replica, outcome), func(t *testing.T) {
						var payload any = &kvrpcpb.GetRequest{Version: 97}
						if command == tikvrpc.CmdScan {
							payload = &kvrpcpb.ScanRequest{Version: 97}
						} else if command == tikvrpc.CmdBatchGet {
							payload = &kvrpcpb.BatchGetRequest{Version: 97}
						}
						original := tikvrpc.NewRequest(command, payload, kvrpcpb.Context{RegionId: 7, StaleRead: stale})
						original.ReplicaReadType = replica
						if err := tikvrpc.SetContext(original, nil, &metapb.Peer{Id: 8, StoreId: 9, Role: metapb.PeerRole_Learner}); err != nil {
							t.Fatal(err)
						}
						var regionErr *errorpb.Error
						var wantErr error
						if outcome == "region-error" {
							regionErr = &errorpb.Error{Message: "Learner unavailable"}
						} else if outcome == "transport-error" {
							wantErr = errors.New("Learner transport unavailable")
						}
						var response any = &kvrpcpb.GetResponse{RegionError: regionErr}
						if command == tikvrpc.CmdScan {
							response = &kvrpcpb.ScanResponse{RegionError: regionErr}
						} else if command == tikvrpc.CmdBatchGet {
							response = &kvrpcpb.BatchGetResponse{RegionError: regionErr}
						}
						wantResponse := &tikvrpc.Response{Resp: response}
						if wantErr != nil {
							wantResponse = nil
						}
						sent := 0
						client := learnerReadClient{Client: learnerRequestSink{send: func(address string, req *tikvrpc.Request) (*tikvrpc.Response, error) {
							sent++
							// Observe even a center destination unchanged: the production
							// guard, never this observer, owns Learner routing enforcement.
							if address != "center:20160" || req != original || req.Req != payload || req.Peer.Id != 8 || req.Peer.StoreId != 9 || req.StaleRead != stale || req.ReplicaReadType != replica {
								t.Fatalf("observer rewrote request %s: %+v", address, req)
							}
							var wire *kvrpcpb.Context
							var version uint64
							switch command {
							case tikvrpc.CmdGet:
								wire, version = req.Get().Context, req.Get().Version
							case tikvrpc.CmdBatchGet:
								wire, version = req.BatchGet().Context, req.BatchGet().Version
							case tikvrpc.CmdScan:
								wire, version = req.Scan().Context, req.Scan().Version
							}
							if wire.Peer.Id != 8 || wire.StaleRead != stale || version != 97 {
								t.Fatalf("observer changed wire context or timestamp: %+v version=%d", wire, version)
							}
							return wantResponse, wantErr
						}}}
						ctx := context.WithValue(t.Context(), learnerControlReadKey{}, true)
						response, err := client.SendRequest(ctx, "center:20160", original, time.Second)
						if response != wantResponse || err != wantErr || sent != 1 {
							t.Fatalf("observer changed transport result: response=%v err=%v sent=%d", response, err, sent)
						}
						want := learnerReadObservation{Address: "center:20160", RegionID: 7, PeerID: 8, StoreID: 9, Role: metapb.PeerRole_Learner, ReplicaRead: replica, Stale: stale, Type: command}
						if got := client.snapshot(); !reflect.DeepEqual(got, []learnerReadObservation{want}) {
							t.Fatalf("dispatch observation=%+v, want %+v", got, want)
						}
					})
				}
			}
		}
	}
}

func TestLearnerControlIgnoresUnmarkedReadsAndGuardRejections(t *testing.T) {
	guardErr := errors.New("production routing guard rejected target")
	client := learnerReadClient{Client: learnerRequestSink{send: func(address string, req *tikvrpc.Request) (*tikvrpc.Response, error) {
		if address != "center:20160" || req.StaleRead || req.ReplicaReadType != tikvkv.ReplicaReadLeader {
			t.Fatal("primary read semantics changed")
		}
		return &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{}}, nil
	}}}
	request := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{}, kvrpcpb.Context{RegionId: 12})
	if _, err := client.SendRequest(t.Context(), "center:20160", request, time.Second); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(t.Context(), learnerControlReadKey{}, true)
	ctx = interceptor.WithRPCInterceptor(ctx, interceptor.NewRPCInterceptor("production-guard", func(_ interceptor.RPCInterceptorFunc) interceptor.RPCInterceptorFunc {
		return func(_ string, _ *tikvrpc.Request) (*tikvrpc.Response, error) { return nil, guardErr }
	}))
	if _, err := client.SendRequest(ctx, "center:20160", request, time.Second); err != guardErr {
		t.Fatalf("routing rejection changed: %v", err)
	}
	if got := client.snapshot(); len(got) != 0 {
		t.Fatalf("unmarked read or rejected selection counted as a data attempt: %+v", got)
	}
}
