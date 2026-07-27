package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type InvokeRequest struct {
	Build BuildRequest
	Name  string
	Args  json.RawMessage
}

// ResolveInvoke re-reads and reauthorizes a Tool immediately before dispatch.
func (b *Builder) ResolveInvoke(ctx context.Context, req InvokeRequest) (Tool, json.RawMessage, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Tool{}, nil, fmt.Errorf("%w: tool name is required", ErrInvalidTool)
	}
	kit, err := b.Build(ctx, req.Build)
	if err != nil {
		return Tool{}, nil, err
	}
	tool, ok := kit.Find(name)
	if !ok {
		return Tool{}, nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	args := normalizeToolArgs(req.Args)
	if err := validateToolArgs(tool, args); err != nil {
		return Tool{}, nil, err
	}
	return tool, cloneRaw(args), nil
}
