package gizedge

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	store "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestPrepareWorkspaceConfigLoadsSystemLogStore(t *testing.T) {
	edgeKey := testKeyPair(t, 0x19)
	upstreamKey := testKeyPair(t, 0x20)
	t.Setenv("EDGE_LOG_ENDPOINT", "https://tls.example.test")
	t.Setenv("EDGE_LOG_REGION", "test-region")
	t.Setenv("EDGE_LOG_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("EDGE_LOG_ACCESS_KEY_SECRET", "test-access-secret")
	t.Setenv("EDGE_LOG_TOPIC_ID", "test-topic")
	t.Setenv("EDGE_LOG_NODE_ID", "edge-a")
	dir := t.TempDir()
	writeConfig(t, dir, `
identity:
  private-key: `+edgeKey.Private.String()+`
upstreams:
  - endpoint: server-a.example.com:9820
    public-key: `+upstreamKey.Public.String()+`
http:
  listeners:
    - listen: 0.0.0.0:9821
storage:
  volc-logs:
    kind: volc-tls
    endpoint: ${EDGE_LOG_ENDPOINT}
    region: ${EDGE_LOG_REGION}
    access_key_id: ${EDGE_LOG_ACCESS_KEY_ID}
    access_key_secret: ${EDGE_LOG_ACCESS_KEY_SECRET}
stores:
  logs:
    kind: log.immutable
    storage: volc-logs
    topic_id: ${EDGE_LOG_TOPIC_ID}
system-log:
  level: info
  node_id: ${EDGE_LOG_NODE_ID}
  sinks:
    - kind: stderr
    - kind: store
      store: logs
`)

	cfg, err := PrepareWorkspaceConfig(dir)
	if err != nil {
		t.Fatalf("PrepareWorkspaceConfig error = %v", err)
	}
	physical, ok := cfg.Storage["volc-logs"].(storage.VolcTLSConfig)
	if !ok {
		t.Fatalf("Storage[volc-logs] = %#v", cfg.Storage["volc-logs"])
	}
	if physical.Endpoint != "https://tls.example.test" || physical.Region != "test-region" ||
		physical.AccessKeyID != "test-access-key" || physical.AccessKeySecret != "test-access-secret" {
		t.Fatalf("Storage[volc-logs] = %#v", physical)
	}
	logical := cfg.Stores["logs"]
	if logical.Kind != store.KindLogImmutable || logical.Storage != "volc-logs" || logical.TopicID != "test-topic" {
		t.Fatalf("Stores[logs] = %#v", logical)
	}
	if cfg.SystemLog.Level != "info" || cfg.SystemLog.NodeID != "edge-a" || len(cfg.SystemLog.Sinks) != 2 ||
		cfg.SystemLog.Sinks[0].Kind != gizlog.SinkStderr || cfg.SystemLog.Sinks[1].Store != "logs" {
		t.Fatalf("SystemLog = %#v", cfg.SystemLog)
	}
}

func TestPrepareWorkspaceConfigDefaultsSystemLogToStderr(t *testing.T) {
	edgeKey := testKeyPair(t, 0x21)
	upstreamKey := testKeyPair(t, 0x22)
	dir := t.TempDir()
	writeConfig(t, dir, `
identity:
  private-key: `+edgeKey.Private.String()+`
upstreams:
  - endpoint: server-a.example.com:9820
    public-key: `+upstreamKey.Public.String()+`
http:
  listeners:
    - listen: 0.0.0.0:9821
`)

	cfg, err := PrepareWorkspaceConfig(dir)
	if err != nil {
		t.Fatalf("PrepareWorkspaceConfig error = %v", err)
	}
	if cfg.SystemLog.Level != "info" || len(cfg.SystemLog.Sinks) != 1 || cfg.SystemLog.Sinks[0].Kind != gizlog.SinkStderr {
		t.Fatalf("SystemLog = %#v, want info stderr", cfg.SystemLog)
	}
}

func TestPrepareWorkspaceConfigRejectsInvalidSystemLogTopology(t *testing.T) {
	edgeKey := testKeyPair(t, 0x23)
	upstreamKey := testKeyPair(t, 0x24)
	prefix := `
identity:
  private-key: ` + edgeKey.Private.String() + `
upstreams:
  - endpoint: server-a.example.com:9820
    public-key: ` + upstreamKey.Public.String() + `
http:
  listeners:
    - listen: 0.0.0.0:9821
`
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "storage kind", body: "storage:\n  logs:\n    kind: memory\n", want: "storage.logs: only volc-tls is supported"},
		{name: "mutable log", body: "stores:\n  logs:\n    kind: log.mutable\n    storage: volc-logs\n", want: "stores.logs: only log.immutable is supported"},
		{name: "missing physical", body: "stores:\n  logs:\n    kind: log.immutable\n    storage: volc-logs\n", want: "references missing storage.volc-logs"},
		{name: "missing sink", body: "system-log:\n  sinks:\n    - kind: store\n      store: logs\n", want: "store \"logs\" is not configured"},
		{name: "query store", body: "system-log:\n  query_store: logs\n  sinks:\n    - kind: store\n      store: logs\n", want: "system-log.query_store is not supported"},
		{name: "level", body: "system-log:\n  level: verbose\n", want: "system-log.level"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, prefix+tc.body)
			_, err := PrepareWorkspaceConfig(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("PrepareWorkspaceConfig error = %v, want %q", err, tc.want)
			}
		})
	}
}
