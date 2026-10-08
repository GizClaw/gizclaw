// Package toolcatalog resolves Profile Tool aliases for discovery and execution.
package toolcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/runtimealias"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit"
	"github.com/google/jsonschema-go/jsonschema"
)

// Availability separates a configured binding from observed device support.
type Availability struct {
	Supported bool
	Online    bool
	Reason    string
}

// Devices observes and executes only the authenticated owner's concrete Peer.
// DeviceSnapshotter batches capability discovery without probing every Tool.
type DeviceSnapshotter interface {
	Snapshot(context.Context, string, apitypes.RuntimeProfile) (Devices, error)
}

type Devices interface {
	Inspect(context.Context, string, apitypes.RuntimeProfile, Tool) (Availability, error)
	Invoke(context.Context, string, apitypes.RuntimeProfile, Tool, json.RawMessage) (json.RawMessage, error)
}

// Tool is an ephemeral, fully bound capability; HTTP credentials stay private.
type Tool struct {
	Alias        string
	FunctionName string
	Description  string
	Binding      apitypes.RuntimeProfileToolBinding
	Schema       jsonschema.Schema
	Source       string
	Target       map[string]any
	HTTP         *toolkit.Tool
	Authorize    func(context.Context) error
	Availability
	Available bool
}

// Catalog is shared by AgentHost and authenticated Peer discovery.
type Catalog struct {
	Tools   *toolkit.Server
	Devices Devices
}

// FunctionName maps the Profile grammar to the provider grammar. Underscores
// are forbidden in aliases, so replacing dots with underscores is reversible.
func FunctionName(alias string) (string, error) {
	if err := runtimealias.Validate("tool", alias); err != nil {
		return "", err
	}
	return strings.ReplaceAll(alias, ".", "_"), nil
}

// BindingSchema verifies source identity and creates the bounded argument schema.
func BindingSchema(profile apitypes.RuntimeProfile, binding apitypes.RuntimeProfileToolBinding) (string, map[string]any, jsonschema.Schema, error) {
	count := 0
	if binding.ResourceId != "" {
		count++
	}
	if binding.Mhs != nil {
		count++
	}
	if binding.ClientTool != nil {
		count++
	}
	if count != 1 {
		return "", nil, jsonschema.Schema{}, errors.New("Tool binding requires exactly one source")
	}
	if binding.ResourceId != "" {
		return "http_request", map[string]any{}, jsonschema.Schema{}, customid.ValidateResourceID(binding.ResourceId)
	}
	if binding.ClientTool != nil {
		name := binding.ClientTool.Name
		if _, err := rpcapi.ClientToolByName(name); err != nil {
			return "", nil, jsonschema.Schema{}, err
		}
		schema, err := apitypes.InnerToolSchema("client_tool", name, nil)
		if err == nil && name == "run.workspace.set" {
			// The model catalog selects a Profile Workflow alias. The existing
			// control-app API also accepts Workspace names; those are not model
			// catalog identities and must not become a second target authority.
			delete(schema.Properties, "workspace_name")
			schema.OneOf = nil
			schema.Required = []string{"workflow_name"}
			schema.Description = "Select one Workflow alias from the bound RuntimeProfile for the current device. Ordinary selection does not start opening speech."
			if property := schema.Properties["kickoff"]; property != nil {
				property.Description = "Omitted or false means ordinary program selection with no opening speech. Use true only when the user explicitly asks the new agent to speak first after reload; starting or selecting a program alone does not request kickoff."
			}
			aliases := []string{}
			for alias := range profile.Spec.Workflows {
				aliases = append(aliases, alias)
			}
			slices.Sort(aliases)
			if property := schema.Properties["workflow_name"]; property != nil {
				property.Enum = []any{}
				for _, alias := range aliases {
					property.Enum = append(property.Enum, alias)
				}
			}
		}

		return "client_tool", map[string]any{"name": name}, schema, err
	}
	config := binding.Mhs
	if config.Operation != "read" && config.Operation != "write" {
		return "", nil, jsonschema.Schema{}, errors.New("invalid MHS operation")
	}
	if profile.Spec.Mhs == nil || profile.Spec.Mhs.V0 == nil {
		return "", nil, jsonschema.Schema{}, errors.New("MHS manifest is required")
	}
	for _, device := range profile.Spec.Mhs.V0.Devices {
		if device.Id != config.Id {
			continue
		}
		target := map[string]any{"id": device.Id, "hwd": string(device.Hwd), "operation": string(config.Operation)}
		if config.Operation == "read" {
			if config.Fields != nil {
				return "", nil, jsonschema.Schema{}, errors.New("MHS read cannot select write fields")
			}
			return "mhs", target, jsonschema.Schema{Type: "object", AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}, nil
		}
		if config.Fields == nil {
			return "", nil, jsonschema.Schema{}, errors.New("MHS write fields are required")
		}
		schema, err := apitypes.InnerToolSchema("mhs", string(device.Hwd), *config.Fields)
		target["fields"] = append([]string(nil), (*config.Fields)...)
		return "mhs", target, schema, err
	}
	return "", nil, jsonschema.Schema{}, fmt.Errorf("MHS instance %q is not declared", config.Id)
}

// Resolve retains configured, unavailable entries for discovery. An explicit
// selection restricts the list; nil means all configured bindings.
func (c *Catalog) Resolve(ctx context.Context, owner string, profile apitypes.RuntimeProfile, selection *[]string) ([]Tool, error) {
	if profile.Spec.Resources.Tools == nil {
		return []Tool{}, nil
	}
	aliases := make([]string, 0, len(*profile.Spec.Resources.Tools))
	for alias := range *profile.Spec.Resources.Tools {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	devices := Devices(nil)
	if c != nil {
		devices = c.Devices
	}
	if snapshots, ok := devices.(DeviceSnapshotter); ok {
		var err error
		devices, err = snapshots.Snapshot(ctx, owner, profile)
		if err != nil {
			return nil, err
		}
	}
	result := make([]Tool, 0, len(aliases))
	names := make(map[string]string)
	for _, alias := range aliases {
		if selection != nil && !slices.Contains(*selection, alias) {
			continue
		}
		name, err := FunctionName(alias)
		if err != nil {
			return nil, err
		}
		if previous, exists := names[name]; exists {
			return nil, fmt.Errorf("Tool names %q and %q collide", previous, alias)
		}
		names[name] = alias
		binding := (*profile.Spec.Resources.Tools)[alias]
		source, target, schema, err := BindingSchema(profile, binding)
		tool := Tool{Alias: alias, FunctionName: name, Binding: binding, Source: source, Target: target, Schema: schema}
		if text, ok := binding.I18n["zh-CN"]; ok {
			tool.Description = text.DisplayName
			if text.Description != nil {
				tool.Description += ": " + *text.Description
			}
			if binding.ClientTool != nil && binding.ClientTool.Name == "run.workspace.set" {
				aliases := []string{}
				for alias := range profile.Spec.Workflows {
					aliases = append(aliases, alias)
				}
				slices.Sort(aliases)
				for _, alias := range aliases {
					text := profile.Spec.Workflows[alias].I18n["zh-CN"]
					tool.Description += "; " + alias + " = " + text.DisplayName
					if text.Description != nil {
						tool.Description += " (" + *text.Description + ")"
					}
				}
			}

		}
		if err != nil {
			tool.Schema = jsonschema.Schema{Type: "object"}
			tool.Reason = "invalid_binding"
			result = append(result, tool)
			continue
		}
		if source == "http_request" {
			if c == nil || c.Tools == nil {
				return nil, toolkit.ErrNotConfigured
			}
			resource, err := c.Tools.GetToolByID(ctx, binding.ResourceId)
			if errors.Is(err, toolkit.ErrToolNotFound) {
				tool.Schema = jsonschema.Schema{Type: "object"}
				tool.Reason = "resource_missing"
			} else if err != nil {
				return nil, err
			} else {
				tool.HTTP = &resource
				tool.Schema = resource.InputSchema
				tool.Supported, tool.Online = true, true
				tool.Available = resource.Enabled
				if !resource.Enabled {
					tool.Reason = "disabled"
				}
			}
		} else if devices == nil {
			tool.Reason = "capability_unknown"
		} else {
			availability, err := devices.Inspect(ctx, owner, profile, tool)
			if err != nil {
				return nil, err
			}
			tool.Availability = availability
			tool.Available = tool.Supported && tool.Online && tool.Reason == ""
		}
		result = append(result, tool)
	}
	return result, nil
}

// WorkflowBinding resolves the exact Profile alias or its unique resource-id
// binding. All Workflow toolkit settings use the same ambiguity checks.
func WorkflowBinding(profile apitypes.RuntimeProfile, workflowName, workflowID string) (apitypes.RuntimeProfileWorkflowBinding, error) {
	if workflowName == "" {
		for alias, binding := range profile.Spec.Workflows {
			if binding.ResourceId != workflowID {
				continue
			}
			if workflowName != "" {
				return apitypes.RuntimeProfileWorkflowBinding{}, errors.New("ambiguous Workflow binding")
			}
			workflowName = alias
		}
	}
	binding, ok := profile.Spec.Workflows[workflowName]
	if !ok || binding.ResourceId != workflowID {
		return apitypes.RuntimeProfileWorkflowBinding{}, errors.New("Workflow binding is unavailable")
	}
	return binding, nil
}

// Selection returns a Workflow's alias opt-in narrowed by its Workspace. The
// resource policy grants no authority; legacy Workspace IDs only restrict HTTP.
func Selection(profile apitypes.RuntimeProfile, workflowName, workflowID string, policy *apitypes.ToolkitPolicy) ([]string, error) {
	binding, err := WorkflowBinding(profile, workflowName, workflowID)
	if err != nil {
		return nil, err
	}
	if binding.Toolkit == nil || binding.Toolkit.ToolNames == nil {
		return []string{}, nil
	}
	allowed := append([]string(nil), (*binding.Toolkit.ToolNames)...)
	if policy != nil {
		if policy.ToolNames != nil && policy.ToolIds != nil {
			return nil, errors.New("tool_names and tool_ids cannot be combined")
		}
		allowed = slices.DeleteFunc(allowed, func(alias string) bool {
			if policy.ToolNames != nil {
				return !slices.Contains(*policy.ToolNames, alias)
			}
			if policy.ToolIds != nil {
				if profile.Spec.Resources.Tools == nil {
					return true
				}
				tool, exists := (*profile.Spec.Resources.Tools)[alias]
				return !exists || tool.ResourceId == "" || !slices.Contains(*policy.ToolIds, tool.ResourceId)
			}
			return false
		})
	}
	sort.Strings(allowed)
	return slices.Compact(allowed), nil
}

// ValidateArguments rejects malformed, extra or missing model parameters before
// any transport executes. Validation errors never include argument values.
func ValidateArguments(tool Tool, args json.RawMessage) error {
	var value map[string]any
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(args))
	if err := validateJSONValue(decoder, 0); err != nil {
		return errors.New("Tool arguments are malformed or contain duplicate keys")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("Tool arguments contain trailing data")
	}
	if err := json.Unmarshal(args, &value); err != nil || value == nil {
		return errors.New("Tool arguments must be a JSON object")
	}
	resolved, err := tool.Schema.Resolve(nil)
	if err != nil {
		return errors.New("Tool input schema is invalid")
	}
	if err := resolved.Validate(value); err != nil {
		return errors.New("Tool arguments do not match input schema")
	}
	return nil
}

// Validate each nested object before decoding; JSON's last-key-wins behavior
// must not hide a conflicting target or value in an untrusted Tool call.
func validateJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON nesting is too deep")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate JSON key")
			}
			seen[name] = true
			if err := validateJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}

// InvocationError carries a bounded Server-owned failure classification rather
// than raw device payloads, provider messages or model arguments.
type InvocationError struct{ Code string }

func (e *InvocationError) Error() string { return "Tool device operation failed" }
