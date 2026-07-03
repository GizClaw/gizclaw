package contextstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

const currentLink = "current"

// Store manages a context root directory.
type Store struct {
	Root string
}

// CreateOptions holds optional settings for a newly-created context.
type CreateOptions struct {
	Description     string
	ServerPublicKey string
}

// Create creates a new context directory with a generated key pair and config.
func (s *Store) Create(name, endpoint, serverPublicKey string) error {
	return s.CreateWithOptions(name, endpoint, CreateOptions{ServerPublicKey: serverPublicKey})
}

// CreateWithOptions creates a new context directory with a generated key pair and config.
func (s *Store) CreateWithOptions(name, endpoint string, opts CreateOptions) error {
	if err := validateName(name); err != nil {
		return err
	}
	if err := validateEndpoint("server.endpoint", endpoint); err != nil {
		return err
	}
	dir := filepath.Join(s.Root, name)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("contextstore: %q already exists", name)
	}
	publicKeyText := strings.TrimSpace(opts.ServerPublicKey)
	if publicKeyText == "" {
		return fmt.Errorf("contextstore: missing server public key")
	}
	var serverPublicKey giznet.PublicKey
	if err := serverPublicKey.UnmarshalText([]byte(publicKeyText)); err != nil {
		return fmt.Errorf("contextstore: invalid server public key: %w", err)
	}
	if serverPublicKey.IsZero() {
		return fmt.Errorf("contextstore: invalid server public key: zero key")
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("contextstore: mkdir: %w", err)
	}
	if _, err := LoadIdentityOrGenerate(filepath.Join(dir, IdentityFile)); err != nil {
		return fmt.Errorf("contextstore: generate key: %w", err)
	}

	cfg := Config{
		Description: strings.TrimSpace(opts.Description),
		Server: ServerConfig{
			Endpoint:  endpoint,
			PublicKey: serverPublicKey,
		},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("contextstore: marshal config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), data, 0o600); err != nil {
		return fmt.Errorf("contextstore: write config: %w", err)
	}

	link := filepath.Join(s.Root, currentLink)
	if _, err := os.Lstat(link); os.IsNotExist(err) {
		if err := os.Symlink(name, link); err != nil {
			return fmt.Errorf("contextstore: symlink current: %w", err)
		}
	}
	return nil
}

// Use switches the current context by updating the symlink.
func (s *Store) Use(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	dir := filepath.Join(s.Root, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("contextstore: %q does not exist", name)
	}
	link := filepath.Join(s.Root, currentLink)
	_ = os.Remove(link)
	if err := os.Symlink(name, link); err != nil {
		return fmt.Errorf("contextstore: symlink: %w", err)
	}
	return nil
}

// Delete removes a context by name. If the deleted context is current, the
// current symlink is removed and no replacement context is selected.
func (s *Store) Delete(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	dir := filepath.Join(s.Root, name)
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return fmt.Errorf("contextstore: %q does not exist", name)
	}
	if err != nil {
		return fmt.Errorf("contextstore: stat %q: %w", name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("contextstore: %q is not a context directory", name)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("contextstore: delete %q: %w", name, err)
	}
	link := filepath.Join(s.Root, currentLink)
	if target, err := os.Readlink(link); err == nil && filepath.Base(filepath.Clean(target)) == name {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("contextstore: remove current link: %w", err)
		}
	}
	return nil
}

// Current returns the currently active context, or nil if none is set.
func (s *Store) Current() (*Context, error) {
	link := filepath.Join(s.Root, currentLink)
	target, err := os.Readlink(link)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("contextstore: readlink: %w", err)
	}
	dir := target
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(s.Root, dir)
	}
	return Load(dir)
}

// LoadByName loads a context by its plain name.
func (s *Store) LoadByName(name string) (*Context, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.Root, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("contextstore: %q does not exist", name)
	}
	return Load(dir)
}

// List returns the names of all contexts, sorted alphabetically.
// The returned current name is empty if no current context is set.
func (s *Store) List() (names []string, current string, err error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("contextstore: readdir: %w", err)
	}
	link := filepath.Join(s.Root, currentLink)
	if target, err := os.Readlink(link); err == nil {
		current = filepath.Base(filepath.Clean(target))
	}
	for _, e := range entries {
		if e.Name() == currentLink || !e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, current, nil
}

// ListSummaries returns context metadata for list UIs.
func (s *Store) ListSummaries() ([]Summary, error) {
	names, current, err := s.List()
	if err != nil {
		return nil, err
	}
	out := make([]Summary, 0, len(names))
	for _, name := range names {
		summary, err := LoadSummary(filepath.Join(s.Root, name))
		if err != nil {
			return nil, err
		}
		summary.Current = name == current
		out = append(out, summary)
	}
	return out, nil
}

func validateName(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return fmt.Errorf("contextstore: invalid name %q", name)
	}
	return nil
}
