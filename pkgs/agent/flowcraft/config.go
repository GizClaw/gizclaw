// Package flowcraft implements a GizClaw-owned Flowcraft graph Agent.
package flowcraft

import (
	"context"
	"time"

	flowgraph "github.com/GizClaw/flowcraft/sdk/graph"
	"github.com/GizClaw/flowcraft/sdk/graph/runner"
	"github.com/GizClaw/flowcraft/sdk/llm"
	flowworkspace "github.com/GizClaw/flowcraft/sdk/workspace"
	commonagent "github.com/GizClaw/gizclaw-go/pkgs/agent"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

// Config is the complete owned runtime contract. Product resources and
// credentials are resolved before construction.
type Config struct {
	ID           string
	Conversation string
	// HistoryWorkspace scopes persisted history independently from the runtime
	// agent ID. When empty, ID is used.
	HistoryWorkspace string
	Graph            flowgraph.GraphDefinition
	Resolver         llm.LLMResolver
	Workspace        flowworkspace.Workspace
	Toolkit          commonagent.Toolkit
	History          logstore.MutableStore
	Memory           memory.Store
	MemoryLimit      int
	PublishNodes     map[string]bool
	MaxIterations    int
	Parallel         runner.ParallelConfig
	MaxToolCalls     int
	ToolTimeout      time.Duration
	Output           commonagent.OutputConfig
	// ExternalOutputObservation defers pull-visible history and Memory updates
	// to BeginOutput and ObserveOutput. Embedded adapters use this when they
	// buffer or transform the core Agent output before returning it to a device.
	ExternalOutputObservation bool
	InputProvider             func(context.Context) (map[string]any, error)
	// OnBackgroundError observes failures that happen after assistant content
	// has crossed the pull-visible boundary, such as final history persistence
	// or Memory observation. Such failures do not retract delivered output.
	OnBackgroundError func(error)
}
