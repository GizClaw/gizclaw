package agenthost

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
)

// ToolkitContext is the resolved ToolKit runtime available to an agent factory.
type ToolkitContext struct {
	Builder      *toolkit.Builder
	Executors    *toolkit.ExecutorRegistry
	BuildRequest toolkit.BuildRequest
}

func (c *ToolkitContext) BuildToolkit(ctx context.Context) (toolkit.ToolKit, error) {
	if c == nil {
		return toolkit.ToolKit{}, nil
	}
	if c.Builder == nil {
		return toolkit.ToolKit{}, fmt.Errorf("%w: builder is required", toolkit.ErrNotConfigured)
	}
	return c.Builder.Build(ctx, c.BuildRequest)
}

func (c *ToolkitContext) Invoke(ctx context.Context, callID, name string, args json.RawMessage) (toolkit.Result, error) {
	if c == nil || c.Builder == nil || c.Executors == nil {
		return toolkit.Result{}, toolkit.ErrNotConfigured
	}
	return c.Builder.Invoke(ctx, c.Executors, toolkit.InvokeRequest{
		Build:  c.BuildRequest,
		CallID: callID,
		Name:   name,
		Args:   args,
	})
}
