package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

type resourceIDs map[string]map[string]string

type runner struct {
	bin               string
	configHome        string
	context           string
	syncVolcTenant    string
	providerVoicesSet bool
	ids               resourceIDs
}

type applyResult struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Name         string         `json:"name"`
	ProviderData map[string]any `json:"provider_data,omitempty"`
}

type applyListResult struct {
	Items []applyResult `json:"items"`
}

func main() {
	var bin string
	var configHome string
	var contextName string
	var syncVolcTenant string
	var firmwareName string
	var firmwareAsset string
	flag.StringVar(&bin, "bin", "", "path to the gizclaw CLI")
	flag.StringVar(&configHome, "config-home", "", "CLI XDG config home")
	flag.StringVar(&contextName, "context", "admin", "admin context name")
	flag.StringVar(&syncVolcTenant, "sync-volc-tenant", "volc-main", "Volc tenant name whose voices must be synced before RuntimeProfile apply")
	flag.StringVar(&firmwareName, "firmware-name", "", "firmware name whose asset should be uploaded after apply")
	flag.StringVar(&firmwareAsset, "firmware-asset", "", "firmware asset to upload after apply")
	flag.Parse()

	if strings.TrimSpace(bin) == "" || strings.TrimSpace(configHome) == "" || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	r := &runner{
		bin:            bin,
		configHome:     configHome,
		context:        contextName,
		syncVolcTenant: syncVolcTenant,
		ids:            resourceIDs{},
	}
	for _, path := range flag.Args() {
		if err := r.applyFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "apply fixture %s: %v\n", path, err)
			os.Exit(1)
		}
	}
	if firmwareName != "" || firmwareAsset != "" {
		if firmwareName == "" || firmwareAsset == "" {
			fmt.Fprintln(os.Stderr, "firmware-name and firmware-asset must be set together")
			os.Exit(2)
		}
		if err := r.uploadFirmware(firmwareName, firmwareAsset); err != nil {
			fmt.Fprintf(os.Stderr, "upload firmware asset: %v\n", err)
			os.Exit(1)
		}
	}
}

func (r *runner) applyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	return r.applyDocument(document)
}

func (r *runner) applyDocument(document map[string]any) error {
	kind, err := requiredString(document, "kind")
	if err != nil {
		return err
	}
	if kind == "ResourceList" {
		spec, err := requiredMap(document, "spec")
		if err != nil {
			return err
		}
		items, ok := spec["items"].([]any)
		if !ok || len(items) == 0 {
			return errors.New("ResourceList spec.items must be a non-empty array")
		}
		itemKind := ""
		uniform := true
		for index, raw := range items {
			item, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("ResourceList spec.items[%d] must be an object", index)
			}
			kind, err := requiredString(item, "kind")
			if err != nil {
				return fmt.Errorf("ResourceList spec.items[%d]: %w", index, err)
			}
			if itemKind == "" {
				itemKind = kind
			} else if itemKind != kind {
				uniform = false
			}
		}
		if uniform {
			if itemKind == "RuntimeProfile" && !r.providerVoicesSet {
				if err := r.syncProviderVoices(); err != nil {
					return err
				}
				r.providerVoicesSet = true
			}
			for index, raw := range items {
				item := raw.(map[string]any)
				if err := resolveResourceIDs(item, itemKind, r.ids); err != nil {
					return fmt.Errorf("ResourceList spec.items[%d]: %w", index, err)
				}
			}
			return r.applyResolvedDocument(document)
		}
		for index, raw := range items {
			item := raw.(map[string]any)
			if err := r.applyDocument(item); err != nil {
				return fmt.Errorf("ResourceList spec.items[%d]: %w", index, err)
			}
		}
		return nil
	}

	if kind == "RuntimeProfile" && !r.providerVoicesSet {
		if err := r.syncProviderVoices(); err != nil {
			return err
		}
		r.providerVoicesSet = true
	}
	if err := resolveResourceIDs(document, kind, r.ids); err != nil {
		return err
	}
	return r.applyResolvedDocument(document)
}

func (r *runner) applyResolvedDocument(document map[string]any) error {
	kind, err := requiredString(document, "kind")
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "gizclaw-e2e-resource-*.yaml")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	output, err := r.run("admin", "apply", "--context", r.context, "-f", path)
	if err != nil {
		return err
	}
	if _, err := os.Stdout.Write(output); err != nil {
		return err
	}
	if kind == "ResourceList" {
		var result applyListResult
		if err := yaml.Unmarshal(output, &result); err != nil {
			return fmt.Errorf("decode ResourceList apply result: %w", err)
		}
		if len(result.Items) == 0 {
			return fmt.Errorf("invalid ResourceList apply result: %s", strings.TrimSpace(string(output)))
		}
		for _, item := range result.Items {
			if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.Name) == "" {
				return fmt.Errorf("invalid ResourceList item apply result: %s", strings.TrimSpace(string(output)))
			}
			r.ids.put(item.Kind, item.Name, item.ID)
		}
		return nil
	}
	var result applyResult
	if err := yaml.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("decode apply result: %w", err)
	}
	if strings.TrimSpace(result.ID) == "" || result.Kind != kind || strings.TrimSpace(result.Name) == "" {
		return fmt.Errorf("invalid apply result: %s", strings.TrimSpace(string(output)))
	}
	r.ids.put(result.Kind, result.Name, result.ID)
	return nil
}

func (r *runner) syncProviderVoices() error {
	tenantID, err := r.ids.require("VolcTenant", r.syncVolcTenant)
	if err != nil {
		return err
	}
	if _, err := r.run("admin", "volc-tenants", "sync-voices", tenantID, "--context", r.context); err != nil {
		return fmt.Errorf("sync VolcTenant/%s voices: %w", r.syncVolcTenant, err)
	}
	output, err := r.run("admin", "voices", "list", "--provider-kind", "volc-tenant", "--provider-id", tenantID, "--context", r.context)
	if err != nil {
		return fmt.Errorf("list synced VolcTenant/%s voices: %w", r.syncVolcTenant, err)
	}
	var voices []applyResult
	if err := yaml.Unmarshal(output, &voices); err != nil {
		return fmt.Errorf("decode synced voices: %w", err)
	}
	if len(voices) == 0 {
		return fmt.Errorf("VolcTenant/%s voice sync returned no voices", r.syncVolcTenant)
	}
	return recordSyncedVoices(r.ids, r.syncVolcTenant, voices)
}

func recordSyncedVoices(ids resourceIDs, tenantName string, voices []applyResult) error {
	for _, voice := range voices {
		if voice.ID == "" || voice.Name == "" {
			return errors.New("synced voice has no canonical ID or name")
		}
		ids.put("Voice", voice.Name, voice.ID)
		providerVoiceID, _ := voice.ProviderData["voice_id"].(string)
		providerVoiceID = strings.TrimSpace(providerVoiceID)
		if providerVoiceID == "" {
			return fmt.Errorf("synced voice %q has no provider voice_id", voice.Name)
		}
		fixtureName := "volc-tenant:" + strings.TrimSpace(tenantName) + ":" + providerVoiceID
		ids.put("Voice", fixtureName, voice.ID)
	}
	return nil
}

func (r *runner) uploadFirmware(name, asset string) error {
	firmwareID, err := r.ids.require("Firmware", name)
	if err != nil {
		return err
	}
	_, err = r.run("admin", "firmwares", "upload-artifact", firmwareID, "--channel", "stable", "-f", asset, "--context", r.context)
	return err
}

func (r *runner) run(args ...string) ([]byte, error) {
	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		output, err := r.runOnce(args...)
		if err == nil {
			return output, nil
		}
		lastErr = err
		if !isTransientCommandError(err) || attempt == maxAttempts {
			break
		}
		time.Sleep(time.Duration(attempt) * 250 * time.Millisecond)
	}
	return nil, lastErr
}

func (r *runner) runOnce(args ...string) ([]byte, error) {
	cmd := exec.Command(r.bin, args...)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+r.configHome)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func isTransientCommandError(err error) bool {
	detail := strings.ToLower(err.Error())
	return strings.Contains(detail, "timeout waiting for client readiness") ||
		strings.Contains(detail, "gizclaw: dial:") ||
		strings.Contains(detail, "connection reset by peer") ||
		strings.Contains(detail, "unexpected eof")
}

func resolveResourceIDs(document map[string]any, kind string, ids resourceIDs) error {
	spec, err := requiredMap(document, "spec")
	if err != nil {
		return err
	}
	switch kind {
	case "DashScopeTenant", "DeepSeekTenant", "GeminiTenant", "MiniMaxTenant", "OpenAITenant", "VolcTenant":
		return replaceReference(spec, "credential_name", "credential_id", "Credential", ids)
	case "Model", "Voice":
		provider, err := requiredMap(spec, "provider")
		if err != nil {
			return err
		}
		providerKind, err := requiredString(provider, "kind")
		if err != nil {
			return err
		}
		tenantKind, ok := tenantResourceKind(providerKind)
		if !ok {
			return fmt.Errorf("unsupported provider kind %q", providerKind)
		}
		return replaceReference(provider, "name", "id", tenantKind, ids)
	case "Workflow":
		return resolveWorkflowToolIDs(spec, ids)
	case "Workspace":
		if err := replaceReference(spec, "workflow_name", "workflow_id", "Workflow", ids); err != nil {
			return err
		}
		return resolveToolkitToolIDs(spec, ids)
	case "RuntimeProfile":
		return resolveRuntimeProfileIDs(spec, ids)
	default:
		return nil
	}
}

func resolveWorkflowToolIDs(spec map[string]any, ids resourceIDs) error {
	if err := resolveToolkitToolIDs(spec, ids); err != nil {
		return err
	}
	pet, ok := spec["pet"].(map[string]any)
	if !ok {
		return nil
	}
	return resolveToolkitToolIDs(pet, ids)
}

func resolveToolkitToolIDs(parent map[string]any, ids resourceIDs) error {
	rawToolkit, exists := parent["toolkit"]
	if !exists {
		return nil
	}
	toolkitPolicy, ok := rawToolkit.(map[string]any)
	if !ok {
		return errors.New("toolkit must be an object")
	}
	rawToolIDs, exists := toolkitPolicy["tool_ids"]
	if !exists {
		return nil
	}
	toolNames, ok := rawToolIDs.([]any)
	if !ok {
		return errors.New("toolkit.tool_ids must be an array")
	}
	toolIDs := make([]any, len(toolNames))
	for index, raw := range toolNames {
		name, ok := raw.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("toolkit.tool_ids[%d] must be a non-empty Tool name", index)
		}
		id, err := ids.require("Tool", name)
		if err != nil {
			return err
		}
		toolIDs[index] = id
	}
	toolkitPolicy["tool_ids"] = toolIDs
	return nil
}

func resolveRuntimeProfileIDs(spec map[string]any, ids resourceIDs) error {
	if rawWorkflows, exists := spec["workflows"]; exists {
		workflows, ok := rawWorkflows.(map[string]any)
		if !ok {
			return errors.New("workflows must be an object")
		}
		if rawSystem, exists := workflows["system"]; exists {
			system, ok := rawSystem.(map[string]any)
			if !ok {
				return errors.New("workflows.system must be an object")
			}
			for _, key := range []string{"friend_chatroom", "group_chatroom", "pet"} {
				if _, exists := system[key]; !exists {
					continue
				}
				name, err := requiredString(system, key)
				if err != nil {
					return err
				}
				system[key], err = ids.require("Workflow", name)
				if err != nil {
					return err
				}
			}
		}
		if rawCollections, exists := workflows["collections"]; exists {
			collections, ok := rawCollections.(map[string]any)
			if !ok {
				return errors.New("workflows.collections must be an object")
			}
			if err := resolveBindingMap(collections, "Workflow", ids); err != nil {
				return err
			}
		}
	}
	rawResources, exists := spec["resources"]
	if !exists {
		return nil
	}
	resources, ok := rawResources.(map[string]any)
	if !ok {
		return errors.New("resources must be an object")
	}
	for field, resourceKind := range map[string]string{
		"models": "Model", "voices": "Voice", "tools": "Tool",
		"pet_defs": "PetDef", "badge_defs": "BadgeDef", "game_defs": "GameDef",
	} {
		rawBindings, exists := resources[field]
		if !exists {
			continue
		}
		bindings, ok := rawBindings.(map[string]any)
		if !ok {
			return fmt.Errorf("resources.%s must be an object", field)
		}
		for bindingName, rawBinding := range bindings {
			binding, ok := rawBinding.(map[string]any)
			if !ok {
				return fmt.Errorf("resources.%s.%s must be an object", field, bindingName)
			}
			if err := replaceReference(binding, "resource_id", "resource_id", resourceKind, ids); err != nil {
				return err
			}
		}
	}
	if rawMemories, exists := resources["memories"]; exists {
		memories, ok := rawMemories.(map[string]any)
		if !ok {
			return errors.New("resources.memories must be an object")
		}
		for bindingName, rawBinding := range memories {
			binding, ok := rawBinding.(map[string]any)
			if !ok {
				return fmt.Errorf("resources.memories.%s must be an object", bindingName)
			}
			if err := replaceReference(binding, "layout_id", "layout_id", "MemoryLayout", ids); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveBindingMap(collections map[string]any, kind string, ids resourceIDs) error {
	for collectionName, rawCollection := range collections {
		collection, ok := rawCollection.(map[string]any)
		if !ok {
			return fmt.Errorf("workflows.collections.%s must be an object", collectionName)
		}
		for bindingName, rawBinding := range collection {
			binding, ok := rawBinding.(map[string]any)
			if !ok {
				return fmt.Errorf("workflows.collections.%s.%s must be an object", collectionName, bindingName)
			}
			if err := replaceReference(binding, "resource_id", "resource_id", kind, ids); err != nil {
				return err
			}
		}
	}
	return nil
}

func replaceReference(object map[string]any, sourceKey, targetKey, kind string, ids resourceIDs) error {
	name, err := requiredString(object, sourceKey)
	if err != nil {
		return err
	}
	id, err := ids.require(kind, name)
	if err != nil {
		return err
	}
	delete(object, sourceKey)
	object[targetKey] = id
	return nil
}

func tenantResourceKind(kind string) (string, bool) {
	switch kind {
	case "dashscope-tenant":
		return "DashScopeTenant", true
	case "deepseek-tenant":
		return "DeepSeekTenant", true
	case "gemini-tenant":
		return "GeminiTenant", true
	case "minimax-tenant":
		return "MiniMaxTenant", true
	case "openai-tenant":
		return "OpenAITenant", true
	case "volc-tenant":
		return "VolcTenant", true
	default:
		return "", false
	}
}

func (ids resourceIDs) put(kind, name, id string) {
	if ids[kind] == nil {
		ids[kind] = map[string]string{}
	}
	ids[kind][name] = id
}

func (ids resourceIDs) require(kind, name string) (string, error) {
	id := strings.TrimSpace(ids[kind][strings.TrimSpace(name)])
	if id == "" {
		return "", fmt.Errorf("fixture reference %s/%s has not been created", kind, name)
	}
	return id, nil
}

func requiredMap(object map[string]any, key string) (map[string]any, error) {
	value, ok := object[key].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return value, nil
}

func requiredString(object map[string]any, key string) (string, error) {
	value, ok := object[key].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return value, nil
}
