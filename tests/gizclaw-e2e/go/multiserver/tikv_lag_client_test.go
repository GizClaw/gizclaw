//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"
	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
)

type learnerRequestSink struct {
	tikv.Client
	send func(string, *tikvrpc.Request) (*tikvrpc.Response, error)
}

func (s learnerRequestSink) SendRequest(_ context.Context, address string, req *tikvrpc.Request, _ time.Duration) (*tikvrpc.Response, error) {
	return s.send(address, req)
}

func TestLearnerControlPinsPDPeerWithoutChangingSDKRequest(t *testing.T) {
	for _, command := range []tikvrpc.CmdType{tikvrpc.CmdGet, tikvrpc.CmdScan} {
		for _, stale := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/stale=%t", command, stale), func(t *testing.T) {
				var payload any = &kvrpcpb.GetRequest{Version: 97}
				if command == tikvrpc.CmdScan {
					payload = &kvrpcpb.ScanRequest{Version: 97}
				}
				original := tikvrpc.NewRequest(command, payload, kvrpcpb.Context{RegionId: 7, StaleRead: stale})
				if err := tikvrpc.SetContext(original, nil, &metapb.Peer{Id: 8, StoreId: 9}); err != nil {
					t.Fatal(err)
				}
				sent := 0
				client := learnerReadClient{
					reads: make(chan string, 1), address: "learner:20160",
					peers: map[uint64]tikvfixture.RegionalPeer{7: {ID: 10, StoreID: 11}},
					Client: learnerRequestSink{send: func(address string, req *tikvrpc.Request) (*tikvrpc.Response, error) {
						sent++
						if address != "learner:20160" || req.RegionId != 7 || req.Peer.Id != 10 || req.Peer.StoreId != 11 || !req.StaleRead {
							t.Fatalf("unexpected diagnostic destination %s: %+v", address, req.Context)
						}
						if command == tikvrpc.CmdGet {
							if req.Get().Context.Peer.Id != 10 || req.Get().Version != 97 {
								t.Fatal("Get wire context or timestamp was lost")
							}
							return &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{}}, nil
						}
						if req.Scan().Context.Peer.Id != 10 || req.Scan().Version != 97 {
							t.Fatal("Scan wire context or timestamp was lost")
						}
						return &tikvrpc.Response{Resp: &kvrpcpb.ScanResponse{}}, nil
					}},
				}
				ctx := context.WithValue(t.Context(), learnerControlReadKey{}, true)
				if _, err := client.SendRequest(ctx, "center:20160", original, time.Second); err != nil {
					t.Fatal(err)
				}
				if sent != 1 || original.Peer.Id != 8 || original.StaleRead != stale || len(client.reads) != 1 {
					t.Fatal("original SDK context changed or the regional read was not observed")
				}
				if command == tikvrpc.CmdGet && original.Get().Context.Peer.Id != 8 {
					t.Fatal("original Get wire context changed")
				}
				if command == tikvrpc.CmdScan && original.Scan().Context.Peer.Id != 8 {
					t.Fatal("original Scan wire context changed")
				}
			})
		}
	}
}

func TestLearnerControlRejectsUnknownRegionAndLeavesStrongReadsAlone(t *testing.T) {
	client := learnerReadClient{reads: make(chan string, 1)}
	request := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{}, kvrpcpb.Context{RegionId: 12, StaleRead: true})
	ctx := context.WithValue(t.Context(), learnerControlReadKey{}, true)
	if _, err := client.SendRequest(ctx, "center:20160", request, time.Second); err == nil {
		t.Fatal("unknown regional peer reached the transport")
	}
	client.Client = learnerRequestSink{send: func(address string, req *tikvrpc.Request) (*tikvrpc.Response, error) {
		if address != "center:20160" || req.StaleRead {
			t.Fatal("strong control read was rerouted")
		}
		return &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{}}, nil
	}}
	request = tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{}, kvrpcpb.Context{RegionId: 12})
	if _, err := client.SendRequest(t.Context(), "center:20160", request, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(client.reads) != 0 {
		t.Fatal("strong read counted as a Learner control")
	}
}
