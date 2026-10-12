package server

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestParseRedisReadWriteConfig(t *testing.T) {
	t.Setenv("TEST_REDIS_READ_URL", "rediss://reader.example.com:6379/0")
	t.Setenv("TEST_REDIS_WRITE_URL", "rediss://writer.example.com:6379/0")
	cfg, err := parseConfigData([]byte(`storage:
  shared:
    kind: redis.readwrite
    read_url: ${TEST_REDIS_READ_URL}
    write_url: ${TEST_REDIS_WRITE_URL}
    read_tls_ca_file: /run/secrets/read-ca.pem
    write_tls_ca_file: /run/secrets/write-ca.pem
stores:
  friends:
    kind: keyvalue
    storage: shared
    prefix: friends
`))
	if err != nil {
		t.Fatal(err)
	}
	physical, err := cfg.Storage["shared"].runtimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := storage.RedisReadWriteConfig{
		Read:  storage.RedisConfig{URL: "rediss://reader.example.com:6379/0", TLSCAFile: "/run/secrets/read-ca.pem"},
		Write: storage.RedisConfig{URL: "rediss://writer.example.com:6379/0", TLSCAFile: "/run/secrets/write-ca.pem"},
	}
	if physical != want {
		t.Fatal("Redis read/write configuration did not preserve both endpoints and CA files")
	}
	if cfg.Stores["friends"].Kind != "keyvalue" || cfg.Stores["friends"].Storage != "shared" {
		t.Fatal("Redis read/write backend changed the logical Store binding")
	}
}

func TestRedisBackendConfigFieldsStayDistinct(t *testing.T) {
	for name, entry := range map[string]map[string]any{
		"plain with read URL":  {"kind": "redis", "url": "redis://primary:6379/0", "read_url": "redis://replica:6379/0"},
		"split with plain URL": {"kind": "redis.readwrite", "url": "redis://primary:6379/0"},
		"split with shared CA": {"kind": "redis.readwrite", "read_url": "rediss://replica:6379/0", "write_url": "rediss://primary:6379/0", "tls_ca_file": "/run/secrets/ca.pem"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateStorageConfigShape(map[string]any{"shared": entry}); err == nil {
				t.Fatal("mixed backend fields were accepted")
			}
		})
	}
	if err := validateStorageConfigShape(map[string]any{"shared": map[string]any{
		"kind": "redis", "url": "redis://primary:6379/0", "tls_ca_file": "",
	}}); err != nil {
		t.Fatalf("ordinary Redis configuration is no longer accepted: %v", err)
	}
}
