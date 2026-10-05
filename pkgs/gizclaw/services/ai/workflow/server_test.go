package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/gofiber/fiber/v2"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestServerWorkflowsCRUD(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	createDoc := mustDocument(t, `{"id":"demo-assistant","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"pet-care.model","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]},"voice_adapter":{"asr_model":"pet-care.asr","default_voice":"pet-care.pet","node_voices":{"answer":"pet-care.answer"}}}}}`)

	createResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &createDoc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}
	if got := workflowDriver(t, apitypes.Workflow(created)); got != "eino" {
		t.Fatalf("CreateWorkflow() driver = %q", got)
	}

	listResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	listed, ok := listResp.(adminhttp.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkflows() response = %#v", listResp)
	}
	if len(listed.Items) != 1 || listed.HasNext {
		t.Fatalf("ListWorkflows() = %#v", listed)
	}

	getResp, err := srv.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	gotDoc, ok := getResp.(adminhttp.GetWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("GetWorkflow() response = %#v", getResp)
	}
	gotSingle := mustSingle(t, apitypes.Workflow(gotDoc))
	if gotSingle.Id != "demo-assistant" {
		t.Fatalf("GetWorkflow() name = %q", gotSingle.Id)
	}
	assertWorkflowDottedAliases(t, gotSingle, "pet-care.model", "pet-care.asr", "pet-care.pet", "pet-care.answer")

	updateDoc := mustDocument(t, `{"id":"demo-assistant","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"story-teller.model","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]},"voice_adapter":{"asr_model":"story-teller.asr","default_voice":"story-teller.narrator","node_voices":{"answer":"story-teller.answer"}}}}}`)
	putResp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{
		Id:   created.Id,
		Body: &updateDoc,
	})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	putDoc, ok := putResp.(adminhttp.PutWorkflow200JSONResponse)
	if !ok {
		t.Fatalf("PutWorkflow() response = %#v", putResp)
	}
	putSingle := mustSingle(t, apitypes.Workflow(putDoc))
	if putSingle.Spec.Eino == nil || putSingle.Spec.Eino.Graph.Name != "assistant" {
		t.Fatalf("PutWorkflow() spec = %#v", putSingle.Spec)
	}
	assertWorkflowDottedAliases(t, putSingle, "story-teller.model", "story-teller.asr", "story-teller.narrator", "story-teller.answer")

	deleteResp, err := srv.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteWorkflow() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkflow200JSONResponse); !ok {
		t.Fatalf("DeleteWorkflow() response = %#v", deleteResp)
	}

	getAfterDelete, err := srv.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: created.Id})
	if err != nil {
		t.Fatalf("GetWorkflow() after delete error = %v", err)
	}
	if _, ok := getAfterDelete.(adminhttp.GetWorkflow404JSONResponse); !ok {
		t.Fatalf("GetWorkflow() after delete response = %#v", getAfterDelete)
	}
}

func assertWorkflowDottedAliases(t *testing.T, workflow adminhttp.WorkflowUpsert, model, asr, defaultVoice, nodeVoice string) {
	t.Helper()
	if workflow.Spec.Eino == nil || workflow.Spec.Eino.VoiceAdapter == nil ||
		workflow.Spec.Eino.VoiceAdapter.AsrModel == nil || *workflow.Spec.Eino.VoiceAdapter.AsrModel != asr ||
		workflow.Spec.Eino.VoiceAdapter.DefaultVoice == nil || *workflow.Spec.Eino.VoiceAdapter.DefaultVoice != defaultVoice ||
		workflow.Spec.Eino.VoiceAdapter.NodeVoices == nil || (*workflow.Spec.Eino.VoiceAdapter.NodeVoices)["answer"] != nodeVoice {
		t.Fatalf("EinoPorted voice aliases = %#v", workflow.Spec.Eino)
	}
	raw, err := json.Marshal(workflow.Spec.Eino)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"model":"`+model+`"`) {
		t.Fatalf("EinoPorted model alias was not preserved: %s", raw)
	}
}

func TestServerRejectsUnknownWorkflowDriver(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := adminhttp.WorkflowUpsert{
		Id:   "bad-workflow",
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriver("bad-driver")},
	}

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestValidateDriverSpecRejectsDoubaoRealtimeTools(t *testing.T) {
	tools := []apitypes.DoubaoRealtimeFunctionTool{{
		Type: apitypes.DoubaoRealtimeFunctionToolTypeFunction,
		Name: "get_weather",
	}}
	err := validateDriverSpec(apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDoubaoRealtime,
		DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{
			Model: "doubao-realtime",
			Tools: &tools,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "tools are unsupported") {
		t.Fatalf("validateDriverSpec() error = %v", err)
	}
}

func TestValidateDriverSpecRequiresDoubaoRealtimeConfigAndModel(t *testing.T) {
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverDoubaoRealtime}); err == nil || !strings.Contains(err.Error(), "spec.doubao_realtime is required") {
		t.Fatalf("validateDriverSpec(missing config) error = %v", err)
	}
	if err := validateDriverSpec(apitypes.WorkflowSpec{
		Driver:         apitypes.WorkflowDriverDoubaoRealtime,
		DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{},
	}); err == nil || !strings.Contains(err.Error(), "spec.doubao_realtime.model is required") {
		t.Fatalf("validateDriverSpec(missing model) error = %v", err)
	}
	if err := validateDriverSpec(apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDoubaoRealtime,
		DoubaoRealtime: &apitypes.DoubaoRealtimeWorkflowSpec{
			Model: "doubao-realtime",
		},
	}); err != nil {
		t.Fatalf("validateDriverSpec(valid config) error = %v", err)
	}
}

func TestValidateDriverSpecRejectsInvalidEinoGraph(t *testing.T) {
	err := validateDriverSpec(apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverEino,
		Eino:   &apitypes.EinoWorkflowSpec{},
	})
	if err == nil || !strings.Contains(err.Error(), "spec.eino") ||
		!strings.Contains(err.Error(), "requires Nodes") {
		t.Fatalf("validateDriverSpec(invalid Eino graph) error = %v", err)
	}
}

func TestValidateDriverSpecRejectsInvalidRealtimeOptions(t *testing.T) {
	temperature := float32(3)
	err := validateDriverSpec(apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDashscopeRealtime,
		DashscopeRealtime: &apitypes.DashScopeRealtimeWorkflowSpec{
			Model:       "dashscope-realtime",
			Temperature: &temperature,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "temperature") {
		t.Fatalf("validateDriverSpec(DashScope temperature) error = %v", err)
	}

	sampleRate := apitypes.DoubaoRealtimeDuplexWorkflowSpecSampleRate(16000)
	err = validateDriverSpec(apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverDoubaoRealtimeDuplex,
		DoubaoRealtimeDuplex: &apitypes.DoubaoRealtimeDuplexWorkflowSpec{
			Model:      "doubao-realtime-duplex",
			SampleRate: &sampleRate,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "sample_rate") {
		t.Fatalf("validateDriverSpec(Doubao sample rate) error = %v", err)
	}
}

func TestServerRejectsEmptyEinoPortedSpec(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	empty := apitypes.EinoWorkflowSpec{}
	doc := adminhttp.WorkflowUpsert{Id: "empty-eino", Spec: apitypes.WorkflowSpec{
		Driver: apitypes.WorkflowDriverEino, Eino: &empty,
	}}

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerRejectsUserSFUWorkflowSpecs(t *testing.T) {
	t.Parallel()

	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu, Sfu: &apitypes.SFUWorkflowSpec{}}); err != nil {
		t.Fatalf("validateDriverSpec(empty sfu) error = %v", err)
	}
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu}); err == nil || !strings.Contains(err.Error(), "spec.sfu is required") {
		t.Fatalf("validateDriverSpec(missing sfu) error = %v", err)
	}
	nonEmpty := apitypes.SFUWorkflowSpec{"url": "wss://sfu"}
	if err := validateDriverSpec(apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverSfu, Sfu: &nonEmpty}); err == nil || !strings.Contains(err.Error(), "empty object") {
		t.Fatalf("validateDriverSpec(non-empty sfu) error = %v", err)
	}
	var decoded adminhttp.WorkflowUpsert
	if err := json.Unmarshal([]byte(`{"id": "user-sfu", "spec": {"driver": "sfu", "sfu": {"room": "x"}}}`), &decoded); err == nil || !strings.Contains(err.Error(), "empty object") {
		t.Fatalf("decode non-empty sfu spec error = %v", err)
	}
}

func TestServerMaterializesProtectedBuiltinSFUWorkflow(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	for range 2 {
		if err := srv.EnsureBuiltinWorkflows(ctx); err != nil {
			t.Fatalf("EnsureBuiltinWorkflows() error = %v", err)
		}
	}
	getResp, err := srv.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: socialutil.SFUWorkflowID})
	if err != nil {
		t.Fatalf("GetWorkflow(builtin) error = %v", err)
	}
	got, ok := getResp.(adminhttp.GetWorkflow200JSONResponse)
	if !ok || got.Id != socialutil.SFUWorkflowID || got.Spec.Driver != apitypes.WorkflowDriverSfu || got.Spec.Sfu == nil || len(*got.Spec.Sfu) != 0 {
		t.Fatalf("GetWorkflow(builtin) = %#v", getResp)
	}
	userDoc := adminhttp.WorkflowUpsert{Id: "user-flow", Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverAstTranslate, AstTranslate: &apitypes.ASTTranslateWorkflowSpec{}}}
	if _, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &userDoc}); err != nil {
		t.Fatalf("CreateWorkflow(user) error = %v", err)
	}
	listResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	list, ok := listResp.(adminhttp.ListWorkflows200JSONResponse)
	if !ok || len(list.Items) != 1 || list.Items[0].Id != "user-flow" || list.HasNext {
		t.Fatalf("ListWorkflows() = %#v, want only user Workflows", listResp)
	}
	builtin := adminhttp.WorkflowUpsert{Id: socialutil.SFUWorkflowID, Spec: BuiltinSFUWorkflow().Spec}
	createResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &builtin})
	if err != nil {
		t.Fatalf("CreateWorkflow(builtin) error = %v", err)
	}
	if conflict, ok := createResp.(adminhttp.CreateWorkflow409JSONResponse); !ok || conflict.Error.Code != BuiltinWorkflowCode {
		t.Fatalf("CreateWorkflow(builtin) response = %#v", createResp)
	}
	putResp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Id: socialutil.SFUWorkflowID, Body: &builtin})
	if err != nil {
		t.Fatalf("PutWorkflow(builtin) error = %v", err)
	}
	if rejected, ok := putResp.(adminhttp.PutWorkflow400JSONResponse); !ok || rejected.Error.Code != BuiltinWorkflowCode {
		t.Fatalf("PutWorkflow(builtin) response = %#v", putResp)
	}
	deleteResp, err := srv.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Id: socialutil.SFUWorkflowID})
	if err != nil {
		t.Fatalf("DeleteWorkflow(builtin) error = %v", err)
	}
	if rejected, ok := deleteResp.(adminhttp.DeleteWorkflow404JSONResponse); !ok || rejected.Error.Code != BuiltinWorkflowCode {
		t.Fatalf("DeleteWorkflow(builtin) response = %#v", deleteResp)
	}
	if _, err := scanWorkflow(srv.DB.QueryRowContext(ctx, `SELECT id,driver,config_json FROM workflows WHERE id=?`, socialutil.SFUWorkflowID)); err != nil {
		t.Fatalf("builtin Workflow after rejected delete: %v", err)
	}
}

func TestServerRejectsInvalidToolkitPolicy(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	toolIDs := []string{""}
	doc := adminhttp.WorkflowUpsert{
		Id: "bad-toolkit",
		Spec: apitypes.WorkflowSpec{
			Driver:  apitypes.WorkflowDriverEino,
			Toolkit: &apitypes.ToolkitPolicy{ToolIds: &toolIDs},
		},
	}

	createResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := createResp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}

	putResp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Id: "bad-toolkit", Body: &doc})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	if _, ok := putResp.(adminhttp.PutWorkflow404JSONResponse); !ok {
		t.Fatalf("PutWorkflow() response = %#v", putResp)
	}
}

func TestServerCreateWorkflowRequiresName(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{"metadata":{},"spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"llm","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerPutRejectsPathNameMismatch(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seed := mustDocument(t, `{"id":"expected-name","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"llm","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)
	createdResponse, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &seed})
	if err != nil {
		t.Fatal(err)
	}
	created := createdResponse.(adminhttp.CreateWorkflow200JSONResponse)
	doc := mustDocument(t, `{"id":"other-name","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"llm","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)

	resp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{
		Id:   created.Id,
		Body: &doc,
	})
	if err != nil {
		t.Fatalf("PutWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow() response = %#v", resp)
	}

	nilCreateResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{})
	if err != nil {
		t.Fatalf("CreateWorkflow(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(nil body) response = %#v", nilCreateResp)
	}

	nilPutResp, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Id: "expected-name"})
	if err != nil {
		t.Fatalf("PutWorkflow(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminhttp.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow(nil body) response = %#v", nilPutResp)
	}
}

func TestServerRejectsNonCanonicalWorkflowName(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{"id":" padded-workflow ","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"llm","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)

	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", resp)
	}
}

func TestServerListWorkflowsPagination(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()

	for _, name := range []string{"alpha001", "beta0001", "gamma001"} {
		doc := mustDocument(t, fmt.Sprintf(`{"id":%q,"spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"llm","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`, name))
		if _, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc}); err != nil {
			t.Fatalf("CreateWorkflow(%q) error = %v", name, err)
		}
	}

	limit := int32(1)
	firstResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{
		Params: adminhttp.ListWorkflowsParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListWorkflows(first page) error = %v", err)
	}
	first, ok := firstResp.(adminhttp.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkflows(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListWorkflows(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{
		Params: adminhttp.ListWorkflowsParams{
			Cursor: &cursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListWorkflows(second page) error = %v", err)
	}
	second, ok := secondResp.(adminhttp.ListWorkflows200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkflows(second page) response = %#v", secondResp)
	}
	if len(second.Items) != 1 {
		t.Fatalf("ListWorkflows(second page) = %#v", second)
	}
}

func TestServerWorkflowConflictAndMissingDelete(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := mustDocument(t, `{"id":"duplicate","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"llm","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)
	if _, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc}); err != nil {
		t.Fatalf("CreateWorkflow(seed) error = %v", err)
	}
	duplicateResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow(duplicate) error = %v", err)
	}
	if _, ok := duplicateResp.(adminhttp.CreateWorkflow409JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(duplicate) response = %#v", duplicateResp)
	}

	deleteResp, err := srv.DeleteWorkflow(ctx, adminhttp.DeleteWorkflowRequestObject{Id: "missing"})
	if err != nil {
		t.Fatalf("DeleteWorkflow(missing) error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkflow404JSONResponse); !ok {
		t.Fatalf("DeleteWorkflow(missing) response = %#v", deleteResp)
	}
}

func TestServerWorkflowStoreNotConfigured(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	ctx := context.Background()
	doc := mustDocument(t, `{"id":"missing-store","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"llm","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)

	listResp, err := srv.ListWorkflows(ctx, adminhttp.ListWorkflowsRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkflows() error = %v", err)
	}
	if _, ok := listResp.(adminhttp.ListWorkflows500JSONResponse); !ok {
		t.Fatalf("ListWorkflows() response = %#v", listResp)
	}
	createResp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow() error = %v", err)
	}
	if _, ok := createResp.(adminhttp.CreateWorkflow500JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResp)
	}
	getResp, err := srv.GetWorkflow(ctx, adminhttp.GetWorkflowRequestObject{Id: "missing-store"})
	if err != nil {
		t.Fatalf("GetWorkflow() error = %v", err)
	}
	if _, ok := getResp.(adminhttp.GetWorkflow500JSONResponse); !ok {
		t.Fatalf("GetWorkflow() response = %#v", getResp)
	}
}

func TestServerRejectsMissingWorkflowRequiredFields(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	for name, doc := range map[string]adminhttp.WorkflowUpsert{
		"name": {
			Spec: apitypes.WorkflowSpec{
				Driver:       apitypes.WorkflowDriverAstTranslate,
				AstTranslate: &apitypes.ASTTranslateWorkflowSpec{},
			},
		},
		"driver": {Id: "bad"},
		"spec":   {Id: "bad"},
	} {
		resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
		if err != nil {
			t.Fatalf("CreateWorkflow(%s) error = %v", name, err)
		}
		if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
			t.Fatalf("CreateWorkflow(%s) response = %#v", name, resp)
		}
	}
}

func TestServerRejectsUnsupportedWorkflowDriver(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	doc := adminhttp.WorkflowUpsert{
		Id:   "bad-version",
		Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriver("example-invalid")},
	}
	resp, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatalf("CreateWorkflow(bad driver) error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkflow400JSONResponse); !ok {
		t.Fatalf("CreateWorkflow(bad driver) response = %#v", resp)
	}
}

func TestWorkflowResponseVisitors(t *testing.T) {
	t.Parallel()

	doc := mustDocument(t, `{"id":"visitor","spec":{"driver":"eino","eino":{"graph":{"name":"assistant","compile":{"node_trigger_mode":"any_predecessor","max_run_steps":16},"state":{"fields":[{"name":"values","type":"object","merge":"replace"},{"name":"channels","type":"object","merge":"replace"},{"name":"answer-messages","type":"messages","merge":"replace"},{"name":"answer-text","type":"string","merge":"replace"}]},"nodes":[{"id":"initialize-conversation","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    channels[\"main\"] = json.decode(json.encode(input[\"history\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"history":{"from":"input.messages"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer-prompt","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    values[\"safety_fence\"] = input[\"fence\"]\n    prompt = ''\n    messages = [{\"role\":\"system\",\"content\":prompt}] if prompt else []\n    for message in channels.get('main', []):\n        content = message.get(\"content\", \"\")\n        if not content:\n            content = \"\".join([part.get(\"text\", \"\") for part in message.get(\"parts\", []) if part.get(\"type\") == \"text\"])\n        messages.append({\"role\":message.get(\"role\", \"user\"),\"content\":content})\n    return {\"values\":values,\"channels\":channels,\"messages\":messages}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"fence":{"from":"input.safety_fence"}},"outputs":{"values":"values","channels":"channels","messages":"answer-messages"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}},{"id":"answer","type":"chat_model","model":"llm","inputs":{"messages":{"from":"answer-messages"}},"outputs":{"text":"answer-text"}},{"id":"answer-capture","type":"script","language":"starlark","entrypoint":"run","source":"def run(input):\n    values = json.decode(json.encode(input[\"values\"]))\n    channels = json.decode(json.encode(input[\"channels\"]))\n    text = input[\"answer\"]\n    channels.setdefault(\"main\", []).append({\"role\":\"assistant\",\"content\":text})\n    return {\"values\":values,\"channels\":channels}\n","inputs":{"values":{"from":"values"},"channels":{"from":"channels"},"answer":{"from":"answer-text"}},"outputs":{"values":"values","channels":"channels"},"limits":{"max_execution_steps":1000000,"timeout":"1s","max_input_bytes":1048576,"max_output_bytes":1048576}}],"edges":[{"from":"start","to":"initialize-conversation"},{"from":"answer-prompt","to":"answer"},{"from":"answer","to":"answer-capture"},{"from":"initialize-conversation","to":"answer-prompt"},{"from":"answer-capture","to":"end"}],"branches":[],"outputs":[{"node":"answer","field":"answer-text","name":"answer","mime_type":"text/plain","primary":true}]},"state_persistence":{"fields":["values","channels"]}}}}`)
	responseDoc := apitypes.Workflow{Id: doc.Id, Spec: doc.Spec}
	cases := map[string]func(*fiber.Ctx) error{
		"create": createWorkflow200Response{doc: responseDoc}.VisitCreateWorkflowResponse,
		"get":    getWorkflow200Response{doc: responseDoc}.VisitGetWorkflowResponse,
		"put":    putWorkflow200Response{doc: responseDoc}.VisitPutWorkflowResponse,
		"delete": deleteWorkflow200Response{doc: responseDoc}.VisitDeleteWorkflowResponse,
	}
	for name, visit := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			app.Get("/", visit)
			resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("content-type = %q, want application/json", got)
			}
		})
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	server := &Server{DB: db}
	if err := server.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return server

}

func mustDocument(t *testing.T, raw string) adminhttp.WorkflowUpsert {
	t.Helper()

	var doc adminhttp.WorkflowUpsert
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return doc
}

func workflowDriver(t *testing.T, doc apitypes.Workflow) string {
	t.Helper()

	return string(doc.Spec.Driver)
}

func mustSingle(t *testing.T, doc apitypes.Workflow) adminhttp.WorkflowUpsert {
	t.Helper()

	return adminhttp.WorkflowUpsert{Id: doc.Id, Spec: doc.Spec}
}
