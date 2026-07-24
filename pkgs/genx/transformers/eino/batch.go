package eino

import (
	"context"
	"fmt"
	"sync"
)

func compileBatch(
	ctx context.Context,
	config *normalizedConfig,
	node NodeDefinition,
	path string,
) (func(context.Context, *runState) (map[string]any, map[string]bool, error), error) {
	graph, err := buildGraph(ctx, config, node.Batch.Graph, path+"."+node.ID+".Batch")
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, parent *runState) (map[string]any, map[string]bool, error) {
		value, err := parent.binding(node.Batch.Items)
		if err != nil {
			return nil, nil, err
		}
		items, ok := value.([]any)
		if !ok {
			return nil, nil, fmt.Errorf("Batch Items must be a list")
		}
		if len(items) == 0 {
			return map[string]any{"items": []any{}}, nil, nil
		}
		batchCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		results := make([]any, len(items))
		sem := make(chan struct{}, node.Batch.MaxConcurrency)
		var wait sync.WaitGroup
		var firstErr error
		var errMu sync.Mutex
		for index, item := range items {
			wait.Add(1)
			go func(index int, item any) {
				defer wait.Done()
				select {
				case sem <- struct{}{}:
				case <-batchCtx.Done():
					return
				}
				defer func() { <-sem }()
				inputs := map[string]any{"item": item}
				capture := &captureEmitter{values: make(map[string]any)}
				child, childErr := newRunState(graph.fields, graphInputFromNodeInputs(inputs), inputs, capture)
				if childErr == nil {
					childErr = graph.execute(batchCtx, child)
				}
				if childErr != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("item %d: %w", index, childErr)
						cancel()
					}
					errMu.Unlock()
					return
				}
				primary, ok := capture.values[graph.primary.Name]
				if !ok {
					errMu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("item %d produced no primary output", index)
						cancel()
					}
					errMu.Unlock()
					return
				}
				results[index] = primary
			}(index, item)
		}
		wait.Wait()
		if firstErr != nil {
			return nil, nil, firstErr
		}
		return map[string]any{"items": results}, nil, nil
	}, nil
}
