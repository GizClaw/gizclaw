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

const (
	peerStreamLifecycleMessage          = "gizclaw: peer stream lifecycle"
	peerStreamLifecycleMaxRetainedTurns = 64
	peerStreamLifecycleMaxOutputRoutes  = 64
)

type peerStreamLifecycle struct {
	logger          *slog.Logger
	tunnelSessionID string
	peerPublicKey   string
	started         time.Time

	mu                 sync.Mutex
	seen               map[string]bool
	terminal           map[string]bool
	lastStage          string
	lastComponentStage map[string]string

	inputEventObserved  bool
	agentInputOpened    bool
	agentInputPushed    bool
	outputEventObserved bool

	nextTurnIndex uint64
	currentTurn   uint64
	turns         map[uint64]*peerStreamTurn
	turnOrder     []uint64
	inputTurns    map[string]uint64
	outputTurns   map[string]uint64
	outputOrder   []string
}

type peerStreamTurn struct {
	index   uint64
	started time.Time

	inputStreamIDHash  string
	outputStreamIDHash string
	lastStage          string
	seen               map[string]bool

	inputTerminalObserved  bool
	interruptObserved      bool
	agentInputPushed       bool
	outputEventObserved    bool
	outputTerminalObserved bool
	terminalPending        bool
}

type peerStreamLifecycleRecord struct {
	component string
	stage     string
	result    string
	reason    string
	turn      *peerStreamTurnSnapshot
}

type peerStreamTurnSnapshot struct {
	index                  uint64
	started                time.Time
	inputStreamIDHash      string
	outputStreamIDHash     string
	lastStage              string
	inputTerminalObserved  bool
	interruptObserved      bool
	agentInputPushed       bool
	outputEventObserved    bool
	outputTerminalObserved bool
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
		turns:              make(map[uint64]*peerStreamTurn),
		inputTurns:         make(map[string]uint64),
		outputTurns:        make(map[string]uint64),
	}
}

func (l *peerStreamLifecycle) accepted() {
	l.recordOnce("server_tunnel/session_accepted", "server_tunnel", "session_accepted")
}

func (l *peerStreamLifecycle) eventStreamAccepted() {
	l.recordOnce("peer_input/event_stream_accepted", "peer_input", "event_stream_accepted")
}

// observeInput records an authorized Peer input event. A BOS owns the turn
// boundary: later packets and terminal events are correlated through the
// input stream identifier without exposing it.
func (l *peerStreamLifecycle) observeInput(event *eventpb.PeerEvent) {
	if l == nil || event == nil {
		return
	}
	streamID := strings.TrimSpace(event.StreamID())
	var records []peerStreamLifecycleRecord
	l.mu.Lock()
	l.inputEventObserved = true
	if event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_BOS {
		records = append(records, l.startTurnLocked(streamID)...)
	}
	turn := l.inputTurnLocked(streamID)
	if turn != nil {
		if record := l.turnRecordOnceLocked(turn, "peer_input", "input_first_event", "success", ""); record != nil {
			records = append(records, *record)
		}
		if isPeerInputTerminal(event) {
			turn.inputTerminalObserved = true
			result, reason := peerStreamEventOutcome(event)
			if record := l.turnRecordOnceLocked(turn, "peer_input", "input_terminal", result, reason); record != nil {
				records = append(records, *record)
			}
			delete(l.inputTurns, streamID)
		}
	}
	l.mu.Unlock()
	l.logTurnRecords(records)
	l.recordOnce("peer_input/input_first_event", "peer_input", "input_first_event",
		slog.String("stream_id_hash", safeStreamIDHash(streamID)))
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
	streamID := streamIDFromChunk(chunk)
	var record *peerStreamLifecycleRecord
	l.mu.Lock()
	l.agentInputPushed = true
	turn := l.inputTurnLocked(streamID)
	if turn != nil {
		turn.agentInputPushed = true
		record = l.turnRecordOnceLocked(turn, "peer_input", "agent_input_first_push", "success", "")
	}
	l.mu.Unlock()
	if record != nil {
		l.logTurnRecord(*record)
	}
	l.recordOnce("peer_input/agent_input_first_push", "peer_input", "agent_input_first_push",
		slog.String("stream_id_hash", safeStreamIDHash(streamID)))
}

func (l *peerStreamLifecycle) observeInterrupt() {
	if l == nil {
		return
	}
	var record *peerStreamLifecycleRecord
	l.mu.Lock()
	if turn := l.turns[l.currentTurn]; turn != nil {
		turn.interruptObserved = true
		record = l.turnRecordOnceLocked(turn, "peer_input", "interrupt_observed", "interrupted", "control_interrupt")
	}
	l.mu.Unlock()
	if record != nil {
		l.logTurnRecord(*record)
	}
}

func (l *peerStreamLifecycle) observeOutput(ctx context.Context, chunk *genx.MessageChunk, workspaceName func(context.Context) string) {
	if l == nil || chunk == nil {
		return
	}
	streamID := streamIDFromChunk(chunk)
	workspace := ""
	if workspaceName != nil {
		workspace = strings.TrimSpace(workspaceName(ctx))
	}
	var records []peerStreamLifecycleRecord
	l.mu.Lock()
	l.outputEventObserved = true
	turn := l.outputTurnLocked(streamID)
	if turn != nil {
		turn.outputEventObserved = true
		if turn.outputStreamIDHash == "" {
			turn.outputStreamIDHash = safeStreamIDHash(streamID)
		}
		if record := l.turnRecordOnceLocked(turn, "agent_output", "output_first_event", "success", ""); record != nil {
			records = append(records, *record)
		}
		if chunk.IsEndOfStream() {
			turn.outputTerminalObserved = true
			result, reason := peerStreamChunkOutcome(chunk)
			if record := l.turnRecordOnceLocked(turn, "agent_output", "output_terminal", result, reason); record != nil {
				records = append(records, *record)
			}
			delete(l.outputTurns, streamID)
			if turn.terminalPending {
				records = append(records, l.terminateTurnLocked(turn, "replaced", "input_replaced"))
			} else {
				records = append(records, l.terminateTurnLocked(turn, result, reason))
			}
		}
	}
	l.mu.Unlock()
	for i := range records {
		if records[i].stage == "output_first_event" && workspace != "" {
			l.logTurnRecordWithAttrs(records[i], slog.String("workspace_name", workspace))
			continue
		}
		l.logTurnRecord(records[i])
	}
	attrs := make([]slog.Attr, 0, 2)
	if streamID != "" {
		attrs = append(attrs, slog.String("stream_id_hash", safeStreamIDHash(streamID)))
	}
	if workspace != "" {
		attrs = append(attrs, slog.String("workspace_name", workspace))
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
	var turnRecords []peerStreamLifecycleRecord
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
	if component == "agent_output" {
		turnRecords = l.terminateOutputTurnsLocked(result, reason)
	} else if component == "peer_input" || component == "server_tunnel" {
		turnRecords = l.terminateAllTurnsLocked(result, reason)
	}
	l.mu.Unlock()
	l.logTurnRecords(turnRecords)
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

func (l *peerStreamLifecycle) startTurnLocked(streamID string) []peerStreamLifecycleRecord {
	records := make([]peerStreamLifecycleRecord, 0, 4)
	if previous := l.turns[l.currentTurn]; previous != nil {
		previous.interruptObserved = true
		if record := l.turnRecordOnceLocked(previous, "peer_input", "interrupt_observed", "interrupted", "input_replaced"); record != nil {
			records = append(records, *record)
		}
		// Keep the replaced turn until its output route is observed or an owner
		// closes. Output is a single ordered stream, so the oldest replaced turn
		// without an assigned route owns the next previously unseen route.
		previous.terminalPending = true
	}
	l.nextTurnIndex++
	turn := &peerStreamTurn{
		index:             l.nextTurnIndex,
		started:           time.Now(),
		inputStreamIDHash: safeStreamIDHash(streamID),
		seen:              make(map[string]bool),
	}
	l.turns[turn.index] = turn
	l.turnOrder = append(l.turnOrder, turn.index)
	l.currentTurn = turn.index
	if streamID != "" {
		l.inputTurns[streamID] = turn.index
	}
	if record := l.turnRecordOnceLocked(turn, "peer_turn", "turn_started", "success", "accepted_input_bos"); record != nil {
		records = append(records, *record)
	}
	for len(l.turns) > peerStreamLifecycleMaxRetainedTurns {
		oldest := l.oldestTurnLocked()
		if oldest == nil {
			break
		}
		records = append(records, l.terminateTurnLocked(oldest, "incomplete", "state_limit"))
	}
	return records
}

func (l *peerStreamLifecycle) inputTurnLocked(streamID string) *peerStreamTurn {
	if streamID != "" {
		if turn := l.turns[l.inputTurns[streamID]]; turn != nil {
			return turn
		}
	}
	return l.turns[l.currentTurn]
}

func (l *peerStreamLifecycle) outputTurnLocked(streamID string) *peerStreamTurn {
	if streamID != "" {
		if turn := l.turns[l.outputTurns[streamID]]; turn != nil {
			return turn
		}
	}
	turn := l.oldestPendingOutputTurnLocked()
	if turn == nil {
		turn = l.turns[l.currentTurn]
	}
	if turn != nil && streamID != "" {
		l.outputTurns[streamID] = turn.index
		l.outputOrder = append(l.outputOrder, streamID)
		for len(l.outputTurns) > peerStreamLifecycleMaxOutputRoutes && len(l.outputOrder) > 0 {
			oldest := l.outputOrder[0]
			l.outputOrder = l.outputOrder[1:]
			delete(l.outputTurns, oldest)
		}
	}
	return turn
}

func (l *peerStreamLifecycle) oldestPendingOutputTurnLocked() *peerStreamTurn {
	for _, index := range l.turnOrder {
		turn := l.turns[index]
		if turn != nil && turn.terminalPending && !turn.outputEventObserved {
			return turn
		}
	}
	return nil
}

func (l *peerStreamLifecycle) oldestTurnLocked() *peerStreamTurn {
	for len(l.turnOrder) > 0 {
		index := l.turnOrder[0]
		l.turnOrder = l.turnOrder[1:]
		if turn := l.turns[index]; turn != nil {
			return turn
		}
	}
	return nil
}

func (l *peerStreamLifecycle) turnRecordOnceLocked(
	turn *peerStreamTurn,
	component string,
	stage string,
	result string,
	reason string,
) *peerStreamLifecycleRecord {
	if turn == nil || turn.seen[stage] {
		return nil
	}
	turn.seen[stage] = true
	turn.lastStage = stage
	return &peerStreamLifecycleRecord{
		component: component,
		stage:     stage,
		result:    result,
		reason:    reason,
		turn:      snapshotPeerStreamTurn(turn),
	}
}

func (l *peerStreamLifecycle) terminateTurnLocked(turn *peerStreamTurn, result, reason string) peerStreamLifecycleRecord {
	previousStage := turn.lastStage
	turn.lastStage = "turn_terminal"
	record := peerStreamLifecycleRecord{
		component: "peer_turn",
		stage:     "turn_terminal",
		result:    result,
		reason:    reason,
		turn:      snapshotPeerStreamTurn(turn),
	}
	record.turn.lastStage = previousStage
	l.releaseTurnLocked(turn.index)
	return record
}

func (l *peerStreamLifecycle) terminateAllTurnsLocked(result, reason string) []peerStreamLifecycleRecord {
	records := make([]peerStreamLifecycleRecord, 0, len(l.turns))
	seen := make(map[uint64]bool, len(l.turns))
	for _, index := range append([]uint64(nil), l.turnOrder...) {
		if turn := l.turns[index]; turn != nil {
			records = append(records, l.terminateTurnLocked(turn, result, reason))
			seen[index] = true
		}
	}
	for index, turn := range l.turns {
		if !seen[index] {
			records = append(records, l.terminateTurnLocked(turn, result, reason))
		}
	}
	return records
}

func (l *peerStreamLifecycle) terminateOutputTurnsLocked(result, reason string) []peerStreamLifecycleRecord {
	records := make([]peerStreamLifecycleRecord, 0, len(l.turns)*2)
	for len(l.turns) > 0 {
		turn := l.oldestTurnLocked()
		if turn == nil {
			for _, candidate := range l.turns {
				turn = candidate
				break
			}
		}
		if turn == nil {
			break
		}
		turn.outputTerminalObserved = true
		if record := l.turnRecordOnceLocked(turn, "agent_output", "output_terminal", result, reason); record != nil {
			records = append(records, *record)
		}
		records = append(records, l.terminateTurnLocked(turn, result, reason))
	}
	return records
}

func (l *peerStreamLifecycle) releaseTurnLocked(index uint64) {
	delete(l.turns, index)
	turnOrder := l.turnOrder[:0]
	for _, turnIndex := range l.turnOrder {
		if turnIndex != index {
			turnOrder = append(turnOrder, turnIndex)
		}
	}
	l.turnOrder = turnOrder
	for streamID, turnIndex := range l.inputTurns {
		if turnIndex == index {
			delete(l.inputTurns, streamID)
		}
	}
	for streamID, turnIndex := range l.outputTurns {
		if turnIndex == index {
			delete(l.outputTurns, streamID)
		}
	}
	outputOrder := l.outputOrder[:0]
	for _, streamID := range l.outputOrder {
		if _, retained := l.outputTurns[streamID]; retained {
			outputOrder = append(outputOrder, streamID)
		}
	}
	l.outputOrder = outputOrder
	if l.currentTurn == index {
		l.currentTurn = 0
	}
}

func snapshotPeerStreamTurn(turn *peerStreamTurn) *peerStreamTurnSnapshot {
	if turn == nil {
		return nil
	}
	return &peerStreamTurnSnapshot{
		index:                  turn.index,
		started:                turn.started,
		inputStreamIDHash:      turn.inputStreamIDHash,
		outputStreamIDHash:     turn.outputStreamIDHash,
		lastStage:              turn.lastStage,
		inputTerminalObserved:  turn.inputTerminalObserved,
		interruptObserved:      turn.interruptObserved,
		agentInputPushed:       turn.agentInputPushed,
		outputEventObserved:    turn.outputEventObserved,
		outputTerminalObserved: turn.outputTerminalObserved,
	}
}

func (l *peerStreamLifecycle) logTurnRecords(records []peerStreamLifecycleRecord) {
	for _, record := range records {
		l.logTurnRecord(record)
	}
}

func (l *peerStreamLifecycle) logTurnRecord(record peerStreamLifecycleRecord) {
	l.logTurnRecordWithAttrs(record)
}

func (l *peerStreamLifecycle) logTurnRecordWithAttrs(record peerStreamLifecycleRecord, extra ...slog.Attr) {
	if record.turn == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("component", record.component),
		slog.String("stage", record.stage),
		slog.String("result", record.result),
		slog.Uint64("turn_index", record.turn.index),
		slog.Int64("duration_ms", time.Since(record.turn.started).Milliseconds()),
	}
	if record.reason != "" {
		attrs = append(attrs, slog.String("reason", record.reason))
	}
	if record.turn.inputStreamIDHash != "" {
		attrs = append(attrs, slog.String("input_stream_id_hash", record.turn.inputStreamIDHash))
	}
	if record.turn.outputStreamIDHash != "" {
		attrs = append(attrs, slog.String("output_stream_id_hash", record.turn.outputStreamIDHash))
	}
	if record.stage == "turn_terminal" {
		attrs = append(attrs,
			slog.String("last_stage", record.turn.lastStage),
			slog.Bool("input_terminal_observed", record.turn.inputTerminalObserved),
			slog.Bool("interrupt_observed", record.turn.interruptObserved),
			slog.Bool("agent_input_pushed", record.turn.agentInputPushed),
			slog.Bool("output_event_observed", record.turn.outputEventObserved),
			slog.Bool("output_terminal_observed", record.turn.outputTerminalObserved),
		)
	}
	for _, attr := range extra {
		if attr.Value.Kind() == slog.KindString && strings.TrimSpace(attr.Value.String()) == "" {
			continue
		}
		attrs = append(attrs, attr)
	}
	l.log(attrs...)
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

func peerStreamEventOutcome(event *eventpb.PeerEvent) (string, string) {
	if event == nil || event.StreamError() == nil {
		return "success", "completed"
	}
	err := event.StreamError()
	return peerStreamControlOutcome(err.GetMessage(), err.GetCode())
}

func peerStreamChunkOutcome(chunk *genx.MessageChunk) (string, string) {
	if chunk == nil || chunk.Ctrl == nil {
		return "success", "completed"
	}
	return peerStreamControlOutcome(chunk.Ctrl.Error, chunk.Ctrl.ErrorCode)
}

func peerStreamControlOutcome(message, code string) (string, string) {
	message = strings.ToLower(strings.TrimSpace(message))
	code = strings.ToUpper(strings.TrimSpace(code))
	switch {
	case message == "" && code == "":
		return "success", "completed"
	case message == "interrupted" || code == "STREAM_INTERRUPTED":
		return "interrupted", "expected_interruption"
	case message == context.Canceled.Error() || code == "CANCELED" || code == "CANCELLED" ||
		code == "CONTEXT_CANCELED" || code == "CONTEXT_CANCELLED" ||
		code == "STREAM_CANCELED" || code == "STREAM_CANCELLED":
		return "canceled", "caller_canceled"
	case message == context.DeadlineExceeded.Error() || code == "DEADLINE_EXCEEDED" || code == "TIMEOUT":
		return "timeout", "deadline_exceeded"
	case message == io.EOF.Error() || code == "STREAM_CLOSED" || code == "CLOSED":
		return "closed", "stream_closed"
	default:
		return "runtime_error", "internal_error"
	}
}

func isPeerInputTerminal(event *eventpb.PeerEvent) bool {
	if event == nil {
		return false
	}
	return event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_EOS ||
		event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE
}

func streamIDFromChunk(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return ""
	}
	return strings.TrimSpace(chunk.Ctrl.StreamID)
}

func safeStreamIDHash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:16])
}
