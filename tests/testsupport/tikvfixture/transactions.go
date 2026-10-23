package tikvfixture

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/tikv/client-go/v2/config/retry"
	"github.com/tikv/client-go/v2/oracle"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
)

// NativeRequest routes an unmodified transactional request through real TiKV.
func NativeRequest(ctx context.Context, client *tikv.KVStore, key []byte, req *tikvrpc.Request) (*tikvrpc.Response, error) {
	bo := retry.NewBackofferWithVars(ctx, 5000, nil)
	location, err := client.GetRegionCache().LocateKey(bo, key)
	if err != nil {
		return nil, err
	}
	resp, err := client.SendReq(bo, req, location.Region, 5*time.Second)
	if err != nil {
		return nil, err
	}
	if regionErr, err := resp.GetRegionError(); err != nil || regionErr != nil {
		return nil, fmt.Errorf("%s region error: %v, %v", req.Type, regionErr, err)
	}
	return resp, nil
}

// Timestamp gets a fresh global timestamp under the caller's deadline.
func Timestamp(ctx context.Context, client *tikv.KVStore) (uint64, error) {
	return client.GetTimestampWithRetry(retry.NewBackofferWithVars(ctx, 5000, nil), oracle.GlobalTxnScope)
}

// LeaveSecondaryLock prewrites two real keys, rolls back only the primary and
// verifies that a raw Get still reports the abandoned secondary lock. Reserve
// startTS before the competing snapshots, then install their lock while their
// native BatchGet requests are paused at the fixture gate.
func LeaveSecondaryLock(ctx context.Context, client *tikv.KVStore, primary, secondary []byte, startTS uint64) error {
	ts := startTS
	for _, key := range [][]byte{primary, secondary} {
		resp, err := NativeRequest(ctx, client, key, tikvrpc.NewRequest(tikvrpc.CmdPrewrite, &kvrpcpb.PrewriteRequest{
			Mutations:   []*kvrpcpb.Mutation{{Op: kvrpcpb.Op_Put, Key: key, Value: []byte("abandoned-e2e-write")}},
			PrimaryLock: primary, StartVersion: ts, LockTtl: 1, TxnSize: 2,
		}))
		if err != nil {
			return err
		}
		if result := resp.Resp.(*kvrpcpb.PrewriteResponse); len(result.Errors) != 0 {
			return fmt.Errorf("prewrite abandoned transaction: %s", result)
		}
	}
	resp, err := NativeRequest(ctx, client, primary, tikvrpc.NewRequest(tikvrpc.CmdBatchRollback, &kvrpcpb.BatchRollbackRequest{Keys: [][]byte{primary}, StartVersion: ts}))
	if err != nil {
		return err
	}
	if result := resp.Resp.(*kvrpcpb.BatchRollbackResponse); result.Error != nil {
		return fmt.Errorf("rollback primary: %s", result)
	}
	readTS, err := Timestamp(ctx, client)
	if err != nil {
		return err
	}
	resp, err = NativeRequest(ctx, client, secondary, tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Key: secondary, Version: readTS}))
	if err != nil {
		return err
	}
	lock := resp.Resp.(*kvrpcpb.GetResponse).GetError().GetLocked()
	if lock == nil || lock.LockVersion != ts || !bytes.Equal(lock.PrimaryLock, primary) {
		return fmt.Errorf("expected residual secondary lock: %s", resp.Resp)
	}
	return nil
}

// CheckSecondaryRemoved scans the exact key after observing completed cleanup.
func CheckSecondaryRemoved(ctx context.Context, client *tikv.KVStore, key []byte, ts uint64) error {
	end := append(bytes.Clone(key), 0)
	resp, err := NativeRequest(ctx, client, key, tikvrpc.NewRequest(tikvrpc.CmdScanLock, &kvrpcpb.ScanLockRequest{StartKey: key, EndKey: end, MaxVersion: ts, Limit: 1}))
	if err != nil {
		return err
	}
	if result := resp.Resp.(*kvrpcpb.ScanLockResponse); result.Error != nil || len(result.Locks) != 0 {
		return fmt.Errorf("secondary lock remains: %s", result)
	}
	return nil
}

// WaitHealthySafeTS requires every fixture store to remain Up and advance its
// native safe timestamp beyond target; this catches asynchronous observer exits.
func WaitHealthySafeTS(ctx context.Context, client *tikv.KVStore, pd string, target uint64) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var stores struct {
			Stores []struct {
				Store struct {
					Address   string `json:"address"`
					StateName string `json:"state_name"`
				} `json:"store"`
			} `json:"stores"`
		}
		if err := HTTP(ctx, http.MethodGet, "http://"+pd+"/pd/api/v1/stores", nil, &stores); err != nil {
			return err
		}
		if len(stores.Stores) != 2 {
			return fmt.Errorf("expected center voter and Learner, got %d stores", len(stores.Stores))
		}
		ready := true
		for _, entry := range stores.Stores {
			if entry.Store.StateName != "Up" {
				return fmt.Errorf("store %s is %s", entry.Store.Address, entry.Store.StateName)
			}
			resp, err := client.GetTiKVClient().SendRequest(ctx, entry.Store.Address, tikvrpc.NewRequest(tikvrpc.CmdStoreSafeTS, &kvrpcpb.StoreSafeTSRequest{KeyRange: &kvrpcpb.KeyRange{}}), time.Second)
			if err != nil {
				return fmt.Errorf("store %s after cleanup: %w", entry.Store.Address, err)
			}
			ready = ready && resp.Resp.(*kvrpcpb.StoreSafeTSResponse).SafeTs >= target
		}
		if ready {
			_, placed, err := ReadTopology(ctx, pd)
			if err != nil || !placed {
				return fmt.Errorf("center/Learner placement changed: ready=%v error=%v", placed, err)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
