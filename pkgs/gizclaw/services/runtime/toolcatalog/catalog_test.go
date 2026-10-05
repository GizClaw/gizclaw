package toolcatalog

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/google/jsonschema-go/jsonschema"
)

func toolProfile() apitypes.RuntimeProfile {
	return apitypes.RuntimeProfile{Id: "profile", Spec: apitypes.RuntimeProfileSpec{
		Mhs: &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "display.main", Hwd: apitypes.MhsV0DeviceHwdDisplay}}}},
		Resources: apitypes.RuntimeProfileResources{Tools: &map[string]apitypes.RuntimeProfileToolBinding{
			"screen.brightness": {Mhs: &apitypes.RuntimeProfileMhsTool{Id: "display.main", Operation: apitypes.RuntimeProfileMhsToolOperationWrite, Fields: &[]string{"brightness_percent"}}},
			"music-play":        {ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.play"}},
		}},
		Workflows: apitypes.RuntimeProfileWorkflows{"assistant": {ResourceId: "workflow", Toolkit: &apitypes.RuntimeProfileToolSelection{ToolNames: &[]string{"screen.brightness", "music-play"}}}},
	}}
}

func TestModelNameMappingIsExplicitAndInjective(t *testing.T) {
	for alias, want := range map[string]string{"screen.brightness": "screen_brightness", "screen-brightness": "screen-brightness", "web-search": "web-search"} {
		got, err := FunctionName(alias)
		if err != nil || got != want {
			t.Fatalf("%s => %s, %v", alias, got, err)
		}
	}
	for _, alias := range []string{"screen_brightness", "UPPER", "screen..brightness", "screen.-brightness"} {
		if _, err := FunctionName(alias); err == nil {
			t.Fatalf("accepted invalid alias %q", alias)
		}
	}
}

func TestInnerSchemasFixTargetAndBoundFields(t *testing.T) {
	profile := toolProfile()
	for alias, binding := range *profile.Spec.Resources.Tools {
		source, target, schema, err := BindingSchema(profile, binding)
		if err != nil {
			t.Fatalf("%s: %v", alias, err)
		}
		tool := Tool{Schema: schema}
		if source == "mhs" {
			if target["id"] != "display.main" || target["hwd"] != "display" {
				t.Fatalf("wrong target: %v", target)
			}
			if err := ValidateArguments(tool, json.RawMessage(`{"brightness_percent":30}`)); err != nil {
				t.Fatal(err)
			}
			for _, args := range []string{`{}`, `{"brightness_percent":101}`, `{"brightness_percent":-1}`, `{"brightness_percent":1.5}`, `{"brightness_percent":30,"enabled":true}`, `{"brightness_percent":30,"id":"display.other"}`, `null`, `{"brightness_percent":30,"brightness_percent":40}`, `{"brightness_percent":30} {}`} {
				if err := ValidateArguments(tool, json.RawMessage(args)); err == nil {
					t.Fatalf("accepted %s", args)
				}
			}
		} else {
			if target["name"] != "audioplayer.play" {
				t.Fatalf("wrong procedure: %v", target)
			}
			if err := ValidateArguments(tool, json.RawMessage(`{"tool":"device.reboot"}`)); err == nil {
				t.Fatal("model can change procedure")
			}
		}
	}
}

func TestRejectMixedSourcesAndUndeclaredHardware(t *testing.T) {
	profile := toolProfile()
	for _, binding := range []apitypes.RuntimeProfileToolBinding{
		{},
		{ResourceId: "http", ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.play"}},
		{ClientTool: &apitypes.RuntimeProfileClientTool{Name: "undefined.play"}},
		{Mhs: &apitypes.RuntimeProfileMhsTool{Id: "display.other", Operation: apitypes.RuntimeProfileMhsToolOperationWrite, Fields: &[]string{"brightness_percent"}}},
		{Mhs: &apitypes.RuntimeProfileMhsTool{Id: "display.main", Operation: apitypes.RuntimeProfileMhsToolOperationWrite, Fields: &[]string{"invented"}}},
		{Mhs: &apitypes.RuntimeProfileMhsTool{Id: "display.main", Operation: apitypes.RuntimeProfileMhsToolOperationWrite}},
	} {
		if _, _, _, err := BindingSchema(profile, binding); err == nil {
			t.Fatalf("accepted %+v", binding)
		}
	}
}

func TestWorkflowSelectionAndWorkspaceNarrowing(t *testing.T) {
	profile := toolProfile()
	for _, test := range []struct {
		policy *apitypes.ToolkitPolicy
		want   []string
	}{
		{nil, []string{"music-play", "screen.brightness"}},
		{&apitypes.ToolkitPolicy{ToolNames: &[]string{}}, []string{}},
		{&apitypes.ToolkitPolicy{ToolNames: &[]string{"screen.brightness", "unbound"}}, []string{"screen.brightness"}},
		{&apitypes.ToolkitPolicy{ToolIds: &[]string{"legacy-resource"}}, []string{}},
	} {
		got, err := Selection(profile, "assistant", "workflow", test.policy)
		if err != nil || !slices.Equal(got, test.want) {
			t.Fatalf("selection = %v, %v; want %v", got, err, test.want)
		}
	}
	binding := profile.Spec.Workflows["assistant"]
	binding.Toolkit = nil
	profile.Spec.Workflows["assistant"] = binding
	if got, err := Selection(profile, "assistant", "workflow", nil); err != nil || len(got) != 0 {
		t.Fatalf("omitted selection grants %v, %v", got, err)
	}
	if _, err := Selection(profile, "assistant", "foreign-workflow", nil); err == nil {
		t.Fatal("foreign workflow accepted")
	}
}

func TestHTTPAliasRebindDisableAndDeletion(t *testing.T) {
	server := toolkittest.New(t)
	for _, id := range []string{"first", "second"} {
		_, err := server.CreateTool(t.Context(), toolkit.Tool{ID: id, InvokeName: "private_" + id, Type: toolkit.ToolTypeHTTPRequest, Enabled: true, InputSchema: jsonschema.Schema{Type: "object"}, HTTP: &toolkit.HTTPRequest{URL: "https://example.com/" + id, Method: "GET", Auth: toolkit.HTTPAuth{Method: "none"}, Timeout: time.Second, MaxResponseBytes: 1024}})
		if err != nil {
			t.Fatal(err)
		}
	}
	profile := toolProfile()
	profile.Spec.Resources.Tools = &map[string]apitypes.RuntimeProfileToolBinding{"search": {ResourceId: "first"}}
	catalog := Catalog{Tools: server}
	first, err := catalog.Resolve(t.Context(), "owner", profile, nil)
	if err != nil || len(first) != 1 || first[0].FunctionName != "search" || !first[0].Available || first[0].HTTP.ID != "first" {
		t.Fatalf("first = %+v, %v", first, err)
	}
	(*profile.Spec.Resources.Tools)["search"] = apitypes.RuntimeProfileToolBinding{ResourceId: "second"}
	second, err := catalog.Resolve(t.Context(), "owner", profile, nil)
	if err != nil || second[0].HTTP.ID != "second" || second[0].FunctionName != "search" {
		t.Fatalf("rebind = %+v, %v", second, err)
	}
	if err := server.DeleteTool(t.Context(), "second"); err != nil {
		t.Fatal(err)
	}
	deleted, err := catalog.Resolve(t.Context(), "owner", profile, nil)
	if err != nil || len(deleted) != 1 || deleted[0].Available || deleted[0].Reason != "resource_missing" {
		t.Fatalf("deleted = %+v, %v", deleted, err)
	}
}

func TestNestedDuplicateArgumentsAreRejected(t *testing.T) {
	tool := Tool{Schema: jsonschema.Schema{Type: "object"}}
	for _, args := range []string{`{"a":{"x":1,"x":2}}`, `{"a":[{"x":1,"x":2}]}`} {
		if err := ValidateArguments(tool, json.RawMessage(args)); err == nil {
			t.Fatalf("accepted duplicate object: %s", args)
		}
	}
}

func TestProgramSelectionRequiresProfileWorkflowAlias(t *testing.T) {
	profile := toolProfile()
	_, _, schema, err := BindingSchema(profile, apitypes.RuntimeProfileToolBinding{ClientTool: &apitypes.RuntimeProfileClientTool{Name: "run.workspace.set"}})
	if err != nil {
		t.Fatal(err)
	}
	tool := Tool{Schema: schema}
	if err := ValidateArguments(tool, json.RawMessage(`{"workflow_name":"assistant"}`)); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{}`, `{"workspace_name":"foreign"}`, `{"workflow_name":"foreign"}`, `{"workflow_name":"assistant","workspace_name":"foreign"}`} {
		if err := ValidateArguments(tool, json.RawMessage(args)); err == nil {
			t.Fatalf("accepted unbound target: %s", args)
		}
	}
}

func TestDefaultAudioPlaybackAllowsOmittedIndex(t *testing.T) {
	_, _, schema, err := BindingSchema(toolProfile(), apitypes.RuntimeProfileToolBinding{ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.play"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateArguments(Tool{Schema: schema}, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func TestMissingResourceCatalogRemainsEncodable(t *testing.T) {
	profile := toolProfile()
	profile.Spec.Resources.Tools = &map[string]apitypes.RuntimeProfileToolBinding{"lookup": {ResourceId: "missing"}}
	catalog := Catalog{Tools: toolkittest.New(t)}
	tools, err := catalog.Resolve(t.Context(), "owner", profile, nil)
	if err != nil || len(tools) != 1 {
		t.Fatalf("catalog=%v error=%v", tools, err)
	}
	var payload rpcapi.RPCPayload
	if err := payload.FromToolGetResponse(rpcapi.ToolGetResponse{Value: rpcapi.Tool{Name: tools[0].Alias, InputSchema: tools[0].Schema, UnavailableReason: tools[0].Reason}}); err != nil {
		t.Fatal(err)
	}
	decoded, err := payload.AsToolGetResponse()
	if err != nil || decoded.Value.UnavailableReason != "resource_missing" {
		t.Fatalf("decoded=%v error=%v", decoded, err)
	}
}

func TestWorkflowAliasRecoveryIsUnique(t *testing.T) {
	profile := toolProfile()
	if names, err := Selection(profile, "", "workflow", nil); err != nil || len(names) != 2 {
		t.Fatalf("names=%v error=%v", names, err)
	}
	profile.Spec.Workflows["other"] = profile.Spec.Workflows["assistant"]
	if _, err := Selection(profile, "", "workflow", nil); err == nil {
		t.Fatal("ambiguous alias was silently chosen")
	}
}
