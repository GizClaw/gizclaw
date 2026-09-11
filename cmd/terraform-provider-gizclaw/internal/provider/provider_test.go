package provider

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func providerConfig(t *testing.T, p provider.Provider, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	var schemaResp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &schemaResp)
	attrs := map[string]tftypes.Value{
		"context":  tftypes.NewValue(tftypes.String, nil),
		"endpoint": tftypes.NewValue(tftypes.String, nil),
	}
	maps.Copy(attrs, values)
	objectType := schemaResp.Schema.Type().TerraformType(context.Background())
	return tfsdk.Config{Schema: schemaResp.Schema, Raw: tftypes.NewValue(objectType, attrs)}
}

func createTestContext(t *testing.T, name string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	root, err := contextconn.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := (&contextstore.Store{Root: root}).Create(name, "http://127.0.0.1:9820"); err != nil {
		t.Fatal(err)
	}
}

func configure(t *testing.T, values map[string]tftypes.Value) provider.ConfigureResponse {
	t.Helper()
	p := New("test")()
	var resp provider.ConfigureResponse
	p.Configure(context.Background(), provider.ConfigureRequest{Config: providerConfig(t, p, values)}, &resp)
	return resp
}

func TestProviderConfigureUsesContextAttributeAndEndpoint(t *testing.T) {
	createTestContext(t, "prod")
	t.Setenv(contextEnv, "missing")
	resp := configure(t, map[string]tftypes.Value{
		"context":  tftypes.NewValue(tftypes.String, "prod"),
		"endpoint": tftypes.NewValue(tftypes.String, "https://admin.example.com:443"),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics = %v", resp.Diagnostics)
	}
	client, ok := resp.ResourceData.(*adminClient)
	if !ok {
		t.Fatalf("resource data = %T", resp.ResourceData)
	}
	if client.opts.Context != "prod" || client.opts.Endpoint != "https://admin.example.com:443" {
		t.Fatalf("options = %+v", client.opts)
	}
	again := configure(t, map[string]tftypes.Value{
		"context":  tftypes.NewValue(tftypes.String, "prod"),
		"endpoint": tftypes.NewValue(tftypes.String, "https://admin.example.com:443"),
	})
	if again.ResourceData != resp.ResourceData {
		t.Fatal("provider instances for the same context did not share one client")
	}
}

func TestProviderConfigureDefaultsFromEnvironment(t *testing.T) {
	createTestContext(t, "staging")
	t.Setenv(contextEnv, "staging")
	t.Setenv(endpointEnv, "https://staging.example.com")
	resp := configure(t, nil)
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics = %v", resp.Diagnostics)
	}
	client := resp.ResourceData.(*adminClient)
	if client.opts.Context != "staging" || client.opts.Endpoint != "https://staging.example.com" {
		t.Fatalf("options = %+v", client.opts)
	}
}

func TestProviderConfigureReportsInvalidContext(t *testing.T) {
	createTestContext(t, "prod")
	for name, values := range map[string]map[string]tftypes.Value{
		"missing context": {"context": tftypes.NewValue(tftypes.String, "missing")},
		"http endpoint":   {"context": tftypes.NewValue(tftypes.String, "prod"), "endpoint": tftypes.NewValue(tftypes.String, "http://admin.example.com")},
		"unknown context": {"context": tftypes.NewValue(tftypes.String, tftypes.UnknownValue)},
	} {
		t.Run(name, func(t *testing.T) {
			if resp := configure(t, values); !resp.Diagnostics.HasError() {
				t.Fatal("configure succeeded")
			}
		})
	}
	t.Setenv(endpointEnv, "https://admin.example.com/path")
	if resp := configure(t, map[string]tftypes.Value{"context": tftypes.NewValue(tftypes.String, "prod")}); !resp.Diagnostics.HasError() {
		t.Fatal("invalid GIZCLAW_ENDPOINT accepted")
	}
}

func TestProviderAttributeValidators(t *testing.T) {
	for _, tc := range []struct {
		validator validator.String
		value     string
		ok        bool
	}{
		{endpointValidator{}, "https://admin.example.com", true},
		{endpointValidator{}, "https://user@admin.example.com", false},
		{endpointValidator{}, "https://admin.example.com/api", false},
		{contextNameValidator{}, "prod", true},
		{contextNameValidator{}, "", false},
	} {
		var resp validator.StringResponse
		tc.validator.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(tc.value)}, &resp)
		if resp.Diagnostics.HasError() == tc.ok {
			t.Fatalf("%T(%q) diagnostics = %v", tc.validator, tc.value, resp.Diagnostics)
		}
	}
}

func TestProviderMetadataAndResources(t *testing.T) {
	p := New("1.2.3")()
	var meta provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &meta)
	if meta.TypeName != "gizclaw" || meta.Version != "1.2.3" {
		t.Fatalf("metadata = %+v", meta)
	}
	resources := p.Resources(context.Background())
	if len(resources) != 1 {
		t.Fatalf("resources = %d", len(resources))
	}
	var resourceMeta resource.MetadataResponse
	resources[0]().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: meta.TypeName}, &resourceMeta)
	if resourceMeta.TypeName != "gizclaw_resource" {
		t.Fatalf("resource type = %q", resourceMeta.TypeName)
	}
	dataSources := p.DataSources(context.Background())
	if len(dataSources) != 1 {
		t.Fatalf("data sources = %d", len(dataSources))
	}
	var dataSourceMeta datasource.MetadataResponse
	dataSources[0]().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: meta.TypeName}, &dataSourceMeta)
	if dataSourceMeta.TypeName != "gizclaw_catalog" {
		t.Fatalf("data source type = %q", dataSourceMeta.TypeName)
	}
	if !strings.HasPrefix(Address, "gizclaw.local/") {
		t.Fatalf("address = %q", Address)
	}
}
