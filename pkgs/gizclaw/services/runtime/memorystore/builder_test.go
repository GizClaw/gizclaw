package memorystore

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestBuildRejectsMismatchedCanonicalLayoutID(t *testing.T) {
	request := supportedFlowcraftTestRequest(t)
	request.Layout.Id = "different-layout-id"

	_, err := Build(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "different-layout-id" does not match binding layout_id "layout-id"`) {
		t.Fatalf("Build() error = %v", err)
	}
}

func TestBuildRejectsEmptyCanonicalLayoutID(t *testing.T) {
	request := supportedFlowcraftTestRequest(t)
	request.Layout.Id = ""

	_, err := Build(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), `layout id "" does not match binding layout_id "layout-id"`) {
		t.Fatalf("Build() error = %v", err)
	}
}

func TestBuildVolcMem0UsesVolcProtocol(t *testing.T) {
	requestPath := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestPath <- request.Method + " " + request.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"event_id":"job"}]}`)
	}))
	t.Cleanup(server.Close)

	connection := apitypes.RuntimeProfileMemoryConnection{}
	if err := connection.FromRuntimeProfileVolcMem0Connection(apitypes.RuntimeProfileVolcMem0Connection{
		ApiKey:          "key",
		Endpoint:        server.URL,
		MemoryProjectId: "project",
		Type:            apitypes.RuntimeProfileVolcMem0ConnectionTypeVolcMem0,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := Build(t.Context(), Request{
		WorkspaceID: "workspace",
		BindingName: "memory",
		Layout:      apitypes.MemoryLayout{Id: "layout-id"},
		Binding: apitypes.RuntimeProfileMemoryBinding{
			LayoutId:   "layout-id",
			Driver:     apitypes.RuntimeProfileMemoryDriverVolcMem0,
			Connection: connection,
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	observed, err := result.Store.Observe(t.Context(), memory.Observation{
		Scope: memory.Scope{UserID: "user"},
		Text:  "remember this",
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if observed.Operation == nil {
		t.Fatal("Observe() returned no Volc operation")
	}
	if got := <-requestPath; got != "POST /v1/memories/" {
		t.Fatalf("Volc request = %q, want POST /v1/memories/", got)
	}
}

func TestProjectionSignatureExcludesExtractionAndWritePolicy(t *testing.T) {
	policy := testFlowcraftPolicy()
	before, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.Extraction.SystemPrompt = new("changed")
	policy.Write.Mode = apitypes.FlowcraftMemoryWritePolicyModeAsyncSemantic
	after, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("extraction/write policy changed derived-index identity")
	}
	policy.GraphEnabled = new(false)
	changed, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	if before == changed {
		t.Fatal("graph policy did not change derived-index identity")
	}
}

func TestFlowcraftConfigIncludesLaneExtractionInstructions(t *testing.T) {
	policy := testFlowcraftPolicy()
	policy.Lanes[0].Description = new("Durable story facts.")
	policy.Lanes[0].Extract = new("Capture only facts already narrated.")
	policy.Lanes[0].Recall = new("Use only after the Graph selects this lane.")
	config, err := flowcraftConfig(policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config.Extraction.SystemPrompt, "Extract: Capture only facts already narrated.") {
		t.Fatalf("extraction prompt = %q", config.Extraction.SystemPrompt)
	}
	if strings.Contains(config.Extraction.SystemPrompt, "Use only after the Graph") {
		t.Fatalf("recall guidance leaked into extraction prompt = %q", config.Extraction.SystemPrompt)
	}
}

func TestFlowcraftConfigCanDisableModelExtraction(t *testing.T) {
	policy := testFlowcraftPolicy()
	policy.Extraction.Model = "extraction"
	policy.Extraction.Enabled = new(false)

	config, err := flowcraftConfig(policy, nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.Extraction.Model != "" {
		t.Fatalf("extraction model = %q, want disabled", config.Extraction.Model)
	}
	if len(config.LaneNames) == 0 {
		t.Fatal("disabling model extraction removed direct-Fact lane policy")
	}
}

func testFlowcraftPolicy() apitypes.FlowcraftMemoryLayoutPolicy {
	return apitypes.FlowcraftMemoryLayoutPolicy{
		Extraction: apitypes.FlowcraftMemoryExtractionPolicy{
			Mode: apitypes.FlowcraftMemoryExtractionPolicyModeTwoPass,
		},
		Lanes: []apitypes.FlowcraftMemoryLanePolicy{{
			Name: "facts",
			Kind: apitypes.FlowcraftMemoryLanePolicyKindNote,
		}},
		Write: apitypes.FlowcraftMemoryWritePolicy{
			Mode: apitypes.FlowcraftMemoryWritePolicyModeSync,
			Tier: apitypes.FlowcraftMemoryWritePolicyTierGeneral,
		},
	}
}
