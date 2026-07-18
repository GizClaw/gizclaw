package agenthost

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestAudioOutputTracksKeyByStreamAndCanonicalMIME(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	chunks := []*genx.MessageChunk{
		pcmOutputChunk("stream-a", "audio/L16; rate=16000; channels=1", []byte{1, 0}, false, ""),
		pcmOutputChunk("stream-a", "AUDIO/L16; channels=1; rate=16000", []byte{2, 0}, false, ""),
		pcmOutputChunk("stream-a", "audio/L16; rate=24000; channels=1", []byte{3, 0}, false, ""),
		pcmOutputChunk("stream-b", "audio/L16; rate=16000; channels=1", []byte{4, 0}, false, ""),
	}
	for _, chunk := range chunks {
		if err := tracks.consume(chunk); err != nil {
			t.Fatalf("consume(%#v) error = %v", chunk.Ctrl, err)
		}
	}
	if got := len(tracks.channels); got != 3 {
		t.Fatalf("active channels = %d, want 3", got)
	}
	if got := len(creator.tracks); got != 3 {
		t.Fatalf("created tracks = %d, want 3", got)
	}
}

func TestAudioOutputTracksMIMEEOSClosesOnlyMatchingTrack(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	mime16 := "audio/L16; rate=16000; channels=1"
	mime24 := "audio/L16; rate=24000; channels=1"
	for _, chunk := range []*genx.MessageChunk{
		pcmOutputChunk("stream-a", mime16, []byte{1, 0}, false, ""),
		pcmOutputChunk("stream-a", mime24, []byte{2, 0}, false, ""),
		pcmOutputChunk("stream-b", mime16, []byte{3, 0}, false, ""),
		pcmOutputChunk("stream-a", mime16, []byte{4, 0}, true, ""),
	} {
		if err := tracks.consume(chunk); err != nil {
			t.Fatalf("consume() error = %v", err)
		}
	}
	if got := len(tracks.channels); got != 2 {
		t.Fatalf("active channels after MIME EOS = %d, want 2", got)
	}
	if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{5, 0})); err == nil || !strings.Contains(err.Error(), "CloseWrite") {
		t.Fatalf("write after normal EOS error = %v, want CloseWrite", err)
	}
	if err := creator.tracks[1].Write(pcm.L16Mono24K.DataChunk([]byte{6, 0})); err != nil {
		t.Fatalf("unrelated MIME track write error = %v", err)
	}
	if err := creator.tracks[2].Write(pcm.L16Mono16K.DataChunk([]byte{7, 0})); err != nil {
		t.Fatalf("unrelated stream track write error = %v", err)
	}
}

func TestAudioOutputTracksErrorEOSAndRouteEOS(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	mime16 := "audio/L16; rate=16000; channels=1"
	mime24 := "audio/L16; rate=24000; channels=1"
	for _, chunk := range []*genx.MessageChunk{
		pcmOutputChunk("stream-a", mime16, []byte{1, 0}, false, ""),
		pcmOutputChunk("stream-a", mime24, []byte{2, 0}, false, ""),
		pcmOutputChunk("stream-b", mime16, []byte{3, 0}, false, ""),
		pcmOutputChunk("stream-a", mime16, nil, true, "interrupted"),
	} {
		if err := tracks.consume(chunk); err != nil {
			t.Fatalf("consume() error = %v", err)
		}
	}
	if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{4, 0})); err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("write after Error EOS error = %v, want interrupted", err)
	}
	if err := tracks.consume(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "stream-a", EndOfStream: true}}); err != nil {
		t.Fatalf("control EOS error = %v", err)
	}
	if got := len(tracks.channels); got != 1 {
		t.Fatalf("active channels after route EOS = %d, want 1", got)
	}
	if err := creator.tracks[1].Write(pcm.L16Mono24K.DataChunk([]byte{5, 0})); err == nil || !strings.Contains(err.Error(), "CloseWrite") {
		t.Fatalf("route track write error = %v, want CloseWrite", err)
	}
	if err := creator.tracks[2].Write(pcm.L16Mono16K.DataChunk([]byte{6, 0})); err != nil {
		t.Fatalf("other route track write error = %v", err)
	}
}

func TestAudioOutputTracksRejectInvalidPCMWithContext(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	err := tracks.consume(pcmOutputChunk("answer", "audio/L16; rate=44100; channels=1", []byte{1, 0}, false, ""))
	if err == nil || !strings.Contains(err.Error(), `stream_id="answer"`) || !strings.Contains(err.Error(), "44100") {
		t.Fatalf("consume invalid PCM error = %v", err)
	}
	if len(tracks.channels) != 0 {
		t.Fatalf("active channels after invalid PCM = %d, want 0", len(tracks.channels))
	}
}

func TestAudioOutputTracksRejectMalformedAudioMIMEWithContext(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	err := tracks.consume(pcmOutputChunk("answer", "audio/L16; rate", []byte{1, 0}, false, ""))
	if err == nil || !strings.Contains(err.Error(), `stream_id="answer"`) || !strings.Contains(err.Error(), `mime="audio/L16; rate"`) {
		t.Fatalf("consume malformed MIME error = %v", err)
	}
	if len(tracks.channels) != 0 {
		t.Fatalf("active channels after malformed MIME = %d, want 0", len(tracks.channels))
	}
}

func TestAudioOutputMP3DecoderFinalizesToPCM(t *testing.T) {
	var encoded bytes.Buffer
	encoder, err := mp3.NewEncoder(&encoded, 24000, 1)
	if err != nil {
		t.Fatalf("NewEncoder() error = %v", err)
	}
	input := make([]byte, 24000/50*2)
	for i := 0; i < len(input); i += 2 {
		input[i] = byte(i)
	}
	if _, err := encoder.Write(input); err != nil {
		t.Fatalf("encoder.Write() error = %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("encoder.Close() error = %v", err)
	}

	decoder, err := newAudioPCMDecoder("audio/mpeg")
	if err != nil {
		t.Fatalf("newAudioPCMDecoder() error = %v", err)
	}
	if chunks, err := decoder.Decode(encoded.Bytes()); err != nil || len(chunks) != 0 {
		t.Fatalf("Decode() = %d chunks, %v; want buffered MP3", len(chunks), err)
	}
	chunks, err := decoder.(audioPCMFinalizer).Finalize()
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if len(chunks) != 1 || chunks[0].Format() != pcm.L16Mono48K || chunks[0].Len() == 0 {
		t.Fatalf("Finalize() chunks = %#v, want non-empty 48 kHz mono PCM", chunks)
	}
	if err := decoder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestRawOpusDecoderAcceptsMaximumDurationPacket(t *testing.T) {
	encoder, err := opus.NewEncoder(48000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder() error = %v", err)
	}
	defer encoder.Close()
	frames := make([][]byte, 6)
	for i := range frames {
		frames[i], err = encoder.Encode(make([]int16, 48000*20/1000), 48000*20/1000)
		if err != nil {
			t.Fatalf("Encode(20ms frame %d) error = %v", i, err)
		}
	}
	packet := []byte{frames[0][0] | 0x03, byte(len(frames))}
	for _, frame := range frames {
		packet = append(packet, frame[1:]...)
	}
	decoder, err := newAudioPCMDecoder("audio/opus")
	if err != nil {
		t.Fatalf("newAudioPCMDecoder() error = %v", err)
	}
	defer decoder.Close()
	chunks, err := decoder.Decode(packet)
	if err != nil {
		t.Fatalf("Decode(120ms) error = %v", err)
	}
	if len(chunks) != 1 || chunks[0].Len() != 48000*120/1000*2 {
		t.Fatalf("Decode(120ms) chunks = %#v", chunks)
	}
}

func TestMixerOutputPublishesEOSAfterTrackDrain(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	observed := make(chan struct{})
	source := &blockingSliceStream{sliceStream: sliceStream{chunks: []*genx.MessageChunk{
		pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, true, ""),
	}, doneErr: genx.ErrDone}, release: make(chan struct{})}
	output := newRecordingObservationStream(source)
	done := make(chan error, 1)
	go func() {
		done <- (MixerOutput{
			Tracks:            creator,
			WaitForAudioDrain: true,
			Observe: func(*genx.MessageChunk) error {
				close(observed)
				return nil
			},
		}).ConsumeAgentOutput(context.Background(), output)
	}()
	select {
	case <-observed:
		t.Fatal("EOS was observed before mixer drain")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case <-output.observed:
		t.Fatal("stream observation was acknowledged before mixer drain")
	default:
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buffer := make([]byte, creator.mixer.Output().BytesInDuration(60*time.Millisecond))
		_, _ = creator.mixer.Read(buffer)
		_, _ = creator.mixer.Read(buffer)
	}()
	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("EOS was not observed after mixer drain")
	}
	select {
	case <-output.observed:
	case <-time.After(time.Second):
		t.Fatal("stream observation was not acknowledged after mixer drain")
	}
	close(source.release)
	if err := creator.mixer.Close(); err != nil {
		t.Fatalf("mixer.Close() error = %v", err)
	}
	<-readDone
	if err := <-done; err != nil {
		t.Fatalf("ConsumeAgentOutput() error = %v", err)
	}
}

type recordingObservationStream struct {
	genx.Stream
	deferred chan struct{}
	observed chan *genx.MessageChunk
	once     sync.Once
}

func newRecordingObservationStream(stream genx.Stream) *recordingObservationStream {
	return &recordingObservationStream{
		Stream:   stream,
		deferred: make(chan struct{}),
		observed: make(chan *genx.MessageChunk, 1),
	}
}

func (s *recordingObservationStream) DeferOutputObservation() {
	s.once.Do(func() { close(s.deferred) })
}

func (s *recordingObservationStream) ObserveOutput(chunk *genx.MessageChunk) {
	s.observed <- chunk
}

type blockingSliceStream struct {
	sliceStream
	release chan struct{}
}

func (s *blockingSliceStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) > 0 {
		return s.sliceStream.Next()
	}
	<-s.release
	return nil, s.doneErr
}

func TestMixerOutputConsumesInterruptWhilePreviousTrackDrains(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	var observed []*genx.MessageChunk
	output := &notifyingSliceStream{sliceStream: sliceStream{chunks: []*genx.MessageChunk{
		pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, true, ""),
		pcmOutputChunk("answer", "audio/pcm", nil, true, "interrupted"),
	}, doneErr: genx.ErrDone}, secondRead: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- (MixerOutput{
			Tracks:            creator,
			WaitForAudioDrain: true,
			Observe: func(chunk *genx.MessageChunk) error {
				observed = append(observed, chunk)
				return nil
			},
		}).ConsumeAgentOutput(t.Context(), output)
	}()
	var track pcm.Track
	select {
	case track = <-creator.created:
	case <-time.After(time.Second):
		t.Fatal("audio track was not created")
	}
	select {
	case <-output.secondRead:
	case <-time.After(time.Second):
		t.Fatal("consumer stopped reading while the previous track drained")
	}
	waitForTrackWriteError(t, track, "interrupted")
	buffer := make([]byte, creator.mixer.Output().BytesInDuration(60*time.Millisecond))
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = creator.mixer.Read(buffer)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ConsumeAgentOutput() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ConsumeAgentOutput() did not finish after interruption")
	}
	if err := creator.mixer.Close(); err != nil {
		t.Fatalf("mixer.Close() error = %v", err)
	}
	<-readDone
	if len(observed) != 1 || observed[0].Ctrl == nil || observed[0].Ctrl.Error != "interrupted" {
		t.Fatalf("observed chunks = %#v, want only interrupted EOS", observed)
	}
}

func TestMixerOutputConsumesRouteInterruptWhilePreviousTrackDrains(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	var observed []*genx.MessageChunk
	interrupt := &genx.MessageChunk{
		Ctrl: &genx.StreamCtrl{StreamID: "answer", EndOfStream: true, Error: "interrupted"},
	}
	routeEOS := &genx.MessageChunk{
		Ctrl: &genx.StreamCtrl{StreamID: "answer", EndOfStream: true},
	}
	output := &notifyingSliceStream{sliceStream: sliceStream{chunks: []*genx.MessageChunk{
		pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, true, ""),
		routeEOS,
		interrupt,
	}, doneErr: genx.ErrDone}, secondRead: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- (MixerOutput{
			Tracks:            creator,
			WaitForAudioDrain: true,
			Observe: func(chunk *genx.MessageChunk) error {
				observed = append(observed, chunk)
				return nil
			},
		}).ConsumeAgentOutput(t.Context(), output)
	}()
	var track pcm.Track
	select {
	case track = <-creator.created:
	case <-time.After(time.Second):
		t.Fatal("audio track was not created")
	}
	select {
	case <-output.secondRead:
	case <-time.After(time.Second):
		t.Fatal("consumer stopped reading while the previous track drained")
	}
	waitForTrackWriteError(t, track, "interrupted")
	buffer := make([]byte, creator.mixer.Output().BytesInDuration(60*time.Millisecond))
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = creator.mixer.Read(buffer)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ConsumeAgentOutput() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ConsumeAgentOutput() did not finish after route interruption")
	}
	if err := creator.mixer.Close(); err != nil {
		t.Fatalf("mixer.Close() error = %v", err)
	}
	<-readDone
	if len(observed) != 1 || observed[0] != interrupt {
		t.Fatalf("observed chunks = %#v, want only route interrupted EOS", observed)
	}
}

func TestMixerOutputClosesBlockedReaderAfterObserveError(t *testing.T) {
	wantErr := errors.New("observe failed")
	output := newCloseAwareOutputStream(&genx.MessageChunk{Part: genx.Text("hello")})
	err := (MixerOutput{Observe: func(*genx.MessageChunk) error {
		return wantErr
	}}).ConsumeAgentOutput(t.Context(), output)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ConsumeAgentOutput() error = %v, want %v", err, wantErr)
	}
	select {
	case <-output.nextDone:
	case <-time.After(time.Second):
		t.Fatal("output.Next() remained blocked after consumer failure")
	}
	if !errors.Is(output.closeError(), wantErr) {
		t.Fatalf("output close error = %v, want %v", output.closeError(), wantErr)
	}
}

type closeAwareOutputStream struct {
	chunks   []*genx.MessageChunk
	closed   chan struct{}
	nextDone chan struct{}
	once     sync.Once
	mu       sync.Mutex
	err      error
}

func newCloseAwareOutputStream(chunks ...*genx.MessageChunk) *closeAwareOutputStream {
	return &closeAwareOutputStream{
		chunks:   chunks,
		closed:   make(chan struct{}),
		nextDone: make(chan struct{}),
	}
}

func (s *closeAwareOutputStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) > 0 {
		chunk := s.chunks[0]
		s.chunks = s.chunks[1:]
		return chunk, nil
	}
	<-s.closed
	close(s.nextDone)
	return nil, s.closeError()
}

func (s *closeAwareOutputStream) Close() error {
	return s.CloseWithError(nil)
}

func (s *closeAwareOutputStream) CloseWithError(err error) error {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
	s.once.Do(func() { close(s.closed) })
	return nil
}

func (s *closeAwareOutputStream) closeError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func waitForTrackWriteError(t *testing.T, track pcm.Track, contains string) {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		lastErr = track.Write(pcm.L16Mono16K.DataChunk(nil))
		if lastErr != nil && strings.Contains(lastErr.Error(), contains) {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("track write error = %v, want error containing %q", lastErr, contains)
		case <-ticker.C:
		}
	}
}

func TestAudioOutputTracksWaitCancellationKeepsPendingTrack(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	if err := tracks.consume(pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, true, "")); err != nil {
		t.Fatalf("consume() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tracks.waitPending(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitPending() error = %v, want context canceled", err)
	}
	if !tracks.hasPending() {
		t.Fatal("pending track was removed before it drained")
	}
	if err := tracks.closeWithError(ctx.Err()); err != nil {
		t.Fatalf("closeWithError() error = %v", err)
	}
	if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{2, 0})); !errors.Is(err, context.Canceled) {
		t.Fatalf("write after cancellation error = %v, want context canceled", err)
	}
}

type notifyingSliceStream struct {
	sliceStream
	reads      int
	secondRead chan struct{}
}

func (s *notifyingSliceStream) Next() (*genx.MessageChunk, error) {
	s.reads++
	if s.reads == 2 {
		close(s.secondRead)
	}
	return s.sliceStream.Next()
}

func TestAudioOutputTracksErrorEOSAbandonsPartialOgg(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	if err := tracks.consume(pcmOutputChunk("answer", "audio/ogg", []byte("OggS"), false, "")); err != nil {
		t.Fatalf("consume partial Ogg error = %v", err)
	}
	if err := tracks.consume(pcmOutputChunk("answer", "audio/ogg", nil, true, "interrupted")); err != nil {
		t.Fatalf("consume interrupted EOS error = %v", err)
	}
}

func TestOggOpusPCMDecoderAcceptsChainedStreams(t *testing.T) {
	encoder, err := opus.NewEncoder(48000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder() error = %v", err)
	}
	defer encoder.Close()
	packet, err := encoder.Encode(make([]int16, 960), 960)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	first, err := historyOggOpusAsset([][]byte{packet})
	if err != nil {
		t.Fatalf("historyOggOpusAsset(first) error = %v", err)
	}
	second, err := historyOggOpusAsset([][]byte{packet})
	if err != nil {
		t.Fatalf("historyOggOpusAsset(second) error = %v", err)
	}
	decoder, err := newAudioPCMDecoder("audio/ogg")
	if err != nil {
		t.Fatalf("newAudioPCMDecoder() error = %v", err)
	}
	defer decoder.Close()
	chunks, err := decoder.Decode(append(first, second...))
	if err != nil {
		t.Fatalf("Decode(chained Ogg) error = %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("Decode(chained Ogg) chunks = %d, want 2", len(chunks))
	}
}

func pcmOutputChunk(streamID, mimeType string, data []byte, eos bool, errorText string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: mimeType, Data: data},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: eos, Error: errorText},
	}
}

type recordingAudioTrackCreator struct {
	mixer   *pcm.Mixer
	tracks  []pcm.Track
	created chan pcm.Track
}

func newRecordingAudioTrackCreator() *recordingAudioTrackCreator {
	return &recordingAudioTrackCreator{
		mixer:   pcm.NewMixer(pcm.L16Mono16K),
		created: make(chan pcm.Track, 1),
	}
}

func (c *recordingAudioTrackCreator) CreateAudioTrack(opts ...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	track, ctrl, err := c.mixer.CreateTrack(opts...)
	if err == nil {
		c.tracks = append(c.tracks, track)
		select {
		case c.created <- track:
		default:
		}
	}
	return track, ctrl, err
}

func TestMixerOutputOuterCloseModes(t *testing.T) {
	t.Run("normal completion closes writes", func(t *testing.T) {
		creator := newRecordingAudioTrackCreator()
		output := &sliceStream{
			chunks:  []*genx.MessageChunk{pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, false, "")},
			doneErr: genx.ErrDone,
		}
		if err := (MixerOutput{Tracks: creator}).ConsumeAgentOutput(t.Context(), output); err != nil {
			t.Fatalf("ConsumeAgentOutput() error = %v", err)
		}
		if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{2, 0})); err == nil || !strings.Contains(err.Error(), "CloseWrite") {
			t.Fatalf("write after completion error = %v, want CloseWrite", err)
		}
	})

	t.Run("normal completion drains active track", func(t *testing.T) {
		creator := newRecordingAudioTrackCreator()
		output := &sliceStream{
			chunks:  []*genx.MessageChunk{pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, false, "")},
			doneErr: genx.ErrDone,
		}
		done := make(chan error, 1)
		go func() {
			done <- (MixerOutput{Tracks: creator, WaitForAudioDrain: true}).ConsumeAgentOutput(t.Context(), output)
		}()
		select {
		case <-creator.created:
		case <-time.After(time.Second):
			t.Fatal("audio track was not created")
		}
		select {
		case err := <-done:
			t.Fatalf("ConsumeAgentOutput() finished before active track drained: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		buffer := make([]byte, creator.mixer.Output().BytesInDuration(60*time.Millisecond))
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			_, _ = creator.mixer.Read(buffer)
			_, _ = creator.mixer.Read(buffer)
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("ConsumeAgentOutput() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("ConsumeAgentOutput() did not finish after active track drained")
		}
		if err := creator.mixer.Close(); err != nil {
			t.Fatalf("mixer.Close() error = %v", err)
		}
		<-readDone
	})

	t.Run("outer error closes with error", func(t *testing.T) {
		creator := newRecordingAudioTrackCreator()
		wantErr := errors.New("provider failed")
		output := &sliceStream{
			chunks:  []*genx.MessageChunk{pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, false, "")},
			doneErr: wantErr,
		}
		err := (MixerOutput{Tracks: creator}).ConsumeAgentOutput(t.Context(), output)
		if !errors.Is(err, wantErr) {
			t.Fatalf("ConsumeAgentOutput() error = %v, want %v", err, wantErr)
		}
		if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{2, 0})); !errors.Is(err, wantErr) {
			t.Fatalf("write after outer error = %v, want %v", err, wantErr)
		}
	})
}
