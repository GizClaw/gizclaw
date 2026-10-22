// Package peerpresence shares connection and running-Workspace presence through
// the Peer KV store. Ordinary reads may use a Redis replica; writes and their
// generation conditions execute at the write authority.
package peerpresence

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	leaseDuration = 30 * time.Second
	renewInterval = 10 * time.Second
	writeTimeout  = 2 * time.Second
)

// Snapshot contains only transition-driven presence. LastSeenAt is the last
// connection activity observed at a transition, rather than a traffic heartbeat.
type Snapshot struct {
	Online        bool      `json:"online"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	WorkspaceName string    `json:"workspace_name,omitempty"`
}

type record struct {
	Snapshot
	Server     string `json:"server"`
	Generation string `json:"generation"`
}

type lease struct {
	Generation string    `json:"generation"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type pending struct {
	publisher *Publisher
	snapshot  Snapshot
	revision  uint64
	running   bool
}

type ownership struct {
	expected    []byte
	initialized bool
	claimed     bool
	fenced      bool
}

// Publisher owns the presence token of one accepted Peer connection. Keep it
// for that connection's entire lifetime, including temporary offline states.
// Only a new accepted connection should receive a new Publisher.
// The zero value must not be used.
type Publisher struct {
	service *Service
	peer    giznet.PublicKey
	token   string
	// Only the service's serial Peer writer accesses ownership. Keeping it on
	// the Publisher preserves fencing when an offline worker is retired.
	ownership ownership
}

// NewPublisher creates a connection-scoped publisher without storage I/O or
// workers. Its first online transition may claim the Peer at the authority.
func (s *Service) NewPublisher(peer giznet.PublicKey) *Publisher {
	return &Publisher{service: s, peer: peer, token: rand.Text()}
}

// Service owns a Server generation, its lease and asynchronous per-Peer writers.
// Construct it before Start; Close stops and waits for all owned workers without
// closing the borrowed Store. The zero value must not be used.
type Service struct {
	store      kv.Store
	server     string
	generation string
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	started    bool
	starting   bool
	closed     bool
	pending    map[giznet.PublicKey]*pending
	mutations  chan transitionMutation
	workers    sync.WaitGroup
}

// New creates a presence service without starting workers or doing storage I/O.
func New(store kv.Store, server giznet.PublicKey) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{store: store, server: server.String(), generation: rand.Text(),
		ctx: ctx, cancel: cancel, pending: make(map[giznet.PublicKey]*pending), mutations: make(chan transitionMutation, transitionBatchSize)}
}

func generationKey(server string) kv.Key    { return kv.Key{"servers", server, "generation"} }
func leaseKey(server string) kv.Key         { return kv.Key{"servers", server, "lease"} }
func peerKey(peer giznet.PublicKey) kv.Key  { return kv.Key{"peers", peer.String()} }
func ownerKey(peer giznet.PublicKey) kv.Key { return kv.Key{"owners", peer.String()} }

func (s *Service) leaseEntry() kv.Entry {
	expires := time.Now().UTC().Add(leaseDuration + kv.ReadDelay(s.store))
	value, _ := json.Marshal(lease{Generation: s.generation, ExpiresAt: expires})
	return kv.Entry{Key: leaseKey(s.server), Value: value, Deadline: expires}
}

// Start publishes a new boot generation and starts its lease renewal. A new
// generation fences all writes and lease renewals from an older Server process.
// Call Start once before publishing connection transitions.
func (s *Service) Start(ctx context.Context) error {
	if s.store == nil {
		return errors.New("peerpresence: nil store")
	}
	s.mu.Lock()
	if s.closed || s.started || s.starting {
		s.mu.Unlock()
		return errors.New("peerpresence: service already started or closed")
	}
	s.cancel()
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.starting = true
	s.workers.Add(1)
	s.mu.Unlock()
	defer s.workers.Done()
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	if err := s.store.BatchSet(ctx, []kv.Entry{
		{Key: generationKey(s.server), Value: []byte(s.generation)}, s.leaseEntry(),
	}); err != nil {
		return fmt.Errorf("peerpresence: start Server lease: %w", err)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("peerpresence: service closed")
	}
	s.started = true
	s.workers.Add(3)
	s.mu.Unlock()
	go s.renew()
	go s.writeBatches()
	go s.writeBatches()
	return nil
}

func (s *Service) renew() {
	defer s.workers.Done()
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, writeTimeout)
			matched, err := s.store.ApplyMutation(ctx, kv.Mutation{
				Conditions: []kv.Condition{{Key: generationKey(s.server), Expected: []byte(s.generation)}},
				Entries:    []kv.Entry{s.leaseEntry()},
			})
			cancel()
			if err != nil {
				slog.Warn("peerpresence: renew Server lease", "error", err)
			} else if !matched {
				s.cancel()
				return
			}
		}
	}
}

// Publish queues the latest connection transition without storage I/O. Calls
// for a Peer must arrive in connection order. Reuse this Publisher for all of
// its connection's Workspace and offline transitions, including rollback.
// Connection ordering is per Peer; bounded workers batch storage commits.
// Failures retry until superseded or closed.
func (publisher *Publisher) Publish(snapshot Snapshot) {
	s, peer := publisher.service, publisher.peer
	if peer.IsZero() {
		return
	}
	if !snapshot.Online {
		snapshot.WorkspaceName = ""
	}
	snapshot.LastSeenAt = snapshot.LastSeenAt.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started || s.closed || s.ctx.Err() != nil {
		return
	}
	p := s.pending[peer]
	if p == nil {
		p = &pending{}
		s.pending[peer] = p
	} else if p.publisher == publisher && p.snapshot == snapshot {
		return
	}
	p.publisher = publisher
	p.snapshot = snapshot
	p.revision++
	if !p.running {
		p.running = true
		s.workers.Add(1)
		go s.write(peer, p)
	}
}

func (s *Service) write(peer giznet.PublicKey, p *pending) {
	defer s.workers.Done()
	for s.ctx.Err() == nil {
		s.mu.Lock()
		publisher, snapshot, revision := p.publisher, p.snapshot, p.revision
		s.mu.Unlock()
		ctx, cancel := context.WithTimeout(s.ctx, writeTimeout)
		err := s.writeTransition(ctx, publisher, snapshot)
		cancel()
		if err != nil {
			slog.Warn("peerpresence: publish Peer transition", "peer_public_key", peer.String(), "error", err)
			timer := time.NewTimer(time.Second)
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}
		s.mu.Lock()
		if revision == p.revision {
			p.running = false
			if !snapshot.Online {
				delete(s.pending, peer)
			}
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
	}
}

// writeTransition claims a connection once, then fences every later transition
// at the write authority. Retaining the original claim condition across errors
// prevents an uncertain commit from reclaiming ownership after another Server.
func (s *Service) writeTransition(ctx context.Context, publisher *Publisher, snapshot Snapshot) error {
	peer, owner := publisher.peer, &publisher.ownership
	if owner.fenced {
		return nil
	}
	ctx = kv.WithPrimaryReads(ctx)
	token := []byte(s.generation + "/" + publisher.token)
	if !snapshot.Online && !owner.claimed {
		// A coalesced disconnect cannot claim another connection's Peer. If
		// an online attempt lost its response, revoke it only if it committed.
		if !owner.initialized {
			return nil
		}
		current, err := s.readOwner(ctx, peer)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, token) {
			return nil
		}
		owner.expected, owner.claimed = token, true
	}
	if !owner.initialized {
		expected, err := s.readOwner(ctx, peer)
		if err != nil {
			return err
		}
		owner.expected, owner.initialized = expected, true
	}
	value, _ := json.Marshal(record{Snapshot: snapshot, Server: s.server, Generation: s.generation})
	for {
		mutation := kv.Mutation{
			Conditions: []kv.Condition{
				{Key: generationKey(s.server), Expected: []byte(s.generation)},
				{Key: ownerKey(peer), Expected: owner.expected},
			},
			Entries: []kv.Entry{{Key: peerKey(peer), Value: value}},
		}
		if !owner.claimed {
			mutation.Entries = append(mutation.Entries, kv.Entry{Key: ownerKey(peer), Value: token})
		}
		matched, err := s.applyTransition(ctx, mutation)
		if err != nil {
			return err
		}
		if matched {
			owner.expected, owner.claimed = token, true
			return nil
		}
		generation, err := s.store.Get(ctx, generationKey(s.server))
		if errors.Is(err, kv.ErrNotFound) || (err == nil && !bytes.Equal(generation, []byte(s.generation))) {
			s.cancel()
			return nil
		}
		if err != nil {
			return err
		}
		current, err := s.readOwner(ctx, peer)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, token) {
			owner.fenced = true
			return nil
		}
		// An earlier attempt committed but its response was lost. Continue
		// using the committed token rather than claiming the Peer again.
		owner.expected, owner.claimed = token, true
	}
}

func (s *Service) readOwner(ctx context.Context, peer giznet.PublicKey) ([]byte, error) {
	value, err := s.store.Get(ctx, ownerKey(peer))
	if errors.Is(err, kv.ErrNotFound) {
		return nil, nil
	}
	return value, err
}

// Read returns the shared snapshot using ordinary Store reads. Missing, expired
// or replaced Server leases make a stored Peer offline without losing its last
// observed time. Storage errors are returned and never fall back to SQL.
func (s *Service) Read(ctx context.Context, peer giznet.PublicKey) (Snapshot, error) {
	result := s.ReadBatch(ctx, []giznet.PublicKey{peer})[0]
	return result.Snapshot, result.Err
}

// ReadResult contains one Peer's validated snapshot or its read/decode error.
// A missing Peer is a successful, empty offline snapshot.
type ReadResult struct {
	Snapshot Snapshot
	Err      error
}

// ReadBatch reads Peer records together, then reads each referenced Server's
// lease once. Both rounds share a two-second budget and preserve input order.
// Individual failures do not discard other Peers' successful snapshots.
func (s *Service) ReadBatch(ctx context.Context, peers []giznet.PublicKey) []ReadResult {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	keys := make([]kv.Key, len(peers))
	for i, peer := range peers {
		keys[i] = peerKey(peer)
	}
	results := make([]ReadResult, len(peers))
	records := make([]record, len(peers))
	leaseIndices := make(map[string]int)
	var leaseKeys []kv.Key
	for i, value := range s.store.BatchGet(ctx, keys) {
		if errors.Is(value.Err, kv.ErrNotFound) {
			continue
		}
		if value.Err != nil {
			results[i].Err = value.Err
			continue
		}
		record := &records[i]
		if err := json.Unmarshal(value.Value, record); err != nil {
			results[i].Err = fmt.Errorf("peerpresence: decode Peer: %w", err)
			continue
		}
		var server giznet.PublicKey
		if err := server.UnmarshalText([]byte(record.Server)); err != nil || server.IsZero() || server.String() != record.Server || record.Generation == "" {
			results[i].Err = errors.New("peerpresence: invalid Peer record")
			continue
		}
		if !record.Online {
			record.WorkspaceName = ""
		} else if _, ok := leaseIndices[record.Server]; !ok {
			leaseIndices[record.Server] = len(leaseKeys)
			leaseKeys = append(leaseKeys, leaseKey(record.Server))
		}
		results[i].Snapshot = record.Snapshot
	}
	values := s.store.BatchGet(ctx, leaseKeys)
	leases := make([]lease, len(values))
	for i := range values {
		if values[i].Err == nil {
			if err := json.Unmarshal(values[i].Value, &leases[i]); err != nil {
				values[i].Err = fmt.Errorf("peerpresence: decode Server lease: %w", err)
			}
		}
	}
	now := time.Now()
	for i := range results {
		result, record := &results[i], &records[i]
		if result.Err != nil || !record.Online {
			continue
		}
		index := leaseIndices[record.Server]
		if err := values[index].Err; err != nil && !errors.Is(err, kv.ErrNotFound) {
			*result = ReadResult{Err: err}
			continue
		}
		if leases[index].Generation != record.Generation || !now.Before(leases[index].ExpiresAt) {
			result.Snapshot.Online = false
			result.Snapshot.WorkspaceName = ""
		}
	}
	return results
}

// Close cancels all workers and revokes only this generation's Server lease.
// Readers then consider all of this Server's Peers offline after replication.
func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.workers.Wait()
	if s.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	_, err := s.store.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{{Key: generationKey(s.server), Expected: []byte(s.generation)}},
		DeleteKeys: []kv.Key{leaseKey(s.server)},
	})
	return err
}
