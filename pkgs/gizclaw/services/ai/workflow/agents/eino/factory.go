package eino

import (
	"context"
	"fmt"
	"strings"
	"time"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	einoagent "github.com/GizClaw/gizclaw-go/pkgs/agent/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/components/model"
)

const Type = "eino"

const defaultToolTimeout = 30 * time.Second

type Factory struct {
	GenX    *peergenx.Service
	History logstore.MutableStore
	Memory  memory.Store

	modelFactory func(context.Context, *peergenx.Service, string) (model.ToolCallingChatModel, error)
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	config, err := f.config(ctx, spec)
	if err != nil {
		return nil, err
	}
	runtime, err := einoagent.New(ctx, config)
	if err != nil {
		return nil, err
	}
	return agenthost.NewTransformerAgent(runtime), nil
}

func (f Factory) config(ctx context.Context, spec agenthost.Spec) (einoagent.Config, error) {
	if f.GenX == nil {
		return einoagent.Config{}, fmt.Errorf("eino: peergenx service is required")
	}
	workflow := spec.Workflow.Spec.Eino
	if workflow == nil {
		return einoagent.Config{}, fmt.Errorf("eino: workflow eino spec is required")
	}
	modelID := strings.TrimSpace(workflow.Model)
	if modelID == "" {
		return einoagent.Config{}, fmt.Errorf("eino: model is required")
	}
	chatModel, err := f.chatModel(ctx, modelID)
	if err != nil {
		return einoagent.Config{}, err
	}
	toolkit := commonagent.EmptyToolkit()
	if spec.Toolkit != nil {
		toolkit, err = spec.Toolkit.BuildAgentToolkit(ctx)
		if err != nil {
			return einoagent.Config{}, fmt.Errorf("eino: build toolkit: %w", err)
		}
	}
	config := einoagent.Config{Model: chatModel, Toolkit: toolkit, Memory: f.Memory, ToolTimeout: defaultToolTimeout}
	if workflow.SystemPrompt != nil {
		config.SystemPrompt = *workflow.SystemPrompt
	}
	if workflow.MaxSteps != nil {
		config.MaxSteps = *workflow.MaxSteps
	}
	if workflow.MaxToolCalls != nil {
		config.MaxToolCalls = *workflow.MaxToolCalls
	}
	if f.History != nil {
		stream := "agent.eino." + strings.TrimSpace(spec.Workspace.Name)
		if stream == "agent.eino." {
			return einoagent.Config{}, fmt.Errorf("eino: workspace name is required when history is configured")
		}
		config.History = &einoagent.HistoryConfig{Store: f.History, Stream: stream, RecentLimit: 100}
	}
	return config, nil
}

func (f Factory) chatModel(ctx context.Context, modelID string) (model.ToolCallingChatModel, error) {
	if f.modelFactory != nil {
		return f.modelFactory(ctx, f.GenX, modelID)
	}
	if _, err := f.GenX.ResolveGenerator(ctx, "model/"+modelID); err != nil {
		return nil, fmt.Errorf("eino: resolve model %q: %w", modelID, err)
	}
	chatModel, err := einoagent.NewGenXChatModel(f.GenX.Generator(), "model/"+modelID)
	if err != nil {
		return nil, err
	}
	return chatModel, nil
}
