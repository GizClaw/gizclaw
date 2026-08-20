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
	Version    string       `json:"version"`
	Status     string       `json:"status"`
	StartedAt  time.Time    `json:"started_at"`
	DurationMS int64        `json:"duration_ms"`
	Tasks      []TaskReport `json:"tasks"`
}
type TaskReport struct {
	Path        string            `json:"path"`
	Name        string            `json:"name"`
	TaskID      string            `json:"task_id"`
	Status      string            `json:"status"`
	RepeatIndex int               `json:"repeat_index"`
	DurationMS  int64             `json:"duration_ms"`
	Clients     map[string]string `json:"clients,omitempty"`
	Steps       []StepReport      `json:"steps"`
	Cleanup     []StepReport      `json:"cleanup,omitempty"`
	Error       string            `json:"error,omitempty"`
}
type StepReport struct {
	ID         string         `json:"id"`
	Operation  string         `json:"operation"`
	Client     string         `json:"client,omitempty"`
	Status     string         `json:"status"`
	Stage      string         `json:"stage"`
	DurationMS int64          `json:"duration_ms"`
	Error      string         `json:"error,omitempty"`
	Evidence   map[string]any `json:"evidence,omitempty"`
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
func writeReport(path string, report Report) error {
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
