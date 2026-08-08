package server

import "testing"

func FuzzParseConfigData(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("listen: 127.0.0.1:9820\nendpoint: 127.0.0.1:9820\n"),
		[]byte("identity:\n  private-key: \"not-a-key\"\n"),
		[]byte("admin-public-key: \"not-a-key\"\n"),
		[]byte("log:\n  level: debug\n"),
		[]byte("friend_groups: {}\n"),
		[]byte("storage:\n  memory:\n    kind: keyvalue\n    memory: {}\nstores:\n  peers:\n    kind: keyvalue\n    storage: memory\n    prefix: peers\n"),
		[]byte("services:\n  peer:\n    store: peers\n    route_store: peer-routes\n    run_store: peer-run\n"),
		[]byte("storage:\n  analytics:\n    kind: sql\n    clickhouse:\n      dsn: ${DSN}\nstores:\n  history:\n    kind: log.mutable\n    storage: analytics\n    clickhouse:\n      database: default\n      table: history\nservices:\n  agent_host:\n    flowcraft:\n      history_store: history\n"),
		[]byte("listen: ["),
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			return
		}
		fileCfg, err := parseConfigData(data)
		if err != nil {
			return
		}
		cfg, err := mergeFileConfig(Config{}, fileCfg)
		if err != nil {
			t.Fatalf("mergeFileConfig() error = %v", err)
		}
		_ = cfg.validate()
	})
}
