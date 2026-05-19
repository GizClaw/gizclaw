package gizrun

import "context"

func init() {
	ExitAt(0x00, func(ctx *RunContext) error {
		if !ctx.publicListening {
			return nil
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), ctx.shutdownTimeout())
		defer cancel()
		err := ctx.publicApp.ShutdownWithContext(shutdownCtx)
		ctx.publicListening = false
		return err
	})
}
