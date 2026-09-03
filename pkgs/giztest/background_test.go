package giztest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func backgroundDocument(steps []Step) *Document {
	return &Document{
		Name: "background", Path: "background.yaml", Repeat: 1,
		Variables: map[string]VariableSpec{
			"received": {Direction: "output", Type: "integer"},
		},
		Clients: map[string]ClientSpec{"peer": {}},
		Steps:   steps,
	}
}

func listenStep(id, duration string) Step {
	return Step{ID: id, Client: "peer", Background: true, PeerStream: &PeerStreamOperation{Mode: "listen", Duration: duration}}
}

// backgroundResultDriver prepares every background step into a run phase
// that waits for the given delay, honoring cancellation, and then reports
// result. It records the awaiter each prepare saw.
func backgroundResultDriver(delay time.Duration, result StepResult, err error) (*stubDriver, *atomic.Value) {
	var awaiter atomic.Value
	driver := &stubDriver{}
	driver.prepareBackground = func(req StepRequest) (BackgroundStep, error) {
		if req.Awaiter != nil {
			awaiter.Store(req.Awaiter.ID)
		}
		return backgroundRun(func(ctx context.Context) (StepResult, error) {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
				return result, err
			case <-ctx.Done():
				return StepResult{}, context.Cause(ctx)
			}
		}), nil
	}
	return driver, &awaiter
}

func TestRunTaskAwaitAppliesBackgroundResult(t *testing.T) {
	driver, awaiter := backgroundResultDriver(50*time.Millisecond, StepResult{
		Value:    map[string]any{"audio_bytes": 42, "packets": 3},
		Evidence: map[string]any{"packets": 3},
	}, nil)
	packets := 3
	doc := backgroundDocument([]Step{
		listenStep("listen", "300ms"),
		{ID: "wait", Await: "listen", Timeout: "5s", Capture: map[string]string{"received": "/audio_bytes"}, Expect: map[string]Expectation{
			"/packets": {Equals: packets},
		}},
	})
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: io.Discard})
	if result.Status != "passed" || len(result.Steps) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if got, _ := awaiter.Load().(string); got != "wait" {
		t.Fatalf("prepare saw awaiter %q, want the await step", got)
	}
	started := result.Steps[0]
	if started.Status != "started" || started.Operation != "peer_stream" || started.Evidence["await"] != "wait" || started.Evidence["background"] != true {
		t.Fatalf("background step report = %#v", started)
	}
	awaited := result.Steps[1]
	if awaited.Status != "passed" || awaited.Operation != "await" || awaited.Client != "peer" || awaited.Evidence["step"] != "listen" || awaited.Evidence["packets"] != 3 {
		t.Fatalf("await step report = %#v", awaited)
	}
	if _, ok := awaited.Evidence["background_duration_ms"]; !ok {
		t.Fatalf("await evidence lacks background duration: %#v", awaited.Evidence)
	}
	if driver.streamed != 1 || driver.closed != 1 {
		t.Fatalf("session teardown streamed/closed = %d/%d, want 1/1", driver.streamed, driver.closed)
	}
}

func TestRunTaskAwaitSurfacesBackgroundFailure(t *testing.T) {
	driver, _ := backgroundResultDriver(0, StepResult{}, errors.New("peer_stream closed before the listen duration elapsed"))
	doc := backgroundDocument([]Step{
		listenStep("listen", "5s"),
		{ID: "wait", Await: "listen", Timeout: "5s"},
	})
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: io.Discard})
	if result.Status != "failed" || len(result.Steps) != 2 || result.Steps[0].Status != "started" {
		t.Fatalf("result = %#v", result)
	}
	awaited := result.Steps[1]
	if awaited.Status != "failed" || !strings.Contains(awaited.Error, "closed before the listen duration") || awaited.Evidence["step"] != "listen" {
		t.Fatalf("await step report = %#v", awaited)
	}
}

func TestRunTaskAwaitTimeoutCancelsBackgroundAndReportsUnawaited(t *testing.T) {
	driver, _ := backgroundResultDriver(time.Minute, StepResult{}, nil)
	doc := backgroundDocument([]Step{
		listenStep("first", "1m"),
		listenStep("second", "1m"),
		{ID: "wait_first", Await: "first", Timeout: "100ms"},
		{ID: "wait_second", Await: "second", Timeout: "5s"},
	})
	started := time.Now()
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: io.Discard})
	if time.Since(started) > 5*time.Second {
		t.Fatal("aborted task waited for the full background duration")
	}
	if result.Status != "failed" || len(result.Steps) != 4 {
		t.Fatalf("result = %#v", result)
	}
	awaited := result.Steps[2]
	if awaited.Status != "failed" || !strings.Contains(awaited.Error, "cancelled before it finished") || awaited.Evidence["deadline"] != "timeout" {
		t.Fatalf("await step report = %#v", awaited)
	}
	cancelled := result.Steps[3]
	if cancelled.ID != "second" || cancelled.Status != "cancelled" || cancelled.Stage != "background" || !strings.Contains(cancelled.Error, "cancelled before await step wait_second") {
		t.Fatalf("cancelled step report = %#v", cancelled)
	}
	if driver.streamed != 1 || driver.closed != 1 {
		t.Fatalf("session teardown streamed/closed = %d/%d, want 1/1", driver.streamed, driver.closed)
	}
}

func TestRunTaskRejectsBackgroundStepWithoutBackgroundSession(t *testing.T) {
	doc := backgroundDocument([]Step{
		listenStep("listen", "1s"),
		{ID: "wait", Await: "listen"},
	})
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: &stubDriver{}, Out: io.Discard})
	if result.Status != "failed" || len(result.Steps) != 1 || !strings.Contains(result.Steps[0].Error, "cannot run peer_stream steps in the background") {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunTaskReportsBackgroundPrepareFailure(t *testing.T) {
	driver := &stubDriver{prepareBackground: func(StepRequest) (BackgroundStep, error) {
		return nil, errors.New("background steps cannot play audio interactively")
	}}
	doc := backgroundDocument([]Step{
		listenStep("listen", "1s"),
		{ID: "wait", Await: "listen"},
	})
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: io.Discard})
	if result.Status != "failed" || len(result.Steps) != 1 || !strings.Contains(result.Steps[0].Error, "play audio") {
		t.Fatalf("result = %#v", result)
	}
}

// stuckBackgroundDriver prepares a run phase that ignores cancellation until
// release is closed, modeling a stream that does not honor its context: every
// wait on the background goroutine must be bounded.
func stuckBackgroundDriver(release <-chan struct{}) *stubDriver {
	return &stubDriver{prepareBackground: func(StepRequest) (BackgroundStep, error) {
		return backgroundRun(func(context.Context) (StepResult, error) {
			<-release
			return StepResult{}, context.Canceled
		}), nil
	}}
}

func TestRunTaskAwaitTimeoutIsBoundedWhenBackgroundIgnoresCancellation(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	doc := backgroundDocument([]Step{
		listenStep("listen", "1m"),
		{ID: "wait", Await: "listen", Timeout: "100ms"},
	})
	started := time.Now()
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: stuckBackgroundDriver(release), Out: io.Discard, backgroundCancelGrace: 100 * time.Millisecond})
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("await waited %s for a step that ignores cancellation", elapsed)
	}
	if result.Status != "failed" || len(result.Steps) != 2 {
		t.Fatalf("result = %#v", result)
	}
	awaited := result.Steps[1]
	if awaited.Status != "failed" || !strings.Contains(awaited.Error, "did not stop within") {
		t.Fatalf("await step report = %#v", awaited)
	}
	if awaited.Evidence["unfinished"] != true || awaited.Evidence["deadline"] != "timeout" {
		t.Fatalf("await evidence = %#v", awaited.Evidence)
	}
}

// A background goroutine that still owns its stream also owns the session's
// clients, so finalizers must not run and the session must not be torn down
// underneath it.
func TestRunTaskSkipsFinalizersWhileBackgroundStepStillOwnsStream(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	driver := stuckBackgroundDriver(release)
	var output bytes.Buffer
	doc := backgroundDocument([]Step{
		listenStep("listen", "1m"),
		{ID: "wait", Await: "listen", Timeout: "100ms"},
	})
	doc.Variables["evidence"] = VariableSpec{Direction: "input", Type: "string", Value: "cleanup-ran"}
	doc.Finally = []Step{{ID: "cleanup", Output: &OutputOperation{Variable: "evidence"}}}
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: &output, backgroundCancelGrace: 100 * time.Millisecond})
	if result.Status != "failed" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Cleanup) != 0 || output.Len() != 0 {
		t.Fatalf("finally steps ran while the stream was still owned: %#v %q", result.Cleanup, output.String())
	}
	if !strings.Contains(result.Error, "still hold their PeerStream") {
		t.Fatalf("result error = %q, want the retained stream report", result.Error)
	}
	if driver.streamed != 0 || driver.closed != 0 {
		t.Fatalf("session torn down under a running background step: streamed/closed = %d/%d", driver.streamed, driver.closed)
	}
}

func TestRunTaskCancelsUnawaitedBackgroundStepsBeforeFinalizers(t *testing.T) {
	driver, _ := backgroundResultDriver(time.Minute, StepResult{}, nil)
	driver.execute = func(context.Context, StepRequest) (StepResult, error) {
		return StepResult{}, errors.New("ping failed")
	}
	var output bytes.Buffer
	doc := backgroundDocument([]Step{
		listenStep("listen", "1m"),
		{ID: "ping", Client: "peer", RPC: &RPCOperation{Method: "all.ping", Request: map[string]any{}}},
		{ID: "wait", Await: "listen", Timeout: "5s"},
	})
	doc.Variables["evidence"] = VariableSpec{Direction: "input", Type: "string", Value: "cleanup-ran"}
	doc.Finally = []Step{{ID: "cleanup", Output: &OutputOperation{Variable: "evidence"}}}
	started := time.Now()
	result := runTask(context.Background(), task{doc: doc}, Options{Driver: driver, Out: &output})
	if time.Since(started) > 5*time.Second {
		t.Fatal("aborted task waited for the background step")
	}
	if result.Status != "failed" || len(result.Steps) != 3 || result.Error != "ping failed" {
		t.Fatalf("result = %#v", result)
	}
	cancelled := result.Steps[2]
	if cancelled.ID != "listen" || cancelled.Status != "cancelled" || cancelled.Evidence["background"] != true {
		t.Fatalf("cancelled step report = %#v", cancelled)
	}
	if len(result.Cleanup) != 1 || output.String() != "evidence=cleanup-ran\n" {
		t.Fatalf("finalizers did not run after the background step was cancelled: %#v %q", result.Cleanup, output.String())
	}
}

func TestRunStepRejectsLoneAwait(t *testing.T) {
	vars, err := NewVariables(nil)
	if err != nil {
		t.Fatal(err)
	}
	step := Step{ID: "wait", Await: "listen"}
	report, err := RunStep(context.Background(), "background.yaml", step, &stubSession{driver: &stubDriver{}}, vars, nil, Options{Driver: &stubDriver{}, Out: io.Discard}, nil)
	if err == nil || !strings.Contains(err.Error(), "can only run inside a task") || report.Status != "failed" {
		t.Fatalf("report = %#v err = %v", report, err)
	}
}
