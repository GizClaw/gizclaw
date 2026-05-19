package gizrun

import "context"

func init() {
	ExitAt(0x01, func(ctx *RunContext) error {
		if !ctx.debugListening {
			return nil
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ctx.shutdownTimeout())
		defer cancel()
		err := ctx.debugApp.ShutdownWithContext(shutdownCtx)
		ctx.debugListening = false
		return err
	})
}
