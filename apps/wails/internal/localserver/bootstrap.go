package localserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

const (
	// RegistrationTokenFile is the private workspace file that hands the local
	// Desktop client's registration credential to the Play surface.
	RegistrationTokenFile       = "registration-token"
	appRegistrationTokenName    = "app:com.gizclaw.opensource"
	legacyRegistrationTokenName = "desktop-local"
	defaultRuntimeProfileName   = "default"
)

// Bootstrapper applies a validated catalog through the packaged companion CLI.
type Bootstrapper struct {
	Catalog    *Catalog
	Executable func() (string, error)
	Run        func(context.Context, string, []string, []string) error
	RunOutput  func(context.Context, string, []string, []string) ([]byte, error)
}

// MigrateRuntimeContract installs the fixed App runtime contract for a
// completed legacy Pod. It reapplies the bundled Workflows referenced by
// RuntimeProfile/default before replacing that Profile; unrelated resources
// and Workflows remain untouched.
func (b *Bootstrapper) MigrateRuntimeContract(ctx context.Context, podDir string) error {
	if b == nil || b.Catalog == nil || b.Executable == nil {
		return fmt.Errorf("local server bootstrap: bootstrapper is not configured")
	}
	var profile *ResourceEntry
	for i := range b.Catalog.Resources {
		entry := &b.Catalog.Resources[i]
		if entry.Kind == "RuntimeProfile" && entry.Name == defaultRuntimeProfileName {
			profile = entry
			break
		}
	}
	if profile == nil {
		return fmt.Errorf("local server bootstrap: RuntimeProfile/%s is missing from the catalog", defaultRuntimeProfileName)
	}
	contractEntries, err := b.runtimeContractEntries(*profile)
	if err != nil {
		return err
	}
	executable, err := b.Executable()
	if err != nil {
		return err
	}
	tempDir, environment, err := prepareAdminWorkspace(podDir)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	environment = setCommandEnvironment(environment, "input", "${input}")
	run := b.Run
	if run == nil {
		run = runBootstrapCommand
	}
	for _, entry := range contractEntries {
		if entry.Kind == "RuntimeProfile" {
			for _, item := range b.Catalog.VoiceSyncs {
				args := []string{"admin", item.Provider + "-tenants", "sync-voices", item.Tenant, "--context", "local"}
				if err := runBootstrapOperation(ctx, run, executable, args, environment); err != nil {
					return fmt.Errorf("local server bootstrap: sync %s voices for %s during runtime migration: %w", item.Provider, item.Tenant, err)
				}
			}
		}
		if entry.Kind == "RuntimeProfile" {
			err := runBootstrapOperation(ctx, run, executable, []string{"admin", "runtime-profiles", "delete", entry.Name, "--context", "local"}, environment)
			if err != nil && !strings.Contains(err.Error(), "RESOURCE_NOT_FOUND:") {
				return fmt.Errorf("local server bootstrap: replace %s/%s: %w", entry.Kind, entry.Name, err)
			}
		}
		file, err := b.extract(tempDir, entry.Path)
		if err != nil {
			return err
		}
		if err := runBootstrapOperation(ctx, run, executable, []string{"admin", "apply", "--context", "local", "-f", file}, environment); err != nil {
			return fmt.Errorf("local server bootstrap: migrate %s/%s: %w", entry.Kind, entry.Name, err)
		}
	}
	// Retrying a partially completed migration must produce a raw token that
	// matches the private handoff file written below.
	_ = runBootstrapOperation(ctx, run, executable, []string{"admin", "registration-tokens", "delete", appRegistrationTokenName, "--context", "local"}, environment)
	if err := b.createRegistrationToken(ctx, tempDir, podDir, executable, environment); err != nil {
		return fmt.Errorf("local server bootstrap: migrate RegistrationToken/%s: %w", appRegistrationTokenName, err)
	}
	if err := runBootstrapOperation(ctx, run, executable, []string{"admin", "registration-tokens", "delete", legacyRegistrationTokenName, "--context", "local"}, environment); err != nil && !strings.Contains(err.Error(), "RESOURCE_NOT_FOUND:") {
		return fmt.Errorf("local server bootstrap: retire RegistrationToken/%s: %w", legacyRegistrationTokenName, err)
	}
	return nil
}

func (b *Bootstrapper) runtimeContractEntries(profile ResourceEntry) ([]ResourceEntry, error) {
	data, err := fs.ReadFile(b.Catalog.FS, profile.Path)
	if err != nil {
		return nil, fmt.Errorf("local server bootstrap: read %s: %w", profile.Path, err)
	}
	var document struct {
		Spec struct {
			Workflows struct {
				System struct {
					FriendChatroom string `yaml:"friend_chatroom"`
					GroupChatroom  string `yaml:"group_chatroom"`
					Pet            string `yaml:"pet"`
				} `yaml:"system"`
				Collections map[string]map[string]struct {
					ResourceID string `yaml:"resource_id"`
				} `yaml:"collections"`
			} `yaml:"workflows"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("local server bootstrap: parse %s: %w", profile.Path, err)
	}
	referenced := make(map[string]bool)
	for _, bindings := range document.Spec.Workflows.Collections {
		for _, binding := range bindings {
			name := strings.TrimSpace(binding.ResourceID)
			if name != "" {
				referenced[name] = true
			}
		}
	}
	for _, name := range []string{
		document.Spec.Workflows.System.FriendChatroom,
		document.Spec.Workflows.System.GroupChatroom,
		document.Spec.Workflows.System.Pet,
	} {
		name = strings.TrimSpace(name)
		if name != "" {
			referenced[name] = true
		}
	}
	entries := make([]ResourceEntry, 0, len(referenced)+1)
	for _, entry := range b.Catalog.Resources {
		if entry.Kind == "Workflow" && referenced[entry.Name] {
			entries = append(entries, entry)
			delete(referenced, entry.Name)
		}
	}
	if len(referenced) != 0 {
		missing := make([]string, 0, len(referenced))
		for name := range referenced {
			missing = append(missing, name)
		}
		sort.Strings(missing)
		return nil, fmt.Errorf("local server bootstrap: RuntimeProfile/%s references Workflows missing from the catalog: %s", profile.Name, strings.Join(missing, ", "))
	}
	return append(entries, profile), nil
}

func prepareAdminWorkspace(podDir string) (string, []string, error) {
	tempDir, err := os.MkdirTemp(podDir, ".runtime-contract-")
	if err != nil {
		return "", nil, fmt.Errorf("local server bootstrap: create private migration workspace: %w", err)
	}
	cleanup := func(err error) (string, []string, error) {
		_ = os.RemoveAll(tempDir)
		return "", nil, err
	}
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return cleanup(fmt.Errorf("local server bootstrap: secure private migration workspace: %w", err))
	}
	configHome := filepath.Join(tempDir, "config")
	contextDir := filepath.Join(configHome, "gizclaw", "local")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		return cleanup(fmt.Errorf("local server bootstrap: create Admin context: %w", err))
	}
	contextData, err := os.ReadFile(filepath.Join(podDir, "admin_context", "local", "config.yaml"))
	if err != nil {
		return cleanup(fmt.Errorf("local server bootstrap: read generated Admin context: %w", err))
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), contextData, 0o600); err != nil {
		return cleanup(fmt.Errorf("local server bootstrap: materialize Admin context: %w", err))
	}
	environment := mergedCommandEnvironment(nil)
	environment = setCommandEnvironment(environment, "XDG_CONFIG_HOME", configHome)
	environment = setCommandEnvironment(environment, "AppData", configHome)
	return tempDir, environment, nil
}

// Apply creates every declarative resource, synchronizes dynamic voice
// resources, and uploads PetDef assets to one newly started local Server.
func (b *Bootstrapper) Apply(ctx context.Context, podDir string, savedEnvironment map[string]string) error {
	if b == nil || b.Catalog == nil || b.Executable == nil {
		return fmt.Errorf("local server bootstrap: bootstrapper is not configured")
	}
	executable, err := b.Executable()
	if err != nil {
		return err
	}
	resolved, missing := b.Catalog.ResolveEnvironment(savedEnvironment, os.LookupEnv)
	if len(missing) != 0 {
		return fmt.Errorf("local server bootstrap: missing environment: %s", strings.Join(missing, ", "))
	}
	tempDir, err := os.MkdirTemp(podDir, ".bootstrap-")
	if err != nil {
		return fmt.Errorf("local server bootstrap: create private workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return fmt.Errorf("local server bootstrap: secure private workspace: %w", err)
	}
	configHome := filepath.Join(tempDir, "config")
	contextDir := filepath.Join(configHome, "gizclaw", "local")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		return fmt.Errorf("local server bootstrap: create Admin context: %w", err)
	}
	contextData, err := os.ReadFile(filepath.Join(podDir, "admin_context", "local", "config.yaml"))
	if err != nil {
		return fmt.Errorf("local server bootstrap: read generated Admin context: %w", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), contextData, 0o600); err != nil {
		return fmt.Errorf("local server bootstrap: materialize Admin context: %w", err)
	}

	environment := mergedCommandEnvironment(resolved)
	environment = setCommandEnvironment(environment, "XDG_CONFIG_HOME", configHome)
	environment = setCommandEnvironment(environment, "AppData", configHome)
	environment = setCommandEnvironment(environment, "input", "${input}")
	run := b.Run
	if run == nil {
		run = runBootstrapCommand
	}
	apply := func(entry ResourceEntry) error {
		file, err := b.extract(tempDir, entry.Path)
		if err != nil {
			return err
		}
		args := []string{"admin", "apply", "--context", "local", "-f", file}
		if err := runBootstrapOperation(ctx, run, executable, args, environment); err != nil {
			return fmt.Errorf("local server bootstrap: apply %s/%s from %s: %w", entry.Kind, entry.Name, entry.Path, err)
		}
		return nil
	}
	applyEntries := func(listName string, entries []ResourceEntry) error {
		if len(entries) == 0 {
			return nil
		}
		file, err := b.extractResourceList(tempDir, listName, entries)
		if err != nil {
			return err
		}
		args := []string{"admin", "apply", "--context", "local", "-f", file}
		if err := runBootstrapOperation(ctx, run, executable, args, environment); err == nil {
			return nil
		}
		// ResourceList applies items sequentially and may have partially succeeded.
		// Reapplying the idempotent entries individually both completes the batch
		// after a transport failure and identifies a deterministic bad resource.
		for _, entry := range entries {
			if err := apply(entry); err != nil {
				return err
			}
		}
		return nil
	}
	resources := make([]ResourceEntry, 0, len(b.Catalog.Resources))
	runtimeProfiles := make([]ResourceEntry, 0, 1)
	for _, entry := range b.Catalog.Resources {
		if entry.Kind == "RuntimeProfile" {
			runtimeProfiles = append(runtimeProfiles, entry)
			continue
		}
		resources = append(resources, entry)
	}
	if err := applyEntries("desktop-bootstrap-resources", resources); err != nil {
		return err
	}
	for _, item := range b.Catalog.VoiceSyncs {
		args := []string{"admin", item.Provider + "-tenants", "sync-voices", item.Tenant, "--context", "local"}
		if err := runBootstrapOperation(ctx, run, executable, args, environment); err != nil {
			return fmt.Errorf("local server bootstrap: sync %s voices for %s: %w", item.Provider, item.Tenant, err)
		}
	}
	if err := applyEntries("desktop-bootstrap-runtime-profiles", runtimeProfiles); err != nil {
		return err
	}
	for _, asset := range b.Catalog.PetDefPIXAs {
		file, err := b.extract(tempDir, asset.PIXA)
		if err != nil {
			return err
		}
		args := []string{"admin", "pet-defs", "upload-pixa", asset.PetDef, "--context", "local", "-f", file}
		if err := runBootstrapOperation(ctx, run, executable, args, environment); err != nil {
			return fmt.Errorf("local server bootstrap: upload PetDef/%s PIXA: %w", asset.PetDef, err)
		}
	}
	if err := b.createRegistrationToken(ctx, tempDir, podDir, executable, environment); err != nil {
		return err
	}
	return nil
}

// RecoverRegistrationToken replaces the local Desktop token when an existing
// Pod predates token handoff or its private handoff file has been lost. The raw
// token cannot be recovered from server storage, so replacement is required.
func (b *Bootstrapper) RecoverRegistrationToken(ctx context.Context, podDir string, savedEnvironment map[string]string) error {
	if b == nil || b.Catalog == nil || b.Executable == nil {
		return fmt.Errorf("local server bootstrap: bootstrapper is not configured")
	}
	executable, err := b.Executable()
	if err != nil {
		return err
	}
	resolved, missing := b.Catalog.ResolveEnvironment(savedEnvironment, os.LookupEnv)
	if len(missing) != 0 {
		return fmt.Errorf("local server bootstrap: missing environment: %s", strings.Join(missing, ", "))
	}
	tempDir, err := os.MkdirTemp(podDir, ".registration-token-")
	if err != nil {
		return fmt.Errorf("local server bootstrap: create private token workspace: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return fmt.Errorf("local server bootstrap: secure private token workspace: %w", err)
	}
	configHome := filepath.Join(tempDir, "config")
	contextDir := filepath.Join(configHome, "gizclaw", "local")
	if err := os.MkdirAll(contextDir, 0o700); err != nil {
		return fmt.Errorf("local server bootstrap: create Admin context: %w", err)
	}
	contextData, err := os.ReadFile(filepath.Join(podDir, "admin_context", "local", "config.yaml"))
	if err != nil {
		return fmt.Errorf("local server bootstrap: read generated Admin context: %w", err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "config.yaml"), contextData, 0o600); err != nil {
		return fmt.Errorf("local server bootstrap: materialize Admin context: %w", err)
	}

	environment := mergedCommandEnvironment(resolved)
	environment = setCommandEnvironment(environment, "XDG_CONFIG_HOME", configHome)
	environment = setCommandEnvironment(environment, "AppData", configHome)
	run := b.Run
	if run == nil {
		run = runBootstrapCommand
	}
	// A missing token resource is the expected legacy case. Ignore deletion
	// errors here; creation below still reports connection, authorization, or
	// conflict failures without pretending recovery succeeded.
	_ = runBootstrapOperation(ctx, run, executable, []string{"admin", "registration-tokens", "delete", appRegistrationTokenName, "--context", "local"}, environment)
	if err := b.createRegistrationToken(ctx, tempDir, podDir, executable, environment); err != nil {
		return fmt.Errorf("local server bootstrap: recover Play registration token: %w", err)
	}
	return nil
}

func (b *Bootstrapper) createRegistrationToken(ctx context.Context, tempDir, podDir, executable string, environment []string) error {
	request := struct {
		Name               string `json:"name"`
		RuntimeProfileName string `json:"runtime_profile_name"`
	}{
		Name:               appRegistrationTokenName,
		RuntimeProfileName: defaultRuntimeProfileName,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("local server bootstrap: encode RegistrationToken request: %w", err)
	}
	requestFile := filepath.Join(tempDir, "registration-token.json")
	if err := os.WriteFile(requestFile, data, 0o600); err != nil {
		return fmt.Errorf("local server bootstrap: write RegistrationToken request: %w", err)
	}
	runOutput := b.RunOutput
	if runOutput == nil {
		runOutput = runBootstrapCommandOutput
	}
	output, err := runOutput(ctx, executable, []string{"admin", "registration-tokens", "create", "--context", "local", "-f", requestFile}, environment)
	if err != nil {
		return fmt.Errorf("local server bootstrap: create RegistrationToken/%s: %w", appRegistrationTokenName, err)
	}
	var result struct {
		Token *string `json:"token"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("local server bootstrap: decode RegistrationToken response: %w", err)
	}
	token := ""
	if result.Token != nil {
		token = strings.TrimSpace(*result.Token)
	}
	if token == "" {
		return fmt.Errorf("local server bootstrap: RegistrationToken response did not include a raw token")
	}
	workspaceDir := filepath.Join(podDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return fmt.Errorf("local server bootstrap: create private token directory: %w", err)
	}
	tokenFile := filepath.Join(workspaceDir, RegistrationTokenFile)
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		return fmt.Errorf("local server bootstrap: persist RegistrationToken: %w", err)
	}
	if err := os.Chmod(tokenFile, 0o600); err != nil {
		return fmt.Errorf("local server bootstrap: secure RegistrationToken: %w", err)
	}
	return nil
}

func (b *Bootstrapper) extractResourceList(root, name string, entries []ResourceEntry) (string, error) {
	items := make([]any, 0, len(entries))
	for _, entry := range entries {
		data, err := fs.ReadFile(b.Catalog.FS, entry.Path)
		if err != nil {
			return "", fmt.Errorf("local server bootstrap: read bundled %s: %w", entry.Path, err)
		}
		var item any
		if err := yaml.Unmarshal(data, &item); err != nil {
			return "", fmt.Errorf("local server bootstrap: parse bundled %s: %w", entry.Path, err)
		}
		items = append(items, item)
	}
	document := map[string]any{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind":       "ResourceList",
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"items": items},
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("local server bootstrap: encode %s: %w", name, err)
	}
	destination := filepath.Join(root, name+".yaml")
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		return "", fmt.Errorf("local server bootstrap: write %s: %w", name, err)
	}
	return destination, nil
}

func runBootstrapOperation(
	ctx context.Context,
	run func(context.Context, string, []string, []string) error,
	executable string,
	args, environment []string,
) error {
	const maxAttempts = 4
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := run(ctx, executable, args, environment)
		if err == nil || ctx.Err() != nil || !isTransientBootstrapCommandError(err) || attempt == maxAttempts {
			return err
		}
		delay := time.Duration(attempt) * 250 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func isTransientBootstrapCommandError(err error) bool {
	detail := strings.ToLower(err.Error())
	return strings.Contains(detail, "gizclaw: dial:") &&
		(strings.Contains(detail, "context deadline exceeded") ||
			strings.Contains(detail, "connection reset by peer") ||
			strings.Contains(detail, "unexpected eof"))
}

// ResolveEnvironment applies Desktop-saved values before process values and
// reports required names that still have neither a value nor a catalog default.
func (c *Catalog) ResolveEnvironment(saved map[string]string, lookup func(string) (string, bool)) (map[string]string, []string) {
	resolved := map[string]string{}
	var missing []string
	for _, requirement := range c.Requirements {
		if value := saved[requirement.Name]; value != "" {
			resolved[requirement.Name] = value
			continue
		}
		if value, ok := lookup(requirement.Name); ok && value != "" {
			resolved[requirement.Name] = value
			continue
		}
		if requirement.Default == nil {
			missing = append(missing, requirement.Name)
		}
	}
	return resolved, missing
}

func (b *Bootstrapper) extract(root, name string) (string, error) {
	data, err := fs.ReadFile(b.Catalog.FS, name)
	if err != nil {
		return "", fmt.Errorf("local server bootstrap: read bundled %s: %w", name, err)
	}
	destination := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", fmt.Errorf("local server bootstrap: create directory for %s: %w", name, err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		return "", fmt.Errorf("local server bootstrap: extract %s: %w", name, err)
	}
	return destination, nil
}

func runBootstrapCommand(ctx context.Context, executable string, args, environment []string) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = environment
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if detail := redactedBootstrapCommandError(stderr.String(), environment); detail != "" {
			return fmt.Errorf("%w: %s", err, detail)
		}
		return err
	}
	return nil
}

func runBootstrapCommandOutput(ctx context.Context, executable string, args, environment []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = environment
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if detail := redactedBootstrapCommandError(stderr.String(), environment); detail != "" {
			return nil, fmt.Errorf("%w: %s", err, detail)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func redactedBootstrapCommandError(stderr string, environment []string) string {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return ""
	}
	var secrets []string
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && value != "" && strings.HasPrefix(name, "GIZCLAW_") {
			secrets = append(secrets, value)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		detail = strings.ReplaceAll(detail, secret, "[REDACTED]")
	}
	const maxDetailBytes = 4096
	if len(detail) > maxDetailBytes {
		detail = detail[:maxDetailBytes] + "..."
	}
	return detail
}

func mergedCommandEnvironment(overrides map[string]string) []string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	maps.Copy(values, overrides)
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]string, 0, len(names))
	for _, name := range names {
		environment = append(environment, name+"="+values[name])
	}
	return environment
}

func setCommandEnvironment(environment []string, name, value string) []string {
	return setCommandEnvironmentForOS(environment, name, value, runtime.GOOS)
}

func setCommandEnvironmentForOS(environment []string, name, value, goos string) []string {
	for i, entry := range environment {
		entryName, _, ok := strings.Cut(entry, "=")
		if ok && (entryName == name || goos == "windows" && strings.EqualFold(entryName, name)) {
			environment[i] = name + "=" + value
			return environment
		}
	}
	return append(environment, name+"="+value)
}
