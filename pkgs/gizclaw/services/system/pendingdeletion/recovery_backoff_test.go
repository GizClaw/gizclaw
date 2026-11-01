package pendingdeletion

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestFailedRecoveryBackoffTracksObservationAndOutcome(t *testing.T) {
	now := time.Unix(1000, 0)
	task := Task{Source: "peer", Record: Record{DeletionID: "one", Descriptor: []byte("not retained")},
		Status: StatusFailed, UpdatedAt: now, LastErrorCode: "legacy"}
	var backoff failedRecoveryBackoff
	first := backoff.begin(task, now)
	if first == nil || len(first.observed.Record.Descriptor) != 0 {
		t.Fatal("first probe must reserve the observation without retaining its payload")
	}
	if backoff.begin(task, now) != nil {
		t.Fatal("same observation was probed concurrently")
	}
	if !backoff.shouldWarn(first, "ambiguous") || backoff.shouldWarn(first, "ambiguous") {
		t.Fatal("unchanged rejection was not deduplicated")
	}
	backoff.end(first, now, true)
	if backoff.begin(task, now.Add(time.Second)) != nil {
		t.Fatal("retained failure bypassed backoff")
	}
	second := backoff.begin(task, now.Add(31*time.Second))
	if second == nil || backoff.shouldWarn(second, "ambiguous") || !backoff.shouldWarn(second, "different") {
		t.Fatal("periodic probe must deduplicate the same outcome and report a changed outcome")
	}
	backoff.end(second, now.Add(31*time.Second), true)
	if backoff.begin(task, now.Add(32*time.Second)) != nil {
		t.Fatal("second rejection bypassed exponential backoff")
	}
	task.UpdatedAt = task.UpdatedAt.Add(time.Nanosecond)
	changed := backoff.begin(task, now.Add(32*time.Second))
	if changed == nil || !backoff.shouldWarn(changed, "ambiguous") {
		t.Fatal("changed failed observation did not bypass old backoff and warning state")
	}
	backoff.end(changed, now, false)
	if len(backoff.entries) != 0 {
		t.Fatal("success did not release cached observation")
	}
}

func TestFailedRecoveryBackoffIsBoundedAndKeepsInFlightProbes(t *testing.T) {
	now := time.Unix(1000, 0)
	backoff := failedRecoveryBackoff{capacity: 2}
	task := func(id string) Task {
		return Task{Source: "peer", Record: Record{DeletionID: id}, Status: StatusFailed}
	}
	active := backoff.begin(task("active"), now)
	idle := backoff.begin(task("idle"), now)
	backoff.end(idle, now, true)
	next := backoff.begin(task("next"), now)
	if active == nil || next == nil || len(backoff.entries) != 2 {
		t.Fatal("cache did not evict the retained idle observation at capacity")
	}
	if backoff.begin(task("overflow"), now) != nil || backoff.begin(task("active"), now) != nil {
		t.Fatal("bounded cache evicted an in-flight probe or allowed a duplicate")
	}
	backoff.end(active, now, false)
	backoff.end(next, now, false)
	if len(backoff.entries) != 0 {
		t.Fatal("completed probes remained cached")
	}
}

func TestFailedRecoveryBackoffDoesNotHoldOtherResources(t *testing.T) {
	source := KVSource{Store: kv.NewMemory(nil), SourceName: "peer", OwnedKinds: []Kind{KindPeer}}
	blocked := seedFailedRecoveryTask(t, source, "blocked", time.Now().Add(-time.Minute))
	other := seedFailedRecoveryTask(t, source, "other", time.Now().Add(-time.Minute))
	entered, release := make(chan struct{}, 2), make(chan struct{})
	handler := &automaticKVHandler{finalizingKVHandler: finalizingKVHandler{kind: KindPeer, source: source},
		probe: func(ctx context.Context, task Task) (bool, error) {
			if task.Record.ResourceID != "blocked" {
				return true, nil
			}
			entered <- struct{}{}
			select {
			case <-release:
				return false, Terminal("ambiguous", "retained", nil)
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}}
	registry := NewRegistry()
	if err := registry.Register(source, handler); err != nil {
		t.Fatal(err)
	}
	processor, err := NewProcessor(registry, recoveryTestConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	process := func(task Task) <-chan struct{} {
		done := make(chan struct{})
		workers.Go(func() {
			defer close(done)
			processor.process(t.Context(), dispatchItem{source: source, ref: Reference{
				Source: task.Source, DeletionID: task.Record.DeletionID, MarkerFingerprint: task.MarkerFingerprint,
			}, failed: &task})
		})
		return done
	}
	process(blocked)
	defer func() { close(release); workers.Wait() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("blocked probe did not start")
	}
	for _, task := range []Task{blocked, other} {
		select {
		case <-process(task):
		case <-time.After(time.Second):
			t.Fatal("duplicate observation or independent resource was blocked by probe ownership")
		}
	}
	select {
	case <-entered:
		t.Fatal("same failed observation entered its domain probe twice")
	default:
	}
	if handler.calls.Load() != 1 {
		t.Fatal("independent resource was not finalized while the first probe remained blocked")
	}
}
