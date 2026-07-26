//go:build gizclaw_genx_e2e

package transformer

import (
	"context"
	"testing"
	"time"

	dashscope "github.com/GizClaw/dashscope-realtime-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/dashscoperealtime"
)

func TestDashScopeRealtimeToolInvokerContinuation(t *testing.T) {
	loadGenXE2EEnv(t)
	apiKey := firstEnv(dashScopeAPIKeyEnv, "GIZCLAW_E2E_DASHSCOPE_API_KEY")
	if apiKey == "" {
		t.Skipf("set %s in tests/genx-e2e/.env", dashScopeAPIKeyEnv)
	}

	invoker := &realtimeE2EToolInvoker{}
	transformer, err := dashscoperealtime.New(dashscoperealtime.Config{
		Client:       dashscope.NewClient(apiKey),
		Model:        dashscope.ModelQwen35OmniPlusRealtime,
		VAD:          dashscope.VADModeDisabled,
		Instructions: realtimeToolInstructions,
		ToolInvoker:  invoker,
		MaxToolCalls: realtimeToolCallLimit,
	})
	if err != nil {
		t.Fatalf("dashscoperealtime.New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	defer input.CloseWithError(context.Canceled)
	output, err := transformer.Transform(ctx, input)
	if err != nil {
		t.Fatalf("DashScope Transform() failed: %v", err)
	}
	defer output.CloseWithError(context.Canceled)

	packets := embeddedToolPromptOpusPackets(t)
	feedDone := make(chan error, 1)
	go func() {
		feedDone <- pushDashScopeToolTurn(
			ctx,
			input,
			"dashscope-tool-invoker-e2e",
			packets,
		)
	}()
	response := waitForRealtimeToolContinuation(t, ctx, output, feedDone)
	assertRealtimeToolCalls(t, invoker)
	t.Logf(
		"DashScope Realtime tool calls=%d continuation response=%q",
		len(invoker.snapshot()),
		response,
	)
}
