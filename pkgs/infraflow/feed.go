// Package infraflow stores bounded five-second infrastructure activity frames.
// It has no user/session index, raw event log, or HTTP write surface.
package infraflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"sync"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

const (
	Window          = 5 * time.Second
	MaxFlows        = 64
	RetentionFrames = 120
	MaxReadFrames   = 3
	MaxFrameBytes   = 32 * 1024
	MaxCounter      = 1<<53 - 1
	Path            = "/gizclaw/v1/infra/flows"
)

var (
	ErrStaleFrame     = errors.New("infraflow: stale frame")
	ErrWindowConflict = errors.New("infraflow: conflicting window publisher")
	idPattern         = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,80}$`)
)

type cacheSlot struct {
	until   time.Time
	head    int64
	frames  []infraapi.Frame
	pending chan struct{}
	err     error
}

// Feed borrows a KV Store. Slots are fixed: 120 frames and one head per
// environment, regardless of traffic volume. Close remains the caller's job.
type Feed struct {
	store kv.Store
	mu    sync.Mutex
	cache [4]cacheSlot
}

// New borrows the existing KV interface, including its atomic conditional writes.
// It creates no background work or network connections.
func New(store kv.Store) (*Feed, error) {
	if store == nil {
		return nil, errors.New("infraflow: KV Store is required")
	}
	return &Feed{store: store}, nil
}

func environmentIndex(env infraapi.Environment) int {
	switch env {
	case "dev":
		return 0
	case "e2e":
		return 1
	case "prod":
		return 2
	case "trial":
		return 3
	default:
		return -1
	}
}

func headKey(env infraapi.Environment) kv.Key { return kv.Key{"infra-flow", "v1", string(env), "head"} }
func slotKey(env infraapi.Environment, end int64) kv.Key {
	return kv.Key{"infra-flow", "v1", string(env), "slots", strconv.FormatInt(end/Window.Milliseconds()%RetentionFrames, 10)}
}

func validFlow(f infraapi.Flow) bool {
	return idPattern.MatchString(f.Source) && idPattern.MatchString(f.Target) && f.Source != f.Target && f.Kind.Valid() &&
		f.Count >= 0 && f.Count <= MaxCounter && f.Bytes >= 0 && f.Bytes <= MaxCounter &&
		f.DurationCount >= 0 && f.DurationCount <= f.Count &&
		!math.IsNaN(f.DurationSumMs) && !math.IsInf(f.DurationSumMs, 0) && f.DurationSumMs >= 0 && f.DurationSumMs <= MaxCounter &&
		(f.DurationCount != 0 || f.DurationSumMs == 0)
}

func routeKey(f infraapi.Flow) string { return f.Source + "|" + f.Target + "|" + string(f.Kind) }

func validateFrame(frame infraapi.Frame) error {
	if environmentIndex(frame.Environment) < 0 || frame.WindowStart < 0 || frame.WindowStart > 8640000000000000 || frame.WindowEnd > 8640000000000000 ||
		frame.WindowEnd-frame.WindowStart != Window.Milliseconds() || frame.WindowEnd%Window.Milliseconds() != 0 ||
		frame.Id != strconv.FormatInt(frame.WindowEnd, 10) || len(frame.Flows) > MaxFlows || frame.Flows == nil {
		return errors.New("infraflow: invalid frame window or capacity")
	}
	seen := make(map[string]bool, len(frame.Flows))
	var count, bytes, durations int64
	var durationSum float64
	for _, flow := range frame.Flows {
		key := routeKey(flow)
		if !validFlow(flow) || seen[key] {
			return errors.New("infraflow: invalid or duplicate flow")
		}
		seen[key] = true
		if count > MaxCounter-flow.Count || bytes > MaxCounter-flow.Bytes || durations > MaxCounter-flow.DurationCount || durationSum > MaxCounter-flow.DurationSumMs {
			return errors.New("infraflow: frame counter capacity exceeded")
		}
		count += flow.Count
		bytes += flow.Bytes
		durations += flow.DurationCount
		durationSum += flow.DurationSumMs
	}
	return nil
}

// Append commits one completed window and its head atomically. A second
// publisher may retry an identical frame, but cannot replace the same window.
func (f *Feed) Append(ctx context.Context, frame infraapi.Frame) error {
	if err := validateFrame(frame); err != nil {
		return err
	}
	data, err := json.Marshal(frame)
	if err != nil || len(data) > MaxFrameBytes {
		return errors.New("infraflow: frame exceeds encoded capacity")
	}
	ctx = kv.WithPrimaryReads(ctx)
	for range 4 {
		previous, err := f.store.Get(ctx, headKey(frame.Environment))
		if err != nil && !errors.Is(err, kv.ErrNotFound) {
			return fmt.Errorf("infraflow: read head: %w", err)
		}
		if errors.Is(err, kv.ErrNotFound) {
			previous = nil
		}
		if previous != nil {
			head, parseErr := strconv.ParseInt(string(previous), 10, 64)
			if parseErr != nil {
				return errors.New("infraflow: invalid stored head")
			}
			if head > frame.WindowEnd {
				return ErrStaleFrame
			}
			if head == frame.WindowEnd {
				existing, readErr := f.store.Get(ctx, slotKey(frame.Environment, head))
				if readErr != nil {
					return readErr
				}
				if bytes.Equal(existing, data) {
					return nil
				}
				return ErrWindowConflict
			}
		}
		matched, err := f.store.ApplyMutation(ctx, kv.Mutation{
			Conditions: []kv.Condition{{Key: headKey(frame.Environment), Expected: previous}},
			Entries:    []kv.Entry{{Key: slotKey(frame.Environment, frame.WindowEnd), Value: data}, {Key: headKey(frame.Environment), Value: []byte(frame.Id)}},
		})
		if err != nil {
			return fmt.Errorf("infraflow: append window: %w", err)
		}
		if matched {
			f.mu.Lock()
			f.cache[environmentIndex(frame.Environment)].until = time.Time{}
			f.mu.Unlock()
			return nil
		}
	}
	return errors.New("infraflow: concurrent publisher contention")
}

func (f *Feed) load(ctx context.Context, env infraapi.Environment) (int64, []infraapi.Frame, error) {
	ctx = kv.WithPrimaryReads(ctx)
	value, err := f.store.Get(ctx, headKey(env))
	if errors.Is(err, kv.ErrNotFound) {
		return 0, []infraapi.Frame{}, nil
	}
	if err != nil {
		return 0, nil, err
	}
	head, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil || head <= 0 || head%Window.Milliseconds() != 0 {
		return 0, nil, errors.New("infraflow: invalid stored head")
	}
	frames := make([]infraapi.Frame, 0, MaxReadFrames)
	for offset := MaxReadFrames - 1; offset >= 0; offset-- {
		end := head - int64(offset)*Window.Milliseconds()
		data, err := f.store.Get(ctx, slotKey(env, end))
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, nil, err
		}
		if len(data) > MaxFrameBytes {
			return 0, nil, errors.New("infraflow: oversized stored frame")
		}
		var frame infraapi.Frame
		if err := json.Unmarshal(data, &frame); err != nil {
			return 0, nil, errors.New("infraflow: malformed stored frame")
		}
		if frame.Environment != env || frame.WindowEnd != end {
			continue
		} // A wrapped slot is never replayed.
		if err := validateFrame(frame); err != nil {
			return 0, nil, err
		}
		frames = append(frames, frame)
	}
	if len(frames) == 0 || frames[len(frames)-1].WindowEnd != head {
		return 0, nil, errors.New("infraflow: head frame unavailable")
	}
	return head, frames, nil
}

// recent shares at most four KV reads per environment per five seconds across
// dashboard readers. KV work runs outside the cache lock and honors cancellation.
func (f *Feed) recent(ctx context.Context, env infraapi.Environment) (int64, []infraapi.Frame, error) {
	index := environmentIndex(env)
	for {
		f.mu.Lock()
		slot := &f.cache[index]
		if time.Now().Before(slot.until) {
			head, frames, err := slot.head, slot.frames, slot.err
			f.mu.Unlock()
			return head, frames, err
		}
		if pending := slot.pending; pending != nil {
			f.mu.Unlock()
			select {
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			case <-pending:
				continue
			}
		}
		pending := make(chan struct{})
		slot.pending = pending
		f.mu.Unlock()
		head, frames, err := f.load(ctx, env)
		f.mu.Lock()
		slot = &f.cache[index]
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			slot.head, slot.frames, slot.err, slot.until = head, frames, err, time.Now().Add(Window)
		}
		slot.pending = nil
		close(pending)
		f.mu.Unlock()
		return head, frames, err
	}
}

// Read returns the newest frame on first connection. Readers can catch up by at
// most three consecutive frames; a gap or expired/future cursor resets to latest.
func (f *Feed) Read(ctx context.Context, env infraapi.Environment, cursor string) (infraapi.Page, error) {
	page := infraapi.Page{Frames: []infraapi.Frame{}, ServerTime: time.Now().UnixMilli(), Limits: infraapi.Limits{WindowMs: Window.Milliseconds(), MaxFlows: MaxFlows, RetentionFrames: RetentionFrames, MaxReadFrames: MaxReadFrames}}
	if err := ctx.Err(); err != nil {
		return page, err
	}
	if environmentIndex(env) < 0 {
		return page, errors.New("infraflow: invalid environment")
	}
	var after int64
	if cursor != "" {
		var err error
		after, err = strconv.ParseInt(cursor, 10, 64)
		if err != nil || !validCursor(cursor) {
			return page, errors.New("infraflow: invalid cursor")
		}
	}
	head, frames, err := f.recent(ctx, env)
	if err != nil {
		return page, err
	}
	if head == 0 {
		page.Reset = cursor != ""
		return page, nil
	}
	page.Cursor = strconv.FormatInt(head, 10)
	if after == head {
		return page, nil
	}
	selected := []infraapi.Frame{frames[len(frames)-1]}
	if after != 0 && after < head && head-after <= MaxReadFrames*Window.Milliseconds() {
		selected = nil
		for _, frame := range frames {
			if frame.WindowEnd > after {
				selected = append(selected, frame)
			}
		}
		if len(selected) == 0 || selected[0].WindowStart != after || int64(len(selected))*Window.Milliseconds() != head-after {
			selected = []infraapi.Frame{frames[len(frames)-1]}
			page.Reset = true
		}
	} else {
		page.Reset = cursor != ""
	}
	for _, frame := range selected {
		frame.Flows = append([]infraapi.Flow{}, frame.Flows...)
		page.Frames = append(page.Frames, frame)
	}
	return page, nil
}
