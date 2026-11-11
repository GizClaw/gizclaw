package tikvfixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

// UsageGate forwards native TiKV requests and responses unchanged. Only the
// explicitly overlaid Docker binary installs its private coordination listener.
// A response gate retains the real response and its original read timestamp.
// It never replaces a KV value, manufactures a conflict, or changes routing.
type UsageGate struct {
	tikv.Client
	listener        net.Listener
	server          *http.Server
	served          chan error
	closing         chan struct{}
	closed          chan struct{}
	stopping        atomic.Bool
	closeErr        error
	mu              sync.Mutex
	rules           map[string]*usageRule
	center          chan struct{}
	services        atomic.Pointer[UsageServices]
	readClock       atomic.Pointer[usageReadClock]
	blockedStacks   []usageBlockedFrame
	blockedRequests []usageBlockedRequest
	ruleHistory     []map[string]UsageObservation
	stackWorkers    sync.WaitGroup
	recoveryMu      sync.Mutex
	recoveryPaused  bool
	recoveryActive  int
	recoveryChanged chan struct{}
}

// UsageRule selects one native boundary by physical-key fragments. Key contains
// unescaped logical segments: the probe escapes each segment before matching.
// Commands use client-go names, such as Get, BatchGet, Scan and Prewrite.
// Source can be learner or center. Stage is request or response. Skip counts
// matches before holding one; CountOnly observes every match without blocking.
type UsageRule struct {
	ID          string   `json:"id"`
	Key         []string `json:"key"`
	Commands    []string `json:"commands"`
	Source      string   `json:"source,omitempty"`
	Stage       string   `json:"stage"`
	Skip        int64    `json:"skip,omitempty"`
	CountOnly   bool     `json:"count_only,omitempty"`
	Diagnostics bool     `json:"diagnostics,omitempty"`
	MutationOp  string   `json:"mutation_op,omitempty"`
	HoldAll     bool     `json:"hold_all,omitempty"`
}

// UsageObservation distinguishes application traffic from fixture diagnostics.
// All counters describe actual transport calls; hit means the selected boundary
// was reached, while completed means the original transport returned.
type UsageObservation struct {
	ID                  string                 `json:"id"`
	Hits                int64                  `json:"hits"`
	Completed           int64                  `json:"completed"`
	ApplicationReads    int64                  `json:"application_reads"`
	DiagnosticReads     int64                  `json:"diagnostic_reads"`
	LearnerReads        int64                  `json:"learner_reads"`
	CenterReads         int64                  `json:"center_reads"`
	Errors              int64                  `json:"errors"`
	WriteConflicts      int64                  `json:"write_conflicts"`
	MissingResponses    int64                  `json:"missing_responses"`
	PositiveResponses   int64                  `json:"positive_responses"`
	NativeNotFoundFlags int64                  `json:"native_not_found_flags"`
	LastReadVersion     uint64                 `json:"last_read_version"`
	LastResponse        *usageResponseMetadata `json:"last_response,omitempty"`
	LastMutation        *usageMutationMetadata `json:"last_mutation,omitempty"`
	HeldRequests        int64                  `json:"held_requests"`
	Held                bool                   `json:"held"`
	Released            bool                   `json:"released"`
}

type usageRule struct {
	UsageRule
	match                                []byte
	hits, completed, app, diagnostic     atomic.Int64
	learner, center, failures, conflicts atomic.Int64
	missing, positive, notFoundFlags     atomic.Int64
	version                              atomic.Uint64
	lastResponse                         atomic.Pointer[usageResponseMetadata]
	lastMutation                         atomic.Pointer[usageMutationMetadata]
	held                                 atomic.Int64
	released                             atomic.Bool
	hit, release                         chan struct{}
	hitOnce, releaseOnce                 sync.Once
}

type usageDiagnosticKey struct{}

var usageTransport atomic.Pointer[UsageGate]

// StartUsageGate explicitly starts the test-only HTTP listener. The returned
// wrapper owns client on success. Its Close cancels waiters before transport.
func StartUsageGate(client tikv.Client, address string) (*UsageGate, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	g := &UsageGate{Client: client, listener: listener, served: make(chan error, 1), closing: make(chan struct{}), closed: make(chan struct{}), rules: make(map[string]*usageRule), recoveryChanged: make(chan struct{})}
	g.server = &http.Server{Handler: http.HandlerFunc(g.control), ReadHeaderTimeout: time.Second}
	usageTransport.Store(g)
	go func() {
		err := g.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		g.served <- err
	}()
	return g, nil
}

// URL returns the isolated fixture control origin.
func (g *UsageGate) URL() string { return "http://" + g.listener.Addr().String() }

// Close cancels blocked requests and joins the listener before closing TiKV.
func (g *UsageGate) Close() error {
	if !g.stopping.CompareAndSwap(false, true) {
		<-g.closed
		return g.closeErr
	}
	close(g.closing)
	g.mu.Lock()
	g.mu.Unlock()
	g.stackWorkers.Wait()
	usageTransport.CompareAndSwap(g, nil)
	g.closeErr = errors.Join(g.server.Close(), <-g.served, g.Client.Close())
	close(g.closed)
	return g.closeErr
}

func (r *usageRule) unblock() {
	r.releaseOnce.Do(func() {
		r.released.Store(true)
		close(r.release)
	})
}

func (r *usageRule) observation() UsageObservation {
	return UsageObservation{ID: r.ID, Hits: r.hits.Load(), Completed: r.completed.Load(), ApplicationReads: r.app.Load(), DiagnosticReads: r.diagnostic.Load(), LearnerReads: r.learner.Load(), CenterReads: r.center.Load(), Errors: r.failures.Load(), WriteConflicts: r.conflicts.Load(), MissingResponses: r.missing.Load(), PositiveResponses: r.positive.Load(), NativeNotFoundFlags: r.notFoundFlags.Load(), LastResponse: r.lastResponse.Load(), LastMutation: r.lastMutation.Load(), HeldRequests: r.held.Load(), LastReadVersion: r.version.Load(), Held: r.held.Load() > 0, Released: r.released.Load()}
}

func usageJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func usageDecode(r *http.Request, value any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}

func (g *UsageGate) rule(id string) *usageRule {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rules[id]
}

func (g *UsageGate) control(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if r.Method == http.MethodGet && r.URL.Path == "/debug/blocked-stacks" {
		g.mu.Lock()
		frames := append([]usageBlockedFrame(nil), g.blockedStacks...)
		requests := append([]usageBlockedRequest(nil), g.blockedRequests...)
		history := append([]map[string]UsageObservation(nil), g.ruleHistory...)
		g.mu.Unlock()
		usageJSON(w, map[string]any{"diagnostic": true, "frames": frames, "requests": requests, "rule_history": history})
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/debug/stack" {
		buffer := make([]byte, 2<<20)
		length := runtime.Stack(buffer, true)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(buffer[:length])
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/status" {
		g.mu.Lock()
		states := make(map[string]UsageObservation, len(g.rules))
		for id, rule := range g.rules {
			states[id] = rule.observation()
		}
		blocked := g.center != nil
		g.mu.Unlock()
		usageJSON(w, map[string]any{"diagnostic": true, "rules": states, "center_blocked": blocked, "services_ready": g.services.Load() != nil})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/arm":
		var input UsageRule
		if err := usageDecode(r, &input); err != nil || input.ID == "" || len(input.Key) == 0 || len(input.Commands) == 0 || input.Skip < 0 || (input.Stage != "request" && input.Stage != "response") || (input.Source != "" && input.Source != "learner" && input.Source != "center") {
			http.Error(w, "id, key, commands and request/response stage required", http.StatusBadRequest)
			return
		}
		if input.MutationOp != "" && input.MutationOp != "Put" && input.MutationOp != "Del" {
			http.Error(w, "mutation_op must be Put or Del", http.StatusBadRequest)
			return
		}
		segments := make([]string, len(input.Key))
		for i, part := range input.Key {
			if part == "" {
				http.Error(w, "empty key segment", http.StatusBadRequest)
				return
			}
			segments[i] = url.QueryEscape(part)
		}
		rule := &usageRule{UsageRule: input, match: []byte(strings.Join(segments, ":")), hit: make(chan struct{}), release: make(chan struct{})}
		g.mu.Lock()
		_, exists := g.rules[input.ID]
		if !exists {
			g.rules[input.ID] = rule
		}
		g.mu.Unlock()
		if exists {
			http.Error(w, "rule already exists; reset before rearming", http.StatusConflict)
			return
		}
		usageJSON(w, rule.observation())
	case "/wait", "/release":
		var input struct {
			ID string `json:"id"`
		}
		if err := usageDecode(r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rule := g.rule(input.ID)
		if rule == nil {
			http.Error(w, "unknown rule", http.StatusNotFound)
			return
		}
		if r.URL.Path == "/wait" {
			if err := g.wait(ctx, rule.hit); err != nil {
				http.Error(w, err.Error(), http.StatusGatewayTimeout)
				return
			}
		} else {
			rule.unblock()
		}
		usageJSON(w, rule.observation())
	case "/read-clock":
		var input struct {
			Frozen bool `json:"frozen"`
		}
		if err := usageDecode(r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		clock := g.readClock.Load()
		if clock == nil {
			http.Error(w, "SDK read clock is not attached", http.StatusServiceUnavailable)
			return
		}
		version, err := clock.setFrozen(ctx, input.Frozen)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		usageJSON(w, map[string]any{"diagnostic": true, "frozen": input.Frozen, "read_version": version})
	case "/automatic-recovery":
		var input struct {
			Paused bool `json:"paused"`
		}
		if err := usageDecode(r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		active, err := g.pauseAutomaticRecovery(ctx, input.Paused)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGatewayTimeout)
			return
		}
		usageJSON(w, map[string]any{"diagnostic": true, "paused": input.Paused, "active": active})
	case "/block-center":
		var input struct {
			Blocked bool `json:"blocked"`
		}
		if err := usageDecode(r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var capture chan struct{}
		g.mu.Lock()
		if input.Blocked && g.center == nil {
			g.center = make(chan struct{})
			capture = g.center
		} else if !input.Blocked && g.center != nil {
			close(g.center)
			g.center = nil
		}
		g.mu.Unlock()
		if capture != nil {
			g.captureBlocked(capture)
		}
		usageJSON(w, map[string]any{"diagnostic": true, "center_blocked": input.Blocked})
	case "/reset":
		g.mu.Lock()
		archived := make(map[string]UsageObservation, len(g.rules))
		for id, rule := range g.rules {
			archived[id] = rule.observation()
			rule.unblock()
		}
		if len(archived) != 0 && len(g.ruleHistory) < 16 {
			g.ruleHistory = append(g.ruleHistory, archived)
		}
		g.rules = make(map[string]*usageRule)
		if g.center != nil {
			close(g.center)
			g.center = nil
		}
		g.mu.Unlock()
		if clock := g.readClock.Load(); clock != nil {
			clock.frozen.Store(0)
		}
		_, _ = g.pauseAutomaticRecovery(ctx, false)
		usageJSON(w, map[string]bool{"diagnostic": true, "reset": true})
	case "/invoke":
		var input UsageAction
		if err := usageDecode(r, &input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := g.invoke(ctx, input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		usageJSON(w, result)
	case "/after":
		g.after(ctx, w, r)
	case "/race":
		g.race(ctx, w, r)
	default:
		http.NotFound(w, r)
	}
}

func (g *UsageGate) wait(ctx context.Context, ready <-chan struct{}) error {
	if ready == nil {
		return nil
	}
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-g.closing:
		return errors.New("TiKV usage probe closed")
	}
}

func usageKeys(req *tikvrpc.Request) ([][]byte, bool, uint64) {
	switch req.Type {
	case tikvrpc.CmdGet:
		return [][]byte{req.Get().Key}, true, req.Get().Version
	case tikvrpc.CmdBatchGet:
		return req.BatchGet().Keys, true, req.BatchGet().Version
	case tikvrpc.CmdScan:
		return [][]byte{req.Scan().StartKey}, true, req.Scan().Version
	case tikvrpc.CmdPrewrite:
		keys := make([][]byte, len(req.Prewrite().Mutations))
		for i, mutation := range req.Prewrite().Mutations {
			keys[i] = mutation.Key
		}
		return keys, false, 0
	case tikvrpc.CmdCommit:
		return req.Commit().Keys, false, 0
	default:
		return nil, false, 0
	}
}

func (g *UsageGate) observe(ctx context.Context, req *tikvrpc.Request) []*usageRule {
	keys, read, version := usageKeys(req)
	if len(keys) == 0 {
		return nil
	}
	learner := req.Peer.GetRole() == metapb.PeerRole_Learner
	diagnostic, _ := ctx.Value(usageDiagnosticKey{}).(bool)
	g.mu.Lock()
	defer g.mu.Unlock()
	var matched []*usageRule
	for _, rule := range g.rules {
		if rule.Source == "learner" && !learner || rule.Source == "center" && learner {
			continue
		}
		commandMatch := false
		for _, command := range rule.Commands {
			commandMatch = commandMatch || command == req.Type.String()
		}
		found := false
		if rule.MutationOp != "" {
			if req.Type != tikvrpc.CmdPrewrite {
				continue
			}
			for _, mutation := range req.Prewrite().Mutations {
				if mutation != nil && mutation.Op.String() == rule.MutationOp && bytes.Contains(mutation.Key, rule.match) {
					found = true
					rule.lastMutation.Store(&usageMutationMetadata{Operation: mutation.Op.String(), ValueBytes: len(mutation.Value)})
				}
			}
		} else {
			for _, key := range keys {
				found = found || bytes.Contains(key, rule.match)
			}
		}
		if !commandMatch || !found {
			continue
		}
		if read {
			if diagnostic {
				rule.diagnostic.Add(1)
			} else {
				rule.app.Add(1)
			}
			if learner {
				rule.learner.Add(1)
			} else {
				rule.center.Add(1)
			}
			rule.version.Store(version)
		}
		// Readiness polls cannot steal application gates. A document must opt
		// into diagnostic boundaries explicitly when testing a private adapter.
		if !diagnostic || rule.Diagnostics {
			matched = append(matched, rule)
		}
	}
	return matched
}

func (g *UsageGate) boundary(ctx context.Context, rules []*usageRule, stage string) error {
	for _, rule := range rules {
		if rule.Stage != stage {
			continue
		}
		ordinal := rule.hits.Add(1)
		if ordinal <= rule.Skip {
			continue
		}
		rule.hitOnce.Do(func() { close(rule.hit) })
		if !rule.CountOnly && (rule.HoldAll || ordinal == rule.Skip+1) {
			rule.held.Add(1)
			err := g.wait(ctx, rule.release)
			rule.held.Add(-1)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func usageComplete(rules []*usageRule, response *tikvrpc.Response, err error) {
	info := usageResponseInfo(response)
	for _, rule := range rules {
		rule.lastResponse.Store(&info)
		rule.completed.Add(1)
		if err != nil || response == nil {
			rule.failures.Add(1)
			continue
		}
		if result, ok := response.Resp.(*kvrpcpb.GetResponse); ok && result != nil {
			if result.NotFound {
				rule.notFoundFlags.Add(1)
			}
			if result.RegionError != nil || result.Error != nil {
				rule.failures.Add(1)
				continue
			}
			// client-go Get treats an empty Value as ErrNotExist. TiKV 8.5
			// need not set NotFound. Every persisted application record has a
			// nonempty native header, even when its logical payload is empty.
			if len(result.Value) == 0 {
				rule.missing.Add(1)
			} else {
				rule.positive.Add(1)
			}
		}
		if result, ok := response.Resp.(*kvrpcpb.PrewriteResponse); ok {
			for _, item := range result.Errors {
				if item.GetConflict() != nil {
					rule.conflicts.Add(1)
				}
			}
		}
	}
}

func (g *UsageGate) before(ctx context.Context, address string, req *tikvrpc.Request) ([]*usageRule, error) {
	rules := g.observe(ctx, req)
	if err := g.boundary(ctx, rules, "request"); err != nil {
		return rules, err
	}
	g.mu.Lock()
	blocked := g.center
	g.mu.Unlock()
	if req.Peer.GetRole() != metapb.PeerRole_Learner && !strings.Contains(address, "learner") && !strings.Contains(address, "replication-proxy") {
		g.recordBlocked(address, req, blocked)
		if err := g.wait(ctx, blocked); err != nil {
			return rules, fmt.Errorf("center transport paused: %w", err)
		}
	}
	return rules, nil
}

// SendRequest preserves the original request, destination and native response.
func (g *UsageGate) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	rules, err := g.before(ctx, address, req)
	if err != nil {
		usageComplete(rules, nil, err)
		return nil, err
	}
	response, err := g.Client.SendRequest(ctx, address, req, timeout)
	usageRecordRead(ctx, req, response, err)
	usageComplete(rules, response, err)
	if err == nil {
		if err := g.boundary(ctx, rules, "response"); err != nil {
			return nil, err
		}
	}
	return response, err
}

// SendRequestAsync retains the SDK callback and the original response identity.
func (g *UsageGate) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	var cancel context.CancelFunc
	if req.MaxExecutionDurationMs > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.MaxExecutionDurationMs)*time.Millisecond)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	rules, err := g.before(ctx, address, req)
	if err != nil {
		cancel()
		usageComplete(rules, nil, err)
		callback.Invoke(nil, err)
		return
	}
	callback.Inject(func(response *tikvrpc.Response, err error) (*tikvrpc.Response, error) {
		defer cancel()
		usageRecordRead(ctx, req, response, err)
		usageComplete(rules, response, err)
		if err == nil {
			if err := g.boundary(ctx, rules, "response"); err != nil {
				return nil, err
			}
		}
		return response, err
	})
	g.Client.SendRequestAsync(ctx, address, req, callback)
}
