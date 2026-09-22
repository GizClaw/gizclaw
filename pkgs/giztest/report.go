package giztest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Report struct {
	// Seed is the generated or CLI-supplied run seed. Tasks record their effective seed.
	Seed       int64        `json:"seed"`
	Version    string       `json:"version"`
	Status     string       `json:"status"`
	StartedAt  time.Time    `json:"started_at"`
	DurationMS int64        `json:"duration_ms"`
	Tasks      []TaskReport `json:"tasks"`
}
type TaskReport struct {
	Seed                 int64   `json:"seed"`
	StartJitter          string  `json:"start_jitter"`
	Stagger              string  `json:"stagger"`
	StepJitter           string  `json:"step_jitter"`
	PlannedStartOffsetMS float64 `json:"planned_start_offset_ms"`
	// ActualStartOffsetMS is nil when cancelled before client setup starts.
	ActualStartOffsetMS *float64          `json:"actual_start_offset_ms"`
	StartedAt           *time.Time        `json:"started_at,omitempty"`
	Path                string            `json:"path"`
	Name                string            `json:"name"`
	TaskID              string            `json:"task_id"`
	Status              string            `json:"status"`
	RepeatIndex         int               `json:"repeat_index"`
	DurationMS          int64             `json:"duration_ms"`
	Clients             map[string]string `json:"clients,omitempty"`
	Steps               []StepReport      `json:"steps"`
	Cleanup             []StepReport      `json:"cleanup,omitempty"`
	Error               string            `json:"error,omitempty"`
}
type StepReport struct {
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	StartOffsetMS  *float64        `json:"start_offset_ms"`
	PlannedDelayMS float64         `json:"planned_delay_ms"`
	ID             string          `json:"id"`
	Operation      string          `json:"operation"`
	Client         string          `json:"client,omitempty"`
	Status         string          `json:"status"`
	Stage          string          `json:"stage"`
	DurationMS     int64           `json:"duration_ms"`
	Error          string          `json:"error,omitempty"`
	Evidence       map[string]any  `json:"evidence,omitempty"`
	Attempts       []AttemptReport `json:"attempts,omitempty"`
	// Children reports the outcome of every child of a parallel step, so one
	// child's failure stays visible next to the siblings that succeeded.
	Children []StepReport `json:"children,omitempty"`
}
type AttemptReport struct {
	StartedAt     *time.Time     `json:"started_at,omitempty"`
	StartOffsetMS *float64       `json:"start_offset_ms"`
	Attempt       int            `json:"attempt"`
	Status        string         `json:"status"`
	FailureKind   string         `json:"failure_kind,omitempty"`
	DurationMS    int64          `json:"duration_ms"`
	Error         string         `json:"error,omitempty"`
	Evidence      map[string]any `json:"evidence,omitempty"`
}

func (r *Report) finish(start time.Time) {
	r.DurationMS = time.Since(start).Milliseconds()
	r.Status = "passed"
	for _, task := range r.Tasks {
		if task.Status != "passed" {
			r.Status = "failed"
			break
		}
	}
	sort.Slice(r.Tasks, func(i, j int) bool {
		if r.Tasks[i].Path != r.Tasks[j].Path {
			return r.Tasks[i].Path < r.Tasks[j].Path
		}
		return r.Tasks[i].RepeatIndex < r.Tasks[j].RepeatIndex
	})
}
func WriteReport(path string, report Report) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".giztest-report-*.json")
	if err != nil {
		return err
	}
	name := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	enc := json.NewEncoder(temp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("commit report: %w", err)
	}
	ok = true
	return nil
}
