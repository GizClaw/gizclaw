package storage

import (
	"strings"
	"testing"
	"time"
)

func TestTiKVConfigValidationBeforeNetwork(t *testing.T) {
	for _, cfg := range []TiKVConfig{
		{}, {PDAddresses: []string{"http://pd:2379"}}, {PDAddresses: []string{"pd:0"}}, {PDAddresses: []string{"pd:abc"}}, {PDAddresses: []string{"pd:2379", "pd:2379"}},
		{PDAddresses: []string{"pd:2379"}, ReadLabels: map[string]string{"": "local"}},
		{PDAddresses: []string{"pd:2379"}, ReadLabels: map[string]string{"region": ""}},
		{PDAddresses: []string{"pd:2379"}, ReadLabels: map[string]string{" region": "local"}},
		{PDAddresses: []string{"pd:2379"}, ReadLabels: map[string]string{"region": "local "}},
		{PDAddresses: []string{"pd:2379"}, ReadLabels: map[string]string{"\t": "local"}},
		{PDAddresses: []string{"pd:2379"}, ReadLabels: map[string]string{"region": "\n"}},
		{PDAddresses: []string{"pd:2379"}, TLSCertFile: "cert.pem"},
	} {
		if _, err := New(map[string]Config{"bad": cfg}); err == nil {
			t.Fatalf("accepted invalid config %+v", cfg)
		}
	}
	for _, labels := range []map[string]string{nil, {}, {"region": "local"}} {
		for _, delay := range []time.Duration{0, time.Second, -time.Second, time.Microsecond, 25 * time.Hour} {
			cfg := TiKVConfig{PDAddresses: []string{"[::1]:2379"}, ReadLabels: labels, StaleReadDelay: delay}
			if err := ValidateTiKVConfig(cfg); err != nil {
				t.Fatalf("rejected labels %+v with ignored delay %v: %v", labels, delay, err)
			}
		}
	}
	_, err := New(map[string]Config{"tls": TiKVConfig{PDAddresses: []string{"127.0.0.1:1"}, TLSCAFile: t.TempDir() + "/missing-ca.pem"}})
	if err == nil || strings.Contains(err.Error(), "missing-ca.pem") {
		t.Fatalf("TLS error was not sanitized: %v", err)
	}
}
