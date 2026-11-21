package agenthost

import (
	"context"
	"log/slog"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
)

type mcpRejectReason uint8

const (
	mcpSchemaInvalid mcpRejectReason = iota
	mcpCatalogOrAuthUnavailable
)

var mcpRejectReasons = [...]string{
	mcpSchemaInvalid:            "schema_invalid",
	mcpCatalogOrAuthUnavailable: "catalog_or_auth_unavailable",
}

// Keep the summary bounded independently of the configured or proposed tools.
// Other catalog operations share the unknown bucket, including names that
// never matched a configured operation.
var mcpDiagnosticAliases = [...]string{
	"unknown",
	"litelink.call_tool.search_catalog",
	"litelink.call_tool.get_work",
	"litelink.call_tool.get_album",
}

type mcpDiagnosticsKey struct{}

type mcpDiagnostics struct {
	mu     sync.Mutex
	closed bool
	counts [len(mcpRejectReasons)][len(mcpDiagnosticAliases)]uint64
}

// BeginToolTurn scopes MCP diagnostics to one Eino turn, including its graph
// continuations. streamID is the turn's server-generated output route ID.
func (i *ToolkitInvoker) BeginToolTurn(ctx context.Context, streamID string) (context.Context, func()) {
	if i == nil || i.MCP == nil {
		return ctx, func() {}
	}
	diagnostics := &mcpDiagnostics{}
	logCtx := gizlog.WithStreamID(ctx, streamID)
	return context.WithValue(ctx, mcpDiagnosticsKey{}, diagnostics), func() {
		diagnostics.finish(logCtx)
	}
}

func recordMCPReject(ctx context.Context, operation mcpOperation, reason mcpRejectReason) {
	diagnostics, _ := ctx.Value(mcpDiagnosticsKey{}).(*mcpDiagnostics)
	if diagnostics == nil {
		return
	}
	alias := 0
	if operation.server == "litelink" && operation.operation == "call_tool" {
		switch operation.remote {
		case "search_catalog":
			alias = 1
		case "get_work":
			alias = 2
		case "get_album":
			alias = 3
		}
	}
	diagnostics.mu.Lock()
	defer diagnostics.mu.Unlock()
	if !diagnostics.closed {
		diagnostics.counts[reason][alias]++
	}
}

func (d *mcpDiagnostics) finish(ctx context.Context) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	counts := d.counts
	d.mu.Unlock()

	var reasons []slog.Attr
	for reason, byAlias := range counts {
		var aliases []slog.Attr
		for alias, count := range byAlias {
			if count != 0 {
				aliases = append(aliases, slog.Uint64(mcpDiagnosticAliases[alias], count))
			}
		}
		if len(aliases) != 0 {
			reasons = append(reasons, slog.Attr{Key: mcpRejectReasons[reason], Value: slog.GroupValue(aliases...)})
		}
	}
	if len(reasons) != 0 {
		slog.LogAttrs(ctx, slog.LevelInfo, "agenthost: MCP tool rejections", slog.Attr{Key: "rejections", Value: slog.GroupValue(reasons...)})
	}
}
