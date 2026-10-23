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
	"sync"
	"sync/atomic"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

// ClientGate forwards native TiKV requests and responses unchanged. Its HTTP
// controls coordinate an isolated E2E fixture; production never installs it.
type ClientGate struct {
	tikv.Client
	listener net.Listener
	server   *http.Server
	served   chan error
	closing  chan struct{}
	closed   chan struct{}
	stopping atomic.Bool
	closeErr error
	current  atomic.Pointer[clientGateSession]
}

// ClientGateArm identifies the physical record and abandoned transaction whose
// read, rollback and prewrite requests should rendezvous before forwarding.
type ClientGateArm struct {
	Key     []byte `json:"key"`
	StartTS uint64 `json:"start_ts"`
}

// ClientGateState reports real requests, completed cleanup and write conflicts.
type ClientGateState struct {
	Armed             bool   `json:"armed"`
	Reads             int64  `json:"reads"`
	Prewrites         int64  `json:"prewrites"`
	PrewriteStartTS   uint64 `json:"prewrite_start_ts"`
	Resolves          int64  `json:"resolves"`
	Resolved          int64  `json:"resolved"`
	DuplicateReads    int64  `json:"duplicate_reads"`
	DuplicateResolves int64  `json:"duplicate_resolves"`
	WriteConflicts    int64  `json:"write_conflicts"`
	InFlight          int64  `json:"in_flight"`
}

type clientGatePhase struct {
	released chan struct{}
	once     sync.Once
}

func (p *clientGatePhase) release() { p.once.Do(func() { close(p.released) }) }

type clientGateSession struct {
	ClientGateArm
	reads, prewrites, resolves                           clientGatePhase
	readCount, prewriteCount, resolveCount, resolved     atomic.Int64
	duplicateReads, duplicateResolves, conflicts, active atomic.Int64
	prewriteStartTS                                      atomic.Uint64
}

// StartClientGate opens an explicit test-control listener and takes ownership
// of client only on success. Close cancels blocked requests and joins Serve.
func StartClientGate(client tikv.Client, address string) (*ClientGate, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	g := &ClientGate{Client: client, listener: listener, served: make(chan error, 1), closing: make(chan struct{}), closed: make(chan struct{})}
	g.server = &http.Server{Handler: http.HandlerFunc(g.control), ReadHeaderTimeout: time.Second}
	go func() {
		err := g.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		g.served <- err
	}()
	return g, nil
}

// URL returns the fixture's control endpoint, including an ephemeral port.
func (g *ClientGate) URL() string { return "http://" + g.listener.Addr().String() }

// Close releases callers before closing their borrowed native transport.
func (g *ClientGate) Close() error {
	if !g.stopping.CompareAndSwap(false, true) {
		<-g.closed
		return g.closeErr
	}
	close(g.closing)
	g.closeErr = errors.Join(g.server.Close(), <-g.served, g.Client.Close())
	close(g.closed)
	return g.closeErr
}

func (g *ClientGate) control(w http.ResponseWriter, r *http.Request) {
	session := g.current.Load()
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/arm":
		var arm ClientGateArm
		if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&arm); err != nil || len(arm.Key) == 0 || len(arm.Key) > 4096 || arm.StartTS == 0 {
			http.Error(w, "physical key and transaction timestamp required", http.StatusBadRequest)
			return
		}
		next := &clientGateSession{ClientGateArm: arm, reads: clientGatePhase{released: make(chan struct{})}, prewrites: clientGatePhase{released: make(chan struct{})}, resolves: clientGatePhase{released: make(chan struct{})}}
		if !g.current.CompareAndSwap(nil, next) {
			http.Error(w, "a session is already armed", http.StatusConflict)
		}
	case r.Method == http.MethodGet && r.URL.Path == "/status":
		state := ClientGateState{}
		if session != nil {
			state = ClientGateState{Armed: true, Reads: session.readCount.Load(), Prewrites: session.prewriteCount.Load(), PrewriteStartTS: session.prewriteStartTS.Load(),
				Resolves: session.resolveCount.Load(), Resolved: session.resolved.Load(), DuplicateReads: session.duplicateReads.Load(), DuplicateResolves: session.duplicateResolves.Load(),
				WriteConflicts: session.conflicts.Load(), InFlight: session.active.Load()}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(state); err != nil {
			return
		}
	case r.Method == http.MethodPost && r.URL.Path == "/reset":
		if session != nil {
			session.reads.release()
			session.resolves.release()
			session.prewrites.release()
			g.current.CompareAndSwap(session, nil)
		}
	case r.Method == http.MethodPost && session != nil:
		switch r.URL.Path {
		case "/release/reads":
			session.reads.release()
		case "/release/resolves":
			session.resolves.release()
		case "/release/prewrites":
			session.prewrites.release()
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

func (g *ClientGate) observe(req *tikvrpc.Request) (*clientGateSession, *clientGatePhase) {
	session := g.current.Load()
	if session == nil {
		return nil, nil
	}
	var keys [][]byte
	var phase *clientGatePhase
	switch req.Type {
	case tikvrpc.CmdBatchGet:
		keys, phase = req.BatchGet().Keys, &session.reads
	case tikvrpc.CmdPrewrite:
		for _, mutation := range req.Prewrite().Mutations {
			keys = append(keys, mutation.Key)
		}
		phase = &session.prewrites
	case tikvrpc.CmdResolveLock:
		if req.ResolveLock().StartVersion != session.StartTS || req.ResolveLock().CommitVersion != 0 {
			return nil, nil
		}
		keys, phase = req.ResolveLock().Keys, &session.resolves
	default:
		return nil, nil
	}
	found, duplicate := false, false
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		found = found || bytes.Equal(key, session.Key)
		duplicate = duplicate || seen[string(key)]
		seen[string(key)] = true
	}
	if !found {
		return nil, nil
	}
	session.active.Add(1)
	switch phase {
	case &session.reads:
		session.readCount.Add(1)
		if duplicate {
			session.duplicateReads.Add(1)
		}
	case &session.prewrites:
		session.prewriteCount.Add(1)
		session.prewriteStartTS.CompareAndSwap(0, req.Prewrite().StartVersion)
	case &session.resolves:
		session.resolveCount.Add(1)
		if duplicate {
			session.duplicateResolves.Add(1)
		}
	}
	return session, phase
}

func (g *ClientGate) wait(ctx context.Context, phase *clientGatePhase) error {
	if phase == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.closing:
		return errors.New("TiKV fixture client closed")
	case <-phase.released:
		return nil
	}
}

func completeClientGate(session *clientGateSession, resp *tikvrpc.Response, err error) {
	if session == nil {
		return
	}
	defer session.active.Add(-1)
	if err != nil || resp == nil {
		return
	}
	switch result := resp.Resp.(type) {
	case *kvrpcpb.ResolveLockResponse:
		if result.RegionError == nil && result.Error == nil {
			session.resolved.Add(1)
		}
	case *kvrpcpb.PrewriteResponse:
		for _, keyErr := range result.Errors {
			if keyErr.GetConflict() != nil {
				session.conflicts.Add(1)
			}
		}
	}
}

// SendRequest blocks only an armed target, then delegates the original request.
func (g *ClientGate) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	session, phase := g.observe(req)
	if err := g.wait(ctx, phase); err != nil {
		completeClientGate(session, nil, err)
		return nil, err
	}
	resp, err := g.Client.SendRequest(ctx, address, req, timeout)
	completeClientGate(session, resp, err)
	return resp, err
}

// SendRequestAsync retains the SDK callback and the native response unchanged.
func (g *ClientGate) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	session, phase := g.observe(req)
	if err := g.wait(ctx, phase); err != nil {
		completeClientGate(session, nil, err)
		callback.Invoke(nil, err)
		return
	}
	callback.Inject(func(resp *tikvrpc.Response, err error) (*tikvrpc.Response, error) {
		completeClientGate(session, resp, err)
		return resp, err
	})
	g.Client.SendRequestAsync(ctx, address, req, callback)
}

// CheckClientGate rejects incomplete observations and duplicate wire keys.
func CheckClientGate(state ClientGateState) error {
	if state.Reads == 0 || state.Prewrites == 0 || state.PrewriteStartTS == 0 || state.Resolves == 0 || state.Resolved == 0 || state.InFlight != 0 || state.DuplicateReads != 0 || state.DuplicateResolves != 0 {
		return fmt.Errorf("incomplete or duplicate TiKV requests: %+v", state)
	}
	return nil
}
