package runtimeprofile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func audioInputTestBinding(resourceID string, path apitypes.AudioInputPath) apitypes.RuntimeProfileBinding {
	binding := runtimeProfileTestBinding(resourceID)
	binding.AudioInput = &path
	return binding
}

func TestNormalizeProfileWorkflowAudioInput(t *testing.T) {
	t.Parallel()
	normalize := func(spec apitypes.RuntimeProfileSpec) (apitypes.RuntimeProfile, error) {
		return normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test-profile", Spec: spec}, "")
	}

	item, err := normalize(apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
		"assistant":       audioInputTestBinding("assistant", apitypes.AudioInputPathModel),
		"assistant-again": audioInputTestBinding("assistant", apitypes.AudioInputPathModel),
		"assistant-plain": runtimeProfileTestBinding("assistant"),
		"other":           audioInputTestBinding("other", apitypes.AudioInputPathAsr),
	}})
	if err != nil {
		t.Fatalf("normalizeProfile() error = %v", err)
	}
	if got := item.Spec.Workflows["assistant"].AudioInput; got == nil || *got != apitypes.AudioInputPathModel {
		t.Fatalf("workflows.assistant.audio_input = %v", got)
	}
	if got := item.Spec.Workflows["assistant-plain"].AudioInput; got != nil {
		t.Fatalf("workflows.assistant-plain.audio_input = %q, want absent", *got)
	}
	without, err := normalize(apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
		"assistant": runtimeProfileTestBinding("assistant"),
	}})
	if err != nil {
		t.Fatalf("normalizeProfile(without audio_input) error = %v", err)
	}
	if item.Revision == without.Revision {
		t.Fatal("audio_input does not participate in the profile revision")
	}

	for name, test := range map[string]struct {
		spec    apitypes.RuntimeProfileSpec
		wantErr string
	}{
		"unknown path": {
			spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
				"assistant": audioInputTestBinding("assistant", "direct"),
			}},
			wantErr: `workflows.assistant: unsupported audio_input "direct"`,
		},
		"bindings of one Workflow disagree": {
			spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
				"assistant":    audioInputTestBinding("assistant", apitypes.AudioInputPathModel),
				"assistant-v2": audioInputTestBinding("assistant", apitypes.AudioInputPathAsr),
			}},
			wantErr: `workflows.assistant-v2.audio_input "asr" conflicts with workflows.assistant.audio_input "model" for Workflow "assistant"`,
		},
		"model binding": {
			spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{
				Models: &map[string]apitypes.RuntimeProfileBinding{"llm": audioInputTestBinding("chat", apitypes.AudioInputPathModel)},
			}},
			wantErr: "resources.models.llm: audio_input is only valid on workflows",
		},
		"voice binding": {
			spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{
				Voices: &map[string]apitypes.RuntimeProfileBinding{"narrator": audioInputTestBinding("voice", apitypes.AudioInputPathAsr)},
			}},
			wantErr: "resources.voices.narrator: audio_input is only valid on workflows",
		},
	} {
		if _, err := normalize(test.spec); err == nil || !strings.Contains(err.Error(), test.wantErr) {
			t.Fatalf("%s: normalizeProfile() error = %v, want containing %q", name, err, test.wantErr)
		}
	}
}

func TestValidateWorkflowAudioInput(t *testing.T) {
	t.Parallel()
	decode := func(body string) apitypes.WorkflowSpec {
		var spec apitypes.WorkflowSpec
		if err := json.Unmarshal([]byte(body), &spec); err != nil {
			t.Fatalf("decode Workflow spec: %v", err)
		}
		return spec
	}
	const graph = `"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor"},"state":{"fields":[]},"nodes":[%s],"edges":[],"branches":[],"outputs":[]}`
	node := func(flag string) string {
		return `{"id":"answer","type":"chat_model","model":"llm"` + flag + `}`
	}
	eino := func(adapter, flag string) apitypes.WorkflowSpec {
		return decode(`{"driver":"eino","eino":{` + adapter + strings.Replace(graph, "%s", node(flag), 1) + `}}`)
	}
	const asr, transcript = `"voice_adapter":{"asr_model":"asr"},`, `,"audio_transcript":true`
	for _, test := range []struct {
		name     string
		workflow apitypes.WorkflowSpec
		selected apitypes.AudioInputPath
		wantErr  string
	}{
		{name: "both declared model", workflow: eino(asr, transcript), selected: apitypes.AudioInputPathModel},
		{name: "both declared asr", workflow: eino(asr, transcript), selected: apitypes.AudioInputPathAsr},
		{name: "model without node", workflow: eino(asr, ""), selected: apitypes.AudioInputPathModel, wantErr: "requires a chat_model node that sets audio_transcript"},
		{name: "asr without asr_model", workflow: eino("", transcript), selected: apitypes.AudioInputPathAsr, wantErr: "requires voice_adapter.asr_model"},
		{
			name:     "other driver",
			workflow: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft},
			selected: apitypes.AudioInputPathAsr, wantErr: `only valid for Eino Workflows, got driver "flowcraft"`,
		},
	} {
		err := validateWorkflowAudioInput("workflows.assistant", test.workflow, test.selected)
		if test.wantErr == "" {
			if err != nil {
				t.Fatalf("%s: validateWorkflowAudioInput() error = %v", test.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "workflows.assistant") || !strings.Contains(err.Error(), test.wantErr) {
			t.Fatalf("%s: validateWorkflowAudioInput() error = %v, want containing %q", test.name, err, test.wantErr)
		}
	}
}

func TestRuntimeProfileStoresWorkflowAudioInputAndChecksTheWorkflow(t *testing.T) {
	t.Parallel()
	workflowBody := func(flag string) string {
		return `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workflow","metadata":{"id":"assistant"},"spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor"},"state":{"fields":[]},"nodes":[{"id":"answer","type":"chat_model","model":"llm"` + flag + `}],"edges":[],"branches":[],"outputs":[]}}}}`
	}
	const modelBody = `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"chat"},"spec":{"kind":"llm","source":"manual","provider":{"kind":"volc-tenant","id":"volc-ark"},"provider_data":{"api_mode":"chat_completions"}}}`
	resolver := func(flag string) func(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error) {
		return func(_ context.Context, kind apitypes.ResourceKind, _ string) (apitypes.Resource, error) {
			body := modelBody
			if kind == apitypes.ResourceKindWorkflow {
				body = workflowBody(flag)
			}
			var resource apitypes.Resource
			err := json.Unmarshal([]byte(body), &resource)
			return resource, err
		}
	}
	profile := adminhttp.RuntimeProfileUpsert{Id: "test-profile", Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{"assistant": audioInputTestBinding("assistant", apitypes.AudioInputPathModel)},
		Resources: apitypes.RuntimeProfileResources{Models: &map[string]apitypes.RuntimeProfileBinding{"llm": runtimeProfileTestBinding("chat")}},
	}}

	rejecting := &Server{DB: profileSQLTestDB(t), ResolveResource: resolver("")}
	response, err := rejecting.CreateRuntimeProfile(t.Context(), adminhttp.CreateRuntimeProfileRequestObject{Body: &profile})
	if err != nil {
		t.Fatalf("CreateRuntimeProfile(undeclared path) error = %v", err)
	}
	rejected, ok := response.(adminhttp.CreateRuntimeProfile400JSONResponse)
	if !ok || !strings.Contains(rejected.Error.Message, "requires a chat_model node that sets audio_transcript") {
		t.Fatalf("CreateRuntimeProfile(undeclared path) response = %#v", response)
	}

	server := &Server{DB: profileSQLTestDB(t), ResolveResource: resolver(`,"audio_transcript":true`)}
	response, err = server.CreateRuntimeProfile(t.Context(), adminhttp.CreateRuntimeProfileRequestObject{Body: &profile})
	if err != nil {
		t.Fatalf("CreateRuntimeProfile() error = %v", err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("CreateRuntimeProfile() response = %#v", response)
	}
	stored, err := server.ResolveProfile(t.Context(), "test-profile")
	if err != nil {
		t.Fatalf("ResolveProfile() error = %v", err)
	}
	if got := stored.Spec.Workflows["assistant"].AudioInput; got == nil || *got != apitypes.AudioInputPathModel {
		t.Fatalf("stored workflows.assistant.audio_input = %v", got)
	}
}
