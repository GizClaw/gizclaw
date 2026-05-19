package gizrun

func init() {
	ExitAt(0x10, func(ctx *RunContext) error {
		if ctx.metricsPusher == nil {
			return nil
		}
		ctx.metricsPusher.stopAndWait()
		ctx.metricsPusher = nil
		return nil
	})
}
