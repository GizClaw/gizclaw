package storage

import (
	"crypto/x509"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedisOptionsSupportsPlainAndTLSURLs(t *testing.T) {
	plain, err := redisOptions("cache", RedisConfig{URL: "redis://user:password@host:6379/0"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.TLSConfig != nil {
		t.Fatal("redis URL unexpectedly enabled TLS")
	}

	tlsServer := httptest.NewTLSServer(nil)
	t.Cleanup(tlsServer.Close)
	certificate := tlsServer.Certificate()
	caFile := filepath.Join(t.TempDir(), "redis-ca.pem")
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	secure, err := redisOptions("cache", RedisConfig{
		URL:       "rediss://user:password@host:6379/0",
		TLSCAFile: caFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secure.TLSConfig == nil || secure.TLSConfig.RootCAs == nil {
		t.Fatal("rediss URL did not configure custom root CAs")
	}
	if _, err := certificate.Verify(x509.VerifyOptions{Roots: secure.TLSConfig.RootCAs}); err != nil {
		t.Fatalf("custom Redis CA is not trusted: %v", err)
	}
}

func TestRedisOptionsRejectsInvalidTLSCAConfiguration(t *testing.T) {
	invalidCA := filepath.Join(t.TempDir(), "invalid.pem")
	if err := os.WriteFile(invalidCA, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := map[string]RedisConfig{
		"plain URL with CA": {URL: "redis://host:6379/0", TLSCAFile: invalidCA},
		"verification off":  {URL: "rediss://host:6379/0?skip_verify=true"},
		"missing CA file":   {URL: "rediss://host:6379/0", TLSCAFile: filepath.Join(t.TempDir(), "missing.pem")},
		"invalid CA file":   {URL: "rediss://host:6379/0", TLSCAFile: invalidCA},
	}
	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := redisOptions("cache", cfg)
			if err == nil || !strings.HasPrefix(err.Error(), `storage: redis "cache" `) {
				t.Fatalf("redisOptions() error = %v", err)
			}
		})
	}
}

func TestRedisOptionsDoesNotExposeURLSecrets(t *testing.T) {
	const secret = "leaked-password"
	_, err := redisOptions("cache", RedisConfig{URL: "rediss://user:" + secret + "@%zz"})
	if err == nil {
		t.Fatal("redisOptions() error = nil")
	}
	if strings.Contains(err.Error(), secret) || err.Error() != `storage: redis "cache" parse url failed` {
		t.Fatalf("redisOptions() exposed URL details: %v", err)
	}
}
