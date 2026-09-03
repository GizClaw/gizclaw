package contextstore

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

const ConfigFile = "config.yaml"

// ServerConfig holds the connection info for a remote server. Endpoint is an
// http or https base URL such as "http://127.0.0.1:9820" or
// "https://ap.gizclaw.com"; the scheme selects the HTTP access lane, and the
// WebRTC ICE UDP endpoint comes from /server-info instead. A bare host:port is
// accepted and normalized to http.
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
	normalized, err := normalizeServerURL("server.endpoint", cfg.Server.Endpoint)
	if err != nil {
		return Config{}, err
	}
	cfg.Server.Endpoint = normalized
	kp, kpErr := keyPairFromPrivateKey("identity.private-key", cfg.Identity.PrivateKey)
	if kpErr != nil {
		return Config{}, kpErr
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

// normalizeServerURL validates an http or https base URL and returns it
// without a trailing slash. A value with no scheme defaults to http.
func normalizeServerURL(field, endpoint string) (string, error) {
	value := strings.TrimSpace(endpoint)
	if value == "" {
		return "", fmt.Errorf("contextstore: missing %s", field)
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("contextstore: invalid %s: %w", field, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf(
			"contextstore: %s must be an http:// or https:// URL, got %q",
			field,
			endpoint,
		)
	}
	if parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf(
			"contextstore: %s must be http://host[:port] or https://host[:port], got %q",
			field,
			endpoint,
		)
	}
	path := strings.TrimRight(parsed.Path, "/")
	if strings.Contains(path, "//") {
		return "", fmt.Errorf("contextstore: %s path must not contain empty segments", field)
	}
	base := url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: path}
	return base.String(), nil
}

// BaseURL returns the absolute HTTP base URL of the Server access point.
func (s ServerConfig) BaseURL() string {
	return s.Endpoint
}
