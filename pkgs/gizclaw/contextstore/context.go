package contextstore

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

const ConfigFile = "config.yaml"

// ServerConfig holds the connection info for a remote server.
type ServerConfig struct {
	Endpoint string `yaml:"endpoint"`
}

// IdentityConfig holds the local identity material for this context.
type IdentityConfig struct {
	PrivateKey giznet.Key `yaml:"private-key"`
}

// Config is the per-context configuration stored in config.yaml.
type Config struct {
	Description string         `yaml:"description,omitempty"`
	Identity    IdentityConfig `yaml:"identity"`
	Server      ServerConfig   `yaml:"server"`
}

// Context represents a loaded context directory.
type Context struct {
	Name    string
	Dir     string
	Config  Config
	KeyPair *giznet.KeyPair
}

// Summary is the lightweight context metadata used by list UIs and e2e harnesses.
type Summary struct {
	Name           string
	Description    string
	Current        bool
	Endpoint       string
	LocalPublicKey giznet.PublicKey
}

// Load reads a context from its directory.
func Load(dir string) (*Context, error) {
	cfg, err := LoadConfig(dir)
	if err != nil {
		return nil, err
	}
	kp, err := keyPairFromPrivateKey("identity.private-key", cfg.Identity.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &Context{
		Name:    filepath.Base(dir),
		Dir:     dir,
		Config:  cfg,
		KeyPair: kp,
	}, nil
}

// LoadSummary reads context metadata.
func LoadSummary(dir string) (Summary, error) {
	ctx, err := LoadConfig(dir)
	if err != nil {
		return Summary{}, err
	}
	kp, err := keyPairFromPrivateKey("identity.private-key", ctx.Identity.PrivateKey)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{
		Name:           filepath.Base(dir),
		Description:    ctx.Description,
		Endpoint:       ctx.Server.Endpoint,
		LocalPublicKey: kp.Public,
	}
	return summary, nil
}

// LoadConfig reads and validates config.yaml from a context directory.
func LoadConfig(dir string) (Config, error) {
	data, err := os.ReadFile(filepath.Join(dir, ConfigFile))
	if err != nil {
		return Config{}, fmt.Errorf("contextstore: read config: %w", err)
	}
	var cfg Config
	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.DisallowUnknownField()); err != nil {
		return Config{}, fmt.Errorf("contextstore: parse config: %w", err)
	}
	if err := validateEndpoint("server.endpoint", cfg.Server.Endpoint); err != nil {
		return Config{}, err
	}
	kp, err := keyPairFromPrivateKey("identity.private-key", cfg.Identity.PrivateKey)
	if err != nil {
		return Config{}, err
	}
	cfg.Identity.PrivateKey = kp.Private
	return cfg, nil
}

func keyPairFromPrivateKey(field string, privateKey giznet.Key) (*giznet.KeyPair, error) {
	if privateKey.IsZero() {
		return nil, fmt.Errorf("contextstore: missing %s", field)
	}
	kp, err := giznet.NewKeyPair(privateKey)
	if err != nil {
		return nil, fmt.Errorf("contextstore: invalid %s: %w", field, err)
	}
	return kp, nil
}

func validateEndpoint(field, endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("contextstore: missing %s", field)
	}
	if strings.Contains(endpoint, "://") {
		return fmt.Errorf("contextstore: %s must be host:port, got %q", field, endpoint)
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("contextstore: invalid %s: %w", field, err)
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("contextstore: %s host is empty", field)
	}
	if strings.TrimSpace(port) == "" {
		return fmt.Errorf("contextstore: %s port is empty", field)
	}
	return nil
}

// PublicAPIAddr returns the HTTP endpoint host:port.
func (s ServerConfig) PublicAPIAddr() string {
	return s.Endpoint
}
