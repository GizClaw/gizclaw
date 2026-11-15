// Package kvfixture provides borrowed KV views for deterministic replica-lag tests.
package kvfixture

import (
	"context"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// ReadView sends ordinary reads to replica and explicit primary reads to authority.
// Writes delegate to authority. Each optional counter counts read method calls.
// The view borrows both stores; Close does not close either one. Optional atomic
// capabilities can be supplied by an outer fixture when a test needs them.
func ReadView(authority, replica kv.Store, ordinary, primary *atomic.Int32) kv.Store {
	return &readView{Store: authority, replica: replica, ordinary: ordinary, primary: primary}
}

type readView struct {
	kv.Store
	replica  kv.Store
	ordinary *atomic.Int32
	primary  *atomic.Int32
}

func (s *readView) reader(ctx context.Context) kv.Store {
	// WithPrimaryReads is idempotent. This test-only probe avoids exposing the
	// production context marker just to select a deterministic read fixture.
	if kv.WithPrimaryReads(ctx) == ctx {
		if s.primary != nil {
			s.primary.Add(1)
		}
		return s.Store
	}
	if s.ordinary != nil {
		s.ordinary.Add(1)
	}
	return s.replica
}

func (s *readView) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	return s.reader(ctx).Get(ctx, key)
}

func (s *readView) BatchGet(ctx context.Context, keys []kv.Key) []kv.GetResult {
	return s.reader(ctx).BatchGet(ctx, keys)
}

func (s *readView) HasMember(ctx context.Context, key kv.Key, member string) (bool, error) {
	return s.reader(ctx).HasMember(ctx, key, member)
}

func (s *readView) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	return s.reader(ctx).ListMembers(ctx, key)
}

func (s *readView) RangeOrderedMembers(ctx context.Context, key kv.Key, query kv.OrderedRange) ([]string, error) {
	return s.reader(ctx).RangeOrderedMembers(ctx, key, query)
}

func (*readView) Close() error { return nil }
