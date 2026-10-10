package runtimeprofile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestProfileASRModelsNormalizeRevisionAndAgreement(t *testing.T) {
	binding := runtimeProfileTestWorkflowBinding("assistant")
	binding.PttAsrModel = new(" ptt-asr ")
	binding.RealtimeAsrModel = new("stream-asr")
	spec := apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"assistant": binding}}
	got, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: spec}, "")
	if err != nil {
		t.Fatal(err)
	}
	if *got.Spec.Workflows["assistant"].PttAsrModel != "ptt-asr" {
		t.Fatal("ASR alias not normalized")
	}
	plain := runtimeProfileTestWorkflowBinding("assistant")
	without, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"assistant": plain}}}, "")
	if err != nil || without.Revision == got.Revision {
		t.Fatalf("ASR selection missing from revision: %v", err)
	}
	spec.Workflows["other"] = plain
	if _, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: spec}, ""); err == nil || !strings.Contains(err.Error(), "ASR Models conflict") {
		t.Fatalf("inconsistent aliases accepted: %v", err)
	}
	binding.PttAsrModel = new("unsafe?query=true")
	spec.Workflows = apitypes.RuntimeProfileWorkflows{"assistant": binding}
	if _, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: "test", Spec: spec}, ""); err == nil {
		t.Fatal("invalid ASR Model alias accepted")
	}
}

func TestProfileASRReferencesRequireASRModels(t *testing.T) {
	binding := runtimeProfileTestWorkflowBinding("assistant")
	binding.PttAsrModel = new("asr")
	for _, driver := range []apitypes.WorkflowDriver{apitypes.WorkflowDriverEino, apitypes.WorkflowDriverDoubaoRealtime} {
		models := map[string]apitypes.ModelResource{"asr": {Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindAsr}}}
		if err := validateWorkflowASRModels("workflows.assistant", driver, binding, models); err != nil {
			t.Fatal(err)
		}
		models["asr"] = apitypes.ModelResource{Spec: apitypes.ModelSpec{Kind: apitypes.ModelKindLlm}}
		if err := validateWorkflowASRModels("workflows.assistant", driver, binding, models); err == nil || !strings.Contains(err.Error(), "want asr") {
			t.Fatalf("invalid Model kind accepted: %v", err)
		}
	}
	for _, driver := range []apitypes.WorkflowDriver{apitypes.WorkflowDriverDoubaoRealtimeDuplex, apitypes.WorkflowDriverDashscopeRealtime} {
		if err := validateWorkflowASRModels("workflows.assistant", driver, binding, nil); err == nil {
			t.Fatalf("audio-only driver %s accepted external ASR", driver)
		}
	}
	if err := validateWorkflowASRModels("workflows.assistant", apitypes.WorkflowDriverEino, binding, nil); err == nil {
		t.Fatal("missing ASR binding accepted")
	}
}

func TestProfilePersistsASRModelsAndIgnoresLegacyWorkflowSlot(t *testing.T) {
	const workflow = `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Workflow","metadata":{"id":"assistant"},"spec":{"driver":"eino","eino":{"voice_adapter":{"asr_model":"unbound-legacy-slot"},"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor"},"state":{"fields":[]},"nodes":[{"id":"reply","type":"chat_model","model":"llm"}],"edges":[],"branches":[],"outputs":[]}}}}`
	resolver := func(_ context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
		body := workflow
		if kind == apitypes.ResourceKindModel {
			modelKind := "llm"
			if id == "speech" {
				modelKind = "asr"
			}
			body = `{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"` + id + `"},"spec":{"kind":"` + modelKind + `","source":"manual","provider":{"kind":"volc-tenant","id":"volc"},"provider_data":{"api_mode":"chat_completions"}}}`
		}
		var resource apitypes.Resource
		err := json.Unmarshal([]byte(body), &resource)
		return resource, err
	}
	server := &Server{DB: profileSQLTestDB(t), ResolveResource: resolver}
	binding := runtimeProfileTestWorkflowBinding("assistant")
	binding.RealtimeAsrModel = new("asr")
	profile := adminhttp.RuntimeProfileUpsert{Id: "profile", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{"assistant": binding}, Resources: apitypes.RuntimeProfileResources{Models: new(map[string]apitypes.RuntimeProfileBinding{"llm": runtimeProfileTestBinding("chat"), "asr": runtimeProfileTestBinding("speech")})}}}
	response, err := server.CreateRuntimeProfile(t.Context(), adminhttp.CreateRuntimeProfileRequestObject{Body: &profile})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("legacy Workflow incorrectly required its ASR slot: %#v", response)
	}
	stored, err := server.ResolveProfile(t.Context(), "profile")
	if err != nil {
		t.Fatal(err)
	}
	got := stored.Spec.Workflows["assistant"]
	if got.PttAsrModel != nil || got.RealtimeAsrModel == nil || *got.RealtimeAsrModel != "asr" {
		t.Fatalf("stored ASR Models=%#v", got)
	}
}
