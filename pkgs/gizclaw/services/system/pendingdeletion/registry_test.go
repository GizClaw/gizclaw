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
		{name: "invalid source name", source: validSource("Social", KindFriendGroup), handlers: []Handler{&registryTestHandler{kind: KindFriendGroup}}},
		{name: "no kinds", source: validSource("empty")},
		{name: "duplicate kind", source: validSource("duplicate", KindFriendGroup, KindFriendGroup), handlers: []Handler{&registryTestHandler{kind: KindFriendGroup}}},
		{name: "missing handler", source: validSource("missing", KindFriendGroup)},
		{name: "typed nil handler", source: validSource("nil_handler", KindFriendGroup), handlers: []Handler{(*registryTestHandler)(nil)}},
		{name: "unadvertised handler", source: validSource("wrong", KindFriendGroup), handlers: []Handler{&registryTestHandler{kind: KindPeer}}},
		{name: "duplicate handler", source: validSource("handlers", KindFriendGroup), handlers: []Handler{&registryTestHandler{kind: KindFriendGroup}, &registryTestHandler{kind: KindFriendGroup}}},
		{name: "missing store", source: KVSource{
			SourceName: "peer", OwnedKinds: []Kind{KindPeer},
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
