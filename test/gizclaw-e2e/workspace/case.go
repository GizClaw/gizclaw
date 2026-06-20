package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

type workspaceCase string

const (
	workspaceCasePushToTalkRoundtrip workspaceCase = "push-to-talk-roundtrip"
	workspaceCasePushToTalkInterrupt workspaceCase = "push-to-talk-interrupt"
	workspaceCaseRealtimeRoundtrip   workspaceCase = "realtime-roundtrip"
	workspaceCaseRealtimeInterrupt   workspaceCase = "realtime-interrupt"
	workspaceCaseHumanReview         workspaceCase = "human-review"
)

var supportedWorkspaceCases = []workspaceCase{
	workspaceCasePushToTalkRoundtrip,
	workspaceCasePushToTalkInterrupt,
	workspaceCaseRealtimeRoundtrip,
	workspaceCaseRealtimeInterrupt,
	workspaceCaseHumanReview,
}

type workspaceCaseResult struct {
	Rounds     []roundStats
	Interrupts []interruptStats
}

func parseWorkspaceCase(raw string) (workspaceCase, error) {
	value := workspaceCase(strings.ToLower(strings.TrimSpace(raw)))
	if value == "" {
		return "", fmt.Errorf("-case is required; supported cases: %s", supportedWorkspaceCaseNames())
	}
	for _, supported := range supportedWorkspaceCases {
		if value == supported {
			return value, nil
		}
	}
	return "", fmt.Errorf("unsupported -case %q; supported cases: %s", raw, supportedWorkspaceCaseNames())
}

func supportedWorkspaceCaseNames() string {
	names := make([]string, 0, len(supportedWorkspaceCases))
	for _, value := range supportedWorkspaceCases {
		names = append(names, string(value))
	}
	return strings.Join(names, ", ")
}

func (c workspaceCase) applyConfig(cfg config) (config, error) {
	if strings.TrimSpace(cfg.Workflow.Name) == "" {
		return config{}, fmt.Errorf("workflow.name is required")
	}
	cfg.Workspace = workspaceNameForCase(cfg.Workflow.Name, c)
	switch c {
	case workspaceCasePushToTalkRoundtrip, workspaceCasePushToTalkInterrupt, workspaceCaseHumanReview:
		cfg.Workflow.Parameters.Input = string(rpcapi.WorkspaceInputModePushToTalk)
		if c == workspaceCaseHumanReview && cfg.Rounds < 3 {
			cfg.Rounds = 3
		}
	case workspaceCaseRealtimeRoundtrip, workspaceCaseRealtimeInterrupt:
		cfg.Workflow.Parameters.Input = string(rpcapi.WorkspaceInputModeRealtime)
	default:
		return config{}, fmt.Errorf("unsupported workspace case %q", c)
	}
	return cfg, nil
}

func workspaceNameForCase(workflowName string, selected workspaceCase) string {
	name := strings.Trim(strings.ToLower(strings.TrimSpace(workflowName))+"-"+string(selected), "-")
	replacer := strings.NewReplacer("_", "-", ".", "-", " ", "-")
	return compactWorkspaceName(replacer.Replace(name))
}

func compactWorkspaceName(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func (d *personaDriver) runCase(ctx context.Context, selected workspaceCase) (workspaceCaseResult, error) {
	switch selected {
	case workspaceCasePushToTalkRoundtrip:
		rounds, err := d.runPushToTalkRoundtrip(ctx)
		return workspaceCaseResult{Rounds: rounds}, err
	case workspaceCaseRealtimeRoundtrip:
		rounds, err := d.runRealtimeRoundtrip(ctx)
		return workspaceCaseResult{Rounds: rounds}, err
	case workspaceCasePushToTalkInterrupt:
		interrupts, err := d.runPushToTalkInterrupt(ctx)
		return workspaceCaseResult{Interrupts: interrupts}, err
	case workspaceCaseRealtimeInterrupt:
		interrupts, err := d.runRealtimeInterrupt(ctx)
		return workspaceCaseResult{Interrupts: interrupts}, err
	case workspaceCaseHumanReview:
		rounds, err := d.runHumanReview(ctx)
		return workspaceCaseResult{Rounds: rounds}, err
	default:
		return workspaceCaseResult{}, fmt.Errorf("unsupported workspace case %q", selected)
	}
}
