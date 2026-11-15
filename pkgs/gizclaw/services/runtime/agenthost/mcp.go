package agenthost

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/mcpclient"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"github.com/google/jsonschema-go/jsonschema"
	mcpproto "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPConfig is an owner-authorized Profile snapshot. Endpoint and headers remain
// private to product runtime code; Authorize rechecks the current binding.
type MCPConfig struct {
	Registry  *mcpclient.Registry
	ProfileID string
	Servers   map[string]apitypes.RuntimeProfileMCPServer
	Selection map[string]apitypes.RuntimeProfileMCPSelection
	Authorize func(context.Context) error
}

type mcpOperation struct{ server, operation, remote string }

// MCPInvoker exposes only selected MCP capabilities to an Eino generation.
// Its leases share Server-owned connections with other Workflow generations.
type MCPInvoker struct {
	config     MCPConfig
	sources    map[string]*mcpclient.Source
	operations map[string]mcpOperation
	leases     []io.Closer
	once       sync.Once
	closeErr   error
}

func cloneMCPConfig(config MCPConfig) (MCPConfig, error) {
	servers, err := json.Marshal(config.Servers)
	if err != nil {
		return MCPConfig{}, err
	}
	selection, err := json.Marshal(config.Selection)
	if err != nil {
		return MCPConfig{}, err
	}
	config.Servers = nil
	config.Selection = nil
	if err := json.Unmarshal(servers, &config.Servers); err != nil {
		return MCPConfig{}, err
	}
	if err := json.Unmarshal(selection, &config.Selection); err != nil {
		return MCPConfig{}, err
	}
	return config, nil
}

// OpenMCP preloads selected capabilities and rolls back all leases on failure.
func OpenMCP(ctx context.Context, config MCPConfig) (*MCPInvoker, error) {
	var err error
	config, err = cloneMCPConfig(config)
	if err != nil {
		return nil, err
	}
	if config.Registry == nil || config.Authorize == nil {
		return nil, errors.New("MCP runtime is not configured")
	}
	if err := config.Authorize(ctx); err != nil {
		return nil, err
	}
	invoker := &MCPInvoker{config: config, sources: make(map[string]*mcpclient.Source), operations: make(map[string]mcpOperation)}
	for _, alias := range slices.Sorted(maps.Keys(config.Selection)) {
		selection := config.Selection[alias]
		binding, exists := config.Servers[alias]
		if !exists {
			_ = invoker.Close()
			return nil, errors.New("MCP server binding is missing")
		}
		source, lease, err := config.Registry.Acquire(ctx, config.ProfileID, alias, binding)
		if err != nil {
			_ = invoker.Close()
			return nil, err
		}
		invoker.leases = append(invoker.leases, lease)
		resources := selection.Resources != nil && *selection.Resources
		tools := selection.Tools != nil && len(*selection.Tools) > 0
		if err := source.Prepare(ctx, resources, tools); err != nil {
			_ = invoker.Close()
			return nil, err
		}
		invoker.sources[alias] = source
		if resources {
			for _, operation := range []string{"list_resources", "list_resource_templates", "read_resource"} {
				invoker.add(alias, operation, "")
			}
		}
		if tools {
			for _, name := range *selection.Tools {
				invoker.add(alias, "call_tool", name)
			}
		}
	}
	if _, err := invoker.ResolveCatalog(ctx); err != nil {
		_ = invoker.Close()
		return nil, err
	}
	return invoker, nil
}

func (i *MCPInvoker) add(alias, operation, remote string) {
	digest := sha256.Sum256([]byte(alias + "\x00" + operation + "\x00" + remote))
	// Double underscores cannot be produced by the ordinary Profile alias
	// grammar, so these names cannot collide with HTTP or device tools.
	name := fmt.Sprintf("mcp__%x", digest[:20])
	i.operations[name] = mcpOperation{server: alias, operation: operation, remote: remote}
}

// Close releases this generation's leases once; shared consumers remain active.
func (i *MCPInvoker) Close() error {
	i.once.Do(func() {
		for _, lease := range i.leases {
			i.closeErr = errors.Join(i.closeErr, lease.Close())
		}
	})
	return i.closeErr
}

// Owns reports whether name belongs to the generation's authorized MCP surface.
func (i *MCPInvoker) Owns(name string) bool { _, ok := i.operations[name]; return ok }

// ResolveCatalog returns safe model metadata without deployment configuration.
func (i *MCPInvoker) ResolveCatalog(ctx context.Context) ([]toolcatalog.Tool, error) {
	if err := i.config.Authorize(ctx); err != nil {
		return nil, err
	}
	catalog := make([]toolcatalog.Tool, 0, len(i.operations))
	remoteCatalogs := make(map[string]map[string]*mcpproto.Tool)
	for name, operation := range i.operations {
		schema := jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
		description := operation.operation + " from MCP source " + operation.server
		sourceKind := "mcp_resource"
		if operation.operation == "read_resource" {
			schema.Properties = map[string]*jsonschema.Schema{"uri": {Type: "string", MinLength: new(1)}}
			schema.Required = []string{"uri"}
		}
		if operation.operation == "call_tool" {
			tools, exists := remoteCatalogs[operation.server]
			if !exists {
				remoteTools, err := i.sources[operation.server].Tools()
				if err != nil {
					return nil, err
				}
				tools = make(map[string]*mcpproto.Tool, len(remoteTools))
				for _, remote := range remoteTools {
					tools[remote.Name] = remote
				}
				remoteCatalogs[operation.server] = tools
			}
			found := false
			if remote := tools[operation.remote]; remote != nil {
				data, err := json.Marshal(remote.InputSchema)
				if err != nil {
					return nil, errors.New("invalid MCP tool schema")
				}
				schema = jsonschema.Schema{}
				if err := json.Unmarshal(data, &schema); err != nil || schema.Type != "object" {
					return nil, errors.New("MCP tool requires an object schema")
				}
				if _, err := schema.Resolve(nil); err != nil {
					return nil, errors.New("unsupported MCP tool schema")
				}
				description = operation.remote + " from MCP source " + operation.server + ". " + remote.Description
				found = true
			}
			if !found {
				return nil, errors.New("selected MCP tool is unavailable")
			}
			sourceKind = "mcp_tool"
		}
		catalog = append(catalog, toolcatalog.Tool{Alias: operation.server + "." + operation.operation + "." + operation.remote, FunctionName: name, Description: description, Source: sourceKind, Schema: schema, Target: map[string]any{"server": operation.server, "name": operation.remote, "operation": operation.operation}, Available: true, Availability: toolcatalog.Availability{Online: true, Supported: true}})
	}
	slices.SortFunc(catalog, func(a, b toolcatalog.Tool) int { return strings.Compare(a.FunctionName, b.FunctionName) })
	return catalog, nil
}

func (i *MCPInvoker) invoke(ctx context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	if err := i.config.Authorize(ctx); err != nil {
		return recoverableToolError("unavailable", "MCP authorization changed; reload the Workspace"), nil
	}
	operation := i.operations[name]
	source := i.sources[operation.server]
	if operation.operation == "call_tool" {
		result, err := source.Client.CallTool(ctx, operation.remote, args)
		if err != nil {
			return recoverableToolError("mcp_failure", "MCP tool call did not succeed"), nil
		}
		return json.Marshal(result)
	}
	var result any
	var err error
	switch operation.operation {
	case "list_resources":
		result, err = source.Cache.ListResources(ctx)
	case "list_resource_templates":
		result, err = source.Cache.ListResourceTemplates(ctx)
	case "read_resource":
		var input struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(args, &input); err != nil {
			return recoverableToolError("invalid_arguments", "resource URI is required"), nil
		}
		result, err = source.Cache.ReadResource(ctx, input.URI)
	}
	if err != nil {
		return recoverableToolError("mcp_failure", "MCP resource is unavailable"), nil
	}
	return json.Marshal(result)
}

func workflowMCP(profile apitypes.RuntimeProfile, ws apitypes.Workspace) map[string]apitypes.RuntimeProfileMCPSelection {
	if ws.Labels != nil {
		if binding, exists := profile.Spec.Workflows[(*ws.Labels)["workflow_name"]]; exists && binding.ResourceId == ws.WorkflowId {
			if binding.Mcp != nil {
				return *binding.Mcp
			}
			return nil
		}
	}
	for _, binding := range profile.Spec.Workflows {
		if binding.ResourceId == ws.WorkflowId && binding.Mcp != nil {
			return *binding.Mcp
		}
	}
	return nil
}

func (r ServiceResolver) resolveMCP(ctx context.Context, ws apitypes.Workspace) (*MCPConfig, error) {
	profile, ok := ctx.Value(runtimeProfileContextKey{}).(apitypes.RuntimeProfile)
	if !ok {
		return nil, nil
	}
	selection := workflowMCP(profile, ws)
	if len(selection) == 0 {
		return nil, nil
	}
	if r.MCPRegistry == nil || r.RuntimeProfileForOwner == nil || r.Workspaces == nil || ws.OwnerPublicKey == nil {
		return nil, errors.New("MCP requires an owner runtime and registry")
	}
	servers := make(map[string]apitypes.RuntimeProfileMCPServer)
	for alias := range selection {
		if profile.Spec.Resources.McpServers == nil {
			return nil, errors.New("MCP server is not bound")
		}
		server, ok := (*profile.Spec.Resources.McpServers)[alias]
		if !ok {
			return nil, errors.New("MCP server is not bound")
		}
		servers[alias] = server
	}
	owned, err := cloneMCPConfig(MCPConfig{Registry: r.MCPRegistry, ProfileID: profile.Id, Servers: servers, Selection: selection})
	if err != nil {
		return nil, err
	}
	config := &owned
	selection, servers = config.Selection, config.Servers
	owner := *ws.OwnerPublicKey
	config.Authorize = func(callCtx context.Context) error {
		access, ok := resourceAccessFromContext(callCtx)
		if !ok || access.ownerPublicKey != owner {
			return errors.New("MCP owner context is unavailable")
		}
		current, err := r.RuntimeProfileForOwner(callCtx, access.ownerPublicKey)
		if err != nil || current.Id != profile.Id {
			return errors.New("MCP Profile changed")
		}
		workspace, err := r.getAvailableWorkspaceByID(callCtx, ws.Id)
		if err != nil || workspace.OwnerPublicKey == nil || *workspace.OwnerPublicKey != access.ownerPublicKey || workspace.WorkflowId != ws.WorkflowId {
			return errors.New("MCP Workspace changed")
		}
		if !reflect.DeepEqual(workflowMCP(current, workspace), selection) {
			return errors.New("MCP permissions changed")
		}
		for alias, server := range servers {
			if current.Spec.Resources.McpServers == nil || !reflect.DeepEqual((*current.Spec.Resources.McpServers)[alias], server) {
				return errors.New("MCP binding changed")
			}
		}
		return nil
	}
	return config, nil
}
