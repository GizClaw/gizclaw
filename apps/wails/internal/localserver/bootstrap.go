package localserver

import (
	"bytes"
	"context"
	"errors"
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
	legacyRegistrationTokenFile = "registration-token"
	appRegistrationTokenName    = "app:com.gizclaw.opensource"
	legacyRegistrationTokenName = "desktop-local"
	defaultRuntimeProfileName   = "default"
)

// Bootstrapper applies a validated catalog through the packaged companion CLI.
type Bootstrapper struct {
	Catalog    *Catalog
	Resolver   CatalogResolver
	Executable func() (string, error)
	Run        func(context.Context, string, []string, []string) error
}

// MigrateRuntimeContract installs the resolved Raids dependency closure for a
// completed legacy Pod before replacing RuntimeProfile/default.
func (b *Bootstrapper) MigrateRuntimeContract(ctx context.Context, podDir string, savedEnvironment map[string]string) error {
	if b == nil || b.Executable == nil {
		return fmt.Errorf("local server bootstrap: bootstrapper is not configured")
	}
	catalog, err := b.catalog(ctx)
	if err != nil {
		return err
	}
	resolved, missing := catalog.ResolveEnvironment(savedEnvironment, os.LookupEnv)
	if len(missing) != 0 {
		return fmt.Errorf("local server bootstrap: missing environment: %s", strings.Join(missing, ", "))
	}
	var profile *ResourceEntry
	var registrationToken *ResourceEntry
	for i := range catalog.Resources {
		entry := &catalog.Resources[i]
		if entry.Kind == "RuntimeProfile" && entry.Name == defaultRuntimeProfileName {
			profile = entry
		}
		if entry.Kind == "RegistrationToken" && entry.Name == defaultRegistrationTokenName {
			registrationToken = entry
		}
	}
	if profile == nil {
		return fmt.Errorf("local server bootstrap: RuntimeProfile/%s is missing from the catalog", defaultRuntimeProfileName)
	}
	if registrationToken == nil {
		return fmt.Errorf("local server bootstrap: RegistrationToken/%s is missing from the catalog", defaultRegistrationTokenName)
	}
	contractEntries := runtimeContractEntries(catalog, *profile, *registrationToken)
	executable, err := b.Executable()
	if err != nil {
		return err
	}
	tempDir, environment, err := prepareAdminWorkspace(podDir)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	for name, value := range resolved {
		environment = setCommandEnvironment(environment, name, value)
	}
	environment = setCommandEnvironment(environment, "input", "${input}")
	run := b.Run
	if run == nil {
		run = runBootstrapCommand
	}
	for _, entry := range contractEntries {
		if entry.Kind == "RuntimeProfile" {
			if err := b.uploadPetDefPIXAs(ctx, catalog, tempDir, executable, environment, run); err != nil {
				return err
			}
			err := runBootstrapOperation(ctx, run, executable, []string{"admin", "runtime-profiles", "delete", entry.Name, "--context", "local"}, environment)
			if err != nil && !strings.Contains(err.Error(), "RESOURCE_NOT_FOUND:") {
				return fmt.Errorf("local server bootstrap: replace %s/%s: %w", entry.Kind, entry.Name, err)
			}
		}
		file, err := b.extract(catalog, tempDir, entry.Path)
		if err != nil {
			return err
		}
		if err := runBootstrapOperation(ctx, run, executable, []string{"admin", "apply", "--context", "local", "-f", file}, environment); err != nil {
			return fmt.Errorf("local server bootstrap: migrate %s/%s: %w", entry.Kind, entry.Name, err)
		}
	}
	for _, name := range []string{appRegistrationTokenName, legacyRegistrationTokenName} {
		if err := runBootstrapOperation(ctx, run, executable, []string{"admin", "registration-tokens", "delete", name, "--context", "local"}, environment); err != nil && !strings.Contains(err.Error(), "RESOURCE_NOT_FOUND:") {
			return fmt.Errorf("local server bootstrap: retire RegistrationToken/%s: %w", name, err)
		}
	}
	if err := removeLegacyRegistrationTokenFile(podDir); err != nil {
		return err
	}
	return nil
}

func runtimeContractEntries(catalog *Catalog, profile, registrationToken ResourceEntry) []ResourceEntry {
	entries := make([]ResourceEntry, 0, len(catalog.Resources))
	for _, entry := range catalog.Resources {
		if entry.Kind != "RuntimeProfile" && entry.Kind != "RegistrationToken" {
			entries = append(entries, entry)
		}
	}
	return append(entries, profile, registrationToken)
}

func (b *Bootstrapper) runtimeContractEntries(profile, registrationToken ResourceEntry) ([]ResourceEntry, error) {
	catalog, err := b.catalog(context.Background())
	if err != nil {
		return nil, err
	}
	return runtimeContractEntries(catalog, profile, registrationToken), nil
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

// Apply creates the resolved Raids dependencies, RuntimeProfile, and
// RegistrationToken in dependency order.
func (b *Bootstrapper) Apply(ctx context.Context, podDir string, savedEnvironment map[string]string) error {
	if b == nil || b.Executable == nil {
		return fmt.Errorf("local server bootstrap: bootstrapper is not configured")
	}
	catalog, err := b.catalog(ctx)
	if err != nil {
		return err
	}
	executable, err := b.Executable()
	if err != nil {
		return err
	}
	resolved, missing := catalog.ResolveEnvironment(savedEnvironment, os.LookupEnv)
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
		file, err := b.extract(catalog, tempDir, entry.Path)
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
		file, err := b.extractResourceList(catalog, tempDir, listName, entries)
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
	resources := make([]ResourceEntry, 0, len(catalog.Resources))
	runtimeProfiles := make([]ResourceEntry, 0, 1)
	registrationTokens := make([]ResourceEntry, 0, 1)
	for _, entry := range catalog.Resources {
		if entry.Kind == "RuntimeProfile" {
			runtimeProfiles = append(runtimeProfiles, entry)
			continue
		}
		if entry.Kind == "RegistrationToken" {
			registrationTokens = append(registrationTokens, entry)
			continue
		}
		resources = append(resources, entry)
	}
	if err := applyEntries("desktop-bootstrap-resources", resources); err != nil {
		return err
	}
	if err := b.uploadPetDefPIXAs(ctx, catalog, tempDir, executable, environment, run); err != nil {
		return err
	}
	if err := applyEntries("desktop-bootstrap-runtime-profiles", runtimeProfiles); err != nil {
		return err
	}
	if len(registrationTokens) != 1 || registrationTokens[0].Name != defaultRegistrationTokenName {
		return fmt.Errorf("local server bootstrap: expected exactly one RegistrationToken/%s", defaultRegistrationTokenName)
	}
	if err := apply(registrationTokens[0]); err != nil {
		return err
	}
	if err := removeLegacyRegistrationTokenFile(podDir); err != nil {
		return err
	}
	return nil
}

func (b *Bootstrapper) uploadPetDefPIXAs(
	ctx context.Context,
	catalog *Catalog,
	tempDir, executable string,
	environment []string,
	run func(context.Context, string, []string, []string) error,
) error {
	for _, asset := range catalog.PetDefPIXAs {
		file, err := b.extract(catalog, tempDir, asset.PIXA)
		if err != nil {
			return err
		}
		args := []string{"admin", "pet-defs", "upload-pixa", asset.PetDef, "--context", "local", "-f", file}
		if err := runBootstrapOperation(ctx, run, executable, args, environment); err != nil {
			return fmt.Errorf("local server bootstrap: upload PetDef/%s PIXA: %w", asset.PetDef, err)
		}
	}
	return nil
}

func (b *Bootstrapper) catalog(ctx context.Context) (*Catalog, error) {
	if b.Catalog != nil {
		return b.Catalog, nil
	}
	if b.Resolver == nil {
		return nil, errors.New("local server bootstrap: catalog is not configured")
	}
	catalog, err := b.Resolver.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("local server bootstrap: resolve catalog: %w", err)
	}
	return catalog, nil
}

func removeLegacyRegistrationTokenFile(podDir string) error {
	tokenFile := filepath.Join(podDir, "workspace", legacyRegistrationTokenFile)
	if err := os.Remove(tokenFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("local server bootstrap: remove obsolete RegistrationToken handoff: %w", err)
	}
	return nil
}

func (b *Bootstrapper) extractResourceList(catalog *Catalog, root, name string, entries []ResourceEntry) (string, error) {
	items := make([]any, 0, len(entries))
	for _, entry := range entries {
		data, err := fs.ReadFile(catalog.FS, entry.Path)
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

func (b *Bootstrapper) extract(catalog *Catalog, root, name string) (string, error) {
	data, err := fs.ReadFile(catalog.FS, name)
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
