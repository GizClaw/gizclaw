package edge

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/goccy/go-yaml"
)

const workspaceConfigFile = "config.yaml"

const (
	TLSCertSourceDisabled = "disabled"
	TLSCertSourceEdgeRPC  = "edge-rpc"
	TLSCertSourceFile     = "file"
)

type Config struct {
	KeyPair  *giznet.KeyPair
	Listen   string
	Endpoint string
	Upstream UpstreamConfig
	TLS      TLSConfig
}

type IdentityConfig struct {
	PrivateKey giznet.Key `yaml:"private-key"`
}

type UpstreamConfig struct {
	Endpoint  string           `yaml:"endpoint"`
	PublicKey giznet.PublicKey `yaml:"public-key"`
}

type TLSConfig struct {
	CertSource string `yaml:"cert-source"`
}

type ConfigFile struct {
	Identity IdentityConfig `yaml:"identity"`
	Listen   string         `yaml:"listen"`
	Endpoint string         `yaml:"endpoint"`
	Upstream UpstreamConfig `yaml:"upstream"`
	TLS      TLSConfig      `yaml:"tls"`
}

func LoadConfig(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	return parseConfigData(data)
}

func parseConfigData(data []byte) (ConfigFile, error) {
	var raw ConfigFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ConfigFile{}, err
	}
	if raw.TLS.CertSource == "" {
		raw.TLS.CertSource = TLSCertSourceDisabled
	}
	return raw, nil
}

func DefaultConfig() Config {
	return Config{
		Listen:   "0.0.0.0:9821",
		Endpoint: "0.0.0.0:9821",
		TLS: TLSConfig{
			CertSource: TLSCertSourceDisabled,
		},
	}
}

func PrepareWorkspaceConfig(root string) (Config, error) {
	fileCfg, err := LoadConfig(filepath.Join(root, workspaceConfigFile))
	if err != nil {
		return Config{}, fmt.Errorf("edge: load config: %w", err)
	}
	return prepareConfig(Config{}, fileCfg)
}

func prepareConfig(cfg Config, fileCfg ConfigFile) (Config, error) {
	if cfg.Listen == "" {
		cfg.Listen = fileCfg.Listen
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = fileCfg.Endpoint
	}
	if cfg.Upstream.Endpoint == "" {
		cfg.Upstream.Endpoint = fileCfg.Upstream.Endpoint
	}
	if cfg.Upstream.PublicKey.IsZero() {
		cfg.Upstream.PublicKey = fileCfg.Upstream.PublicKey
	}
	if cfg.TLS.CertSource == "" || cfg.TLS.CertSource == TLSCertSourceDisabled {
		cfg.TLS = fileCfg.TLS
	}
	if cfg.TLS.CertSource == "" {
		cfg.TLS.CertSource = TLSCertSourceDisabled
	}
	if fileCfg.Identity.PrivateKey.IsZero() {
		return Config{}, fmt.Errorf("edge: invalid identity.private-key: zero key")
	}
	keyPair, err := giznet.NewKeyPair(fileCfg.Identity.PrivateKey)
	if err != nil {
		return Config{}, fmt.Errorf("edge: invalid identity.private-key: %w", err)
	}
	cfg.KeyPair = keyPair
	if cfg.Listen == "" {
		cfg.Listen = DefaultConfig().Listen
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = cfg.Listen
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg Config) validate() error {
	if cfg.KeyPair == nil {
		return fmt.Errorf("edge: missing identity.private-key")
	}
	if cfg.Listen == "" {
		return fmt.Errorf("edge: missing listen")
	}
	if cfg.Endpoint == "" {
		return fmt.Errorf("edge: missing endpoint")
	}
	if cfg.Upstream.Endpoint == "" {
		return fmt.Errorf("edge: missing upstream.endpoint")
	}
	if _, err := cfg.UpstreamURL(); err != nil {
		return err
	}
	switch cfg.TLS.CertSource {
	case TLSCertSourceDisabled, TLSCertSourceEdgeRPC, TLSCertSourceFile:
		return nil
	default:
		return fmt.Errorf("edge: invalid tls.cert-source %q", cfg.TLS.CertSource)
	}
}

func (cfg Config) UpstreamURL() (*url.URL, error) {
	endpoint := strings.TrimSpace(cfg.Upstream.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("edge: missing upstream.endpoint")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	upstreamURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("edge: invalid upstream.endpoint: %w", err)
	}
	if upstreamURL.Scheme != "http" && upstreamURL.Scheme != "https" {
		return nil, fmt.Errorf("edge: invalid upstream.endpoint scheme %q", upstreamURL.Scheme)
	}
	if upstreamURL.Host == "" {
		return nil, fmt.Errorf("edge: invalid upstream.endpoint: missing host")
	}
	return upstreamURL, nil
}
