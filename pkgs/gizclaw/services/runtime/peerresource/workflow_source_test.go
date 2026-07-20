package peerresource

import (
	"context"
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
	document := apitypes.Workflow{Name: name, Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverFlowcraft}}
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
