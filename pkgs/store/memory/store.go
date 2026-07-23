// Package memory defines the provider-neutral long-term memory contract.
package memory

import (
	"context"
	"errors"
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

// OperationWaiter is implemented by stores whose Observe method can return a
// pending operation. Wait blocks until the operation reaches a terminal state
// or ctx is cancelled. The returned result is authoritative for the operation.
type OperationWaiter interface {
	Wait(context.Context, string) (ObserveResult, error)
}

// AsyncOperationProcessor is implemented by caller-owned stores that can
// materialize a pending Observe operation without blocking the response that
// submitted it.
type AsyncOperationProcessor interface {
	ProcessAsync(context.Context, string) (ObserveResult, error)
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
