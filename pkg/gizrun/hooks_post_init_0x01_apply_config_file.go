package gizrun

func init() {
	PostInitAt(0x01, func(ctx *RunContext) error {
		section, ok := ctx.config("gizrun")
		if !ok {
			return nil
		}
		config, ok := section.(gizrunConfig)
		if !ok {
			return nil
		}
		if config.DebugPort != nil {
			ctx.debugPort = *config.DebugPort
		}
		if config.Addr != "" {
			ctx.addr = config.Addr
		}
		if config.Pprof != nil {
			ctx.enablePprof = *config.Pprof
		}
		if config.Metrics != nil && config.Metrics.Push != nil {
			ctx.metricsPush = *config.Metrics.Push
		}
		return nil
	})
}
