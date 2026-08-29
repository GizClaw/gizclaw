package pet

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestTurnInputsContainOnlyPetDomainContext(t *testing.T) {
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
	)
	if got := inputs["tmp_pet_character_prompt"]; got != "PetDef character" {
		t.Fatalf("character prompt = %q", got)
	}
	if got := inputs["tmp_pet_voice_prompt"]; got != "PetDef voice" {
		t.Fatalf("voice prompt = %q", got)
	}
	if got := inputs["tmp_pet_attribute_prompt"]; got != "当前名字：小火花\n当前生活属性：life=99.00，health=80.00，satiety=12.00，hygiene=70.00，mood=60.00，energy=50.00\n当前成长属性：experience=7，level=1\n当前生命周期：alive" {
		t.Fatalf("attribute prompt = %q", got)
	}
}

func TestFactoryDelegatesNestedWorkflowToRegisteredFactory(t *testing.T) {
	registry := agenthost.NewRegistry()
	nested := &captureFactory{}
	if err := registry.Register("flowcraft", nested); err != nil {
		t.Fatal(err)
	}
	flowcraft := apitypes.FlowcraftWorkflowSpec{}
	factory := Factory{
		Pets: staticPetContext{
			pet:    apitypes.Pet{DisplayName: "Dewey"},
			petDef: apitypes.PetDef{Spec: apitypes.PetDefSpec{Character: apitypes.PetDefCharacterSpec{Prompt: "character"}, Voice: apitypes.PetDefVoiceSpec{Prompt: "voice"}}},
		},
		Factories: registry,
	}
	owner := "peer-a"
	memoryBinding := &apitypes.RuntimeProfileMemoryBinding{LayoutId: "pet-memory-layout-id", Driver: apitypes.RuntimeProfileMemoryDriverFlowcraft}
	memoryLayout := &apitypes.MemoryLayout{Id: "pet-memory-layout-id"}
	agent, err := factory.NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-id-a", Name: "pet-demo", OwnerPublicKey: &owner},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet: &apitypes.PetWorkflowSpec{
				Driver:    apitypes.ReusableWorkflowDriverFlowcraft,
				Flowcraft: &flowcraft,
			},
		}},
		AgentType:             Type,
		MemoryBinding:         memoryBinding,
		MemoryLayout:          memoryLayout,
		MemoryName:            "pet-memory",
		MemoryProfileID:       "profile-id",
		MemoryProfileRevision: "revision-1",
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if agent == nil || nested.spec.AgentType != "flowcraft" || nested.spec.Workflow.Spec.Flowcraft == nil {
		t.Fatalf("delegated spec = %#v", nested.spec)
	}
	if nested.spec.Workspace.Parameters != nil || nested.spec.BoardInputs == nil {
		t.Fatalf("delegated Workspace/input provider = %#v", nested.spec)
	}
	if nested.spec.MemoryBinding != memoryBinding || nested.spec.MemoryLayout != memoryLayout ||
		nested.spec.MemoryName != "pet-memory" || nested.spec.MemoryProfileID != "profile-id" ||
		nested.spec.MemoryProfileRevision != "revision-1" {
		t.Fatalf("delegated outer Memory binding = %#v", nested.spec)
	}
	inputs, err := nested.spec.BoardInputs(t.Context())
	if err != nil || inputs["tmp_pet_character_prompt"] != "character" {
		t.Fatalf("BoardInputs() = %#v, %v", inputs, err)
	}
	if _, err := agent.Transform(t.Context(), nil); err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if nested.transformer.inputs["tmp_pet_character_prompt"] != "character" {
		t.Fatalf("nested Transform context inputs = %#v", nested.transformer.inputs)
	}
}

func TestFactoryInjectsPetContextForEveryReusableDriver(t *testing.T) {
	testCases := []struct {
		name           string
		spec           apitypes.PetWorkflowSpec
		payloadPresent func(apitypes.WorkflowSpec) bool
	}{
		{
			name: "flowcraft",
			spec: apitypes.PetWorkflowSpec{
				Driver:    apitypes.ReusableWorkflowDriverFlowcraft,
				Flowcraft: &apitypes.FlowcraftWorkflowSpec{},
			},
			payloadPresent: func(spec apitypes.WorkflowSpec) bool { return spec.Flowcraft != nil },
		},
		{
			name: "doubao-realtime",
			spec: apitypes.PetWorkflowSpec{
				Driver:         apitypes.ReusableWorkflowDriverDoubaoRealtime,
				DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{},
			},
			payloadPresent: func(spec apitypes.WorkflowSpec) bool { return spec.DoubaoRealtime != nil },
		},
		{
			name: "dashscope-realtime",
			spec: apitypes.PetWorkflowSpec{
				Driver:            apitypes.ReusableWorkflowDriverDashscopeRealtime,
				DashscopeRealtime: &apitypes.DashScopeRealtimeWorkflowSpec{},
			},
			payloadPresent: func(spec apitypes.WorkflowSpec) bool { return spec.DashscopeRealtime != nil },
		},
		{
			name: "doubao-realtime-duplex",
			spec: apitypes.PetWorkflowSpec{
				Driver:               apitypes.ReusableWorkflowDriverDoubaoRealtimeDuplex,
				DoubaoRealtimeDuplex: &apitypes.DoubaoRealtimeDuplexWorkflowSpec{},
			},
			payloadPresent: func(spec apitypes.WorkflowSpec) bool { return spec.DoubaoRealtimeDuplex != nil },
		},
		{
			name: "eino",
			spec: apitypes.PetWorkflowSpec{
				Driver: apitypes.ReusableWorkflowDriverEino,
				Eino: &apitypes.EinoWorkflowSpec{VoiceAdapter: &apitypes.VoiceAdapter{
					AsrModel: new("speech.asr"),
				}},
			},
			payloadPresent: func(spec apitypes.WorkflowSpec) bool {
				return spec.Eino != nil && spec.Eino.VoiceAdapter != nil &&
					spec.Eino.VoiceAdapter.AsrModel != nil && *spec.Eino.VoiceAdapter.AsrModel == "speech.asr"
			},
		},
		{
			name: "ast-translate",
			spec: apitypes.PetWorkflowSpec{
				Driver:       apitypes.ReusableWorkflowDriverAstTranslate,
				AstTranslate: &apitypes.ASTTranslateWorkflowSpec{},
			},
			payloadPresent: func(spec apitypes.WorkflowSpec) bool { return spec.AstTranslate != nil },
		},
		{
			name: "chatroom",
			spec: apitypes.PetWorkflowSpec{
				Driver:   apitypes.ReusableWorkflowDriverChatroom,
				Chatroom: &apitypes.ChatRoomWorkflowSpec{},
			},
			payloadPresent: func(spec apitypes.WorkflowSpec) bool { return spec.Chatroom != nil },
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			registry := agenthost.NewRegistry()
			nested := &captureFactory{}
			if err := registry.Register(testCase.name, nested); err != nil {
				t.Fatal(err)
			}
			agent, err := (Factory{
				Pets: staticPetContext{
					pet:    apitypes.Pet{DisplayName: "Dewey"},
					petDef: apitypes.PetDef{Spec: apitypes.PetDefSpec{Character: apitypes.PetDefCharacterSpec{Prompt: testCase.name}}},
				},
				Factories: registry,
			}).NewAgent(t.Context(), agenthost.Spec{
				Workspace: apitypes.Workspace{Id: "workspace-id-a", Name: "pet-demo"},
				Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
					Driver: apitypes.WorkflowDriverPet,
					Pet:    &testCase.spec,
				}},
				AgentType: Type,
			})
			if err != nil {
				t.Fatalf("NewAgent() error = %v", err)
			}
			if !testCase.payloadPresent(nested.spec.Workflow.Spec) {
				t.Fatalf("nested Workflow payload was not forwarded: %#v", nested.spec.Workflow.Spec)
			}
			if nested.spec.Workspace.Parameters != nil {
				t.Fatalf("nested Workspace parameters = %#v, want cleared driver overrides", nested.spec.Workspace.Parameters)
			}
			if _, err := agent.Transform(t.Context(), nil); err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if nested.transformer.inputs["tmp_pet_character_prompt"] != testCase.name {
				t.Fatalf("nested Transform context inputs = %#v", nested.transformer.inputs)
			}
		})
	}
}

func TestNestedWorkspaceParametersForwardPetInputMode(t *testing.T) {
	input := apitypes.WorkspaceInputModeRealtime
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromPetWorkspaceParameters(apitypes.PetWorkspaceParameters{Input: &input}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		driver apitypes.ReusableWorkflowDriver
		want   string
	}{
		{driver: apitypes.ReusableWorkflowDriverFlowcraft, want: "flowcraft"},
		{driver: apitypes.ReusableWorkflowDriverDoubaoRealtime, want: "doubao-realtime"},
		{driver: apitypes.ReusableWorkflowDriverEino, want: "eino"},
		{driver: apitypes.ReusableWorkflowDriverAstTranslate, want: "ast-translate"},
		{driver: apitypes.ReusableWorkflowDriverChatroom, want: "chatroom"},
	}
	for _, test := range tests {
		t.Run(string(test.driver), func(t *testing.T) {
			nested, err := nestedWorkspaceParameters(&parameters, test.driver)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := nested.Discriminator(); err != nil || got != test.want {
				t.Fatalf("Discriminator() = %q, %v; want %q", got, err, test.want)
			}
			value, err := nested.ValueByDiscriminator()
			if err != nil {
				t.Fatal(err)
			}
			switch typed := value.(type) {
			case apitypes.FlowcraftWorkspaceParameters:
				assertPetNestedInput(t, typed.Input, input)
			case apitypes.DoubaoRealtimeWorkspaceParameters:
				assertPetNestedInput(t, typed.Input, input)
			case apitypes.EinoWorkspaceParameters:
				assertPetNestedInput(t, typed.Input, input)
			case apitypes.ASTTranslateWorkspaceParameters:
				assertPetNestedInput(t, typed.Input, input)
			case apitypes.ChatRoomWorkspaceParameters:
				assertPetNestedInput(t, typed.Input, input)
			default:
				t.Fatalf("nested parameters type = %T", value)
			}
		})
	}

	if _, err := nestedWorkspaceParameters(&parameters, apitypes.ReusableWorkflowDriverDashscopeRealtime); err == nil ||
		!strings.Contains(err.Error(), "does not support workspace input switching") {
		t.Fatalf("unsupported driver error = %v", err)
	}
}

func assertPetNestedInput(t testing.TB, got *apitypes.WorkspaceInputMode, want apitypes.WorkspaceInputMode) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("nested input = %v, want %q", got, want)
	}
}

func TestFactoryRefreshesNestedBoardInputsWithinLongLivedTransform(t *testing.T) {
	registry := agenthost.NewRegistry()
	nested := &boardInputsCaptureFactory{}
	if err := registry.Register("flowcraft", nested); err != nil {
		t.Fatal(err)
	}
	pets := &sequencedPetContext{}
	agent, err := (Factory{
		Pets:      pets,
		Factories: registry,
	}).NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "workspace-id-a", Name: "pet-demo"},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet: &apitypes.PetWorkflowSpec{
				Driver:    apitypes.ReusableWorkflowDriverFlowcraft,
				Flowcraft: &apitypes.FlowcraftWorkflowSpec{},
			},
		}},
		AgentType: Type,
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if _, err := agent.Transform(t.Context(), nil); err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if pets.calls != 2 {
		t.Fatalf("ResolvePetContext calls = %d, want 2", pets.calls)
	}
	if got := pets.workspaceIDs; len(got) != 2 || got[0] != "workspace-id-a" || got[1] != "workspace-id-a" {
		t.Fatalf("ResolvePetContext workspace IDs = %#v, want canonical Workspace ID for initial and per-turn resolution", got)
	}
	if got := nested.inputs["tmp_pet_attribute_prompt"]; !strings.Contains(got.(string), "当前名字：pet-2") {
		t.Fatalf("nested BoardInputs() = %#v", nested.inputs)
	}
}

func TestFactoryRequiresCanonicalWorkspaceID(t *testing.T) {
	registry := agenthost.NewRegistry()
	if err := registry.Register("flowcraft", &captureFactory{}); err != nil {
		t.Fatal(err)
	}
	pets := &sequencedPetContext{}
	_, err := (Factory{Pets: pets, Factories: registry}).NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "peer-facing-name"},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet: &apitypes.PetWorkflowSpec{
				Driver:    apitypes.ReusableWorkflowDriverFlowcraft,
				Flowcraft: &apitypes.FlowcraftWorkflowSpec{},
			},
		}},
		AgentType: Type,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid workspace id") {
		t.Fatalf("NewAgent() error = %v, want missing canonical Workspace ID", err)
	}
	if pets.calls != 0 {
		t.Fatalf("ResolvePetContext calls = %d, want no fallback to peer-facing Workspace name", pets.calls)
	}
}

type staticPetContext struct {
	pet    apitypes.Pet
	petDef apitypes.PetDef
}

func (s staticPetContext) ResolvePetContext(context.Context, string) (apitypes.Pet, apitypes.PetDef, error) {
	return s.pet, s.petDef, nil
}

type sequencedPetContext struct {
	calls        int
	workspaceIDs []string
}

func (s *sequencedPetContext) ResolvePetContext(_ context.Context, workspaceID string) (apitypes.Pet, apitypes.PetDef, error) {
	s.calls++
	s.workspaceIDs = append(s.workspaceIDs, workspaceID)
	return apitypes.Pet{DisplayName: fmt.Sprintf("pet-%d", s.calls)}, apitypes.PetDef{}, nil
}

type captureFactory struct {
	spec        agenthost.Spec
	transformer contextCaptureTransformer
}

func (f *captureFactory) NewAgent(_ context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	f.spec = spec
	return agenthost.NewTransformerAgent(&f.transformer), nil
}

type contextCaptureTransformer struct {
	inputs map[string]any
}

func (t *contextCaptureTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	t.inputs, _ = agenthost.BoardInputsFromContext(ctx)
	return input, nil
}

type boardInputsCaptureFactory struct {
	inputs map[string]any
}

func (f *boardInputsCaptureFactory) NewAgent(_ context.Context, spec agenthost.Spec) (agenthost.Agent, error) {
	return agenthost.NewTransformerAgent(transformerFunc(func(ctx context.Context, input genx.Stream) (genx.Stream, error) {
		inputs, err := spec.BoardInputs(ctx)
		if err != nil {
			return nil, err
		}
		f.inputs = inputs
		return input, nil
	})), nil
}

type transformerFunc func(context.Context, genx.Stream) (genx.Stream, error)

func (f transformerFunc) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return f(ctx, input)
}
