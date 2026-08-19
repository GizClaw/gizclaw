package audiodock

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
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

func TestDockDoesNotTerminateTranscriptOnHistoryAudioEOS(t *testing.T) {
	asr := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
		go func() {
			defer output.Close()
			defer source.Close()
			for {
				_, err := source.Next()
				if err != nil {
					return
				}
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Name: "transcript",
					Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", BeginOfStream: true},
				})
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Name: "transcript",
					Part: genx.Text("hello"),
					Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"},
				})
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Name: "transcript",
					Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}},
					Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: genx.HistoryUserAudioLabel},
				})
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Name: "transcript",
					Part: &genx.Blob{MIMEType: "audio/opus"},
					Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: genx.HistoryUserAudioLabel, EndOfStream: true},
				})
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Name: "transcript",
					Part: genx.Text("hello"),
					Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript"},
				})
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleUser,
					Name: "transcript",
					Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{StreamID: "audio-1", Label: "transcript", EndOfStream: true},
				})
				return
			}
		}()
		return output, nil
	})
	agent := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 4})
		go func() {
			defer output.Close()
			defer source.Close()
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				if _, isText := chunk.Part.(genx.Text); isText {
					continue
				}
				_ = output.Push(chunk)
			}
		}()
		return output, nil
	})
	dock, err := New(Config{Agent: agent, ASR: asr})
	if err != nil {
		t.Fatal(err)
	}
	input := &sliceStream{chunks: []*genx.MessageChunk{{
		Role: genx.RoleUser,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{9}},
		Ctrl: &genx.StreamCtrl{StreamID: "audio-1", BeginOfStream: true},
	}}}
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)

	var historyData, historyEOS, transcriptText, transcriptEOS int
	for _, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		switch chunk.Ctrl.Label {
		case genx.HistoryUserAudioLabel:
			if chunk.IsEndOfStream() {
				historyEOS++
			} else if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
				historyData++
			}
		case "transcript":
			if chunk.IsEndOfStream() {
				transcriptEOS++
			} else if chunk.Part == genx.Text("hello") {
				transcriptText++
			}
		}
	}
	if historyData != 1 || historyEOS != 1 {
		t.Fatalf("history audio data/EOS = %d/%d, want 1/1; chunks = %#v", historyData, historyEOS, chunks)
	}
	if transcriptText != 2 || transcriptEOS != 1 {
		t.Fatalf("transcript text/EOS = %d/%d, want 2/1; chunks = %#v", transcriptText, transcriptEOS, chunks)
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

func TestDockKeepsCompositionAliveAcrossRepeatedInterruptions(t *testing.T) {
	const interruptions = 3
	input := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 16})
	asr := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 16})
		go func() {
			defer output.Close()
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				if chunk == nil || !chunk.IsEndOfStream() {
					continue
				}
				streamID := dockStreamID(chunk)
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleUser, Name: "transcript", Part: genx.Text("turn:" + streamID),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript", BeginOfStream: true},
				})
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""),
					Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: "transcript", EndOfStream: true},
				})
			}
		}()
		return output, nil
	})

	agent := transformerFunc(func(_ context.Context, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 32})
		go func() {
			defer output.Close()
			var active chan struct{}
			var turns sync.WaitGroup
			turn := 0
			for {
				chunk, err := source.Next()
				if err != nil {
					if active != nil {
						close(active)
					}
					turns.Wait()
					return
				}
				if chunk == nil {
					continue
				}
				if chunk.IsBeginOfStream() && chunk.Part == nil && active != nil {
					close(active)
					active = nil
				}
				if _, ok := chunk.Part.(genx.Text); !ok || !chunk.IsEndOfStream() {
					continue
				}
				turn++
				streamID := fmt.Sprintf("agent-response-%d", turn)
				cancel := make(chan struct{})
				active = cancel
				turns.Add(1)
				go func(current int, responseID string, interrupted <-chan struct{}) {
					defer turns.Done()
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel, Name: "answer", Part: genx.Text(fmt.Sprintf("answer-%d", current)),
						Ctrl: &genx.StreamCtrl{StreamID: responseID, Label: "assistant", BeginOfStream: true},
					})
					if current <= interruptions {
						<-interrupted
						_ = output.Push(&genx.MessageChunk{
							Role: genx.RoleModel, Name: "answer", Part: genx.Text(""),
							Ctrl: &genx.StreamCtrl{StreamID: responseID, Label: "assistant", EndOfStream: true, Error: "interrupted"},
						})
						return
					}
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel, Name: "answer", Part: genx.Text(""),
						Ctrl: &genx.StreamCtrl{StreamID: responseID, Label: "assistant", EndOfStream: true},
					})
				}(turn, streamID, cancel)
			}
		}()
		return output, nil
	})

	tts := muxFunc(func(ctx context.Context, _ string, source genx.Stream) (genx.Stream, error) {
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 8})
		go func() {
			defer output.Close()
			begun := false
			for {
				chunk, err := source.Next()
				if err != nil {
					return
				}
				if chunk == nil {
					continue
				}
				if !begun {
					begun = true
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel, Name: chunk.Name, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}},
						Ctrl: &genx.StreamCtrl{StreamID: dockStreamID(chunk), Label: "assistant", BeginOfStream: true},
					})
				}
				if chunk.IsEndOfStream() {
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel, Name: chunk.Name, Part: &genx.Blob{MIMEType: "audio/opus"},
						Ctrl: &genx.StreamCtrl{StreamID: dockStreamID(chunk), Label: "assistant", EndOfStream: true},
					})
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

	dock, err := New(Config{Agent: agent, ASR: asr, TTS: tts, ResolveVoice: fixedVoice("voice/test")})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		chunk *genx.MessageChunk
		err   error
	}
	results := make(chan result, 64)
	go func() {
		for {
			chunk, nextErr := output.Next()
			results <- result{chunk: chunk, err: nextErr}
			if nextErr != nil {
				return
			}
		}
	}()

	responses := make(map[string]map[string]int)
	responseEOS := make(map[string]map[string]int)
	responseIDs := make([]string, interruptions+1)
	var assistantChunks []*genx.MessageChunk
	for turn := 1; turn <= interruptions+1; turn++ {
		inputID := fmt.Sprintf("input-%d", turn)
		for _, chunk := range []*genx.MessageChunk{
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: inputID, BeginOfStream: true}},
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{byte(turn)}}, Ctrl: &genx.StreamCtrl{StreamID: inputID}},
			{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: inputID, EndOfStream: true}},
		} {
			if err := input.Push(chunk); err != nil {
				t.Fatalf("push turn %d: %v", turn, err)
			}
		}
		responseID := ""
		audioStarted := make(map[string]bool)
		for {
			select {
			case got := <-results:
				if got.err != nil {
					t.Fatalf("read turn %d: %v", turn, got.err)
				}
				chunk := got.chunk
				if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != "assistant" {
					continue
				}
				assistantChunks = append(assistantChunks, chunk.Clone())
				mimeType := "text/plain"
				if value, ok := chunk.MIMEType(); ok {
					mimeType = value
				}
				if responses[chunk.Ctrl.StreamID] == nil {
					responses[chunk.Ctrl.StreamID] = make(map[string]int)
				}
				if responseEOS[chunk.Ctrl.StreamID] == nil {
					responseEOS[chunk.Ctrl.StreamID] = make(map[string]int)
				}
				if mimeType == "audio/opus" && chunk.IsBeginOfStream() {
					audioStarted[chunk.Ctrl.StreamID] = true
				}
				if chunk.IsEndOfStream() {
					responseEOS[chunk.Ctrl.StreamID][mimeType]++
					if chunk.Ctrl.Error == "interrupted" {
						responses[chunk.Ctrl.StreamID][mimeType]++
					}
				}
				if text, ok := chunk.Part.(genx.Text); ok && text == genx.Text(fmt.Sprintf("answer-%d", turn)) {
					responseID = chunk.Ctrl.StreamID
					responseIDs[turn-1] = responseID
				}
				if responseID != "" && audioStarted[responseID] {
					goto responseStarted
				}
			case <-time.After(time.Second):
				t.Fatalf("turn %d response did not start", turn)
			}
		}
	responseStarted:
	}
	_ = input.Close()
	for got := range results {
		if got.err != nil {
			if !errors.Is(got.err, io.EOF) {
				t.Fatalf("read final output: %v", got.err)
			}
			break
		}
		chunk := got.chunk
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != "assistant" || !chunk.IsEndOfStream() {
			continue
		}
		assistantChunks = append(assistantChunks, chunk.Clone())
		mimeType := "text/plain"
		if value, ok := chunk.MIMEType(); ok {
			mimeType = value
		}
		if responses[chunk.Ctrl.StreamID] == nil {
			responses[chunk.Ctrl.StreamID] = make(map[string]int)
		}
		if responseEOS[chunk.Ctrl.StreamID] == nil {
			responseEOS[chunk.Ctrl.StreamID] = make(map[string]int)
		}
		responseEOS[chunk.Ctrl.StreamID][mimeType]++
		if chunk.Ctrl.Error == "interrupted" {
			responses[chunk.Ctrl.StreamID][mimeType]++
		}
	}
	interruptedResponses := 0
	for streamID, routes := range responses {
		if routes["text/plain"] == 1 && routes["audio/opus"] == 1 &&
			responseEOS[streamID]["text/plain"] == 1 && responseEOS[streamID]["audio/opus"] == 1 {
			interruptedResponses++
		}
	}
	if interruptedResponses != interruptions {
		t.Fatalf("interrupted response routes = %#v, want %d responses with one text and audio EOS", responses, interruptions)
	}
	for turn := range interruptions {
		assertDockHandoffOrder(t, assistantChunks, responseIDs[turn], responseIDs[turn+1])
	}
}

func assertDockHandoffOrder(t *testing.T, chunks []*genx.MessageChunk, previousID, nextID string) {
	t.Helper()
	if previousID == "" || nextID == "" {
		t.Fatalf("response IDs = %q -> %q, want both populated", previousID, nextID)
	}
	lastPreviousEOS := -1
	firstNextBOS := -1
	previousEOS := make(map[string]bool)
	ended := make(map[string]bool)
	for index, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != "assistant" {
			continue
		}
		mimeType, ok := chunk.MIMEType()
		if !ok {
			continue
		}
		if chunk.Ctrl.StreamID == previousID {
			if ended[mimeType] {
				t.Fatalf("%s emitted %s after EOS at index %d: %#v", previousID, mimeType, index, chunks)
			}
			if chunk.IsEndOfStream() {
				if chunk.Ctrl.Error != "interrupted" {
					t.Fatalf("%s %s EOS error = %q, want interrupted", previousID, mimeType, chunk.Ctrl.Error)
				}
				ended[mimeType] = true
				previousEOS[mimeType] = true
				lastPreviousEOS = max(lastPreviousEOS, index)
			}
		}
		if chunk.Ctrl.StreamID == nextID && chunk.IsBeginOfStream() && firstNextBOS < 0 {
			firstNextBOS = index
		}
	}
	if !previousEOS["text/plain"] || !previousEOS["audio/opus"] {
		t.Fatalf("%s interrupted EOS routes = %#v, want text/plain and audio/opus: %#v", previousID, previousEOS, chunks)
	}
	if firstNextBOS < 0 || lastPreviousEOS >= firstNextBOS {
		t.Fatalf("%s last EOS index = %d, %s first BOS index = %d: %#v", previousID, lastPreviousEOS, nextID, firstNextBOS, chunks)
	}
}

func TestDockReplacementBOSInterruptsPulledUndeliveredTerminal(t *testing.T) {
	invocation := streamkit.NewInvocation(t.Context(), streamkit.OutputConfig{InitialCapacity: 8})
	response, err := invocation.StartResponse(streamkit.ResponseConfig{
		StreamID: "assistant-1",
		Role:     genx.RoleModel,
		Name:     "answer",
		Label:    "assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	route := &dockRoute{
		response:  response,
		role:      genx.RoleModel,
		name:      "answer",
		label:     "assistant",
		ttsRoutes: make(map[string]*dockTTSRoute),
		ttsPipes:  make(map[string]*ttsPipe),
	}
	normalEOS := &genx.MessageChunk{
		Role: genx.RoleModel,
		Part: &genx.Blob{MIMEType: "audio/pcm"},
		Ctrl: &genx.StreamCtrl{StreamID: "assistant-1", Label: "assistant", EndOfStream: true},
	}
	mimeType, tracked := route.trackPendingTerminal(normalEOS)
	if err := invocation.EmitTracked(response, normalEOS, func(*genx.MessageChunk) {
		route.clearPendingTerminal(mimeType, tracked)
	}, func(*genx.MessageChunk) {
		route.clearPendingTerminal(mimeType, tracked)
	}); err != nil {
		t.Fatal(err)
	}
	route.closed.Store(true)
	if err := invocation.FinishResponse(response, ""); err != nil {
		t.Fatal(err)
	}
	invocation.Output().DeferOutputObservation()
	pulled, err := invocation.Output().Next()
	if err != nil || pulled.Ctrl == nil || !pulled.IsEndOfStream() {
		t.Fatalf("pulled normal terminal = (%#v, %v)", pulled, err)
	}

	sourceOutput := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 1})
	source, err := streamkit.NewResponseStream(sourceOutput)
	if err != nil {
		t.Fatal(err)
	}
	run := &dockRun{
		invocation:       invocation,
		source:           source,
		routes:           map[string]*dockRoute{"assistant-1": route},
		discardSourceIDs: make(map[string]bool),
	}
	run.beginInputTurn("input-2")
	interrupt, err := invocation.Output().Next()
	if err != nil {
		t.Fatal(err)
	}
	if interrupt.Ctrl == nil || interrupt.Ctrl.StreamID != "assistant-1" ||
		interrupt.Ctrl.Error != "interrupted" || !interrupt.IsEndOfStream() {
		t.Fatalf("replacement terminal = %#v", interrupt)
	}
	if blob, ok := interrupt.Part.(*genx.Blob); !ok || blob.MIMEType != "audio/pcm" {
		t.Fatalf("replacement terminal part = %#v", interrupt.Part)
	}
	invocation.Output().AbandonOutputObservation(pulled)
	invocation.Output().ObserveOutput(interrupt)
	if err := sourceOutput.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDockRepeatedBOSPreservesPulledUndeliveredTextAndAudioTerminals(t *testing.T) {
	invocation := streamkit.NewInvocation(t.Context(), streamkit.OutputConfig{InitialCapacity: 8})
	response, err := invocation.StartResponse(streamkit.ResponseConfig{
		StreamID: "assistant-1",
		Role:     genx.RoleModel,
		Name:     "answer",
		Label:    "assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	route := &dockRoute{
		response:  response,
		role:      genx.RoleModel,
		name:      "answer",
		label:     "assistant",
		ttsRoutes: make(map[string]*dockTTSRoute),
		ttsPipes:  make(map[string]*ttsPipe),
	}
	for _, terminal := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "assistant-1", Label: "assistant", EndOfStream: true}},
		{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "assistant-1", Label: "assistant", EndOfStream: true}},
	} {
		mimeType, tracked := route.trackPendingTerminal(terminal)
		if err := invocation.EmitTracked(response, terminal, func(*genx.MessageChunk) {
			route.clearPendingTerminal(mimeType, tracked)
		}, func(*genx.MessageChunk) {
			route.clearPendingTerminal(mimeType, tracked)
		}); err != nil {
			t.Fatal(err)
		}
	}
	route.closed.Store(true)
	if err := invocation.FinishResponse(response, ""); err != nil {
		t.Fatal(err)
	}
	invocation.Output().DeferOutputObservation()
	var pulled []*genx.MessageChunk
	for range 2 {
		chunk, err := invocation.Output().Next()
		if err != nil {
			t.Fatal(err)
		}
		pulled = append(pulled, chunk)
	}

	sourceOutput := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 1})
	source, err := streamkit.NewResponseStream(sourceOutput)
	if err != nil {
		t.Fatal(err)
	}
	run := &dockRun{
		invocation:       invocation,
		source:           source,
		routes:           map[string]*dockRoute{"assistant-1": route},
		discardSourceIDs: make(map[string]bool),
	}
	run.beginInputTurn("input-2")
	run.beginInputTurn("input-3")

	interrupts := make(map[string]int)
	for range 2 {
		chunk, err := invocation.Output().Next()
		if err != nil {
			t.Fatal(err)
		}
		mimeType, ok := chunk.MIMEType()
		if !ok || chunk.Ctrl == nil || !chunk.IsEndOfStream() || chunk.Ctrl.Error != "interrupted" {
			t.Fatalf("replacement terminal = %#v", chunk)
		}
		interrupts[mimeType]++
		invocation.Output().ObserveOutput(chunk)
	}
	if interrupts["text/plain"] != 1 || interrupts["audio/pcm"] != 1 {
		t.Fatalf("replacement terminals = %#v, want one text and one audio", interrupts)
	}
	for _, chunk := range pulled {
		invocation.Output().AbandonOutputObservation(chunk)
	}
	if err := sourceOutput.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDockForwardModelChunkRacingInterruptDoesNotLeakOrFail(t *testing.T) {
	for iteration := range 100 {
		invocation := streamkit.NewInvocation(t.Context(), streamkit.OutputConfig{InitialCapacity: 8})
		response, err := invocation.StartResponse(streamkit.ResponseConfig{
			StreamID: "assistant-1",
			Role:     genx.RoleModel,
			Name:     "answer",
			Label:    "assistant",
		})
		if err != nil {
			t.Fatalf("iteration %d: StartResponse: %v", iteration, err)
		}
		if err := invocation.Emit(response, &genx.MessageChunk{
			Role: genx.RoleModel, Name: "answer", Part: genx.Text("partial"),
			Ctrl: &genx.StreamCtrl{StreamID: "assistant-1", Label: "assistant", BeginOfStream: true},
		}); err != nil {
			t.Fatalf("iteration %d: emit partial: %v", iteration, err)
		}
		first, err := invocation.Output().Next()
		if err != nil || first == nil || !first.IsBeginOfStream() {
			t.Fatalf("iteration %d: first output = (%#v, %v)", iteration, first, err)
		}

		sourceOutput := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 1})
		source, err := streamkit.NewResponseStream(sourceOutput)
		if err != nil {
			t.Fatalf("iteration %d: source: %v", iteration, err)
		}
		route := &dockRoute{
			response:  response,
			role:      genx.RoleModel,
			name:      "answer",
			label:     "assistant",
			ttsRoutes: make(map[string]*dockTTSRoute),
			ttsPipes:  make(map[string]*ttsPipe),
		}
		run := &dockRun{
			dock:             &Dock{},
			invocation:       invocation,
			source:           source,
			routes:           map[string]*dockRoute{"assistant-1": route},
			discardSourceIDs: make(map[string]bool),
		}
		start := make(chan struct{})
		errs := make(chan error, 1)
		var wg sync.WaitGroup
		wg.Go(func() {
			<-start
			errs <- run.forwardModelChunk(t.Context(), &genx.MessageChunk{
				Role: genx.RoleModel, Name: "answer", Part: genx.Text("racy"),
				Ctrl: &genx.StreamCtrl{StreamID: "assistant-1", Label: "assistant"},
			})
		})
		wg.Go(func() {
			<-start
			run.beginInputTurn("input-2")
		})
		close(start)
		wg.Wait()
		if err := <-errs; err != nil {
			t.Fatalf("iteration %d: forwardModelChunk: %v", iteration, err)
		}
		if err := invocation.Output().Close(); err != nil {
			t.Fatalf("iteration %d: close output: %v", iteration, err)
		}
		chunks := readAll(t, invocation.Output())
		interruptedEOS := 0
		for _, chunk := range chunks {
			if chunk.Part == genx.Text("racy") {
				t.Fatalf("iteration %d: racy output leaked after replacement: %#v", iteration, chunks)
			}
			if chunk.Ctrl != nil && chunk.Ctrl.StreamID == "assistant-1" && chunk.IsEndOfStream() && chunk.Ctrl.Error == "interrupted" {
				interruptedEOS++
			}
		}
		if interruptedEOS != 1 {
			t.Fatalf("iteration %d: interrupted EOS = %d, chunks=%#v", iteration, interruptedEOS, chunks)
		}
		_ = sourceOutput.Close()
	}
}

func TestDockInterruptsPendingTTSBeforeNextTranscript(t *testing.T) {
	invocation := streamkit.NewInvocation(t.Context(), streamkit.OutputConfig{InitialCapacity: 8})
	response, err := invocation.StartResponse(streamkit.ResponseConfig{
		StreamID: "assistant-1",
		Role:     genx.RoleModel,
		Name:     "answer",
		Label:    "assistant",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := invocation.Emit(response, &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("partial")}); err != nil {
		t.Fatal(err)
	}
	pipeCtx, cancelPipe := context.WithCancel(t.Context())
	pipeInput := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 1})
	pipeOutput := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 1})
	route := &dockRoute{
		response: response,
		role:     genx.RoleModel,
		name:     "answer",
		label:    "assistant",
		ttsPipes: map[string]*ttsPipe{
			"answer": {input: pipeInput, output: pipeOutput, cancel: cancelPipe},
		},
	}
	run := &dockRun{
		invocation: invocation,
		routes:     map[string]*dockRoute{"assistant-1": route},
	}
	run.interruptOpenRoutes("interrupted")
	if pipeCtx.Err() == nil {
		t.Fatal("pending TTS was not cancelled")
	}

	transcript, err := invocation.StartResponse(streamkit.ResponseConfig{
		StreamID: "input-2",
		Role:     genx.RoleUser,
		Label:    "transcript",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := invocation.Emit(transcript, &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("new transcript")}); err != nil {
		t.Fatal(err)
	}
	if err := invocation.Output().Close(); err != nil {
		t.Fatal(err)
	}

	chunks := readAll(t, invocation.Output())
	controlEOSIndex, transcriptIndex := -1, -1
	for index, chunk := range chunks {
		if chunk == nil || chunk.Ctrl == nil {
			continue
		}
		if chunk.Ctrl.StreamID == "assistant-1" && chunk.Part == nil &&
			chunk.IsEndOfStream() && chunk.Ctrl.Error == "interrupted" {
			controlEOSIndex = index
		}
		if chunk.Ctrl.StreamID == "input-2" && chunk.Part == genx.Text("new transcript") {
			transcriptIndex = index
		}
	}
	if controlEOSIndex < 0 || transcriptIndex <= controlEOSIndex {
		t.Fatalf("control EOS index = %d, transcript index = %d; chunks=%#v", controlEOSIndex, transcriptIndex, chunks)
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
			_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Name: first.Name, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: first.Ctrl.Label, BeginOfStream: true}})
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
	var textEOS, audioBOS, audio, audioEOS int
	audioBOSIndex, audioDataIndex, textEOSIndex, audioEOSIndex := -1, -1, -1, -1
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
			if chunk.IsBeginOfStream() {
				audioBOS++
				audioBOSIndex = index
			}
			if len(part.Data) > 0 {
				audio++
				audioDataIndex = index
			}
			if chunk.IsEndOfStream() {
				audioEOS++
				audioEOSIndex = index
			}
		}
	}
	if textEOS != 1 || audioBOS != 1 || audio != 1 || audioEOS != 1 {
		t.Fatalf("textEOS/audioBOS/audio/audioEOS = %d/%d/%d/%d; chunks=%#v", textEOS, audioBOS, audio, audioEOS, chunks)
	}
	if audioBOSIndex < 0 || audioDataIndex <= audioBOSIndex || textEOSIndex <= audioDataIndex || audioEOSIndex <= textEOSIndex {
		t.Fatalf("lifecycle indexes BOS/data/textEOS/audioEOS = %d/%d/%d/%d; chunks=%#v", audioBOSIndex, audioDataIndex, textEOSIndex, audioEOSIndex, chunks)
	}
}

func TestDockMergesCompliantTTSLifecyclesByMIME(t *testing.T) {
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "narrator", Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "one", BeginOfStream: true}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "character", Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
		),
		TTS: muxFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 3})
			go func() {
				defer output.Close()
				first, nextErr := input.Next()
				if nextErr != nil {
					return
				}
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: first.Ctrl.StreamID, BeginOfStream: true}})
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte(pattern)}, Ctrl: &genx.StreamCtrl{StreamID: first.Ctrl.StreamID}})
				for {
					chunk, err := input.Next()
					if err != nil || chunk.IsEndOfStream() {
						break
					}
				}
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: first.Ctrl.StreamID, EndOfStream: true}})
			}()
			return output, nil
		}),
		ResolveVoice: func(_ context.Context, request VoiceRequest) (string, error) {
			return request.Name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	var audioBOS, audioData, audioEOS int
	var audio []*genx.MessageChunk
	for _, chunk := range chunks {
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || blob.MIMEType != "audio/opus" {
			continue
		}
		audio = append(audio, chunk)
		if chunk.IsBeginOfStream() {
			audioBOS++
		}
		if len(blob.Data) > 0 {
			audioData++
		}
		if chunk.IsEndOfStream() {
			audioEOS++
		}
	}
	if audioBOS != 1 || audioData != 2 || audioEOS != 1 {
		t.Fatalf("merged audio BOS/data/EOS = %d/%d/%d; chunks=%#v", audioBOS, audioData, audioEOS, chunks)
	}
	if len(audio) != 4 || !audio[0].IsBeginOfStream() || !audio[len(audio)-1].IsEndOfStream() {
		t.Fatalf("merged audio lifecycle = %#v, want BOS/data/data/EOS", audio)
	}
}

func TestDockRejectsInvalidChildTTSLifecycle(t *testing.T) {
	for _, test := range []struct {
		name string
		push func(*streamkit.Output)
		want string
	}{
		{
			name: "data before BOS",
			push: func(output *streamkit.Output) {
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("audio")}})
			},
			want: "before its BOS",
		},
		{
			name: "missing EOS",
			push: func(output *streamkit.Output) {
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{BeginOfStream: true}})
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("audio")}})
			},
			want: "without EOS",
		},
		{
			name: "duplicate BOS",
			push: func(output *streamkit.Output) {
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{BeginOfStream: true}})
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{BeginOfStream: true}})
			},
			want: "duplicate BOS",
		},
		{
			name: "data after EOS",
			push: func(output *streamkit.Output) {
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{BeginOfStream: true}})
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{EndOfStream: true}})
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("late")}})
			},
			want: "after its EOS",
		},
		{
			name: "mismatched StreamID boundaries",
			push: func(output *streamkit.Output) {
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "first", BeginOfStream: true}})
				_ = output.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "second", EndOfStream: true}})
			},
			want: "before its BOS",
		},
		{
			name: "no MIME lifecycle",
			push: func(*streamkit.Output) {},
			want: "without a MIME lifecycle",
		},
		{
			name: "control-only boundary",
			push: func(output *streamkit.Output) {
				_ = output.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{EndOfStream: true}})
			},
			want: "non-MIME chunk",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dock, err := New(Config{
				Agent: fixedAgentOutput(
					&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "one", BeginOfStream: true}},
					&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
				),
				TTS: muxFunc(func(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
					output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
					go func() {
						defer output.Close()
						if _, nextErr := input.Next(); nextErr != nil {
							return
						}
						test.push(output)
					}()
					return output, nil
				}),
				ResolveVoice: fixedVoice("voice"),
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
				if chunk.Ctrl != nil && strings.Contains(chunk.Ctrl.Error, test.want) {
					return
				}
			}
			t.Fatalf("missing lifecycle error %q; chunks=%#v", test.want, chunks)
		})
	}
}

func TestDockComposesSharedTTSLifecycle(t *testing.T) {
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Ctrl: &genx.StreamCtrl{StreamID: "provider", BeginOfStream: true}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "provider"}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "provider", EndOfStream: true}},
		),
		TTS: muxFunc(func(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
			return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{}, "audio/opus", func(_ context.Context, text string, _ streamkit.TTSMeta, _ string, emit func([]byte) error) error {
				return emit([]byte(text))
			}), nil
		}),
		ResolveVoice: fixedVoice("voice"),
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	var audio []*genx.MessageChunk
	for _, chunk := range chunks {
		if blob, ok := chunk.Part.(*genx.Blob); ok && blob.MIMEType == "audio/opus" {
			audio = append(audio, chunk)
		}
	}
	if len(audio) != 3 || !audio[0].IsBeginOfStream() || !ttsChunkHasData(audio[1]) || !audio[2].IsEndOfStream() {
		t.Fatalf("shared TTS audio lifecycle = %#v, want BOS/data/EOS", audio)
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

func TestDockClosesStartedTTSAudioWhenProviderStreamFails(t *testing.T) {
	want := errors.New("doubaospeech: quota exceeded for types: concurrency (code=45000292)")
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
		),
		TTS: muxFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
			return &chunksThenErrorStream{
				chunks: []*genx.MessageChunk{{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: "audio/opus"},
					Ctrl: &genx.StreamCtrl{BeginOfStream: true},
				}},
				err: want,
			}, nil
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
	var audio []*genx.MessageChunk
	for _, chunk := range chunks {
		if mimeType, ok := chunk.MIMEType(); ok && mimeType == "audio/opus" {
			audio = append(audio, chunk)
		}
	}
	if len(audio) != 2 || !audio[0].IsBeginOfStream() || audio[0].IsEndOfStream() ||
		audio[1].IsBeginOfStream() || !audio[1].IsEndOfStream() || audio[1].Ctrl.Error != want.Error() ||
		audio[0].Ctrl.StreamID == "" || audio[1].Ctrl.StreamID != audio[0].Ctrl.StreamID {
		t.Fatalf("failed TTS audio lifecycle = %#v, want BOS/error EOS", audio)
	}
}

func TestDockBoundsTTSCompletionAfterTextEOS(t *testing.T) {
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
		),
		TTS: muxFunc(func(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
			output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
			go func() {
				first, nextErr := input.Next()
				if nextErr != nil || first == nil {
					return
				}
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: "audio/opus"},
					Ctrl: &genx.StreamCtrl{BeginOfStream: true},
				})
				_ = output.Push(&genx.MessageChunk{
					Role: genx.RoleModel,
					Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("partial")},
				})
				<-ctx.Done()
			}()
			return output, nil
		}),
		ResolveVoice:         fixedVoice("voice/narrator"),
		TTSCompletionTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	chunks := readAll(t, output)
	var textEOS, audioEOS bool
	for _, chunk := range chunks {
		if chunk.Ctrl == nil || !chunk.IsEndOfStream() || !strings.Contains(chunk.Ctrl.Error, "TTS completion timeout") {
			continue
		}
		switch chunk.Part.(type) {
		case genx.Text:
			textEOS = true
		case *genx.Blob:
			audioEOS = true
		}
	}
	if !textEOS || !audioEOS {
		t.Fatalf("timeout EOS text/audio = %t/%t; chunks=%#v", textEOS, audioEOS, chunks)
	}
}

func TestDockPreservesTTSTerminalEOSError(t *testing.T) {
	want := errors.New("tts synthesis failed")
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
		),
		TTS: muxFunc(func(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
			output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 3})
			go func() {
				defer output.Close()
				for {
					chunk, err := input.Next()
					if err != nil {
						return
					}
					if !chunk.IsEndOfStream() {
						continue
					}
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: "audio/opus"},
						Ctrl: &genx.StreamCtrl{BeginOfStream: true},
					})
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte("audio")},
					})
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: "audio/opus"},
						Ctrl: &genx.StreamCtrl{EndOfStream: true, Error: want.Error()},
					})
					return
				}
			}()
			return output, nil
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
		blob, ok := chunk.Part.(*genx.Blob)
		if ok && blob.MIMEType == "audio/opus" && chunk.IsEndOfStream() && chunk.Ctrl.Error == want.Error() {
			return
		}
	}
	t.Fatalf("TTS terminal EOS error was not preserved: %#v", chunks)
}

func TestDockMergesErrorOnlyTTSLifecycle(t *testing.T) {
	want := errors.New("tts synthesis failed before audio")
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "answer", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
		),
		TTS: muxFunc(func(_ context.Context, _ string, input genx.Stream) (genx.Stream, error) {
			output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 1})
			go func() {
				defer output.Close()
				for {
					chunk, err := input.Next()
					if err != nil {
						return
					}
					if !chunk.IsEndOfStream() {
						continue
					}
					_ = output.Push(&genx.MessageChunk{
						Role: genx.RoleModel,
						Part: &genx.Blob{MIMEType: "audio/opus"},
						Ctrl: &genx.StreamCtrl{
							StreamID:      "child",
							BeginOfStream: true,
							EndOfStream:   true,
							Error:         want.Error(),
						},
					})
					return
				}
			}()
			return output, nil
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
	var audio []*genx.MessageChunk
	for _, chunk := range chunks {
		if mimeType, ok := chunk.MIMEType(); ok && mimeType == "audio/opus" {
			audio = append(audio, chunk)
		}
	}
	if len(audio) != 2 || !audio[0].IsBeginOfStream() || audio[0].IsEndOfStream() || audio[0].Ctrl.Error != "" ||
		audio[1].IsBeginOfStream() || !audio[1].IsEndOfStream() || audio[1].Ctrl.Error != want.Error() ||
		audio[0].Ctrl.StreamID == "" || audio[1].Ctrl.StreamID != audio[0].Ctrl.StreamID {
		t.Fatalf("error-only audio lifecycle = %#v, want BOS/error EOS", audio)
	}
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
				_ = output.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: chunk.Ctrl.StreamID, BeginOfStream: true}})
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
		wg.Go(func() {
			input := &sliceStream{chunks: []*genx.MessageChunk{{Role: genx.RoleUser, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: text, EndOfStream: true}}}}
			output, err := dock.Transform(t.Context(), input)
			if err != nil {
				t.Errorf("Transform(%s): %v", text, err)
				return
			}
			_ = readAll(t, output)
		})
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if seen["alpha"] != 1 || seen["beta"] != 1 {
		t.Fatalf("voice calls = %#v", seen)
	}
}

func TestDockResolvesVoicePerPublisherNode(t *testing.T) {
	var mu sync.Mutex
	var patterns []string
	tts := muxFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
		mu.Lock()
		patterns = append(patterns, pattern)
		mu.Unlock()
		output := streamkit.NewOutput(streamkit.OutputConfig{InitialCapacity: 2})
		go func() {
			defer output.Close()
			for {
				chunk, err := input.Next()
				if err != nil {
					return
				}
				if chunk.IsEndOfStream() {
					return
				}
			}
		}()
		return output, nil
	})
	dock, err := New(Config{
		Agent: fixedAgentOutput(
			&genx.MessageChunk{Role: genx.RoleModel, Name: "narrator", Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Name: "character", Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "one"}},
			&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "one", EndOfStream: true}},
		),
		TTS: tts,
		ResolveVoice: func(_ context.Context, request VoiceRequest) (string, error) {
			return "voice/" + request.Name, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	_ = readAll(t, output)
	mu.Lock()
	defer mu.Unlock()
	sort.Strings(patterns)
	if !slices.Equal(patterns, []string{"voice/character", "voice/narrator"}) {
		t.Fatalf("TTS patterns = %v", patterns)
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

type chunksThenErrorStream struct {
	chunks []*genx.MessageChunk
	err    error
}

func (s *chunksThenErrorStream) Next() (*genx.MessageChunk, error) {
	if len(s.chunks) == 0 {
		return nil, s.err
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (*chunksThenErrorStream) Close() error               { return nil }
func (*chunksThenErrorStream) CloseWithError(error) error { return nil }

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
