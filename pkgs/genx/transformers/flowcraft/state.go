package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/flowcraft/sdk/engine"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func loadBoardState(ctx context.Context, store kv.Store, contextID string) (map[string]any, error) {
	if store == nil {
		return nil, nil
	}
	data, err := store.Get(ctx, kv.Key{contextID})
	if errors.Is(err, kv.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("flowcraft: load State: %w", err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("flowcraft: decode State: %w", err)
	}
	return state, nil
}

func saveBoardState(ctx context.Context, store kv.Store, contextID string, board *engine.Board) error {
	if store == nil || board == nil {
		return nil
	}
	vars, err := serializableBoardVariables(board)
	if err != nil {
		return err
	}
	data, err := json.Marshal(vars)
	if err != nil {
		return fmt.Errorf("flowcraft: encode State: %w", err)
	}
	if err := store.Set(ctx, kv.Key{contextID}, data); err != nil {
		return fmt.Errorf("flowcraft: save State: %w", err)
	}
	return nil
}

func serializableBoardVariables(board *engine.Board) (map[string]any, error) {
	return copiedBoardVariables(board, true)
}

func observationBoardVariables(board *engine.Board) (map[string]any, error) {
	if board == nil {
		return nil, nil
	}
	result := make(map[string]any)
	for key, value := range board.Vars() {
		lower := strings.ToLower(key)
		if internalBoardVariable(lower) || strings.HasPrefix(lower, "__") {
			continue
		}
		data, err := json.Marshal(value)
		if err != nil {
			// Observation builders consume a best-effort snapshot. One transient
			// runtime value must not prevent configured string board facts from
			// being observed.
			continue
		}
		var copied any
		if err := json.Unmarshal(data, &copied); err != nil {
			continue
		}
		result[key] = copied
	}
	return result, nil
}

func copiedBoardVariables(board *engine.Board, excludeTransient bool) (map[string]any, error) {
	if board == nil {
		return nil, nil
	}
	vars := board.Vars()
	for key := range vars {
		lower := strings.ToLower(key)
		if internalBoardVariable(lower) || strings.HasPrefix(lower, "__") || excludeTransient && strings.HasPrefix(lower, "tmp_") {
			delete(vars, key)
		}
	}
	data, err := json.Marshal(vars)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: copy Board variables: %w", err)
	}
	var copied map[string]any
	if err := json.Unmarshal(data, &copied); err != nil {
		return nil, fmt.Errorf("flowcraft: copy Board variables: %w", err)
	}
	return copied, nil
}

func internalBoardVariable(name string) bool {
	for _, prefix := range []string{"response", "usage", "tool"} {
		if name == prefix || strings.HasPrefix(name, prefix+".") || strings.HasPrefix(name, prefix+"_") {
			return true
		}
	}
	return false
}
