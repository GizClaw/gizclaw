package memory

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
)

const runtimeScopeAttribute = "gizclaw.runtime_scope"

type nativeScoper interface {
	scoped(string) Store
}

// Scoped returns a view that cannot observe or recall facts outside scope.
// Built-in providers use their native partition keys; custom stores receive a
// mandatory provider-neutral attribute and recall filter.
func Scoped(store Store, scope string) Store {
	if store == nil {
		return nil
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return store
	}
	if provider, ok := store.(nativeScoper); ok {
		return provider.scoped(scope)
	}
	view := &scopedStore{Store: store, scope: scope}
	if waiter, ok := store.(OperationWaiter); ok {
		return &scopedWaitStore{scopedStore: view, waiter: waiter}
	}
	return view
}

type scopedStore struct {
	Store
	scope string
}

func (s *scopedStore) Observe(ctx context.Context, observation Observation) (ObserveResult, error) {
	observation.Context = cloneMap(observation.Context)
	if observation.Context == nil {
		observation.Context = make(map[string]any)
	}
	observation.Context[runtimeScopeAttribute] = s.scope
	return s.Store.Observe(ctx, observation)
}

func (s *scopedStore) Recall(ctx context.Context, query Query) (RecallResult, error) {
	query.Filters = append(append([]Filter(nil), query.Filters...), Filter{
		Field: runtimeScopeAttribute, Operator: FilterEqual, Value: s.scope,
	})
	return s.Store.Recall(ctx, query)
}

func (s *scopedStore) Update(context.Context, UpdateRequest) (Fact, error) {
	return Fact{}, fmt.Errorf("%w: scoped mutations require provider-native isolation", ErrUnsupported)
}

func (s *scopedStore) Delete(context.Context, DeleteRequest) error {
	return fmt.Errorf("%w: scoped mutations require provider-native isolation", ErrUnsupported)
}

type scopedWaitStore struct {
	*scopedStore
	waiter OperationWaiter
}

func (s *scopedWaitStore) Wait(ctx context.Context, operationID string) (ObserveResult, error) {
	return s.waiter.Wait(ctx, operationID)
}

func scopedID(provider string, values ...string) string {
	var identity strings.Builder
	identity.WriteString(provider)
	for _, value := range values {
		fmt.Fprintf(&identity, "|%d:%s", len(value), value)
	}
	return fmt.Sprintf("gizclaw-%s-%x", provider, sha256.Sum256([]byte(identity.String())))
}
