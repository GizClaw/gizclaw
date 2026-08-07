package pendingdeletion

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type registryTestHandler struct {
	kind Kind
}

func (h *registryTestHandler) Kind() Kind { return h.kind }
func (h *registryTestHandler) Handle(context.Context, Claim) error {
	return nil
}

type registryStoreWithoutCompare struct {
	kv.Store
}

func (s registryStoreWithoutCompare) CreateIfAbsent(ctx context.Context, guard kv.Entry, entries []kv.Entry) ([]byte, bool, error) {
	return kv.CreateIfAbsent(ctx, s.Store, guard, entries)
}

func TestRegistryRejectsInvalidRegistrations(t *testing.T) {
	validSource := func(name string, kinds ...Kind) KVSource {
		return KVSource{Store: kv.NewMemory(nil), SourceName: name, OwnedKinds: kinds}
	}
	for _, test := range []struct {
		name     string
		source   Source
		handlers []Handler
	}{
		{name: "typed nil source", source: (*KVSource)(nil)},
		{name: "invalid source name", source: validSource("Gameplay", KindPet), handlers: []Handler{&registryTestHandler{kind: KindPet}}},
		{name: "no kinds", source: validSource("empty")},
		{name: "duplicate kind", source: validSource("duplicate", KindPet, KindPet), handlers: []Handler{&registryTestHandler{kind: KindPet}}},
		{name: "missing handler", source: validSource("missing", KindPet)},
		{name: "typed nil handler", source: validSource("nil_handler", KindPet), handlers: []Handler{(*registryTestHandler)(nil)}},
		{name: "unadvertised handler", source: validSource("wrong", KindPet), handlers: []Handler{&registryTestHandler{kind: KindPeer}}},
		{name: "duplicate handler", source: validSource("handlers", KindPet), handlers: []Handler{&registryTestHandler{kind: KindPet}, &registryTestHandler{kind: KindPet}}},
		{name: "missing compare and mutate", source: KVSource{
			Store: registryStoreWithoutCompare{Store: kv.NewMemory(nil)}, SourceName: "peer", OwnedKinds: []Kind{KindPeer},
		}, handlers: []Handler{&registryTestHandler{kind: KindPeer}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := NewRegistry().Register(test.source, test.handlers...); err == nil {
				t.Fatal("Register() error = nil")
			}
		})
	}
}

func TestRegistryRejectsDuplicateSource(t *testing.T) {
	registry := NewRegistry()
	source := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	if err := registry.Register(source, &registryTestHandler{kind: KindPeer}); err != nil {
		t.Fatalf("Register(first) error = %v", err)
	}
	if err := registry.Register(source, &registryTestHandler{kind: KindPeer}); err == nil {
		t.Fatal("Register(second) error = nil")
	}
}
