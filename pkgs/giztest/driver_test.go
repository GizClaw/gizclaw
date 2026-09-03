package giztest

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// stubDriver executes client-facing steps from a scripted table so the core
// runner can be tested without a transport.
type stubDriver struct {
	// openErr fails Open, exercising the client-setup failure path.
	openErr error
	// execute produces one step's outcome. A nil execute passes every step
	// with an empty object value.
	execute func(ctx context.Context, req StepRequest) (StepResult, error)
	// closeStreamsErr fails the between-steps stream close.
	closeStreamsErr error
	// prepareParallel, when set, makes the session a ParallelSession whose
	// prepare phase is this function.
	prepareParallel func(req StepRequest) (ParallelChild, error)

	// operations overrides the driver's supported set; nil means all of them.
	operations []string

	mu       sync.Mutex
	closed   int
	streamed int
}

func (d *stubDriver) Operations() []string {
	if d.operations != nil {
		return d.operations
	}
	return []string{"rpc", "rpc_stream", "client_rpc", "http", "speech", "peer_stream", "workspace_relay", "parallel"}
}

func (d *stubDriver) ValidateStep(*Document, Step) error { return nil }

func (d *stubDriver) Open(context.Context, *Document, *Variables) (Session, error) {
	if d.openErr != nil {
		return nil, d.openErr
	}
	if d.prepareParallel != nil {
		return &stubParallelSession{stubSession{driver: d}}, nil
	}
	return &stubSession{driver: d}, nil
}

func (d *stubDriver) FailureCode(err error) (int32, string, bool) {
	var failure stubFailure
	if errors.As(err, &failure) {
		return failure.code, failure.message, true
	}
	return 0, "", false
}

// stubFailure is the structured error the stub driver reports for
// expect_error steps.
type stubFailure struct {
	code    int32
	message string
}

func (e stubFailure) Error() string { return fmt.Sprintf("code %d: %s", e.code, e.message) }

type stubSession struct{ driver *stubDriver }

func (s *stubSession) Fingerprints() map[string]string { return map[string]string{"peer": "stub"} }

func (s *stubSession) Execute(ctx context.Context, req StepRequest) (StepResult, error) {
	if s.driver.execute == nil {
		return StepResult{Value: map[string]any{}}, nil
	}
	return s.driver.execute(ctx, req)
}

func (s *stubSession) CloseStreams() error {
	s.driver.mu.Lock()
	defer s.driver.mu.Unlock()
	s.driver.streamed++
	return s.driver.closeStreamsErr
}

func (s *stubSession) Close() {
	s.driver.mu.Lock()
	defer s.driver.mu.Unlock()
	s.driver.closed++
}

// stubParallelSession is a stubSession that can also run steps concurrently.
type stubParallelSession struct{ stubSession }

func (s *stubParallelSession) PrepareParallel(req StepRequest) (ParallelChild, error) {
	return s.driver.prepareParallel(req)
}

// parallelRun is the run phase a stub prepare returns.
type parallelRun func(ctx context.Context) (StepResult, error)

func (r parallelRun) Run(ctx context.Context) (StepResult, error) { return r(ctx) }
