package doubaorealtimeduplex

import (
	"errors"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestRealtimeAssistantLifecycleFacadeBranches(t *testing.T) {
	state := newRealtimeAssistantLifecycle()
	if state.currentEpoch() != 1 || !state.acceptsOutput() {
		t.Fatalf("initial lifecycle = epoch %d accept %t", state.currentEpoch(), state.acceptsOutput())
	}
	state.setAccept(false)
	if state.acceptsOutput() {
		t.Fatal("setAccept(false) did not reject output")
	}
	epoch := state.nextEpoch()
	if epoch == 0 || state.canPush(epoch) {
		t.Fatalf("next epoch = %d, canPush = %t", epoch, state.canPush(epoch))
	}
	epoch = state.markStarted("response")
	state.markRouteStarted(epoch, true)
	state.markRouteStarted(epoch, false)
	state.markRouteDoneStream("response", true)
	state.markRouteDoneStream("response", false)
	if streamID, interrupted := state.interrupt("response", false); interrupted || streamID != "" {
		t.Fatalf("completed route interruption = (%q, %t)", streamID, interrupted)
	}
	if interruption := state.interruptRoutes("response", true); interruption.StreamID != "response" {
		t.Fatalf("forced route interruption = %#v", interruption)
	}
	observeRealtimeAssistantOutput(nil, "assistant", nil)
}

func TestDoubaoRealtimeDuplexBufferDiscardBranches(t *testing.T) {
	var nilBuffer *bufferStream
	if nilBuffer.discard(nil) != 0 || nilBuffer.discardChunks(nil) != nil {
		t.Fatal("nil buffer discard returned data")
	}
	withoutOutput := &bufferStream{}
	if withoutOutput.discard(nil) != 0 || withoutOutput.discardChunks(nil) != nil {
		t.Fatal("buffer without output returned data")
	}

	output := newBufferStream(3)
	keep := &genx.MessageChunk{Part: genx.Text("keep")}
	drop := &genx.MessageChunk{Part: genx.Text("drop")}
	if err := output.Push(keep); err != nil {
		t.Fatalf("Push(keep) error = %v", err)
	}
	if err := output.Push(drop); err != nil {
		t.Fatalf("Push(drop) error = %v", err)
	}
	discarded := output.discardChunks(func(chunk *genx.MessageChunk) bool { return chunk == drop })
	if len(discarded) != 1 || discarded[0] != drop {
		t.Fatalf("discarded chunks = %#v", discarded)
	}
	if output.discard(func(chunk *genx.MessageChunk) bool { return chunk == keep }) != 1 {
		t.Fatal("discard() did not remove the remaining chunk")
	}
}

func TestPendingChunkStreamDelegationBranches(t *testing.T) {
	rest := &trackingCloseStream{}
	if got := withDoubaoRealtimeDuplexPendingChunk(rest, nil); got != rest {
		t.Fatalf("nil pending stream = %#v, want rest", got)
	}

	first := &genx.MessageChunk{Part: genx.Text("first")}
	stream := withDoubaoRealtimeDuplexPendingChunk(rest, first).(*doubaoRealtimeDuplexPendingChunkStream)
	chunk, err, done := stream.NextOrDone(make(chan struct{}))
	if chunk != first || err != nil || done {
		t.Fatalf("pending NextOrDone() = (%#v, %v, %t)", chunk, err, done)
	}
	if _, err := stream.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("delegated Next() error = %v, want EOF", err)
	}

	doneAware := &doneAwareRealtimeStream{chunk: &genx.MessageChunk{Part: genx.Text("delegated")}}
	stream = &doubaoRealtimeDuplexPendingChunkStream{rest: doneAware}
	chunk, err, done = stream.NextOrDone(make(chan struct{}))
	if chunk != doneAware.chunk || err != nil || done || doneAware.calls != 1 {
		t.Fatalf("done-aware delegation = (%#v, %v, %t), calls %d", chunk, err, done, doneAware.calls)
	}

	fallback := &sliceRealtimeStream{chunks: []*genx.MessageChunk{{Part: genx.Text("fallback")}}}
	stream = &doubaoRealtimeDuplexPendingChunkStream{rest: fallback}
	chunk, err, done = stream.NextOrDone(make(chan struct{}))
	if chunk == nil || err != nil || done {
		t.Fatalf("fallback delegation = (%#v, %v, %t)", chunk, err, done)
	}
}

func TestDoubaoRealtimeDuplexInputReaderBranches(t *testing.T) {
	pendingChunk := &genx.MessageChunk{Part: genx.Text("pending")}
	reader := &doubaoRealtimeDuplexInputReader{pending: &doubaoRealtimeDuplexInputResult{chunk: pendingChunk}}
	if chunk, err := reader.Next(); chunk != pendingChunk || err != nil || reader.pending != nil {
		t.Fatalf("pending Next() = (%#v, %v), pending %#v", chunk, err, reader.pending)
	}

	done := make(chan struct{})
	close(done)
	reader.pending = &doubaoRealtimeDuplexInputResult{chunk: pendingChunk}
	if chunk, err, stopped := reader.NextOrDone(done); chunk != nil || err != nil || !stopped || reader.pending == nil {
		t.Fatalf("pending done = (%#v, %v, %t), pending %#v", chunk, err, stopped, reader.pending)
	}
	if chunk, err, stopped := reader.NextOrDone(make(chan struct{})); chunk != pendingChunk || err != nil || stopped || reader.pending != nil {
		t.Fatalf("pending delivery = (%#v, %v, %t), pending %#v", chunk, err, stopped, reader.pending)
	}

	closedResults := make(chan doubaoRealtimeDuplexInputResult)
	close(closedResults)
	reader.results = closedResults
	if chunk, err := reader.Next(); chunk != nil || !errors.Is(err, io.EOF) {
		t.Fatalf("closed Next() = (%#v, %v)", chunk, err)
	}
	if chunk, err, stopped := reader.NextOrDone(make(chan struct{})); chunk != nil || !errors.Is(err, io.EOF) || stopped {
		t.Fatalf("closed NextOrDone() = (%#v, %v, %t)", chunk, err, stopped)
	}

	source := &trackingCloseStream{}
	reader = newDoubaoRealtimeDuplexInputReader(source)
	if err := reader.CloseWithError(nil); err != nil || source.closed != 1 {
		t.Fatalf("CloseWithError(nil) = %v, source closed %d", err, source.closed)
	}
	if err := reader.CloseWithError(errors.New("ignored second close")); err != nil || source.closeErr != nil {
		t.Fatalf("second CloseWithError() = %v, source error %v", err, source.closeErr)
	}

	wantErr := errors.New("input failed")
	source = &trackingCloseStream{}
	reader = newDoubaoRealtimeDuplexInputReader(source)
	if err := reader.CloseWithError(wantErr); err != nil || !errors.Is(source.closeErr, wantErr) {
		t.Fatalf("CloseWithError(error) = %v, source error %v", err, source.closeErr)
	}
}

func TestDoubaoRealtimeDuplexSmallHelperBranches(t *testing.T) {
	formats := map[string]string{
		"mp3": "audio/mpeg", "ogg_opus": "audio/ogg", "pcm": "audio/pcm",
		"pcm_s16le": "audio/pcm", "unknown": "audio/pcm", " PCM ": "audio/pcm",
	}
	for format, want := range formats {
		transformer := newTransformer(nil, withFormat(format))
		if got := transformer.mimeType(); got != want {
			t.Errorf("format %q MIME = %q, want %q", format, got, want)
		}
		wantOutput := want
		if format == "ogg_opus" {
			wantOutput = "audio/opus"
		}
		if got := transformer.outputMIMEType(); got != wantOutput {
			t.Errorf("format %q output MIME = %q, want %q", format, got, wantOutput)
		}
	}
	if firstNonEmptyString(" ", " value ", "later") != "value" || firstNonEmptyString("", " ") != "" {
		t.Fatal("firstNonEmptyString() did not trim/select values")
	}

	transformer := newTransformer(nil)
	transformer.pushInputEOSError(nil, "stream", errors.New("ignored"))
	output := newBufferStream(2)
	transformer.pushInputEOSError(output, "stream", nil)
	wantErr := errors.New("failed")
	transformer.pushInputEOSError(output, "stream", wantErr)
	for index := range 2 {
		chunk, err := output.Next()
		if err != nil || chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.StreamID != "stream" {
			t.Fatalf("error lifecycle chunk %d = (%#v, %v)", index, chunk, err)
		}
		if index == 0 && !chunk.IsBeginOfStream() {
			t.Fatalf("first chunk = %#v, want BOS", chunk)
		}
		if index == 1 && (!chunk.IsEndOfStream() || chunk.Ctrl.Error != wantErr.Error()) {
			t.Fatalf("last chunk = %#v, want error EOS", chunk)
		}
	}
}

type doneAwareRealtimeStream struct {
	chunk *genx.MessageChunk
	calls int
}

func (s *doneAwareRealtimeStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (s *doneAwareRealtimeStream) Close() error                      { return nil }
func (s *doneAwareRealtimeStream) CloseWithError(error) error        { return nil }
func (s *doneAwareRealtimeStream) NextOrDone(<-chan struct{}) (*genx.MessageChunk, error, bool) {
	s.calls++
	return s.chunk, nil, false
}
