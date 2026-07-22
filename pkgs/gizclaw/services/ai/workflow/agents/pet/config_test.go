package pet

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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

func TestFixedFlowcraftConfigOwnsPetGraphAndAsyncMemoryLayout(t *testing.T) {
	cfg := fixedFlowcraftConfig("chat-model", "extract-model", "", "peer")
	memory := cfg["memory"].(map[string]any)
	for _, legacy := range []string{"workspace", "history", "settings"} {
		if _, exists := cfg[legacy]; exists {
			t.Fatalf("fixed config contains legacy %q: %#v", legacy, cfg[legacy])
		}
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
	agent := cfg["agent"].(map[string]any)
	graph := agent["graph"].(map[string]any)
	if graph["entry"] != "prepare_pet_context" {
		t.Fatalf("graph entry = %#v", graph["entry"])
	}
	nodes := graph["nodes"].([]any)
	answer := nodes[1].(map[string]any)["config"].(map[string]any)
	if answer["model"] != "chat-model" {
		t.Fatalf("answer model = %#v", answer["model"])
	}
	if _, ok := cfg["tools"]; ok {
		t.Fatalf("pet config unexpectedly contains tools: %#v", cfg["tools"])
	}
}

func TestFixedFlowcraftConfigLoadsAsPublicSpec(t *testing.T) {
	cfg := fixedFlowcraftConfig("chat-model", "extract-model", "embedding-model", "agent")
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var spec apitypes.FlowcraftWorkflowSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("public Flowcraft config rejected fixed Pet config: %v", err)
	}
	if spec.Conversation == nil || spec.Conversation.Starts == nil || *spec.Conversation.Starts != apitypes.FlowcraftConversationStartsAgent {
		t.Fatalf("conversation = %#v", spec.Conversation)
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
		Config: Config{GenerateModel: "server-chat", ExtractModel: "server-extract", ASRModel: "server-asr"},
	}).NewAgent(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "server-chat") || !strings.Contains(err.Error(), "resolve model alias") || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("NewAgent() error = %v, want missing configured model %q", err, "server-chat")
	}
}

func TestFactoryRejectsMissingServerModelConfig(t *testing.T) {
	petSpec := apitypes.PetWorkflowSpec{}
	parameters := petParameters(t)
	spec := agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "pet-123", Parameters: &parameters},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet:    &petSpec,
		}},
	}
	_, err := (Factory{Pets: staticPetProvider{}}).NewAgent(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "generate_model") || !strings.Contains(err.Error(), "system_tasks.pet_flowcraft_workflow") {
		t.Fatalf("NewAgent() error = %v", err)
	}
}

func TestResolveModelsUsesOnlyServerConfig(t *testing.T) {
	models, err := resolveModels(Config{
		GenerateModel:  "  server-chat  ",
		ExtractModel:   " server-extract ",
		EmbeddingModel: " server-embedding ",
		ASRModel:       " server-asr ",
	})
	if err != nil {
		t.Fatalf("resolveModels() error = %v", err)
	}
	want := Config{
		GenerateModel:  "server-chat",
		ExtractModel:   "server-extract",
		EmbeddingModel: "server-embedding",
		ASRModel:       "server-asr",
	}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("resolveModels() = %#v, want %#v", models, want)
	}
}

func TestResolveModelsRejectsMissingServerConfig(t *testing.T) {
	_, err := resolveModels(Config{})
	if err == nil || !strings.Contains(err.Error(), "generate_model") || !strings.Contains(err.Error(), "system_tasks.pet_flowcraft_workflow") {
		t.Fatalf("resolveModels() error = %v", err)
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
