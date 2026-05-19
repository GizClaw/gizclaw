package gizrun

import "github.com/GizClaw/gizclaw-go/pkg/gizrun/internal/configfile"

func init() {
	InitAt(0x01, func() error {
		RegisterConfigParser("env", configfile.ParseFunc(parseConfig[envConfig]))
		RegisterConfigParser("gizrun", configfile.ParseFunc(parseConfig[gizrunConfig]))
		RegisterConfigParser("volc_log", configfile.ParseFunc(parseConfig[volcLogConfig]))
		return nil
	})
}
