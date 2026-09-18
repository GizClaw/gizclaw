package gizclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestDeviceSettingsRoutesForwardAndValidate(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	brightness := int64(40)
	stored := rpcapi.DeviceSettings{ScreenBrightness: &brightness}
	var patches []rpcapi.DeviceSettings
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch req.Method {
		case rpcapi.RPCMethodClientDeviceSettingsGet:
			return newRPCResultResponse(req.Id, rpcapi.ClientDeviceSettingsGetResponse{Value: stored}, (*rpcapi.RPCPayload).FromClientDeviceSettingsGetResponse)
		case rpcapi.RPCMethodClientDeviceSettingsSet:
			params, err := req.Params.AsClientDeviceSettingsSetRequest()
			if err != nil {
				return nil, err
			}
			patches = append(patches, params.Value)
			if params.Value.AlertMode != nil {
				stored.AlertMode = params.Value.AlertMode
			}
			return newRPCResultResponse(req.Id, rpcapi.ClientDeviceSettingsSetResponse{Value: stored}, (*rpcapi.RPCPayload).FromClientDeviceSettingsSetResponse)
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unexpected"}.RPCResponse(), nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)

	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/settings", "")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"screen_brightness":40}` {
		t.Fatalf("get settings status = %d body=%s", response.Code, response.Body.String())
	}
	response = f.do(t, http.MethodPatch, "/gizclaw/v1/device/settings", `{"alert_mode":"vibrate","nfc_enabled":false}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"alert_mode":"vibrate"`) {
		t.Fatalf("patch settings status = %d body=%s", response.Code, response.Body.String())
	}
	if len(patches) != 1 || patches[0].NfcEnabled == nil || *patches[0].NfcEnabled || patches[0].ScreenBrightness != nil {
		t.Fatalf("forwarded patches = %+v, want only the members present", patches)
	}
	for _, body := range []string{
		`{"alert_mode":"loud"}`,
		`{"screen_brightness":101}`,
		`{"auto_sleep_timeout_ms":-1}`,
		`{"locale":"zh_CN"}`,
		`{"alert_mode":"ring","led_brightness":-1}`,
	} {
		if response := f.do(t, http.MethodPatch, "/gizclaw/v1/device/settings", body); response.Code != http.StatusBadRequest {
			t.Fatalf("patch %s status = %d body=%s", body, response.Code, response.Body.String())
		}
	}
	if len(patches) != 1 {
		t.Fatalf("invalid patches reached the device: %+v", patches)
	}
}

func TestDeviceFactoryResetMarksTransition(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var got rpcapi.ClientDeviceFactoryResetRequest
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		if req.Method != rpcapi.RPCMethodClientDeviceFactoryReset {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unexpected"}.RPCResponse(), nil
		}
		params, err := req.Params.AsClientDeviceFactoryResetRequest()
		if err != nil {
			return nil, err
		}
		got = params
		return newRPCResultResponse(req.Id, rpcapi.ClientDeviceFactoryResetResponse{}, (*rpcapi.RPCPayload).FromClientDeviceFactoryResetResponse)
	})
	f.manager.SetPeerUp(f.owner, device)
	if response := f.do(t, http.MethodPost, "/gizclaw/v1/device/actions/factory-reset", `{"keep_network":true}`); response.Code != http.StatusNoContent {
		t.Fatalf("factory reset status = %d body=%s", response.Code, response.Body.String())
	}
	if got.KeepNetwork == nil || !*got.KeepNetwork {
		t.Fatalf("factory reset request = %+v", got)
	}
	// The acknowledging connection is resetting, so later commands answer
	// offline instead of reaching a device that is erasing its state.
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/settings", "")
	if response.Code != http.StatusConflict || errorCode(t, response) != deviceOfflineCode {
		t.Fatalf("after reset status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestDeviceRPCMethodsAndRunWorkspace(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	var sets []rpcapi.ClientRunWorkspaceSetRequest
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch req.Method {
		case rpcapi.RPCMethodClientRPCMethodsGet:
			return newRPCResultResponse(req.Id, rpcapi.ClientRPCMethodsGetResponse{Methods: []string{"client.run.workspace.set"}}, (*rpcapi.RPCPayload).FromClientRPCMethodsGetResponse)
		case rpcapi.RPCMethodClientRunWorkspaceSet:
			params, err := req.Params.AsClientRunWorkspaceSetRequest()
			if err != nil {
				return nil, err
			}
			sets = append(sets, params)
			return newRPCResultResponse(req.Id, rpcapi.ClientRunWorkspaceSetResponse{}, (*rpcapi.RPCPayload).FromClientRunWorkspaceSetResponse)
		default:
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unexpected"}.RPCResponse(), nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)

	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/rpc-methods", "")
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"methods":["client.run.workspace.set"]}` {
		t.Fatalf("rpc methods status = %d body=%s", response.Code, response.Body.String())
	}
	if response := f.do(t, http.MethodPut, "/gizclaw/v1/device/run/workspace", `{"collection":"stories","workflow_name":"bedtime","kickoff":true}`); response.Code != http.StatusAccepted {
		t.Fatalf("workspace set status = %d body=%s", response.Code, response.Body.String())
	}
	if len(sets) != 1 || sets[0].WorkflowName == nil || *sets[0].WorkflowName != "bedtime" || sets[0].Kickoff == nil || !*sets[0].Kickoff {
		t.Fatalf("forwarded workspace sets = %+v", sets)
	}
	for _, body := range []string{`{}`, `{"workspace_name":"a","collection":"b","workflow_name":"c"}`, `{"collection":"stories"}`, `{"workspace_name":""}`} {
		if response := f.do(t, http.MethodPut, "/gizclaw/v1/device/run/workspace", body); response.Code != http.StatusBadRequest {
			t.Fatalf("workspace set %s status = %d body=%s", body, response.Code, response.Body.String())
		}
	}
	if len(sets) != 1 {
		t.Fatalf("invalid targets reached the device: %+v", sets)
	}
}

func TestDeviceRuntimeReportsPendingWorkspace(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	if _, err := f.manager.PeerRun.SetRunAgent(context.Background(), f.owner, apitypes.AgentSelection{WorkspaceName: "bedtime"}); err != nil {
		t.Fatal(err)
	}
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/runtime", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"pending_workspace_name":"bedtime"`) ||
		strings.Contains(response.Body.String(), "active_workspace_name") {
		t.Fatalf("runtime status = %d body=%s", response.Code, response.Body.String())
	}
}

type fakeControlProfiles struct {
	tools map[string]apitypes.RuntimeProfileBinding
}

func (p fakeControlProfiles) ResolveOwnerProfile(context.Context, string) (apitypes.RuntimeProfile, error) {
	return apitypes.RuntimeProfile{Id: "p", Revision: "1", Spec: apitypes.RuntimeProfileSpec{
		Resources: apitypes.RuntimeProfileResources{Tools: &p.tools},
	}}, nil
}

type fakeControlTools map[string]toolkit.Tool

func (t fakeControlTools) GetToolByID(_ context.Context, id string) (toolkit.Tool, error) {
	tool, ok := t[id]
	if !ok {
		return toolkit.Tool{}, toolkit.ErrToolNotFound
	}
	return tool, nil
}

func controlToolFixture(t *testing.T) *deviceHTTPFixture {
	t.Helper()
	f := newDeviceHTTPFixture(t)
	var schema jsonschema.Schema
	if err := json.Unmarshal([]byte(`{"type":"object","properties":{"minutes":{"type":"integer","minimum":1}},"required":["minutes"],"additionalProperties":false}`), &schema); err != nil {
		t.Fatal(err)
	}
	owner := apitypes.RuntimeProfileBindingControlAccessOwner
	i18n := map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Usage limit"}, "zh-CN": {DisplayName: "使用时长"}}
	profiles := fakeControlProfiles{tools: map[string]apitypes.RuntimeProfileBinding{
		"usage_limit": {ResourceId: "usage", I18n: i18n, ControlAccess: &owner},
		"ai_only":     {ResourceId: "usage", I18n: i18n},
		"server_http": {ResourceId: "http", I18n: i18n, ControlAccess: &owner},
		"disabled":    {ResourceId: "off", I18n: i18n, ControlAccess: &owner},
	}}
	tools := fakeControlTools{
		"usage": {ID: "usage", InvokeName: "set_usage_limit", Type: toolkit.ToolTypeClientRPC, Enabled: true, InputSchema: schema},
		"http":  {ID: "http", InvokeName: "fetch", Type: toolkit.ToolTypeHTTPRequest, Enabled: true, InputSchema: schema},
		"off":   {ID: "off", InvokeName: "off", Type: toolkit.ToolTypeClientRPC, Enabled: false, InputSchema: schema},
	}
	base := f.public.DeviceReads
	f.public.DeviceReads = func(owner giznet.PublicKey) peerresource.DeviceReads {
		reads := base(owner)
		reads.Profiles = profiles
		reads.Tools = tools
		return reads
	}
	return f
}

func TestDeviceToolsListsOnlyControlExposedClientTools(t *testing.T) {
	f := controlToolFixture(t)
	// Listing reads Server configuration only, so it answers with the device
	// offline.
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/tools", "")
	if response.Code != http.StatusOK {
		t.Fatalf("list tools status = %d body=%s", response.Code, response.Body.String())
	}
	var list struct {
		Items []struct {
			Name          string         `json:"name"`
			ControlAccess string         `json:"control_access"`
			InputSchema   map[string]any `json:"input_schema"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "usage_limit" || list.Items[0].ControlAccess != "owner" || list.Items[0].InputSchema["type"] != "object" {
		t.Fatalf("listed tools = %+v", list.Items)
	}
}

func TestDeviceToolInvokeValidatesAndForwards(t *testing.T) {
	f := controlToolFixture(t)
	var invoked []rpcapi.ToolInvokeRequest
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		if req.Method != rpcapi.RPCMethodClientToolInvoke {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.StatusCodeUnimplemented, Message: "unexpected"}.RPCResponse(), nil
		}
		params, err := req.Params.AsToolInvokeRequest()
		if err != nil {
			return nil, err
		}
		invoked = append(invoked, params)
		return newRPCResultResponse(req.Id, rpcapi.ToolInvokeResponse{DataJson: `{"ok":true}`}, (*rpcapi.RPCPayload).FromToolInvokeResponse)
	})
	f.manager.SetPeerUp(f.owner, device)

	response := f.do(t, http.MethodPost, "/gizclaw/v1/device/tools/usage_limit/actions/invoke", `{"args":{"minutes":30}}`)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"data_json":"{\"ok\":true}"}` {
		t.Fatalf("invoke status = %d body=%s", response.Code, response.Body.String())
	}
	if len(invoked) != 1 || invoked[0].InvokeName != "set_usage_limit" || invoked[0].Args["minutes"] != float64(30) {
		t.Fatalf("forwarded invocations = %+v", invoked)
	}
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/gizclaw/v1/device/tools/usage_limit/actions/invoke", `{"args":{"minutes":0}}`, http.StatusBadRequest},
		{"/gizclaw/v1/device/tools/usage_limit/actions/invoke", `{}`, http.StatusBadRequest},
		{"/gizclaw/v1/device/tools/ai_only/actions/invoke", `{"args":{"minutes":5}}`, http.StatusNotFound},
		{"/gizclaw/v1/device/tools/server_http/actions/invoke", `{"args":{"minutes":5}}`, http.StatusNotFound},
		{"/gizclaw/v1/device/tools/disabled/actions/invoke", `{"args":{"minutes":5}}`, http.StatusNotFound},
		{"/gizclaw/v1/device/tools/missing/actions/invoke", `{"args":{"minutes":5}}`, http.StatusNotFound},
	} {
		if response := f.do(t, http.MethodPost, tc.path, tc.body); response.Code != tc.status {
			t.Fatalf("%s %s status = %d body=%s", tc.path, tc.body, response.Code, response.Body.String())
		}
	}
	if len(invoked) != 1 {
		t.Fatalf("rejected invocations reached the device: %+v", invoked)
	}
}

func TestDeviceToolInvokeRejectsNonJSONDeviceResult(t *testing.T) {
	f := controlToolFixture(t)
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return newRPCResultResponse(req.Id, rpcapi.ToolInvokeResponse{DataJson: "not json"}, (*rpcapi.RPCPayload).FromToolInvokeResponse)
	})
	f.manager.SetPeerUp(f.owner, device)
	response := f.do(t, http.MethodPost, "/gizclaw/v1/device/tools/usage_limit/actions/invoke", `{"args":{"minutes":30}}`)
	if response.Code != http.StatusBadGateway || errorCode(t, response) != deviceErrorCode {
		t.Fatalf("invoke status = %d body=%s", response.Code, response.Body.String())
	}
}
