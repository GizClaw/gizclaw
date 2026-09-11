package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type crudHarness struct {
	t        *testing.T
	resource *adminResource
	schema   resource.SchemaResponse
	server   *fakeServer
}

func newCRUDHarness(t *testing.T) *crudHarness {
	t.Helper()
	server := newFakeServer()
	client, _ := newTestClient(server)
	r := &adminResource{}
	var configureResp resource.ConfigureResponse
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: client}, &configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configure: %v", configureResp.Diagnostics)
	}
	h := &crudHarness{t: t, resource: r, server: server}
	r.Schema(context.Background(), resource.SchemaRequest{}, &h.schema)
	return h
}

func (h *crudHarness) emptyState() tfsdk.State {
	return tfsdk.State{Schema: h.schema.Schema, Raw: tftypes.NewValue(h.schema.Schema.Type().TerraformType(context.Background()), nil)}
}

func (h *crudHarness) state(model adminResourceModel) tfsdk.State {
	h.t.Helper()
	state := h.emptyState()
	if diags := state.Set(context.Background(), &model); diags.HasError() {
		h.t.Fatalf("set state: %v", diags)
	}
	return state
}

func (h *crudHarness) plan(model adminResourceModel) tfsdk.Plan {
	h.t.Helper()
	state := h.state(model)
	return tfsdk.Plan{Schema: state.Schema, Raw: state.Raw}
}

func (h *crudHarness) model(state tfsdk.State) adminResourceModel {
	h.t.Helper()
	var model adminResourceModel
	if diags := state.Get(context.Background(), &model); diags.HasError() {
		h.t.Fatalf("get state: %v", diags)
	}
	return model
}

func plannedModel(spec string) adminResourceModel {
	return adminResourceModel{
		ID:            types.StringUnknown(),
		APIVersion:    types.StringUnknown(),
		Kind:          types.StringValue("Model"),
		ResourceID:    types.StringValue("example-model"),
		Spec:          types.StringValue(spec),
		InputRevision: types.StringNull(),
	}
}

func TestAdminResourceCRUDAgainstFakeServer(t *testing.T) {
	h := newCRUDHarness(t)
	ctx := context.Background()

	createResp := resource.CreateResponse{State: h.emptyState()}
	h.resource.Create(ctx, resource.CreateRequest{Plan: h.plan(plannedModel(`{"kind":"llm"}`))}, &createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics)
	}
	created := h.model(createResp.State)
	if created.ID.ValueString() != "Model/example-model" || created.APIVersion.ValueString() != resourceAPIVersion {
		t.Fatalf("created = %+v", created)
	}

	// Remote drift is recorded on refresh.
	h.server.mu.Lock()
	h.server.stored["Model/example-model"] = []byte(`{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"example-model"},"spec":{"kind":"embedding"}}`)
	h.server.mu.Unlock()
	readResp := resource.ReadResponse{State: createResp.State}
	h.resource.Read(ctx, resource.ReadRequest{State: createResp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResp.Diagnostics)
	}
	if got := h.model(readResp.State).Spec.ValueString(); got != `{"kind":"embedding"}` {
		t.Fatalf("refreshed spec = %q", got)
	}
	h.server.mu.Lock()
	delete(h.server.stored, "Model/example-model")
	h.server.mu.Unlock()

	updatePlan := plannedModel(`{"kind":"llm","source":"manual"}`)
	updatePlan.ID = created.ID
	updatePlan.APIVersion = created.APIVersion
	updateResp := resource.UpdateResponse{State: readResp.State}
	h.resource.Update(ctx, resource.UpdateRequest{Plan: h.plan(updatePlan), State: readResp.State}, &updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResp.Diagnostics)
	}
	if len(h.server.applied) != 2 || !strings.Contains(string(h.server.applied[1]), `"source":"manual"`) {
		t.Fatalf("applied = %s", h.server.applied)
	}

	deleteResp := resource.DeleteResponse{State: updateResp.State}
	h.resource.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResp.Diagnostics)
	}
	// Deleting an already absent resource succeeds.
	h.resource.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, &deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("second delete: %v", deleteResp.Diagnostics)
	}

	// A NOT_FOUND read removes the resource from state.
	goneResp := resource.ReadResponse{State: updateResp.State}
	h.resource.Read(ctx, resource.ReadRequest{State: updateResp.State}, &goneResp)
	if goneResp.Diagnostics.HasError() || !goneResp.State.Raw.IsNull() {
		t.Fatalf("read after delete: diags=%v state=%v", goneResp.Diagnostics, goneResp.State.Raw)
	}
}

func TestAdminResourceReadKeepsStateOnTransportFailure(t *testing.T) {
	h := newCRUDHarness(t)
	h.server.put(t, modelManifest)
	h.server.failNext.Store(maxOperationAttempts)
	state := h.state(adminResourceModel{
		ID: types.StringValue("Model/example-model"), APIVersion: types.StringValue(resourceAPIVersion),
		Kind: types.StringValue("Model"), ResourceID: types.StringValue("example-model"),
		Spec: types.StringValue(`{"kind":"llm","source":"manual"}`), InputRevision: types.StringNull(),
	})
	readResp := resource.ReadResponse{State: state}
	h.resource.Read(context.Background(), resource.ReadRequest{State: state}, &readResp)
	if !readResp.Diagnostics.HasError() || readResp.State.Raw.IsNull() {
		t.Fatalf("transport failure removed state or succeeded: %v", readResp.Diagnostics)
	}
}

func TestAdminResourceCreateRejectsMismatchedIdentity(t *testing.T) {
	h := newCRUDHarness(t)
	h.server.mu.Lock()
	h.server.stored["Model/example-model"] = []byte(`{"apiVersion":"gizclaw.admin/v1alpha1","kind":"Model","metadata":{"id":"other"},"spec":{}}`)
	h.server.mu.Unlock()
	createResp := resource.CreateResponse{State: h.emptyState()}
	h.resource.Create(context.Background(), resource.CreateRequest{Plan: h.plan(plannedModel(`{"kind":"llm"}`))}, &createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("mismatched response identity accepted")
	}
}

func TestAdminResourceImportState(t *testing.T) {
	h := newCRUDHarness(t)
	resp := resource.ImportStateResponse{State: h.emptyState()}
	h.resource.ImportState(context.Background(), resource.ImportStateRequest{ID: "Model/folder/id"}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("import: %v", resp.Diagnostics)
	}
	var kind, id types.String
	resp.State.GetAttribute(context.Background(), path.Root("kind"), &kind)
	resp.State.GetAttribute(context.Background(), path.Root("resource_id"), &id)
	if kind.ValueString() != "Model" || id.ValueString() != "folder/id" {
		t.Fatalf("imported %s/%s", kind, id)
	}
	bad := resource.ImportStateResponse{State: h.emptyState()}
	h.resource.ImportState(context.Background(), resource.ImportStateRequest{ID: "Unknown/id"}, &bad)
	if !bad.Diagnostics.HasError() {
		t.Fatal("unknown kind import accepted")
	}
}
