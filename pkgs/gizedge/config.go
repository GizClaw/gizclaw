package gizedge

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/goccy/go-yaml"
)

const workspaceConfigFile = "config.yaml"

const (
	TLSCertSourceDisabled = "disabled"
	TLSCertSourceEdgeRPC  = "edge-rpc"
	TLSCertSourceFile     = "file"
)

type Config struct {
	KeyPair   *giznet.KeyPair
	Listen    string
	Endpoint  string
	Upstream  UpstreamConfig
	TLS       TLSConfig
	TURN      TURNConfig
	Gateway   GatewayConfig
	Metrics   MetricsConfig
	Storage   map[string]storage.Config
	Stores    map[string]store.Config
	SystemLog gizlog.Config
}

type IdentityConfig struct {
	PrivateKey giznet.Key `yaml:"private-key"`
}

type UpstreamConfig struct {
	Endpoint           string                `yaml:"endpoint"`
	PublicKey          giznet.PublicKey      `yaml:"public-key"`
	ICETransportPolicy string                `yaml:"ice-transport-policy"`
	ICEServers         []gizwebrtc.ICEServer `yaml:"ice-servers"`
}

type TLSConfig struct {
	CertSource string `yaml:"cert-source"`
}

type TURNConfig struct {
	Listen         string `yaml:"listen"`
	PublicEndpoint string `yaml:"public-endpoint"`
	RelayAddress   string `yaml:"relay-address"`
	Realm          string `yaml:"realm"`
	Username       string `yaml:"username"`
	Credential     string `yaml:"credential"`
	RelayMinPort   uint16 `yaml:"relay-min-port"`
	RelayMaxPort   uint16 `yaml:"relay-max-port"`
}

type GatewayConfig struct {
	Enabled              bool          `yaml:"enabled"`
	MaxSessions          int           `yaml:"max-sessions"`
	MaxUpstreams         int           `yaml:"max-upstreams"`
	SessionsPerUpstream  int           `yaml:"sessions-per-upstream"`
	ChannelsPerSession   int           `yaml:"channels-per-session"`
	ChannelsPerUpstream  int           `yaml:"channels-per-upstream"`
	MaxPendingHandshakes int           `yaml:"max-pending-handshakes"`
	SessionBufferBytes   int64         `yaml:"session-buffer-bytes"`
	IdleTimeout          time.Duration `yaml:"idle-timeout"`
	DrainTimeout         time.Duration `yaml:"drain-timeout"`
}

// MetricsConfig configures the standalone Edge process metrics backend.
// Metrics are disabled when all fields are empty.
type MetricsConfig struct {
	RemoteWriteURL string `yaml:"remote-write-url"`
	QueryURL       string `yaml:"query-url"`
	BearerToken    string `yaml:"bearer-token"`
}

type storageFileConfig struct {
	Kind            string `yaml:"kind"`
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	AccessKeyID     string `yaml:"access_key_id"`
	AccessKeySecret string `yaml:"access_key_secret"`
}

func (cfg storageFileConfig) runtimeConfig() (storage.Config, error) {
	if cfg.Kind != storage.KindVolcTLS {
		return nil, fmt.Errorf("only %s is supported", storage.KindVolcTLS)
	}
	return storage.VolcTLSConfig{
		Endpoint:        os.ExpandEnv(cfg.Endpoint),
		Region:          os.ExpandEnv(cfg.Region),
		AccessKeyID:     os.ExpandEnv(cfg.AccessKeyID),
		AccessKeySecret: os.ExpandEnv(cfg.AccessKeySecret),
	}, nil
}

type storeFileConfig struct {
	Kind    string `yaml:"kind"`
	Storage string `yaml:"storage"`
	TopicID string `yaml:"topic_id"`
}

func (cfg storeFileConfig) runtimeConfig() (store.Config, error) {
	if cfg.Kind != store.KindLogImmutable {
		return store.Config{}, fmt.Errorf("only %s is supported", store.KindLogImmutable)
	}
	return store.Config{
		Kind: cfg.Kind, Storage: cfg.Storage, TopicID: os.ExpandEnv(cfg.TopicID),
	}, nil
}

type ConfigFile struct {
	Identity  IdentityConfig               `yaml:"identity"`
	Listen    string                       `yaml:"listen"`
	Endpoint  string                       `yaml:"endpoint"`
	Upstream  UpstreamConfig               `yaml:"upstream"`
	TLS       TLSConfig                    `yaml:"tls"`
	TURN      TURNConfig                   `yaml:"turn"`
	Gateway   GatewayConfig                `yaml:"gateway"`
	Metrics   MetricsConfig                `yaml:"metrics"`
	Storage   map[string]storageFileConfig `yaml:"storage"`
	Stores    map[string]storeFileConfig   `yaml:"stores"`
	SystemLog *gizlog.Config               `yaml:"system-log"`
}

func LoadConfig(path string) (ConfigFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	return parseConfigData(data)
}

func parseConfigData(data []byte) (ConfigFile, error) {
	if err := rejectRetiredGatewayConfig(data); err != nil {
		return ConfigFile{}, err
	}
	var raw ConfigFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ConfigFile{}, err
	}
	if raw.TLS.CertSource == "" {
		raw.TLS.CertSource = TLSCertSourceDisabled
	}
	if raw.SystemLog != nil {
		prepared, err := gizlog.PrepareConfigAt("system-log", *raw.SystemLog)
		if err != nil {
			return ConfigFile{}, fmt.Errorf("edge: %w", err)
		}
		raw.SystemLog = &prepared
	}
	return raw, nil
}

func rejectRetiredGatewayConfig(data []byte) error {
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	gateway, ok := document["gateway"].(map[string]any)
	if !ok {
		return nil
	}
	retiredSettings := []struct {
		key     string
		message string
	}{
		{key: "ice-udp-listen", message: "was removed; use top-level listen"},
		{key: "public-ice-udp", message: "was removed; use top-level endpoint"},
		{key: "streams-per-upstream", message: "is retired; use gateway.channels-per-upstream"},
		{key: "delegated-envelope-validity", message: "is retired"},
	}
	for _, retired := range retiredSettings {
		if _, exists := gateway[retired.key]; exists {
			return fmt.Errorf("edge: gateway.%s %s", retired.key, retired.message)
		}
	}
	return nil
}

func DefaultConfig() Config {
	return Config{
		Listen:   "0.0.0.0:9821",
		Endpoint: "0.0.0.0:9821",
		TLS: TLSConfig{
			CertSource: TLSCertSourceDisabled,
		},
		Gateway: defaultGatewayConfig(),
	}
}

func defaultGatewayConfig() GatewayConfig {
	return GatewayConfig{
		MaxSessions:          30000,
		MaxUpstreams:         16,
		SessionsPerUpstream:  2048,
		ChannelsPerSession:   32,
		ChannelsPerUpstream:  8192,
		MaxPendingHandshakes: 64,
		SessionBufferBytes:   1 << 20,
		IdleTimeout:          5 * time.Minute,
		DrainTimeout:         30 * time.Second,
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
	if cfg.Upstream.ICETransportPolicy == "" {
		cfg.Upstream.ICETransportPolicy = fileCfg.Upstream.ICETransportPolicy
	}
	if len(cfg.Upstream.ICEServers) == 0 {
		cfg.Upstream.ICEServers = append([]gizwebrtc.ICEServer(nil), fileCfg.Upstream.ICEServers...)
	}
	if cfg.TLS.CertSource == "" || cfg.TLS.CertSource == TLSCertSourceDisabled {
		cfg.TLS = fileCfg.TLS
	}
	if cfg.TURN.Listen == "" {
		cfg.TURN = fileCfg.TURN
	}
	cfg.Gateway = mergeGatewayConfig(cfg.Gateway, fileCfg.Gateway)
	if cfg.Metrics.RemoteWriteURL == "" {
		cfg.Metrics.RemoteWriteURL = fileCfg.Metrics.RemoteWriteURL
	}
	if cfg.Metrics.QueryURL == "" {
		cfg.Metrics.QueryURL = fileCfg.Metrics.QueryURL
	}
	if cfg.Metrics.BearerToken == "" {
		cfg.Metrics.BearerToken = fileCfg.Metrics.BearerToken
	}
	if len(cfg.Storage) == 0 {
		var err error
		cfg.Storage, err = runtimeStorageConfigs(fileCfg.Storage)
		if err != nil {
			return Config{}, err
		}
	}
	if len(cfg.Stores) == 0 {
		var err error
		cfg.Stores, err = runtimeStoreConfigs(fileCfg.Stores)
		if err != nil {
			return Config{}, err
		}
	}
	if cfg.SystemLog.IsZero() && fileCfg.SystemLog != nil {
		cfg.SystemLog = *fileCfg.SystemLog
	}
	preparedLog, err := gizlog.PrepareConfigAt("system-log", cfg.SystemLog)
	if err != nil {
		return Config{}, fmt.Errorf("edge: %w", err)
	}
	cfg.SystemLog = preparedLog
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
	cfg.Gateway = applyGatewayDefaults(cfg.Gateway)
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
	if _, _, err := netSplitNumericHostPort("listen", cfg.Listen); err != nil {
		return err
	}
	if _, _, err := netSplitNumericHostPort("endpoint", cfg.Endpoint); err != nil {
		return err
	}
	if cfg.Upstream.Endpoint == "" {
		return fmt.Errorf("edge: missing upstream.endpoint")
	}
	if cfg.Upstream.PublicKey.IsZero() {
		return fmt.Errorf("edge: missing upstream.public-key")
	}
	if _, err := cfg.UpstreamURL(); err != nil {
		return err
	}
	if err := cfg.Upstream.validate(); err != nil {
		return err
	}
	if err := cfg.TURN.validate(); err != nil {
		return err
	}
	if err := cfg.Gateway.validate(); err != nil {
		return err
	}
	if err := cfg.validateSystemLog(); err != nil {
		return err
	}
	switch cfg.TLS.CertSource {
	case TLSCertSourceDisabled:
		return nil
	case TLSCertSourceEdgeRPC, TLSCertSourceFile:
		return fmt.Errorf("edge: tls.cert-source %q is not implemented", cfg.TLS.CertSource)
	default:
		return fmt.Errorf("edge: invalid tls.cert-source %q", cfg.TLS.CertSource)
	}
}

func runtimeStorageConfigs(configs map[string]storageFileConfig) (map[string]storage.Config, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	out := make(map[string]storage.Config, len(configs))
	for name, cfg := range configs {
		runtime, err := cfg.runtimeConfig()
		if err != nil {
			return nil, fmt.Errorf("edge: storage.%s: %w", name, err)
		}
		out[name] = runtime
	}
	return out, nil
}

func runtimeStoreConfigs(configs map[string]storeFileConfig) (map[string]store.Config, error) {
	if len(configs) == 0 {
		return nil, nil
	}
	out := make(map[string]store.Config, len(configs))
	for name, cfg := range configs {
		runtime, err := cfg.runtimeConfig()
		if err != nil {
			return nil, fmt.Errorf("edge: stores.%s: %w", name, err)
		}
		out[name] = runtime
	}
	return out, nil
}

func (cfg Config) validateSystemLog() error {
	if cfg.SystemLog.QueryStore != "" {
		return fmt.Errorf("edge: system-log.query_store is not supported")
	}
	for name, physical := range cfg.Storage {
		switch physical.(type) {
		case storage.VolcTLSConfig, *storage.VolcTLSConfig:
		default:
			return fmt.Errorf("edge: storage.%s only supports %s", name, storage.KindVolcTLS)
		}
	}
	for name, logical := range cfg.Stores {
		if logical.Kind != store.KindLogImmutable {
			return fmt.Errorf("edge: stores.%s only supports %s", name, store.KindLogImmutable)
		}
		if _, ok := cfg.Storage[logical.Storage]; !ok {
			return fmt.Errorf("edge: stores.%s references missing storage.%s", name, logical.Storage)
		}
	}
	for index, sink := range cfg.SystemLog.Sinks {
		if sink.Kind != gizlog.SinkStore {
			continue
		}
		if _, ok := cfg.Stores[sink.Store]; !ok {
			return fmt.Errorf("edge: system-log.sinks[%d].store %q is not configured", index, sink.Store)
		}
	}
	return nil
}

func (cfg UpstreamConfig) relayEnabled() bool {
	return cfg.ICETransportPolicy == "relay" && len(cfg.ICEServers) > 0
}

func (cfg UpstreamConfig) validate() error {
	policy := cfg.ICETransportPolicy
	if policy == "" && len(cfg.ICEServers) == 0 {
		return nil
	}
	if policy != "relay" {
		return fmt.Errorf("edge: upstream.ice-transport-policy must be relay when upstream ICE servers are configured")
	}
	if len(cfg.ICEServers) < 2 {
		return fmt.Errorf("edge: upstream.ice-servers must contain at least two relay members")
	}
	seen := make(map[string]int, len(cfg.ICEServers))
	for i, server := range cfg.ICEServers {
		endpoint, err := upstreamRelayEndpoint(server)
		if err != nil {
			return fmt.Errorf("edge: invalid upstream.ice-servers[%d]: %w", i, err)
		}
		if previous, ok := seen[endpoint]; ok {
			return fmt.Errorf("edge: upstream.ice-servers[%d] duplicates member %d", i, previous)
		}
		seen[endpoint] = i
		if err := validateUpstreamRelayCredentials(server); err != nil {
			return fmt.Errorf("edge: invalid upstream.ice-servers[%d]: %w", i, err)
		}
	}
	return nil
}

func upstreamRelayEndpoint(server gizwebrtc.ICEServer) (string, error) {
	if len(server.URLs) != 1 {
		return "", fmt.Errorf("urls must contain exactly one TURN endpoint")
	}
	raw := server.URLs[0]
	if raw != strings.TrimSpace(raw) {
		return "", fmt.Errorf("urls must not contain surrounding whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("urls contain an invalid TURN endpoint")
	}
	if !strings.HasPrefix(raw, "turn:") || parsed.Scheme != "turn" || parsed.Opaque == "" || parsed.Host != "" || parsed.User != nil {
		return "", fmt.Errorf("urls must use a turn URI without userinfo")
	}
	if parsed.Fragment != "" {
		return "", fmt.Errorf("urls must not contain a fragment")
	}
	query := parsed.Query()
	if len(query) != 1 || len(query["transport"]) != 1 || query.Get("transport") != "udp" {
		return "", fmt.Errorf("urls must contain only transport=udp")
	}
	host, portText, err := net.SplitHostPort(parsed.Opaque)
	if err != nil {
		return "", fmt.Errorf("urls must contain a literal IP and explicit port")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return "", fmt.Errorf("urls host must be a literal IP")
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return "", fmt.Errorf("urls port must be between 1 and 65535")
	}
	return net.JoinHostPort(ip.String(), strconv.FormatUint(port, 10)), nil
}

func validateUpstreamRelayCredentials(server gizwebrtc.ICEServer) error {
	switch server.CredentialMode {
	case "", gizwebrtc.ICECredentialModeStatic:
		if strings.TrimSpace(server.Username) == "" || strings.TrimSpace(server.Credential) == "" {
			return fmt.Errorf("static credential mode requires username and credential")
		}
	case gizwebrtc.ICECredentialModeTURNREST:
		if strings.TrimSpace(server.Credential) == "" {
			return fmt.Errorf("turn-rest credential mode requires credential")
		}
	default:
		return fmt.Errorf("credential-mode is unsupported")
	}
	return nil
}

func mergeGatewayConfig(cfg, file GatewayConfig) GatewayConfig {
	if !cfg.Enabled {
		cfg.Enabled = file.Enabled
	}
	if cfg.MaxSessions == 0 {
		cfg.MaxSessions = file.MaxSessions
	}
	if cfg.MaxUpstreams == 0 {
		cfg.MaxUpstreams = file.MaxUpstreams
	}
	if cfg.SessionsPerUpstream == 0 {
		cfg.SessionsPerUpstream = file.SessionsPerUpstream
	}
	if cfg.ChannelsPerSession == 0 {
		cfg.ChannelsPerSession = file.ChannelsPerSession
	}
	if cfg.ChannelsPerUpstream == 0 {
		cfg.ChannelsPerUpstream = file.ChannelsPerUpstream
	}
	if cfg.MaxPendingHandshakes == 0 {
		cfg.MaxPendingHandshakes = file.MaxPendingHandshakes
	}
	if cfg.SessionBufferBytes == 0 {
		cfg.SessionBufferBytes = file.SessionBufferBytes
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = file.IdleTimeout
	}
	if cfg.DrainTimeout == 0 {
		cfg.DrainTimeout = file.DrainTimeout
	}
	return cfg
}

func applyGatewayDefaults(cfg GatewayConfig) GatewayConfig {
	defaults := defaultGatewayConfig()
	return mergeGatewayConfig(cfg, defaults)
}

func (cfg GatewayConfig) validate() error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.MaxSessions <= 0 {
		return fmt.Errorf("edge: gateway.max-sessions must be positive")
	}
	if cfg.MaxUpstreams <= 0 || cfg.MaxUpstreams > 64 {
		return fmt.Errorf("edge: gateway.max-upstreams must be between 1 and 64")
	}
	if cfg.SessionsPerUpstream <= 0 || cfg.SessionsPerUpstream > 2048 {
		return fmt.Errorf("edge: gateway.sessions-per-upstream must be between 1 and 2048")
	}
	if cfg.MaxSessions > cfg.MaxUpstreams*cfg.SessionsPerUpstream {
		return fmt.Errorf("edge: gateway.max-sessions exceeds upstream pool capacity")
	}
	if cfg.ChannelsPerSession < 3 || cfg.ChannelsPerSession > 32 {
		return fmt.Errorf("edge: gateway.channels-per-session must be between 3 and 32")
	}
	if cfg.ChannelsPerUpstream < 3*cfg.SessionsPerUpstream || cfg.ChannelsPerUpstream > 8192 {
		return fmt.Errorf("edge: gateway.channels-per-upstream must be between three times sessions-per-upstream and 8192")
	}
	if cfg.MaxPendingHandshakes <= 0 || cfg.MaxPendingHandshakes > cfg.MaxSessions || cfg.MaxPendingHandshakes > 2048 {
		return fmt.Errorf("edge: gateway.max-pending-handshakes must be between 1 and the smaller of max-sessions and 2048")
	}
	if cfg.SessionBufferBytes < 64*1024 || cfg.SessionBufferBytes > 16*1024*1024 {
		return fmt.Errorf("edge: gateway.session-buffer-bytes must be between 65536 and 16777216")
	}
	if cfg.IdleTimeout <= 0 {
		return fmt.Errorf("edge: gateway.idle-timeout must be positive")
	}
	if cfg.DrainTimeout <= 0 {
		return fmt.Errorf("edge: gateway.drain-timeout must be positive")
	}
	return nil
}

func (cfg TURNConfig) enabled() bool {
	return strings.TrimSpace(cfg.Listen) != ""
}

func (cfg TURNConfig) validate() error {
	if !cfg.enabled() {
		return nil
	}
	for field, value := range map[string]string{
		"public-endpoint": cfg.PublicEndpoint,
		"realm":           cfg.Realm,
		"username":        cfg.Username,
		"credential":      cfg.Credential,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("edge: turn.%s is required", field)
		}
	}
	if _, _, err := netSplitHostPort("turn.listen", cfg.Listen); err != nil {
		return err
	}
	host, _, err := netSplitHostPort("turn.public-endpoint", cfg.PublicEndpoint)
	if err != nil {
		return err
	}
	relayAddress := strings.TrimSpace(cfg.RelayAddress)
	if relayAddress == "" {
		relayAddress = host
	}
	if net.ParseIP(relayAddress) == nil {
		return fmt.Errorf("edge: turn.relay-address must be an IP address")
	}
	if cfg.RelayMinPort == 0 {
		return fmt.Errorf("edge: turn.relay-min-port is required")
	}
	if cfg.RelayMaxPort == 0 {
		return fmt.Errorf("edge: turn.relay-max-port is required")
	}
	if cfg.RelayMaxPort < cfg.RelayMinPort {
		return fmt.Errorf("edge: turn.relay-max-port must be >= turn.relay-min-port")
	}
	return nil
}

func netSplitHostPort(field, value string) (string, string, error) {
	if strings.Contains(value, "://") {
		return "", "", fmt.Errorf("edge: %s must be host:port, got %q", field, value)
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil {
		return "", "", fmt.Errorf("edge: invalid %s: %w", field, err)
	}
	if strings.TrimSpace(host) == "" {
		return "", "", fmt.Errorf("edge: %s host is empty", field)
	}
	if strings.TrimSpace(port) == "" {
		return "", "", fmt.Errorf("edge: %s port is empty", field)
	}
	return host, port, nil
}

func netSplitNumericHostPort(field, value string) (string, string, error) {
	host, port, err := netSplitHostPort(field, value)
	if err != nil {
		return "", "", err
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return "", "", fmt.Errorf("edge: %s port must be between 1 and 65535, got %q", field, port)
	}
	return host, port, nil
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
