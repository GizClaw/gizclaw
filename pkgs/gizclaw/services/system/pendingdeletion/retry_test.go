package pendingdeletion

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRetryDelayIsBoundedAndExponential(t *testing.T) {
	initial := 100 * time.Millisecond
	maximum := time.Second
	for failures := 1; failures <= 40; failures++ {
		delay := retryDelay(initial, maximum, failures)
		base := initial << min(failures-1, 30)
		if base > maximum || base < 0 {
			base = maximum
		}
		if delay < base*3/4 || delay > base || delay > maximum {
			t.Fatalf("retryDelay(%d) = %s, want [%s, %s]", failures, delay, base*3/4, base)
		}
	}
}

func TestOutcomeErrorsNormalizeSafeMetadata(t *testing.T) {
	cause := errors.New("raw backend error")
	outcome := Deferred(strings.Repeat("c", 100), strings.Repeat("m", 400), time.Second).(*OutcomeError)
	if outcome.Class != OutcomeDeferred || len(outcome.Code) != 64 || len(outcome.Message) != 256 || outcome.After != time.Second {
		t.Fatalf("Deferred() = %#v", outcome)
	}
	withCause := Retryable("temporary", "safe", cause).(*OutcomeError)
	if withCause.Error() != cause.Error() || !errors.Is(withCause, cause) {
		t.Fatalf("Retryable() = %#v", withCause)
	}
	invalid := normalizeOutcome(&OutcomeError{Class: "unknown", Err: cause})
	if invalid.Class != OutcomeTerminal || invalid.Code != "invalid_outcome" {
		t.Fatalf("normalizeOutcome() = %#v", invalid)
	}
}

func TestProcessorConfigValidation(t *testing.T) {
	valid := DefaultConfig()
	if err := valid.Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate() = %v", err)
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.ScanInterval = 0 },
		func(c *Config) { c.PageSize = 0 },
		func(c *Config) { c.Workers = 257 },
		func(c *Config) { c.AttemptTimeout = c.LeaseDuration },
		func(c *Config) { c.RetryInitial = c.RetryMax + time.Second },
	} {
		config := valid
		mutate(&config)
		if err := config.Validate(); err == nil {
			t.Fatalf("Validate(%#v) error = nil", config)
		}
	}
}

func TestOutcomeMetadataIsBounded(t *testing.T) {
	err := Terminal(string(make([]byte, 100)), string(make([]byte, 400)), nil)
	outcome, ok := err.(*OutcomeError)
	if !ok || len(outcome.Code) > 64 || len(outcome.Message) > 256 {
		t.Fatalf("Terminal() = %#v", err)
	}
}
