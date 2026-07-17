package localserver

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Bootstrapper applies a validated catalog through the packaged companion CLI.
type Bootstrapper struct {
	Catalog    *Catalog
	Executable func() (string, error)
	Run        func(context.Context, string, []string, []string) error
}

// Apply creates every declarative resource, synchronizes dynamic voice
// resources, and uploads owner-managed assets to one newly started local Server.
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
		if err := run(ctx, executable, args, environment); err != nil {
			return fmt.Errorf("local server bootstrap: apply %s/%s from %s: %w", entry.Kind, entry.Name, entry.Path, err)
		}
		return nil
	}
	for _, entry := range b.Catalog.Resources {
		if strings.Contains(entry.Path, "/90-acl/") {
			continue
		}
		if err := apply(entry); err != nil {
			return err
		}
	}
	for _, item := range b.Catalog.VoiceSyncs {
		args := []string{"admin", item.Provider + "-tenants", "sync-voices", item.Tenant, "--context", "local"}
		if err := run(ctx, executable, args, environment); err != nil {
			return fmt.Errorf("local server bootstrap: sync %s voices for %s: %w", item.Provider, item.Tenant, err)
		}
	}
	for _, entry := range b.Catalog.Resources {
		if !strings.Contains(entry.Path, "/90-acl/") {
			continue
		}
		if err := apply(entry); err != nil {
			return err
		}
	}
	for _, icon := range b.Catalog.WorkflowIcons {
		for _, asset := range []struct {
			format string
			path   string
		}{{format: "png", path: icon.PNG}, {format: "pixa", path: icon.PIXA}} {
			file, err := b.extract(tempDir, asset.path)
			if err != nil {
				return err
			}
			args := []string{"admin", "workflows", "upload-icon", icon.Workflow, "--format", asset.format, "--context", "local", "-f", file}
			if err := run(ctx, executable, args, environment); err != nil {
				return fmt.Errorf("local server bootstrap: upload Workflow/%s %s icon: %w", icon.Workflow, asset.format, err)
			}
		}
	}
	for _, asset := range b.Catalog.PetDefPIXAs {
		file, err := b.extract(tempDir, asset.PIXA)
		if err != nil {
			return err
		}
		args := []string{"admin", "pet-defs", "upload-pixa", asset.PetDef, "--context", "local", "-f", file}
		if err := run(ctx, executable, args, environment); err != nil {
			return fmt.Errorf("local server bootstrap: upload PetDef/%s PIXA: %w", asset.PetDef, err)
		}
	}
	return nil
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
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
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
	prefix := name + "="
	for i, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			environment[i] = prefix + value
			return environment
		}
	}
	return append(environment, prefix+value)
}
