package eino

import (
	"context"
	"fmt"
	"sync"
)

type raceCandidate struct {
	index  int
	values map[string]any
	state  map[string]any
	err    error
}

type raceCapture struct {
	captureEmitter
	once    sync.Once
	index   int
	started chan<- int
}

func (capture *raceCapture) Emit(output OutputDefinition, value any) error {
	if err := capture.captureEmitter.Emit(output, value); err != nil {
		return err
	}
	capture.once.Do(func() {
		select {
		case capture.started <- capture.index:
		default:
		}
	})
	return nil
}

func compileRace(
	ctx context.Context,
	config *normalizedConfig,
	node NodeDefinition,
	path string,
) (func(context.Context, *runState) (map[string]any, map[string]bool, error), error) {
	graphs := make([]*compiledGraph, len(node.Race.Branches))
	for index, branch := range node.Race.Branches {
		graph, err := buildGraph(ctx, config, branch.Graph, fmt.Sprintf("%s.%s.Race[%s]", path, node.ID, branch.ID))
		if err != nil {
			return nil, err
		}
		graphs[index] = graph
	}
	return func(ctx context.Context, parent *runState) (map[string]any, map[string]bool, error) {
		inputs, err := parent.nodeInputs(node.Inputs)
		if err != nil {
			return nil, nil, err
		}
		raceCtx, cancelAll := context.WithCancel(ctx)
		defer cancelAll()
		results := make(chan raceCandidate, len(graphs))
		started := make(chan int, len(graphs))
		sem := make(chan struct{}, node.Race.MaxConcurrency)
		branchContexts := make([]context.Context, len(graphs))
		branchCancels := make([]context.CancelFunc, len(graphs))
		for index := range graphs {
			branchContexts[index], branchCancels[index] = context.WithCancel(raceCtx)
		}
		cancelLosers := func(winner int) {
			for index, cancel := range branchCancels {
				if index != winner {
					cancel()
				}
			}
		}
		var wait sync.WaitGroup
		for index, graph := range graphs {
			wait.Add(1)
			go func(index int, graph *compiledGraph) {
				defer wait.Done()
				branchCtx := branchContexts[index]
				select {
				case sem <- struct{}{}:
				case <-branchCtx.Done():
					results <- raceCandidate{index: index, err: context.Cause(branchCtx)}
					return
				}
				defer func() { <-sem }()
				capture := &raceCapture{
					captureEmitter: captureEmitter{values: make(map[string]any)},
					index:          index, started: started,
				}
				childState, childErr := newRunState(graph.fields, graphInputFromNodeInputs(inputs), inputs, capture)
				if childErr == nil {
					childErr = graph.execute(branchCtx, childState)
				}
				var snapshot map[string]any
				if childErr == nil {
					snapshot, childErr = childState.snapshot()
				}
				results <- raceCandidate{
					index: index, values: capture.values, state: snapshot, err: childErr,
				}
			}(index, graph)
		}
		allDone := make(chan struct{})
		go func() {
			wait.Wait()
			close(allDone)
		}()

		firstOutputWinner := func() (int, error) {
			select {
			case first := <-started:
				winner := first
				for {
					select {
					case candidate := <-started:
						winner = min(winner, candidate)
					default:
						return winner, nil
					}
				}
			case <-allDone:
				select {
				case winner := <-started:
					return winner, nil
				default:
					return -1, nil
				}
			case <-ctx.Done():
				return -1, context.Cause(ctx)
			}
		}
		var winner int = -1
		switch node.Race.Winner.Mode {
		case RaceFirstOutput:
			winner, err = firstOutputWinner()
			if err != nil {
				return nil, nil, err
			}
			if winner >= 0 {
				cancelLosers(winner)
			}
		case RaceFirstSuccess, RacePredicate:
			// Selection happens from completed candidates below.
		default:
			return nil, nil, fmt.Errorf("unsupported Race winner mode %q", node.Race.Winner.Mode)
		}

		var winnerResult raceCandidate
		var failures []error
		remaining := len(graphs)
		for remaining > 0 {
			select {
			case candidate := <-results:
				remaining--
				if candidate.err != nil {
					failures = append(failures, candidate.err)
					continue
				}
				switch node.Race.Winner.Mode {
				case RaceFirstOutput:
					if candidate.index == winner {
						winnerResult = candidate
						remaining = 0
					}
				case RaceFirstSuccess:
					winnerResult = candidate
					winner = candidate.index
					cancelLosers(winner)
					remaining = 0
				case RacePredicate:
					if node.Race.Winner.When == nil {
						return nil, nil, fmt.Errorf("Race predicate winner requires When")
					}
					matched, matchErr := evaluatePredicate(*node.Race.Winner.When, candidate.state)
					if matchErr != nil {
						failures = append(failures, matchErr)
						continue
					}
					if matched {
						winnerResult = candidate
						winner = candidate.index
						cancelLosers(winner)
						remaining = 0
					}
				}
			case <-ctx.Done():
				cancelAll()
				<-allDone
				return nil, nil, context.Cause(ctx)
			}
		}
		cancelAll()
		<-allDone
		if winner < 0 || winnerResult.values == nil {
			return nil, nil, fmt.Errorf("Race has no winner: %v", failures)
		}
		return winnerResult.values, nil, nil
	}, nil
}
