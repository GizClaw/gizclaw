package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type InvokeRequest struct {
	Build          BuildRequest
	CallID         string
	DeclaredToolID string
	Name           string
	Args           json.RawMessage
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
	tool, ok := kit.FindByName(name)
	if !ok {
		tool, ok = kit.Find(name)
	}
	if !ok {
		return Result{}, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	if toolID := strings.TrimSpace(req.DeclaredToolID); toolID != "" && tool.ID != toolID {
		return Result{}, fmt.Errorf("%w: declared tool %q no longer resolves from name %q", ErrToolNotFound, toolID, name)
	}
	args := normalizeToolArgs(req.Args)
	if err := validateToolArgs(tool, args); err != nil {
		return Result{}, err
	}
	return executors.Invoke(ctx, Call{
		ID:        req.CallID,
		Tool:      tool,
		Args:      cloneRaw(args),
		SubjectID: strings.TrimSpace(req.Build.OwnerPublicKey),
	})
}
