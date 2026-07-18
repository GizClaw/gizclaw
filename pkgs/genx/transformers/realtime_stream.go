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

	mu        sync.Mutex
	active    bool
	streamID  string
	activeAt  uint64
	textDone  bool
	audioDone bool
}

type realtimeAssistantInterruption struct {
	streamID    string
	interrupted bool
	textOpen    bool
	audioOpen   bool
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
	s.textDone = false
	s.audioDone = false
	s.mu.Unlock()
}

func (s *realtimeAssistantLifecycle) markStarted(streamID string) uint64 {
	epoch := s.currentEpoch()
	streamID = strings.TrimSpace(streamID)
	s.mu.Lock()
	if s.active && s.activeAt == epoch {
		if streamID != "" {
			s.streamID = streamID
		}
	} else if streamID != "" {
		s.active = true
		s.streamID = streamID
		s.activeAt = epoch
		s.textDone = false
		s.audioDone = false
	}
	s.mu.Unlock()
	return epoch
}

func (s *realtimeAssistantLifecycle) markDone(epoch uint64) {
	s.mu.Lock()
	if s.activeAt == epoch {
		s.active = false
		s.textDone = true
		s.audioDone = true
	}
	s.mu.Unlock()
}

func (s *realtimeAssistantLifecycle) markDoneStream(streamID string) {
	s.mu.Lock()
	if s.streamID == streamID {
		s.active = false
		s.textDone = true
		s.audioDone = true
	}
	s.mu.Unlock()
}

func (s *realtimeAssistantLifecycle) markTextDone(epoch uint64) {
	s.markRouteDone(epoch, true)
}

func (s *realtimeAssistantLifecycle) markAudioDone(epoch uint64) {
	s.markRouteDone(epoch, false)
}

func (s *realtimeAssistantLifecycle) markRouteDone(epoch uint64, text bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active || s.activeAt != epoch {
		return
	}
	if text {
		s.textDone = true
	} else {
		s.audioDone = true
	}
	s.active = !(s.textDone && s.audioDone)
}

func (s *realtimeAssistantLifecycle) markRouteDoneStream(streamID string, text bool) {
	streamID = strings.TrimSpace(streamID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active || streamID == "" || s.streamID != streamID {
		return
	}
	if text {
		s.textDone = true
	} else {
		s.audioDone = true
	}
	s.active = !(s.textDone && s.audioDone)
}

func observeRealtimeAssistantOutput(assistant *realtimeAssistantLifecycle, label string, chunk *genx.MessageChunk) {
	if assistant == nil || chunk == nil || chunk.Ctrl == nil || chunk.Role != genx.RoleModel ||
		chunk.Ctrl.Label != label || !chunk.IsEndOfStream() {
		return
	}
	switch chunk.Part.(type) {
	case genx.Text:
		assistant.markRouteDoneStream(chunk.Ctrl.StreamID, true)
	case *genx.Blob:
		assistant.markRouteDoneStream(chunk.Ctrl.StreamID, false)
	}
}

func (s *realtimeAssistantLifecycle) interruptRoutes(fallback string, force bool) realtimeAssistantInterruption {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active && !force {
		return realtimeAssistantInterruption{}
	}
	wasActive := s.active
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
	interruption := realtimeAssistantInterruption{
		streamID:    streamID,
		interrupted: true,
		textOpen:    !s.textDone,
		audioOpen:   !s.audioDone,
	}
	if !wasActive && force {
		interruption.textOpen = true
		interruption.audioOpen = true
	}
	s.textDone = true
	s.audioDone = true
	return interruption
}

func (s *realtimeAssistantLifecycle) interrupt(fallback string, force bool) (string, bool) {
	interruption := s.interruptRoutes(fallback, force)
	return interruption.streamID, interruption.interrupted
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
	maxBytes int64

	committed        bool
	terminal         bool
	retained         []*genx.MessageChunk
	retainedDuration time.Duration
	retainedBytes    int64
	limitErr         error
}

func newRealtimePTTOutputGate(output realtimeChunkOutput, streamID, label string, limit time.Duration, maxBytes int64) *realtimePTTOutputGate {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	return &realtimePTTOutputGate{
		output:   output,
		streamID: streamID,
		label:    strings.TrimSpace(label),
		limit:    limit,
		maxBytes: maxBytes,
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
	bytes := realtimeAssistantAudioBytes(chunk, g.label)
	durationExceeded := g.limit > 0 && duration > 0 && duration > g.limit-g.retainedDuration
	bytesExceeded := g.maxBytes > 0 && bytes > g.maxBytes-g.retainedBytes
	if durationExceeded || bytesExceeded {
		g.retained = nil
		g.retainedDuration = 0
		g.retainedBytes = 0
		g.terminal = true
		g.limitErr = fmt.Errorf("%w for StreamID %q (limit %s)", errRealtimePTTOutputLimit, g.streamID, g.limit)
		if err := g.output.Push(realtimePTTOutputLimitChunk(g.streamID, g.label, g.limitErr)); err != nil {
			return err
		}
		return g.limitErr
	}

	g.retainedDuration += duration
	g.retainedBytes += bytes
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
			g.retainedBytes = 0
			return err
		}
	}
	g.retained = nil
	g.retainedDuration = 0
	g.retainedBytes = 0
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
	g.retainedBytes = 0
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

func realtimeAssistantAudioBytes(chunk *genx.MessageChunk, label string) int64 {
	if chunk == nil || chunk.Role != genx.RoleModel || chunk.Ctrl == nil || chunk.Ctrl.Label != label {
		return 0
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || blob == nil {
		return 0
	}
	return int64(len(blob.Data))
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

	generation   uint64
	active       bool
	inputEnded   bool
	asrEnded     bool
	committed    bool
	streamID     string
	hypothesis   string
	output       realtimeChunkOutput
	assistantOut *realtimePTTOutputGate
	completion   *doubaoRealtimePTTCompletion
}

type doubaoRealtimePTTASRQueue struct {
	mu          sync.Mutex
	generations []uint64
}

type doubaoRealtimePTTResponse struct {
	streamID   string
	epoch      uint64
	identity   doubaoRealtimePTTResponseIdentity
	output     *realtimePTTOutputGate
	completion *doubaoRealtimePTTCompletion

	ttsStarted  bool
	ttsFinished bool
	chatEnded   bool
}

type doubaoRealtimePTTCompletion struct {
	done chan struct{}
	once sync.Once
}

func newDoubaoRealtimePTTCompletion() *doubaoRealtimePTTCompletion {
	return &doubaoRealtimePTTCompletion{done: make(chan struct{})}
}

func (c *doubaoRealtimePTTCompletion) complete() {
	if c != nil {
		c.once.Do(func() { close(c.done) })
	}
}

type doubaoRealtimePTTResponseIdentity struct {
	replyID    string
	questionID string
}

type doubaoRealtimePTTResponses struct {
	items []*doubaoRealtimePTTResponse
}

type doubaoRealtimeTextResponse struct {
	ttsStarted  bool
	ttsFinished bool
	chatEnded   bool
}

func (r *doubaoRealtimeTextResponse) done() bool {
	return r != nil && r.chatEnded && r.ttsFinished
}

type doubaoRealtimeTextResponses struct {
	mu    sync.Mutex
	items []*doubaoRealtimeTextResponse
	done  chan struct{}
}

func (q *doubaoRealtimeTextResponses) begin() *doubaoRealtimeTextResponse {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		q.done = make(chan struct{})
	}
	response := &doubaoRealtimeTextResponse{}
	q.items = append(q.items, response)
	return response
}

func (q *doubaoRealtimeTextResponses) cancel(response *doubaoRealtimeTextResponse) {
	if q == nil || response == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, candidate := range q.items {
		if candidate != response {
			continue
		}
		q.removeLocked(i)
		return
	}
}

func (q *doubaoRealtimeTextResponses) responseDone() (<-chan struct{}, bool) {
	if q == nil {
		return nil, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 || q.done == nil {
		return nil, false
	}
	return q.done, true
}

func (q *doubaoRealtimeTextResponses) markTTSStarted() {
	q.updateFirst(func(response *doubaoRealtimeTextResponse) {
		response.ttsStarted = true
	})
}

func (q *doubaoRealtimeTextResponses) markTTSFinished() {
	q.updateFirst(func(response *doubaoRealtimeTextResponse) {
		response.ttsStarted = true
		response.ttsFinished = true
	})
}

func (q *doubaoRealtimeTextResponses) markChatEnded() {
	q.updateFirst(func(response *doubaoRealtimeTextResponse) {
		response.chatEnded = true
	})
}

func (q *doubaoRealtimeTextResponses) updateFirst(update func(*doubaoRealtimeTextResponse)) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return
	}
	response := q.items[0]
	update(response)
	if response.done() {
		q.removeLocked(0)
	}
}

func (q *doubaoRealtimeTextResponses) removeLocked(index int) {
	copy(q.items[index:], q.items[index+1:])
	q.items[len(q.items)-1] = nil
	q.items = q.items[:len(q.items)-1]
	if len(q.items) == 0 && q.done != nil {
		close(q.done)
		q.done = nil
	}
}

func (t *doubaoRealtimePTTTurn) begin(output realtimeChunkOutput, streamID, assistantLabel string, limit time.Duration, maxBytes int64) {
	if t == nil {
		return
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		streamID = "audio"
	}
	t.mu.Lock()
	previous := t.assistantOut
	previousCompletion := t.completion
	t.generation++
	if t.generation == 0 {
		t.generation = 1
	}
	t.active = true
	t.inputEnded = false
	t.asrEnded = false
	t.committed = false
	t.streamID = streamID
	t.hypothesis = ""
	t.output = output
	t.assistantOut = newRealtimePTTOutputGate(output, streamID, assistantLabel, limit, maxBytes)
	t.completion = newDoubaoRealtimePTTCompletion()
	t.mu.Unlock()
	previous.Discard()
	previousCompletion.complete()
}

func (t *doubaoRealtimePTTTurn) updateHypothesis(text string) {
	t.updateHypothesisFor(t.currentGeneration(), text)
}

func (t *doubaoRealtimePTTTurn) updateHypothesisFor(generation uint64, text string) {
	if t == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	t.mu.Lock()
	if t.active && t.generation == generation && !t.committed {
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
	_, err := t.markASREndedFor(t.currentGeneration())
	return err
}

func (t *doubaoRealtimePTTTurn) markASREndedFor(generation uint64) (bool, error) {
	if t == nil {
		return false, nil
	}
	t.mu.Lock()
	if !t.active || t.generation != generation {
		t.mu.Unlock()
		return false, nil
	}
	t.asrEnded = true
	t.mu.Unlock()
	return true, t.commitIfReady()
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

func (t *doubaoRealtimePTTTurn) bindResponseFor(generation, epoch uint64, identity doubaoRealtimePTTResponseIdentity) *doubaoRealtimePTTResponse {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active || t.generation != generation || t.assistantOut == nil {
		return nil
	}
	return &doubaoRealtimePTTResponse{
		streamID:   t.streamID,
		epoch:      epoch,
		identity:   identity.normalized(),
		output:     t.assistantOut,
		completion: t.completion,
	}
}

func (t *doubaoRealtimePTTTurn) currentGeneration() uint64 {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.generation
}

func (q *doubaoRealtimePTTASRQueue) add(generation uint64) {
	if q == nil || generation == 0 {
		return
	}
	q.mu.Lock()
	q.generations = append(q.generations, generation)
	q.mu.Unlock()
}

func (q *doubaoRealtimePTTASRQueue) peek(fallback uint64) uint64 {
	if q == nil {
		return fallback
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.generations) == 0 {
		return fallback
	}
	return q.generations[0]
}

func (q *doubaoRealtimePTTASRQueue) take() (uint64, bool) {
	if q == nil {
		return 0, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.generations) == 0 {
		return 0, false
	}
	generation := q.generations[0]
	copy(q.generations, q.generations[1:])
	q.generations[len(q.generations)-1] = 0
	q.generations = q.generations[:len(q.generations)-1]
	return generation, true
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
	completion := t.completion
	t.active = false
	t.committed = false
	t.hypothesis = ""
	t.assistantOut = nil
	t.completion = nil
	t.mu.Unlock()
	output.Discard()
	completion.complete()
	return streamID, true, committed
}

func (t *doubaoRealtimePTTTurn) responseDone() (<-chan struct{}, bool) {
	if t == nil {
		return nil, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active || !t.inputEnded || t.completion == nil {
		return nil, false
	}
	return t.completion.done, true
}

func (r *doubaoRealtimePTTResponse) push(chunk *genx.MessageChunk) error {
	if r == nil || r.output == nil {
		return nil
	}
	return r.output.Push(chunk)
}

func (r *doubaoRealtimePTTResponse) done() bool {
	return r != nil && r.chatEnded && r.ttsFinished
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
		response.completion.complete()
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
	completion := t.completion
	t.active = false
	t.committed = false
	t.hypothesis = ""
	t.assistantOut = nil
	t.completion = nil
	t.mu.Unlock()
	output.Discard()
	completion.complete()
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
