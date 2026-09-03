package giztest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func parallelDocument(steps []Step) *Document {
	return &Document{
		Name: "parallel", Path: "parallel.yaml", Repeat: 1,
		Variables: map[string]VariableSpec{
			"received": {Direction: "output", Type: "integer"},
		},
		Clients: map[string]ClientSpec{"peer": {}},
		Steps:   steps,
	}
}

func listenChild(id string, duration string) Step {
	return Step{ID: id, Client: "peer", PeerStream: &PeerStreamOperation{Mode: "listen", Duration: duration}}
}

// parallelResultDriver prepares every child into a run phase that waits for
// the given delay, honoring cancellation, and then reports the child's own
// result. It records the parent every prepare saw and how many children were
// running at the same moment.
func parallelResultDriver(delay time.Duration, results map[string]StepResult, errs map[string]error) (*stubDriver, *atomic.Value, *atomic.Int64) {
	var parent atomic.Value
	var peak atomic.Int64
	var running atomic.Int64
	driver := &stubDriver{}
	driver.prepareParallel = func(req StepRequest) (ParallelChild, error) {
		if req.Parent != nil {
			parent.Store(req.Parent.ID)
		}
		id := req.Step.ID
		return parallelRun(func(ctx context.Context) (StepResult, error) {
			for current := running.Add(1); ; {
				top := peak.Load()
				if current <= top || peak.CompareAndSwap(top, current) {
					break
				}
			}
			defer running.Add(-1)
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
				return results[id], errs[id]
			case <-ctx.Done():
				return StepResult{}, context.Cause(ctx)
			}
		}), nil
	}
	return driver, &parent, &peak
}

// TestRunTaskParallelRunsChildrenTogetherAndKeysTheResult covers the contract
// a broadcast scenario relies on: children run at the same moment, and the
// step value is one object keyed by child id, so capture and expect address a
// child by JSON pointer.
func TestRunTaskParallelRunsChildrenTogetherAndKeysTheResult(t *testing.T) {
	driver, parent, peak := parallelResultDriver(50*time.Millisecond, map[string]StepResult{
		"listen": {Value: map[string]any{"audio_bytes": 42, "packets": 3}, Evidence: map[string]any{"packets": 3}},
		"speak":  {Value: map[string]any{"input_sent": true}, Evidence: map[string]any{"input_packets": 9}},
	}, nil)
	packets := 3
	doc := parallelDocument([]Step{{
		ID: "talk", Timeout: "5s",
		Parallel: []Step{listenChild("listen", "300ms"), {ID: "speak", Client: "peer", PeerStream: &PeerStreamOperation{Mode: "push-to-talk", Input: "x", Completion: "input_sent"}}},
		Capture:  map[string]string{"received": "/listen/audio_bytes"},
		Expect: map[string]Expectation{
			"/listen/packets":   {Equals: packets},
			"/speak/input_sent": {Equals: true},
		},
	}})
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: io.Discard})
	if result.Status != "passed" || len(result.Steps) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := peak.Load(); got != 2 {
		t.Fatalf("children running at once = %d, want 2", got)
	}
	if got, _ := parent.Load().(string); got != "talk" {
		t.Fatalf("prepare saw parent %q, want the parallel step", got)
	}
	step := result.Steps[0]
	if step.Operation != "parallel" || step.Status != "passed" || step.Client != "" || step.Evidence["parallel"] != 2 {
		t.Fatalf("parallel step report = %#v", step)
	}
	if len(step.Children) != 2 {
		t.Fatalf("child reports = %#v", step.Children)
	}
	for i, want := range []string{"listen", "speak"} {
		child := step.Children[i]
		if child.ID != want || child.Status != "passed" || child.Operation != "peer_stream" || child.Client != "peer" || child.Stage != "parallel" {
			t.Fatalf("child report %d = %#v", i, child)
		}
	}
	if step.Children[0].Evidence["packets"] != 3 || step.Children[1].Evidence["input_packets"] != 9 {
		t.Fatalf("child evidence = %#v", step.Children)
	}
	if driver.streamed != 1 || driver.closed != 1 {
		t.Fatalf("session teardown streamed/closed = %d/%d, want 1/1", driver.streamed, driver.closed)
	}
}

// One failing child fails the step, and every child's own outcome stays in
// the report next to it.
func TestRunTaskParallelReportsEveryChildOutcome(t *testing.T) {
	driver, _, _ := parallelResultDriver(0, map[string]StepResult{
		"heard": {Value: map[string]any{"audio_bytes": 4096}},
	}, map[string]error{
		"silent": errors.New("peer_stream closed before the listen duration elapsed"),
	})
	doc := parallelDocument([]Step{{
		ID: "talk", Timeout: "5s",
		Parallel: []Step{listenChild("heard", "1s"), listenChild("silent", "1s")},
	}})
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: io.Discard})
	if result.Status != "failed" || len(result.Steps) != 1 {
		t.Fatalf("result = %#v", result)
	}
	step := result.Steps[0]
	if step.Status != "failed" || !strings.Contains(step.Error, "closed before the listen duration") {
		t.Fatalf("parallel step report = %#v", step)
	}
	if step.Children[0].Status != "passed" || step.Children[1].Status != "failed" ||
		!strings.Contains(step.Children[1].Error, "closed before the listen duration") {
		t.Fatalf("child reports = %#v", step.Children)
	}
	if driver.streamed != 1 || driver.closed != 1 {
		t.Fatalf("session teardown streamed/closed = %d/%d, want 1/1", driver.streamed, driver.closed)
	}
}

// The step timeout bounds the whole group: every child is cancelled and the
// task continues to its finalizers.
func TestRunTaskParallelTimeoutCancelsEveryChild(t *testing.T) {
	driver, _, _ := parallelResultDriver(time.Minute, nil, nil)
	var output bytes.Buffer
	doc := parallelDocument([]Step{{
		ID: "talk", Timeout: "100ms",
		Parallel: []Step{listenChild("first", "1m"), listenChild("second", "1m")},
	}})
	doc.Variables["evidence"] = VariableSpec{Direction: "input", Type: "string", Value: "cleanup-ran"}
	doc.Finally = []Step{{ID: "cleanup", Output: &OutputOperation{Variable: "evidence"}}}
	started := time.Now()
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: &output})
	if time.Since(started) > 10*time.Second {
		t.Fatal("timed-out parallel step waited for the full child duration")
	}
	if result.Status != "failed" || len(result.Steps) != 1 {
		t.Fatalf("result = %#v", result)
	}
	step := result.Steps[0]
	if step.Status != "failed" || !strings.Contains(step.Error, "cancelled its children before they finished") {
		t.Fatalf("parallel step report = %#v", step)
	}
	for _, child := range step.Children {
		if child.Status != "failed" || !strings.Contains(child.Error, "context deadline exceeded") {
			t.Fatalf("child report = %#v", child)
		}
	}
	if len(result.Cleanup) != 1 || output.String() != "evidence=cleanup-ran\n" {
		t.Fatalf("finalizers did not run after the children were cancelled: %#v %q", result.Cleanup, output.String())
	}
}

func TestRunTaskRejectsParallelStepWithoutParallelSession(t *testing.T) {
	doc := parallelDocument([]Step{{
		ID: "talk", Parallel: []Step{listenChild("first", "1s"), listenChild("second", "1s")},
	}})
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: &stubDriver{}, Out: io.Discard})
	if result.Status != "failed" || len(result.Steps) != 1 || !strings.Contains(result.Steps[0].Error, "cannot run parallel steps") {
		t.Fatalf("result = %#v", result)
	}
}

// A prepare failure stops the step before any child starts, because prepare
// is the phase that reads the task's Variables.
func TestRunTaskReportsParallelPrepareFailure(t *testing.T) {
	var started atomic.Int64
	driver := &stubDriver{prepareParallel: func(req StepRequest) (ParallelChild, error) {
		if req.Step.ID == "second" {
			return nil, errors.New("parallel children cannot play audio interactively")
		}
		return parallelRun(func(context.Context) (StepResult, error) {
			started.Add(1)
			return StepResult{}, nil
		}), nil
	}}
	doc := parallelDocument([]Step{{
		ID: "talk", Parallel: []Step{listenChild("first", "1s"), listenChild("second", "1s")},
	}})
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: io.Discard})
	if result.Status != "failed" || len(result.Steps) != 1 || !strings.Contains(result.Steps[0].Error, "play audio") {
		t.Fatalf("result = %#v", result)
	}
	if started.Load() != 0 {
		t.Fatal("a child started even though a sibling failed to prepare")
	}
}

// stuckParallelDriver prepares run phases that ignore cancellation until
// release is closed, modeling a stream that does not honor its context: every
// wait on a child goroutine must be bounded.
func stuckParallelDriver(release <-chan struct{}) *stubDriver {
	return &stubDriver{prepareParallel: func(StepRequest) (ParallelChild, error) {
		return parallelRun(func(context.Context) (StepResult, error) {
			<-release
			return StepResult{}, context.Canceled
		}), nil
	}}
}

// A child goroutine that still owns its stream also owns the session's
// clients, so finalizers must not run and the session must not be torn down
// underneath it.
func TestRunTaskSkipsFinalizersWhileParallelChildStillOwnsStream(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	driver := stuckParallelDriver(release)
	var output bytes.Buffer
	doc := parallelDocument([]Step{{
		ID: "talk", Timeout: "100ms",
		Parallel: []Step{listenChild("first", "1m"), listenChild("second", "1m")},
	}})
	doc.Variables["evidence"] = VariableSpec{Direction: "input", Type: "string", Value: "cleanup-ran"}
	doc.Finally = []Step{{ID: "cleanup", Output: &OutputOperation{Variable: "evidence"}}}
	started := time.Now()
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: &output, parallelCancelGrace: 100 * time.Millisecond})
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("task waited %s for children that ignore cancellation", elapsed)
	}
	if result.Status != "failed" {
		t.Fatalf("result = %#v", result)
	}
	step := result.Steps[0]
	if !strings.Contains(step.Error, "did not stop after cancellation") {
		t.Fatalf("parallel step report = %#v", step)
	}
	for _, child := range step.Children {
		if child.Status != "failed" || child.Evidence["unfinished"] != true {
			t.Fatalf("child report = %#v", child)
		}
	}
	if ids, ok := step.Evidence["unfinished"].([]string); !ok || !slices.Equal(ids, []string{"first", "second"}) {
		t.Fatalf("step evidence = %#v", step.Evidence)
	}
	if len(result.Cleanup) != 0 || output.Len() != 0 {
		t.Fatalf("finally steps ran while a stream was still owned: %#v %q", result.Cleanup, output.String())
	}
	if !strings.Contains(result.Error, "still hold their PeerStream") {
		t.Fatalf("result error = %q, want the retained stream report", result.Error)
	}
	if driver.streamed != 0 || driver.closed != 0 {
		t.Fatalf("session torn down under a running child: streamed/closed = %d/%d", driver.streamed, driver.closed)
	}
}

func TestParallelChildCaptures(t *testing.T) {
	t.Parallel()
	parent := Step{ID: "talk", Capture: map[string]string{
		"carol_audio": "/carol_hear/audio",
		"whole":       "/carol_hear",
		"other":       "/bob_speak/input_packets",
	}}
	got := ParallelChildCaptures(parent, "carol_hear")
	if len(got) != 2 || got["carol_audio"] != "/audio" || got["whole"] != "" {
		t.Fatalf("ParallelChildCaptures() = %#v", got)
	}
}
