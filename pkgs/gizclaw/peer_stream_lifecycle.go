package gizclaw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
)

const peerStreamLifecycleMessage = "gizclaw: peer stream lifecycle"

type peerStreamLifecycle struct {
	logger          *slog.Logger
	tunnelSessionID string
	peerPublicKey   string
	started         time.Time

	mu                  sync.Mutex
	seen                map[string]bool
	terminal            map[string]bool
	lastStage           string
	lastComponentStage  map[string]string
	inputEventObserved  bool
	agentInputOpened    bool
	agentInputPushed    bool
	outputEventObserved bool
}

func newPeerStreamLifecycle(logger *slog.Logger, tunnelSessionID, peerPublicKey string) *peerStreamLifecycle {
	if logger == nil {
		logger = slog.Default()
	}
	return &peerStreamLifecycle{
		logger:             logger,
		tunnelSessionID:    strings.TrimSpace(tunnelSessionID),
		peerPublicKey:      strings.TrimSpace(peerPublicKey),
		started:            time.Now(),
		seen:               make(map[string]bool),
		terminal:           make(map[string]bool),
		lastComponentStage: make(map[string]string),
	}
}

func (l *peerStreamLifecycle) accepted() {
	l.recordOnce("server_tunnel/session_accepted", "server_tunnel", "session_accepted")
}

func (l *peerStreamLifecycle) eventStreamAccepted() {
	l.recordOnce("peer_input/event_stream_accepted", "peer_input", "event_stream_accepted")
}

func (l *peerStreamLifecycle) observeInput(event *eventpb.PeerEvent) {
	if l == nil || event == nil {
		return
	}
	l.mu.Lock()
	l.inputEventObserved = true
	l.mu.Unlock()
	l.recordOnce("peer_input/input_first_event", "peer_input", "input_first_event",
		slog.String("stream_id_hash", safeStreamIDHash(event.StreamID())))
}

func (l *peerStreamLifecycle) observeAgentInputOpen() {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.agentInputOpened = true
	l.mu.Unlock()
	l.recordOnce("peer_input/agent_input_opened", "peer_input", "agent_input_opened")
}

func (l *peerStreamLifecycle) observeAgentInputPush(chunk *genx.MessageChunk) {
	if l == nil || chunk == nil {
		return
	}
	l.mu.Lock()
	l.agentInputPushed = true
	l.mu.Unlock()
	if chunk.Ctrl == nil {
		l.recordOnce("peer_input/agent_input_first_push", "peer_input", "agent_input_first_push")
		return
	}
	l.recordOnce("peer_input/agent_input_first_push", "peer_input", "agent_input_first_push",
		slog.String("stream_id_hash", safeStreamIDHash(chunk.Ctrl.StreamID)))
}

func (l *peerStreamLifecycle) observeOutput(ctx context.Context, chunk *genx.MessageChunk, workspaceName func(context.Context) string) {
	if l == nil || chunk == nil {
		return
	}
	l.mu.Lock()
	l.outputEventObserved = true
	l.mu.Unlock()
	attrs := make([]slog.Attr, 0, 2)
	if chunk.Ctrl != nil {
		attrs = append(attrs, slog.String("stream_id_hash", safeStreamIDHash(chunk.Ctrl.StreamID)))
	}
	if workspaceName != nil {
		attrs = append(attrs, slog.String("workspace_name", strings.TrimSpace(workspaceName(ctx))))
	}
	l.recordOnce("agent_output/output_first_event", "agent_output", "output_first_event", attrs...)
}

func (l *peerStreamLifecycle) finish(component string, err error) {
	if l == nil {
		return
	}
	component = strings.TrimSpace(component)
	if component == "" {
		return
	}
	result, reason := peerStreamLifecycleResult(err)
	l.mu.Lock()
	if l.terminal[component] {
		l.mu.Unlock()
		return
	}
	l.terminal[component] = true
	lastStage := l.lastComponentStage[component]
	if component == "server_tunnel" {
		lastStage = l.lastStage
	}
	inputObserved := l.inputEventObserved
	inputOpened := l.agentInputOpened
	inputPushed := l.agentInputPushed
	outputObserved := l.outputEventObserved
	l.mu.Unlock()
	l.log(
		slog.String("component", component),
		slog.String("stage", "terminal"),
		slog.String("result", result),
		slog.String("reason", reason),
		slog.Int64("duration_ms", time.Since(l.started).Milliseconds()),
		slog.String("last_stage", lastStage),
		slog.Bool("input_event_observed", inputObserved),
		slog.Bool("agent_input_opened", inputOpened),
		slog.Bool("agent_input_pushed", inputPushed),
		slog.Bool("output_event_observed", outputObserved),
	)
}

func (l *peerStreamLifecycle) recordOnce(key, component, stage string, attrs ...slog.Attr) {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.seen[key] {
		l.mu.Unlock()
		return
	}
	l.seen[key] = true
	l.lastStage = stage
	l.lastComponentStage[component] = stage
	l.mu.Unlock()
	recordAttrs := []slog.Attr{
		slog.String("component", component),
		slog.String("stage", stage),
		slog.String("result", "success"),
		slog.Int64("duration_ms", time.Since(l.started).Milliseconds()),
	}
	for _, attr := range attrs {
		if attr.Value.Kind() == slog.KindString && strings.TrimSpace(attr.Value.String()) == "" {
			continue
		}
		recordAttrs = append(recordAttrs, attr)
	}
	l.log(recordAttrs...)
}

func (l *peerStreamLifecycle) log(attrs ...slog.Attr) {
	if l == nil || l.logger == nil {
		return
	}
	if l.tunnelSessionID != "" {
		attrs = append(attrs, slog.String("tunnel_session_id", l.tunnelSessionID))
	}
	if l.peerPublicKey != "" {
		attrs = append(attrs, slog.String("peer_public_key", l.peerPublicKey))
	}
	l.logger.LogAttrs(context.Background(), slog.LevelInfo, peerStreamLifecycleMessage, attrs...)
}

func peerStreamLifecycleResult(err error) (string, string) {
	switch {
	case err == nil:
		return "success", "completed"
	case errors.Is(err, context.Canceled):
		return "canceled", "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout", "deadline_exceeded"
	case errors.Is(err, io.EOF), isPeerServiceClosed(err):
		return "closed", "stream_closed"
	default:
		return "runtime_error", "internal_error"
	}
}

func safeStreamIDHash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}
