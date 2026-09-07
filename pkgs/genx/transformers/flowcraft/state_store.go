package flowcraft

import "context"

// StateStore persists serializable Board variables for one caller-owned scope.
// LoadState returns nil without an error when no checkpoint exists.
// The caller owns the store and its lifecycle.
type StateStore interface {
	LoadState(context.Context, string) ([]byte, error)
	SaveState(context.Context, string, []byte) error
}
