package giztest

import (
	"context"
	"slices"
)

// CoreOperations are the step operations the runner executes itself, without
// a client. Every other operation is dispatched to the Driver.
var CoreOperations = []string{"barrier", "output", "review"}

// Driver executes the client-facing steps of a Giztest document.
//
// The gizclaw CLI implements it with sdk/go/gizcli; the cgo runner in
// tests/gizclaw-e2e implements it with the C SDKs. A driver declares the
// operations it can execute so validation rejects an unsupported document
// instead of silently skipping a step.
type Driver interface {
	// Operations lists the step operations this driver executes, excluding
	// CoreOperations. LoadDocuments rejects any document that uses an
	// operation outside the union of the two sets.
	//
	// "await" is the operation of a step that consumes a background step.
	// The runner executes it itself, but only a driver whose Session
	// implements BackgroundSession lists it: a driver that cannot run a
	// peer_stream step concurrently with the task omits it, so a document
	// with background steps is rejected or skipped instead of failing at run
	// time.
	Operations() []string

	// ValidateStep checks driver-specific details of one step without
	// connecting, such as whether an RPC request matches its schema. It runs
	// for both steps and finalizers.
	ValidateStep(doc *Document, step Step) error

	// Open connects the clients one task needs. A non-nil Session is returned
	// even alongside an error so the runner can report the clients that did
	// connect and close them.
	Open(ctx context.Context, doc *Document, vars *Variables) (Session, error)

	// FailureCode reports the structured error code and message carried by
	// err, used to evaluate a step's expect_error. ok is false when err does
	// not carry one.
	FailureCode(err error) (code int32, message string, ok bool)
}

// Session is one task's connected client set. The runner owns its lifecycle:
// it calls CloseStreams after the document's steps and Close after its
// finalizers.
type Session interface {
	// Fingerprints maps client name to the identity recorded in the report.
	Fingerprints() map[string]string

	// Execute runs one client-facing step and reports its value, the value
	// stored by save_as, and the evidence recorded in the report. Returning a
	// non-nil error fails the step; wrap the error in AssertionError when the
	// operation itself succeeded but an expectation did not hold.
	Execute(ctx context.Context, req StepRequest) (StepResult, error)

	// CloseStreams closes streams the document held open across steps. It runs
	// once, after the last step and before the finalizers.
	CloseStreams() error

	// Close releases the clients. It is safe to call on a Session returned
	// alongside an error from Open.
	Close()
}

// StepRequest is one client-facing step handed to a Session.
type StepRequest struct {
	// DocumentPath is the source file, used for document-scoped caches.
	DocumentPath string
	Step         Step
	// Vars resolves ${name} references and holds the specs of the variables
	// the step captures into.
	Vars *Variables
	// Cleanup reports whether the step came from the document's finally block.
	Cleanup bool
	// Awaiter is the await step that consumes Step's result, set only when
	// Step is a background step. A background step declares no capture of
	// its own, so the awaiter's capture map is what bounds the data the
	// background step retains, such as the /audio capture limit.
	Awaiter *Step
}

/*
BackgroundSession is the optional Session capability behind background and
await steps. The runner keeps the orchestration itself: it starts the step in
a goroutine, bounds every wait on it, joins it in the await step, cancels the
ones the document never reached, and refuses to tear the session down while a
goroutine that ignored cancellation still owns a stream. The driver only
splits one step into a prepare phase and a run phase.

Prepare runs on the task goroutine while it still owns req.Vars exclusively:
it resolves the step input, the awaiter's capture bounds, and the client, and
fails fast when the driver cannot run this step concurrently. Run then drives
the operation from the background goroutine without touching Variables,
because the task goroutine keeps assigning them while the step runs. The
cancellation of the context passed to Run is the only stop signal a
background step receives; a driver must release its stream promptly when it
fires, or the runner reports the task as leaking that stream.
*/
type BackgroundSession interface {
	Session

	// PrepareBackground resolves everything req.Step needs from req.Vars and
	// returns the step's run phase. req.Awaiter is the await step that will
	// consume the result.
	PrepareBackground(req StepRequest) (BackgroundStep, error)
}

// BackgroundStep is the run phase of a prepared background step.
type BackgroundStep interface {
	// Run drives the step until it completes or ctx is cancelled. It must
	// not read or assign Variables.
	Run(ctx context.Context) (StepResult, error)
}

// StepResult is what a Session produced for one step.
type StepResult struct {
	// Value is asserted by expect and read by capture.
	Value any
	// Saved is stored by save_as. A nil Saved falls back to Value.
	Saved any
	// Evidence is recorded in the step report. It must not carry secrets
	// unless the document opted into full evidence.
	Evidence map[string]any
}

// operationSupported reports whether the runner or driver can execute op.
func operationSupported(driver Driver, op string) bool {
	if slices.Contains(CoreOperations, op) {
		return true
	}
	if driver == nil {
		return false
	}
	return slices.Contains(driver.Operations(), op)
}
