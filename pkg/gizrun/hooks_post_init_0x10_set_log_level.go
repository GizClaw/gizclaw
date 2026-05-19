package gizrun

func init() {
	PostInitAt(0x10, func(ctx *RunContext) error {
		if ctx.verbose.set {
			ctx.logLevel.Set(ctx.verbose.level)
		}
		return nil
	})
}
