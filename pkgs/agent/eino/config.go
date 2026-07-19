// Package eino implements the Eino-backed GizClaw Agent runtime.
package eino

import (
	"time"

	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/cloudwego/eino/components/model"
)

const defaultMaxSteps = 12

// HistoryConfig owns Eino's ordered conversation record schema. The store is
// supplied by the host but is not closed by the Agent.
type HistoryConfig struct {
	Store       logstore.MutableStore
	Stream      string
	RecentLimit int
}

// Config is the typed construction contract for an Eino Agent.
type Config struct {
	Model        model.ToolCallingChatModel
	Toolkit      commonagent.Toolkit
	History      *HistoryConfig
	Memory       memory.Store
	MemoryLimit  int
	SystemPrompt string
	MaxSteps     int
	MaxToolCalls int
	ToolTimeout  time.Duration
	Output       commonagent.OutputConfig
	// OnBackgroundError observes failures that happen after assistant content
	// has crossed the pull-visible boundary, such as final history persistence
	// or Memory observation. Such failures do not retract delivered output.
	OnBackgroundError func(error)
}
