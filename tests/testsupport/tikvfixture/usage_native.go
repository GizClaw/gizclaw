package tikvfixture

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikvrpc"
)

type usageMutationMetadata struct {
	Operation  string `json:"operation"`
	ValueBytes int    `json:"value_bytes"`
}

type usageResponseMetadata struct {
	Type         string `json:"type"`
	ValueBytes   int    `json:"value_bytes"`
	NotFound     bool   `json:"not_found"`
	RegionError  bool   `json:"region_error"`
	DataNotReady bool   `json:"data_not_ready"`
	KeyError     bool   `json:"key_error"`
}

func usageResponseInfo(response *tikvrpc.Response) usageResponseMetadata {
	if response == nil {
		return usageResponseMetadata{Type: "nil"}
	}
	info := usageResponseMetadata{Type: fmt.Sprintf("%T", response.Resp)}
	if value, ok := response.Resp.(*kvrpcpb.GetResponse); ok && value != nil {
		info.ValueBytes, info.NotFound = len(value.Value), value.NotFound
		info.RegionError, info.KeyError = value.RegionError != nil, value.Error != nil
		info.DataNotReady = value.RegionError.GetDataIsNotReady() != nil
	}
	return info
}

type usageReadReceiptKey struct{}
type usageReadReceipt struct{ learner atomic.Bool }

func usageRecordRead(ctx context.Context, req *tikvrpc.Request, response *tikvrpc.Response, err error) {
	receipt, _ := ctx.Value(usageReadReceiptKey{}).(*usageReadReceipt)
	if receipt == nil || err != nil || req.Type != tikvrpc.CmdGet || req.Peer.GetRole() != metapb.PeerRole_Learner || response == nil {
		return
	}
	value, ok := response.Resp.(*kvrpcpb.GetResponse)
	if ok && value != nil && value.RegionError == nil && value.Error == nil {
		receipt.learner.Store(true)
	}
}

type usageBlockedRequest struct {
	CapturedAt      time.Time `json:"captured_at"`
	Address         string    `json:"address"`
	Command         string    `json:"command"`
	PeerRole        string    `json:"peer_role"`
	ReplicaReadType string    `json:"replica_read_type"`
	ReplicaRead     bool      `json:"replica_read"`
	StaleRead       bool      `json:"stale_read"`
	RegionID        uint64    `json:"region_id"`
	Version         uint64    `json:"version"`
	KeyClass        string    `json:"key_class"`
	KeySHA256       string    `json:"key_sha256"`
	PeerPublicKey   string    `json:"peer_public_key,omitempty"`
}

func (g *UsageGate) recordBlocked(address string, req *tikvrpc.Request, block <-chan struct{}) {
	if block == nil {
		return
	}
	keys, _, version := usageKeys(req)
	info := usageBlockedRequest{CapturedAt: time.Now().UTC(), Address: address, Command: req.Type.String(), PeerRole: req.Peer.GetRole().String(), ReplicaReadType: fmt.Sprint(req.ReplicaReadType), ReplicaRead: req.ReplicaRead, StaleRead: req.StaleRead, RegionID: req.RegionId, Version: version, KeyClass: "other"}
	if len(keys) != 0 {
		info.KeySHA256 = fmt.Sprintf("%x", sha256.Sum256(keys[0]))
		if _, suffix, ok := strings.Cut(string(keys[0]), "by-pubkey:"); ok {
			info.KeyClass = "peer-record"
			if key, err := usagePeer(suffix); err == nil {
				info.PeerPublicKey = key.String()
			}
		} else if bytes.Contains(keys[0], []byte("presence:")) {
			info.KeyClass = "presence"
		} else if bytes.Contains(keys[0], []byte("api-keys:")) {
			info.KeyClass = "api-key"
		} else if bytes.Contains(keys[0], []byte("by-peer:")) {
			info.KeyClass = "assignment"
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.center == block && len(g.blockedRequests) < 32 {
		g.blockedRequests = append(g.blockedRequests, info)
	}
}
