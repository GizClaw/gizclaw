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

type runner struct {
	bin               string
	configHome        string
	context           string
	expectAction      string
	syncVolcTenantID  string
	providerVoicesSet bool
}

type applyResult struct {
	Action string `json:"action"`
	ID     string `json:"id"`
	Kind   string `json:"kind"`
}

type applyListResult struct {
	Items []applyResult `json:"items"`
}

func main() {
	var bin string
	var configHome string
	var contextName string
	var expectAction string
	var syncVolcTenantID string
	flag.StringVar(&bin, "bin", "", "path to the gizclaw CLI")
	flag.StringVar(&configHome, "config-home", "", "CLI XDG config home")
	flag.StringVar(&contextName, "context", "admin", "admin context name")
	flag.StringVar(&expectAction, "expect-action", "", "required action for every apply result (for example created or unchanged)")
	flag.StringVar(&syncVolcTenantID, "sync-volc-tenant-id", "", "Volc tenant ID whose voices must be synced before RuntimeProfile apply")
	flag.Parse()

	if strings.TrimSpace(bin) == "" || strings.TrimSpace(configHome) == "" || flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	r := &runner{
		bin:              bin,
		configHome:       configHome,
		context:          contextName,
		expectAction:     strings.TrimSpace(expectAction),
		syncVolcTenantID: syncVolcTenantID,
	}
	for _, path := range flag.Args() {
		if err := r.applyFile(path); err != nil {
			fmt.Fprintf(os.Stderr, "apply fixture %s: %v\n", path, err)
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
	if documentContainsKind(document, "RuntimeProfile") && strings.TrimSpace(r.syncVolcTenantID) != "" && !r.providerVoicesSet {
		if err := r.syncProviderVoices(); err != nil {
			return err
		}
		r.providerVoicesSet = true
	}
	return r.applyDocument(document, data)
}

func (r *runner) applyDocument(document map[string]any, data []byte) error {
	expected, isList, err := resourceIdentities(document)
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
	if isList {
		var result applyListResult
		if err := yaml.Unmarshal(output, &result); err != nil {
			return fmt.Errorf("decode ResourceList apply result: %w", err)
		}
		if err := validateApplyResults(expected, result.Items, r.expectAction); err != nil {
			return fmt.Errorf("invalid ResourceList apply result: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	var result applyResult
	if err := yaml.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("decode apply result: %w", err)
	}
	if err := validateApplyResults(expected, []applyResult{result}, r.expectAction); err != nil {
		return fmt.Errorf("invalid apply result: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func resourceIdentities(document map[string]any) ([]applyResult, bool, error) {
	kind, err := requiredString(document, "kind")
	if err != nil {
		return nil, false, err
	}
	if kind != "ResourceList" {
		id, err := resourceID(document)
		if err != nil {
			return nil, false, err
		}
		return []applyResult{{ID: id, Kind: kind}}, false, nil
	}
	spec, err := requiredMap(document, "spec")
	if err != nil {
		return nil, true, err
	}
	items, ok := spec["items"].([]any)
	if !ok || len(items) == 0 {
		return nil, true, errors.New("ResourceList spec.items must be a non-empty array")
	}
	result := make([]applyResult, 0, len(items))
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("ResourceList spec.items[%d] must be an object", index)
		}
		itemKind, err := requiredString(item, "kind")
		if err != nil {
			return nil, true, fmt.Errorf("ResourceList spec.items[%d]: %w", index, err)
		}
		id, err := resourceID(item)
		if err != nil {
			return nil, true, fmt.Errorf("ResourceList spec.items[%d]: %w", index, err)
		}
		result = append(result, applyResult{ID: id, Kind: itemKind})
	}
	return result, true, nil
}

func resourceID(document map[string]any) (string, error) {
	metadata, err := requiredMap(document, "metadata")
	if err != nil {
		return "", err
	}
	return requiredString(metadata, "id")
}

func validateApplyResults(expected, actual []applyResult, expectAction string) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("items = %d, want %d", len(actual), len(expected))
	}
	for index := range expected {
		if actual[index].Kind != expected[index].Kind || actual[index].ID != expected[index].ID {
			return fmt.Errorf("items[%d] = %s/%s, want %s/%s", index, actual[index].Kind, actual[index].ID, expected[index].Kind, expected[index].ID)
		}
		if expectAction != "" && actual[index].Action != expectAction {
			return fmt.Errorf("items[%d] action = %q, want %q", index, actual[index].Action, expectAction)
		}
	}
	return nil
}

func documentContainsKind(document map[string]any, wanted string) bool {
	kind, _ := document["kind"].(string)
	if kind == wanted {
		return true
	}
	if kind != "ResourceList" {
		return false
	}
	spec, _ := document["spec"].(map[string]any)
	items, _ := spec["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if documentContainsKind(item, wanted) {
			return true
		}
	}
	return false
}

func (r *runner) syncProviderVoices() error {
	tenantID := strings.TrimSpace(r.syncVolcTenantID)
	if tenantID == "" {
		return errors.New("sync-volc-tenant-id must be a non-empty ID")
	}
	if _, err := r.run("admin", "volc-tenants", "sync-voices", tenantID, "--context", r.context); err != nil {
		return fmt.Errorf("sync VolcTenant/%s voices: %w", tenantID, err)
	}
	output, err := r.run("admin", "voices", "list", "--provider-kind", "volc-tenant", "--provider-id", tenantID, "--context", r.context)
	if err != nil {
		return fmt.Errorf("list synced VolcTenant/%s voices: %w", tenantID, err)
	}
	var voices []applyResult
	if err := yaml.Unmarshal(output, &voices); err != nil {
		return fmt.Errorf("decode synced voices: %w", err)
	}
	if len(voices) == 0 {
		return fmt.Errorf("VolcTenant/%s voice sync returned no voices", tenantID)
	}
	for index, voice := range voices {
		if strings.TrimSpace(voice.ID) == "" {
			return fmt.Errorf("synced voices[%d] has no canonical ID", index)
		}
	}
	return nil
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
