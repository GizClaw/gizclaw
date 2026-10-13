package audiodock

import (
	"context"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestUnknownSpeakerConfigPolicy(t *testing.T) {
	agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) { return &sliceStream{}, nil })
	for _, tc := range []struct {
		policy                UnknownSpeakerPolicy
		backpressure, wantErr bool
	}{
		{"", false, false}, {UnknownSpeakerPreserve, false, false}, {UnknownSpeakerDefaultVoice, false, false}, {"bad", false, true}, {UnknownSpeakerDefaultVoice, true, true}, {UnknownSpeakerPreserve, true, false},
	} {
		_, err := New(Config{Agent: agent, UnknownSpeakerPolicy: tc.policy, Backpressure: tc.backpressure})
		if (err != nil) != tc.wantErr {
			t.Fatalf("policy=%q backpressure=%v err=%v", tc.policy, tc.backpressure, err)
		}
	}
}

func TestUnknownSpeakerDockUsesDefaultVoice(t *testing.T) {
	for _, tc := range []struct {
		name, input, wantText, wantAudio string
		voices                           map[string]string
		controlOnlyEOS                   bool
	}{
		{name: "unknown only", input: "【陌生人】你好", wantText: "你好", wantAudio: "default:你好"},
		{name: "known then unknown", input: "【狐】狐狸\n【陌生人】你好", wantText: "狐狸\n你好", wantAudio: "fox:狐狸\ndefault:你好", voices: map[string]string{"狐": "fox"}},
		{name: "unclosed marker on control EOS", input: "【陌", wantText: "【陌", wantAudio: "default:【陌", controlOnlyEOS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
				var chunks []*genx.MessageChunk
				for i := 0; i < len(tc.input); i++ {
					chunks = append(chunks, &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(tc.input[i : i+1]), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: i == 0}})
				}
				terminal := &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}}
				if tc.controlOnlyEOS {
					terminal.Part = nil
				}
				chunks = append(chunks, terminal)
				return &sliceStream{chunks: chunks}, nil
			})
			tts := muxFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
				var spoken strings.Builder
				for {
					chunk, err := input.Next()
					if chunk != nil {
						if text, ok := chunk.Part.(genx.Text); ok {
							spoken.WriteString(string(text))
						}
					}
					if err != nil {
						break
					}
				}
				return &sliceStream{chunks: []*genx.MessageChunk{{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte(pattern + ":" + spoken.String())}, Ctrl: &genx.StreamCtrl{StreamID: "tts", BeginOfStream: true, EndOfStream: true}}}}, nil
			})
			dock, err := New(Config{Agent: agent, TTS: tts, ResolveVoice: fixedVoice("default"), SpeakerVoices: tc.voices, UnknownSpeakerPolicy: UnknownSpeakerDefaultVoice})
			if err != nil {
				t.Fatal(err)
			}
			out, err := dock.Transform(t.Context(), emptyStream{})
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			var text, audio, history strings.Builder
			for _, chunk := range readAll(t, out) {
				switch part := chunk.Part.(type) {
				case genx.Text:
					text.WriteString(string(part))
					if chunk.HistoryText != nil {
						history.WriteString(string(*chunk.HistoryText))
					}
				case *genx.Blob:
					audio.Write(part.Data)
				}
			}
			if text.String() != tc.wantText || audio.String() != tc.wantAudio {
				t.Fatalf("text=%q audio=%q want text=%q audio=%q", text.String(), audio.String(), tc.wantText, tc.wantAudio)
			}
			if history.String() != tc.input {
				t.Fatalf("semantic history=%q want %q", history.String(), tc.input)
			}
		})
	}
}

func TestUnknownSpeakerDockEmptyResolvedVoiceStillDeliversText(t *testing.T) {
	agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		return &sliceStream{chunks: []*genx.MessageChunk{{Role: genx.RoleModel, Part: genx.Text("【陌生人】你好"), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: true, EndOfStream: true}}}}, nil
	})
	var synthesisCalls atomic.Int32
	tts := muxFunc(func(context.Context, string, genx.Stream) (genx.Stream, error) {
		synthesisCalls.Add(1)
		return &sliceStream{}, nil
	})
	dock, err := New(Config{Agent: agent, TTS: tts, ResolveVoice: fixedVoice(""), UnknownSpeakerPolicy: UnknownSpeakerDefaultVoice})
	if err != nil {
		t.Fatal(err)
	}
	out, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var text strings.Builder
	for _, chunk := range readAll(t, out) {
		if body, ok := chunk.Part.(genx.Text); ok {
			text.WriteString(string(body))
		}
	}
	if text.String() != "你好" || synthesisCalls.Load() != 0 {
		t.Fatalf("text=%q synthesis calls=%d", text.String(), synthesisCalls.Load())
	}
}

func TestUnknownSpeakerKeepsPublisherVoiceSelection(t *testing.T) {
	chunk := func(name, text string) *genx.MessageChunk {
		return &genx.MessageChunk{Role: genx.RoleModel, Name: name, Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "reply", MessageID: "message/" + name}}
	}
	for _, tc := range []struct {
		name              string
		chunks            []*genx.MessageChunk
		wantText, history map[string]string
		wantVoices        []string
		terminalName      string
		contentEOS        bool
	}{
		{
			name:       "distinct voices on one response",
			chunks:     []*genx.MessageChunk{chunk("narrator", "【新伙伴】first\n"), chunk("character", "【另一伙伴】second")},
			wantText:   map[string]string{"narrator": "first\n", "character": "second"},
			history:    map[string]string{"narrator": "【新伙伴】first\n", "character": "【另一伙伴】second"},
			wantVoices: []string{"character:second", "narrator:first\n"},
		},
		{
			name:       "interleaved split markers",
			chunks:     []*genx.MessageChunk{chunk("narrator", "【新"), chunk("character", "【另一"), chunk("narrator", "伙伴】first\n"), chunk("character", "伙伴】second")},
			wantText:   map[string]string{"narrator": "first\n", "character": "second"},
			history:    map[string]string{"narrator": "【新伙伴】first\n", "character": "【另一伙伴】second"},
			wantVoices: []string{"character:second", "narrator:first\n"},
		},
		{
			name:       "control EOS flushes owning publisher",
			chunks:     []*genx.MessageChunk{chunk("narrator", "【未"), chunk("character", "【另一伙伴】second"), chunk("narrator", "闭")},
			wantText:   map[string]string{"narrator": "【未闭", "character": "second"},
			history:    map[string]string{"narrator": "【未闭", "character": "【另一伙伴】second"},
			wantVoices: []string{"character:second", "narrator:【未闭"},
		},
		{
			name:         "named control EOS keeps prefix message identity",
			chunks:       []*genx.MessageChunk{chunk("narrator", "【未"), chunk("character", "【另一伙伴】second"), chunk("narrator", "闭")},
			wantText:     map[string]string{"narrator": "【未闭", "character": "second"},
			history:      map[string]string{"narrator": "【未闭", "character": "【另一伙伴】second"},
			wantVoices:   []string{"character:second", "narrator:【未闭"},
			terminalName: "narrator",
		},
		{
			name:       "content EOS completes its own candidate",
			chunks:     []*genx.MessageChunk{chunk("narrator", "【未闭"), chunk("character", "【另一"), chunk("character", "伙伴】second")},
			wantText:   map[string]string{"narrator": "【未闭", "character": "second"},
			history:    map[string]string{"narrator": "【未闭", "character": "【另一伙伴】second"},
			wantVoices: []string{"character:second", "narrator:【未闭"},
			contentEOS: true,
		},
		{
			name:       "marker only publisher has no synthesis or history",
			chunks:     []*genx.MessageChunk{chunk("narrator", "【新伙伴】"), chunk("narrator", ""), chunk("character", "【另一伙伴】second")},
			wantText:   map[string]string{"character": "second"},
			history:    map[string]string{"character": "【另一伙伴】second"},
			wantVoices: []string{"character:second"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.chunks[0].Ctrl.BeginOfStream = true
			if tc.contentEOS {
				tc.chunks[len(tc.chunks)-1].Ctrl.EndOfStream = true
			} else {
				tc.chunks = append(tc.chunks, &genx.MessageChunk{Role: genx.RoleModel, Name: tc.terminalName, Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}})
			}
			model := streamkit.NewOutput(streamkit.OutputConfig{})
			var delivered, abandoned atomic.Int32
			for _, item := range tc.chunks {
				if err := model.PushTracked(item, func(*genx.MessageChunk) { delivered.Add(1) }, func(*genx.MessageChunk) { abandoned.Add(1) }); err != nil {
					t.Fatal(err)
				}
			}
			if err := model.Close(); err != nil {
				t.Fatal(err)
			}
			agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) { return model, nil })
			calls := make(chan string, 3)
			tts := muxFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
				var spoken strings.Builder
				for {
					item, err := input.Next()
					if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
						break
					}
					if err != nil {
						return nil, err
					}
					if item != nil {
						if text, ok := item.Part.(genx.Text); ok {
							spoken.WriteString(string(text))
						}
					}
				}
				calls <- pattern + ":" + spoken.String()
				return &sliceStream{chunks: []*genx.MessageChunk{{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte("audio")}, Ctrl: &genx.StreamCtrl{StreamID: "tts", BeginOfStream: true, EndOfStream: true}}}}, nil
			})
			dock, err := New(Config{Agent: agent, TTS: tts, ResolveVoice: func(_ context.Context, request VoiceRequest) (string, error) { return request.Name, nil }, UnknownSpeakerPolicy: UnknownSpeakerDefaultVoice})
			if err != nil {
				t.Fatal(err)
			}
			out, err := dock.Transform(t.Context(), emptyStream{})
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			texts, histories := make(map[string]string), make(map[string]string)
			for _, item := range readAll(t, out) {
				if item.Ctrl != nil && item.Ctrl.Error != "" {
					t.Fatalf("stream error: %+v", item.Ctrl)
				}
				if text, ok := item.Part.(genx.Text); ok && text != "" {
					if item.Role != genx.RoleModel || item.Ctrl == nil || item.Ctrl.MessageID != "message/"+item.Name {
						t.Fatalf("publisher metadata lost: %+v", item)
					}
					texts[item.Name] += string(text)
					if item.HistoryText != nil {
						histories[item.Name] += string(*item.HistoryText)
					}
				}
			}
			var got []string
			for len(calls) > 0 {
				got = append(got, <-calls)
			}
			slices.Sort(got)
			if !maps.Equal(texts, tc.wantText) || !maps.Equal(histories, tc.history) || !slices.Equal(got, tc.wantVoices) {
				t.Fatalf("texts=%q history=%q voices=%q want texts=%q history=%q voices=%q", texts, histories, got, tc.wantText, tc.history, tc.wantVoices)
			}
			// Source EOF can abandon pending observations before the final pull.
			// A derived prefix flush must not acknowledge an extra producer chunk.
			if delivered.Load()+abandoned.Load() != int32(len(tc.chunks)) {
				t.Fatalf("producer acknowledgements delivered=%d abandoned=%d, want %d original chunks total", delivered.Load(), abandoned.Load(), len(tc.chunks))
			}
		})
	}
}

func TestUnknownSpeakerKeepsInitialPublisherVoiceMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, first                         string
		speakerMap, knownPrefix, controlEOS bool
	}{
		{name: "empty map marker only", first: "【伙伴】"},
		{name: "empty map split marker", first: "【伙"},
		{name: "speaker map marker only", first: "【伙伴】", speakerMap: true},
		{name: "speaker map split marker", first: "【伙", speakerMap: true},
		{name: "known then unknown marker only", first: "【伙伴】", speakerMap: true, knownPrefix: true},
		{name: "known then unknown split marker", first: "【伙", speakerMap: true, knownPrefix: true},
		{name: "empty map unclosed control EOS", first: "【未闭", controlEOS: true},
		{name: "known then unclosed control EOS", first: "【未闭", speakerMap: true, knownPrefix: true, controlEOS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rest := "body"
			if tc.first == "【伙" {
				rest = "伴】body"
			}
			var voices map[string]string
			if tc.speakerMap {
				voices = map[string]string{"known": "voice/known"}
			}
			modelChunks := []*genx.MessageChunk{
				{Role: genx.RoleModel, Name: "narrator", Part: genx.Text(tc.first), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "voice/label", MessageID: "original-message", BeginOfStream: true}},
				{Role: genx.RoleModel, Name: "narrator", Part: genx.Text(rest), Ctrl: &genx.StreamCtrl{StreamID: "reply"}},
				{Role: genx.RoleModel, Name: "narrator", Part: genx.Text(" second"), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}},
			}
			wantText, wantHistory, wantAudio := "body second", "【伙伴】body second", "default:body second"
			if tc.controlEOS {
				modelChunks = []*genx.MessageChunk{modelChunks[0], {Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}}}
				wantText, wantHistory, wantAudio = tc.first, tc.first, "default:"+tc.first
			}
			if tc.knownPrefix {
				modelChunks[0].Ctrl.BeginOfStream = false
				modelChunks = append([]*genx.MessageChunk{{Role: genx.RoleModel, Name: "narrator", Part: genx.Text("【known】known\n"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "voice/label", MessageID: "original-message", BeginOfStream: true}}}, modelChunks...)
				wantText, wantHistory, wantAudio = "known\n"+wantText, "【known】known\n"+wantHistory, "known:known"+wantAudio
			}
			requests := make(chan VoiceRequest, 1)
			tts := muxFunc(func(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
				prefix := "default:"
				if pattern == "voice/known" {
					prefix = "known:"
				} else if pattern != "voice/label" {
					return nil, errors.New("wrong publisher voice")
				}
				return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{}, "audio/opus", func(_ context.Context, text string, _ streamkit.TTSMeta, _ string, emit func([]byte) error) error {
					return emit([]byte(prefix + text))
				}), nil
			})
			dock, err := New(Config{
				Agent: fixedAgentOutput(modelChunks...), TTS: tts, SpeakerVoices: voices,
				ResolveVoice: func(_ context.Context, request VoiceRequest) (string, error) {
					requests <- request
					return request.Label, nil
				},
				UnknownSpeakerPolicy: UnknownSpeakerDefaultVoice,
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := dock.Transform(t.Context(), emptyStream{})
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			var text, history, audio strings.Builder
			for _, item := range readAll(t, out) {
				if item.Ctrl != nil && item.Ctrl.Error != "" {
					t.Fatalf("stream error: %+v", item.Ctrl)
				}
				if part, ok := item.Part.(genx.Text); ok && part != "" {
					if item.Name != "narrator" || item.Ctrl == nil || item.Ctrl.Label != "voice/label" || item.Ctrl.MessageID != "original-message" {
						t.Fatalf("publisher delivery metadata lost: %+v", item)
					}
					text.WriteString(string(part))
					if item.HistoryText != nil {
						history.WriteString(string(*item.HistoryText))
					}
				}
				if part, ok := item.Part.(*genx.Blob); ok {
					audio.Write(part.Data)
				}
			}
			if text.String() != wantText || audio.String() != wantAudio || history.String() != wantHistory {
				t.Fatalf("text=%q audio=%q history=%q want text=%q audio=%q history=%q", text.String(), audio.String(), history.String(), wantText, wantAudio, wantHistory)
			}
			wantRequestText := genx.Text(tc.first)
			if tc.speakerMap && !tc.controlEOS {
				wantRequestText = "body"
			}
			select {
			case request := <-requests:
				if request.Name != "narrator" || request.Label != "voice/label" || request.Chunk == nil || request.Chunk.Part != wantRequestText || request.Chunk.Ctrl == nil || request.Chunk.Ctrl.MessageID != "original-message" {
					t.Fatalf("initial VoiceResolver request lost: %+v", request)
				}
			default:
				t.Fatal("initial publisher voice was not resolved")
			}
		})
	}
}

func TestUnknownSpeakerInterruptDropsPublisherPrefixes(t *testing.T) {
	for _, speakerMap := range []bool{false, true} {
		name := "empty map"
		if speakerMap {
			name = "speaker map"
		}
		t.Run(name, func(t *testing.T) {
			invocation := streamkit.NewInvocation(t.Context(), streamkit.OutputConfig{})
			defer invocation.Cancel(context.Canceled)
			model := streamkit.NewOutput(streamkit.OutputConfig{})
			source, err := streamkit.NewResponseStream(model)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			started, canceled := make(chan struct{}), make(chan struct{})
			tts := muxFunc(func(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
				if pattern == "active" {
					close(started)
					<-ctx.Done()
					close(canceled)
					return nil, ctx.Err()
				}
				return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{}, "audio/opus", func(_ context.Context, text string, _ streamkit.TTSMeta, _ string, emit func([]byte) error) error {
					return emit([]byte(text))
				}), nil
			})
			var voices map[string]string
			initial := []*genx.MessageChunk{
				{Role: genx.RoleModel, Name: "pending", Part: genx.Text("【未闭"), Ctrl: &genx.StreamCtrl{StreamID: "old", Label: "old-pending", MessageID: "old-pending-message", BeginOfStream: true}},
				{Role: genx.RoleModel, Name: "active", Part: genx.Text("【伙伴】spoken"), Ctrl: &genx.StreamCtrl{StreamID: "old", Label: "old-active", MessageID: "old-active-message"}},
			}
			if speakerMap {
				voices = map[string]string{"known": "active"}
				initial[0], initial[1] = initial[1], initial[0]
				initial[0].Part = genx.Text("【known】spoken")
				initial[0].Ctrl.BeginOfStream, initial[1].Ctrl.BeginOfStream = true, false
			}
			dock, err := New(Config{Agent: fixedAgentOutput(), TTS: tts, ResolveVoice: func(_ context.Context, request VoiceRequest) (string, error) { return request.Name, nil }, SpeakerVoices: voices, UnknownSpeakerPolicy: UnknownSpeakerDefaultVoice})
			if err != nil {
				t.Fatal(err)
			}
			run := &dockRun{dock: dock, invocation: invocation, source: source, routes: make(map[string]*dockRoute), discardSourceIDs: make(map[string]bool)}
			for _, item := range initial {
				if err := run.forwardModelChunk(t.Context(), item); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("publisher TTS startup timeout")
			}
			if err := run.beginInputTurn("replacement"); err != nil {
				t.Fatal(err)
			}
			select {
			case <-canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("publisher TTS was not canceled")
			}
			for _, item := range []*genx.MessageChunk{
				{Role: genx.RoleModel, Ctrl: &genx.StreamCtrl{StreamID: "old", EndOfStream: true}},
				{Role: genx.RoleModel, Name: "fresh", Part: genx.Text("【新伙伴】new reply"), Ctrl: &genx.StreamCtrl{StreamID: "new", Label: "fresh-route", MessageID: "fresh-message", BeginOfStream: true, EndOfStream: true}},
			} {
				if err := run.forwardModelChunk(t.Context(), item); err != nil {
					t.Fatal(err)
				}
			}
			run.tts.Wait()
			if err := invocation.Output().Close(); err != nil {
				t.Fatal(err)
			}
			var text, history, audio strings.Builder
			for _, item := range readAll(t, invocation.Output()) {
				if part, ok := item.Part.(genx.Text); ok {
					if part != "" && (item.Name != "fresh" || item.Ctrl == nil || item.Ctrl.Label != "fresh-route" || item.Ctrl.MessageID != "fresh-message") {
						t.Fatalf("old publisher metadata after interrupt: %+v", item)
					}
					text.WriteString(string(part))
				}
				if item.HistoryText != nil {
					history.WriteString(string(*item.HistoryText))
				}
				if part, ok := item.Part.(*genx.Blob); ok {
					audio.Write(part.Data)
				}
			}
			if text.String() != "new reply" || history.String() != "【新伙伴】new reply" || audio.String() != "new reply" {
				t.Fatalf("stale publisher output after interrupt: text=%q history=%q audio=%q", text.String(), history.String(), audio.String())
			}
		})
	}
}
