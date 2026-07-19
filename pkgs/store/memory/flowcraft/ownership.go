package flowcraft

import (
	"context"

	"github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/memory/retrieval"
)

type nonClosingTemporalStore struct{ recall.TemporalStore }

func (nonClosingTemporalStore) Close() error { return nil }

func (s nonClosingTemporalStore) ListScopes(ctx context.Context, query recall.ScopeListQuery) ([]recall.Scope, error) {
	if enumerator, ok := s.TemporalStore.(recall.ScopeEnumerator); ok {
		return enumerator.ListScopes(ctx, query)
	}
	return nil, nil
}

type nonClosingEvidenceStore struct{ recall.EvidenceStore }

func (nonClosingEvidenceStore) Close() error { return nil }

type nonClosingSideEffectOutbox struct{ recall.SideEffectOutbox }

func (nonClosingSideEffectOutbox) Close() error { return nil }

type nonClosingRetrievalIndex struct{ retrieval.Index }

func (nonClosingRetrievalIndex) Close() error { return nil }
