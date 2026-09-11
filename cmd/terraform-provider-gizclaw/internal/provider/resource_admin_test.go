package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestModelEnvelope(t *testing.T) {
	model := adminResourceModel{
		APIVersion: types.StringValue(resourceAPIVersion),
		Kind:       types.StringValue("Model"),
		ResourceID: types.StringValue("example-model"),
		Spec:       types.StringValue(`{"kind":"llm","source":"manual"}`),
	}
	encoded, err := modelEnvelope(model)
	if err != nil {
		t.Fatal(err)
	}
	var got resourceEnvelope
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.APIVersion != resourceAPIVersion || got.Kind != "Model" || got.Metadata.ID != "example-model" {
		t.Fatalf("unexpected envelope: %+v", got)
	}
}

func TestModelEnvelopeRejectsNonObjectSpec(t *testing.T) {
	_, err := modelEnvelope(adminResourceModel{
		Kind: types.StringValue("Model"), ResourceID: types.StringValue("example-model"), Spec: types.StringValue(`[]`),
	})
	if err == nil {
		t.Fatal("expected non-object spec error")
	}
}

func TestModelEnvelopeRejectsInvalidResourceID(t *testing.T) {
	_, err := modelEnvelope(adminResourceModel{
		Kind: types.StringValue("Model"), ResourceID: types.StringValue(" padded-id "), Spec: types.StringValue(`{}`),
	})
	if err == nil || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("error = %v, want exact resource ID validation", err)
	}
}

func TestParseAdminResourceImportIDPreservesOpaqueID(t *testing.T) {
	kind, resourceID, err := parseAdminResourceImportID("Model/folder/resource%25:id")
	if err != nil {
		t.Fatal(err)
	}
	if kind != "Model" || resourceID != "folder/resource%25:id" {
		t.Fatalf("kind/resourceID = %q/%q", kind, resourceID)
	}
	for _, value := range []string{"Model", "/resource-id", "Model/ padded-id", "Model/.", "Unknown/id", "ResourceList/id"} {
		if _, _, err := parseAdminResourceImportID(value); err == nil {
			t.Fatalf("parseAdminResourceImportID(%q) succeeded", value)
		}
	}
}

func TestUpgradeStateV0PreservesResourceIdentity(t *testing.T) {
	ctx := context.Background()
	resourceUnderTest := &adminResource{}
	upgrader, ok := resourceUnderTest.UpgradeState(ctx)[0]
	if !ok || upgrader.PriorSchema == nil {
		t.Fatal("schema version 0 state upgrader is missing")
	}

	prior := tfsdk.State{Schema: upgrader.PriorSchema}
	diagnostics := prior.Set(ctx, &adminResourceModelV0{
		ID:         types.StringValue("Model/example-model"),
		APIVersion: types.StringValue(resourceAPIVersion),
		Kind:       types.StringValue("Model"),
		Name:       types.StringValue("example-model"),
		Spec:       types.StringValue(`{"kind":"llm","source":"manual"}`),
	})
	if diagnostics.HasError() {
		t.Fatalf("set prior state: %v", diagnostics)
	}

	var currentSchema resource.SchemaResponse
	resourceUnderTest.Schema(ctx, resource.SchemaRequest{}, &currentSchema)
	response := resource.UpgradeStateResponse{State: tfsdk.State{Schema: currentSchema.Schema}}
	upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{State: &prior}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("upgrade state: %v", response.Diagnostics)
	}

	var got adminResourceModel
	diagnostics = response.State.Get(ctx, &got)
	if diagnostics.HasError() {
		t.Fatalf("read upgraded state: %v", diagnostics)
	}
	if got.ID.ValueString() != "Model/example-model" ||
		got.APIVersion.ValueString() != resourceAPIVersion ||
		got.Kind.ValueString() != "Model" ||
		got.ResourceID.ValueString() != "example-model" ||
		got.Spec.ValueString() != `{"kind":"llm","source":"manual"}` {
		t.Fatalf("unexpected upgraded state: %#v", got)
	}
}

func TestValidateStoredIdentity(t *testing.T) {
	matching := resourceEnvelope{Kind: "Model", Metadata: resourceMetadata{ID: "example-model"}}
	if err := validateStoredIdentity("Model", "example-model", matching); err != nil {
		t.Fatal(err)
	}
	for _, stored := range []resourceEnvelope{
		{Kind: "Voice", Metadata: resourceMetadata{ID: "example-model"}},
		{Kind: "Model", Metadata: resourceMetadata{ID: "different-model"}},
	} {
		if err := validateStoredIdentity("Model", "example-model", stored); err == nil {
			t.Fatalf("mismatched response %#v succeeded", stored)
		}
	}
}

func TestSetModelPreservesConfiguredSpec(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(`{"api_key":"${API_KEY}"}`)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "Credential",
		Metadata:   resourceMetadata{ID: "example-credential"},
		Spec:       json.RawMessage(`{"api_key":"resolved-secret"}`),
	}, false)
	if got := model.Spec.ValueString(); got != `{"api_key":"${API_KEY}"}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelRecordsObservedSpec(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(`{"kind":"old"}`)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "Model",
		Metadata:   resourceMetadata{ID: "example-model"},
		Spec:       json.RawMessage(`{"kind":"new"}`),
	}, true)
	if got := model.Spec.ValueString(); got != `{"kind":"new"}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelPreservesSemanticallyEqualSpec(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(`{"models":{"chat":{"resource_id":"example"}},"collections":{}}`)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "RuntimeProfile",
		Metadata:   resourceMetadata{ID: "example-profile"},
		Spec:       json.RawMessage(`{"collections":{},"models":{"chat":{"resource_id":"example"}}}`),
	}, true)
	if got := model.Spec.ValueString(); got != `{"models":{"chat":{"resource_id":"example"}},"collections":{}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelPreservesConfiguredFieldsOmittedByRead(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"mem0":{"custom_instructions":"write-only"},"volc_mem0":{"strategies":[{"type":"semantic","custom_instructions":"write-only"}]}}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "MemoryLayout",
		Metadata:   resourceMetadata{ID: "example-memory"},
		Spec:       json.RawMessage(`{"mem0":{},"volc_mem0":{"strategies":[{"type":"semantic"}]}}`),
	}, true)
	if got := model.Spec.ValueString(); got != `{"mem0":{"custom_instructions":"write-only"},"volc_mem0":{"strategies":[{"type":"semantic","custom_instructions":"write-only"}]}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelRecordsNestedCustomInstructionOmission(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"mem0":{"extension":{"custom_instructions":"ordinary field"}}}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "MemoryLayout",
		Metadata:   resourceMetadata{ID: "example-memory"},
		Spec:       json.RawMessage(`{"mem0":{"extension":{}}}`),
	}, true)
	if got := model.Spec.ValueString(); got != `{"mem0":{"extension":{}}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelPreservesCustomInstructionTrailingWhitespace(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"mem0":{"custom_instructions":"Keep this prompt."}}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "MemoryLayout",
		Metadata:   resourceMetadata{ID: "example-memory"},
		Spec:       json.RawMessage("{\"mem0\":{\"custom_instructions\":\"Keep this prompt.\\n\"}}"),
	}, true)
	if got := model.Spec.ValueString(); got != `{"mem0":{"custom_instructions":"Keep this prompt."}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelRecordsCustomInstructionWhitespaceForOtherKinds(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"custom_instructions":"Keep this prompt."}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "Workflow",
		Metadata:   resourceMetadata{ID: "example-workflow"},
		Spec:       json.RawMessage("{\"custom_instructions\":\"Keep this prompt.\\n\"}"),
	}, true)
	if got := model.Spec.ValueString(); got != "{\"custom_instructions\":\"Keep this prompt.\\n\"}" {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelRecordsRemovedPendingFirmwareSlot(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"description":"declarative channels","slots":{"stable":{},"beta":{},"develop":{}}}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "Firmware",
		Metadata:   resourceMetadata{ID: "example-firmware"},
		Spec: json.RawMessage(
			`{"description":"declarative channels","slots":{"stable":{},"beta":{},"develop":{"version":"dev","files":[{"path":"firmware.bin"}]},"pending":{}}}`,
		),
	}, true)
	if got := model.Spec.ValueString(); got != `{"description":"declarative channels","slots":{"stable":{},"beta":{},"develop":{"version":"dev","files":[{"path":"firmware.bin"}]},"pending":{}}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelRecordsUnexpectedFirmwareSlot(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"description":"declarative channels","slots":{"stable":{},"beta":{},"develop":{}}}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "Firmware",
		Metadata:   resourceMetadata{ID: "example-firmware"},
		Spec: json.RawMessage(
			`{"description":"declarative channels","slots":{"stable":{},"beta":{},"develop":{},"experimental":{}}}`,
		),
	}, true)
	if got := model.Spec.ValueString(); got != `{"description":"declarative channels","slots":{"stable":{},"beta":{},"develop":{},"experimental":{}}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelRecordsFirmwareDescriptionDrift(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"description":"declarative channels","slots":{"develop":{}}}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "Firmware",
		Metadata:   resourceMetadata{ID: "example-firmware"},
		Spec:       json.RawMessage(`{"description":"changed","slots":{"develop":{"version":"dev"}}}`),
	}, true)
	if got := model.Spec.ValueString(); got != `{"description":"changed","slots":{"develop":{"version":"dev"}}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelRecordsObservedChangeWithinReadSubset(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"kind":"llm","credential":"write-only"}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "Model",
		Metadata:   resourceMetadata{ID: "example-model"},
		Spec:       json.RawMessage(`{"kind":"embedding"}`),
	}, true)
	if got := model.Spec.ValueString(); got != `{"kind":"embedding"}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelRecordsOrdinaryConfiguredFieldOmittedByRead(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(
		`{"collections":{"assistants":["chat"]},"workflow":"chat"}`,
	)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "RuntimeProfile",
		Metadata:   resourceMetadata{ID: "example-profile"},
		Spec:       json.RawMessage(`{"collections":{"assistants":["chat"]}}`),
	}, true)
	if got := model.Spec.ValueString(); got != `{"collections":{"assistants":["chat"]}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelPreservesEnvironmentPlaceholderWhenObservedValueMatches(t *testing.T) {
	t.Setenv("GIZCLAW_MINIMAX_CN_GROUP_ID", "1234567890123456789")
	configured := `{"base_url":"https://api.minimaxi.com","group_id":"${GIZCLAW_MINIMAX_CN_GROUP_ID}"}`
	model := adminResourceModel{Spec: types.StringValue(configured)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "MiniMaxTenant",
		Metadata:   resourceMetadata{ID: "minimax-cn"},
		Spec:       json.RawMessage(`{"base_url":"https://api.minimaxi.com","group_id":"1234567890123456789"}`),
	}, true)
	if got := model.Spec.ValueString(); got != configured {
		t.Fatalf("spec = %q", got)
	}

	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "MiniMaxTenant",
		Metadata:   resourceMetadata{ID: "minimax-cn"},
		Spec:       json.RawMessage(`{"base_url":"https://api.minimaxi.com","group_id":"different"}`),
	}, true)
	if got := model.Spec.ValueString(); got == configured {
		t.Fatal("mismatched observed environment value was accepted")
	}
}

func TestSetModelPreservesCredentialSpecOnRead(t *testing.T) {
	model := adminResourceModel{Spec: types.StringValue(`{"body":{"api_key":"${API_KEY}"}}`)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "Credential",
		Metadata:   resourceMetadata{ID: "example-credential"},
		Spec:       json.RawMessage(`{"body":{"api_key":"redacted"}}`),
	}, true)
	if got := model.Spec.ValueString(); got != `{"body":{"api_key":"${API_KEY}"}}` {
		t.Fatalf("spec = %q", got)
	}
}

func TestSetModelPreservesDefaultedEnvironmentPlaceholders(t *testing.T) {
	t.Setenv("GIZCLAW_TEST_SET_REGION", "")
	t.Setenv("GIZCLAW_TEST_SET_HOST", "api.example.com")
	configured := `{"region":"${GIZCLAW_TEST_SET_REGION:-cn-beijing}","base_url":"https://${GIZCLAW_TEST_SET_HOST}/v1","unset":"${GIZCLAW_TEST_SET_UNSET:-fallback}"}`
	model := adminResourceModel{Spec: types.StringValue(configured)}
	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "VolcTenant",
		Metadata:   resourceMetadata{ID: "volc"},
		Spec:       json.RawMessage(`{"region":"cn-beijing","base_url":"https://api.example.com/v1","unset":"fallback"}`),
	}, true)
	if got := model.Spec.ValueString(); got != configured {
		t.Fatalf("spec = %q", got)
	}

	setModel(&model, resourceEnvelope{
		APIVersion: resourceAPIVersion,
		Kind:       "VolcTenant",
		Metadata:   resourceMetadata{ID: "volc"},
		Spec:       json.RawMessage(`{"region":"cn-shanghai","base_url":"https://api.example.com/v1","unset":"fallback"}`),
	}, true)
	if got := model.Spec.ValueString(); got == configured {
		t.Fatal("drift from a defaulted placeholder was hidden")
	}
}
