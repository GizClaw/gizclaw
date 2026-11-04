// Command presence-benchmark measures production presence publication against
// an explicitly supplied, isolated TiKV cluster. The runner owns the cluster.
package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"
	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

type rpcClient struct {
	tikv.Client
	delay     time.Duration
	requests  atomic.Int64
	prewrites atomic.Int64
	conflicts atomic.Int64
}

func (c *rpcClient) SendRequest(ctx context.Context, address string, request *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	if c.delay > 0 {
		timer := time.NewTimer(c.delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	c.requests.Add(1)
	if request.Type == tikvrpc.CmdPrewrite {
		c.prewrites.Add(1)
	}
	response, err := c.Client.SendRequest(ctx, address, request, timeout)
	if err == nil && request.Type == tikvrpc.CmdPrewrite {
		prewrite, ok := response.Resp.(*kvrpcpb.PrewriteResponse)
		if !ok {
			return nil, errors.New("unexpected native prewrite response")
		}
		for _, failure := range prewrite.Errors {
			if failure.GetConflict() != nil {
				c.conflicts.Add(1)
			}
		}
	}
	return response, err
}

func (c *rpcClient) SendRequestAsync(ctx context.Context, address string, request *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	go func() {
		response, err := c.SendRequest(ctx, address, request, 5*time.Second)
		callback.Schedule(response, err)
	}()
}

type committed struct {
	values []peerpresence.Snapshot
	at     time.Time
}

type observedStore struct {
	kv.Store
	events chan committed
	ctx    context.Context
	errors atomic.Int64
}

func (s *observedStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	matched, err := s.Store.ApplyMutation(ctx, mutation)
	if err != nil {
		s.errors.Add(1)
	}
	if err == nil && matched {
		event := committed{at: time.Now()}
		for _, entry := range mutation.Entries {
			var snapshot peerpresence.Snapshot
			if json.Unmarshal(entry.Value, &snapshot) == nil && !snapshot.LastSeenAt.IsZero() {
				event.values = append(event.values, snapshot)
			}
		}
		if len(event.values) > 0 {
			select {
			case s.events <- event:
			case <-s.ctx.Done():
			}
		}
	}
	return matched, err
}

type operation struct {
	peer    int
	started time.Time
	value   peerpresence.Snapshot
}

type report struct {
	Label              string   `json:"label"`
	Peers              int      `json:"peers"`
	Servers            int      `json:"servers"`
	Voters             int      `json:"verified_voters"`
	RPCDelayMS         float64  `json:"rpc_delay_ms"`
	WindowSeconds      float64  `json:"window_seconds"`
	DrainSeconds       float64  `json:"drain_seconds"`
	Submitted          int      `json:"submitted"`
	CompletedInWindow  int      `json:"completed_in_window"`
	Completed          int      `json:"completed_including_drain"`
	WithinBudget       int      `json:"completed_within_2s"`
	Uncompleted        int      `json:"uncompleted_after_drain"`
	Mismatched         int      `json:"mismatched_commit_snapshots"`
	VerifiedSnapshots  int      `json:"verified_final_snapshots"`
	ReadErrors         int      `json:"final_snapshot_read_errors"`
	Throughput         float64  `json:"window_completions_per_second"`
	P50MS              *float64 `json:"completed_latency_p50_ms"`
	P95MS              *float64 `json:"completed_latency_p95_ms"`
	MaxMS              *float64 `json:"completed_latency_max_ms"`
	RPCs               int64    `json:"tikv_rpc_calls"`
	Prewrites          int64    `json:"prewrite_rpc_calls"`
	Conflicts          int64    `json:"prewrite_write_conflict_errors"`
	MutationErrors     int64    `json:"mutation_errors"`
	PrewritesPerCommit *float64 `json:"prewrites_per_completed_transition"`
}

func main() {
	pd := flag.String("pd", "", "isolated PD host:port; required")
	peers := flag.Int("peers", 500, "concurrent Peer publishers")
	servers := flag.Int("servers", 1, "Server services sharing the cluster")
	delay := flag.Duration("delay", 20*time.Millisecond, "added time before each real TiKV RPC")
	duration := flag.Duration("duration", 15*time.Second, "closed-loop submission window")
	drain := flag.Duration("drain", 6*time.Second, "maximum final drain; incomplete operations stay failures")
	output := flag.String("output", "", "report JSON path; required")
	label := flag.String("label", "", "source variant and repetition")
	flag.Parse()
	if *pd == "" || *output == "" || *peers < 1 || *peers > 2000 || *servers < 1 || *servers > min(*peers, 64) || *delay < 0 || *delay > time.Second || *duration < time.Second || *duration > 5*time.Minute || *drain < time.Second || *drain > 30*time.Second {
		fmt.Fprintln(os.Stderr, "invalid benchmark arguments")
		os.Exit(2)
	}
	result, err := run(*pd, *peers, *servers, *delay, *duration, *drain, *label)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err == nil {
		err = os.WriteFile(*output, append(raw, '\n'), 0o600)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(string(raw))
}

// Read placement rather than changing it: every application Region must have
// exactly three live voting stores. The benchmark runner creates them locally.
func verifyVoters(ctx context.Context, pd string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var topology struct {
			Regions []struct {
				Peers []struct {
					StoreID uint64 `json:"store_id"`
					Role    int    `json:"role"`
				} `json:"peers"`
			} `json:"regions"`
		}
		err := tikvfixture.HTTP(ctx, "GET", "http://"+pd+"/pd/api/v1/regions", nil, &topology)
		ready := err == nil && len(topology.Regions) > 0
		for _, region := range topology.Regions {
			stores := make(map[uint64]bool)
			for _, peer := range region.Peers {
				ready = ready && peer.Role == 0 && peer.StoreID != 0
				stores[peer.StoreID] = true
			}
			ready = ready && len(region.Peers) == 3 && len(stores) == 3
		}
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("three-voter readiness: %w (last request: %v)", ctx.Err(), err)
		case <-ticker.C:
		}
	}
}

func run(pd string, count, serverCount int, delay, duration, drain time.Duration, label string) (report, error) {
	ctx, cancel := context.WithTimeout(context.Background(), duration+drain+90*time.Second)
	defer cancel()
	if err := verifyVoters(ctx, pd); err != nil {
		return report{}, err
	}
	physical, err := storage.New(map[string]storage.Config{"writer": storage.TiKVConfig{PDAddresses: []string{pd}}, "reader": storage.TiKVConfig{PDAddresses: []string{pd}}})
	if err != nil {
		return report{}, err
	}
	defer physical.Close()
	client, _, err := physical.TiKV("writer")
	if err != nil {
		return report{}, err
	}
	observedRPC := &rpcClient{Client: client.GetTiKVClient(), delay: delay}
	client.SetTiKVClient(observedRPC)
	root, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{}, nil)
	if err != nil {
		return report{}, err
	}
	prefix := kv.Key{"presence-benchmark-" + rand.Text()}
	store := &observedStore{Store: kv.Prefixed(root, prefix), ctx: ctx, events: make(chan committed, 8192)}
	services := make([]*peerpresence.Service, serverCount)
	for i := range services {
		services[i] = peerpresence.New(store, giznet.PublicKey{byte(i + 1)})
		defer services[i].Close()
		if err := services[i].Start(ctx); err != nil {
			return report{}, err
		}
	}
	readerClient, _, err := physical.TiKV("reader")
	if err != nil {
		return report{}, err
	}
	readerRoot, err := kv.NewTiKVWithClient(readerClient, kv.TiKVReadOptions{}, nil)
	if err != nil {
		return report{}, err
	}
	reader := peerpresence.New(kv.Prefixed(readerRoot, prefix), giznet.PublicKey{99})
	keys := make([]giznet.PublicKey, count)
	publishers := make([]*peerpresence.Publisher, count)
	for i := range keys {
		binary.BigEndian.PutUint64(keys[i][:], uint64(i+100))
		publishers[i] = services[i%serverCount].NewPublisher(keys[i])
	}
	result := report{Label: label, Peers: count, Servers: serverCount, Voters: 3, RPCDelayMS: float64(delay) / float64(time.Millisecond), WindowSeconds: duration.Seconds()}
	rpcBefore, prewriteBefore, conflictsBefore := observedRPC.requests.Load(), observedRPC.prewrites.Load(), observedRPC.conflicts.Load()
	started := time.Now()
	cutoff := started.Add(duration)
	end := cutoff.Add(drain)
	sequence := int64(0)
	cycles := make([]int, count)
	wanted := make([]peerpresence.Snapshot, count)
	pending := make(map[int64]operation, count)
	publish := func(peer int) {
		sequence++
		value := peerpresence.Snapshot{Online: cycles[peer]%3 != 2, LastSeenAt: started.UTC().Add(time.Duration(sequence))}
		if cycles[peer]%3 == 1 {
			value.WorkspaceName = "benchmark-room"
		}
		cycles[peer]++
		wanted[peer] = value
		pending[value.LastSeenAt.UnixNano()] = operation{peer: peer, started: time.Now(), value: value}
		result.Submitted++
		publishers[peer].Publish(value)
	}
	for i := range publishers {
		publish(i)
	}
	var latencies []time.Duration
	timer := time.NewTimer(time.Until(end))
	defer timer.Stop()
collect:
	for len(pending) > 0 {
		select {
		case <-ctx.Done():
			return report{}, ctx.Err()
		case <-timer.C:
			break collect
		case event := <-store.events:
			for _, value := range event.values {
				key := value.LastSeenAt.UnixNano()
				operation, ok := pending[key]
				if !ok {
					continue
				}
				delete(pending, key)
				if value != operation.value {
					result.Mismatched++
					continue
				}
				elapsed := event.at.Sub(operation.started)
				result.Completed++
				latencies = append(latencies, elapsed)
				if elapsed <= 2*time.Second {
					result.WithinBudget++
				}
				if !event.at.After(cutoff) {
					result.CompletedInWindow++
				}
				if time.Now().Before(cutoff) {
					publish(operation.peer)
				}
			}
		}
	}
	result.DrainSeconds = max(0, time.Since(started).Seconds()-duration.Seconds())
	result.Uncompleted = len(pending)
	result.Throughput = float64(result.CompletedInWindow) / duration.Seconds()
	result.RPCs = observedRPC.requests.Load() - rpcBefore
	result.Prewrites = observedRPC.prewrites.Load() - prewriteBefore
	result.Conflicts = observedRPC.conflicts.Load() - conflictsBefore
	result.MutationErrors = store.errors.Load()
	if result.Completed > 0 {
		perCommit := float64(result.Prewrites) / float64(result.Completed)
		result.PrewritesPerCommit = &perCommit
		slices.Sort(latencies)
		p50 := float64(latencies[(len(latencies)-1)/2]) / float64(time.Millisecond)
		p95 := float64(latencies[(len(latencies)-1)*95/100]) / float64(time.Millisecond)
		maximum := float64(latencies[len(latencies)-1]) / float64(time.Millisecond)
		result.P50MS, result.P95MS, result.MaxMS = &p50, &p95, &maximum
	}
	for i, snapshot := range reader.ReadBatch(ctx, keys) {
		if snapshot.Err != nil {
			result.ReadErrors++
		} else if snapshot.Snapshot == wanted[i] {
			result.VerifiedSnapshots++
		}
	}
	if result.Mismatched > 0 {
		return result, errors.New("committed snapshots differ from submitted transitions")
	}
	return result, nil
}
