//go:build gizclaw_e2e

package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestEinoWorkflowInvokesHTTPAndCurrentPeerTools(t *testing.T) {
	if err := probeLiveWorkspaceSetup(); err != nil {
		t.Fatalf("required e2e setup server is not available: %v", err)
	}
	runID := time.Now().UnixNano()
	httpTool := fmt.Sprintf("e2e_http_token_%x", runID)
	clientTool := fmt.Sprintf("e2e_client_token_%x", runID)
	httpToken := fmt.Sprintf("HTTP_TOOL_OK_%X", runID)
	clientToken := fmt.Sprintf("CLIENT_TOOL_OK_%X", runID)

	registrationToken := createChatRegistrationToken(t, workspaceCaseTextRoundtrip)
	path := filepath.Join("..", "..", "testdata", "workspaces", "eino-memory.json")
	cfg, err := loadConfig(path, clientContextConfigPath())
	if err != nil {
		t.Fatalf("load Eino Tool E2E config: %v", err)
	}
	restoreCfg := cfg
	restoreCfg.workspaceSuffix = fmt.Sprintf("tool-restore-%x", runID)
	restoreWorkspace, err := prepareChatToolRestoreWorkspace(
		restoreCfg,
		workspaceCaseTextRoundtrip,
		registrationToken,
	)
	if err != nil {
		t.Fatalf("prepare Tool E2E restore Workspace: %v", err)
	}
	workflowName := configureChatToolResources(t, runID, map[string]apitypes.ToolSpec{
		httpTool:   httpTokenToolSpec(t),
		clientTool: clientTokenToolSpec(t),
	})
	t.Setenv("GIZCLAW_E2E_CHAT_REGISTRATION_TOKEN", registrationToken)

	cfg.Workflow.Name = workflowName
	cfg.Workflow.Memory = ""
	cfg.workspaceSuffix = fmt.Sprintf("tools-%x", runID)
	cfg.toolIDs = []string{httpTool, clientTool}
	var clientCalls atomic.Int32
	cfg.toolHandlers = map[string]gizcli.ToolHandler{
		clientTool: func(_ context.Context, arguments json.RawMessage) (json.RawMessage, error) {
			var input struct {
				Key string `json:"key"`
			}
			if err := json.Unmarshal(arguments, &input); err != nil {
				return nil, fmt.Errorf("decode Client Tool arguments: %w", err)
			}
			if input.Key != "current-peer" {
				return nil, fmt.Errorf("unexpected Client Tool key %q", input.Key)
			}
			clientCalls.Add(1)
			result, err := json.Marshal(map[string]string{"token": clientToken})
			return result, err
		},
	}
	cfg.Utterances = []string{fmt.Sprintf(
		"必须先调用工具 %s，参数 key 必须是 %q；收到结果后再调用工具 %s，参数 value 必须是 %q。"+
			"两个工具都成功后，只用一句话原样写出两个工具返回的 token，不要猜测。",
		clientTool, "current-peer", httpTool, httpToken,
	)}

	result, runErr := runLoadedConfigWithResultAndInspect(
		cfg,
		workspaceCaseTextRoundtrip,
		func(ctx context.Context, client *gizcli.Client, _ config) error {
			listed, listErr := client.ListTools(ctx, "tool-e2e.list", rpcapi.ToolListRequest{})
			if listErr != nil {
				return fmt.Errorf("list Tool E2E RuntimeProfile catalog: %w", listErr)
			}
			if len(listed.Items) != 2 {
				return fmt.Errorf("Tool E2E RuntimeProfile catalog has %d Tools, want 2", len(listed.Items))
			}
			state, stateErr := client.GetServerRunWorkspace(ctx, "tool-e2e.runtime")
			if stateErr != nil {
				return fmt.Errorf("read Tool E2E runtime state: %w", stateErr)
			}
			if state.Message != nil && strings.TrimSpace(*state.Message) != "" {
				return fmt.Errorf("Tool E2E runtime state=%s: %s", state.RuntimeState, *state.Message)
			}
			return nil
		},
	)
	cleanupErr := cleanupChatToolWorkspace(
		cfg,
		workspaceCaseTextRoundtrip,
		registrationToken,
		restoreWorkspace,
	)
	if err := errors.Join(runErr, cleanupErr); err != nil {
		t.Fatalf("run Eino Tool E2E (client calls=%d): %v", clientCalls.Load(), err)
	}
	if clientCalls.Load() != 1 {
		t.Fatalf("current-Peer Client Tool calls = %d, want 1", clientCalls.Load())
	}
	if len(result.Rounds) != 1 {
		t.Fatalf("Eino Tool E2E rounds = %d, want 1", len(result.Rounds))
	}
	answer := result.Rounds[0].AssistantText
	if !strings.Contains(answer, httpToken) || !strings.Contains(answer, clientToken) {
		t.Fatalf("Eino Tool E2E response = %q, want HTTP token %q and Client token %q", answer, httpToken, clientToken)
	}
	t.Logf("verified http_request=%s client_rpc=%s client_calls=%d", httpToken, clientToken, clientCalls.Load())
}

func prepareChatToolRestoreWorkspace(
	fallback config,
	selectedCase workspaceCase,
	registrationToken string,
) (workspace string, resultErr error) {
	selectedFallback, err := selectedCase.applyConfig(fallback)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, serveDone, err := dialClient(fallback)
	if err != nil {
		return "", fmt.Errorf("dial Tool E2E restore client: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, client.Close())
		<-serveDone
	}()
	if _, err := client.Register(ctx, "tool-e2e.restore.register", registrationToken); err != nil {
		return "", fmt.Errorf("register Tool E2E restore client: %w", err)
	}
	state, err := client.GetServerRunWorkspace(ctx, "tool-e2e.restore.get")
	if err != nil {
		return "", fmt.Errorf("read Tool E2E restore Workspace: %w", err)
	}
	if current := strings.TrimSpace(state.WorkspaceName); current != "" {
		workspace, workspaceErr := client.GetWorkspace(
			ctx,
			"tool-e2e.restore.workspace.get",
			rpcapi.WorkspaceGetRequest{Name: current},
		)
		if workspaceErr == nil {
			if _, workflowErr := client.GetWorkflow(
				ctx,
				"tool-e2e.restore.workflow.get",
				rpcapi.WorkflowGetRequest{Name: workspace.Value.WorkflowName},
			); workflowErr == nil {
				return current, nil
			} else if !isRPCNotFound(workflowErr) {
				return "", fmt.Errorf("validate Tool E2E restore Workflow: %w", workflowErr)
			}
		} else if !isRPCNotFound(workspaceErr) {
			return "", fmt.Errorf("validate Tool E2E restore Workspace: %w", workspaceErr)
		}
	}
	if _, err := ensureWorkspace(ctx, client, selectedFallback); err != nil {
		return "", fmt.Errorf("create Tool E2E fallback Workspace: %w", err)
	}
	if err := selectAndReloadAgent(ctx, client, selectedFallback); err != nil {
		return "", fmt.Errorf("select Tool E2E fallback Workspace: %w", err)
	}
	return selectedFallback.Workspace, nil
}

func cleanupChatToolWorkspace(
	cfg config,
	selectedCase workspaceCase,
	registrationToken string,
	restoreWorkspace string,
) (resultErr error) {
	selected, err := selectedCase.applyConfig(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, serveDone, err := dialClient(cfg)
	if err != nil {
		return fmt.Errorf("dial Tool E2E cleanup client: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, client.Close())
		<-serveDone
	}()
	if _, err := client.Register(ctx, "tool-e2e.cleanup.register", registrationToken); err != nil {
		return fmt.Errorf("register Tool E2E cleanup client: %w", err)
	}
	if _, err := client.StopServerRun(ctx, "tool-e2e.cleanup.stop"); err != nil {
		resultErr = errors.Join(resultErr, fmt.Errorf("stop Tool E2E Workspace: %w", err))
	}
	if restoreWorkspace == selected.Workspace {
		resultErr = errors.Join(resultErr, fmt.Errorf("Tool E2E restore Workspace must differ from temporary Workspace %q", selected.Workspace))
	} else if err := selectAndReloadAgent(ctx, client, config{Workspace: restoreWorkspace}); err != nil {
		resultErr = errors.Join(
			resultErr,
			fmt.Errorf("restore Tool E2E Workspace %q: %w", restoreWorkspace, err),
		)
	}
	if _, err := client.DeleteWorkspace(
		ctx,
		"tool-e2e.cleanup.delete",
		rpcapi.WorkspaceDeleteRequest{Name: selected.Workspace},
	); err != nil && !isRPCNotFound(err) {
		resultErr = errors.Join(
			resultErr,
			fmt.Errorf("delete Tool E2E Workspace %q: %w", selected.Workspace, err),
		)
	}
	return resultErr
}

func httpTokenToolSpec(t *testing.T) apitypes.ToolSpec {
	t.Helper()
	auth := apitypes.ToolHTTPAuth{}
	if err := auth.FromToolHTTPAuthNone(apitypes.ToolHTTPAuthNone{
		Method: apitypes.ToolHTTPAuthNoneMethodNone,
	}); err != nil {
		t.Fatalf("encode HTTP Tool auth: %v", err)
	}
	responsePointer := "/args/value"
	query := []apitypes.ToolHTTPArgumentBinding{{
		ArgumentPointer: "/value",
		Target:          "value",
		Required:        boolPtr(true),
	}}
	statuses := []int{200}
	description := "Return the exact verification value through a real HTTPS request."
	var spec apitypes.ToolSpec
	if err := spec.FromHTTPToolSpec(apitypes.HTTPToolSpec{
		Description: &description,
		InputSchema: exactObjectSchema("value"),
		Http: apitypes.ToolHTTPRequest{
			Auth: auth, Method: apitypes.ToolHTTPMethodGET,
			Url: "https://postman-echo.com/get", Query: &query,
			ResponsePointer: &responsePointer, SuccessStatusCodes: &statuses,
			Timeout: "20s", MaxResponseBytes: 64 << 10,
		},
	}); err != nil {
		t.Fatalf("encode HTTP Tool spec: %v", err)
	}
	return spec
}

func clientTokenToolSpec(t *testing.T) apitypes.ToolSpec {
	t.Helper()
	description := "Ask the currently connected Peer for its private verification token."
	var spec apitypes.ToolSpec
	if err := spec.FromClientRPCToolSpec(apitypes.ClientRPCToolSpec{
		Description: &description,
		InputSchema: exactObjectSchema("key"),
	}); err != nil {
		t.Fatalf("encode Client Tool spec: %v", err)
	}
	return spec
}

func exactObjectSchema(required string) jsonschema.Schema {
	return jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			required: {Type: "string"},
		},
		Required:             []string{required},
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
	}
}

func configureChatToolResources(
	t *testing.T,
	runID int64,
	specs map[string]apitypes.ToolSpec,
) string {
	t.Helper()
	h := clitest.NewSetupHarness(t, "go-chat-tools")
	identitiesHome := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_IDENTITIES_HOME"))
	if identitiesHome == "" {
		identitiesHome = filepath.Join(h.RepoRoot, "tests", "gizclaw-e2e", "testdata", "identities")
	}
	adminContext := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_ADMIN_IDENTITY"))
	if adminContext == "" {
		adminContext = "admin"
	}
	h.SetContextDirAlias("admin-a", filepath.Join(identitiesHome, adminContext))
	admin := h.ConnectClientFromContextEventually("admin-a", 30*time.Second)
	t.Cleanup(func() { _ = admin.Close() })
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create Tool E2E admin client: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	workflowName := fmt.Sprintf("e2e-tool-workflow-%x", runID)
	workflow := toolE2EWorkflow(t, workflowName)
	var workflowResource apitypes.Resource
	if err := workflowResource.FromWorkflowResource(workflow); err != nil {
		t.Fatalf("encode Tool E2E Workflow resource: %v", err)
	}
	workflowResponse, err := api.ApplyResourceWithResponse(ctx, workflowResource)
	if err != nil {
		t.Fatalf("apply Tool E2E Workflow resource: %v", err)
	}
	if workflowResponse.JSON200 == nil || workflowResponse.JSON200.Id == nil {
		t.Fatalf(
			"apply Tool E2E Workflow resource status %d: %s",
			workflowResponse.StatusCode(),
			strings.TrimSpace(string(workflowResponse.Body)),
		)
	}
	workflowID := *workflowResponse.JSON200.Id
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = api.DeleteResourceWithResponse(cleanupCtx, apitypes.ResourceKindWorkflow, workflowID)
	})

	bindings := make(map[string]apitypes.RuntimeProfileBinding, len(specs))
	bindingIndex := 0
	for name, spec := range specs {
		var resource apitypes.Resource
		if err := resource.FromToolResource(apitypes.ToolResource{
			ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
			Kind:       apitypes.ToolResourceKindTool,
			Metadata:   apitypes.ResourceMetadata{Id: name},
			Spec:       spec,
		}); err != nil {
			t.Fatalf("encode Tool resource %q: %v", name, err)
		}
		response, err := api.ApplyResourceWithResponse(ctx, resource)
		if err != nil {
			t.Fatalf("apply Tool resource %q: %v", name, err)
		}
		if response.JSON200 == nil || response.JSON200.Id == nil {
			t.Fatalf("apply Tool resource %q status %d: %s", name, response.StatusCode(), strings.TrimSpace(string(response.Body)))
		}
		bindingIndex++
		alias := fmt.Sprintf("tool-e2e-%d", bindingIndex)
		bindings[alias] = apitypes.RuntimeProfileBinding{
			ResourceId: *response.JSON200.Id,
			I18n: map[string]apitypes.RuntimeProfileI18nText{
				"en":    {DisplayName: name},
				"zh-CN": {DisplayName: name},
			},
		}
		toolID := *response.JSON200.Id
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_, _ = api.DeleteResourceWithResponse(cleanupCtx, apitypes.ResourceKindTool, toolID)
		})
	}

	const profileName = "e2e-chat"
	current, found, err := clitest.RuntimeProfileByID(ctx, api, profileName)
	if err != nil || !found {
		t.Fatalf("get Tool E2E RuntimeProfile: %v", err)
	}
	originalSpec, err := cloneRuntimeProfileSpec(current.Spec)
	if err != nil {
		t.Fatalf("snapshot Tool E2E RuntimeProfile: %v", err)
	}
	profileSpec, err := cloneRuntimeProfileSpec(originalSpec)
	if err != nil {
		t.Fatalf("clone Tool E2E RuntimeProfile: %v", err)
	}
	profileSpec.Resources.Tools = &bindings
	profileSpec.Workflows.Collections["assistants"][workflowName] = apitypes.RuntimeProfileBinding{
		ResourceId: workflowID,
		I18n: map[string]apitypes.RuntimeProfileI18nText{
			"en":    {DisplayName: "E2E Tool workflow"},
			"zh-CN": {DisplayName: "E2E Tool 工作流"},
		},
	}
	updated, err := api.PutRuntimeProfileWithResponse(ctx, current.Id, adminhttp.RuntimeProfileUpsert{
		Id:   profileName,
		Spec: profileSpec,
	})
	if err != nil {
		t.Fatalf("put Tool E2E RuntimeProfile: %v", err)
	}
	if updated.JSON200 == nil {
		t.Fatalf("put Tool E2E RuntimeProfile status %d: %s", updated.StatusCode(), strings.TrimSpace(string(updated.Body)))
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		restored, restoreErr := api.PutRuntimeProfileWithResponse(cleanupCtx, current.Id, adminhttp.RuntimeProfileUpsert{
			Id:   profileName,
			Spec: originalSpec,
		})
		if restoreErr != nil {
			t.Errorf("restore Tool E2E RuntimeProfile: %v", restoreErr)
			return
		}
		if restored.JSON200 == nil {
			t.Errorf(
				"restore Tool E2E RuntimeProfile status %d: %s",
				restored.StatusCode(),
				strings.TrimSpace(string(restored.Body)),
			)
			return
		}
		actual, getErr := api.GetRuntimeProfileWithResponse(cleanupCtx, current.Id)
		if getErr != nil {
			t.Errorf("verify restored Tool E2E RuntimeProfile: %v", getErr)
			return
		}
		if actual.JSON200 == nil {
			t.Errorf(
				"verify restored Tool E2E RuntimeProfile status %d: %s",
				actual.StatusCode(),
				strings.TrimSpace(string(actual.Body)),
			)
			return
		}
		wantJSON, marshalErr := json.Marshal(originalSpec)
		if marshalErr != nil {
			t.Errorf("encode original Tool E2E RuntimeProfile: %v", marshalErr)
			return
		}
		gotJSON, marshalErr := json.Marshal(actual.JSON200.Spec)
		if marshalErr != nil {
			t.Errorf("encode restored Tool E2E RuntimeProfile: %v", marshalErr)
			return
		}
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("restored Tool E2E RuntimeProfile differs from its original snapshot")
		}
	})
	return workflowName
}

func cloneRuntimeProfileSpec(source apitypes.RuntimeProfileSpec) (apitypes.RuntimeProfileSpec, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return apitypes.RuntimeProfileSpec{}, err
	}
	var clone apitypes.RuntimeProfileSpec
	if err := json.Unmarshal(data, &clone); err != nil {
		return apitypes.RuntimeProfileSpec{}, err
	}
	return clone, nil
}

func toolE2EWorkflow(t *testing.T, name string) apitypes.WorkflowResource {
	t.Helper()
	outputs := map[string]string{"text": "answer"}
	inputs := map[string]apitypes.EinoBinding{
		"messages": {From: "input.messages"},
	}
	maxTokens := 256
	var node apitypes.EinoNode
	if err := node.FromEinoChatModelNode(apitypes.EinoChatModelNode{
		Id: "model", Type: apitypes.EinoChatModelNodeTypeChatModel,
		Model: "llm", Inputs: &inputs, Outputs: &outputs, MaxTokens: &maxTokens,
	}); err != nil {
		t.Fatalf("encode Tool E2E Eino model node: %v", err)
	}
	primary := true
	maxRunSteps := 8
	return apitypes.WorkflowResource{
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.WorkflowResourceKindWorkflow,
		Metadata:   apitypes.ResourceMetadata{Id: name},
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverEino,
			Eino: &apitypes.EinoWorkflowSpec{Graph: apitypes.EinoGraph{
				Name: name,
				Compile: apitypes.EinoGraphCompile{
					NodeTriggerMode: apitypes.EinoGraphCompileNodeTriggerModeAnyPredecessor,
					MaxRunSteps:     &maxRunSteps,
				},
				State: apitypes.EinoState{Fields: []apitypes.EinoStateField{{
					Name: "answer", Type: apitypes.EinoStateFieldTypeString,
					Merge: apitypes.EinoStateFieldMergeReplace,
				}}},
				Nodes: []apitypes.EinoNode{node},
				Edges: []apitypes.EinoEdge{
					{From: "start", To: "model"},
					{From: "model", To: "end"},
				},
				Branches: []apitypes.EinoBranch{},
				Outputs: []apitypes.EinoOutput{{
					Node: "model", Field: "answer", Name: "assistant",
					MimeType: "text/plain", Primary: &primary,
				}},
			}},
		},
	}
}
