package storage

import (
	"strings"
	"testing"
	"time"
)

func TestTiKVConfigValidationBeforeNetwork(t *testing.T) {
	for _, cfg := range []TiKVConfig{
		{}, {PDAddresses: []string{"http://pd:2379"}}, {PDAddresses: []string{"pd:0"}}, {PDAddresses: []string{"pd:abc"}}, {PDAddresses: []string{"pd:2379", "pd:2379"}},
		{PDAddresses: []string{"pd:2379"}, StaleReadDelay: time.Second},
		{PDAddresses: []string{"pd:2379"}, ReadLabels: map[string]string{"region": "local"}},
		{PDAddresses: []string{"pd:2379"}, StaleReadDelay: time.Microsecond, ReadLabels: map[string]string{"region": "local"}},
		{PDAddresses: []string{"pd:2379"}, TLSCertFile: "cert.pem"},
	} {
		if _, err := New(map[string]Config{"bad": cfg}); err == nil {
			t.Fatalf("accepted invalid config %+v", cfg)
		}
	}
	for _, cfg := range []TiKVConfig{{PDAddresses: []string{"pd:2379"}}, {PDAddresses: []string{"[::1]:2379"}, StaleReadDelay: time.Second, ReadLabels: map[string]string{"region": "local"}}} {
		if err := ValidateTiKVConfig(cfg); err != nil {
			t.Fatal(err)
		}
	}
	_, err := New(map[string]Config{"tls": TiKVConfig{PDAddresses: []string{"127.0.0.1:1"}, TLSCAFile: t.TempDir() + "/missing-ca.pem"}})
	if err == nil || strings.Contains(err.Error(), "missing-ca.pem") {
		t.Fatalf("TLS error was not sanitized: %v", err)
	}
}
