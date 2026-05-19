package gizrun

import "fmt"

func init() {
	PostInitAt(0x00, func(ctx *RunContext) error {
		section, ok := ctx.config("env")
		if !ok {
			return nil
		}
		config, ok := section.(envConfig)
		if !ok {
			return fmt.Errorf("gizrun env: unexpected config type %T", section)
		}
		return config.apply()
	})
}
