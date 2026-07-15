package pet

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/flowcraft/memory/recall"
	"github.com/GizClaw/flowcraft/memory/recall/recalltest"
	recallworkspace "github.com/GizClaw/flowcraft/memory/recall/store/workspace"
	sdkworkspace "github.com/GizClaw/flowcraft/sdk/workspace"
	flowclaw "github.com/GizClaw/flowcraft/sdkx/claw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"gopkg.in/yaml.v3"
)

func TestTurnInputsComposeWorkspacePromptsAndDefinedAttributes(t *testing.T) {
	persona := "Workspace personality"
	voice := "Workspace speaking style"
	inputs := turnInputs(
		apitypes.Pet{
			DisplayName: "小火花",
			Life:        apitypes.PetLife{"hunger": 12, "clean": 80, "unknown": 999},
			Progression: apitypes.PetProgression{"xp": 7},
		},
		apitypes.PetDef{Spec: apitypes.PetDefSpec{
			Attr: apitypes.PetDefAttrSpec{
				Life:        apitypes.PetAttrGroupSpec{"hunger": {}, "clean": {}},
				Progression: apitypes.PetAttrGroupSpec{"xp": {}},
			},
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
	if got := inputs["tmp_pet_attribute_prompt"]; got != "当前名字：小火花\n当前生活属性：clean=80，hunger=12\n当前成长属性：xp=7" {
		t.Fatalf("attribute prompt = %q", got)
	}
	if strings.Contains(inputs["tmp_pet_attribute_prompt"].(string), "unknown") {
		t.Fatalf("attribute prompt leaked undefined attribute: %q", inputs["tmp_pet_attribute_prompt"])
	}
}

func TestFixedFlowcraftConfigOwnsPetGraphAndAsyncMemoryLayout(t *testing.T) {
	cfg := fixedFlowcraftConfig("pet-123", "chat-model", "extract-model", false)
	settings := cfg["settings"].(map[string]any)
	if settings["generate_model"] != "chat-model" || settings["extract_model"] != "extract-model" {
		t.Fatalf("configured model resource names = %#v", settings)
	}
	memory := cfg["memory"].(map[string]any)
	if got := memory["scope"]; !reflect.DeepEqual(got, map[string]any{
		"runtime_id": "gizclaw-pet",
		"user_id":    "pet-123",
		"agent_id":   "pet-123",
	}) {
		t.Fatalf("memory scope = %#v", got)
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
	if _, ok := cfg["tools"]; ok {
		t.Fatalf("pet config unexpectedly contains tools: %#v", cfg["tools"])
	}
}

func TestPetAsyncMemoryUsesWorkspaceQueueContract(t *testing.T) {
	recalltest.RunAsyncSemanticQueueSuite(t, func(t testing.TB) recall.AsyncSemanticQueue {
		backend, err := recallworkspace.Open(t.TempDir())
		if err != nil {
			t.Fatalf("workspace queue Open() error = %v", err)
		}
		t.Cleanup(func() { _ = backend.Close() })
		return backend.AsyncSemanticQueue()
	})
}

func TestPetAsyncMemoryQueueSurvivesWorkspaceReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	scope := recall.Scope{RuntimeID: "gizclaw-pet", UserID: "pet-123", AgentID: "pet-123"}
	backend, err := recallworkspace.Open(dir)
	if err != nil {
		t.Fatalf("workspace queue Open() error = %v", err)
	}
	if _, err := backend.AsyncSemanticQueue().Enqueue(ctx, recall.AsyncSemanticJob{RequestID: "relationship-1", Scope: scope}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	reopened, err := recallworkspace.Open(dir)
	if err != nil {
		t.Fatalf("workspace queue reopen error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	jobs, err := reopened.AsyncSemanticQueue().Claim(ctx, recall.AsyncSemanticClaimOptions{
		WorkerID: "pet-test",
		Now:      time.Now(),
		Max:      1,
		Scope:    &scope,
	})
	if err != nil {
		t.Fatalf("Claim() after reopen error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].RequestID != "relationship-1" {
		t.Fatalf("Claim() after reopen = %#v", jobs)
	}
}

func TestFixedFlowcraftConfigLoadsInClaw(t *testing.T) {
	cfg := fixedFlowcraftConfig("pet-123", "chat-model", "extract-model", false)
	cfg["models"] = map[string]any{
		"chat":      "generate_model",
		"extractor": "extract_model",
		"llm": map[string]any{
			"chat-model":    map[string]any{"provider": "mock", "model": "mock-generate"},
			"extract-model": map[string]any{"provider": "mock", "model": "mock-extract"},
		},
	}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal() error = %v", err)
	}
	ws, err := sdkworkspace.NewLocalWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalWorkspace() error = %v", err)
	}
	if err := ws.Write(context.Background(), "config.yaml", raw); err != nil {
		t.Fatalf("workspace.Write() error = %v", err)
	}
	claw, err := flowclaw.New(ws)
	if err != nil {
		t.Fatalf("claw.New() rejected fixed Pet config: %v", err)
	}
	if err := claw.CloseContext(context.Background()); err != nil {
		t.Fatalf("CloseContext() error = %v", err)
	}
}

func TestFactoryRejectsMissingOrAmbiguousPetBinding(t *testing.T) {
	petSpec := apitypes.PetWorkflowSpec{}
	parameters := petParameters(t)
	spec := agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "pet-123", Parameters: &parameters},
		Workflow: apitypes.WorkflowDocument{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet:    &petSpec,
		}},
	}
	spec.Runtime.LocalDir = t.TempDir()
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
		Workflow: apitypes.WorkflowDocument{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet:    &petSpec,
		}},
	}
	spec.Runtime.LocalDir = t.TempDir()
	_, err := (Factory{
		GenX: peergenx.New(peergenx.Service{Models: emptyPetModels{}}),
		Pets: staticPetProvider{
			pet:    apitypes.Pet{DisplayName: "Spark"},
			petDef: apitypes.PetDef{},
		},
		Config: Config{GenerateModel: "server-chat", ExtractModel: "server-extract", ASRModel: "server-asr"},
	}).NewAgent(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "server-chat") || !strings.Contains(err.Error(), "not accessible as a generator") {
		t.Fatalf("NewAgent() error = %v, want missing configured model %q", err, "server-chat")
	}
}

func TestFactoryRejectsMissingServerAndWorkspaceModelConfig(t *testing.T) {
	petSpec := apitypes.PetWorkflowSpec{}
	parameters := petParameters(t)
	spec := agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "pet-123", Parameters: &parameters},
		Workflow: apitypes.WorkflowDocument{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet:    &petSpec,
		}},
	}
	spec.Runtime.LocalDir = t.TempDir()
	_, err := (Factory{Pets: staticPetProvider{}}).NewAgent(context.Background(), spec)
	if err == nil || !strings.Contains(err.Error(), "generate_model") || !strings.Contains(err.Error(), "system_tasks.pet_flowcraft_workflow") {
		t.Fatalf("NewAgent() error = %v", err)
	}
}

func TestResolveModelsUsesServerConfigAndWorkspaceOverrides(t *testing.T) {
	workspaceChat := "workspace-chat"
	workspaceASR := "  workspace-asr  "
	models, err := resolveModels(apitypes.PetWorkspaceParameters{
		GenerateModel: &workspaceChat,
		AsrModel:      &workspaceASR,
	}, Config{
		GenerateModel:  "server-chat",
		ExtractModel:   "server-extract",
		EmbeddingModel: "server-embedding",
		ASRModel:       "server-asr",
	})
	if err != nil {
		t.Fatalf("resolveModels() error = %v", err)
	}
	want := Config{
		GenerateModel:  "workspace-chat",
		ExtractModel:   "server-extract",
		EmbeddingModel: "server-embedding",
		ASRModel:       "workspace-asr",
	}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("resolveModels() = %#v, want %#v", models, want)
	}
}

func TestResolveModelsRejectsMissingServerAndWorkspaceConfig(t *testing.T) {
	_, err := resolveModels(apitypes.PetWorkspaceParameters{}, Config{})
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
