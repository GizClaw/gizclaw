package server

import (
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestParseConfigDataExpandsEnvironmentInEveryValue(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	edgeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	adminKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	token := "gizclaw_mk_01234567890123456789012345678901"
	for name, value := range map[string]string{
		"T_PRIVATE_KEY":   serverKey.Private.String(),
		"T_EDGE_KEY":      edgeKey.Public.String(),
		"T_ADMIN_KEY":     adminKey.Public.String(),
		"T_LISTEN":        "127.0.0.1:9820",
		"T_ENDPOINT":      "203.0.113.10:9820",
		"T_DIR":           "/run/secrets",
		"T_TURN_URL":      "turn:turn.example.com:3478",
		"T_TRUE":          "true",
		"T_MONITOR_TOKEN": token,
		"T_KIND":          "postgresql",
		"T_DSN":           "postgres://gizclaw:pa$word@db:5432/gizclaw",
		"T_TTL":           "2160h",
		"T_STORE":         "business",
		"T_SFU_URL":       "  wss://sfu.example.com  ",
		"T_DURATION":      "7s",
		"T_LEVEL":         "debug",
		"T_NODE":          "server-a",
		"T_INT":           "1024",
		"T_WORKERS":       "8",
		"T_ZERO_PADDED":   "007",
	} {
		t.Setenv(name, value)
	}
	data := `identity:
  private-key: ${T_PRIVATE_KEY}
webrtc:
  listen: ${T_LISTEN}
  endpoint: "${T_ENDPOINT}"
http:
  listeners:
    - listen: $T_LISTEN
      tls: {cert-file: "${T_DIR}/cert.pem", key-file: '${T_DIR}/key.pem'}
edge-nodes:
  - ${T_EDGE_KEY}
admin-public-key: ${T_ADMIN_KEY}
ice-servers:
  - urls: ["${T_TURN_URL}"]
    username: ${T_TRUE}
    credential: pa$$word
monitor:
  token: ${T_MONITOR_TOKEN}
storage:
  ${T_KIND}:
    kind: ${T_KIND}
    dsn: "${T_DSN}"
stores:
  logs:
    kind: log.immutable
    storage: ${T_KIND}
    topic_id: ${T_ZERO_PADDED}
    ttl: ${T_TTL}
services:
  peer:
    store: ${T_STORE}
  sfu:
    url: ${T_SFU_URL}
    api_key_file: ${T_DIR}/sfu_key
    api_secret_file: ${T_DIR}/sfu_secret
    recheck_interval: ${T_DURATION}
  system_log: {level: "${T_LEVEL}", node_id: "${T_NODE}", sinks: [{kind: stderr}]}
speech:
  transcription:
    max_audio_bytes: ${T_INT}
    request_timeout: ${T_DURATION}
pending_deletion:
  workers: ${T_WORKERS}
  scan_interval: ${T_DURATION}
profiling:
  enabled: ${T_TRUE}
  store: "${T_TRUE}"
`
	cfg, err := parseConfigData([]byte(data))
	if err != nil {
		t.Fatalf("parseConfigData error = %v", err)
	}
	if cfg.Identity.PrivateKey != serverKey.Private {
		t.Fatalf("identity.private-key = %s", cfg.Identity.PrivateKey)
	}
	if *cfg.WebRTC != (WebRTCConfig{Listen: "127.0.0.1:9820", Endpoint: "203.0.113.10:9820"}) {
		t.Fatalf("webrtc = %+v", *cfg.WebRTC)
	}
	wantListener := HTTPListenerConfig{Listen: "127.0.0.1:9820", TLS: HTTPListenerTLSConfig{CertFile: "/run/secrets/cert.pem", KeyFile: "/run/secrets/key.pem"}}
	if len(cfg.HTTP.Listeners) != 1 || cfg.HTTP.Listeners[0] != wantListener {
		t.Fatalf("http.listeners = %+v", cfg.HTTP.Listeners)
	}
	if len(cfg.EdgeNodes) != 1 || cfg.EdgeNodes[0] != edgeKey.Public || cfg.AdminPublicKey != adminKey.Public {
		t.Fatalf("edge-nodes = %v, admin-public-key = %s", cfg.EdgeNodes, cfg.AdminPublicKey)
	}
	if len(cfg.ICEServers) != 1 || !reflect.DeepEqual(cfg.ICEServers[0].URLs, []string{"turn:turn.example.com:3478"}) ||
		cfg.ICEServers[0].Username != "true" || cfg.ICEServers[0].Credential != "pa$word" {
		t.Fatalf("ice-servers = %+v", cfg.ICEServers)
	}
	if cfg.Monitor.Token != token {
		t.Fatalf("monitor.token = %q", cfg.Monitor.Token)
	}
	// Mapping keys stay literal; a "$" inside an expanded value is not expanded again.
	if got := cfg.Storage["${T_KIND}"]; got != (storageFileConfig{Kind: storage.KindPostgreSQL, DSN: "postgres://gizclaw:pa$word@db:5432/gizclaw"}) {
		t.Fatalf("storage = %+v", cfg.Storage)
	}
	if got := cfg.Stores["logs"]; got != (storeFileConfig{Kind: "log.immutable", Storage: "postgresql", TopicID: "007", TTL: "2160h"}) {
		t.Fatalf("stores.logs = %+v", got)
	}
	services := cfg.Services
	if services == nil || services.Peer == nil || services.Peer.Store != "business" || services.SFU == nil || services.SystemLog == nil {
		t.Fatalf("services = %+v", services)
	}
	if *services.SFU != (SFUConfig{URL: "  wss://sfu.example.com  ", APIKeyFile: "/run/secrets/sfu_key", APISecretFile: "/run/secrets/sfu_secret", RecheckInterval: "7s"}) {
		t.Fatalf("services.sfu = %+v", *services.SFU)
	}
	if err := services.SFU.validate(); err != nil {
		t.Fatalf("services.sfu.validate() error = %v", err)
	}
	if services.SystemLog.Level != "debug" || services.SystemLog.NodeID != "server-a" {
		t.Fatalf("services.system_log = %+v", services.SystemLog)
	}
	if cfg.Speech.Transcription.MaxAudioBytes != 1024 || cfg.Speech.Transcription.RequestTimeout != "7s" {
		t.Fatalf("speech = %+v", cfg.Speech)
	}
	if cfg.PendingDeletion.Workers != 8 || cfg.PendingDeletion.ScanInterval != "7s" {
		t.Fatalf("pending_deletion = %+v", cfg.PendingDeletion)
	}
	if cfg.Profiling != (ProfilingConfig{Enabled: true, Store: "true"}) {
		t.Fatalf("profiling = %+v", cfg.Profiling)
	}
}

func TestParseConfigDataSFUURLEnvironment(t *testing.T) {
	t.Setenv("T_SFU_URL", "ws://127.0.0.1:7880")
	t.Setenv("T_SFU_HTTP_URL", "https://sfu.example.com")
	sfuConfig := func(url string) string {
		return "services:\n  sfu:\n    url: " + url + "\n    api_key_file: /run/secrets/sfu_key\n    api_secret_file: /run/secrets/sfu_secret\n"
	}
	for name, test := range map[string]struct {
		url  string
		want string
	}{
		"expanded env url": {"${T_SFU_URL}", "ws://127.0.0.1:7880"},
		"literal url":      {"wss://sfu.internal", "wss://sfu.internal"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseConfigData([]byte(sfuConfig(test.url)))
			if err != nil {
				t.Fatalf("parseConfigData error = %v", err)
			}
			if cfg.Services.SFU.URL != test.want {
				t.Fatalf("services.sfu.url = %q, want %q", cfg.Services.SFU.URL, test.want)
			}
			if err := cfg.Services.SFU.validate(); err != nil {
				t.Fatalf("validate() error = %v", err)
			}
		})
	}
	for _, url := range []string{"${T_SFU_URL_UNSET}", `"${T_SFU_URL_UNSET}"`} {
		if _, err := parseConfigData([]byte(sfuConfig(url))); err == nil || !strings.Contains(err.Error(), "services.sfu.url must not be empty") {
			t.Fatalf("parseConfigData(unset %s) error = %v", url, err)
		}
	}
	cfg, err := parseConfigData([]byte(sfuConfig("${T_SFU_HTTP_URL}")))
	if err != nil {
		t.Fatalf("parseConfigData(https) error = %v", err)
	}
	if err := cfg.Services.SFU.validate(); err == nil || !strings.Contains(err.Error(), "services.sfu.url must be a ws:// or wss:// URL") {
		t.Fatalf("validate(https) error = %v", err)
	}
}

func TestParseConfigDataExpandsAnchorsAndTaggedValues(t *testing.T) {
	t.Setenv("T_STORE", "shared")
	t.Setenv("T_TRUE", "true")
	data := `services:
  peer: {store: &store "${T_STORE}"}
  peer_run: {store: *store}
  api_key:
    store: !!str ${T_TRUE}
  credential:
    store: |-
      ${T_STORE}-block
profiling:
  enabled: !!bool ${T_TRUE}
`
	cfg, err := parseConfigData([]byte(data))
	if err != nil {
		t.Fatalf("parseConfigData error = %v", err)
	}
	if cfg.Services.Peer.Store != "shared" || cfg.Services.PeerRun.Store != "shared" || cfg.Services.APIKey.Store != "true" ||
		cfg.Services.Credential.Store != "shared-block" {
		t.Fatalf("services = peer %+v, peer_run %+v, api_key %+v, credential %+v", cfg.Services.Peer, cfg.Services.PeerRun, cfg.Services.APIKey, cfg.Services.Credential)
	}
	if !cfg.Profiling.Enabled {
		t.Fatalf("profiling = %+v", cfg.Profiling)
	}
}

func TestParseConfigDataHandlesEmptyDocuments(t *testing.T) {
	for _, data := range []string{"", "# only a comment\n", "---\n"} {
		cfg, err := parseConfigData([]byte(data))
		if err != nil {
			t.Fatalf("parseConfigData(%q) error = %v", data, err)
		}
		if cfg.Services != nil || cfg.WebRTC != nil || len(cfg.Storage) != 0 {
			t.Fatalf("parseConfigData(%q) = %+v, want empty", data, cfg)
		}
	}
}

func TestExpandConfigEnvEscapesDollar(t *testing.T) {
	t.Setenv("T_NAME", "value")
	for input, want := range map[string]string{
		"${T_NAME}":       "value",
		"$T_NAME/suffix":  "value/suffix",
		"$$T_NAME":        "$T_NAME",
		"cost: $$5":       "cost: $5",
		"trailing$":       "trailing$",
		"${T_NAME_UNSET}": "",
	} {
		if got := expandConfigEnv(input); got != want {
			t.Fatalf("expandConfigEnv(%q) = %q, want %q", input, got, want)
		}
	}
}
