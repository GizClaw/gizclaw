package doubaorealtime

import (
	"time"

	"github.com/GizClaw/doubao-speech-go"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// Model is the only Doubao Realtime Duplex protocol version with the required
// function-call contract.
const Model = doubaospeech.RealtimeDuplexModelDefault

// Config contains the complete runtime dependencies for one Doubao Realtime
// Agent. Pattern selects the registered provider primitive; Model selects the
// upstream function-call-capable protocol version.
type Config struct {
	Transformer genx.Transformer
	Pattern     string
	Model       string
	Toolkit     commonagent.Toolkit

	MaxToolCalls int
	ToolTimeout  time.Duration
}
