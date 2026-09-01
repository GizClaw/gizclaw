package redis8

import (
	"context"
	"fmt"
	"sort"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/sdk/errdefs"
)

type evidenceStore struct{ state *stateStore }

func (store *evidenceStore) Append(ctx context.Context, scope flowrecall.Scope, factID string, refs []flowrecall.EvidenceRef) error {
	if scope.RuntimeID == "" {
		return errdefs.Validationf("flowcraft redis8 evidence: scope.runtime_id is required")
	}
	if factID == "" {
		return errdefs.Validationf("flowcraft redis8 evidence: fact id is required")
	}
	if len(refs) == 0 {
		return nil
	}
	return store.state.update(ctx, func(state *durableState) error {
		for i, ref := range refs {
			if ref.ID == "" {
				ref.ID = fmt.Sprintf("%s#%d", factID, i)
			}
			found := -1
			ordinal := 0
			for j, row := range state.Evidence {
				if sameScope(row.Scope, scope) && row.FactID == factID {
					if row.Ordinal >= ordinal {
						ordinal = row.Ordinal + 1
					}
					if row.EvidenceID == ref.ID {
						found = j
					}
				}
			}
			row := evidenceRecord{Scope: scope, FactID: factID, EvidenceID: ref.ID, Ordinal: ordinal, Ref: ref}
			if found >= 0 {
				row.Ordinal = state.Evidence[found].Ordinal
				state.Evidence[found] = row
			} else {
				state.Evidence = append(state.Evidence, row)
			}
		}
		return nil
	})
}
func (store *evidenceStore) Get(ctx context.Context, scope flowrecall.Scope, id string) (flowrecall.EvidenceRef, error) {
	state, err := store.state.read(ctx)
	if err != nil {
		return flowrecall.EvidenceRef{}, err
	}
	for _, row := range state.Evidence {
		if sameScope(row.Scope, scope) && row.EvidenceID == id {
			return row.Ref, nil
		}
	}
	return flowrecall.EvidenceRef{}, flowrecall.ErrStoreNotFound
}
func (store *evidenceStore) ListByFact(ctx context.Context, scope flowrecall.Scope, factID string) ([]flowrecall.EvidenceRef, error) {
	if factID == "" {
		return nil, nil
	}
	state, err := store.state.read(ctx)
	if err != nil {
		return nil, err
	}
	var rows []evidenceRecord
	for _, row := range state.Evidence {
		if sameScope(row.Scope, scope) && row.FactID == factID {
			rows = append(rows, row)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Ordinal < rows[j].Ordinal })
	out := make([]flowrecall.EvidenceRef, len(rows))
	for i, row := range rows {
		out[i] = row.Ref
	}
	return out, nil
}
func (store *evidenceStore) ListFactIDs(ctx context.Context, scope flowrecall.Scope) ([]string, error) {
	state, err := store.state.read(ctx)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, row := range state.Evidence {
		if sameScope(row.Scope, scope) {
			set[row.FactID] = true
		}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}
func (store *evidenceStore) ForgetByFact(ctx context.Context, scope flowrecall.Scope, factIDs []string) error {
	targets := map[string]bool{}
	for _, id := range factIDs {
		targets[id] = true
	}
	return store.state.update(ctx, func(state *durableState) error {
		out := state.Evidence[:0]
		for _, row := range state.Evidence {
			if sameScope(row.Scope, scope) && targets[row.FactID] {
				continue
			}
			out = append(out, row)
		}
		state.Evidence = out
		return nil
	})
}
func (store *evidenceStore) Close() error { return nil }

var _ flowrecall.EvidenceStore = (*evidenceStore)(nil)
