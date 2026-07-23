package chatroom

import (
	"context"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxchatroom "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/chatroom"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

const Type = "chatroom"

// Factory adapts GizClaw workspace configuration to the reusable Chatroom
// Transformer. It owns no stream or provider lifecycle.
type Factory struct {
	Transformer         genx.TransformerMux
	TransformerForOwner func(context.Context, string) (genx.TransformerMux, error)
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	workflow := spec.Workflow.Spec.Chatroom
	if workflow == nil {
		return nil, fmt.Errorf("chatroom: workflow spec.chatroom is required")
	}
	asr, err := resolveOwnerTransformer(ctx, spec.Workspace, f.Transformer, f.TransformerForOwner)
	if err != nil {
		return nil, err
	}
	config := genxchatroom.Config{ASR: asr, InputMode: genxchatroom.InputModePushToTalk}
	mergeWorkflowConfig(&config, *workflow)
	if spec.Workspace.Parameters != nil {
		parameters, err := spec.Workspace.Parameters.AsChatRoomWorkspaceParameters()
		if err != nil {
			return nil, fmt.Errorf("chatroom: decode workspace parameters: %w", err)
		}
		if err := mergeWorkspaceConfig(&config, parameters); err != nil {
			return nil, err
		}
	}
	transformer, err := genxchatroom.New(config)
	if err != nil {
		return nil, err
	}
	return agenthost.NewTransformerAgent(transformer), nil
}

func resolveOwnerTransformer(ctx context.Context, workspace apitypes.Workspace, fallback genx.TransformerMux, resolve func(context.Context, string) (genx.TransformerMux, error)) (genx.TransformerMux, error) {
	if workspace.OwnerPublicKey == nil || strings.TrimSpace(*workspace.OwnerPublicKey) == "" {
		return fallback, nil
	}
	owner := strings.TrimSpace(*workspace.OwnerPublicKey)
	if resolve == nil {
		return nil, fmt.Errorf("chatroom: workspace %q owner transformer resolver is required", workspace.Name)
	}
	transformer, err := resolve(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("chatroom: workspace %q owner runtime: %w", workspace.Name, err)
	}
	if transformer == nil {
		return nil, fmt.Errorf("chatroom: workspace %q owner runtime returned no transformer", workspace.Name)
	}
	return transformer, nil
}

func mergeWorkflowConfig(config *genxchatroom.Config, workflow apitypes.ChatRoomWorkflowSpec) {
	if config == nil || workflow.Transcript == nil {
		return
	}
	config.TranscriptEnabled = boolValue(workflow.Transcript.Enabled)
	if model := stringValue(workflow.Transcript.AsrModel); model != "" {
		config.ASRPattern = "model/" + model
	}
}

func mergeWorkspaceConfig(config *genxchatroom.Config, parameters apitypes.ChatRoomWorkspaceParameters) error {
	if !parameters.AgentType.Valid() {
		return fmt.Errorf("chatroom: unsupported agent_type %q", parameters.AgentType)
	}
	if parameters.Mode != nil && !parameters.Mode.Valid() {
		return fmt.Errorf("chatroom: unsupported mode %q", *parameters.Mode)
	}
	if parameters.Input != nil {
		if !parameters.Input.Valid() {
			return fmt.Errorf("chatroom: unsupported input %q", *parameters.Input)
		}
		config.InputMode = genxchatroom.InputMode(*parameters.Input)
	}
	if parameters.Transcript == nil {
		return nil
	}
	if parameters.Transcript.Enabled != nil {
		config.TranscriptEnabled = *parameters.Transcript.Enabled
	}
	if model := stringValue(parameters.Transcript.AsrModel); model != "" {
		config.ASRPattern = "model/" + model
	}
	return nil
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
