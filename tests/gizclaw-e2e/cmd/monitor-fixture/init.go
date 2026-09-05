package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func initialize(dir string) error {
	var env strings.Builder
	values := make(map[string]string)
	set := func(name, value string) {
		values[name] = value
		fmt.Fprintf(&env, "%s=%s\n", name, value)
	}
	for _, role := range []string{"SERVER", "EDGE", "ADMIN"} {
		key, err := giznet.GenerateKeyPair()
		if err != nil {
			return err
		}
		set("MONITOR_"+role+"_PRIVATE_KEY", key.Private.String())
		set("MONITOR_"+role+"_PUBLIC_KEY", key.Public.String())
		if role == "ADMIN" {
			set("GIZCLAW_E2E_ADMIN_PRIVATE_KEY", key.Private.String())
		}
	}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		return err
	}
	set("GIZCLAW_MONITOR_TOKEN", "gizclaw_mk_"+key.Private.String())
	for _, role := range []string{"server", "edge"} {
		root := filepath.Join(dir, role)
		if err := os.MkdirAll(root, 0700); err != nil {
			return err
		}
		config, err := os.ReadFile(filepath.Join("tests/gizclaw-e2e/docker/monitor", role+".yaml"))
		if err != nil {
			return err
		}
		var missing string
		resolved := os.Expand(string(config), func(name string) string {
			value, ok := values[name]
			if !ok {
				missing = name
			}
			return value
		})
		if missing != "" {
			return fmt.Errorf("unknown Monitor config variable %s", missing)
		}
		if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(resolved), 0600); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(dir, "runtime.env"), []byte(env.String()), 0600)
}
