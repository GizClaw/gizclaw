package giztest

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func timingDocument() *Document {
	return &Document{Name: "timing", Path: "timing.giztest.yaml", Repeat: 4,
		Steps:   []Step{{ID: "first", RPC: &RPCOperation{Method: "all.ping"}}, {ID: "second", RPC: &RPCOperation{Method: "all.ping"}}},
		Finally: []Step{{ID: "cleanup", RPC: &RPCOperation{Method: "all.ping"}}},
	}
}

func TestTimingOffsets(t *testing.T) {
	seed := int64(42)
	doc := timingDocument()
	doc.StartJitter, doc.Stagger, doc.StepJitter, doc.Seed = "30s", "2s", "3s", &seed
	config, err := doc.timing(TimingOverrides{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[time.Duration]bool{}
	for index := range 1000 {
		a := config.plan(doc, index, 99)
		b := config.plan(doc, index, 100)
		base := time.Duration(index) * 2 * time.Second
		if a.startOffset < base || a.startOffset >= base+30*time.Second {
			t.Fatalf("index %d: offset %s", index, a.startOffset)
		}
		if a.startOffset != b.startOffset {
			t.Fatal("document seed did not override run seed")
		}
		seen[a.startOffset-base] = true
	}
	if len(seen) < 900 {
		t.Fatalf("only %d distinct jitter values", len(seen))
	}
	config.startJitter = 0
	for i := range doc.Repeat {
		if got := config.plan(doc, i, seed).startOffset; got != time.Duration(i)*2*time.Second {
			t.Fatalf("stagger %d = %s", i, got)
		}
	}
	config.stagger = 0
	if got := config.plan(doc, 3, seed).startOffset; got != 0 {
		t.Fatalf("default offset = %s", got)
	}
}

func TestTimingOverrides(t *testing.T) {
	seed, zeroSeed := int64(17), int64(0)
	doc := timingDocument()
	doc.StartJitter, doc.Stagger, doc.StepJitter, doc.Seed = "30s", "2s", "3s", &seed
	zero := "0"
	config, err := doc.timing(TimingOverrides{StartJitter: &zero, Stagger: &zero, StepJitter: &zero, Seed: &zeroSeed})
	if err != nil {
		t.Fatal(err)
	}
	if config.startJitter != 0 || config.stagger != 0 || config.stepJitter != 0 || *config.seed != 0 {
		t.Fatalf("config = %+v", config)
	}
	if doc.StartJitter != "30s" || *doc.Seed != 17 {
		t.Fatal("override mutated document")
	}
	only := "4s"
	config, err = doc.timing(TimingOverrides{Stagger: &only})
	if err != nil || config.startJitter != 30*time.Second || config.stagger != 4*time.Second || config.stepJitter != 3*time.Second || *config.seed != 17 {
		t.Fatalf("config = %+v, error = %v", config, err)
	}
	for _, value := range []string{"", "-1s", "tomorrow", "0..3s"} {
		if err := ValidateTiming([]*Document{doc}, TimingOverrides{StepJitter: &value}); err == nil {
			t.Fatalf("accepted override %q", value)
		}
	}
}

func TestTimingValidation(t *testing.T) {
	for _, fields := range []string{
		"start_jitter: 30s\nstagger: 2s\nstep_jitter: 3s\nseed: 0\n",
		"start_jitter: '0'\nstagger: 0s\nstep_jitter: 1h2m3.5s\nseed: 9007199254740991\n",
		"start_jitter: 9223372036854775807ns\n",
		"step_jitter: 0.5ns\n",
	} {
		_, err := LoadDocument(writeTestDocument(t, strings.Replace(validDocument, "clients:\n", fields+"clients:\n", 1)), nil)
		if err != nil {
			t.Fatalf("valid %q: %v", fields, err)
		}
	}
	for _, fields := range []string{
		"start_jitter: -1s\n", "stagger: nonsense\n", "step_jitter: 0..3s\n", "start_jitter: ''\n",
		"start_jitter: 0\n", "start_jitter: null\n", "step_jitter: {min: 0s, max: 3s}\n", "start_jitter: 1e6s\n",
		"seed: -1\n", "seed: 1.5\n", "seed: '1'\n", "seed: null\n", "seed: 9007199254740992\n",
		"start_jitter: 9223372036854775808ns\n", "repeat: 3\nstagger: 4611686018427387904ns\n",
		"repeat: 2\nstagger: 9223372036854775807ns\nstart_jitter: 2ns\n", "start_jitter_mode: uniform\n",
	} {
		_, err := LoadDocument(writeTestDocument(t, strings.Replace(validDocument, "clients:\n", fields+"clients:\n", 1)), nil)
		if err == nil {
			t.Fatalf("accepted invalid %q", fields)
		}
	}
	doc := timingDocument()
	doc.Steps = append(doc.Steps, Step{ID: "barrier", Barrier: &BarrierOperation{}})
	for _, fields := range []TimingOverrides{{StartJitter: new("1s")}, {Stagger: new("1s")}, {StepJitter: new("1s")}} {
		if err := ValidateTiming([]*Document{doc}, fields); err == nil || !strings.Contains(err.Error(), "barrier") {
			t.Fatalf("barrier validation = %v", err)
		}
	}
	if err := ValidateTiming([]*Document{doc}, TimingOverrides{}); err != nil {
		t.Fatal(err)
	}
}

func TestTimingReproducibleAcrossWorkersAndDocumentSelection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		doc := timingDocument()
		doc.StartJitter, doc.StepJitter = "30s", "3s"
		first := Run(context.Background(), []*Document{doc}, Options{Driver: &stubDriver{}, Parallel: 4})
		other := timingDocument()
		other.Name, other.Path = "other", "other.giztest.yaml"
		second := Run(context.Background(), []*Document{other, doc}, Options{Driver: &stubDriver{}, Parallel: 1, Timing: TimingOverrides{Seed: &first.Seed}})
		if first.Status != "passed" || second.Status != "passed" {
			t.Fatalf("run status: %s / %s", first.Status, second.Status)
		}
		for i, a := range first.Tasks {
			b := second.Tasks[i+other.Repeat]
			if a.Seed != first.Seed || b.Seed != a.Seed || a.PlannedStartOffsetMS != b.PlannedStartOffsetMS {
				t.Fatalf("task %d did not reproduce", i)
			}
			for j := range a.Steps {
				if a.Steps[j].PlannedDelayMS != b.Steps[j].PlannedDelayMS {
					t.Fatalf("task %d step %d jitter changed", i, j)
				}
			}
		}
	})
}

func TestTimingScheduleAndReport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		doc := timingDocument()
		doc.Stagger, doc.Timeout = "1s", "100ms"
		report := Run(context.Background(), []*Document{doc}, Options{Driver: &stubDriver{}, Parallel: 1})
		if report.Status != "passed" {
			t.Fatalf("report = %+v", report)
		}
		for i, task := range report.Tasks {
			want := float64(i) * 1000
			if task.PlannedStartOffsetMS != want || task.ActualStartOffsetMS == nil || *task.ActualStartOffsetMS != want {
				t.Fatalf("task %d = %+v", i, task)
			}
			if !task.StartedAt.Equal(report.StartedAt.Add(time.Duration(i) * time.Second)) {
				t.Fatal("task start timestamp differs from offset")
			}
			for _, step := range append(task.Steps, task.Cleanup...) {
				if step.StartOffsetMS == nil || *step.StartOffsetMS != want || step.StartedAt == nil || !step.StartedAt.Equal(*task.StartedAt) {
					t.Fatalf("step = %+v", step)
				}
			}
		}
		encoded, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(encoded, &object); err != nil {
			t.Fatal(err)
		}
		task := object["tasks"].([]any)[0].(map[string]any)
		for _, key := range []string{"seed", "start_jitter", "stagger", "step_jitter", "planned_start_offset_ms", "actual_start_offset_ms", "started_at"} {
			if _, ok := task[key]; !ok {
				t.Fatalf("missing report field %s", key)
			}
		}
		step := task["steps"].([]any)[0].(map[string]any)
		for _, key := range []string{"started_at", "start_offset_ms", "planned_delay_ms"} {
			if _, ok := step[key]; !ok {
				t.Fatalf("missing step field %s", key)
			}
		}
	})
}

func TestTimingDueOrderAndWorkerLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		doc := timingDocument()
		doc.StartJitter, doc.Seed = "1s", new(int64(42))
		config, _ := doc.timing(TimingOverrides{})
		for _, parallel := range []int{1, 4} {
			report := Run(context.Background(), []*Document{doc}, Options{Driver: &stubDriver{execute: func(_ context.Context, req StepRequest) (StepResult, error) {
				if req.Step.ID == "first" {
					time.Sleep(2 * time.Second)
				}
				return StepResult{}, nil
			}}, Parallel: parallel})
			starts := make([]float64, 0, doc.Repeat)
			for i, task := range report.Tasks {
				plan := milliseconds(config.plan(doc, i, 42).startOffset)
				if task.ActualStartOffsetMS == nil || *task.ActualStartOffsetMS < plan {
					t.Fatalf("started before due: %+v", task)
				}
				if parallel == 4 && *task.ActualStartOffsetMS != plan {
					t.Fatalf("waiting task occupied a slot: %+v", task)
				}
				starts = append(starts, *task.ActualStartOffsetMS)
			}
			if parallel == 1 {
				for i, a := range starts {
					for j, b := range starts {
						if i != j && a >= b && a-b < 2000 {
							t.Fatal("parallel=1 overlapped tasks")
						}
					}
				}
			}
		}
	})
}

func TestTimingThinkTimeAndCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		doc := timingDocument()
		doc.Repeat, doc.StepJitter, doc.Seed = 1, "3s", new(int64(42))
		doc.Steps[1].Retry = &RetrySpec{Attempts: 2, On: []string{"assertion"}}
		calls := 0
		driver := &stubDriver{execute: func(_ context.Context, req StepRequest) (StepResult, error) {
			if req.Step.ID == "second" {
				calls++
				if calls == 1 {
					return StepResult{}, NewAssertionError(errors.New("try again"))
				}
			}
			return StepResult{}, nil
		}}
		report := Run(context.Background(), []*Document{doc}, Options{Driver: driver, Parallel: 1})
		task := report.Tasks[0]
		second := task.Steps[1]
		if report.Status != "passed" || second.PlannedDelayMS <= 0 || second.PlannedDelayMS >= 3000 {
			t.Fatalf("report = %+v", report)
		}
		if *second.StartOffsetMS != second.PlannedDelayMS || *task.Steps[0].StartOffsetMS != 0 {
			t.Fatal("jitter did not occur between steps")
		}
		for _, attempt := range second.Attempts {
			if *attempt.StartOffsetMS != *second.StartOffsetMS {
				t.Fatal("retry received think-time jitter")
			}
		}
		if *task.Cleanup[0].StartOffsetMS != *second.StartOffsetMS || task.Cleanup[0].PlannedDelayMS != 0 {
			t.Fatal("cleanup was delayed")
		}
	})
}

func TestTimingCancellation(t *testing.T) {
	for _, think := range []bool{false, true} {
		t.Run(map[bool]string{false: "scheduled", true: "think-time"}[think], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				doc := timingDocument()
				doc.Stagger = "1h"
				if think {
					doc.Repeat, doc.StepJitter, doc.Timeout = 1, "1h", "2h"
				}
				ctx, cancel := context.WithCancelCause(context.Background())
				defer cancel(nil)
				cause := errors.New("operator stopped load")
				go func() { time.Sleep(time.Second); cancel(cause) }()
				report := Run(ctx, []*Document{doc}, Options{Driver: &stubDriver{}, Parallel: 4, Timing: TimingOverrides{Seed: new(int64(42))}})
				if report.DurationMS != 1000 || report.Status != "failed" {
					t.Fatalf("report = %+v", report)
				}
				if think {
					task := report.Tasks[0]
					if task.Steps[1].StartedAt != nil || task.Steps[1].StartOffsetMS != nil || task.Steps[1].Stage != "step_jitter" || task.Cleanup[0].Status != "passed" {
						t.Fatalf("task = %+v", task)
					}
				} else {
					for _, task := range report.Tasks[1:] {
						if task.ActualStartOffsetMS != nil || task.StartedAt != nil || task.Error != cause.Error() {
							t.Fatalf("cancelled task = %+v", task)
						}
					}
				}
			})
		})
	}
}

func TestTimingDocumentSeedAndZeroOverrideInReport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		doc := timingDocument()
		doc.Seed, doc.StartJitter, doc.StepJitter = new(int64(23)), "1h", "1h"
		zero := "0"
		report := Run(context.Background(), []*Document{doc}, Options{Driver: &stubDriver{}, Parallel: 4, Timing: TimingOverrides{StartJitter: &zero, StepJitter: &zero, Seed: new(int64(0))}})
		if report.Seed != 0 || report.DurationMS != 0 {
			t.Fatalf("report = %+v", report)
		}
		for _, task := range report.Tasks {
			if task.Seed != 0 || task.StartJitter != "0s" || !reflect.DeepEqual(task.ActualStartOffsetMS, new(float64(0))) {
				t.Fatalf("task = %+v", task)
			}
		}
	})
}

func TestTimingParallelChildReportsActualStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		doc := timingDocument()
		doc.Repeat, doc.StepJitter, doc.Seed = 1, "3s", new(int64(42))
		doc.Steps[1] = Step{ID: "group", Parallel: []Step{
			{ID: "immediate", Client: "peer", PeerStream: &PeerStreamOperation{Mode: "listen"}},
			{ID: "delayed", Client: "peer", Delay: "200ms", PeerStream: &PeerStreamOperation{Mode: "listen"}},
		}}
		driver := &stubDriver{prepareParallel: func(req StepRequest) (ParallelChild, error) {
			return parallelRun(func(context.Context) (StepResult, error) { return StepResult{}, nil }), nil
		}}
		report := Run(context.Background(), []*Document{doc}, Options{Driver: driver, Parallel: 1})
		group := report.Tasks[0].Steps[1]
		if report.Status != "passed" || len(group.Children) != 2 {
			t.Fatalf("report = %+v", report)
		}
		if *group.Children[0].StartOffsetMS != *group.StartOffsetMS || *group.Children[1].StartOffsetMS-*group.Children[0].StartOffsetMS != 200 {
			t.Fatalf("child timing changed: %+v", group.Children)
		}
		for _, child := range group.Children {
			if child.PlannedDelayMS != 0 {
				t.Fatal("child received think time")
			}
		}
	})
}

func TestTimingUnfinishedParallelChildRetainsStart(t *testing.T) {
	started := time.Now()
	child := &parallelChild{step: Step{ID: "unfinished"}, done: make(chan struct{}), started: make(chan time.Time, 1)}
	child.started <- started
	_, reports, _ := parallelReports([]*parallelChild{child}, time.Second, nil, started.Add(-time.Second))
	if reports[0].StartedAt == nil || !reports[0].StartedAt.Equal(started) || *reports[0].StartOffsetMS != 1000 {
		t.Fatalf("unfinished child = %+v", reports[0])
	}
}

func TestTimingLoadUsesEffectiveValues(t *testing.T) {
	body := strings.Split(validDocument, "steps:\n")[0] + "start_jitter: 1s\nstagger: 1s\nstep_jitter: 1s\nsteps:\n  - id: sync\n    barrier: {}\n"
	path := writeTestDocument(t, body)
	if _, err := LoadDocument(path, nil); err == nil {
		t.Fatal("validation accepted nonzero barrier timing")
	}
	zero := "0"
	partial := TimingOverrides{StartJitter: &zero}
	if _, err := LoadDocumentsWithTiming([]string{path}, nil, partial); err == nil {
		t.Fatal("partial override accepted remaining nonzero barrier delays")
	}
	timing := TimingOverrides{StartJitter: &zero, Stagger: &zero, StepJitter: &zero}
	docs, err := LoadDocumentsWithTiming([]string{path}, nil, timing)
	if err != nil {
		t.Fatal(err)
	}
	if docs[0].StartJitter != "1s" || docs[0].Stagger != "1s" || docs[0].StepJitter != "1s" {
		t.Fatal("load mutated declared timing")
	}
	supported, skipped, err := LoadSupportedDocumentsWithTiming([]string{path}, &stubDriver{}, timing)
	if err != nil || len(supported) != 1 || len(skipped) != 0 {
		t.Fatalf("supported=%d skipped=%d error=%v", len(supported), len(skipped), err)
	}
	bad := writeTestDocument(t, strings.Replace(body, "start_jitter: 1s", "start_jitter: -1s", 1))
	if _, err := LoadDocumentsWithTiming([]string{bad}, nil, timing); err == nil {
		t.Fatal("override bypassed document schema validation")
	}
}
