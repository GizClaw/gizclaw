package flowcraft

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	genxflowcraft "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/flowcraft"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestMapGraphSupportsPublicNodesAndDerivesPublishers(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"prepare",
			"nodes":[
				{"id":"prepare","type":"script","config":{"source":"board.setVar('ok', true);"}},
				{"id":"route","type":"passthrough"},
				{"id":"answer","type":"llm","publish":true,"config":{"model":"llm","max_tokens":128}}
			],
			"edges":[{"from":"prepare","to":"route"},{"from":"route","to":"answer"},{"from":"answer","to":"__end__"}]
		}
	}`)
	graph, publish, err := mapGraph(spec.Graph)
	if err != nil {
		t.Fatalf("mapGraph() error = %v", err)
	}
	if len(graph.Nodes) != 3 || graph.Nodes[0].Type != "script" || graph.Nodes[1].Type != "passthrough" || graph.Nodes[2].Type != "llm" {
		t.Fatalf("mapped nodes = %#v", graph.Nodes)
	}
	if !reflect.DeepEqual(publish, []string{"answer"}) {
		t.Fatalf("publish nodes = %#v", publish)
	}
	if graph.Nodes[2].Config["model"] != "llm" {
		t.Fatalf("LLM config = %#v", graph.Nodes[2].Config)
	}
}

func TestFactoryConstructsWithoutLocalWorkspace(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}
	}`)
	agent, err := (Factory{GenX: peergenx.New(peergenx.Service{})}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace-a"},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &spec,
		}},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if closer, ok := agent.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}

func TestFlowcraftASRPatternHonorsWorkspaceInputMode(t *testing.T) {
	tests := []struct {
		name      string
		alias     string
		inputMode apitypes.WorkspaceInputMode
		want      string
	}{
		{
			name:      "push to talk",
			alias:     "asr",
			inputMode: apitypes.WorkspaceInputModePushToTalk,
			want:      "model/asr",
		},
		{
			name:      "realtime",
			alias:     "asr",
			inputMode: apitypes.WorkspaceInputModeRealtime,
			want:      "model/asr?emit_interim=true",
		},
		{
			name:      "realtime preserves existing parameters",
			alias:     "model/asr?language=zh-CN",
			inputMode: apitypes.WorkspaceInputModeRealtime,
			want:      "model/asr?language=zh-CN&emit_interim=true",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flowcraftASRPattern(tt.alias, tt.inputMode); got != tt.want {
				t.Fatalf("flowcraftASRPattern(%q, %q) = %q, want %q", tt.alias, tt.inputMode, got, tt.want)
			}
		})
	}
}

func TestResolveFlowcraftInputMode(t *testing.T) {
	pushToTalk := apitypes.WorkspaceInputModePushToTalk
	realtime := apitypes.WorkspaceInputModeRealtime
	unsupported := apitypes.WorkspaceInputMode("unsupported")
	tests := []struct {
		name    string
		input   *apitypes.WorkspaceInputMode
		want    apitypes.WorkspaceInputMode
		wantErr string
	}{
		{name: "missing defaults to push to talk", want: apitypes.WorkspaceInputModePushToTalk},
		{name: "explicit push to talk", input: &pushToTalk, want: apitypes.WorkspaceInputModePushToTalk},
		{name: "realtime", input: &realtime, want: apitypes.WorkspaceInputModeRealtime},
		{name: "unsupported", input: &unsupported, wantErr: `unsupported workspace input "unsupported"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveFlowcraftInputMode(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveFlowcraftInputMode() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveFlowcraftInputMode() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveFlowcraftInputMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFactoryRejectsUnsupportedWorkspaceInput(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}
	}`)
	inputMode := apitypes.WorkspaceInputMode("unsupported")
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromFlowcraftWorkspaceParameters(apitypes.FlowcraftWorkspaceParameters{
		AgentType: apitypes.FlowcraftWorkspaceParametersAgentTypeFlowcraft,
		Input:     &inputMode,
	}); err != nil {
		t.Fatalf("FromFlowcraftWorkspaceParameters() error = %v", err)
	}
	_, err := (Factory{GenX: peergenx.New(peergenx.Service{})}).NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace-a", Parameters: &parameters},
		Workflow: apitypes.Workflow{Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &spec,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), `flowcraft: unsupported workspace input "unsupported"`) {
		t.Fatalf("NewAgent() error = %v, want unsupported workspace input", err)
	}
}

func TestRealtimeASRPatternCompletesTurnsBeforeAudioEOS(t *testing.T) {
	asrPattern := make(chan string, 1)
	var asrCalls int
	asr := transformerMuxFunc(func(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
		asrCalls++
		asrPattern <- pattern
		output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
		go func() {
			defer input.Close()
			utterance := 0
			for {
				chunk, err := input.Next()
				if err != nil {
					_ = output.Done(genx.Usage{})
					return
				}
				blob, ok := chunk.Part.(*genx.Blob)
				if !ok || len(blob.Data) == 0 || blob.Data[0] == 0 {
					continue
				}
				utterance++
				streamID := "audio-1"
				deltas := []genx.Text{"hello ", "world"}
				if utterance == 2 {
					streamID = "audio-1:asr:2"
					deltas = []genx.Text{"second ", "turn"}
				}
				_ = output.Add(
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript", BeginOfStream: true}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: deltas[0], Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript"}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: deltas[1], Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript"}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript", EndOfStream: true}},
				)
			}
		}()
		return output.Stream(), nil
	})
	core := newEchoFlowcraftCore(t)
	dock, err := audiodock.New(audiodock.Config{
		Agent: core,
		ASR: patternTransformer{
			mux:     asr,
			pattern: flowcraftASRPattern("asr", apitypes.WorkspaceInputModeRealtime),
		},
	})
	if err != nil {
		t.Fatalf("audiodock.New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	output, err := dock.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	if err := input.Add(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio-1", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("input.Add() error = %v", err)
	}

	select {
	case got := <-asrPattern:
		if got != "model/asr?emit_interim=true" {
			t.Fatalf("ASR pattern = %q, want realtime pattern", got)
		}
	case <-time.After(time.Second):
		t.Fatal("ASR did not start")
	}
	first := readModelResponse(output)
	select {
	case result := <-first:
		t.Fatalf("response before definite utterance = %q, %#v, %v", result.text, result.chunk, result.err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := input.Add(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio-1"},
	}); err != nil {
		t.Fatalf("input.Add(first utterance) error = %v", err)
	}
	select {
	case result := <-first:
		if result.err != nil {
			t.Fatalf("output.Next() error before first response = %v", result.err)
		}
		if result.text != "hello world" || !result.chunk.IsEndOfStream() {
			t.Fatalf("response = %q, %#v; want combined delta response", result.text, result.chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("no first response before raw audio EOS")
	}
	if err := input.Add(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{2}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio-1"},
	}); err != nil {
		t.Fatalf("input.Add(second utterance) error = %v", err)
	}
	select {
	case result := <-readModelResponse(output):
		if result.err != nil {
			t.Fatalf("output.Next() error before second response = %v", result.err)
		}
		if result.text != "second turn" || !result.chunk.IsEndOfStream() {
			t.Fatalf("second response = %q, %#v; want combined delta response", result.text, result.chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("no second response before raw audio EOS")
	}
	if asrCalls != 1 {
		t.Fatalf("ASR Transform calls = %d, want one continuous session", asrCalls)
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input.Done() error = %v", err)
	}
}

func TestPushToTalkASRPatternWaitsForAudioEOS(t *testing.T) {
	asr := transformerMuxFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
		if pattern != "model/asr" {
			return nil, errors.New("unexpected push-to-talk ASR pattern")
		}
		output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
		go func() {
			defer input.Close()
			for {
				chunk, err := input.Next()
				if err != nil {
					_ = output.Done(genx.Usage{})
					return
				}
				if !chunk.IsEndOfStream() {
					continue
				}
				_ = output.Add(
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", BeginOfStream: true}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("push "), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("to talk"), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"}},
					&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", EndOfStream: true}},
				)
			}
		}()
		return output.Stream(), nil
	})
	dock, err := audiodock.New(audiodock.Config{
		Agent: newEchoFlowcraftCore(t),
		ASR: patternTransformer{
			mux:     asr,
			pattern: flowcraftASRPattern("asr", apitypes.WorkspaceInputModePushToTalk),
		},
	})
	if err != nil {
		t.Fatalf("audiodock.New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	output, err := dock.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	defer output.Close()
	if err := input.Add(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio-1", BeginOfStream: true},
	}); err != nil {
		t.Fatalf("input.Add(audio) error = %v", err)
	}
	response := readModelResponse(output)
	select {
	case result := <-response:
		t.Fatalf("response before client audio EOS = %q, %#v, %v", result.text, result.chunk, result.err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := input.Add(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "audio-1", EndOfStream: true},
	}); err != nil {
		t.Fatalf("input.Add(audio EOS) error = %v", err)
	}
	select {
	case result := <-response:
		if result.err != nil {
			t.Fatalf("output.Next() error after audio EOS = %v", result.err)
		}
		if result.text != "push to talk" || !result.chunk.IsEndOfStream() {
			t.Fatalf("response = %q, %#v; want one combined response", result.text, result.chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("no response after client audio EOS")
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatalf("input.Done() error = %v", err)
	}
}

type modelResult struct {
	chunk *genx.MessageChunk
	text  string
	err   error
}

func readModelResponse(output genx.Stream) <-chan modelResult {
	result := make(chan modelResult, 1)
	go func() {
		var text strings.Builder
		for {
			chunk, err := output.Next()
			if err != nil {
				result <- modelResult{err: err}
				return
			}
			if chunk == nil || chunk.Role != genx.RoleModel {
				continue
			}
			if delta, ok := chunk.Part.(genx.Text); ok {
				text.WriteString(string(delta))
			}
			if chunk.IsEndOfStream() {
				result <- modelResult{chunk: chunk, text: text.String()}
				return
			}
		}
	}()
	return result
}

func newEchoFlowcraftCore(t *testing.T) *genxflowcraft.Agent {
	t.Helper()
	core, err := genxflowcraft.New(genxflowcraft.Config{
		ID: "flowcraft-input-mode-test", Name: "Flowcraft input mode test",
		Graph: flowgraph.GraphDefinition{
			Name: "echo", Entry: "echo",
			Nodes: []flowgraph.NodeDefinition{{
				ID: "echo", Type: "llm", Config: map[string]any{"model": "echo"},
			}},
		},
		PublishNodes: []string{"echo"},
		Models:       echoFlowcraftGenerator{},
	})
	if err != nil {
		t.Fatalf("flowcraft.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := core.Close(); err != nil {
			t.Errorf("Flowcraft core Close() error = %v", err)
		}
	})
	return core
}

type echoFlowcraftGenerator struct{}

func (echoFlowcraftGenerator) GenerateStream(_ context.Context, _ string, modelContext genx.ModelContext) (genx.Stream, error) {
	var input strings.Builder
	for message := range modelContext.Messages() {
		if message.Role != genx.RoleUser {
			continue
		}
		contents, ok := message.Payload.(genx.Contents)
		if !ok {
			continue
		}
		input.Reset()
		for _, part := range contents {
			if text, ok := part.(genx.Text); ok {
				input.WriteString(string(text))
			}
		}
	}
	output := genx.NewGrowableStreamBuilder(modelContext, 2)
	_ = output.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(input.String())})
	_ = output.Done(genx.Usage{})
	return output.Stream(), nil
}

func (echoFlowcraftGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not supported")
}

func TestBuildRuntimeEmbedderRequiresProviderCredential(t *testing.T) {
	makeModel := func(t *testing.T, provider apitypes.ModelProviderKind, data apitypes.ModelProviderData) apitypes.Model {
		t.Helper()
		return apitypes.Model{Id: "embedding", Provider: apitypes.ModelProvider{Kind: provider}, ProviderData: data}
	}
	dashScopeData := apitypes.ModelProviderData{}
	if err := dashScopeData.FromDashScopeTenantModelProviderData(apitypes.DashScopeTenantModelProviderData{}); err != nil {
		t.Fatal(err)
	}
	volcData := apitypes.ModelProviderData{}
	if err := volcData.FromVolcTenantModelProviderData(apitypes.VolcTenantModelProviderData{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		config peergenx.EmbeddingConfig
		want   string
	}{
		{
			name: "dashscope api key",
			config: peergenx.EmbeddingConfig{
				Model:      makeModel(t, apitypes.ModelProviderKindDashscopeTenant, dashScopeData),
				Tenant:     peergenx.Tenant{Kind: string(apitypes.ModelProviderKindDashscopeTenant), DashScope: &apitypes.DashScopeTenant{}},
				Credential: apitypes.Credential{Name: "dashscope-empty", Body: testDashScopeCredentialBody("")},
			},
			want: "credential \"dashscope-empty\" has no api_key",
		},
		{
			name: "volc ark api key",
			config: peergenx.EmbeddingConfig{
				Model:      makeModel(t, apitypes.ModelProviderKindVolcTenant, volcData),
				Tenant:     peergenx.Tenant{Kind: string(apitypes.ModelProviderKindVolcTenant), Volc: &apitypes.VolcTenant{}},
				Credential: apitypes.Credential{Name: "volc-empty", Body: testVolcCredentialBodyFromStrings(nil)},
			},
			want: "credential \"volc-empty\" has no ark_api_key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildRuntimeEmbedder(tc.config); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("buildRuntimeEmbedder() error = %v, want %q", err, tc.want)
			}
		})
	}
}

type transformerFunc func(context.Context, genx.Stream) (genx.Stream, error)

func (f transformerFunc) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return f(ctx, input)
}

type transformerMuxFunc func(context.Context, string, genx.Stream) (genx.Stream, error)

func (f transformerMuxFunc) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	return f(ctx, pattern, input)
}

func TestFactoryUsesWorkspaceOwnerGenX(t *testing.T) {
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}
	}`)
	owner := "owner-public-key"
	called := false
	factory := Factory{GenXForOwner: func(_ context.Context, gotOwner string) (*peergenx.Service, error) {
		called = true
		if gotOwner != owner {
			t.Fatalf("owner = %q, want %q", gotOwner, owner)
		}
		return peergenx.New(peergenx.Service{}), nil
	}}
	agent, err := factory.NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace-a", OwnerPublicKey: &owner},
		Workflow:  apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &spec}},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	if !called {
		t.Fatal("owner GenX resolver was not called")
	}
	if closer, ok := agent.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}
}

func TestFactoryUsesResolvedWorkspaceMemory(t *testing.T) {
	backing := &recordingExternalMemoryStore{}
	spec := decodeFlowcraftSpec(t, `{
		"graph":{
			"name":"graph","entry":"route",
			"nodes":[{"id":"route","type":"passthrough","publish":true}],
			"edges":[{"from":"route","to":"__end__"}]
		}
	}`)
	agent, err := (Factory{GenX: peergenx.New(peergenx.Service{})}).NewAgent(t.Context(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "workspace-a"},
		Workflow: apitypes.Workflow{Name: "workflow-a", Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &spec,
		}},
		Memory: backing, MemoryKind: "mem0",
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	t.Cleanup(func() {
		if closer, ok := agent.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	stats, err := agent.MemoryStats(t.Context(), apitypes.PeerRunMemoryStatsRequest{})
	if err != nil {
		t.Fatalf("MemoryStats() error = %v", err)
	}
	if !stats.Enabled || stats.Backend == nil || *stats.Backend != "mem0" {
		t.Fatalf("MemoryStats() = %#v", stats)
	}
}

func TestMapInitiativePreservesWorkspacePolicy(t *testing.T) {
	agent := apitypes.FlowcraftConversationStartsAgent
	peer := apitypes.FlowcraftConversationStartsPeer
	if got := mapInitiative(&apitypes.FlowcraftConversation{Starts: &agent}, "once_when_empty"); got != genxflowcraft.InitiativeOnceWhenEmpty {
		t.Fatalf("once_when_empty = %q", got)
	}
	if got := mapInitiative(&apitypes.FlowcraftConversation{Starts: &agent}, "on_reload"); got != genxflowcraft.InitiativeOnReload {
		t.Fatalf("on_reload = %q", got)
	}
	if got := mapInitiative(&apitypes.FlowcraftConversation{Starts: &peer}, "once_when_empty"); got != genxflowcraft.InitiativeDisabled {
		t.Fatalf("peer initiative = %q", got)
	}
}

func TestWorkspaceAgentScopeIncludesOwnerWhenAvailable(t *testing.T) {
	if got, want := workspaceAgentScope("owner-a", "workspace-a", "assistant"), "o/95256875151043ab/w/0dcf2d98505da17d/a/a39a7ffad4a3013f"; got != want {
		t.Fatalf("workspaceAgentScope() = %q, want %q", got, want)
	}
	if got, want := workspaceAgentScope("", "workspace-a", "assistant"), "w/0dcf2d98505da17d/a/a39a7ffad4a3013f"; got != want {
		t.Fatalf("workspaceAgentScope() without owner = %q, want %q", got, want)
	}
	if got := workspaceAgentScope(strings.Repeat("owner", 100), strings.Repeat("workspace", 100), strings.Repeat("agent", 100)); len(got) != len("o/")+16+len("/w/")+16+len("/a/")+16 {
		t.Fatalf("workspaceAgentScope() length = %d for long identities, got %q", len(got), got)
	}
}

func TestFlowcraftStateStoreUsesCanonicalIsolatedScope(t *testing.T) {
	base := kv.NewMemory(nil)
	scope := workspaceAgentScope("owner-a", "workspace-a", "assistant")
	state := flowcraftStateStore(base, scope)
	if err := state.Set(t.Context(), kv.Key{"checkpoint"}, []byte("private")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	expectedKey := append(kv.Key{"flowcraft"}, strings.Split(scope, "/")...)
	expectedKey = append(expectedKey, "checkpoint")
	value, err := base.Get(t.Context(), expectedKey)
	if err != nil {
		t.Fatalf("base.Get(%v) error = %v", expectedKey, err)
	}
	if string(value) != "private" {
		t.Fatalf("base.Get(%v) = %q", expectedKey, value)
	}

	for _, otherScope := range []string{
		workspaceAgentScope("owner-b", "workspace-a", "assistant"),
		workspaceAgentScope("owner-a", "workspace-b", "assistant"),
		workspaceAgentScope("owner-a", "workspace-a", "other-agent"),
	} {
		if _, err := flowcraftStateStore(base, otherScope).Get(t.Context(), kv.Key{"checkpoint"}); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("scope %q Get() error = %v, want ErrNotFound", otherScope, err)
		}
	}
}

func TestManagedAgentClosesOwnedResourcesOnceConcurrently(t *testing.T) {
	closer := &countingCloser{}
	agent := &managedAgent{owned: []io.Closer{closer}}
	var wait sync.WaitGroup
	for range 16 {
		wait.Go(func() {
			if err := agent.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
	}
	wait.Wait()
	if calls := closer.count(); calls != 1 {
		t.Fatalf("owned close calls = %d, want 1", calls)
	}
}

func decodeFlowcraftSpec(t *testing.T, raw string) apitypes.FlowcraftWorkflowSpec {
	t.Helper()
	var spec apitypes.FlowcraftWorkflowSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("decode Flowcraft spec: %v", err)
	}
	return spec
}

type countingCloser struct {
	mu    sync.Mutex
	calls int
}

type recordingExternalMemoryStore struct {
	observation memorystore.Observation
}

func (s *recordingExternalMemoryStore) Observe(_ context.Context, observation memorystore.Observation) (memorystore.ObserveResult, error) {
	s.observation = observation
	return memorystore.ObserveResult{}, nil
}

func (*recordingExternalMemoryStore) Recall(context.Context, memorystore.Query) (memorystore.RecallResult, error) {
	return memorystore.RecallResult{}, nil
}

func (*recordingExternalMemoryStore) Update(context.Context, memorystore.UpdateRequest) (memorystore.Fact, error) {
	return memorystore.Fact{}, nil
}

func (*recordingExternalMemoryStore) Delete(context.Context, memorystore.DeleteRequest) error {
	return nil
}

func (c *countingCloser) Close() error {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return nil
}

func (c *countingCloser) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}
