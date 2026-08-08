package pendingdeletion

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrNotFound reports that an active task no longer exists.
	ErrNotFound = errors.New("pending deletion: task not found")
	// ErrConflict reports a conditional transition that no longer applies.
	ErrConflict = errors.New("pending deletion: task conflict")
	// ErrInvalid reports invalid operator input or persisted task state.
	ErrInvalid = errors.New("pending deletion: invalid task")
)

// Status is the durable state of an active deletion task.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusRetryWait Status = "retry_wait"
	StatusFailed    Status = "failed"
)

// Phase identifies a replay-safe handler checkpoint.
type Phase string

const (
	PhaseValidate Phase = "validate"
	PhaseFinalize Phase = "finalize"
)

var phasePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// ValidatePhase rejects unbounded phase names before they reach storage or metrics.
func ValidatePhase(phase Phase) error {
	if !phasePattern.MatchString(string(phase)) {
		return fmt.Errorf("%w: invalid phase", ErrInvalid)
	}
	return nil
}

// Task is an active mutable cleanup task. Completed tasks do not exist.
type Task struct {
	Source            string
	Record            Record
	MarkerFingerprint string
	Status            Status
	Phase             Phase
	FailureCount      int
	NextAttemptAt     time.Time
	LeaseToken        string
	LeaseDeadline     time.Time
	LastErrorCode     string
	LastErrorMessage  string
	UpdatedAt         time.Time
}

// Reference is one due task observation. Observations are not claims.
type Reference struct {
	Source            string
	DeletionID        string
	MarkerFingerprint string
}

// Claim is an exclusive, time-bounded attempt over one immutable marker.
type Claim struct {
	Task
}

// SourceListOptions bounds one source query for active tasks.
type SourceListOptions struct {
	Kinds           map[Kind]bool
	Statuses        map[Status]bool
	StartTime       *time.Time
	EndTime         *time.Time
	AfterCreatedAt  *time.Time
	AfterSource     string
	AfterDeletionID string
	Limit           int
}

// Source is a durable task backend owned by one product domain.
type Source interface {
	Name() string
	Kinds() []Kind
	ScanDue(context.Context, time.Time, int, string) ([]Reference, string, error)
	Claim(context.Context, Reference, time.Time, time.Duration) (Claim, bool, error)
	Renew(context.Context, Claim, time.Time, time.Duration) error
	Checkpoint(context.Context, Claim, Phase, time.Time) (Claim, error)
	Defer(context.Context, Claim, string, string, time.Time, time.Time) error
	Fail(context.Context, Claim, string, string, bool, time.Time, time.Time, int) error
	GetTask(context.Context, string) (Task, error)
	ListTasks(context.Context, SourceListOptions) ([]Task, error)
	Retry(context.Context, string, time.Time) (Task, error)
}

// SourceStats is an optional efficient active-task telemetry surface.
type SourceStats interface {
	ActiveStats(context.Context, time.Time) (activeDepth int64, oldestCreatedAt time.Time, err error)
}

// Handler owns resource validation and domain-atomic finalization for one kind.
type Handler interface {
	Kind() Kind
	Handle(context.Context, Claim) error
}

// OutcomeClass selects the durable transition after a handler error.
type OutcomeClass string

const (
	OutcomeDeferred  OutcomeClass = "deferred"
	OutcomeRetryable OutcomeClass = "retryable"
	OutcomeTerminal  OutcomeClass = "terminal"
)

// OutcomeError contains only bounded, operator-safe failure metadata.
type OutcomeError struct {
	Class   OutcomeClass
	Code    string
	Message string
	After   time.Duration
	Err     error
}

func (e *OutcomeError) Error() string {
	if e == nil {
		return "pending deletion outcome"
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *OutcomeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Deferred reports a safety precondition that should be retried without budget use.
func Deferred(code, message string, after time.Duration) error {
	return newOutcomeError(OutcomeDeferred, code, message, after, nil)
}

// Retryable reports a bounded transient failure.
func Retryable(code, message string, err error) error {
	return newOutcomeError(OutcomeRetryable, code, message, 0, err)
}

// Terminal reports an unsafe or invalid task that requires operator action.
func Terminal(code, message string, err error) error {
	return newOutcomeError(OutcomeTerminal, code, message, 0, err)
}

func newOutcomeError(class OutcomeClass, code, message string, after time.Duration, err error) error {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		code = string(class)
	}
	if len(code) > 64 {
		code = code[:64]
	}
	if message == "" {
		message = code
	}
	if len(message) > 256 {
		message = message[:256]
	}
	return &OutcomeError{Class: class, Code: code, Message: message, After: after, Err: err}
}

// ValidateTask verifies the complete persisted task envelope.
func ValidateTask(task Task) error {
	if strings.TrimSpace(task.Source) == "" || task.Source != strings.TrimSpace(task.Source) {
		return fmt.Errorf("%w: invalid source", ErrInvalid)
	}
	if err := task.Record.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	fingerprint, err := Fingerprint(task.Record)
	if err != nil || fingerprint != task.MarkerFingerprint {
		return fmt.Errorf("%w: marker fingerprint mismatch", ErrInvalid)
	}
	return validateTaskState(task)
}

func validateStoredTask(task Task) error {
	if strings.TrimSpace(task.Source) == "" || task.Source != strings.TrimSpace(task.Source) {
		return fmt.Errorf("%w: invalid source", ErrInvalid)
	}
	fingerprint, err := StoredFingerprint(task.Record)
	if err != nil || fingerprint != task.MarkerFingerprint {
		return fmt.Errorf("%w: stored marker fingerprint mismatch", ErrInvalid)
	}
	return validateTaskState(task)
}

func validateTaskState(task Task) error {
	switch task.Status {
	case StatusQueued, StatusRunning, StatusRetryWait, StatusFailed:
	default:
		return fmt.Errorf("%w: invalid status %q", ErrInvalid, task.Status)
	}
	if err := ValidatePhase(task.Phase); err != nil {
		return err
	}
	if task.FailureCount < 0 {
		return fmt.Errorf("%w: negative failure count", ErrInvalid)
	}
	if task.UpdatedAt.IsZero() {
		return fmt.Errorf("%w: empty updated time", ErrInvalid)
	}
	if len(task.LastErrorCode) > 64 || len(task.LastErrorMessage) > 256 {
		return fmt.Errorf("%w: failure metadata exceeds bounds", ErrInvalid)
	}
	if task.Status == StatusRunning {
		if task.LeaseToken == "" || len(task.LeaseToken) > 128 || task.LeaseDeadline.IsZero() {
			return fmt.Errorf("%w: invalid running lease", ErrInvalid)
		}
	} else if task.LeaseToken != "" || !task.LeaseDeadline.IsZero() {
		return fmt.Errorf("%w: inactive task has a lease", ErrInvalid)
	}
	if (task.Status == StatusQueued || task.Status == StatusRetryWait) && task.NextAttemptAt.IsZero() {
		return fmt.Errorf("%w: due task has no next attempt time", ErrInvalid)
	}
	return nil
}
