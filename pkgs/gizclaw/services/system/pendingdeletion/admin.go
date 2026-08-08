package pendingdeletion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultListLimit = 100
	maxListLimit     = 500
)

// ListRequest selects active tasks. Start is inclusive and End is exclusive.
type ListRequest struct {
	Source    string
	Kind      Kind
	Status    Status
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Cursor    string
}

// ListResult is one bounded active-task page.
type ListResult struct {
	Tasks      []Task
	NextCursor string
}

type listCursor struct {
	Source     string `json:"source,omitempty"`
	Kind       Kind   `json:"kind,omitempty"`
	Status     Status `json:"status,omitempty"`
	Start      string `json:"start,omitempty"`
	End        string `json:"end,omitempty"`
	CreatedAt  string `json:"created_at"`
	LastSource string `json:"last_source"`
	DeletionID string `json:"deletion_id"`
}

// Admin aggregates active tasks across registered sources.
type Admin struct {
	registry *Registry
	now      func() time.Time
	wake     func()
}

// NewAdmin constructs an operator service over a validated registry.
func NewAdmin(registry *Registry, wake func()) *Admin {
	return &Admin{registry: registry, now: time.Now, wake: wake}
}

// List returns active tasks in (created_at, source, deletion_id) order.
func (a *Admin) List(ctx context.Context, request ListRequest) (ListResult, error) {
	if a == nil || a.registry == nil {
		return ListResult{}, errors.New("pending deletion: admin is not configured")
	}
	if request.Limit == 0 {
		request.Limit = defaultListLimit
	}
	if request.Limit < 1 || request.Limit > maxListLimit {
		return ListResult{}, fmt.Errorf("%w: invalid limit", ErrInvalid)
	}
	if request.StartTime != nil && request.EndTime != nil && !request.EndTime.After(*request.StartTime) {
		return ListResult{}, fmt.Errorf("%w: end time must be after start time", ErrInvalid)
	}
	if request.Kind != "" && !request.Kind.valid() {
		return ListResult{}, fmt.Errorf("%w: invalid kind", ErrInvalid)
	}
	if request.Status != "" {
		switch request.Status {
		case StatusQueued, StatusRunning, StatusRetryWait, StatusFailed:
		default:
			return ListResult{}, fmt.Errorf("%w: invalid status", ErrInvalid)
		}
	}
	cursor, err := decodeListCursor(request)
	if err != nil {
		return ListResult{}, err
	}
	sources, err := a.selectedSources(request.Source)
	if err != nil {
		return ListResult{}, err
	}
	options := SourceListOptions{Limit: request.Limit + 1}
	if request.Kind != "" {
		options.Kinds = map[Kind]bool{request.Kind: true}
	}
	if request.Status != "" {
		options.Statuses = map[Status]bool{request.Status: true}
	}
	options.StartTime = request.StartTime
	options.EndTime = request.EndTime
	if cursor != nil {
		createdAt, parseErr := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
		if parseErr != nil {
			return ListResult{}, fmt.Errorf("%w: invalid cursor timestamp", ErrInvalid)
		}
		options.AfterCreatedAt = &createdAt
		options.AfterSource = cursor.LastSource
		options.AfterDeletionID = cursor.DeletionID
	}
	all := make([]Task, 0, len(sources)*(request.Limit+1))
	for _, source := range sources {
		tasks, listErr := source.ListTasks(ctx, options)
		if listErr != nil {
			return ListResult{}, listErr
		}
		for _, task := range tasks {
			if validateErr := ValidateTask(task); validateErr != nil {
				return ListResult{}, fmt.Errorf("pending deletion: source %q returned invalid task: %v", source.Name(), validateErr)
			}
		}
		all = append(all, tasks...)
	}
	sort.Slice(all, func(i, j int) bool { return taskLess(all[i], all[j]) })
	result := ListResult{Tasks: all}
	if len(result.Tasks) > request.Limit {
		result.Tasks = result.Tasks[:request.Limit]
		last := result.Tasks[len(result.Tasks)-1]
		encoded, encodeErr := encodeListCursor(request, last)
		if encodeErr != nil {
			return ListResult{}, encodeErr
		}
		result.NextCursor = encoded
	}
	return result, nil
}

// Get returns one active task from an explicit source.
func (a *Admin) Get(ctx context.Context, sourceName, deletionID string) (Task, error) {
	source, err := a.requireSource(sourceName)
	if err != nil {
		return Task{}, err
	}
	if !validDeletionID(deletionID) {
		return Task{}, fmt.Errorf("%w: invalid deletion ID", ErrInvalid)
	}
	task, err := source.GetTask(ctx, deletionID)
	if err != nil {
		return Task{}, err
	}
	if err := ValidateTask(task); err != nil {
		return Task{}, fmt.Errorf("pending deletion: source %q returned invalid task: %v", source.Name(), err)
	}
	return task, nil
}

// Retry requeues one failed active task.
func (a *Admin) Retry(ctx context.Context, sourceName, deletionID string) (Task, error) {
	source, err := a.requireSource(sourceName)
	if err != nil {
		return Task{}, err
	}
	if !validDeletionID(deletionID) {
		return Task{}, fmt.Errorf("%w: invalid deletion ID", ErrInvalid)
	}
	task, err := source.Retry(ctx, deletionID, a.now().UTC())
	if err != nil {
		return Task{}, err
	}
	if err := ValidateTask(task); err != nil {
		return Task{}, fmt.Errorf("pending deletion: source returned invalid retried task: %v", err)
	}
	if a.wake != nil {
		a.wake()
	}
	return task, nil
}

func (a *Admin) requireSource(name string) (Source, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || name != trimmed {
		return nil, fmt.Errorf("%w: source is required", ErrInvalid)
	}
	source, ok := a.registry.source(trimmed)
	if !ok {
		return nil, fmt.Errorf("%w: unknown source", ErrInvalid)
	}
	return source, nil
}

func (a *Admin) selectedSources(name string) ([]Source, error) {
	if name == "" {
		return a.registry.sources(), nil
	}
	source, err := a.requireSource(name)
	if err != nil {
		return nil, err
	}
	return []Source{source}, nil
}

func taskLess(a, b Task) bool {
	if !a.Record.DeletedAt.Equal(b.Record.DeletedAt) {
		return a.Record.DeletedAt.Before(b.Record.DeletedAt)
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	return a.Record.DeletionID < b.Record.DeletionID
}

func decodeListCursor(request ListRequest) (*listCursor, error) {
	if request.Cursor == "" {
		return nil, nil
	}
	if request.Cursor != strings.TrimSpace(request.Cursor) {
		return nil, fmt.Errorf("%w: invalid cursor", ErrInvalid)
	}
	data, err := base64.RawURLEncoding.DecodeString(request.Cursor)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", ErrInvalid)
	}
	var cursor listCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("%w: invalid cursor", ErrInvalid)
	}
	if cursor.Source != request.Source || cursor.Kind != request.Kind || cursor.Status != request.Status ||
		cursor.Start != formatOptionalTime(request.StartTime) || cursor.End != formatOptionalTime(request.EndTime) {
		return nil, fmt.Errorf("%w: cursor filters do not match", ErrInvalid)
	}
	if cursor.CreatedAt == "" || !sourceNamePattern.MatchString(cursor.LastSource) || !validDeletionID(cursor.DeletionID) {
		return nil, fmt.Errorf("%w: incomplete cursor", ErrInvalid)
	}
	return &cursor, nil
}

func validDeletionID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	_, err := uuid.Parse(value)
	return err == nil
}

func encodeListCursor(request ListRequest, task Task) (string, error) {
	data, err := json.Marshal(listCursor{
		Source: request.Source, Kind: request.Kind, Status: request.Status,
		Start: formatOptionalTime(request.StartTime), End: formatOptionalTime(request.EndTime),
		CreatedAt:  task.Record.DeletedAt.UTC().Format(time.RFC3339Nano),
		LastSource: task.Source, DeletionID: task.Record.DeletionID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
