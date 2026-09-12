// Package memory defines the provider-neutral long-term memory contract.
package memory

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidInput reports an invalid observation, query, update, or delete request.
	ErrInvalidInput = errors.New("memory: invalid input")
	// ErrNotFound reports that the selected fact or operation does not exist.
	ErrNotFound = errors.New("memory: not found")
	// ErrUnsupported reports behavior the selected provider cannot implement.
	ErrUnsupported = errors.New("memory: unsupported")
	// ErrConflict reports an optimistic-concurrency or revision conflict.
	ErrConflict = errors.New("memory: conflict")
	// ErrUnavailable reports that the configured provider cannot serve the request.
	ErrUnavailable = errors.New("memory: unavailable")
)

// Store persists observations as facts and recalls, updates, or deletes those
// facts. Implementations must be safe for concurrent use unless their
// constructor explicitly documents otherwise.
type Store interface {
	Observe(context.Context, Observation) (ObserveResult, error)
	Recall(context.Context, Query) (RecallResult, error)
	Update(context.Context, UpdateRequest) (Fact, error)
	Delete(context.Context, DeleteRequest) error
}

// DirectFactObserver is implemented by Stores that accept already-structured
// Observation.Facts without passing them through provider extraction.
type DirectFactObserver interface {
	SupportsDirectFactObservation() bool
}

// SupportsDirectFactObservation reports whether store accepts direct Facts.
// It unwraps borrowed scope views so capability checks remain stable after
// BindApp.
func SupportsDirectFactObservation(store Store) bool {
	for {
		bound, ok := store.(interface{ underlyingMemoryStore() Store })
		if !ok {
			break
		}
		store = bound.underlyingMemoryStore()
	}
	provider, ok := store.(DirectFactObserver)
	return ok && provider.SupportsDirectFactObservation()
}

// ScopePurger is implemented by Stores that can irreversibly remove one scope.
//
// The purge set is exactly the set of Facts that Recall with the same Scope
// can return, together with provider-owned records that could later
// materialize into that set (pending extraction jobs, derived indexes, and
// provenance markers). PurgeScope is idempotent. A provider whose native bulk
// deletion cannot express that set for a Scope returns ErrUnsupported instead
// of deleting more or less than the purge set.
//
// ScopeEmpty reports whether any Fact of the purge set remains. Providers
// whose deletion or extraction completes asynchronously can report false
// after a successful PurgeScope; callers that need a durable guarantee purge
// and re-check until ScopeEmpty reports true.
type ScopePurger interface {
	PurgeScope(context.Context, Scope) error
	ScopeEmpty(context.Context, Scope) (bool, error)
}

// PurgeScope irreversibly removes scope from store. It unwraps borrowed scope
// views such as BindApp and applies their scope binding before delegating, and
// returns ErrUnsupported when the underlying provider cannot purge a scope.
func PurgeScope(ctx context.Context, store Store, scope Scope) error {
	purger, scope, err := scopePurger(store, scope)
	if err != nil {
		return err
	}
	return purger.PurgeScope(ctx, scope)
}

// ScopeEmpty reports whether store still holds any Fact of scope's purge set.
// It unwraps borrowed scope views the same way as PurgeScope.
func ScopeEmpty(ctx context.Context, store Store, scope Scope) (bool, error) {
	purger, scope, err := scopePurger(store, scope)
	if err != nil {
		return false, err
	}
	return purger.ScopeEmpty(ctx, scope)
}

func scopePurger(store Store, scope Scope) (ScopePurger, Scope, error) {
	for {
		if nilInterface(store) {
			return nil, Scope{}, fmt.Errorf("%w: memory store is required", ErrInvalidInput)
		}
		view, ok := store.(interface{ appStoreView() *appStore })
		if !ok {
			break
		}
		bound := view.appStoreView()
		var err error
		if scope, err = bound.bindScope(scope); err != nil {
			return nil, Scope{}, err
		}
		store = bound.store
	}
	purger, ok := store.(ScopePurger)
	if !ok {
		return nil, Scope{}, fmt.Errorf("%w: memory store cannot purge a scope", ErrUnsupported)
	}
	return purger, scope, nil
}

// OperationWaiter is implemented by stores whose Observe method can return a
// pending operation. Wait blocks until the operation reaches a terminal state
// or ctx is cancelled. The returned result is authoritative for the operation.
type OperationWaiter interface {
	Wait(context.Context, OperationRequest) (ObserveResult, error)
}

// AsyncOperationProcessor is implemented by caller-owned stores that can
// materialize a pending Observe operation without blocking the response that
// submitted it.
type AsyncOperationProcessor interface {
	ProcessAsync(context.Context, OperationRequest) (ObserveResult, error)
}

// ProjectionRebuilder rehydrates provider-derived indexes from canonical Facts
// without changing the durable scope identity.
type ProjectionRebuilder interface {
	Rebuild(context.Context, Scope) error
}

// Statistics summarizes one product-owned memory scope when a provider can
// enumerate its materialized facts.
type Statistics struct {
	ItemCount     int64
	LastUpdatedAt time.Time
}

// StatisticsProvider is an optional capability for workspace status APIs.
type StatisticsProvider interface {
	Stats(context.Context, Scope) (Statistics, error)
}
