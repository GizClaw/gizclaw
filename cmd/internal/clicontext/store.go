package clicontext

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/identity"
	"github.com/GizClaw/gizclaw-go/cmd/internal/paths"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/giznoise"
	"github.com/goccy/go-yaml"
)

const currentLink = "current"

// Store manages the CLI context root directory.
type Store struct {
	Root string
}

// CreateOptions holds optional settings for a newly created CLI context.
type CreateOptions struct {
	CipherMode      giznoise.CipherMode
	ServerPublicKey string
	Transport       string
	PublicAPIPort   int
	NoiseUDPPort    int
	ICEPort         int
}

// DefaultStore returns a Store under the gizclaw config directory.
func DefaultStore() (*Store, error) {
	root, err := paths.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("clicontext: config dir: %w", err)
	}
	return &Store{Root: root}, nil
}

// Create creates a new CLI context directory with a generated key pair and config.
func (s *Store) Create(name, serverAddr, serverPublicKey string) error {
	return s.CreateWithOptions(name, serverAddr, CreateOptions{ServerPublicKey: serverPublicKey})
}

// CreateWithOptions creates a new CLI context directory with a generated key pair and config.
func (s *Store) CreateWithOptions(name, serverAddr string, opts CreateOptions) error {
	if err := validateName(name); err != nil {
		return err
	}
	if err := validateCipherMode(opts.CipherMode); err != nil {
		return err
	}
	transport := opts.Transport
	if transport == "" {
		transport = "noise"
	}
	if transport != "noise" && transport != "webrtc" {
		return fmt.Errorf("clicontext: unsupported transport %q", transport)
	}
	dir := filepath.Join(s.Root, name)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("clicontext: %q already exists", name)
	}
	publicKeyText := strings.TrimSpace(opts.ServerPublicKey)
	if publicKeyText == "" {
		return fmt.Errorf("clicontext: missing server public key")
	}
	var serverPublicKey giznet.PublicKey
	if err := serverPublicKey.UnmarshalText([]byte(publicKeyText)); err != nil {
		return fmt.Errorf("clicontext: invalid server public key: %w", err)
	}
	if serverPublicKey.IsZero() {
		return fmt.Errorf("clicontext: invalid server public key: zero key")
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("clicontext: mkdir: %w", err)
	}

	if _, err := identity.LoadOrGenerate(filepath.Join(dir, "identity.key")); err != nil {
		return fmt.Errorf("clicontext: generate key: %w", err)
	}

	host, publicPort, noisePort, icePort, err := normalizeServerEndpoint(serverAddr, "", opts.PublicAPIPort, opts.NoiseUDPPort, opts.ICEPort)
	if err != nil {
		return err
	}

	cfg := struct {
		Server struct {
			Host          string              `yaml:"host"`
			PublicAPIPort int                 `yaml:"public-api-port"`
			NoiseUDPPort  int                 `yaml:"noise-udp-port"`
			ICEPort       int                 `yaml:"ice-port"`
			PublicKey     giznet.PublicKey    `yaml:"public-key"`
			Transport     string              `yaml:"transport"`
			CipherMode    giznoise.CipherMode `yaml:"cipher-mode,omitempty"`
		} `yaml:"server"`
	}{}
	cfg.Server.Host = host
	cfg.Server.PublicAPIPort = publicPort
	cfg.Server.NoiseUDPPort = noisePort
	cfg.Server.ICEPort = icePort
	cfg.Server.PublicKey = serverPublicKey
	cfg.Server.Transport = transport
	cfg.Server.CipherMode = opts.CipherMode
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("clicontext: marshal config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), data, 0o600); err != nil {
		return fmt.Errorf("clicontext: write config: %w", err)
	}

	link := filepath.Join(s.Root, currentLink)
	if _, err := os.Lstat(link); os.IsNotExist(err) {
		_ = os.Symlink(name, link)
	}

	return nil
}

func validateCipherMode(mode giznoise.CipherMode) error {
	switch mode {
	case "", giznoise.CipherModeChaChaPoly, giznoise.CipherModeAES256GCM, giznoise.CipherModePlaintext:
		return nil
	default:
		return fmt.Errorf("clicontext: unsupported cipher-mode %q", mode)
	}
}

// Use switches the current CLI context by updating the symlink.
func (s *Store) Use(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	dir := filepath.Join(s.Root, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("clicontext: %q does not exist", name)
	}

	link := filepath.Join(s.Root, currentLink)
	_ = os.Remove(link)
	if err := os.Symlink(name, link); err != nil {
		return fmt.Errorf("clicontext: symlink: %w", err)
	}
	return nil
}

// Delete removes a CLI context by name. If the deleted context is current, the
// current symlink is removed and no replacement context is selected.
func (s *Store) Delete(name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	dir := filepath.Join(s.Root, name)
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return fmt.Errorf("clicontext: %q does not exist", name)
	}
	if err != nil {
		return fmt.Errorf("clicontext: stat %q: %w", name, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("clicontext: %q is not a context directory", name)
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clicontext: delete %q: %w", name, err)
	}
	link := filepath.Join(s.Root, currentLink)
	if target, err := os.Readlink(link); err == nil && filepath.Base(filepath.Clean(target)) == name {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("clicontext: remove current link: %w", err)
		}
	}
	return nil
}

// Current returns the currently active CLI context, or nil if none is set.
func (s *Store) Current() (*CLIContext, error) {
	link := filepath.Join(s.Root, currentLink)
	target, err := os.Readlink(link)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("clicontext: readlink: %w", err)
	}

	dir := target
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(s.Root, dir)
	}
	return Load(dir)
}

// LoadByName loads a CLI context by its plain name (no path separators allowed).
func (s *Store) LoadByName(name string) (*CLIContext, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.Root, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("clicontext: %q does not exist", name)
	}
	return Load(dir)
}

// List returns the names of all CLI contexts, sorted alphabetically.
// The returned current name is empty if no current is set.
func (s *Store) List() (names []string, current string, err error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("clicontext: readdir: %w", err)
	}

	link := filepath.Join(s.Root, currentLink)
	if target, err := os.Readlink(link); err == nil {
		current = filepath.Base(filepath.Clean(target))
	}

	for _, e := range entries {
		if e.Name() == currentLink {
			continue
		}
		if !e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, current, nil
}

func validateName(name string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || name == "." || name == ".." {
		return fmt.Errorf("clicontext: invalid name %q", name)
	}
	return nil
}
