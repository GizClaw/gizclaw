package pet

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestTurnInputsComposeWorkspacePromptsAndDefinedAttributes(t *testing.T) {
	persona := "Workspace personality"
	voice := "Workspace speaking style"
	inputs := turnInputs(
		apitypes.Pet{
			DisplayName: "小火花",
			Stats: apitypes.PetStats{
				Life: 99, Health: 80, Satiety: 12, Hygiene: 70, Mood: 60, Energy: 50,
			},
			Progression: apitypes.PetProgression{Experience: 7, Level: 1},
			Lifecycle:   apitypes.PetLifecycleAlive,
		},
		apitypes.PetDef{Spec: apitypes.PetDefSpec{
			Character: apitypes.PetDefCharacterSpec{Prompt: "PetDef character"},
			Voice:     apitypes.PetDefVoiceSpec{Prompt: "PetDef voice"},
		}},
		apitypes.PetWorkspaceParameters{
			Persona: &apitypes.PetPersonaParameters{Prompt: &persona},
			Voice:   apitypes.PetVoiceParameters{VoiceId: "voice", Prompt: &voice},
		},
	)
	if got := inputs["tmp_pet_character_prompt"]; got != "PetDef character\n\nWorkspace personality" {
		t.Fatalf("character prompt = %q", got)
	}
	if got := inputs["tmp_pet_voice_prompt"]; got != "PetDef voice\n\nWorkspace speaking style" {
		t.Fatalf("voice prompt = %q", got)
	}
	if got := inputs["tmp_pet_attribute_prompt"]; got != "当前名字：小火花\n当前生活属性：life=99.00，health=80.00，satiety=12.00，hygiene=70.00，mood=60.00，energy=50.00\n当前成长属性：experience=7，level=1\n当前生命周期：alive" {
		t.Fatalf("attribute prompt = %q", got)
	}
}

func TestFixedPetGraphAndMemoryUseRuntimeProfileAliases(t *testing.T) {
	graph := fixedPetGraph()
	if err := graph.Validate(); err != nil {
		t.Fatalf("fixedPetGraph().Validate() error = %v", err)
	}
	if graph.Entry != "prepare_pet_context" || len(graph.Nodes) != 2 || graph.Nodes[1].Config["model"] != petChatModelAlias || graph.Nodes[1].Config["max_tokens"] != 2048 {
		t.Fatalf("fixed graph = %#v", graph)
	}
	memoryConfig, err := fixedPetMemory()
	if err != nil {
		t.Fatalf("fixedPetMemory() error = %v", err)
	}
	raw, err := json.Marshal(memoryConfig)
	if err != nil {
		t.Fatalf("json.Marshal(memory) error = %v", err)
	}
	var memory map[string]any
	if err := json.Unmarshal(raw, &memory); err != nil {
		t.Fatalf("json.Unmarshal(memory) error = %v", err)
	}
	for _, legacy := range []string{"scope", "retrieval"} {
		if _, exists := memory[legacy]; exists {
			t.Fatalf("memory contains legacy %q: %#v", legacy, memory[legacy])
		}
	}
	write := memory["write"].(map[string]any)
	if write["mode"] != "async_semantic" || write["save_conversation"] != true {
		t.Fatalf("memory write = %#v", write)
	}
	extract := memory["extract"].(map[string]any)
	if extract["mode"] != "two_pass" {
		t.Fatalf("memory extract = %#v", extract)
	}
	extractPrompt := extract["system_prompt"].(string)
	for _, requirement := range []string{"ordinary greeting", "one concise current relationship state", "not general pretrained knowledge"} {
		if !strings.Contains(extractPrompt, requirement) {
			t.Fatalf("extract prompt missing %q: %s", requirement, extractPrompt)
		}
	}
	lanes := memory["layout"].(map[string]any)["lanes"].([]any)
	wantKinds := map[string]string{
		"relationship_state": "state",
		"owner_profile":      "state",
		"owner_preferences":  "preference",
		"pet_knowledge":      "note",
		"owner_pet_facts":    "relation",
		"shared_events":      "event",
	}
	for _, raw := range lanes {
		lane := raw.(map[string]any)
		name := lane["name"].(string)
		if lane["kind"] != wantKinds[name] {
			t.Fatalf("lane %q = %#v", name, lane)
		}
		delete(wantKinds, name)
	}
	if len(wantKinds) != 0 {
		t.Fatalf("missing memory lanes: %#v", wantKinds)
	}
	if extract["model"] != petExtractModelAlias {
		t.Fatalf("extract model = %#v", extract["model"])
	}
}

func TestFactoryRejectsMissingOrAmbiguousPetBinding(t *testing.T) {
	petSpec := apitypes.PetWorkflowSpec{}
	parameters := petParameters(t)
	spec := agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "pet-123", Parameters: &parameters},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet:    &petSpec,
		}},
	}
	wantErr := errors.New("multiple pets")
	_, err := (Factory{Pets: failingPetProvider{err: wantErr}}).NewAgent(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("NewAgent() error = %v", err)
	}
}

func TestFactoryRejectsMissingPetContextProvider(t *testing.T) {
	petSpec := apitypes.PetWorkflowSpec{}
	parameters := petParameters(t)
	_, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "pet-123", Parameters: &parameters},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet:    &petSpec,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "gameplay context provider") {
		t.Fatalf("NewAgent() error = %v", err)
	}
}

func TestFactoryRequiresConfiguredModelResourcesToBeOperational(t *testing.T) {
	petSpec := apitypes.PetWorkflowSpec{}
	parameters := petParameters(t)
	spec := agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "pet-123", Parameters: &parameters},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet:    &petSpec,
		}},
	}
	_, err := (Factory{
		GenX: peergenx.New(peergenx.Service{Models: emptyPetModels{}}),
		Pets: staticPetProvider{
			pet:    apitypes.Pet{DisplayName: "Spark"},
			petDef: apitypes.PetDef{},
		},
	}).NewAgent(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), petChatModelAlias) || !strings.Contains(err.Error(), "resolve model alias") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("NewAgent() error = %v, want missing RuntimeProfile alias %q", err, petChatModelAlias)
	}
}

func petParameters(t *testing.T) apitypes.WorkspaceParameters {
	t.Helper()
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromPetWorkspaceParameters(apitypes.PetWorkspaceParameters{
		AgentType: apitypes.PetWorkspaceParametersAgentTypePet,
		Voice:     apitypes.PetVoiceParameters{VoiceId: "voice"},
	}); err != nil {
		t.Fatalf("FromPetWorkspaceParameters() error = %v", err)
	}
	return parameters
}

type failingPetProvider struct{ err error }

func (p failingPetProvider) ResolvePetContext(context.Context, string) (apitypes.Pet, apitypes.PetDef, error) {
	return apitypes.Pet{}, apitypes.PetDef{}, p.err
}

type staticPetProvider struct {
	pet    apitypes.Pet
	petDef apitypes.PetDef
}

func (p staticPetProvider) ResolvePetContext(context.Context, string) (apitypes.Pet, apitypes.PetDef, error) {
	return p.pet, p.petDef, nil
}

type emptyPetModels struct{}

func (emptyPetModels) GetModel(context.Context, adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	return adminhttp.GetModel404JSONResponse{}, nil
}

func (emptyPetModels) ListModels(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	return adminhttp.ListModels200JSONResponse{Items: []apitypes.Model{}}, nil
}
