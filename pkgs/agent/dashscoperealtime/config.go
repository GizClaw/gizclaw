package dashscoperealtime

import (
	"time"

	"github.com/GizClaw/dashscope-realtime-go"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// DefaultModel is the Qwen3.5 realtime model used when Config.Model is empty.
const DefaultModel = dashscope.ModelQwen35OmniFlashRealtime

// Config contains the complete runtime dependencies for one DashScope
// Realtime Agent.
type Config struct {
	Transformer genx.Transformer
	Pattern     string
	Model       string
	Toolkit     commonagent.Toolkit

	MaxToolCalls int
	ToolTimeout  time.Duration
}
