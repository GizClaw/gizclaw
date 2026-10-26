package giztestcmd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func historyWaitResult(entries ...map[string]any) map[string]any {
	items := make([]any, len(entries))
	for i, entry := range entries {
		items[i] = entry
	}
	return map[string]any{"items": items}
}

func TestHistoryWaitRequiresThisTurnsReply(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	old := map[string]any{"name": "earlier-reply", "type": "PEER_RUN_HISTORY_ENTRY_TYPE_AGENT"}
	user := map[string]any{"name": "current-input", "type": "PEER_RUN_HISTORY_ENTRY_TYPE_GEAR"}
	reply := map[string]any{"name": "current-reply", "type": "PEER_RUN_HISTORY_ENTRY_TYPE_AGENT"}
	before := workspaceHistoryNames(historyWaitResult(old))
	var phase atomic.Int32
	queried := make(chan int32, 10)
	query := func(context.Context) (map[string]any, error) {
		stage := phase.Load()
		queried <- stage
		switch stage {
		case 0:
			return historyWaitResult(old), nil
		case 1:
			return historyWaitResult(user, old), nil
		default:
			return historyWaitResult(reply, user, old), nil
		}
	}
	type result struct {
		name string
		err  error
	}
	finished := make(chan result, 1)
	go func() {
		name, err := waitForCurrentWorkspaceHistory(ctx, query, before, true)
		finished <- result{name, err}
	}()
	for stage := range int32(2) {
		phase.Store(stage)
		for observed := int32(-1); observed < stage; {
			select {
			case observed = <-queried:
			case <-ctx.Done():
				t.Fatal("History query did not progress")
			}
		}
		select {
		case got := <-finished:
			t.Fatalf("prior reply or current input satisfied reply persistence: %+v", got)
		case <-time.After(20 * time.Millisecond):
		}
	}
	phase.Store(2)
	select {
	case got := <-finished:
		if got.err != nil || got.name != "current-reply" {
			t.Fatalf("new reply receipt changed: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("current reply was not recognized")
	}
}

func TestHistoryWaitPropagatesQueryErrorAndCancellation(t *testing.T) {
	queryErr := errors.New("history unavailable")
	_, err := waitForCurrentWorkspaceHistory(t.Context(), func(context.Context) (map[string]any, error) {
		return nil, queryErr
	}, nil, true)
	if !errors.Is(err, queryErr) {
		t.Fatalf("query error hidden: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = waitForCurrentWorkspaceHistory(ctx, func(context.Context) (map[string]any, error) {
		return historyWaitResult(), nil
	}, nil, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait survived: %v", err)
	}
}

func TestCurrentHistoryReceiptAllowsInputOnlyTurns(t *testing.T) {
	before := workspaceHistoryNames(historyWaitResult(map[string]any{"name": "prior-input"}))
	result := historyWaitResult(map[string]any{"name": ""}, map[string]any{"name": "current-input", "type": "PEER_RUN_HISTORY_ENTRY_TYPE_GEAR"}, map[string]any{"name": "prior-input"})
	if name := firstCurrentHistoryName(result, before, false); name != "current-input" {
		t.Fatalf("input-only receipt changed: %q", name)
	}
	if name := firstCurrentHistoryName(result, before, true); name != "" {
		t.Fatalf("user input counted as an assistant reply: %q", name)
	}
}
