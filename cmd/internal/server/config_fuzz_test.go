package server

import "testing"

func FuzzParseConfigData(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("listen: 127.0.0.1:9820\nendpoint: 127.0.0.1:9820\n"),
		[]byte("identity:\n  private-key: \"not-a-key\"\n"),
		[]byte("admin-public-key: \"not-a-key\"\n"),
		[]byte("log:\n  level: debug\n"),
		[]byte("friend_groups:\n  message_default_ttl: 24h\n  message_max_ttl: 7d\n  message_cleanup_interval: 5m\n  message_max_audio_bytes: 2097152\n"),
		[]byte("storage:\n  memory:\n    kind: keyvalue\n    memory: {}\nstores:\n  peers:\n    kind: keyvalue\n    storage: memory\n    prefix: peers\n"),
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
		cfg, err := mergeFileConfig(DefaultConfig(), fileCfg)
		if err != nil {
			t.Fatalf("mergeFileConfig() error = %v", err)
		}
		_ = cfg.validate()
	})
}
