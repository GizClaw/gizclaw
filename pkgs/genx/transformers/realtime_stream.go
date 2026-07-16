package transformers

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type realtimeAssistantLifecycle struct {
	epoch  atomic.Uint64
	accept atomic.Bool

	mu       sync.Mutex
	active   bool
	streamID string
	activeAt uint64
}

func newRealtimeAssistantLifecycle() *realtimeAssistantLifecycle {
	s := &realtimeAssistantLifecycle{}
	s.epoch.Store(1)
	s.accept.Store(true)
	return s
}

func (s *realtimeAssistantLifecycle) currentEpoch() uint64 { return s.epoch.Load() }
func (s *realtimeAssistantLifecycle) acceptsOutput() bool  { return s.accept.Load() }
func (s *realtimeAssistantLifecycle) setAccept(value bool) { s.accept.Store(value) }
func (s *realtimeAssistantLifecycle) nextEpoch() uint64    { return s.epoch.Add(1) }

func (s *realtimeAssistantLifecycle) markPending(streamID string, epoch uint64) {
	if strings.TrimSpace(streamID) == "" {
		return
	}
	s.mu.Lock()
	s.active = true
	s.streamID = streamID
	s.activeAt = epoch
	s.mu.Unlock()
}

func (s *realtimeAssistantLifecycle) markStarted(streamID string) uint64 {
	epoch := s.currentEpoch()
	s.markPending(streamID, epoch)
	return epoch
}

func (s *realtimeAssistantLifecycle) markDone(epoch uint64) {
	s.mu.Lock()
	if s.activeAt == epoch {
		s.active = false
	}
	s.mu.Unlock()
}

func (s *realtimeAssistantLifecycle) markDoneStream(streamID string) {
	s.mu.Lock()
	if s.streamID == streamID {
		s.active = false
	}
	s.mu.Unlock()
}

func (s *realtimeAssistantLifecycle) interrupt(fallback string, force bool) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active && !force {
		return "", false
	}
	streamID := strings.TrimSpace(s.streamID)
	if streamID == "" {
		streamID = strings.TrimSpace(fallback)
	}
	if streamID == "" {
		streamID = "audio"
	}
	s.active = false
	s.accept.Store(false)
	epoch := s.epoch.Add(1)
	s.activeAt = epoch
	return streamID, true
}

func (s *realtimeAssistantLifecycle) canPush(epoch uint64) bool {
	return s.acceptsOutput() && s.currentEpoch() == epoch
}

type realtimeChunkOutput interface {
	Push(*genx.MessageChunk) error
}

var errRealtimePTTOutputLimit = errors.New("realtime push-to-talk output audio limit exceeded")

type realtimePTTOutputGate struct {
	mu       sync.Mutex
	output   realtimeChunkOutput
	streamID string
	label    string
	limit    time.Duration

	committed        bool
	terminal         bool
	retained         []*genx.MessageChunk
	retainedDuration time.Duration
	limitErr         error
}

func newRealtimePTTOutputGate(output realtimeChunkOutput, streamID, label string, limit time.Duration) *realtimePTTOutputGate {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	return &realtimePTTOutputGate{
		output:   output,
		streamID: streamID,
		label:    strings.TrimSpace(label),
		limit:    limit,
	}
}

func (g *realtimePTTOutputGate) Push(chunk *genx.MessageChunk) error {
	if g == nil || chunk == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.terminal {
		return nil
	}
	if g.committed {
		return g.output.Push(chunk)
	}

	duration := realtimeAssistantOpusDuration(chunk, g.label)
	if g.limit > 0 && duration > 0 && duration > g.limit-g.retainedDuration {
		g.retained = nil
		g.retainedDuration = 0
		g.terminal = true
		g.limitErr = fmt.Errorf("%w for StreamID %q (limit %s)", errRealtimePTTOutputLimit, g.streamID, g.limit)
		if err := g.output.Push(realtimePTTOutputLimitChunk(g.streamID, g.label, g.limitErr)); err != nil {
			return err
		}
		return g.limitErr
	}

	g.retainedDuration += duration
	g.retained = append(g.retained, chunk.Clone())
	return nil
}

func (g *realtimePTTOutputGate) Commit() error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.limitErr != nil {
		return g.limitErr
	}
	if g.terminal || g.committed {
		return nil
	}
	for _, chunk := range g.retained {
		if err := g.output.Push(chunk); err != nil {
			g.terminal = true
			g.retained = nil
			g.retainedDuration = 0
			return err
		}
	}
	g.retained = nil
	g.retainedDuration = 0
	g.committed = true
	return nil
}

func (g *realtimePTTOutputGate) Discard() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.committed {
		return
	}
	g.terminal = true
	g.retained = nil
	g.retainedDuration = 0
}

func realtimeAssistantOpusDuration(chunk *genx.MessageChunk, label string) time.Duration {
	if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.Label != label {
		return 0
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil || len(blob.Data) == 0 || baseAudioMIME(blob.MIMEType) != "audio/opus" {
		return 0
	}
	return time.Duration(historyOpusPacketDurationMS(blob.Data)) * time.Millisecond
}

func realtimePTTOutputLimitChunk(streamID, label string, err error) *genx.MessageChunk {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: label,
		Ctrl: &genx.StreamCtrl{
			StreamID:    streamID,
			Label:       label,
			EndOfStream: true,
			Error:       errText,
		},
	}
}

type doubaoRealtimePTTTurn struct {
	mu sync.Mutex

	active       bool
	inputEnded   bool
	asrEnded     bool
	committed    bool
	streamID     string
	hypothesis   string
	output       realtimeChunkOutput
	assistantOut *realtimePTTOutputGate
}

type doubaoRealtimePTTResponse struct {
	streamID string
	epoch    uint64
	identity doubaoRealtimePTTResponseIdentity
	output   *realtimePTTOutputGate

	ttsStarted  bool
	ttsFinished bool
	chatEnded   bool
}

type doubaoRealtimePTTResponseIdentity struct {
	replyID    string
	questionID string
}

type doubaoRealtimePTTResponses struct {
	items []*doubaoRealtimePTTResponse
}

func (t *doubaoRealtimePTTTurn) begin(output realtimeChunkOutput, streamID, assistantLabel string, limit time.Duration) {
	if t == nil {
		return
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	t.mu.Lock()
	previous := t.assistantOut
	t.active = true
	t.inputEnded = false
	t.asrEnded = false
	t.committed = false
	t.streamID = streamID
	t.hypothesis = ""
	t.output = output
	t.assistantOut = newRealtimePTTOutputGate(output, streamID, assistantLabel, limit)
	t.mu.Unlock()
	previous.Discard()
}

func (t *doubaoRealtimePTTTurn) updateHypothesis(text string) {
	if t == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	t.mu.Lock()
	if t.active && !t.committed {
		t.hypothesis = text
	}
	t.mu.Unlock()
}

func (t *doubaoRealtimePTTTurn) markInputEnded() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	if t.active {
		t.inputEnded = true
	}
	t.mu.Unlock()
	return t.commitIfReady()
}

func (t *doubaoRealtimePTTTurn) markASREnded() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	if t.active {
		t.asrEnded = true
	}
	t.mu.Unlock()
	return t.commitIfReady()
}

func (t *doubaoRealtimePTTTurn) commitIfReady() error {
	t.mu.Lock()
	if !t.active || !t.inputEnded || !t.asrEnded || t.committed {
		t.mu.Unlock()
		return nil
	}
	t.committed = true
	streamID := t.streamID
	text := t.hypothesis
	output := t.output
	assistantOut := t.assistantOut
	t.mu.Unlock()

	if strings.TrimSpace(text) != "" {
		if err := output.Push(&genx.MessageChunk{
			Role: genx.RoleUser,
			Part: genx.Text(text),
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeTranscriptLabel},
		}); err != nil {
			return err
		}
	}
	if err := output.Push(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: doubaoRealtimeTranscriptLabel, EndOfStream: true},
	}); err != nil {
		return err
	}
	return assistantOut.Commit()
}

func (t *doubaoRealtimePTTTurn) pushAssistant(chunk *genx.MessageChunk) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	if !t.active {
		t.mu.Unlock()
		return nil
	}
	output := t.assistantOut
	t.mu.Unlock()
	return output.Push(chunk)
}

func (t *doubaoRealtimePTTTurn) bindResponse(epoch uint64, identity doubaoRealtimePTTResponseIdentity) *doubaoRealtimePTTResponse {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active || t.assistantOut == nil {
		return nil
	}
	return &doubaoRealtimePTTResponse{
		streamID: t.streamID,
		epoch:    epoch,
		identity: identity.normalized(),
		output:   t.assistantOut,
	}
}

func (t *doubaoRealtimePTTTurn) discardResponse(response *doubaoRealtimePTTResponse) (streamID string, active, committed bool) {
	if t == nil || response == nil {
		return "", false, false
	}
	t.mu.Lock()
	if !t.active || t.assistantOut != response.output {
		t.mu.Unlock()
		return "", false, false
	}
	streamID = t.streamID
	committed = t.committed
	output := t.assistantOut
	t.active = false
	t.committed = false
	t.hypothesis = ""
	t.assistantOut = nil
	t.mu.Unlock()
	output.Discard()
	return streamID, true, committed
}

func (r *doubaoRealtimePTTResponse) push(chunk *genx.MessageChunk) error {
	if r == nil || r.output == nil {
		return nil
	}
	return r.output.Push(chunk)
}

func (r *doubaoRealtimePTTResponse) done() bool {
	return r != nil && r.chatEnded && (!r.ttsStarted || r.ttsFinished)
}

func (i doubaoRealtimePTTResponseIdentity) normalized() doubaoRealtimePTTResponseIdentity {
	i.replyID = strings.TrimSpace(i.replyID)
	i.questionID = strings.TrimSpace(i.questionID)
	return i
}

func (i doubaoRealtimePTTResponseIdentity) empty() bool {
	i = i.normalized()
	return i.replyID == "" && i.questionID == ""
}

func (i doubaoRealtimePTTResponseIdentity) matches(other doubaoRealtimePTTResponseIdentity) bool {
	i = i.normalized()
	other = other.normalized()
	if i.conflicts(other) {
		return false
	}
	return (i.replyID != "" && i.replyID == other.replyID) ||
		(i.questionID != "" && i.questionID == other.questionID)
}

func (i doubaoRealtimePTTResponseIdentity) conflicts(other doubaoRealtimePTTResponseIdentity) bool {
	i = i.normalized()
	other = other.normalized()
	return (i.replyID != "" && other.replyID != "" && i.replyID != other.replyID) ||
		(i.questionID != "" && other.questionID != "" && i.questionID != other.questionID)
}

func (i *doubaoRealtimePTTResponseIdentity) merge(other doubaoRealtimePTTResponseIdentity) {
	if i == nil {
		return
	}
	other = other.normalized()
	if strings.TrimSpace(i.replyID) == "" {
		i.replyID = other.replyID
	}
	if strings.TrimSpace(i.questionID) == "" {
		i.questionID = other.questionID
	}
}

func (q *doubaoRealtimePTTResponses) add(response *doubaoRealtimePTTResponse) {
	if q == nil || response == nil {
		return
	}
	q.items = append(q.items, response)
}

func (q *doubaoRealtimePTTResponses) match(identity doubaoRealtimePTTResponseIdentity) *doubaoRealtimePTTResponse {
	if q == nil || len(q.items) == 0 {
		return nil
	}
	identity = identity.normalized()
	if identity.empty() {
		return q.items[0]
	}
	for _, response := range q.items {
		if response.identity.matches(identity) {
			response.identity.merge(identity)
			return response
		}
	}
	for _, response := range q.items {
		if response.identity.empty() {
			response.identity = identity
			return response
		}
	}
	if len(q.items) == 1 && !q.items[0].identity.conflicts(identity) {
		q.items[0].identity.merge(identity)
		return q.items[0]
	}
	return nil
}

func (q *doubaoRealtimePTTResponses) finish(response *doubaoRealtimePTTResponse) {
	if q == nil || response == nil || !response.done() {
		return
	}
	for i, candidate := range q.items {
		if candidate != response {
			continue
		}
		copy(q.items[i:], q.items[i+1:])
		q.items[len(q.items)-1] = nil
		q.items = q.items[:len(q.items)-1]
		return
	}
}

func (t *doubaoRealtimePTTTurn) discard() (streamID string, active, committed bool) {
	if t == nil {
		return "", false, false
	}
	t.mu.Lock()
	if !t.active {
		t.mu.Unlock()
		return "", false, false
	}
	streamID = t.streamID
	committed = t.committed
	output := t.assistantOut
	t.active = false
	t.committed = false
	t.hypothesis = ""
	t.assistantOut = nil
	t.mu.Unlock()
	output.Discard()
	return streamID, true, committed
}

func (t *doubaoRealtimePTTTurn) stream() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.streamID
}

type doubaoPushToTalkPhase uint8

const (
	doubaoPushToTalkIdle doubaoPushToTalkPhase = iota
	doubaoPushToTalkCapturing
	doubaoPushToTalkWaitingResponse
	doubaoPushToTalkResponding
	doubaoPushToTalkDiscarding
)

// doubaoPushToTalkState owns the user-turn lifecycle independently from the
// provider session. In particular, WaitingResponse is interruptible even when
// the provider has not emitted its first assistant event yet.
type doubaoPushToTalkState struct {
	mu         sync.Mutex
	phase      doubaoPushToTalkPhase
	streamID   string
	ttsStarted bool
}

func (s *doubaoPushToTalkState) begin(streamID string) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase == doubaoPushToTalkDiscarding {
		return false, "", fmt.Errorf("doubao realtime push-to-talk received BOS before failed turn EOS")
	}
	if s.phase == doubaoPushToTalkCapturing {
		return false, "", fmt.Errorf("doubao realtime push-to-talk received BOS while already capturing")
	}
	bargeIn := s.phase == doubaoPushToTalkWaitingResponse || s.phase == doubaoPushToTalkResponding
	interruptedStreamID := ""
	if bargeIn {
		interruptedStreamID = s.streamID
	}
	s.phase = doubaoPushToTalkCapturing
	s.streamID = strings.TrimSpace(streamID)
	s.ttsStarted = false
	return bargeIn, interruptedStreamID, nil
}

func (s *doubaoPushToTalkState) abort() (string, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase == doubaoPushToTalkIdle {
		return "", false, false
	}
	streamID := s.streamID
	wasCapturing := s.phase == doubaoPushToTalkCapturing
	if wasCapturing {
		s.phase = doubaoPushToTalkDiscarding
	} else {
		s.phase = doubaoPushToTalkIdle
		s.streamID = ""
	}
	s.ttsStarted = false
	return streamID, wasCapturing, true
}

func (s *doubaoPushToTalkState) discard(chunk *genx.MessageChunk) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != doubaoPushToTalkDiscarding {
		return false
	}
	if realtimeAudioInputEOS(chunk) {
		s.phase = doubaoPushToTalkIdle
		s.streamID = ""
		s.ttsStarted = false
	}
	return true
}

func (s *doubaoPushToTalkState) requireCapturing(kind string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != doubaoPushToTalkCapturing {
		return fmt.Errorf("doubao realtime push-to-talk received %s outside an active BOS/EOS turn", kind)
	}
	return nil
}

func (s *doubaoPushToTalkState) end() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != doubaoPushToTalkCapturing {
		return fmt.Errorf("doubao realtime push-to-talk received EOS before active BOS")
	}
	s.phase = doubaoPushToTalkWaitingResponse
	return nil
}

func (s *doubaoPushToTalkState) responseStarted(streamID string, tts bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamID != strings.TrimSpace(streamID) {
		return
	}
	if s.phase == doubaoPushToTalkWaitingResponse || s.phase == doubaoPushToTalkResponding {
		s.phase = doubaoPushToTalkResponding
		s.ttsStarted = s.ttsStarted || tts
	}
}

func (s *doubaoPushToTalkState) chatEnded(streamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamID != strings.TrimSpace(streamID) {
		return
	}
	if s.phase == doubaoPushToTalkResponding && !s.ttsStarted {
		s.phase = doubaoPushToTalkIdle
	}
}

func (s *doubaoPushToTalkState) ttsFinished(streamID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.streamID != strings.TrimSpace(streamID) {
		return
	}
	if s.phase == doubaoPushToTalkResponding {
		s.phase = doubaoPushToTalkIdle
		s.ttsStarted = false
	}
}

func (s *doubaoPushToTalkState) current() doubaoPushToTalkPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase
}

func realtimeChunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	if chunk != nil && chunk.Ctrl != nil {
		streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
		if streamID != "" && streamID != "audio" {
			return streamID
		}
	}
	return fallback
}

func chunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	return realtimeChunkInputStreamID(chunk, fallback)
}

type doubaoRealtimeStreamIDs struct {
	mu sync.Mutex

	mode       DoubaoRealtimeMode
	baseInput  string
	inputID    string
	responseID string
	segment    int
}

func newDoubaoRealtimeStreamIDs(mode DoubaoRealtimeMode) *doubaoRealtimeStreamIDs {
	return &doubaoRealtimeStreamIDs{mode: mode}
}

func (s *doubaoRealtimeStreamIDs) beginInput(id string) {
	if s == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		id = "audio"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.baseInput = id
	s.segment = 1
	s.inputID = s.inputForSegmentLocked()
}

func (s *doubaoRealtimeStreamIDs) input() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.inputID) == "" {
		s.inputID = s.inputForSegmentLocked()
	}
	return s.inputID
}

func (s *doubaoRealtimeStreamIDs) response() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.responseID
}

func (s *doubaoRealtimeStreamIDs) serviceInput(chunk *genx.MessageChunk) string {
	if s == nil {
		return chunkInputStreamID(chunk, "")
	}
	s.mu.Lock()
	s.ensureBaseFromChunkLocked(chunk)
	base := s.baseInput
	s.mu.Unlock()
	return chunkInputStreamID(chunk, base)
}

func (s *doubaoRealtimeStreamIDs) historyInput(chunk *genx.MessageChunk) string {
	if s == nil {
		return chunkInputStreamID(chunk, "")
	}
	if s.mode != DoubaoRealtimeModeRealtime {
		return chunkInputStreamID(chunk, s.input())
	}
	s.mu.Lock()
	s.ensureBaseFromChunkLocked(chunk)
	s.mu.Unlock()
	current := s.input()
	if strings.TrimSpace(current) != "" {
		return current
	}
	return chunkInputStreamID(chunk, "")
}

func (s *doubaoRealtimeStreamIDs) endInputSegment() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.inputID) == "" {
		s.inputID = s.inputForSegmentLocked()
	}
	ended := s.inputID
	s.responseID = ended
	if s.mode == DoubaoRealtimeModeRealtime {
		s.segment++
		s.inputID = s.inputForSegmentLocked()
	}
	return ended
}

func (s *doubaoRealtimeStreamIDs) ensureBaseFromChunkLocked(chunk *genx.MessageChunk) {
	if s == nil || strings.TrimSpace(s.baseInput) != "" {
		return
	}
	id := chunkInputStreamID(chunk, "")
	if strings.TrimSpace(id) == "" {
		return
	}
	s.baseInput = id
	if s.segment <= 0 {
		s.segment = 1
	}
	s.inputID = s.inputForSegmentLocked()
}

func (s *doubaoRealtimeStreamIDs) inputForSegmentLocked() string {
	base := strings.TrimSpace(s.baseInput)
	if base == "" {
		base = "audio"
	}
	if s.mode != DoubaoRealtimeModeRealtime {
		return base
	}
	segment := s.segment
	if segment <= 0 {
		segment = 1
	}
	return fmt.Sprintf("%s:rt:%d", base, segment)
}
