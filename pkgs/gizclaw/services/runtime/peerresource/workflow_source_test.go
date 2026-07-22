package peerresource

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestListRuntimeWorkflowsUsesCollectionAliasesAndSkipsDanglingBindings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	workflows := &workflow.Server{Store: store}
	createWorkflowForCollectionTest(t, ctx, workflows, "runtime-chat")
	createWorkflowForCollectionTest(t, ctx, workflows, "runtime-translate")
	bindings := map[string]apitypes.RuntimeProfileBinding{
		"translate": collectionTestBinding("runtime-translate", "Translate"),
		"chat":      collectionTestBinding("runtime-chat", "Chat"),
		"missing":   collectionTestBinding("deleted-workflow", "Missing"),
	}
	server := &Server{Workflows: workflows}
	items, err := server.listRuntimeWorkflows(ctx, "assistants", bindings, []string{"chat", "missing", "translate"})
	if err != nil {
		t.Fatalf("listRuntimeWorkflows() error = %v", err)
	}
	aliases := make([]string, len(items))
	for i, item := range items {
		aliases[i] = item.Alias
		if item.Collection != "assistants" || item.I18n["en"].DisplayName == "" {
			t.Fatalf("workflow projection = %#v", item)
		}
		if item.Alias == "translate" && (item.WorkspaceLangPair == nil || *item.WorkspaceLangPair != "zh/ja") {
			t.Fatalf("translation workflow projection = %#v", item)
		}
	}
	if !reflect.DeepEqual(aliases, []string{"chat", "translate"}) {
		t.Fatalf("aliases = %#v", aliases)
	}
}

func TestWorkflowListRequiresCollection(t *testing.T) {
	server := &Server{Workflows: &workflow.Server{Store: kv.NewMemory(nil)}}
	params := rpcapi.RPCPayload{}
	if err := params.FromWorkflowListRequest(rpcapi.WorkflowListRequest{}); err != nil {
		t.Fatal(err)
	}
	response := server.handleWorkflowList(context.Background(), &rpcapi.RPCRequest{Id: "request", Params: &params})
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("response = %#v", response)
	}
}

func TestAliasGetsHideDanglingCanonicalResourceIDs(t *testing.T) {
	t.Parallel()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	models := map[string]apitypes.RuntimeProfileBinding{
		"chat": collectionTestBinding("tenant/model/canonical-secret", "Chat"),
	}
	voices := map[string]apitypes.RuntimeProfileBinding{
		"narrator": collectionTestBinding("volc-tenant:main:canonical-secret", "Narrator"),
	}
	profile := apitypes.RuntimeProfile{
		Name: "default", Revision: "r1",
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{Models: &models, Voices: &voices},
			Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
				"assistants": {
					"chat": collectionTestBinding("canonical-secret-workflow", "Chat"),
				},
			}},
		},
	}
	server := &Server{
		Workflows: &workflow.Server{Store: store},
		Models:    &model.Server{Store: store},
		Voices:    &voice.Server{Store: store},
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}

	var workflowPayload rpcapi.RPCPayload
	if err := workflowPayload.FromWorkflowGetRequest(rpcapi.WorkflowGetRequest{Alias: "chat"}); err != nil {
		t.Fatal(err)
	}
	assertAliasNotFound(t, server.handleWorkflowGet(context.Background(), &rpcapi.RPCRequest{Id: "workflow", Params: &workflowPayload}), "workflow not found", "canonical-secret-workflow")

	var modelPayload rpcapi.RPCPayload
	if err := modelPayload.FromModelGetRequest(rpcapi.ModelGetRequest{Alias: "chat"}); err != nil {
		t.Fatal(err)
	}
	assertAliasNotFound(t, server.handleModelGet(context.Background(), &rpcapi.RPCRequest{Id: "model", Params: &modelPayload}), "model not found", "tenant/model/canonical-secret")

	var voicePayload rpcapi.RPCPayload
	if err := voicePayload.FromVoiceGetRequest(rpcapi.VoiceGetRequest{Alias: "narrator"}); err != nil {
		t.Fatal(err)
	}
	assertAliasNotFound(t, server.handleVoiceGet(context.Background(), &rpcapi.RPCRequest{Id: "voice", Params: &voicePayload}), "voice not found", "volc-tenant:main:canonical-secret")
}

func TestListModelsProjectsRuntimeAliases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	models := &model.Server{Store: store}
	canonical := adminhttp.ModelUpsert{
		Id: "tenant-model-canonical", Kind: apitypes.ModelKindLlm, Source: apitypes.ModelSourceManual,
		Provider: apitypes.ModelProvider{Kind: apitypes.ModelProviderKindOpenaiTenant, Name: "primary"},
	}
	var providerData apitypes.ModelProviderData
	upstreamModel := "tenant-model-upstream"
	falseValue := false
	if err := providerData.FromOpenAITenantModelProviderData(apitypes.OpenAITenantModelProviderData{
		UpstreamModel:      upstreamModel,
		SupportJsonOutput:  &falseValue,
		SupportToolCalls:   &falseValue,
		SupportTextOnly:    &falseValue,
		UseSystemRole:      &falseValue,
		SupportTemperature: &falseValue,
		SupportThinking:    &falseValue,
	}); err != nil {
		t.Fatalf("FromOpenAITenantModelProviderData() error = %v", err)
	}
	canonical.ProviderData = providerData
	response, err := models.CreateModel(ctx, adminhttp.CreateModelRequestObject{Body: &canonical})
	if err != nil {
		t.Fatalf("CreateModel() error = %v", err)
	}
	if _, ok := response.(adminhttp.CreateModel200JSONResponse); !ok {
		t.Fatalf("CreateModel() response = %#v", response)
	}
	bindings := map[string]apitypes.RuntimeProfileBinding{
		"extract-model":  collectionTestBinding("tenant-model-canonical", "Extract Model"),
		"generate-model": collectionTestBinding("tenant-model-canonical", "Generate Model"),
		"missing-model":  collectionTestBinding("deleted-model", "Missing Model"),
	}
	profile := apitypes.RuntimeProfile{
		Name: "default", Revision: "r1",
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Models: &bindings}},
	}
	server := &Server{Models: models, RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}

	listed, err := server.ListModels(ctx, adminhttp.ListModelsRequestObject{})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	list, ok := listed.(adminhttp.ListModels200JSONResponse)
	if !ok {
		t.Fatalf("ListModels() response = %#v", listed)
	}
	ids := make([]string, len(list.Items))
	for i, item := range list.Items {
		ids[i] = item.Id
	}
	if want := []string{"extract-model", "generate-model"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ListModels() ids = %#v, want aliases %#v", ids, want)
	}
	gotResponse, err := server.GetModel(ctx, adminhttp.GetModelRequestObject{Id: "generate-model"})
	if err != nil {
		t.Fatalf("GetModel(alias) error = %v", err)
	}
	got, ok := gotResponse.(adminhttp.GetModel200JSONResponse)
	if !ok || got.Id != "generate-model" {
		t.Fatalf("GetModel(alias) = %#v", gotResponse)
	}
	canonicalResponse, err := server.GetModel(ctx, adminhttp.GetModelRequestObject{Id: "tenant-model-canonical"})
	if err != nil {
		t.Fatalf("GetModel(canonical) error = %v", err)
	}
	if _, ok := canonicalResponse.(adminhttp.GetModel404JSONResponse); !ok {
		t.Fatalf("GetModel(canonical) = %#v, want 404", canonicalResponse)
	}
}

func TestListVoicesProjectsRuntimeAliases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	voices := &voice.Server{Store: store}
	canonical := adminhttp.VoiceUpsert{
		Id: "openai-tenant:primary:canonical-voice", Source: apitypes.VoiceSourceManual,
		Provider: apitypes.VoiceProvider{Kind: apitypes.VoiceProviderKindOpenaiTenant, Name: "primary"},
	}
	response, err := voices.CreateVoice(ctx, adminhttp.CreateVoiceRequestObject{Body: &canonical})
	if err != nil {
		t.Fatalf("CreateVoice() error = %v", err)
	}
	if _, ok := response.(adminhttp.CreateVoice200JSONResponse); !ok {
		t.Fatalf("CreateVoice() response = %#v", response)
	}
	bindings := map[string]apitypes.RuntimeProfileBinding{
		"assistant-voice": collectionTestBinding(string(canonical.Id), "Assistant Voice"),
		"narrator-voice":  collectionTestBinding(string(canonical.Id), "Narrator Voice"),
		"missing-voice":   collectionTestBinding("openai-tenant:primary:deleted", "Missing Voice"),
	}
	profile := apitypes.RuntimeProfile{
		Name: "default", Revision: "r1",
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Voices: &bindings}},
	}
	server := &Server{Voices: voices, RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}

	listed, err := server.ListVoices(ctx, adminhttp.ListVoicesRequestObject{})
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	list, ok := listed.(adminhttp.ListVoices200JSONResponse)
	if !ok {
		t.Fatalf("ListVoices() response = %#v", listed)
	}
	ids := make([]string, len(list.Items))
	for i, item := range list.Items {
		ids[i] = string(item.Id)
	}
	if want := []string{"assistant-voice", "narrator-voice"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ListVoices() ids = %#v, want aliases %#v", ids, want)
	}
	gotResponse, err := server.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: "narrator-voice"})
	if err != nil {
		t.Fatalf("GetVoice(alias) error = %v", err)
	}
	got, ok := gotResponse.(adminhttp.GetVoice200JSONResponse)
	if !ok || got.Id != "narrator-voice" {
		t.Fatalf("GetVoice(alias) = %#v", gotResponse)
	}
	canonicalResponse, err := server.GetVoice(ctx, adminhttp.GetVoiceRequestObject{Id: canonical.Id})
	if err != nil {
		t.Fatalf("GetVoice(canonical) error = %v", err)
	}
	if _, ok := canonicalResponse.(adminhttp.GetVoice404JSONResponse); !ok {
		t.Fatalf("GetVoice(canonical) = %#v, want 404", canonicalResponse)
	}

	var listPayload rpcapi.RPCPayload
	if err := listPayload.FromVoiceListRequest(rpcapi.VoiceListRequest{}); err != nil {
		t.Fatal(err)
	}
	rpcListResponse := server.handleVoiceList(ctx, &rpcapi.RPCRequest{Id: "voice-list", Params: &listPayload})
	if rpcListResponse.Error != nil || rpcListResponse.Result == nil {
		t.Fatalf("handleVoiceList() response = %#v", rpcListResponse)
	}
	rpcList, err := rpcListResponse.Result.AsVoiceListResponse()
	if err != nil {
		t.Fatalf("AsVoiceListResponse() error = %v", err)
	}
	rpcAliases := make([]string, len(rpcList.Items))
	for i, item := range rpcList.Items {
		rpcAliases[i] = item.Alias
	}
	if want := []string{"assistant-voice", "narrator-voice"}; !reflect.DeepEqual(rpcAliases, want) {
		t.Fatalf("handleVoiceList() aliases = %#v, want %#v", rpcAliases, want)
	}

	var getPayload rpcapi.RPCPayload
	if err := getPayload.FromVoiceGetRequest(rpcapi.VoiceGetRequest{Alias: "narrator-voice"}); err != nil {
		t.Fatal(err)
	}
	rpcGetResponse := server.handleVoiceGet(ctx, &rpcapi.RPCRequest{Id: "voice-get", Params: &getPayload})
	if rpcGetResponse.Error != nil || rpcGetResponse.Result == nil {
		t.Fatalf("handleVoiceGet() response = %#v", rpcGetResponse)
	}
	rpcGet, err := rpcGetResponse.Result.AsVoiceGetResponse()
	if err != nil {
		t.Fatalf("AsVoiceGetResponse() error = %v", err)
	}
	if rpcGet.Value.Alias != "narrator-voice" {
		t.Fatalf("handleVoiceGet() alias = %q, want narrator-voice", rpcGet.Value.Alias)
	}
}

func assertAliasNotFound(t *testing.T, response *rpcapi.RPCResponse, message, canonicalID string) {
	t.Helper()
	if response == nil || response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeNotFound {
		t.Fatalf("response = %#v, want NOT_FOUND", response)
	}
	if response.Error.Message != message {
		t.Fatalf("message = %q, want %q", response.Error.Message, message)
	}
	if strings.Contains(response.Error.Message, canonicalID) {
		t.Fatalf("message %q exposes canonical ID %q", response.Error.Message, canonicalID)
	}
}

func createWorkflowForCollectionTest(t *testing.T, ctx context.Context, server *workflow.Server, name string) {
	t.Helper()
	var flowcraftSpec apitypes.FlowcraftWorkflowSpec
	if err := json.Unmarshal([]byte(`{
		"agent": {
			"id": "assistant",
			"name": "Assistant",
			"graph": {
				"name": "assistant",
				"entry": "answer",
				"nodes": [{"id": "answer", "type": "passthrough", "publish": true}],
				"edges": [{"from": "answer", "to": "__end__"}]
			}
		}
	}`), &flowcraftSpec); err != nil {
		t.Fatalf("decode test Flowcraft config: %v", err)
	}
	spec := apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft, Flowcraft: &flowcraftSpec}
	if strings.Contains(name, "translate") {
		langPair := "zh/ja"
		spec = apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverAstTranslate,
			AstTranslate: &apitypes.ASTTranslateWorkflowSpec{
				TranslationModel: "translation-model",
				LangPair:         &langPair,
			},
		}
	}
	document := apitypes.Workflow{Name: name, Spec: spec}
	response, err := server.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &document})
	if err != nil {
		t.Fatalf("CreateWorkflow(%q) error = %v", name, err)
	}
	if _, ok := response.(adminhttp.CreateWorkflow200JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(%q) response = %#v", name, response)
	}
}

func collectionTestBinding(resourceID, displayName string) apitypes.RuntimeProfileBinding {
	return apitypes.RuntimeProfileBinding{ResourceId: resourceID, I18n: map[string]apitypes.RuntimeProfileI18nText{
		"en": {DisplayName: displayName}, "zh-CN": {DisplayName: displayName},
	}}
}
