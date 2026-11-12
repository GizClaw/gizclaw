package runtimeprofile

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"golang.org/x/net/http/httpguts"
)

func normalizeMCP(spec *apitypes.RuntimeProfileSpec, aliases map[string]string) error {
	servers := make(map[string]apitypes.RuntimeProfileMCPServer)
	if spec.Resources.McpServers != nil {
		for rawAlias, server := range *spec.Resources.McpServers {
			alias := strings.TrimSpace(rawAlias)
			if err := ValidateAlias("MCP server", alias); err != nil {
				return err
			}
			if err := registerProfileAlias(aliases, alias, "MCP server"); err != nil {
				return err
			}
			normalized, err := normalizeMCPServer(server)
			if err != nil {
				return fmt.Errorf("resources.mcp_servers.%s: %w", alias, err)
			}
			servers[alias] = normalized
		}
	}
	spec.Resources.McpServers = nil
	if len(servers) != 0 {
		spec.Resources.McpServers = &servers
	}
	previous := make(map[string]string)
	workflowAliases := make([]string, 0, len(spec.Workflows))
	for alias := range spec.Workflows {
		workflowAliases = append(workflowAliases, alias)
	}
	slices.Sort(workflowAliases)
	for _, alias := range workflowAliases {
		binding := spec.Workflows[alias]
		selection, err := normalizeMCPSelections(binding.Mcp, servers)
		if err != nil {
			return fmt.Errorf("workflows.%s.mcp: %w", alias, err)
		}
		binding.Mcp = selection
		spec.Workflows[alias] = binding
		if other, ok := previous[binding.ResourceId]; ok {
			if !reflect.DeepEqual(binding.Mcp, spec.Workflows[other].Mcp) {
				return fmt.Errorf("workflows.%s.mcp conflicts with workflows.%s.mcp for Workflow %q", alias, other, binding.ResourceId)
			}
		} else {
			previous[binding.ResourceId] = alias
		}
	}
	return nil
}

func normalizeMCPServer(server apitypes.RuntimeProfileMCPServer) (apitypes.RuntimeProfileMCPServer, error) {
	server.Endpoint = strings.TrimSpace(server.Endpoint)
	endpoint, err := url.Parse(server.Endpoint)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.Fragment != "" {
		return server, errors.New("endpoint must be an absolute HTTP(S) URL without userinfo or fragment")
	}
	headers := make(map[string]string)
	if server.Headers != nil {
		for name, value := range *server.Headers {
			lower := strings.ToLower(name)
			if !httpguts.ValidHeaderFieldName(name) || lower == "host" || lower == "accept" || lower == "content-type" || strings.HasPrefix(lower, "mcp-") {
				return server, errors.New("invalid or reserved HTTP header")
			}
			if !httpguts.ValidHeaderFieldValue(value) {
				return server, errors.New("invalid HTTP header value")
			}
			name = http.CanonicalHeaderKey(name)
			if _, duplicate := headers[name]; duplicate {
				return server, errors.New("duplicate HTTP header name")
			}
			headers[name] = value
		}
	}
	server.Headers = nil
	if len(headers) != 0 {
		server.Headers = &headers
	}
	interval := 5 * time.Minute
	if server.RefreshInterval != nil {
		interval, err = time.ParseDuration(strings.TrimSpace(*server.RefreshInterval))
		if err != nil || interval < time.Second || interval > 24*time.Hour {
			return server, errors.New("refresh_interval must be a duration from 1s through 24h")
		}
	}
	server.RefreshInterval = new(interval.String())
	return server, nil
}

func normalizeMCPSelections(raw *map[string]apitypes.RuntimeProfileMCPSelection, servers map[string]apitypes.RuntimeProfileMCPServer) (*map[string]apitypes.RuntimeProfileMCPSelection, error) {
	if raw == nil || len(*raw) == 0 {
		return nil, nil
	}
	selections := make(map[string]apitypes.RuntimeProfileMCPSelection, len(*raw))
	for rawAlias, selection := range *raw {
		alias := strings.TrimSpace(rawAlias)
		if err := ValidateAlias("MCP server", alias); err != nil {
			return nil, err
		}
		if _, duplicate := selections[alias]; duplicate {
			return nil, fmt.Errorf("server alias %q is duplicated after normalization", alias)
		}
		if _, exists := servers[alias]; !exists {
			return nil, fmt.Errorf("server alias %q is not declared in resources.mcp_servers", alias)
		}
		selection.Resources = new(selection.Resources != nil && *selection.Resources)
		var tools []string
		if selection.Tools != nil {
			tools = slices.Clone(*selection.Tools)
		}
		for _, name := range tools {
			if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || len(name) > 128 {
				return nil, fmt.Errorf("server %q Tool names must be non-blank valid UTF-8 and 1..128 bytes", alias)
			}
		}
		slices.Sort(tools)
		for i := 1; i < len(tools); i++ {
			if tools[i] == tools[i-1] {
				return nil, fmt.Errorf("server %q has duplicate Tool name %q", alias, tools[i])
			}
		}
		if !*selection.Resources && len(tools) == 0 {
			return nil, fmt.Errorf("server %q requires resources or at least one explicit Tool name", alias)
		}
		selection.Tools = nil
		if len(tools) != 0 {
			selection.Tools = &tools
		}
		selections[alias] = selection
	}
	return &selections, nil
}

func validateMCPResolver(spec apitypes.RuntimeProfileSpec) error {
	for alias, binding := range spec.Workflows {
		if binding.Mcp != nil && len(*binding.Mcp) != 0 {
			return fmt.Errorf("workflows.%s.mcp: resource resolver required to verify an Eino Workflow", alias)
		}
	}
	return nil
}

func validateWorkflowMCP(path string, driver apitypes.WorkflowDriver, binding apitypes.RuntimeProfileWorkflowBinding) error {
	if binding.Mcp != nil && len(*binding.Mcp) != 0 && driver != apitypes.WorkflowDriverEino {
		return fmt.Errorf("%s.mcp requires an Eino Workflow", path)
	}
	return nil
}
