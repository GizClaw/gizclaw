package giztest

import "context"

type taskIDKey struct{}

// TaskID returns the report's task ID while a Driver opens or executes a task.
// It is empty outside Run. Together with Document.Path it identifies a repeat
// independently of worker scheduling, including parallel children and cleanup.
func TaskID(ctx context.Context) string {
	id, _ := ctx.Value(taskIDKey{}).(string)
	return id
}
