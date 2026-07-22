package audiodock

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestNewValidatesComposition(t *testing.T) {
	agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) { return emptyStream{}, nil })
	tts := muxFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) { return emptyStream{}, nil })
	for _, tc := range []struct {
		name   string
		config Config
		want   string
	}{
		{name: "missing agent", config: Config{}, want: "Agent is required"},
		{name: "resolver without tts", config: Config{Agent: agent, ResolveVoice: fixedVoice("voice")}, want: "ResolveVoice requires TTS"},
		{name: "tts without resolver", config: Config{Agent: agent, TTS: tts}, want: "TTS requires ResolveVoice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.config)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("New() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDockTextOnlyPreservesChunksWithFreshResponseID(t *testing.T) {
	agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		return &sliceStream{chunks: []*genx.MessageChunk{
			{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "provider-response", Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "provider-response", Label: "assistant", EndOfStream: true}},
		}}, nil
	})
	dock, err := New(Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	if len(chunks) != 2 {
		t.Fatalf("chunks = %#v, want 2", chunks)
	}
	streamID := chunks[0].Ctrl.StreamID
	if streamID == "" || streamID == "provider-response" || chunks[1].Ctrl.StreamID != streamID {
		t.Fatalf("response StreamIDs = %q, %q", streamID, chunks[1].Ctrl.StreamID)
	}
	if chunks[0].Part != genx.Text("hello") || !chunks[1].IsEndOfStream() {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestDockKeepsUnnamedSourceChunksOnOneResponse(t *testing.T) {
	agent := fixedAgentOutput(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hel")},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("lo")},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{EndOfStream: true}},
	)
	dock, err := New(Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want 3", chunks)
	}
	streamID := chunks[0].Ctrl.StreamID
	if streamID == "" || chunks[1].Ctrl.StreamID != streamID || chunks[2].Ctrl.StreamID != streamID {
		t.Fatalf("response StreamIDs = %q, %q, %q", streamID, chunks[1].Ctrl.StreamID, chunks[2].Ctrl.StreamID)
	}
	if chunks[0].Part != genx.Text("hel") || chunks[1].Part != genx.Text("lo") || !chunks[2].IsEndOfStream() {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestDockStreamsASRInputBeforeEOS(t *testing.T) {
	input := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
	audioReceived := make(chan struct{})
	asr := transformerFunc(func(ctx context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
		go func() {
			defer output.Close()
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				if chunk == nil {
					continue
				}
				if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
					close(audioReceived)
				}
				if chunk.IsEndOfStream() {
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"}})
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", EndOfStream: true}})
					return
				}
			}
		}()
		return output, nil
	})
	agent := transformerFunc(func(ctx context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
		go func() {
			defer output.Close()
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				if chunk != nil && chunk.IsEndOfStream() {
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("world"), Ctrl: &genx.StreamCtrl{StreamID: "answer", EndOfStream: true}})
					return
				}
			}
		}()
		return output, nil
	})
	dock, err := New(Config{Agent: agent, ASR: asr})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", BeginOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-audioReceived:
	case <-time.After(time.Second):
		t.Fatal("ASR did not receive audio before EOS")
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-1", EndOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	_ = input.Close()
	chunks := readAll(t, output)
	if len(chunks) != 3 {
		t.Fatalf("chunks = %#v, want transcript text, transcript EOS, and answer", chunks)
	}
	var transcriptText, transcriptEOS, answer bool
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" && chunk.Part == genx.Text("hello") {
			transcriptText = true
		}
		if chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" && chunk.IsEndOfStream() {
			transcriptEOS = true
		}
		if chunk.Part == genx.Text("world") && chunk.IsEndOfStream() {
			answer = true
		}
	}
	if !transcriptText || !transcriptEOS || !answer {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestInputRouterRetainsBOSBeforeEventActivation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	router, err := newInputRouter(ctx, &sliceStream{chunks: []*genx.MessageChunk{{
		Role: genx.RoleUser,
		Part: genx.Text("hello"),
		Ctrl: &genx.StreamCtrl{StreamID: "input", BeginOfStream: true},
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := router.AgentInput().Next(); err != nil {
		t.Fatal(err)
	}
	events := router.ActivateEvents()
	if len(events) != 1 || !events[0].begin || events[0].streamID != "input" {
		t.Fatalf("pending events = %#v", events)
	}
	router.CloseWithError(context.Canceled)
}

func TestDockAllowsNewResponseBeforeRealtimeInputEOS(t *testing.T) {
	input := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
	agent := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
		go func() {
			defer output.Close()
			chunk, err := source.Next()
			if err != nil || chunk == nil || !chunk.IsBeginOfStream() {
				return
			}
			_ = output.Push(&genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text("realtime response"),
				Ctrl: &genx.StreamCtrl{StreamID: "response", EndOfStream: true},
			})
		}()
		return output, nil
	})
	dock, err := New(Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := input.Push(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "realtime-input", BeginOfStream: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	if len(chunks) != 1 || chunks[0].Part != genx.Text("realtime response") || !chunks[0].IsEndOfStream() {
		t.Fatalf("chunks = %#v, want response before input EOS", chunks)
	}
}

func TestDockAllowsASRResponseBeforeRealtimeInputEOS(t *testing.T) {
	input := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
	asr := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
		go func() {
			defer output.Close()
			chunk, err := source.Next()
			if err != nil || chunk == nil || !chunk.IsBeginOfStream() {
				return
			}
			_ = output.Push(&genx.MessageChunk{
				Role: genx.RoleUser,
				Name: "transcript",
				Part: genx.Text("hello"),
				Ctrl: &genx.StreamCtrl{StreamID: "realtime-input"},
			})
			_ = output.Push(&genx.MessageChunk{
				Role: genx.RoleUser,
				Name: "transcript",
				Part: genx.Text(""),
				Ctrl: &genx.StreamCtrl{StreamID: "realtime-input", EndOfStream: true},
			})
		}()
		return output, nil
	})
	agent := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
		go func() {
			defer output.Close()
			seenBOS := false
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				if chunk == nil {
					continue
				}
				if chunk.IsBeginOfStream() && chunk.Part == nil {
					seenBOS = true
				}
				if chunk.IsEndOfStream() && seenBOS {
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel,
						Part: genx.Text("response after transcript"),
						Ctrl: &genx.StreamCtrl{StreamID: "response", EndOfStream: true},
					})
					return
				}
			}
		}()
		return output, nil
	})
	dock, err := New(Config{Agent: agent, ASR: asr})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := input.Push(&genx.MessageChunk{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
		Ctrl: &genx.StreamCtrl{StreamID: "realtime-input", BeginOfStream: true},
	}); err != nil {
		t.Fatal(err)
	}

	result := make(chan struct {
		chunk *genx.MessageChunk
		err   error
	}, 1)
	go func() {
		for {
			chunk, err := output.Next()
			if err != nil || chunk == nil || chunk.Role == genx.RoleModel {
				result <- struct {
					chunk *genx.MessageChunk
					err   error
				}{chunk: chunk, err: err}
				return
			}
		}
	}()
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.chunk == nil || got.chunk.Part != genx.Text("response after transcript") || !got.chunk.IsEndOfStream() {
			t.Fatalf("chunk = %#v, want response before raw input EOS", got.chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("AudioDock waited for raw audio EOS after ASR completed the transcript")
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDockASRRoutesOnlyAudioAndBypassesOtherInput(t *testing.T) {
	var mu sync.Mutex
	var asrChunks, agentChunks []*genx.MessageChunk
	asr := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
		go func() {
			defer output.Close()
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				mu.Lock()
				asrChunks = append(asrChunks, chunk)
				mu.Unlock()
				if chunk.IsEndOfStream() {
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("spoken"), Ctrl: &genx.StreamCtrl{StreamID: "audio"}})
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}})
				}
			}
		}()
		return output, nil
	})
	agent := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
		go func() {
			defer output.Close()
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				mu.Lock()
				agentChunks = append(agentChunks, chunk)
				count := len(agentChunks)
				mu.Unlock()
				if count == 5 {
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("ok"), Ctrl: &genx.StreamCtrl{StreamID: "answer", EndOfStream: true}})
					return
				}
			}
		}()
		return output, nil
	})
	dock, err := New(Config{Agent: agent, ASR: asr})
	if err != nil {
		t.Fatal(err)
	}
	input := &sliceStream{chunks: []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: genx.Text("typed"), Ctrl: &genx.StreamCtrl{StreamID: "text", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "image/png", Data: []byte{9}}, Ctrl: &genx.StreamCtrl{StreamID: "image", EndOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio", EndOfStream: true}},
	}}
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	if len(chunks) != 3 {
		t.Fatalf("output chunks = %#v, want transcript text, transcript EOS, and answer", chunks)
	}
	var visibleTranscript, answer bool
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.Label == "transcript" && chunk.Part == genx.Text("spoken") {
			visibleTranscript = true
		}
		if chunk.Part == genx.Text("ok") {
			answer = true
		}
	}
	if !visibleTranscript || !answer {
		t.Fatalf("output chunks = %#v", chunks)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(asrChunks) != 2 {
		t.Fatalf("ASR chunks = %#v, want only 2 audio chunks", asrChunks)
	}
	for _, chunk := range asrChunks {
		mimeType, ok := chunk.MIMEType()
		if !ok || !strings.HasPrefix(mimeType, "audio/") {
			t.Fatalf("ASR received non-audio chunk %#v", chunk)
		}
	}
	if len(agentChunks) != 5 || agentChunks[0].Part != genx.Text("typed") {
		t.Fatalf("Agent chunks = %#v", agentChunks)
	}
	if blob, ok := agentChunks[1].Part.(*genx.Blob); !ok || blob.MIMEType != "image/png" {
		t.Fatalf("non-audio blob did not bypass ASR: %#v", agentChunks[1])
	}
	if !agentChunks[2].IsBeginOfStream() || agentChunks[2].Part != nil {
		t.Fatalf("audio BOS was not forwarded as an Agent control event: %#v", agentChunks[2])
	}
	if agentChunks[3].Part != genx.Text("spoken") || !agentChunks[4].IsEndOfStream() {
		t.Fatalf("ASR transcript was not forwarded: %#v", agentChunks[3:])
	}
}

func TestDockClosingOutputCancelsWholePipeline(t *testing.T) {
	cancelled := make(chan struct{})
	agent := transformerFunc(func(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
		return &contextStream{ctx: ctx, cancelled: cancelled}, nil
	})
	dock, err := New(Config{Agent: agent, ASR: passthroughTransformer{}})
	if err != nil {
		t.Fatal(err)
	}
	input := newBlockingStream()
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("closing AudioDock output did not cancel the Agent pipeline")
	}
	select {
	case <-input.closed:
	case <-time.After(time.Second):
		t.Fatal("closing AudioDock output did not close the upstream input")
	}
}

func TestDockReplacementBOSDiscardsReadAheadOutput(t *testing.T) {
	input := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
	bosSeen := make(chan struct{})
	agent := transformerFunc(func(ctx context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 8})
		_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("delivered"), Ctrl: &genx.StreamCtrl{StreamID: "old"}})
		_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("stale"), Ctrl: &genx.StreamCtrl{StreamID: "old"}})
		go func() {
			defer output.Close()
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				if chunk.IsBeginOfStream() {
					close(bosSeen)
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "old", EndOfStream: true, Error: "interrupted"}})
				}
				if chunk.IsEndOfStream() {
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("fresh"), Ctrl: &genx.StreamCtrl{StreamID: "new"}})
					_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "new", EndOfStream: true}})
					return
				}
				select {
				case <-ctx.Done():
					return
				default:
				}
			}
		}()
		return output, nil
	})
	dock, err := New(Config{Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	first, err := output.Next()
	if err != nil || first.Part != genx.Text("delivered") {
		t.Fatalf("first output = (%#v, %v)", first, err)
	}
	if err := input.Push(&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "input", BeginOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-bosSeen:
	case <-time.After(time.Second):
		t.Fatal("Agent did not receive replacement BOS")
	}
	if err := input.Push(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("new"), Ctrl: &genx.StreamCtrl{StreamID: "input", EndOfStream: true}}); err != nil {
		t.Fatal(err)
	}
	_ = input.Close()
	chunks := append([]*genx.MessageChunk{first}, readAll(t, output)...)
	var interrupted, fresh bool
	for _, chunk := range chunks {
		if chunk.Part == genx.Text("stale") {
			t.Fatalf("unpulled stale output escaped after replacement BOS: %#v", chunks)
		}
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID == first.Ctrl.StreamID && chunk.IsEndOfStream() && chunk.Ctrl.Error == "interrupted" {
			interrupted = true
		}
		if chunk.Part == genx.Text("fresh") && chunk.Ctrl != nil && chunk.Ctrl.StreamID != first.Ctrl.StreamID {
			fresh = true
		}
	}
	if !interrupted || !fresh {
		t.Fatalf("chunks = %#v, want interrupted old route and fresh new route", chunks)
	}
}

func TestDockReturnsTextBeforeTTSAndMergesAudio(t *testing.T) {
	releaseAudio := make(chan struct{})
	ttsStarted := make(chan struct{})
	var request VoiceRequest
	agent := fixedAgentOutput(
		&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "provider", Label: "assistant"}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "provider", Label: "assistant", EndOfStream: true}},
	)
	tts := muxFunc(func(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
		if pattern != "voice/narrator" {
			t.Errorf("TTS pattern = %q", pattern)
		}
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
		go func() {
			defer output.Close()
			first, err := input.Next()
			if err != nil || first == nil {
				return
			}
			close(ttsStarted)
			select {
			case <-releaseAudio:
			case <-ctx.Done():
				return
			}
			streamID := first.Ctrl.StreamID
			_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Name: first.Name, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2}}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: first.Ctrl.Label}})
			_, _ = input.Next()
		}()
		return output, nil
	})
	dock, err := New(Config{
		Agent: agent,
		TTS:   tts,
		ResolveVoice: func(_ context.Context, value VoiceRequest) (string, error) {
			request = value
			return "voice/narrator", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := output.Next()
	if err != nil {
		t.Fatal(err)
	}
	if first.Part != genx.Text("hello") {
		t.Fatalf("first chunk = %#v", first)
	}
	select {
	case <-ttsStarted:
	case <-time.After(time.Second):
		t.Fatal("TTS did not start")
	}
	if request.StreamID != first.Ctrl.StreamID || request.Name != "answer" || request.Label != "assistant" {
		t.Fatalf("voice request = %#v", request)
	}
	close(releaseAudio)
	chunks := append([]*genx.MessageChunk{first}, readAll(t, output)...)
	var textEOS, audio, audioEOS int
	textEOSIndex, audioEOSIndex := -1, -1
	for index, chunk := range chunks {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != first.Ctrl.StreamID {
			t.Fatalf("chunk route = %#v, want %q", chunk, first.Ctrl.StreamID)
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			if chunk.IsEndOfStream() {
				textEOS++
				textEOSIndex = index
			}
		case *genx.Blob:
			if len(part.Data) > 0 {
				audio++
			}
			if chunk.IsEndOfStream() {
				audioEOS++
				audioEOSIndex = index
			}
		}
	}
	if textEOS != 1 || audio != 1 || audioEOS != 1 {
		t.Fatalf("textEOS/audio/audioEOS = %d/%d/%d; chunks=%#v", textEOS, audio, audioEOS, chunks)
	}
	if textEOSIndex < 0 || audioEOSIndex <= textEOSIndex {
		t.Fatalf("audio EOS index = %d, want after text EOS index %d; chunks=%#v", audioEOSIndex, textEOSIndex, chunks)
	}
}

func TestDockVoiceFailureTerminatesOnlyRoute(t *testing.T) {
	want := errors.New("voice unavailable")
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
		),
		TTS: muxFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
			return emptyStream{}, nil
		}),
		ResolveVoice: func(context.Context, VoiceRequest) (string, error) { return "", want },
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	if len(chunks) != 2 || chunks[1].Ctrl == nil || !chunks[1].IsEndOfStream() || !strings.Contains(chunks[1].Ctrl.Error, want.Error()) {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestDockSurfacesTTSFailureAfterTextEOS(t *testing.T) {
	want := errors.New("tts unavailable")
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
		),
		TTS: muxFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
			return errorStream{err: want}, nil
		}),
		ResolveVoice: fixedVoice("voice/narrator"),
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && strings.Contains(chunk.Ctrl.Error, want.Error()) {
			return
		}
	}
	t.Fatalf("TTS failure was not surfaced: %#v", chunks)
}

func TestDockConcurrentTransformsDoNotShareVoiceState(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]int)
	dock, err := New(Config{
		Agent: transformerFunc(func(_ context.Context, input genx.Stream) (genx.Stream, error) {
			chunk, err := input.Next()
			if err != nil {
				return nil, err
			}
			text := chunk.Part.(genx.Text)
			return fixedAgentOutput(&genx.MessageChunk{Role: genx.RoleModel, Name: string(text), Part: text, Ctrl: &genx.StreamCtrl{StreamID: string(text), EndOfStream: true}}).Transform(context.Background(), emptyStream{})
		}),
		TTS: muxFunc(func(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
			output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
			go func() {
				defer output.Close()
				chunk, err := input.Next()
				if err != nil {
					return
				}
				_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: chunk.Ctrl.StreamID, EndOfStream: true}})
			}()
			return output, nil
		}),
		ResolveVoice: func(_ context.Context, value VoiceRequest) (string, error) {
			mu.Lock()
			seen[value.Name]++
			mu.Unlock()
			return "voice/" + value.Name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, text := range []string{"alpha", "beta"} {
		text := text
		wg.Add(1)
		go func() {
			defer wg.Done()
			input := &sliceStream{chunks: []*genx.MessageChunk{{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: text, EndOfStream: true}}}}
			output, err := dock.Transform(t.Context(), input)
			if err != nil {
				t.Errorf("Transform(%s): %v", text, err)
				return
			}
			_ = readAll(t, output)
		}()
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if seen["alpha"] != 1 || seen["beta"] != 1 {
		t.Fatalf("voice calls = %#v", seen)
	}
}

type transformerFunc func(context.Context, genx.Stream) (genx.Stream, error)

func (f transformerFunc) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	return f(ctx, input)
}

type muxFunc func(context.Context, string, genx.Stream) (genx.Stream, error)

func (f muxFunc) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	return f(ctx, pattern, input)
}

func fixedVoice(pattern string) VoiceResolver {
	return func(context.Context, VoiceRequest) (string, error) { return pattern, nil }
}

func fixedAgentOutput(chunks ...*genx.MessageChunk) genx.Transformer {
	return transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		return &sliceStream{chunks: chunks}, nil
	})
}

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (emptyStream) Close() error                      { return nil }
func (emptyStream) CloseWithError(error) error        { return nil }

type errorStream struct{ err error }

func (s errorStream) Next() (*genx.MessageChunk, error) { return nil, s.err }
func (errorStream) Close() error                        { return nil }
func (errorStream) CloseWithError(error) error          { return nil }

type passthroughTransformer struct{}

func (passthroughTransformer) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	return input, nil
}

type contextStream struct {
	ctx       context.Context
	cancelled chan struct{}
	once      sync.Once
}

func (s *contextStream) Next() (*genx.MessageChunk, error) {
	<-s.ctx.Done()
	s.once.Do(func() { close(s.cancelled) })
	return nil, s.ctx.Err()
}

func (s *contextStream) Close() error {
	s.once.Do(func() { close(s.cancelled) })
	return nil
}

func (s *contextStream) CloseWithError(error) error { return s.Close() }

type blockingStream struct {
	closed chan struct{}
	once   sync.Once
}

func newBlockingStream() *blockingStream { return &blockingStream{closed: make(chan struct{})} }

func (s *blockingStream) Next() (*genx.MessageChunk, error) {
	<-s.closed
	return nil, io.EOF
}

func (s *blockingStream) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func (s *blockingStream) CloseWithError(error) error { return s.Close() }

type sliceStream struct {
	mu     sync.Mutex
	chunks []*genx.MessageChunk
	closed bool
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.chunks) == 0 {
		return nil, io.EOF
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *sliceStream) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	return nil
}

func (s *sliceStream) CloseWithError(error) error { return s.Close() }

func readAll(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			return chunks
		}
		if err != nil {
			t.Fatalf("stream.Next() error = %v", err)
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
}
