package doubaorealtimeduplex

import (
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaorealtime"
)

type realtimeAssistantLifecycle struct {
	*doubaorealtime.SharedAssistantLifecycle
}

func newRealtimeAssistantLifecycle() *realtimeAssistantLifecycle {
	return &realtimeAssistantLifecycle{SharedAssistantLifecycle: doubaorealtime.NewSharedAssistantLifecycle()}
}

func (s *realtimeAssistantLifecycle) currentEpoch() uint64         { return s.CurrentEpoch() }
func (s *realtimeAssistantLifecycle) acceptsOutput() bool          { return s.AcceptsOutput() }
func (s *realtimeAssistantLifecycle) setAccept(value bool)         { s.SetAccept(value) }
func (s *realtimeAssistantLifecycle) nextEpoch() uint64            { return s.NextEpoch() }
func (s *realtimeAssistantLifecycle) markStarted(id string) uint64 { return s.MarkStarted(id) }
func (s *realtimeAssistantLifecycle) markRouteDoneStream(id string, text bool) {
	s.MarkRouteDone(id, text)
}
func (s *realtimeAssistantLifecycle) interrupt(id string, force bool) (string, bool) {
	return s.Interrupt(id, force)
}
func (s *realtimeAssistantLifecycle) canPush(epoch uint64) bool { return s.CanPush(epoch) }

func observeRealtimeAssistantOutput(state *realtimeAssistantLifecycle, label string, chunk *genx.MessageChunk) {
	if state != nil {
		doubaorealtime.ObserveSharedAssistantOutput(state.SharedAssistantLifecycle, label, chunk)
	}
}

type doubaoRealtimeDuplexAudioInput struct {
	shared *doubaorealtime.SharedAudioInput
	format string
}

func newDoubaoRealtimeDuplexAudioInput(format string, sampleRate, channels int, transcode bool) *doubaoRealtimeDuplexAudioInput {
	shared := doubaorealtime.NewSharedAudioInput(format, sampleRate, channels, transcode)
	return &doubaoRealtimeDuplexAudioInput{shared: shared, format: shared.Format()}
}

func (a *doubaoRealtimeDuplexAudioInput) prepare(blob *genx.Blob) ([]byte, error) {
	return a.shared.Prepare(blob)
}
func (a *doubaoRealtimeDuplexAudioInput) prepareFrames(blob *genx.Blob) ([][]byte, error) {
	return a.shared.PrepareFrames(blob)
}
func (a *doubaoRealtimeDuplexAudioInput) close() { a.shared.Close() }

type doubaoRealtimeDuplexAudioInputs struct {
	shared *doubaorealtime.SharedAudioInputs
	mu     sync.Mutex
	inputs map[string]*doubaoRealtimeDuplexAudioInput
}

func newDoubaoRealtimeDuplexAudioInputs(format string, sampleRate, channels int, transcode bool) *doubaoRealtimeDuplexAudioInputs {
	return &doubaoRealtimeDuplexAudioInputs{
		shared: doubaorealtime.NewSharedAudioInputs(format, sampleRate, channels, transcode),
		inputs: make(map[string]*doubaoRealtimeDuplexAudioInput),
	}
}

func (a *doubaoRealtimeDuplexAudioInputs) stream(streamID string) *doubaoRealtimeDuplexAudioInput {
	a.mu.Lock()
	defer a.mu.Unlock()
	if input := a.inputs[streamID]; input != nil {
		return input
	}
	shared := a.shared.Stream(streamID)
	input := &doubaoRealtimeDuplexAudioInput{shared: shared, format: shared.Format()}
	a.inputs[streamID] = input
	return input
}
func (a *doubaoRealtimeDuplexAudioInputs) streamForBlob(streamID string, blob *genx.Blob) (*doubaoRealtimeDuplexAudioInput, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	shared, err := a.shared.StreamForBlob(streamID, blob)
	if err != nil {
		return nil, err
	}
	if input := a.inputs[streamID]; input != nil && input.shared == shared {
		return input, nil
	}
	input := &doubaoRealtimeDuplexAudioInput{shared: shared, format: shared.Format()}
	a.inputs[streamID] = input
	return input, nil
}
func (a *doubaoRealtimeDuplexAudioInputs) closeStream(streamID string) {
	a.mu.Lock()
	delete(a.inputs, streamID)
	a.mu.Unlock()
	a.shared.CloseStream(streamID)
}
func (a *doubaoRealtimeDuplexAudioInputs) close() {
	a.mu.Lock()
	clear(a.inputs)
	a.mu.Unlock()
	a.shared.Close()
}

type doubaoRealtimeDuplexStreamIDs struct {
	shared *doubaorealtime.SharedStreamIDs
}

func newDoubaoRealtimeDuplexStreamIDs() *doubaoRealtimeDuplexStreamIDs {
	return &doubaoRealtimeDuplexStreamIDs{shared: doubaorealtime.NewSharedStreamIDs()}
}

func (s *doubaoRealtimeDuplexStreamIDs) beginInput(id string) { s.shared.BeginInput(id) }
func (s *doubaoRealtimeDuplexStreamIDs) input() string        { return s.shared.Input() }
func (s *doubaoRealtimeDuplexStreamIDs) response() string     { return s.shared.Response() }
func (s *doubaoRealtimeDuplexStreamIDs) serviceInput(chunk *genx.MessageChunk) string {
	return s.shared.ServiceInput(chunk)
}
func (s *doubaoRealtimeDuplexStreamIDs) endInputSegment() string { return s.shared.EndInputSegment() }

func realtimeAudioInputEOS(chunk *genx.MessageChunk) bool {
	return doubaorealtime.SharedAudioInputEOS(chunk)
}
func doubaoRealtimeDuplexChunkInputStreamID(chunk *genx.MessageChunk, fallback string) string {
	return doubaorealtime.SharedChunkInputStreamID(chunk, fallback)
}
func doubaoRealtimeDuplexAudioFormat(format string) string {
	return doubaorealtime.SharedAudioFormat(format)
}
func doubaoRealtimeDuplexAudioSampleRate(sampleRate int) int {
	return doubaorealtime.SharedAudioSampleRate(sampleRate)
}
func doubaoRealtimeDuplexPCM16LE(samples []int16) []byte {
	return doubaorealtime.SharedPCM16LE(samples)
}
