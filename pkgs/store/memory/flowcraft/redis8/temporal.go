package redis8

import (
	"context"
	"sort"
	"time"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/sdk/errdefs"
)

type temporalStore struct{ state *stateStore }

func (store *temporalStore) Append(ctx context.Context, facts []flowrecall.TemporalFact) error {
	if len(facts) == 0 {
		return nil
	}
	return store.state.update(ctx, func(state *durableState) error {
		seen := map[string]struct{}{}
		for _, fact := range facts {
			if fact.ID == "" {
				return errdefs.Validationf("flowcraft redis8 temporal: fact id is required")
			}
			if !fact.Kind.IsValid() {
				return errdefs.Validationf("flowcraft redis8 temporal: invalid fact kind %q", fact.Kind)
			}
			if fact.Scope.RuntimeID == "" {
				return errdefs.Validationf("flowcraft redis8 temporal: scope.runtime_id is required")
			}
			key := fact.Scope.PartitionKey() + "\x00" + fact.ID
			if _, ok := seen[key]; ok {
				return errdefs.Conflictf("flowcraft redis8 temporal: duplicate fact id %q", fact.ID)
			}
			seen[key] = struct{}{}
			for _, existing := range state.Facts {
				if sameScope(existing.Scope, fact.Scope) && existing.ID == fact.ID {
					return errdefs.Conflictf("flowcraft redis8 temporal: duplicate fact id %q", fact.ID)
				}
			}
		}
		for _, fact := range facts {
			state.Facts = append(state.Facts, fact.Clone())
		}
		return nil
	})
}

func (store *temporalStore) Get(ctx context.Context, scope flowrecall.Scope, id string) (flowrecall.TemporalFact, error) {
	state, err := store.state.read(ctx)
	if err != nil {
		return flowrecall.TemporalFact{}, err
	}
	for _, fact := range state.Facts {
		if sameScope(fact.Scope, scope) && fact.ID == id {
			return fact.Clone(), nil
		}
	}
	return flowrecall.TemporalFact{}, flowrecall.ErrStoreNotFound
}

func (store *temporalStore) List(ctx context.Context, scope flowrecall.Scope, query flowrecall.ListQuery) ([]flowrecall.TemporalFact, error) {
	state, err := store.state.read(ctx)
	if err != nil {
		return nil, err
	}
	kinds := map[flowrecall.FactKind]bool{}
	for _, kind := range query.Kinds {
		kinds[kind] = true
	}
	var out []flowrecall.TemporalFact
	for _, fact := range state.Facts {
		if !sameScope(fact.Scope, scope) || (!query.IncludeSuperseded && fact.CorrectedBy != "") || (len(kinds) > 0 && !kinds[fact.Kind]) || !containsAllStrings(fact.Entities, query.Entities) {
			continue
		}
		out = append(out, fact.Clone())
	}
	sortFacts(out)
	if query.Limit > 0 && len(out) > query.Limit {
		out = out[:query.Limit]
	}
	return out, nil
}

func containsAllStrings(have, want []string) bool {
	set := map[string]bool{}
	for _, value := range have {
		set[value] = true
	}
	for _, value := range want {
		if !set[value] {
			return false
		}
	}
	return true
}

func (store *temporalStore) find(ctx context.Context, scope flowrecall.Scope, match func(flowrecall.TemporalFact) bool) ([]flowrecall.TemporalFact, error) {
	state, err := store.state.read(ctx)
	if err != nil {
		return nil, err
	}
	var out []flowrecall.TemporalFact
	for _, fact := range state.Facts {
		if sameScope(fact.Scope, scope) && match(fact) {
			out = append(out, fact.Clone())
		}
	}
	sortFacts(out)
	return out, nil
}
func (store *temporalStore) FindByMergeKey(ctx context.Context, scope flowrecall.Scope, key string) ([]flowrecall.TemporalFact, error) {
	if key == "" {
		return nil, nil
	}
	return store.find(ctx, scope, func(f flowrecall.TemporalFact) bool { return f.MergeKey == key })
}
func (store *temporalStore) FindSupersededBy(ctx context.Context, scope flowrecall.Scope, id string) ([]flowrecall.TemporalFact, error) {
	if id == "" {
		return nil, nil
	}
	return store.find(ctx, scope, func(f flowrecall.TemporalFact) bool { return f.CorrectedBy == id })
}
func (store *temporalStore) FindByOriginRequestID(ctx context.Context, scope flowrecall.Scope, id string) ([]flowrecall.TemporalFact, error) {
	if id == "" {
		return nil, nil
	}
	return store.find(ctx, scope, func(f flowrecall.TemporalFact) bool { return f.Origin.RequestID == id })
}
func (store *temporalStore) FindByRevisionSource(ctx context.Context, scope flowrecall.Scope, id string) ([]flowrecall.TemporalFact, error) {
	if id == "" {
		return nil, nil
	}
	return store.find(ctx, scope, func(f flowrecall.TemporalFact) bool {
		fork, _ := f.Metadata["fork_of"].(string)
		contest, _ := f.Metadata["contest_of"].(string)
		return fork == id || contest == id
	})
}

func (store *temporalStore) mutateFact(ctx context.Context, scope flowrecall.Scope, id string, mutate func(*flowrecall.TemporalFact) error) error {
	return store.state.update(ctx, func(state *durableState) error {
		for i := range state.Facts {
			if sameScope(state.Facts[i].Scope, scope) && state.Facts[i].ID == id {
				return mutate(&state.Facts[i])
			}
		}
		return flowrecall.ErrStoreNotFound
	})
}
func (store *temporalStore) UpdateValidity(ctx context.Context, scope flowrecall.Scope, id string, validTo time.Time, correctedBy string) error {
	return store.mutateFact(ctx, scope, id, func(f *flowrecall.TemporalFact) error {
		if f.ValidTo != nil {
			if f.ValidTo.Equal(validTo) && f.CorrectedBy == correctedBy {
				return nil
			}
			return flowrecall.ErrTemporalValidityAlreadyClosed
		}
		value := validTo
		f.ValidTo = &value
		f.CorrectedBy = correctedBy
		return nil
	})
}
func (store *temporalStore) ReopenValidity(ctx context.Context, scope flowrecall.Scope, id, expected string) error {
	return store.mutateFact(ctx, scope, id, func(f *flowrecall.TemporalFact) error {
		if f.ValidTo == nil && f.CorrectedBy == "" {
			return nil
		}
		if f.CorrectedBy != expected {
			return flowrecall.ErrTemporalReopenConflict
		}
		f.ValidTo = nil
		f.CorrectedBy = ""
		return nil
	})
}
func (store *temporalStore) UpdateFeedback(ctx context.Context, scope flowrecall.Scope, id string, r, p float64) error {
	return store.mutateFact(ctx, scope, id, func(f *flowrecall.TemporalFact) error {
		f.Reinforcement += r
		if f.Reinforcement < 0 {
			f.Reinforcement = 0
		}
		f.Penalty += p
		if f.Penalty < 0 {
			f.Penalty = 0
		}
		return nil
	})
}
func (store *temporalStore) MarkClosed(ctx context.Context, scope flowrecall.Scope, id string, closed bool) error {
	return store.mutateFact(ctx, scope, id, func(f *flowrecall.TemporalFact) error { f.Closed = closed; return nil })
}

func (store *temporalStore) Delete(ctx context.Context, scope flowrecall.Scope, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	targets := map[string]bool{}
	for _, id := range ids {
		targets[id] = true
	}
	return store.state.update(ctx, func(state *durableState) error {
		out := state.Facts[:0]
		for _, f := range state.Facts {
			if sameScope(f.Scope, scope) && targets[f.ID] {
				continue
			}
			out = append(out, f)
		}
		state.Facts = out
		return nil
	})
}
func (store *temporalStore) DeleteByScope(ctx context.Context, scope flowrecall.Scope) (int, error) {
	count := 0
	err := store.state.update(ctx, func(state *durableState) error {
		out := state.Facts[:0]
		for _, f := range state.Facts {
			if sameScope(f.Scope, scope) {
				count++
				continue
			}
			out = append(out, f)
		}
		state.Facts = out
		return nil
	})
	return count, err
}
func (store *temporalStore) ListScopes(ctx context.Context, query flowrecall.ScopeListQuery) ([]flowrecall.Scope, error) {
	state, err := store.state.read(ctx)
	if err != nil {
		return nil, err
	}
	unique := map[string]flowrecall.Scope{}
	for _, f := range state.Facts {
		if query.RuntimeID == "" || f.Scope.RuntimeID == query.RuntimeID {
			// Flowcraft defines RuntimeID and UserID as the durable hard
			// partition. AgentID and Federation are soft-isolation metadata,
			// so ScopeEnumerator must return the canonical hard scope.
			hardScope := flowrecall.Scope{RuntimeID: f.Scope.RuntimeID, UserID: f.Scope.UserID}
			unique[hardScope.PartitionKey()] = hardScope
		}
	}
	out := make([]flowrecall.Scope, 0, len(unique))
	for _, scope := range unique {
		out = append(out, scope)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RuntimeID == out[j].RuntimeID {
			return out[i].UserID < out[j].UserID
		}
		return out[i].RuntimeID < out[j].RuntimeID
	})
	return out, nil
}
func (store *temporalStore) ListByID(ctx context.Context, scope flowrecall.Scope, id string) ([]flowrecall.TemporalFact, error) {
	seed, err := store.Get(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	all, err := store.find(ctx, scope, func(flowrecall.TemporalFact) bool { return true })
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{seed.ID: true}
	changed := true
	for changed {
		changed = false
		for _, f := range all {
			if seen[f.ID] {
				for _, prior := range f.Supersedes {
					if prior == "" {
						continue
					}
					if !seen[prior] {
						seen[prior] = true
						changed = true
					}
				}
			}
			if f.CorrectedBy != "" && seen[f.CorrectedBy] && !seen[f.ID] {
				seen[f.ID] = true
				changed = true
			}
		}
	}
	var out []flowrecall.TemporalFact
	for _, f := range all {
		if seen[f.ID] {
			out = append(out, f)
		}
	}
	sortFacts(out)
	return out, nil
}
func (store *temporalStore) Close() error { return nil }

var _ flowrecall.TemporalStore = (*temporalStore)(nil)
var _ flowrecall.ScopeEnumerator = (*temporalStore)(nil)
