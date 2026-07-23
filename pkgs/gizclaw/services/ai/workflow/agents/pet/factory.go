package pet

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	genxflowcraft "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	flowcraftagent "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const Type = "pet"

// ContextProvider resolves the adopted pet attached to a Workspace. The
// provider is invoked for every turn so Gameplay attribute changes are visible
// without recreating the agent.
type ContextProvider interface {
	ResolvePetContext(context.Context, string) (apitypes.Pet, apitypes.PetDef, error)
}

// Factory owns only Pet-specific configuration, Store assembly, and current
// Gameplay Board inputs. Flowcraft execution and audio stream composition are
// delegated to reusable GenX packages.
type Factory struct {
	GenX          *peergenx.Service
	Pets          ContextProvider
	History       logstore.MutableStore
	State         kv.Store
	MemoryObjects objectstore.ObjectStore
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
	if _, _, err := f.Pets.ResolvePetContext(ctx, workspaceName); err != nil {
		return nil, fmt.Errorf("pet: resolve workspace %q: %w", workspaceName, err)
	}
	if f.GenX == nil {
		return nil, fmt.Errorf("pet: peergenx service is required")
	}
	voiceID := strings.TrimSpace(parameters.Voice.VoiceId)
	if voiceID == "" {
		return nil, fmt.Errorf("pet: workspace voice.voice_id is required")
	}
	for _, alias := range []string{petChatModelAlias, petExtractModelAlias} {
		if _, err := f.GenX.ResolveGenerator(ctx, modelPattern(alias)); err != nil {
			return nil, fmt.Errorf("pet: resolve model alias %q: %w", alias, err)
		}
	}
	memoryConfig, err := fixedPetMemory()
	if err != nil {
		return nil, err
	}
	scope := flowcraftagent.WorkspaceAgentScope("", workspaceName, petAgentID)
	memoryBuild, err := (flowcraftagent.Factory{GenX: f.GenX, MemoryObjects: f.MemoryObjects}).BuildMemory(ctx, "", workspaceName, petAgentID, memoryConfig)
	if err != nil {
		return nil, fmt.Errorf("pet: build memory: %w", err)
	}
	config := genxflowcraft.Config{
		ID: petAgentID, Name: "Pet", Description: "An adopted GizClaw pet.",
		Graph: fixedPetGraph(), MaxIterations: 6, PublishNodes: []string{"answer"},
		Models: f.GenX.Generator(), History: f.History, HistoryScope: scope, ContextID: scope,
		Memory: memoryBuild.Store, MemoryScope: memory.Scope(scope),
		RecallProfiles: memoryBuild.RecallProfiles, ObserveEnabled: memoryBuild.ObserveEnabled,
		ObserveWaitForCompletion: memoryBuild.ObserveWaitForCompletion, ObservationBuilder: memoryBuild.ObservationBuilder,
		BoardInputs: func(turnCtx context.Context) (map[string]any, error) {
			pet, petDef, err := f.Pets.ResolvePetContext(turnCtx, workspaceName)
			if err != nil {
				return nil, fmt.Errorf("resolve workspace %q: %w", workspaceName, err)
			}
			return turnInputs(pet, petDef, parameters), nil
		},
	}
	if parameters.Conversation != nil && parameters.Conversation.Initiative != nil && *parameters.Conversation.Initiative == apitypes.PetConversationParametersInitiativeAgent {
		config.Initiative = genxflowcraft.InitiativeOnReload
	}
	if f.State != nil {
		config.State = kv.Prefixed(f.State, kv.Key{"flowcraft", workspaceName, petAgentID})
	}
	core, err := genxflowcraft.New(config)
	if err != nil {
		_ = memoryBuild.Closer.Close()
		return nil, fmt.Errorf("pet: build Flowcraft transformer: %w", err)
	}
	transformer, err := audiodock.New(audiodock.Config{
		Agent: core,
		ASR:   patternTransformer{mux: f.GenX.Transformer(), pattern: modelPattern(petASRModelAlias)},
		TTS:   f.GenX.Transformer(),
		ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) {
			return voicePattern(voiceID), nil
		},
	})
	if err != nil {
		_ = memoryBuild.Closer.Close()
		return nil, fmt.Errorf("pet: build audio dock: %w", err)
	}
	return flowcraftagent.NewManagedAgent(transformer, []io.Closer{memoryBuild.Closer}, memoryBuild.Store, memory.Scope(scope)), nil
}

type patternTransformer struct {
	mux     genx.TransformerMux
	pattern string
}

func (t patternTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return t.mux.Transform(ctx, t.pattern, input)
}

func modelPattern(alias string) string {
	alias = strings.Trim(strings.TrimSpace(alias), "/")
	if strings.Contains(alias, "/") {
		return alias
	}
	return "model/" + alias
}

func voicePattern(alias string) string {
	alias = strings.Trim(strings.TrimSpace(alias), "/")
	if strings.Contains(alias, "/") {
		return alias
	}
	return "voice/" + alias
}
