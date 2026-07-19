package dashscoperealtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	dashscopeagent "github.com/GizClaw/gizclaw-go/pkgs/agent/dashscoperealtime"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "dashscope-realtime"

const defaultToolTimeout = 30 * time.Second

type Factory struct{ Transformer genx.Transformer }

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if f.Transformer == nil {
		return nil, fmt.Errorf("dashscoperealtime: transformer is required")
	}
	workflow := spec.Workflow.Spec.DashscopeRealtime
	if workflow == nil {
		return nil, fmt.Errorf("dashscoperealtime: workflow dashscope_realtime spec is required")
	}
	pattern := normalizeModelPattern(workflow.Model)
	if pattern == "" {
		return nil, fmt.Errorf("dashscoperealtime: model is required")
	}
	providerModel := ""
	if workflow.ProviderModel != nil && strings.TrimSpace(*workflow.ProviderModel) != "" {
		providerModel = strings.TrimSpace(*workflow.ProviderModel)
	}
	toolkit := commonagent.EmptyToolkit()
	if spec.Toolkit != nil {
		resolved, err := spec.Toolkit.BuildAgentToolkit(ctx)
		if err != nil {
			return nil, fmt.Errorf("dashscoperealtime: build toolkit: %w", err)
		}
		toolkit = resolved
	}
	maximum := 0
	if workflow.MaxToolCalls != nil {
		maximum = *workflow.MaxToolCalls
	}
	runtime, err := dashscopeagent.New(dashscopeagent.Config{
		Transformer: f.Transformer, Pattern: pattern, Model: providerModel, Toolkit: toolkit,
		MaxToolCalls: maximum, ToolTimeout: defaultToolTimeout,
	})
	if err != nil {
		return nil, err
	}
	return agenthost.NewTransformerAgent(runtime), nil
}

func normalizeModelPattern(pattern string) string {
	pattern = strings.Trim(strings.TrimSpace(pattern), "/")
	if pattern == "" || strings.Contains(pattern, "/") {
		return pattern
	}
	return "model/" + pattern
}
