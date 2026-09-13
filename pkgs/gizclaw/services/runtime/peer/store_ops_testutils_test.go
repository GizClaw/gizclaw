package peer

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func mustBadgerInMemory(t testing.TB, opts *kv.Options) kv.Store {
	t.Helper()
	store, err := kv.NewBadgerInMemory(opts)
	if err != nil {
		t.Fatalf("NewBadgerInMemory: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
