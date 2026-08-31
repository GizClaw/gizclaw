package redis8

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	"github.com/redis/go-redis/v9"
)

const stateVersion = 1

type durableState struct {
	Version     int                       `json:"version"`
	Facts       []flowrecall.TemporalFact `json:"facts,omitempty"`
	Evidence    []evidenceRecord          `json:"evidence,omitempty"`
	Async       []asyncRecord             `json:"async_semantic,omitempty"`
	SideEffects []sideEffectRecord        `json:"side_effects,omitempty"`
	Counters    map[string]counterPair    `json:"counters,omitempty"`
}

type evidenceRecord struct {
	Scope      flowrecall.Scope       `json:"scope"`
	FactID     string                 `json:"fact_id"`
	EvidenceID string                 `json:"evidence_id"`
	Ordinal    int                    `json:"ordinal"`
	Ref        flowrecall.EvidenceRef `json:"ref"`
}

type asyncRecord struct {
	Job        flowrecall.AsyncSemanticJob     `json:"job"`
	Status     string                          `json:"status"`
	EnqueuedAt time.Time                       `json:"enqueued_at"`
	RetryAt    time.Time                       `json:"retry_at"`
	Failure    flowrecall.AsyncSemanticFailure `json:"failure"`
	Result     flowrecall.AsyncSemanticResult  `json:"result"`
}

type sideEffectRecord struct {
	Job        flowrecall.SideEffectJob     `json:"job"`
	Status     string                       `json:"status"`
	EnqueuedAt time.Time                    `json:"enqueued_at"`
	RetryAt    time.Time                    `json:"retry_at"`
	Failure    flowrecall.SideEffectFailure `json:"failure"`
	Result     flowrecall.SideEffectResult  `json:"result"`
}

type counterPair struct{ Async, SideEffect int }

type stateStore struct {
	client *redis.Client
	key    string
}

func newStateStore(client *redis.Client, prefix string) *stateStore {
	return &stateStore{client: client, key: prefix + ":canonical"}
}

func emptyState() durableState {
	return durableState{Version: stateVersion, Counters: map[string]counterPair{}}
}

func decodeState(raw string) (durableState, error) {
	if raw == "" {
		return emptyState(), nil
	}
	var state durableState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return durableState{}, err
	}
	if state.Version != stateVersion {
		return durableState{}, fmt.Errorf("unsupported state version %d", state.Version)
	}
	if state.Counters == nil {
		state.Counters = map[string]counterPair{}
	}
	return state, nil
}

func (store *stateStore) read(ctx context.Context) (durableState, error) {
	raw, err := store.client.Get(ctx, store.key).Result()
	if errors.Is(err, redis.Nil) {
		return emptyState(), nil
	}
	if err != nil {
		return durableState{}, err
	}
	return decodeState(raw)
}

func (store *stateStore) update(ctx context.Context, mutate func(*durableState) error) error {
	for range 32 {
		err := store.client.Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, store.key).Result()
			if errors.Is(err, redis.Nil) {
				raw, err = "", nil
			}
			if err != nil {
				return err
			}
			state, err := decodeState(raw)
			if err != nil {
				return err
			}
			if err := mutate(&state); err != nil {
				return err
			}
			encoded, err := json.Marshal(state)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error { pipe.Set(ctx, store.key, encoded, 0); return nil })
			return err
		}, store.key)
		if !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}
	return errors.New("flowcraft redis8: concurrent state update did not converge")
}

func sameScope(left, right flowrecall.Scope) bool { return left.PartitionKey() == right.PartitionKey() }
func sortFacts(facts []flowrecall.TemporalFact) {
	sort.SliceStable(facts, func(i, j int) bool {
		if facts[i].ObservedAt.Equal(facts[j].ObservedAt) {
			return facts[i].ID < facts[j].ID
		}
		return facts[i].ObservedAt.Before(facts[j].ObservedAt)
	})
}
func leaseToken() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprint(time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}
