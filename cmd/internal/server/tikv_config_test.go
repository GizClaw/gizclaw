package server

import (
	"reflect"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestTiKVConfiguration(t *testing.T) {
	file := storageFileConfig{Kind: storage.KindTiKV, PDAddresses: []string{"pd-1:2379", "pd-2:2379"}, StaleReadDelay: "5s", ReadLabels: map[string]string{"region": "local"}, TLSCAFile: "ca.pem", TLSCertFile: "cert.pem", TLSKeyFile: "key.pem"}
	got, err := file.runtimeConfig()
	want := storage.TiKVConfig{PDAddresses: file.PDAddresses, StaleReadDelay: 5 * time.Second, ReadLabels: file.ReadLabels, TLSCAFile: "ca.pem", TLSCertFile: "cert.pem", TLSKeyFile: "key.pem"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime config=%+v %v", got, err)
	}
	for _, mapping := range []map[string]any{
		{"kind": "tikv", "pd_addresses": []string{"pd:2379"}, "read_url": "redis://replica:6379"},
		{"kind": "redis", "pd_addresses": []string{"pd:2379"}},
	} {
		if err := validateStorageConfigShape(map[string]any{"state": mapping}); err == nil {
			t.Fatal("accepted mixed backend fields")
		}
	}
	file.StaleReadDelay = "bad"
	if _, err := file.runtimeConfig(); err == nil {
		t.Fatal("accepted invalid stale read duration")
	}
}

func TestParseTiKVConfiguration(t *testing.T) {
	t.Setenv("TIKV_TEST_PD", "pd.example.com:2379")
	cfg, err := parseConfigData([]byte(`storage:
  state:
    kind: tikv
    pd_addresses: ["${TIKV_TEST_PD}"]
    stale_read_delay: 5s
    read_labels:
      region: local
stores:
  peers:
    kind: keyvalue
    storage: state
    prefix: peers
`))
	if err != nil {
		t.Fatal(err)
	}
	physical, err := cfg.Storage["state"].runtimeConfig()
	want := storage.TiKVConfig{PDAddresses: []string{"pd.example.com:2379"}, StaleReadDelay: 5 * time.Second, ReadLabels: map[string]string{"region": "local"}}
	if err != nil || !reflect.DeepEqual(physical, want) {
		t.Fatalf("parsed TiKV config=%+v %v", physical, err)
	}
}
