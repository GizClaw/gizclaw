package runtimeprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func mcpTestProfile() adminhttp.RuntimeProfileUpsert {
	binding := runtimeProfileTestWorkflowBinding("assistant")
	binding.Mcp = new(map[string]apitypes.RuntimeProfileMCPSelection{
		"remote": {Tools: new([]string{"search", "lookup"})},
	})
	return adminhttp.RuntimeProfileUpsert{Id: "profile", Spec: apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{"assistant": binding},
		Resources: apitypes.RuntimeProfileResources{McpServers: new(map[string]apitypes.RuntimeProfileMCPServer{
			"remote": {Endpoint: "https://mcp.example.test/mcp?tenant=example", Headers: new(map[string]string{"authorization": "Bearer private-test-value"})},
		})},
	}}
}

func TestMCPProfileNormalizationAndRevision(t *testing.T) {
	input := mcpTestProfile()
	server := (*input.Spec.Resources.McpServers)["remote"]
	server.Endpoint = " " + server.Endpoint + " "
	input.Spec.Resources.McpServers = new(map[string]apitypes.RuntimeProfileMCPServer{" remote ": server})
	binding := input.Spec.Workflows["assistant"]
	binding.Mcp = new(map[string]apitypes.RuntimeProfileMCPSelection{" remote ": {Tools: new([]string{"search", "lookup"})}})
	input.Spec.Workflows["assistant"] = binding
	got, err := normalizeProfile(input, "")
	if err != nil {
		t.Fatal(err)
	}
	remote := (*got.Spec.Resources.McpServers)["remote"]
	if remote.Endpoint != strings.TrimSpace(server.Endpoint) || remote.RefreshInterval == nil || *remote.RefreshInterval != "5m0s" {
		t.Fatalf("normalized server = %#v", remote)
	}
	if !reflect.DeepEqual(*remote.Headers, map[string]string{"Authorization": "Bearer private-test-value"}) {
		t.Fatalf("headers were not canonicalized: %#v", remote.Headers)
	}
	selection := (*got.Spec.Workflows["assistant"].Mcp)["remote"]
	if selection.Resources == nil || *selection.Resources || selection.Tools == nil || !reflect.DeepEqual(*selection.Tools, []string{"lookup", "search"}) {
		t.Fatalf("normalized permissions = %#v", selection)
	}
	if _, exists := (*input.Spec.Resources.McpServers)["remote"]; exists || (*server.Headers)["authorization"] == "" || (*binding.Mcp)[" remote "].Resources != nil {
		t.Fatal("normalization mutated caller-owned maps")
	}
	equivalent := mcpTestProfile()
	remote = (*equivalent.Spec.Resources.McpServers)["remote"]
	remote.RefreshInterval = new(" 300s ")
	(*equivalent.Spec.Resources.McpServers)["remote"] = remote
	binding = equivalent.Spec.Workflows["assistant"]
	binding.Mcp = new(map[string]apitypes.RuntimeProfileMCPSelection{"remote": {Resources: new(false), Tools: new([]string{"lookup", "search"})}})
	equivalent.Spec.Workflows["assistant"] = binding
	other, err := normalizeProfile(equivalent, "")
	if err != nil || other.Revision != got.Revision {
		t.Fatalf("equivalent configuration revisions differ: %v", err)
	}
	again, err := normalizeProfile(adminhttp.RuntimeProfileUpsert{Id: got.Id, Spec: got.Spec}, "")
	if err != nil || again.Revision != got.Revision {
		t.Fatalf("normalization is not idempotent: %v", err)
	}
}

func TestMCPServerValidation(t *testing.T) {
	for _, endpoint := range []string{"", "/mcp", "ftp://example.test/mcp", "https:///mcp", "https://user:private@example.test/mcp", "https://example.test/mcp#fragment", "https://example.test/%zz"} {
		t.Run("endpoint/"+endpoint, func(t *testing.T) {
			if _, err := normalizeMCPServer(apitypes.RuntimeProfileMCPServer{Endpoint: endpoint}); err == nil {
				t.Fatal("invalid endpoint accepted")
			}
		})
	}
	for _, headers := range []map[string]string{
		{"Host": "example.test"}, {"aCcEpT": "application/json"}, {"CONTENT-TYPE": "application/json"},
		{"mCp-SeSsIoN-Id": "private-session"}, {"MCP-Protocol-Version": "2025-11-25"}, {"MCP-Custom": "value"},
		{"bad name": "value"}, {"": "value"}, {"X-Test": "private-test-value\r\nInjected: true"}, {"X-Test": "bad\x00value"},
		{"Authorization": "one", "authorization": "two"},
	} {
		if _, err := normalizeMCPServer(apitypes.RuntimeProfileMCPServer{Endpoint: "http://localhost/mcp", Headers: &headers}); err == nil {
			t.Fatalf("invalid headers accepted: %#v", headers)
		} else if strings.Contains(err.Error(), "private-test-value") {
			t.Fatal("header value leaked in validation error")
		}
	}
	for _, interval := range []string{"", "invalid", "0", "-1s", "999ms", "24h1ns", "999999999999h"} {
		if _, err := normalizeMCPServer(apitypes.RuntimeProfileMCPServer{Endpoint: "http://localhost/mcp", RefreshInterval: &interval}); err == nil {
			t.Fatalf("invalid interval %q accepted", interval)
		}
	}
	for _, interval := range []string{"1s", "24h"} {
		if _, err := normalizeMCPServer(apitypes.RuntimeProfileMCPServer{Endpoint: "http://localhost/mcp", RefreshInterval: &interval}); err != nil {
			t.Fatalf("boundary interval %q rejected: %v", interval, err)
		}
	}
}

func TestMCPSelectionsRejectInvalidPermissions(t *testing.T) {
	servers := map[string]apitypes.RuntimeProfileMCPServer{"remote": {Endpoint: "https://example.test/mcp"}}
	for _, tools := range [][]string{{""}, {" \t\n"}, {"　"}, {strings.Repeat("x", 129)}, {strings.Repeat("你", 43)}, {string([]byte{0xff})}, {"search", "search"}} {
		selection := map[string]apitypes.RuntimeProfileMCPSelection{"remote": {Tools: &tools}}
		if _, err := normalizeMCPSelections(&selection, servers); err == nil {
			t.Fatalf("invalid Tool names accepted: %q", tools)
		}
	}
	for _, selection := range []map[string]apitypes.RuntimeProfileMCPSelection{
		{"remote": {}}, {"remote": {Resources: new(false), Tools: new([]string{})}},
		{"missing": {Resources: new(true)}}, {"Bad_Alias": {Resources: new(true)}},
		{"remote": {Resources: new(true)}, " remote ": {Resources: new(true)}},
	} {
		if _, err := normalizeMCPSelections(&selection, servers); err == nil {
			t.Fatalf("invalid permissions accepted: %#v", selection)
		}
	}
	for _, tools := range [][]string{{strings.Repeat("x", 128)}, {strings.Repeat("你", 42)}, {"Namespace.remote/tool:v1"}, {" explicit name "}} {
		selection := map[string]apitypes.RuntimeProfileMCPSelection{"remote": {Tools: &tools}}
		got, err := normalizeMCPSelections(&selection, servers)
		if err != nil || !reflect.DeepEqual(*(*got)["remote"].Tools, tools) {
			t.Fatalf("explicit Tool name changed: %q, %v", tools, err)
		}
	}
	resources := map[string]apitypes.RuntimeProfileMCPSelection{"remote": {Resources: new(true)}}
	if _, err := normalizeMCPSelections(&resources, servers); err != nil {
		t.Fatalf("resources-only permissions rejected: %v", err)
	}
}

func TestMCPProfileAliasesAndReferences(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*apitypes.RuntimeProfileSpec)
	}{
		{"missing servers", func(spec *apitypes.RuntimeProfileSpec) { spec.Resources.McpServers = nil }},
		{"invalid server alias", func(spec *apitypes.RuntimeProfileSpec) {
			spec.Resources.McpServers = new(map[string]apitypes.RuntimeProfileMCPServer{"Bad_Alias": {Endpoint: "https://example.test"}})
		}},
		{"duplicate server alias", func(spec *apitypes.RuntimeProfileSpec) {
			(*spec.Resources.McpServers)[" remote "] = (*spec.Resources.McpServers)["remote"]
		}},
		{"model collision", func(spec *apitypes.RuntimeProfileSpec) {
			spec.Resources.Models = new(map[string]apitypes.RuntimeProfileBinding{"remote": runtimeProfileTestBinding("model")})
		}},
		{"voice collision", func(spec *apitypes.RuntimeProfileSpec) {
			spec.Resources.Voices = new(map[string]apitypes.RuntimeProfileBinding{"remote": runtimeProfileTestBinding("voice")})
		}},
		{"workflow collision", func(spec *apitypes.RuntimeProfileSpec) {
			spec.Workflows["remote"] = runtimeProfileTestWorkflowBinding("other")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := mcpTestProfile()
			test.mutate(&input.Spec)
			if _, err := normalizeProfile(input, ""); err == nil {
				t.Fatal("invalid MCP configuration accepted")
			}
		})
	}
}

func TestMCPProfileCanonicalWorkflowAgreement(t *testing.T) {
	for _, test := range []struct {
		name      string
		selection *map[string]apitypes.RuntimeProfileMCPSelection
		conflict  bool
	}{
		{"normalized order and defaults", new(map[string]apitypes.RuntimeProfileMCPSelection{"remote": {Resources: new(false), Tools: new([]string{"lookup", "search"})}}), false},
		{"different resources", new(map[string]apitypes.RuntimeProfileMCPSelection{"remote": {Resources: new(true), Tools: new([]string{"lookup", "search"})}}), true},
		{"different Tools", new(map[string]apitypes.RuntimeProfileMCPSelection{"remote": {Tools: new([]string{"lookup"})}}), true},
		{"omitted", nil, true},
		{"empty", new(map[string]apitypes.RuntimeProfileMCPSelection{}), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := mcpTestProfile()
			binding := runtimeProfileTestWorkflowBinding("assistant")
			binding.Mcp = test.selection
			input.Spec.Workflows["another"] = binding
			_, err := normalizeProfile(input, "")
			if (err != nil) != test.conflict || err != nil && !strings.Contains(err.Error(), "conflicts with") {
				t.Fatalf("agreement error = %v, conflict = %v", err, test.conflict)
			}
		})
	}
	input := mcpTestProfile()
	input.Spec.Workflows["different-workflow"] = runtimeProfileTestWorkflowBinding("different-resource")
	if _, err := normalizeProfile(input, ""); err != nil {
		t.Fatalf("distinct canonical Workflows required matching permissions: %v", err)
	}
	input.Spec.Workflows = apitypes.RuntimeProfileWorkflows{"first": runtimeProfileTestWorkflowBinding("same"), "second": runtimeProfileTestWorkflowBinding("same")}
	second := input.Spec.Workflows["second"]
	second.Mcp = new(map[string]apitypes.RuntimeProfileMCPSelection{})
	input.Spec.Workflows["second"] = second
	if _, err := normalizeProfile(input, ""); err != nil {
		t.Fatalf("omitted and empty selections conflict: %v", err)
	}
}

func TestMCPProfileRequiresEinoAndDoesNotConnect(t *testing.T) {
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer endpoint.Close()
	for _, driver := range []apitypes.WorkflowDriver{
		apitypes.WorkflowDriverEino, apitypes.WorkflowDriverAstTranslate, apitypes.WorkflowDriverDoubaoRealtime,
		apitypes.WorkflowDriverDashscopeRealtime, apitypes.WorkflowDriverDoubaoRealtimeDuplex, apitypes.WorkflowDriverSfu,
	} {
		t.Run(string(driver), func(t *testing.T) {
			input := mcpTestProfile()
			(*input.Spec.Resources.McpServers)["remote"] = apitypes.RuntimeProfileMCPServer{Endpoint: endpoint.URL}
			profile, err := normalizeProfile(input, "")
			if err != nil {
				t.Fatal(err)
			}
			server := &Server{ResolveResource: func(_ context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
				if kind != apitypes.ResourceKindWorkflow || id != "assistant" {
					return apitypes.Resource{}, fmt.Errorf("unexpected resource %s/%s", kind, id)
				}
				spec := apitypes.WorkflowSpec{Driver: driver}
				switch driver {
				case apitypes.WorkflowDriverEino:
					spec.Eino = &apitypes.EinoWorkflowSpec{}
				case apitypes.WorkflowDriverAstTranslate:
					spec.AstTranslate = &apitypes.ASTTranslateWorkflowSpec{}
				case apitypes.WorkflowDriverDoubaoRealtime:
					spec.DoubaoRealtime = &apitypes.DoubaoRealtimeWorkflowSpec{}
				case apitypes.WorkflowDriverDashscopeRealtime:
					spec.DashscopeRealtime = &apitypes.DashScopeRealtimeWorkflowSpec{}
				case apitypes.WorkflowDriverDoubaoRealtimeDuplex:
					spec.DoubaoRealtimeDuplex = &apitypes.DoubaoRealtimeDuplexWorkflowSpec{}
				case apitypes.WorkflowDriverSfu:
					spec.Sfu = new(apitypes.SFUWorkflowSpec{})
				}
				var resource apitypes.Resource
				err := resource.FromWorkflowResource(apitypes.WorkflowResource{
					ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1, Kind: apitypes.WorkflowResourceKindWorkflow,
					Metadata: apitypes.ResourceMetadata{Id: id}, Spec: spec,
				})
				return resource, err
			}}
			err = server.validateResources(t.Context(), profile.Spec)
			if driver == apitypes.WorkflowDriverEino && err != nil || driver != apitypes.WorkflowDriverEino && (err == nil || !strings.Contains(err.Error(), "requires an Eino Workflow")) {
				t.Fatalf("driver %s validation = %v", driver, err)
			}
		})
	}
	if err := (&Server{}).validateResources(t.Context(), mcpTestProfile().Spec); err == nil {
		t.Fatal("MCP permissions accepted without resolving Workflow driver")
	}
	if requests.Load() != 0 {
		t.Fatalf("validation made %d network requests", requests.Load())
	}
}

func TestMCPProfileSQLRoundTripAndRevision(t *testing.T) {
	input := mcpTestProfile()
	profile, err := normalizeProfile(input, "")
	if err != nil {
		t.Fatal(err)
	}
	db := profileSQLTestDB(t)
	if created, err := insertRuntimeProfileSQL(t.Context(), db, profile); err != nil || !created {
		t.Fatalf("insert = %v, %v", created, err)
	}
	stored, version, err := getRuntimeProfileSQL(t.Context(), db, profile.Id)
	if err != nil || !reflect.DeepEqual(stored.Spec, profile.Spec) || stored.Revision != profile.Revision {
		t.Fatalf("MCP configuration did not survive SQL round trip: %v", err)
	}
	for _, mutate := range []func(*adminhttp.RuntimeProfileUpsert){
		func(input *adminhttp.RuntimeProfileUpsert) {
			server := (*input.Spec.Resources.McpServers)["remote"]
			server.Endpoint = "https://other.example.test/mcp"
			(*input.Spec.Resources.McpServers)["remote"] = server
		},
		func(input *adminhttp.RuntimeProfileUpsert) {
			(*(*input.Spec.Resources.McpServers)["remote"].Headers)["authorization"] = "Bearer changed"
		},
		func(input *adminhttp.RuntimeProfileUpsert) {
			server := (*input.Spec.Resources.McpServers)["remote"]
			server.RefreshInterval = new("1m")
			(*input.Spec.Resources.McpServers)["remote"] = server
		},
		func(input *adminhttp.RuntimeProfileUpsert) {
			(*input.Spec.Workflows["assistant"].Mcp)["remote"] = apitypes.RuntimeProfileMCPSelection{Resources: new(true)}
		},
	} {
		changed := mcpTestProfile()
		mutate(&changed)
		next, err := normalizeProfile(changed, "")
		if err != nil || next.Revision == profile.Revision {
			t.Fatalf("MCP change missing from revision: %v", err)
		}
	}
	input.Spec.Resources.McpServers = nil
	input.Spec.Workflows = nil
	cleared, err := normalizeProfile(input, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := updateRuntimeProfileSQL(t.Context(), db, cleared, version); err != nil {
		t.Fatal(err)
	}
	stored, _, err = getRuntimeProfileSQL(t.Context(), db, profile.Id)
	if err != nil || stored.Spec.Resources.McpServers != nil || len(stored.Spec.Workflows) != 0 || stored.Revision != cleared.Revision {
		t.Fatalf("MCP configuration was not cleared: %v", err)
	}
	var raw string
	if err := db.GetContext(t.Context(), &raw, "SELECT resources_json FROM runtime_profiles WHERE id=?", profile.Id); err != nil {
		t.Fatal(err)
	}
	var resources map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &resources); err != nil {
		t.Fatal(err)
	}
	if _, exists := resources["mcp_servers"]; exists {
		t.Fatal("cleared MCP servers remain in resources_json")
	}
}
