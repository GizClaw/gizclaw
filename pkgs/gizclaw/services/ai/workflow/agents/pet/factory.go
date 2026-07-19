package pet

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

const Type = "pet"

// ContextProvider resolves the adopted pet attached to a Workspace. The
// provider is invoked for every turn so Gameplay attribute changes are visible
// without recreating the agent.
type ContextProvider interface {
	ResolvePetContext(context.Context, string) (apitypes.Pet, apitypes.PetDef, error)
}

// Config supplies the server-level model resources used by Pet workflows.
type Config struct {
	GenerateModel  string
	ExtractModel   string
	EmbeddingModel string
	ASRModel       string
}

type Factory struct {
	GenX    *peergenx.Service
	Pets    ContextProvider
	Config  Config
	History logstore.MutableStore
	Memory  memory.Store
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	if spec.Workflow.Spec.Pet == nil {
		return nil, fmt.Errorf("pet: workflow spec.pet is required")
	}
	if f.Pets == nil {
		return nil, fmt.Errorf("pet: gameplay context provider is required")
	}
	if spec.Workspace.Parameters == nil {
		return nil, fmt.Errorf("pet: workspace parameters are required")
	}
	parameters, err := spec.Workspace.Parameters.AsPetWorkspaceParameters()
	if err != nil {
		return nil, fmt.Errorf("pet: decode workspace parameters: %w", err)
	}
	if parameters.AgentType != apitypes.PetWorkspaceParametersAgentTypePet {
		return nil, fmt.Errorf("pet: unsupported agent_type %q", parameters.AgentType)
	}
	if parameters.Input != nil && !parameters.Input.Valid() {
		return nil, fmt.Errorf("pet: unsupported input %q", *parameters.Input)
	}
	if parameters.Conversation != nil && parameters.Conversation.Initiative != nil && !parameters.Conversation.Initiative.Valid() {
		return nil, fmt.Errorf("pet: unsupported conversation.initiative %q", *parameters.Conversation.Initiative)
	}
	workspaceName := strings.TrimSpace(spec.Workspace.Name)
	if workspaceName == "" {
		return nil, fmt.Errorf("pet: workspace name is required")
	}
	localDir := strings.TrimSpace(spec.Runtime.LocalDir)
	if localDir == "" {
		return nil, fmt.Errorf("pet: local workspace directory is required")
	}
	if _, _, err := f.Pets.ResolvePetContext(ctx, workspaceName); err != nil {
		return nil, fmt.Errorf("pet: resolve workspace %q: %w", workspaceName, err)
	}
	voiceID := strings.TrimSpace(parameters.Voice.VoiceId)
	if voiceID == "" {
		return nil, fmt.Errorf("pet: workspace voice.voice_id is required")
	}
	models, err := resolveModels(f.Config)
	if err != nil {
		return nil, err
	}
	starts := "peer"
	initiativePolicy := "once_when_empty"
	if parameters.Conversation != nil && parameters.Conversation.Initiative != nil && *parameters.Conversation.Initiative == apitypes.PetConversationParametersInitiativeAgent {
		starts = "self"
		initiativePolicy = "on_reload"
	}
	inputMode := string(apitypes.WorkspaceInputModePushToTalk)
	if parameters.Input != nil {
		inputMode = string(*parameters.Input)
	}
	configured := flowcraft.ConfiguredAgentOptions{
		Flowcraft:             fixedFlowcraftConfig(workspaceName, models.GenerateModel, models.ExtractModel, models.EmbeddingModel != ""),
		GenerateModel:         models.GenerateModel,
		ExtractModel:          models.ExtractModel,
		EmbeddingModel:        models.EmbeddingModel,
		ASRModel:              models.ASRModel,
		DefaultVoice:          voiceID,
		NodeVoices:            map[string]string{"answer": voiceID},
		Conversation:          starts,
		AgentInitiativePolicy: initiativePolicy,
		InputMode:             inputMode,
		LocalDir:              filepath.Join(localDir, "flowcraft"),
		WorkspaceName:         workspaceName,
		RuntimeScope:          spec.RuntimeScope(),
		InputProvider: func(turnCtx context.Context) (map[string]any, error) {
			pet, petDef, err := f.Pets.ResolvePetContext(turnCtx, workspaceName)
			if err != nil {
				return nil, fmt.Errorf("resolve workspace %q: %w", workspaceName, err)
			}
			return turnInputs(pet, petDef, parameters), nil
		},
		// Toolkit is intentionally omitted. Proactive Pet tools are owned by #224.
	}
	return (flowcraft.Factory{
		GenX: f.GenX, History: f.History, Memory: memory.Scoped(f.Memory, spec.RuntimeScope()),
	}).NewConfiguredAgent(ctx, configured)
}

func resolveModels(server Config) (Config, error) {
	models := Config{
		GenerateModel:  strings.TrimSpace(server.GenerateModel),
		ExtractModel:   strings.TrimSpace(server.ExtractModel),
		EmbeddingModel: strings.TrimSpace(server.EmbeddingModel),
		ASRModel:       strings.TrimSpace(server.ASRModel),
	}
	for _, required := range []struct {
		field string
		value string
	}{
		{field: "generate_model", value: models.GenerateModel},
		{field: "extract_model", value: models.ExtractModel},
		{field: "asr_model", value: models.ASRModel},
	} {
		if required.value == "" {
			return Config{}, fmt.Errorf("pet: %s is not configured in server system_tasks.pet_flowcraft_workflow", required.field)
		}
	}
	return models, nil
}
