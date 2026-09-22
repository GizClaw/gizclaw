package giztest

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"time"
)

const maxTimingSeed = 1<<53 - 1 // Exact in every runner's JSON number representation.

var timingDurationPattern = regexp.MustCompile(`^(0|([0-9]+(\.[0-9]+)?(ns|us|µs|μs|ms|s|m|h))+)$`)

// TimingOverrides replaces document scheduling values when a field is non-nil.
// In particular, an explicit "0" disables a document's delay; seed 0 is valid.
type TimingOverrides struct {
	StartJitter *string
	Stagger     *string
	StepJitter  *string
	Seed        *int64
}

type timingConfig struct {
	startJitter time.Duration
	stagger     time.Duration
	stepJitter  time.Duration
	seed        *int64
}

// ValidateTiming checks effective scheduling values without connecting clients.
// A CLI calls this after loading documents and before Run.
func ValidateTiming(docs []*Document, overrides TimingOverrides) error {
	// Validate flags even when an SDK runner skipped every selected document.
	if _, err := (&Document{}).timing(overrides); err != nil {
		return err
	}
	for _, doc := range docs {
		if _, err := doc.timing(overrides); err != nil {
			return fmt.Errorf("document %s: %w", doc.Name, err)
		}
	}
	return nil
}

func (d *Document) timing(overrides TimingOverrides) (timingConfig, error) {
	var config timingConfig
	for _, field := range []struct {
		name     string
		value    string
		override *string
		target   *time.Duration
	}{
		{"start_jitter", d.StartJitter, overrides.StartJitter, &config.startJitter},
		{"stagger", d.Stagger, overrides.Stagger, &config.stagger},
		{"step_jitter", d.StepJitter, overrides.StepJitter, &config.stepJitter},
	} {
		value := field.value
		if field.override != nil {
			value = *field.override
		} else if value == "" {
			continue
		}
		duration, err := time.ParseDuration(value)
		if err != nil || duration < 0 || !timingDurationPattern.MatchString(value) {
			return config, fmt.Errorf("%s must be a non-negative duration, got %q", field.name, value)
		}
		*field.target = duration
	}
	config.seed = d.Seed
	if overrides.Seed != nil {
		config.seed = overrides.Seed
	}
	if config.seed != nil && (*config.seed < 0 || *config.seed > maxTimingSeed) {
		return config, fmt.Errorf("seed must be an integer in 0..%d", maxTimingSeed)
	}
	// Jitter samples are integral nanoseconds and the upper bound is exclusive.
	maxJitter := max(config.startJitter-1, 0)
	if d.Repeat > 1 && config.stagger > (math.MaxInt64-maxJitter)/time.Duration(d.Repeat-1) {
		return config, fmt.Errorf("(repeat - 1) * stagger + start_jitter exceeds the duration range")
	}
	if d.HasBarrier() && (config.startJitter > 0 || config.stagger > 0 || config.stepJitter > 0) {
		return config, fmt.Errorf("barrier cannot be combined with non-zero start_jitter, stagger or step_jitter")
	}
	return config, nil
}

// timingRandom gives each document/task/purpose its own deterministic stream.
// Worker order, other selected documents and start jitter cannot perturb the
// think times of a task. This seed controls scheduling only, never identities.
func timingRandom(seed int64, name string, index int, purpose string) *rand.Rand {
	hash := sha256.Sum256(fmt.Appendf(nil, "%d\x00%s\x00%d\x00%s", seed, name, index, purpose))
	return rand.New(rand.NewPCG(binary.LittleEndian.Uint64(hash[:8]), binary.LittleEndian.Uint64(hash[8:16])))
}

func sampleJitter(random *rand.Rand, limit time.Duration) time.Duration {
	if limit == 0 {
		return 0
	}
	return time.Duration(random.Int64N(int64(limit)))
}

func (c timingConfig) plan(doc *Document, index int, seed int64) task {
	if c.seed != nil {
		seed = *c.seed
	}
	return task{
		doc: doc, index: index, timing: c, seed: seed,
		startOffset: time.Duration(index)*c.stagger + sampleJitter(timingRandom(seed, doc.Name, index, "start"), c.startJitter),
	}
}

func waitTiming(ctx context.Context, delay time.Duration) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return context.Cause(ctx)
	}
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func taskReport(item task) TaskReport {
	return TaskReport{
		Path: item.doc.Path, Name: item.doc.Name,
		TaskID:      fmt.Sprintf("%s-%04d", item.doc.Name, item.index),
		RepeatIndex: item.index, Status: "failed",
		Seed: item.seed, PlannedStartOffsetMS: milliseconds(item.startOffset),
		StartJitter: item.timing.startJitter.String(), Stagger: item.timing.stagger.String(), StepJitter: item.timing.stepJitter.String(),
	}
}

func setStepStart(report *StepReport, started, runStart time.Time) {
	report.StartedAt = &started
	if !runStart.IsZero() {
		offset := milliseconds(started.Sub(runStart))
		report.StartOffsetMS = &offset
	}
}
