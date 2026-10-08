// This Generator is included only by the local observer-lifecycle build overlay.
package peergenx

import (
	"context"
	"errors"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type lifecycleGenerator struct{}

func (lifecycleGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("fixture does not invoke tools")
}
func (lifecycleGenerator) GenerateStream(ctx context.Context, _ string, mc genx.ModelContext) (genx.Stream, error) {
	out := genx.NewGrowableStreamBuilder(mc, 128)
	stop := context.AfterFunc(ctx, func() { _ = out.Stream().CloseWithError(ctx.Err()) })
	go func() {
		defer stop()
		defer out.Done(genx.Usage{})
		id := genx.NewStreamID()
		for i := 0; i < 64; i++ {
			if err := out.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("Lifecycle fixture reply. "), Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: i == 0}}); err != nil {
				return
			}
			if err := latencyWait(ctx, 2*time.Millisecond); err != nil {
				return
			}
		}
	}()
	return out.Stream(), nil
}
