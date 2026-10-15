package streamkit

import (
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// ttsSynthesisBudget estimates the media still available from accepted jobs.
// It starts consuming on the first ordered output, including time spent waiting
// for the next model token. Provider audio already queued by lookahead jobs also
// contributes, but unfinished synthesis and ungenerated text do not.
type ttsSynthesisBudget struct {
	config   genx.TTSBatchConfig
	measured bool // immutable: this route reports usable audio duration
	changed  chan struct{}
	mu       sync.Mutex
	output   bool
	pending  time.Duration // measured audio awaiting first ordered output
	end      time.Time
	latency  time.Duration
	flights  int
}

func newTTSSynthesisBudget(config genx.TTSBatchConfig, measured bool) *ttsSynthesisBudget {
	return &ttsSynthesisBudget{config: config, measured: measured, changed: make(chan struct{}, 1)}
}

func (b *ttsSynthesisBudget) notify() {
	select {
	case b.changed <- struct{}{}:
	default:
	}
}

func (b *ttsSynthesisBudget) markOutput() {
	b.mu.Lock()
	if !b.output {
		b.output = true
		if b.pending > 0 {
			b.end = time.Now().Add(b.pending)
			b.pending = 0
		}
	}
	b.mu.Unlock()
	b.notify()
}

func (b *ttsSynthesisBudget) start() {
	b.mu.Lock()
	b.flights++
	b.mu.Unlock()
}

func (b *ttsSynthesisBudget) finish() {
	b.mu.Lock()
	b.flights--
	b.mu.Unlock()
	b.notify()
}

func (b *ttsSynthesisBudget) observe(duration, firstAudio time.Duration) {
	if duration <= 0 {
		return
	}
	b.mu.Lock()
	b.latency = max(b.latency, firstAudio)
	if b.output {
		// Once old audio is exhausted, new audio begins at arrival. Carrying
		// a past deficit forever would prevent batching after a recovered stall.
		if now := time.Now(); b.end.Before(now) {
			b.end = now
		}
		b.end = b.end.Add(duration)
	} else {
		b.pending += duration
	}
	b.mu.Unlock()
	b.notify()
}

func (b *ttsSynthesisBudget) deadline(now time.Time) (time.Time, bool) {
	b.mu.Lock()
	end, latency := b.end, b.latency
	awaiting := b.flights > 0 || b.pending > 0
	b.mu.Unlock()
	var deadline time.Time
	if !end.IsZero() {
		// Subtract separately so two valid large reserves cannot wrap Duration.
		deadline = end.Add(-max(b.config.SynthesisLead, latency)).Add(-b.config.SafetyMargin)
	}
	if b.config.WaitBudget != nil {
		external := now.Add(max(0, b.config.WaitBudget()))
		if deadline.IsZero() || external.Before(deadline) {
			deadline = external
		}
	}
	if !deadline.IsZero() {
		return deadline, false
	}
	// Before any measurable audio arrives there is no playback budget to spend.
	// Use an available lookahead slot now: a tiny first phrase can be shorter
	// than the next request's startup latency. Slots still bound concurrency.
	if b.measured && latency == 0 {
		return time.Time{}, false
	}
	// Await the preceding response so subsequent segments can stay together.
	// Unknown formats fall back to submission when that synthesis finishes.
	return time.Time{}, awaiting
}
