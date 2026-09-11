package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const resourceAPIVersion = "gizclaw.admin/v1alpha1"

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type resourceIDValidator struct{}

func (resourceIDValidator) Description(context.Context) string {
	return "must be a valid caller-defined GizClaw resource ID"
}

func (v resourceIDValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (resourceIDValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := customid.ValidateResourceID(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid caller-defined resource ID", err.Error())
	}
}

type resourceKindValidator struct{}

func (resourceKindValidator) Description(context.Context) string {
	return "must be a concrete GizClaw resource kind"
}

func (v resourceKindValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (resourceKindValidator) ValidateString(
	_ context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := validateResourceKind(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid GizClaw resource kind", err.Error())
	}
}

type adminResource struct {
	client *adminClient
}

type adminResourceModel struct {
	ID            types.String `tfsdk:"id"`
	APIVersion    types.String `tfsdk:"api_version"`
	Kind          types.String `tfsdk:"kind"`
	ResourceID    types.String `tfsdk:"resource_id"`
	Spec          types.String `tfsdk:"spec"`
	InputRevision types.String `tfsdk:"input_revision"`
}

type adminResourceModelV0 struct {
	ID         types.String `tfsdk:"id"`
	APIVersion types.String `tfsdk:"api_version"`
	Kind       types.String `tfsdk:"kind"`
	Name       types.String `tfsdk:"name"`
	Spec       types.String `tfsdk:"spec"`
}

func newAdminResource() resource.Resource { return &adminResource{} }

func (r *adminResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (r *adminResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	immutable := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Version:     1,
		Description: "A declarative GizClaw Admin resource managed through the generic resource API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Stable kind/id resource identifier.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"api_version": schema.StringAttribute{
				Optional: true, Computed: true, Description: "GizClaw resource API version.",
				Validators: []validator.String{stringvalidator.OneOf(resourceAPIVersion)},
				// An omitted api_version keeps the stored value; without this an
				// unrelated spec change would plan it as unknown and force replacement.
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
			},
			"kind": schema.StringAttribute{
				Required: true, Description: "GizClaw resource kind.",
				Validators: []validator.String{resourceKindValidator{}}, PlanModifiers: immutable,
			},
			"resource_id": schema.StringAttribute{
				Required: true, Description: "Caller-defined Resource metadata.id.",
				Validators: []validator.String{resourceIDValidator{}}, PlanModifiers: immutable,
			},
			"spec": schema.StringAttribute{
				Required: true, Sensitive: true,
				Description: "Resource spec encoded with jsonencode(...). Marked sensitive because credential specs can contain secrets.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(2)},
			},
			"input_revision": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Opaque local input revision used to re-apply an otherwise unchanged resource.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						sha256Pattern,
						"must be a lowercase 64-character SHA-256 digest",
					),
				},
			},
		},
	}
}

func (r *adminResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	immutable := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{Attributes: map[string]schema.Attribute{
				"id":          schema.StringAttribute{Computed: true},
				"api_version": schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: immutable},
				"kind":        schema.StringAttribute{Required: true, PlanModifiers: immutable},
				"name":        schema.StringAttribute{Required: true, PlanModifiers: immutable},
				"spec":        schema.StringAttribute{Required: true, Sensitive: true},
			}},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior adminResourceModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, &adminResourceModel{
					ID: prior.ID, APIVersion: prior.APIVersion, Kind: prior.Kind,
					ResourceID: prior.Name, Spec: prior.Spec,
				})...)
			},
		},
	}
}

func (r *adminResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*adminClient)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("Expected *adminClient, got %T.", req.ProviderData))
		return
	}
	r.client = client
}

func (r *adminResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan adminResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resourceValue, err := modelEnvelope(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid GizClaw manifest", err.Error())
		return
	}
	stored, err := r.client.apply(ctx, resourceValue)
	if err != nil {
		resp.Diagnostics.AddError("Unable to apply GizClaw resource", err.Error())
		return
	}
	if err := validateStoredIdentity(plan.Kind.ValueString(), plan.ResourceID.ValueString(), stored); err != nil {
		resp.Diagnostics.AddError("Invalid GizClaw resource response", err.Error())
		return
	}
	setModel(&plan, stored, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *adminResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state adminResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	stored, err := r.client.refresh(ctx, state.Kind.ValueString(), state.ResourceID.ValueString())
	if adminresource.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read GizClaw resource", err.Error())
		return
	}
	if err := validateStoredIdentity(state.Kind.ValueString(), state.ResourceID.ValueString(), stored); err != nil {
		resp.Diagnostics.AddError("Invalid GizClaw resource response", err.Error())
		return
	}
	setModel(&state, stored, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *adminResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan adminResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resourceValue, err := modelEnvelope(plan)
	if err != nil {
		resp.Diagnostics.AddError("Invalid GizClaw manifest", err.Error())
		return
	}
	stored, err := r.client.apply(ctx, resourceValue)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update GizClaw resource", err.Error())
		return
	}
	if err := validateStoredIdentity(plan.Kind.ValueString(), plan.ResourceID.ValueString(), stored); err != nil {
		resp.Diagnostics.AddError("Invalid GizClaw resource response", err.Error())
		return
	}
	setModel(&plan, stored, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *adminResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state adminResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.delete(ctx, state.Kind.ValueString(), state.ResourceID.ValueString())
	if err != nil && !adminresource.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete GizClaw resource", err.Error())
	}
}

func (r *adminResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	kind, resourceID, err := parseAdminResourceImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import identifier", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("kind"), kind)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resource_id"), resourceID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func modelEnvelope(model adminResourceModel) ([]byte, error) {
	if err := customid.ValidateResourceID(model.ResourceID.ValueString()); err != nil {
		return nil, fmt.Errorf("metadata.id: %w", err)
	}
	var spec json.RawMessage
	if err := json.Unmarshal([]byte(model.Spec.ValueString()), &spec); err != nil {
		return nil, fmt.Errorf("spec must be valid JSON: %w", err)
	}
	if len(spec) == 0 {
		return nil, fmt.Errorf("spec is required")
	}
	var object map[string]any
	if err := json.Unmarshal(spec, &object); err != nil || object == nil {
		return nil, fmt.Errorf("spec must be a JSON object")
	}
	version := model.APIVersion.ValueString()
	if version == "" {
		version = resourceAPIVersion
	}
	return json.Marshal(resourceEnvelope{APIVersion: version, Kind: model.Kind.ValueString(), Metadata: resourceMetadata{ID: model.ResourceID.ValueString()}, Spec: spec})
}

func parseAdminResourceImportID(value string) (string, string, error) {
	kind, resourceID, ok := strings.Cut(value, "/")
	if !ok || kind == "" {
		return "", "", fmt.Errorf("expected an identifier in the form <kind>/<id>")
	}
	ref, err := adminresource.ParseReference(kind, resourceID)
	if err != nil {
		return "", "", err
	}
	return string(ref.Kind), ref.ID, nil
}

func validateResourceKind(kind string) error {
	_, err := adminresource.ParseReference(kind, "kind-check")
	return err
}

func validateStoredIdentity(kind, resourceID string, stored resourceEnvelope) error {
	if stored.Kind != kind || stored.Metadata.ID != resourceID {
		return fmt.Errorf(
			"response identity %s/%s does not match requested %s/%s",
			stored.Kind,
			stored.Metadata.ID,
			kind,
			resourceID,
		)
	}
	return nil
}

func setModel(model *adminResourceModel, stored resourceEnvelope, updateSpec bool) {
	spec := stored.Spec
	if compacted, err := json.Marshal(json.RawMessage(stored.Spec)); err == nil {
		spec = compacted
	}
	model.ID = types.StringValue(stored.Kind + "/" + stored.Metadata.ID)
	model.APIVersion = types.StringValue(stored.APIVersion)
	model.Kind = types.StringValue(stored.Kind)
	model.ResourceID = types.StringValue(stored.Metadata.ID)
	if updateSpec && stored.Kind != "Credential" &&
		!jsonObservedConsistent(stored.Kind, model.Spec.ValueString(), spec) {
		model.Spec = types.StringValue(string(spec))
	}
}

func jsonObservedConsistent(kind, configured string, observed json.RawMessage) bool {
	var configuredValue any
	if err := json.Unmarshal([]byte(configured), &configuredValue); err != nil {
		return false
	}
	var observedValue any
	if err := json.Unmarshal(observed, &observedValue); err != nil {
		return false
	}
	return observedSubsetOfConfigured(kind, nil, configuredValue, observedValue)
}

func observedSubsetOfConfigured(kind string, fieldPath []string, configured, observed any) bool {
	switch observedValue := observed.(type) {
	case map[string]any:
		configuredValue, ok := configured.(map[string]any)
		if !ok {
			return false
		}
		if kind == "Firmware" && len(fieldPath) == 2 && fieldPath[0] == "slots" &&
			len(configuredValue) == 0 {
			return true
		}
		if kind == "Firmware" && len(fieldPath) == 1 && fieldPath[0] == "slots" {
			for key, configuredEntry := range configuredValue {
				observedEntry, ok := observedValue[key]
				if !ok || !observedSubsetOfConfigured(
					kind,
					append(fieldPath, key),
					configuredEntry,
					observedEntry,
				) {
					return false
				}
			}
			for key := range observedValue {
				if _, configured := configuredValue[key]; !configured {
					return false
				}
			}
			return true
		}
		configuredValue, ok = toolCanonicalHeaders(kind, fieldPath, configuredValue)
		if !ok {
			return false
		}
		for key, configuredEntry := range configuredValue {
			observedEntry, ok := observedValue[key]
			childPath := append(fieldPath, key)
			if !ok {
				if observedFieldMayBeOmitted(kind, childPath) {
					continue
				}
				return false
			}
			if configuredEntry == nil && toolObservedDefault(kind, childPath, observedEntry) {
				continue
			}
			if !observedSubsetOfConfigured(
				kind,
				childPath,
				configuredEntry,
				observedEntry,
			) {
				return false
			}
		}
		for key, observedEntry := range observedValue {
			if _, configured := configuredValue[key]; !configured &&
				!toolObservedDefault(kind, append(fieldPath, key), observedEntry) {
				return false
			}
		}
		return true
	case []any:
		configuredValue, ok := configured.([]any)
		if !ok {
			return false
		}
		configuredValue = toolNormalizedSuccessStatusCodes(kind, fieldPath, configuredValue)
		if len(configuredValue) != len(observedValue) {
			return false
		}
		for index := range observedValue {
			if !observedSubsetOfConfigured(
				kind,
				append(fieldPath, fmt.Sprintf("%d", index)),
				configuredValue[index],
				observedValue[index],
			) {
				return false
			}
		}
		return true
	case string:
		configuredValue, ok := configured.(string)
		if !ok {
			return false
		}
		if expandedEnvironmentValueMatches(configuredValue, observedValue) ||
			toolNormalizedStringMatches(kind, fieldPath, configuredValue, observedValue) {
			return true
		}
		if memoryLayoutCustomInstructionsPath(kind, fieldPath) {
			return strings.TrimSpace(configuredValue) == strings.TrimSpace(observedValue)
		}
		return configuredValue == observedValue
	default:
		return reflect.DeepEqual(configured, observed)
	}
}

// expandedEnvironmentValueMatches reports whether configured contains
// environment references that expand, under the same rules used when the
// manifest is applied, to exactly the observed Server value.
func expandedEnvironmentValueMatches(configured, observed string) bool {
	if !adminresource.HasEnvReference(configured) {
		return false
	}
	expanded, err := adminresource.ExpandEnvString(configured)
	return err == nil && expanded == observed
}

func observedFieldMayBeOmitted(kind string, fieldPath []string) bool {
	return memoryLayoutCustomInstructionsPath(kind, fieldPath)
}

func memoryLayoutCustomInstructionsPath(kind string, fieldPath []string) bool {
	if kind != "MemoryLayout" {
		return false
	}
	if len(fieldPath) == 2 {
		return fieldPath[0] == "mem0" && fieldPath[1] == "custom_instructions"
	}
	if len(fieldPath) != 4 ||
		fieldPath[0] != "volc_mem0" ||
		fieldPath[1] != "strategies" ||
		fieldPath[3] != "custom_instructions" {
		return false
	}
	_, err := strconv.Atoi(fieldPath[2])
	return err == nil
}

var _ resource.Resource = (*adminResource)(nil)
var _ resource.ResourceWithConfigure = (*adminResource)(nil)
var _ resource.ResourceWithImportState = (*adminResource)(nil)
var _ resource.ResourceWithUpgradeState = (*adminResource)(nil)
var _ validator.String = resourceIDValidator{}
