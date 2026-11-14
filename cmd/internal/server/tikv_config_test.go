package server

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
)

func TestTiKVConfiguration(t *testing.T) {
	file := storageFileConfig{Kind: storage.KindTiKV, PDAddresses: []string{"pd-1:2379", "pd-2:2379"}, ReadLabels: map[string]string{"region": "local"}, TLSCAFile: "ca.pem", TLSCertFile: "cert.pem", TLSKeyFile: "key.pem"}
	want := storage.TiKVConfig{PDAddresses: file.PDAddresses, ReadLabels: file.ReadLabels, TLSCAFile: "ca.pem", TLSCertFile: "cert.pem", TLSKeyFile: "key.pem"}
	for _, delay := range []string{"", "5s", "bad", "-1s", "1us", "25h"} {
		file.StaleReadDelay = delay
		got, err := file.runtimeConfig()
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("runtime config with ignored delay %q=%+v %v", delay, got, err)
		}
	}
	for _, mapping := range []map[string]any{
		{"kind": "tikv", "pd_addresses": []string{"pd:2379"}, "read_url": "redis://replica:6379"},
		{"kind": "redis", "pd_addresses": []string{"pd:2379"}},
	} {
		if err := validateStorageConfigShape(map[string]any{"state": mapping}); err == nil {
			t.Fatal("accepted mixed backend fields")
		}
	}
}

func TestParseTiKVConfiguration(t *testing.T) {
	t.Setenv("TIKV_TEST_PD", "pd.example.com:2379")
	for _, labels := range []bool{false, true} {
		for _, delay := range []string{"", "5s", "not-a-duration", "-1s", "1us", "25h"} {
			t.Run(fmt.Sprintf("labels=%t/delay=%s", labels, delay), func(t *testing.T) {
				readConfig := ""
				if delay != "" {
					readConfig += fmt.Sprintf("    stale_read_delay: %q\n", delay)
				}
				want := storage.TiKVConfig{PDAddresses: []string{"pd.example.com:2379"}}
				if labels {
					readConfig += "    read_labels:\n      region: local\n"
					want.ReadLabels = map[string]string{"region": "local"}
				}
				cfg, err := parseConfigData([]byte(`storage:
  state:
    kind: tikv
    pd_addresses: ["${TIKV_TEST_PD}"]
` + readConfig + `stores:
  peers:
    kind: keyvalue
    storage: state
    prefix: peers
`))
				if err != nil {
					t.Fatal(err)
				}
				physical, err := cfg.Storage["state"].runtimeConfig()
				if err != nil || !reflect.DeepEqual(physical, want) {
					t.Fatalf("parsed TiKV config=%+v %v; want %+v", physical, err, want)
				}
			})
		}
	}
}

func TestTiKVConfigurationRejectsUncleanLabels(t *testing.T) {
	for _, labels := range []map[string]string{
		{"": "local"}, {"region": ""}, {" region": "local"}, {"region": "local "}, {"\t": "local"}, {"region": "\n"},
	} {
		file := storageFileConfig{Kind: storage.KindTiKV, PDAddresses: []string{"pd:2379"}, ReadLabels: labels}
		if _, err := file.runtimeConfig(); err == nil {
			t.Fatalf("accepted unclean labels %+v", labels)
		}
	}
}
