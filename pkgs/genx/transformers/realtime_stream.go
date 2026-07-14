package transformers

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

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

type doubaoPushToTalkPhase uint8

const (
	doubaoPushToTalkIdle doubaoPushToTalkPhase = iota
	doubaoPushToTalkCapturing
	doubaoPushToTalkWaitingResponse
	doubaoPushToTalkResponding
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

func (s *doubaoPushToTalkState) responseStarted(tts bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase == doubaoPushToTalkWaitingResponse || s.phase == doubaoPushToTalkResponding {
		s.phase = doubaoPushToTalkResponding
		s.ttsStarted = s.ttsStarted || tts
	}
}

func (s *doubaoPushToTalkState) chatEnded() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase == doubaoPushToTalkResponding && !s.ttsStarted {
		s.phase = doubaoPushToTalkIdle
	}
}

func (s *doubaoPushToTalkState) ttsFinished() {
	s.mu.Lock()
	defer s.mu.Unlock()
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
