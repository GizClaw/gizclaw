package kv

import (
	"context"
	"errors"
	"slices"

	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

type tiKVReadCancelKey struct{}
type tiKVReadStoreKey struct{}

// The pinned SDK's async API does not execute snapshot interceptors before
// sending a request. This shared, stateless decorator checks only contexts
// created by regional read views; primary, write and unrelated traffic passes
// through unchanged. It owns no client, goroutine or connection.
type tiKVReadPolicyClient struct{ tikv.Client }

func (c *tiKVReadPolicyClient) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	if store, ok := ctx.Value(tiKVReadStoreKey{}).(*TiKV); ok {
		if err := store.guardRegionalRead(ctx, req); err != nil {
			callback.Schedule(nil, err)
			return
		}
	}
	c.Client.SendRequestAsync(ctx, address, req, callback)
}

func (c *tiKVReadPolicyClient) CloseAddrVer(address string, version uint64) error {
	if client, ok := c.Client.(interface{ CloseAddrVer(string, uint64) error }); ok {
		return client.CloseAddrVer(address, version)
	}
	return c.Client.CloseAddr(address)
}

func (s *TiKV) installAsyncReadPolicy() {
	client := s.client.GetTiKVClient()
	if client == nil {
		return
	}
	if _, ok := client.(*tiKVReadPolicyClient); !ok {
		s.client.SetTiKVClient(&tiKVReadPolicyClient{Client: client})
	}
}

func (s *TiKV) guardRegionalRead(ctx context.Context, req *tikvrpc.Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.Type != tikvrpc.CmdGet && req.Type != tikvrpc.CmdBatchGet && req.Type != tikvrpc.CmdScan {
		return nil
	}
	if s.isRegionalReadTarget(req) {
		return nil
	}
	if cancel, ok := ctx.Value(tiKVReadCancelKey{}).(context.CancelCauseFunc); ok {
		cancel(errTiKVRegionalReadTarget)
		// The SDK recognizes this owned cancellation as terminal and does not
		// mark a healthy nonmatching target as a failed network endpoint.
		return ctx.Err()
	}
	return errTiKVRegionalReadTarget
}

var errTiKVRegionalReadTarget = errors.New("kv: no available Learner matching configured read labels")

func tiKVMatchesLearner(role metapb.PeerRole, actual, required []*metapb.StoreLabel) bool {
	if role != metapb.PeerRole_Learner {
		return false
	}
	for _, wanted := range required {
		if !slices.ContainsFunc(actual, func(label *metapb.StoreLabel) bool {
			return label.GetKey() == wanted.GetKey() && label.GetValue() == wanted.GetValue()
		}) {
			return false
		}
	}
	return true
}

func (s *TiKV) isRegionalReadTarget(req *tikvrpc.Request) bool {
	if req.Peer.GetRole() != metapb.PeerRole_Learner || s.client.GetRegionCache() == nil {
		return false
	}
	for _, store := range s.client.GetRegionCache().GetAllStores() {
		if store.StoreID() == req.Peer.GetStoreId() {
			return !store.IsTiFlash() && tiKVMatchesLearner(req.Peer.GetRole(), store.GetLabels(), s.labels)
		}
	}
	return false
}
