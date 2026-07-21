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
	if board == nil {
		return nil, nil
	}
	vars := board.Vars()
	for key := range vars {
		lower := strings.ToLower(key)
		if internalBoardVariable(lower) || strings.HasPrefix(lower, "tmp_") || strings.HasPrefix(lower, "__") {
			delete(vars, key)
		}
	}
	data, err := json.Marshal(vars)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: encode State: %w", err)
	}
	var copied map[string]any
	if err := json.Unmarshal(data, &copied); err != nil {
		return nil, fmt.Errorf("flowcraft: copy State: %w", err)
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
