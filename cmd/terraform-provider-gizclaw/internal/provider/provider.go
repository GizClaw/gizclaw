// Package provider implements the GizClaw Terraform provider.
package provider

import (
	"context"
	"os"
	"sync"

	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Address is the provider source address used by Terraform configurations
// and filesystem mirrors.
const Address = "gizclaw.local/gizclaw/gizclaw"

const (
	contextEnv  = "GIZCLAW_CONTEXT"
	endpointEnv = "GIZCLAW_ENDPOINT"
)

type gizclawProvider struct {
	version string
}

type providerModel struct {
	Context  types.String `tfsdk:"context"`
	Endpoint types.String `tfsdk:"endpoint"`
}

// New returns a provider factory for version.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &gizclawProvider{version: version} }
}

func (p *gizclawProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "gizclaw"
	resp.Version = p.version
}

func (p *gizclawProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage declarative GizClaw Admin resources through the identity of a local GizClaw CLI context.",
		Attributes: map[string]schema.Attribute{
			"context": schema.StringAttribute{
				Optional:    true,
				Description: "GizClaw CLI context name under $XDG_CONFIG_HOME/gizclaw. Defaults to GIZCLAW_CONTEXT, then the current context.",
				Validators:  []validator.String{contextNameValidator{}},
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Server endpoint that replaces the context's server.endpoint, as https://host[:port]. Defaults to GIZCLAW_ENDPOINT, then the context endpoint.",
				Validators:  []validator.String{endpointValidator{}},
			},
		},
	}
}

func (p *gizclawProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.Context.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("context"), "Unknown GizClaw context", "The provider context must be known before GizClaw resources can be planned.")
	}
	if config.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Unknown GizClaw endpoint", "The provider endpoint must be known before GizClaw resources can be planned.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	opts := contextconn.Options{
		Context:  stringOrEnv(config.Context, contextEnv),
		Endpoint: stringOrEnv(config.Endpoint, endpointEnv),
	}
	// Load once so a missing context, identity, or invalid endpoint fails at
	// configuration time instead of on the first resource operation.
	if _, err := contextconn.LoadContext(opts); err != nil {
		resp.Diagnostics.AddError("Invalid GizClaw context", err.Error())
		return
	}
	client := sharedClient(opts)
	resp.ResourceData = client
	resp.DataSourceData = client
}

func (p *gizclawProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newAdminResource}
}

func (p *gizclawProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{newCatalogDataSource}
}

func stringOrEnv(value types.String, env string) string {
	if !value.IsNull() {
		return value.ValueString()
	}
	return os.Getenv(env)
}

var (
	sharedClientsMu sync.Mutex
	sharedClients   = map[contextconn.Options]*adminClient{}
)

// sharedClient returns the process-wide client for opts. Provider aliases that
// select the same context share one connection, because connections with the
// same identity replace each other on the Server.
func sharedClient(opts contextconn.Options) *adminClient {
	sharedClientsMu.Lock()
	defer sharedClientsMu.Unlock()
	if client, ok := sharedClients[opts]; ok {
		return client
	}
	client := newAdminClient(opts)
	sharedClients[opts] = client
	return client
}

type contextNameValidator struct{}

func (contextNameValidator) Description(context.Context) string {
	return "must be a non-empty GizClaw context name"
}

func (v contextNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (contextNameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid GizClaw context", "The context name must not be empty.")
	}
}

type endpointValidator struct{}

func (endpointValidator) Description(context.Context) string {
	return "must be https://host[:port] without userinfo, path, query, or fragment"
}

func (v endpointValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (endpointValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := contextconn.ValidateEndpoint(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid GizClaw endpoint", err.Error())
	}
}

var _ provider.Provider = (*gizclawProvider)(nil)
