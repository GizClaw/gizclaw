package gizclaw

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"slices"
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
	epochTurns    map[*genx.ResponseEpoch]uint64
	epochOrder    []*genx.ResponseEpoch
}

type peerStreamTurn struct {
	index   uint64
	started time.Time

	inputStreamIDHash   string
	outputStreamIDHash  string
	lastStage           string
	lastAgentStage      string
	seen                map[string]bool
	producedModalities  map[string]bool
	deliveredModalities map[string]bool
	sourcePartClasses   map[string]bool
	sourceLabelClasses  map[string]bool
	peerEventTypes      map[string]bool
	peerEventKinds      map[string]bool
	peerEventLabels     map[string]bool

	inputTerminalObserved  bool
	interruptObserved      bool
	agentInputPushed       bool
	agentTransformStarted  bool
	outputEventObserved    bool
	outputTerminalObserved bool
	agentTerminalObserved  bool
	assistantEpochBound    bool
	terminalPending        bool
}

type peerStreamLifecycleRecord struct {
	component      string
	stage          string
	result         string
	reason         string
	outputModality string
	terminalClass  string
	turn           *peerStreamTurnSnapshot
}

type peerStreamTurnSnapshot struct {
	index                  uint64
	started                time.Time
	inputStreamIDHash      string
	outputStreamIDHash     string
	lastStage              string
	lastAgentStage         string
	inputTerminalObserved  bool
	interruptObserved      bool
	agentInputPushed       bool
	agentTransformStarted  bool
	outputEventObserved    bool
	outputTerminalObserved bool
	agentTerminalObserved  bool
	producedModalities     string
	deliveredModalities    string
	sourcePartClasses      string
	sourceLabelClasses     string
	peerEventTypes         string
	peerEventKinds         string
	peerEventLabelClasses  string
}

func newPeerStreamLifecycle(logger *slog.Logger, tunnelSessionID, peerPublicKey string) *peerStreamLifecycle {
	logger, enabled := peerStreamLifecycleLogger(logger)
	if !enabled {
		return nil
	}
	return newEnabledPeerStreamLifecycle(logger, tunnelSessionID, peerPublicKey)
}

func peerStreamLifecycleLogger(logger *slog.Logger) (*slog.Logger, bool) {
	if logger == nil {
		logger = slog.Default()
	}
	return logger, logger.Enabled(context.Background(), slog.LevelInfo)
}

func newEnabledPeerStreamLifecycle(logger *slog.Logger, tunnelSessionID, peerPublicKey string) *peerStreamLifecycle {
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
		epochTurns:         make(map[*genx.ResponseEpoch]uint64),
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

func (l *peerStreamLifecycle) observeAgentTransformStarted(chunk *genx.MessageChunk) {
	if l == nil || chunk == nil {
		return
	}
	streamID := streamIDFromChunk(chunk)
	var record *peerStreamLifecycleRecord
	l.mu.Lock()
	turn := l.inputTurnLocked(streamID)
	if turn != nil {
		turn.agentTransformStarted = true
		record = l.turnRecordOnceLocked(turn, "agent_runtime", "agent_transform_started", "success", "")
	}
	l.mu.Unlock()
	if record != nil {
		l.logTurnRecord(*record)
	}
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
	for _, event := range peerStreamEventsFromChunk(chunk) {
		l.observePeerEventDelivered(ctx, chunk, event, workspaceName)
	}
	l.observeOutputDrained(chunk)
}

func (l *peerStreamLifecycle) observePeerEventDelivered(
	ctx context.Context,
	chunk *genx.MessageChunk,
	event *eventpb.PeerEvent,
	workspaceName func(context.Context) string,
) {
	if l == nil || chunk == nil || event == nil {
		return
	}
	streamID := event.StreamID()
	workspace := ""
	if workspaceName != nil {
		workspace = strings.TrimSpace(workspaceName(ctx))
	}
	var records []peerStreamLifecycleRecord
	l.mu.Lock()
	l.outputEventObserved = true
	turn := l.outputTurnLocked(chunk)
	if turn != nil {
		turn.outputEventObserved = true
		if turn.outputStreamIDHash == "" {
			turn.outputStreamIDHash = safeStreamIDHash(streamID)
		}
		if record := l.turnRecordOnceLocked(turn, "agent_output", "output_first_event", "success", ""); record != nil {
			records = append(records, *record)
		}
		if isAgentObservableOutputChunk(chunk) {
			modality := peerEventOutputModality(chunk, event)
			turn.peerEventTypes[peerEventTypeClass(event)] = true
			turn.peerEventKinds[peerEventKindClass(event)] = true
			turn.peerEventLabels[peerLabelClass(event.Label())] = true
			if modality != "" {
				turn.deliveredModalities[modality] = true
				if record := l.turnOutputRecordOnceLocked(turn, "agent_output_delivered", modality); record != nil {
					records = append(records, *record)
				}
			}
			if event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE && modality == "assistant_text" {
				turn.deliveredModalities["assistant_eos"] = true
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

// observeOutputDrained records the final consumer boundary after all Peer
// broadcasts for chunk have succeeded. Audio reaches this point only after its
// mixer track drains, including when aggregate audio suppresses a source EOS.
func (l *peerStreamLifecycle) observeOutputDrained(chunk *genx.MessageChunk) {
	if l == nil || chunk == nil || chunk.Ctrl == nil || !chunk.Ctrl.ResponseEpochEnd {
		return
	}
	var records []peerStreamLifecycleRecord
	l.mu.Lock()
	turn := l.outputTurnLocked(chunk)
	if turn != nil && turn.agentTerminalObserved && !turn.outputTerminalObserved {
		turn.outputTerminalObserved = true
		result, reason := peerStreamChunkOutcome(chunk)
		if record := l.turnRecordOnceLocked(turn, "agent_output", "output_terminal", result, reason); record != nil {
			records = append(records, *record)
		}
		delete(l.outputTurns, streamIDFromChunk(chunk))
		if turn.terminalPending {
			records = append(records, l.terminateTurnLocked(turn, "replaced", "input_replaced"))
		} else {
			records = append(records, l.terminateTurnLocked(turn, result, reason))
		}
	}
	l.mu.Unlock()
	l.logTurnRecords(records)
}

// observeOutputProduced runs at the producer boundary, before the independently
// scheduled consumer and any audio drain. Only the first observable modality
// is logged; later chunks update a bounded terminal snapshot.
func (l *peerStreamLifecycle) observeOutputProduced(chunk *genx.MessageChunk) {
	if l == nil || chunk == nil {
		return
	}
	streamID := streamIDFromChunk(chunk)
	var records []peerStreamLifecycleRecord
	l.mu.Lock()
	turn := l.outputTurnLocked(chunk)
	if turn != nil {
		if turn.outputStreamIDHash == "" {
			turn.outputStreamIDHash = safeStreamIDHash(streamID)
		}
		if isAgentObservableOutputChunk(chunk) {
			modality := peerAgentOutputModality(chunk)
			turn.producedModalities[modality] = true
			turn.sourcePartClasses[peerSourcePartClass(chunk)] = true
			turn.sourceLabelClasses[peerSourceLabelClass(chunk)] = true
			if record := l.turnOutputRecordOnceLocked(turn, "agent_output_produced", modality); record != nil {
				records = append(records, *record)
			}
		}
		if isAssistantOutputChunk(chunk) {
			turn.assistantEpochBound = true
		}
		terminal := chunk.Ctrl != nil && chunk.Ctrl.ResponseEpochEnd && isAssistantOutputChunk(chunk)
		if terminal && !turn.agentTerminalObserved {
			turn.agentTerminalObserved = true
			result, reason := peerStreamChunkOutcome(chunk)
			if record := l.turnRecordOnceLocked(turn, "agent_runtime", "agent_terminal", result, reason); record != nil {
				record.terminalClass = peerAgentTerminalClass(chunk, nil)
				records = append(records, *record)
			}
		}
	}
	l.mu.Unlock()
	l.logTurnRecords(records)
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
		turnRecords = l.terminateAllTurnsLocked(component, result, reason)
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
		waitingForAssistantDelivery := previous.agentInputPushed || previous.agentTransformStarted ||
			previous.assistantEpochBound || (previous.agentTerminalObserved && !previous.outputTerminalObserved)
		if waitingForAssistantDelivery {
			previous.terminalPending = true
		} else {
			if record := l.ensureAgentTerminalLocked(previous, "interrupted", "input_replaced", "interrupted"); record != nil {
				records = append(records, *record)
			}
			records = append(records, l.terminateTurnLocked(previous, "replaced", "input_replaced"))
		}
	}
	l.nextTurnIndex++
	turn := &peerStreamTurn{
		index:               l.nextTurnIndex,
		started:             time.Now(),
		inputStreamIDHash:   safeStreamIDHash(streamID),
		seen:                make(map[string]bool),
		producedModalities:  make(map[string]bool),
		deliveredModalities: make(map[string]bool),
		sourcePartClasses:   make(map[string]bool),
		sourceLabelClasses:  make(map[string]bool),
		peerEventTypes:      make(map[string]bool),
		peerEventKinds:      make(map[string]bool),
		peerEventLabels:     make(map[string]bool),
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
		if record := l.ensureAgentTerminalLocked(oldest, "incomplete", "state_limit", "stream_error"); record != nil {
			records = append(records, *record)
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

func (l *peerStreamLifecycle) outputTurnLocked(chunk *genx.MessageChunk) *peerStreamTurn {
	if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.ResponseEpoch == nil {
		return nil
	}
	epoch := chunk.Ctrl.ResponseEpoch
	if turn := l.turns[l.epochTurns[epoch]]; turn != nil {
		return turn
	}
	owner := epoch.InputStreamID()
	turn := l.turns[l.inputTurns[owner]]
	if owner == "" || turn == nil {
		return nil
	}
	l.epochTurns[epoch] = turn.index
	l.epochOrder = append(l.epochOrder, epoch)
	streamID := streamIDFromChunk(chunk)
	if streamID != "" {
		l.bindOutputRouteLocked(streamID, turn)
	}
	for len(l.epochTurns) > peerStreamLifecycleMaxOutputRoutes && len(l.epochOrder) > 0 {
		oldest := l.epochOrder[0]
		l.epochOrder = l.epochOrder[1:]
		delete(l.epochTurns, oldest)
	}
	return turn
}

func (l *peerStreamLifecycle) bindOutputRouteLocked(streamID string, turn *peerStreamTurn) {
	if streamID == "" || turn == nil {
		return
	}
	l.outputTurns[streamID] = turn.index
	l.outputOrder = append(l.outputOrder, streamID)
	for len(l.outputTurns) > peerStreamLifecycleMaxOutputRoutes && len(l.outputOrder) > 0 {
		oldest := l.outputOrder[0]
		l.outputOrder = l.outputOrder[1:]
		delete(l.outputTurns, oldest)
	}
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
	if component == "agent_runtime" && stage != "agent_terminal" {
		turn.lastAgentStage = stage
	}
	return &peerStreamLifecycleRecord{
		component: component,
		stage:     stage,
		result:    result,
		reason:    reason,
		turn:      snapshotPeerStreamTurn(turn),
	}
}

func (l *peerStreamLifecycle) turnOutputRecordOnceLocked(turn *peerStreamTurn, stage, modality string) *peerStreamLifecycleRecord {
	if turn == nil {
		return nil
	}
	if modality == "" {
		modality = "other"
	}
	if stage != "agent_output_produced" && stage != "agent_output_delivered" {
		return nil
	}
	if turn.seen[stage] {
		return nil
	}
	turn.seen[stage] = true
	turn.lastStage = stage
	turn.lastAgentStage = stage
	return &peerStreamLifecycleRecord{
		component:      "agent_runtime",
		stage:          stage,
		result:         "success",
		outputModality: modality,
		turn:           snapshotPeerStreamTurn(turn),
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

func (l *peerStreamLifecycle) terminateAllTurnsLocked(component, result, reason string) []peerStreamLifecycleRecord {
	records := make([]peerStreamLifecycleRecord, 0, len(l.turns)*2)
	seen := make(map[uint64]bool, len(l.turns))
	for _, index := range append([]uint64(nil), l.turnOrder...) {
		if turn := l.turns[index]; turn != nil {
			if record := l.ensureAgentTerminalLocked(turn, result, reason, peerAgentTerminalClassForLifecycle(component, reason)); record != nil {
				records = append(records, *record)
			}
			records = append(records, l.terminateTurnLocked(turn, result, reason))
			seen[index] = true
		}
	}
	for index, turn := range l.turns {
		if !seen[index] {
			if record := l.ensureAgentTerminalLocked(turn, result, reason, peerAgentTerminalClassForLifecycle(component, reason)); record != nil {
				records = append(records, *record)
			}
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
		if record := l.ensureAgentTerminalLocked(turn, result, reason, peerAgentTerminalClass(nil, lifecycleErrorForReason(reason))); record != nil {
			records = append(records, *record)
		}
		turn.outputTerminalObserved = true
		if record := l.turnRecordOnceLocked(turn, "agent_output", "output_terminal", result, reason); record != nil {
			records = append(records, *record)
		}
		records = append(records, l.terminateTurnLocked(turn, result, reason))
	}
	return records
}

func (l *peerStreamLifecycle) ensureAgentTerminalLocked(turn *peerStreamTurn, result, reason, terminalClass string) *peerStreamLifecycleRecord {
	if turn == nil || turn.agentTerminalObserved {
		return nil
	}
	turn.agentTerminalObserved = true
	record := l.turnRecordOnceLocked(turn, "agent_runtime", "agent_terminal", result, reason)
	if record != nil {
		record.terminalClass = terminalClass
	}
	return record
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
	for epoch, turnIndex := range l.epochTurns {
		if turnIndex == index {
			delete(l.epochTurns, epoch)
		}
	}
	epochOrder := l.epochOrder[:0]
	for _, epoch := range l.epochOrder {
		if _, retained := l.epochTurns[epoch]; retained {
			epochOrder = append(epochOrder, epoch)
		}
	}
	l.epochOrder = epochOrder
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
		lastAgentStage:         turn.lastAgentStage,
		inputTerminalObserved:  turn.inputTerminalObserved,
		interruptObserved:      turn.interruptObserved,
		agentInputPushed:       turn.agentInputPushed,
		agentTransformStarted:  turn.agentTransformStarted,
		outputEventObserved:    turn.outputEventObserved,
		outputTerminalObserved: turn.outputTerminalObserved,
		agentTerminalObserved:  turn.agentTerminalObserved,
		producedModalities:     joinedModalities(turn.producedModalities),
		deliveredModalities:    joinedModalities(turn.deliveredModalities),
		sourcePartClasses:      joinedClasses(turn.sourcePartClasses),
		sourceLabelClasses:     joinedClasses(turn.sourceLabelClasses),
		peerEventTypes:         joinedClasses(turn.peerEventTypes),
		peerEventKinds:         joinedClasses(turn.peerEventKinds),
		peerEventLabelClasses:  joinedClasses(turn.peerEventLabels),
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
	if record.outputModality != "" {
		attrs = append(attrs, slog.String("output_modality", record.outputModality))
	}
	if record.terminalClass != "" {
		attrs = append(attrs, slog.String("terminal_class", record.terminalClass))
	}
	if record.turn.inputStreamIDHash != "" {
		attrs = append(attrs, slog.String("input_stream_id_hash", record.turn.inputStreamIDHash))
	}
	if record.turn.outputStreamIDHash != "" {
		attrs = append(attrs, slog.String("output_stream_id_hash", record.turn.outputStreamIDHash))
	}
	if record.turn.lastAgentStage != "" && (record.stage == "agent_terminal" || record.stage == "turn_terminal") {
		attrs = append(attrs, slog.String("last_agent_stage", record.turn.lastAgentStage))
	}
	if record.stage == "turn_terminal" || record.stage == "agent_terminal" {
		attrs = append(attrs,
			slog.String("last_stage", record.turn.lastStage),
			slog.Bool("input_terminal_observed", record.turn.inputTerminalObserved),
			slog.Bool("interrupt_observed", record.turn.interruptObserved),
			slog.Bool("agent_input_pushed", record.turn.agentInputPushed),
			slog.Bool("agent_transform_started", record.turn.agentTransformStarted),
			slog.Bool("output_event_observed", record.turn.outputEventObserved),
			slog.Bool("output_terminal_observed", record.turn.outputTerminalObserved),
			slog.Bool("agent_terminal_observed", record.turn.agentTerminalObserved),
			slog.String("produced_modalities", record.turn.producedModalities),
			slog.String("delivered_modalities", record.turn.deliveredModalities),
			slog.String("source_part_classes", record.turn.sourcePartClasses),
			slog.String("source_label_classes", record.turn.sourceLabelClasses),
			slog.String("peer_event_types", record.turn.peerEventTypes),
			slog.String("peer_event_kinds", record.turn.peerEventKinds),
			slog.String("peer_event_label_classes", record.turn.peerEventLabelClasses),
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

func peerAgentOutputModality(chunk *genx.MessageChunk) string {
	if chunk == nil {
		return "other"
	}
	label, name := "", strings.ToLower(strings.TrimSpace(chunk.Name))
	if chunk.Ctrl != nil {
		label = strings.ToLower(strings.TrimSpace(chunk.Ctrl.Label))
	}
	if label == "transcript" || name == "transcript" || chunk.Role == genx.RoleUser {
		if _, ok := chunk.Part.(genx.Text); ok || chunk.IsEndOfStream() {
			return "transcript_text"
		}
	}
	if chunk.IsEndOfStream() {
		if result, _ := peerStreamChunkOutcome(chunk); result == "interrupted" {
			return "interrupt"
		}
		return "assistant_eos"
	}
	if _, ok := chunk.Part.(genx.Text); ok {
		return "assistant_text"
	}
	if mimeType, ok := chunk.MIMEType(); ok {
		mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
		if strings.HasPrefix(mediaType, "audio/") || mediaType == "application/ogg" {
			return "assistant_audio"
		}
	}
	if chunk.Ctrl != nil && chunk.Part == nil && chunk.ToolCall == nil {
		return "control"
	}
	return "other"
}

func isAgentObservableOutputChunk(chunk *genx.MessageChunk) bool {
	if chunk == nil || chunk.Ctrl == nil {
		return chunk != nil
	}
	label := strings.TrimSpace(chunk.Ctrl.Label)
	return label != genx.HistoryUserAudioLabel && label != peerStreamEventHistoryUpdatedLabel
}

func isAssistantOutputChunk(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return false
	}
	if chunk.Role == genx.RoleUser || strings.EqualFold(strings.TrimSpace(chunk.Name), "transcript") {
		return false
	}
	if chunk.Ctrl != nil && strings.EqualFold(strings.TrimSpace(chunk.Ctrl.Label), "transcript") {
		return false
	}
	return chunk.Role == genx.RoleModel || peerAssistantRoute(chunk) != "" ||
		(chunk.Ctrl != nil && chunk.IsEndOfStream() && chunk.Part == nil)
}

func peerAssistantRoute(chunk *genx.MessageChunk) string {
	if chunk == nil {
		return ""
	}
	if mimeType, ok := chunk.MIMEType(); ok {
		mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
		switch {
		case mediaType == "text/plain":
			return "text"
		case strings.HasPrefix(mediaType, "audio/") || mediaType == "application/ogg":
			return "audio"
		default:
			return "other"
		}
	}
	return ""
}

func peerAgentTerminalClass(chunk *genx.MessageChunk, err error) string {
	if chunk != nil && chunk.Ctrl != nil {
		code := strings.ToUpper(strings.TrimSpace(chunk.Ctrl.ErrorCode))
		result, _ := peerStreamChunkOutcome(chunk)
		switch {
		case chunk.Ctrl.FailureClass == genx.FailureClassProvider:
			return "provider_error"
		case chunk.Ctrl.FailureClass == genx.FailureClassTransform:
			return "transform_error"
		case result == "interrupted":
			return "interrupted"
		case result == "canceled":
			return "caller_canceled"
		case result == "timeout":
			return "deadline_exceeded"
		case code == "AGENT_RELOAD_FAILED":
			return "transform_error"
		case result == "runtime_error":
			return "stream_error"
		case result == "closed":
			return "stream_error"
		default:
			return "completed"
		}
	}
	if class, ok := genx.FailureClassOf(err); ok {
		switch class {
		case genx.FailureClassProvider:
			return "provider_error"
		case genx.FailureClassTransform:
			return "transform_error"
		}
	}
	switch {
	case err == nil:
		return "completed"
	case errors.Is(err, context.Canceled):
		return "caller_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "stream_error"
	}
}

func lifecycleErrorForReason(reason string) error {
	switch reason {
	case "context_canceled", "caller_canceled":
		return context.Canceled
	case "deadline_exceeded":
		return context.DeadlineExceeded
	default:
		return errors.New("bounded lifecycle failure")
	}
}

func peerAgentTerminalClassForLifecycle(component, reason string) string {
	switch reason {
	case "deadline_exceeded":
		return "deadline_exceeded"
	case "context_canceled", "caller_canceled":
		return "caller_canceled"
	case "stream_closed":
		if component == "peer_input" || component == "server_tunnel" {
			return "caller_canceled"
		}
	}
	return "stream_error"
}

func joinedModalities(values map[string]bool) string {
	order := []string{"transcript_text", "assistant_text", "assistant_audio", "assistant_eos", "interrupt", "control", "other"}
	selected := make([]string, 0, len(values))
	for _, value := range order {
		if values[value] {
			selected = append(selected, value)
		}
	}
	return strings.Join(selected, ",")
}

func joinedClasses(values map[string]bool) string {
	selected := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			selected = append(selected, value)
		}
	}
	slices.Sort(selected)
	return strings.Join(selected, ",")
}

func peerSourcePartClass(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Part == nil {
		return "control"
	}
	switch chunk.Part.(type) {
	case genx.Text:
		return "text"
	case *genx.Blob:
		if route := peerAssistantRoute(chunk); route == "audio" {
			return "audio"
		}
	}
	return "other"
}

func peerSourceLabelClass(chunk *genx.MessageChunk) string {
	if chunk == nil || chunk.Ctrl == nil {
		return "empty"
	}
	label := strings.ToLower(strings.TrimSpace(chunk.Ctrl.Label))
	switch label {
	case "":
		return "empty"
	case "assistant":
		return "assistant"
	case "transcript":
		return "transcript"
	case genx.HistoryUserAudioLabel, peerStreamEventHistoryUpdatedLabel:
		return "history"
	default:
		return "other"
	}
}

func peerLabelClass(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "":
		return "empty"
	case "assistant":
		return "assistant"
	case "transcript":
		return "transcript"
	case genx.HistoryUserAudioLabel, peerStreamEventHistoryUpdatedLabel:
		return "history"
	default:
		return "other"
	}
}

func peerEventTypeClass(event *eventpb.PeerEvent) string {
	if event == nil {
		return ""
	}
	switch event.Type {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_BOS:
		return "bos"
	case eventpb.PeerEventType_PEER_EVENT_TYPE_EOS:
		return "eos"
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA:
		return "text_delta"
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
		return "text_done"
	default:
		return ""
	}
}

func peerEventKindClass(event *eventpb.PeerEvent) string {
	if event == nil {
		return ""
	}
	switch event.StreamKindValue() {
	case eventpb.StreamKind_STREAM_KIND_TEXT:
		return "text"
	case eventpb.StreamKind_STREAM_KIND_AUDIO:
		return "audio"
	case eventpb.StreamKind_STREAM_KIND_VIDEO:
		return "video"
	case eventpb.StreamKind_STREAM_KIND_MIXED:
		return "mixed"
	default:
		return "unspecified"
	}
}

func peerEventOutputModality(chunk *genx.MessageChunk, event *eventpb.PeerEvent) string {
	if event == nil {
		return "other"
	}
	label := peerLabelClass(event.Label())
	if label == "transcript" {
		return "transcript_text"
	}
	if event.Type == eventpb.PeerEventType_PEER_EVENT_TYPE_EOS && event.StreamError() != nil {
		result, _ := peerStreamEventOutcome(event)
		if result == "interrupted" {
			return "interrupt"
		}
	}
	switch event.Type {
	case eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DELTA,
		eventpb.PeerEventType_PEER_EVENT_TYPE_TEXT_DONE:
		return "assistant_text"
	case eventpb.PeerEventType_PEER_EVENT_TYPE_BOS:
		if event.StreamKindValue() == eventpb.StreamKind_STREAM_KIND_AUDIO {
			return "assistant_audio"
		}
		return ""
	case eventpb.PeerEventType_PEER_EVENT_TYPE_EOS:
		if label == "assistant" || event.StreamKindValue() == eventpb.StreamKind_STREAM_KIND_TEXT ||
			event.StreamKindValue() == eventpb.StreamKind_STREAM_KIND_AUDIO || peerChunkHasAssistantFallback(chunk) {
			return "assistant_eos"
		}
		return "other"
	}
	return "other"
}

func peerChunkHasAssistantFallback(chunk *genx.MessageChunk) bool {
	if chunk == nil || chunk.Ctrl == nil || strings.TrimSpace(chunk.Ctrl.Label) != "" {
		return false
	}
	switch chunk.Part.(type) {
	case genx.Text, *genx.Blob:
		return true
	default:
		return false
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
