package clicontext

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/identity"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/giznoise"
	"github.com/goccy/go-yaml"
)

// ServerConfig holds the connection info for a remote server.
type ServerConfig struct {
	Address       string              `yaml:"address,omitempty"`
	Host          string              `yaml:"host,omitempty"`
	PublicAPIPort int                 `yaml:"public-api-port,omitempty"`
	NoiseUDPPort  int                 `yaml:"noise-udp-port,omitempty"`
	ICEPort       int                 `yaml:"ice-port,omitempty"`
	Transport     string              `yaml:"transport,omitempty"`
	PublicKey     giznet.PublicKey    `yaml:"-"`
	CipherMode    giznoise.CipherMode `yaml:"cipher-mode,omitempty"`
}

// Config is the per-cli-context configuration stored in config.yaml.
type Config struct {
	Server ServerConfig `yaml:"server"`
}

// CLIContext represents a loaded CLI context directory.
type CLIContext struct {
	Name    string
	Dir     string
	Config  Config
	KeyPair *giznet.KeyPair
}

// Load reads a CLI context from its directory.
func Load(dir string) (*CLIContext, error) {
	name := filepath.Base(dir)

	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("clicontext: read config: %w", err)
	}
	var keyCheck struct {
		Server map[string]any `yaml:"server"`
	}
	if err := yaml.Unmarshal(data, &keyCheck); err != nil {
		return nil, fmt.Errorf("clicontext: parse config: %w", err)
	}
	if _, ok := keyCheck.Server["private-key"]; ok {
		return nil, fmt.Errorf("clicontext: server.private-key is not supported; use server.public-key")
	}
	if _, ok := keyCheck.Server["identity-key"]; ok {
		return nil, fmt.Errorf("clicontext: server.identity-key is not supported; use server.public-key")
	}
	var raw struct {
		Server struct {
			Address       string              `yaml:"address"`
			Host          string              `yaml:"host"`
			PublicAPIPort int                 `yaml:"public-api-port"`
			NoiseUDPPort  int                 `yaml:"noise-udp-port"`
			ICEPort       int                 `yaml:"ice-port"`
			Transport     string              `yaml:"transport"`
			PublicKey     giznet.PublicKey    `yaml:"public-key"`
			CipherMode    giznoise.CipherMode `yaml:"cipher-mode"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("clicontext: parse config: %w", err)
	}
	if raw.Server.PublicKey.IsZero() {
		return nil, fmt.Errorf("clicontext: missing server.public-key")
	}
	if err := validateCipherMode(raw.Server.CipherMode); err != nil {
		return nil, err
	}
	host, publicPort, noisePort, icePort, err := normalizeServerEndpoint(raw.Server.Address, raw.Server.Host, raw.Server.PublicAPIPort, raw.Server.NoiseUDPPort, raw.Server.ICEPort)
	if err != nil {
		return nil, err
	}
	transport := raw.Server.Transport
	if transport == "" {
		transport = "noise"
	}
	if transport != "noise" && transport != "webrtc" {
		return nil, fmt.Errorf("clicontext: unsupported transport %q", transport)
	}
	cfg := Config{
		Server: ServerConfig{
			Address:       net.JoinHostPort(host, strconv.Itoa(noisePort)),
			Host:          host,
			PublicAPIPort: publicPort,
			NoiseUDPPort:  noisePort,
			ICEPort:       icePort,
			Transport:     transport,
			PublicKey:     raw.Server.PublicKey,
			CipherMode:    raw.Server.CipherMode,
		},
	}

	kp, err := identity.Load(filepath.Join(dir, "identity.key"))
	if err != nil {
		return nil, fmt.Errorf("clicontext: load identity: %w", err)
	}

	return &CLIContext{Name: name, Dir: dir, Config: cfg, KeyPair: kp}, nil
}

// ServerPublicKey parses and returns the server's public key.
func (c *CLIContext) ServerPublicKey() (giznet.PublicKey, error) {
	return c.Config.Server.PublicKey, nil
}

func (s ServerConfig) PublicAPIAddr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.PublicAPIPort))
}

func (s ServerConfig) NoiseUDPAddr() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.NoiseUDPPort))
}

func (s ServerConfig) SignalingURL() string {
	return "http://" + s.PublicAPIAddr() + "/giznet/webrtc/v1/offer"
}

func normalizeServerEndpoint(address, host string, publicPort, noisePort, icePort int) (string, int, int, int, error) {
	if host == "" && address != "" {
		addrHost, addrPort, err := net.SplitHostPort(address)
		if err != nil {
			if strings.Contains(address, ":") {
				return "", 0, 0, 0, fmt.Errorf("clicontext: invalid server address: %w", err)
			}
			host = address
		} else {
			port, err := strconv.Atoi(addrPort)
			if err != nil {
				return "", 0, 0, 0, fmt.Errorf("clicontext: invalid server port: %w", err)
			}
			host = addrHost
			if publicPort == 0 {
				publicPort = port
			}
			if noisePort == 0 {
				noisePort = port
			}
		}
	}
	if host == "" {
		return "", 0, 0, 0, fmt.Errorf("clicontext: missing server.host")
	}
	if publicPort == 0 {
		publicPort = 9820
	}
	if noisePort == 0 {
		noisePort = 9820
	}
	if icePort == 0 {
		icePort = 9821
	}
	for name, port := range map[string]int{"public-api-port": publicPort, "noise-udp-port": noisePort, "ice-port": icePort} {
		if port < 1 || port > 65535 {
			return "", 0, 0, 0, fmt.Errorf("clicontext: server.%s must be between 1 and 65535", name)
		}
	}
	return host, publicPort, noisePort, icePort, nil
}
