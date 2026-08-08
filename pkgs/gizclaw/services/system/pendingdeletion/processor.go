package pendingdeletion

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
)

// Config bounds processor concurrency, attempts, leases, scans, and retries.
type Config struct {
	ScanInterval     time.Duration
	PageSize         int
	DispatchCapacity int
	Workers          int
	LeaseDuration    time.Duration
	AttemptTimeout   time.Duration
	RetryInitial     time.Duration
	RetryMax         time.Duration
	MaxAttempts      int
}

// DefaultConfig returns production-safe processor defaults.
func DefaultConfig() Config {
	return Config{
		ScanInterval:     30 * time.Second,
		PageSize:         100,
		DispatchCapacity: 256,
		Workers:          4,
		LeaseDuration:    2 * time.Minute,
		AttemptTimeout:   90 * time.Second,
		RetryInitial:     5 * time.Second,
		RetryMax:         30 * time.Minute,
		MaxAttempts:      10,
	}
}

// Validate rejects unbounded or internally inconsistent configuration.
func (c Config) Validate() error {
	if c.ScanInterval <= 0 || c.LeaseDuration <= 0 || c.AttemptTimeout <= 0 ||
		c.RetryInitial <= 0 || c.RetryMax <= 0 {
		return errors.New("pending deletion: durations must be positive")
	}
	if c.PageSize <= 0 || c.PageSize > 1000 || c.DispatchCapacity <= 0 ||
		c.DispatchCapacity > 10000 || c.Workers <= 0 || c.Workers > 256 ||
		c.MaxAttempts <= 0 || c.MaxAttempts > 1000 {
		return errors.New("pending deletion: counts are outside supported bounds")
	}
	if c.AttemptTimeout >= c.LeaseDuration {
		return errors.New("pending deletion: attempt timeout must be shorter than lease duration")
	}
	if c.RetryInitial > c.RetryMax {
		return errors.New("pending deletion: retry initial exceeds retry max")
	}
	return nil
}

type dispatchItem struct {
	source Source
	ref    Reference
}

// Processor scans durable sources and executes bounded deletion attempts.
type Processor struct {
	registry *Registry
	config   Config
	metrics  metrics.Store
	now      func() time.Time

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
	wake    chan struct{}
	started bool
	active  atomic.Int64

	scanStart   int
	scanCursors map[string]string
}

// NewProcessor validates and constructs a stopped processor.
func NewProcessor(registry *Registry, config Config, metricsStore metrics.Store) (*Processor, error) {
	if registry == nil {
		return nil, errors.New("pending deletion: registry is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Processor{
		registry:    registry,
		config:      config,
		metrics:     metricsStore,
		now:         time.Now,
		wake:        make(chan struct{}, 1),
		scanCursors: make(map[string]string),
	}, nil
}

// Start begins scanners and workers. Repeated starts are harmless.
func (p *Processor) Start(parent context.Context) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || len(p.registry.sources()) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	p.done = make(chan struct{})
	p.started = true
	p.scanStart = 0
	p.scanCursors = make(map[string]string)
	go p.run(ctx, p.done)
}

// Wake requests a low-latency scan without becoming a correctness dependency.
func (p *Processor) Wake() {
	if p == nil {
		return
	}
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Close cancels scans and attempts and waits for every owned goroutine.
func (p *Processor) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	cancel, done := p.cancel, p.done
	p.cancel = nil
	p.done = nil
	p.started = false
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (p *Processor) run(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	dispatch := make(chan dispatchItem, p.config.DispatchCapacity)
	var workers sync.WaitGroup
	for range p.config.Workers {
		workers.Go(func() {
			p.worker(ctx, dispatch)
		})
	}
	ticker := time.NewTicker(p.config.ScanInterval)
	defer ticker.Stop()
	p.scan(ctx, dispatch)
	for {
		select {
		case <-ctx.Done():
			close(dispatch)
			workers.Wait()
			return
		case <-ticker.C:
			p.scan(ctx, dispatch)
		case <-p.wake:
			p.scan(ctx, dispatch)
		}
	}
}

func (p *Processor) scan(ctx context.Context, dispatch chan<- dispatchItem) {
	sources := p.registry.sources()
	for _, source := range sources {
		p.observeSourceStats(ctx, source)
	}
	if len(sources) == 0 {
		return
	}
	start := p.scanStart % len(sources)
	p.scanStart = (start + 1) % len(sources)
	// Read at most one page per source in one invocation. A deep or sparse
	// source therefore cannot monopolize the scanner. In-memory cursors advance
	// across invocations and reset after the source tail, so non-due or
	// foreign-kind records cannot permanently hide later due work. A fresh
	// processor lifecycle always restarts discovery from the beginning.
	if p.scanCursors == nil {
		p.scanCursors = make(map[string]string)
	}
	for offset := range len(sources) {
		i := (start + offset) % len(sources)
		source := sources[i]
		cursor := p.scanCursors[source.Name()]
		refs, next, err := source.ScanDue(ctx, p.now().UTC(), p.config.PageSize, cursor)
		if err != nil {
			p.observe(source.Name(), "", "", "", "scan_error")
			continue
		}
		for _, ref := range refs {
			select {
			case dispatch <- dispatchItem{source: source, ref: ref}:
			case <-ctx.Done():
				return
			default:
				p.scanStart = (i + 1) % len(sources)
				return
			}
		}
		if next == "" {
			delete(p.scanCursors, source.Name())
		} else {
			p.scanCursors[source.Name()] = next
		}
	}
}

func (p *Processor) worker(ctx context.Context, dispatch <-chan dispatchItem) {
	for item := range dispatch {
		if ctx.Err() != nil {
			return
		}
		p.process(ctx, item)
	}
}

func (p *Processor) process(ctx context.Context, item dispatchItem) {
	now := p.now().UTC()
	claim, claimed, err := item.source.Claim(ctx, item.ref, now, p.config.LeaseDuration)
	if err != nil || !claimed {
		return
	}
	_, handler, ok := p.registry.lookup(item.source.Name(), claim.Record.Kind)
	if !ok {
		_ = item.source.Fail(ctx, claim, "handler_missing", "registered handler is missing", true, now, now, p.config.MaxAttempts)
		return
	}
	p.active.Add(1)
	p.observeValue("gizclaw_pending_deletion_active_workers", map[string]string{}, float64(p.active.Load()))
	defer func() {
		active := p.active.Add(-1)
		p.observeValue("gizclaw_pending_deletion_active_workers", map[string]string{}, float64(active))
	}()
	p.observe(claim.Source, claim.Record.Kind, claim.Status, claim.Phase, "claimed")
	attemptStarted := time.Now()
	attemptOutcome := "abandoned"
	defer func() {
		p.observeValue("gizclaw_pending_deletion_phase_latency_seconds", map[string]string{
			"source": claim.Source, "kind": string(claim.Record.Kind), "phase": metricPhase(claim.Phase), "outcome": attemptOutcome,
		}, time.Since(attemptStarted).Seconds())
	}()

	attemptCtx, cancel := context.WithTimeout(ctx, p.config.AttemptTimeout)
	defer cancel()
	renewDone := make(chan error, 1)
	go p.renew(attemptCtx, cancel, item.source, claim, renewDone)
	handleErr := callHandler(attemptCtx, handler, claim)
	attemptErr := attemptCtx.Err()
	cancel()
	renewErr := <-renewDone
	if ctx.Err() != nil || renewErr != nil {
		if ctx.Err() != nil {
			attemptOutcome = "shutdown"
		} else {
			attemptOutcome = "lease_lost"
		}
		return
	}
	if attemptErr != nil {
		handleErr = attemptErr
	}
	if handleErr == nil {
		attemptOutcome = "completed"
		p.observe(claim.Source, claim.Record.Kind, claim.Status, claim.Phase, "completed")
		return
	}
	if errors.Is(handleErr, context.Canceled) || errors.Is(handleErr, context.DeadlineExceeded) {
		handleErr = Retryable("attempt_timeout", "cleanup attempt timed out", handleErr)
	}
	var outcome *OutcomeError
	if !errors.As(handleErr, &outcome) {
		outcome = &OutcomeError{Class: OutcomeRetryable, Code: "handler_error", Message: "cleanup handler failed", Err: handleErr}
	}
	outcome = normalizeOutcome(outcome)
	attemptOutcome = string(outcome.Class)
	outcomeNow := p.now().UTC()
	var transitionErr error
	switch outcome.Class {
	case OutcomeDeferred:
		delay := outcome.After
		if delay <= 0 {
			delay = p.config.ScanInterval
		}
		transitionErr = item.source.Defer(ctx, claim, outcome.Code, outcome.Message, outcomeNow.Add(delay), outcomeNow)
	case OutcomeTerminal:
		transitionErr = item.source.Fail(ctx, claim, outcome.Code, outcome.Message, true, outcomeNow, outcomeNow, p.config.MaxAttempts)
	default:
		next := outcomeNow.Add(retryDelay(p.config.RetryInitial, p.config.RetryMax, claim.FailureCount+1))
		transitionErr = item.source.Fail(ctx, claim, outcome.Code, outcome.Message, false, next, outcomeNow, p.config.MaxAttempts)
	}
	if transitionErr != nil {
		attemptOutcome = "transition_error"
		p.observe(claim.Source, claim.Record.Kind, claim.Status, claim.Phase, "transition_error")
		return
	}
	p.observe(claim.Source, claim.Record.Kind, claim.Status, claim.Phase, string(outcome.Class))
}

func normalizeOutcome(outcome *OutcomeError) *OutcomeError {
	if outcome == nil {
		return &OutcomeError{Class: OutcomeRetryable, Code: "handler_error", Message: "cleanup handler failed"}
	}
	class := outcome.Class
	switch class {
	case OutcomeDeferred, OutcomeRetryable, OutcomeTerminal:
	default:
		return &OutcomeError{Class: OutcomeTerminal, Code: "invalid_outcome", Message: "cleanup handler returned an invalid outcome", Err: outcome.Err}
	}
	normalized := newOutcomeError(class, outcome.Code, outcome.Message, outcome.After, outcome.Err)
	return normalized.(*OutcomeError)
}

func (p *Processor) renew(ctx context.Context, cancel context.CancelFunc, source Source, claim Claim, done chan<- error) {
	interval := p.config.LeaseDuration / 3
	if interval <= 0 {
		interval = time.Nanosecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			done <- nil
			return
		case now := <-ticker.C:
			if err := source.Renew(ctx, claim, now.UTC(), p.config.LeaseDuration); err != nil {
				if ctx.Err() != nil {
					done <- nil
					return
				}
				cancel()
				done <- err
				return
			}
		}
	}
}

func callHandler(ctx context.Context, handler Handler, claim Claim) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = Terminal("handler_panic", "cleanup handler panicked", fmt.Errorf("panic: %v", recovered))
		}
	}()
	return handler.Handle(ctx, claim)
}

func (p *Processor) observe(source string, kind Kind, status Status, phase Phase, outcome string) {
	if p == nil || p.metrics == nil {
		return
	}
	p.appendMetrics([]metrics.Sample{{
		Name: "gizclaw_pending_deletion_events_total",
		Labels: map[string]string{
			"source":  source,
			"kind":    string(kind),
			"status":  string(status),
			"phase":   metricPhase(phase),
			"outcome": outcome,
		},
		Timestamp: p.now().UTC(),
		Value:     1,
	}})
}

func metricPhase(phase Phase) string {
	if ValidatePhase(phase) != nil {
		return "invalid"
	}
	return string(phase)
}

func (p *Processor) observeSourceStats(ctx context.Context, source Source) {
	if p == nil || p.metrics == nil {
		return
	}
	statsSource, ok := source.(SourceStats)
	if !ok {
		return
	}
	now := p.now().UTC()
	statsCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	depth, oldest, err := statsSource.ActiveStats(statsCtx, now)
	if err != nil {
		p.observe(source.Name(), "", "", "", "stats_error")
		return
	}
	age := 0.0
	if !oldest.IsZero() && oldest.Before(now) {
		age = now.Sub(oldest).Seconds()
	}
	labels := map[string]string{"source": source.Name()}
	p.appendMetrics([]metrics.Sample{
		{Name: "gizclaw_pending_deletion_active_depth", Labels: labels, Timestamp: now, Value: float64(depth)},
		{Name: "gizclaw_pending_deletion_oldest_age_seconds", Labels: labels, Timestamp: now, Value: age},
	})
}

func (p *Processor) observeValue(name string, labels map[string]string, value float64) {
	if p == nil || p.metrics == nil {
		return
	}
	p.appendMetrics([]metrics.Sample{{Name: name, Labels: labels, Timestamp: p.now().UTC(), Value: value}})
}

func (p *Processor) appendMetrics(samples []metrics.Sample) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := p.metrics.Append(ctx, samples)
	if err != nil {
		log.Printf("pending deletion: metrics append failed")
	}
}
