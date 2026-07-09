package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type InvokeRequest struct {
	Build  BuildRequest
	CallID string
	Name   string
	Args   json.RawMessage
}

func (b *Builder) Invoke(ctx context.Context, executors *ExecutorRegistry, req InvokeRequest) (Result, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Result{}, fmt.Errorf("%w: tool call name is required", ErrInvalidTool)
	}
	kit, err := b.Build(ctx, req.Build)
	if err != nil {
		return Result{}, err
	}
	tool, ok := kit.Find(name)
	if !ok {
		tool, ok = kit.FindByName(name)
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return executors.Invoke(ctx, Call{
		ID:        req.CallID,
		Tool:      tool,
		Args:      cloneRaw(req.Args),
		SubjectID: strings.TrimSpace(req.Build.Subject.Id),
	})
}
