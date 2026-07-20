package doubaorealtime

import (
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// SharedAssistantLifecycle is the common response interruption state used by
// the two Doubao realtime protocol adapters.
type SharedAssistantLifecycle struct {
	inner *realtimeAssistantLifecycle
}

func NewSharedAssistantLifecycle() *SharedAssistantLifecycle {
	return &SharedAssistantLifecycle{inner: newRealtimeAssistantLifecycle()}
}

func (s *SharedAssistantLifecycle) CurrentEpoch() uint64         { return s.inner.currentEpoch() }
func (s *SharedAssistantLifecycle) AcceptsOutput() bool          { return s.inner.acceptsOutput() }
func (s *SharedAssistantLifecycle) SetAccept(value bool)         { s.inner.setAccept(value) }
func (s *SharedAssistantLifecycle) NextEpoch() uint64            { return s.inner.nextEpoch() }
func (s *SharedAssistantLifecycle) MarkStarted(id string) uint64 { return s.inner.markStarted(id) }
func (s *SharedAssistantLifecycle) MarkRouteDone(id string, text bool) {
	s.inner.markRouteDoneStream(id, text)
}
func (s *SharedAssistantLifecycle) Interrupt(id string, force bool) (string, bool) {
	return s.inner.interrupt(id, force)
}
func (s *SharedAssistantLifecycle) CanPush(epoch uint64) bool { return s.inner.canPush(epoch) }

func ObserveSharedAssistantOutput(state *SharedAssistantLifecycle, label string, chunk *genx.MessageChunk) {
	if state != nil {
		observeRealtimeAssistantOutput(state.inner, label, chunk)
	}
}

// SharedStreamIDs maintains the common per-segment input and response IDs.
type SharedStreamIDs struct {
	inner *doubaoRealtimeStreamIDs
}

func NewSharedStreamIDs() *SharedStreamIDs {
	return &SharedStreamIDs{inner: newDoubaoRealtimeStreamIDs(ModeRealtime)}
}

func (s *SharedStreamIDs) BeginInput(id string) { s.inner.beginInput(id) }
func (s *SharedStreamIDs) Input() string        { return s.inner.input() }
func (s *SharedStreamIDs) Response() string     { return s.inner.response() }
func (s *SharedStreamIDs) ServiceInput(chunk *genx.MessageChunk) string {
	return s.inner.serviceInput(chunk)
}
func (s *SharedStreamIDs) EndInputSegment() string { return s.inner.endInputSegment() }

func SharedChunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	return realtimeChunkInputStreamID(chunk, fallback)
}

// SharedAudioInput adapts one MIME-stable input route.
type SharedAudioInput struct {
	inner *doubaoRealtimeAudioInput
}

func NewSharedAudioInput(format string, sampleRate, channels int, transcode bool) *SharedAudioInput {
	return &SharedAudioInput{inner: newDoubaoRealtimeAudioInput(format, sampleRate, channels, transcode)}
}

func (a *SharedAudioInput) Prepare(blob *genx.Blob) ([]byte, error) {
	return a.inner.prepare(blob)
}
func (a *SharedAudioInput) PrepareFrames(blob *genx.Blob) ([][]byte, error) {
	return a.inner.prepareFrames(blob)
}
func (a *SharedAudioInput) Format() string { return a.inner.format }
func (a *SharedAudioInput) Close()         { a.inner.close() }

// SharedAudioInputs owns independent codec state for every input StreamID.
type SharedAudioInputs struct {
	inner  *doubaoRealtimeAudioInputs
	mu     sync.Mutex
	inputs map[string]*SharedAudioInput
}

func NewSharedAudioInputs(format string, sampleRate, channels int, transcode bool) *SharedAudioInputs {
	return &SharedAudioInputs{
		inner:  newDoubaoRealtimeAudioInputs(format, sampleRate, channels, transcode),
		inputs: make(map[string]*SharedAudioInput),
	}
}

func (a *SharedAudioInputs) Stream(streamID string) *SharedAudioInput {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := realtimeStreamKey(streamID)
	if input := a.inputs[key]; input != nil {
		return input
	}
	input := &SharedAudioInput{inner: a.inner.stream(key)}
	a.inputs[key] = input
	return input
}
func (a *SharedAudioInputs) StreamForBlob(streamID string, blob *genx.Blob) (*SharedAudioInput, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := realtimeStreamKey(streamID)
	inner, err := a.inner.streamForBlob(key, blob)
	if err != nil {
		return nil, err
	}
	if input := a.inputs[key]; input != nil {
		return input, nil
	}
	input := &SharedAudioInput{inner: inner}
	a.inputs[key] = input
	return input, nil
}
func (a *SharedAudioInputs) CloseStream(streamID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := realtimeStreamKey(streamID)
	delete(a.inputs, key)
	a.inner.closeStream(key)
}
func (a *SharedAudioInputs) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	clear(a.inputs)
	a.inner.close()
}

func SharedAudioInputEOS(chunk *genx.MessageChunk) bool { return realtimeAudioInputEOS(chunk) }
func SharedAudioFormat(format string) string            { return realtimeAudioFormat(format) }
func SharedAudioSampleRate(sampleRate int) int          { return realtimeAudioSampleRate(sampleRate) }
func SharedPCM16LE(samples []int16) []byte              { return realtimePCM16LE(samples) }
