package audiodock

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
)

func TestSpeakerParser(t *testing.T) {
	voices := map[string]string{"旁白": "narrator", "狐": "fox"}
	for _, tc := range []struct{ name, input, want, patterns string }{
		{"split", "前言【旁白】故事【狐】你好", "前言故事你好", "narrator,fox"},
		{"unknown", "【狐】你好【陌生人】原文", "你好【陌生人】原文", "fox,"},
		{"brackets", "正文【注释】原文", "正文【注释】原文", ""},
		{"empty", "【狐】【旁白】", "", ""},
		{"same", "【狐】甲【狐】乙", "甲乙", "fox"},
		{"incomplete", "【狐】甲【旁", "甲【旁", "fox,"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Every byte boundary, including UTF-8 code points split across chunks.
			runes := []byte(tc.input)
			for split := 0; split <= len(runes); split++ {
				p := speakerParser{}
				parts := p.feed(string(runes[:split]), false, voices)
				parts = append(parts, p.feed(string(runes[split:]), true, voices)...)
				var text strings.Builder
				var patterns []string
				for _, part := range parts {
					text.WriteString(part.text)
					if len(patterns) == 0 || patterns[len(patterns)-1] != part.pattern {
						patterns = append(patterns, part.pattern)
					}
				}
				got := strings.Join(patterns, ",")
				if tc.name == "split" {
					got = strings.TrimPrefix(got, ",")
				}
				if text.String() != tc.want || got != tc.patterns {
					t.Fatalf("split %d: text=%q patterns=%q", split, text.String(), got)
				}
			}
		})
	}
}

func TestSpeakerSegmentsPrefetchOrderedOutput(t *testing.T) {
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
		return &sliceStream{chunks: []*genx.MessageChunk{
			{Role: genx.RoleModel, Part: genx.Text("【甲】one"), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: true}},
			{Role: genx.RoleModel, Part: genx.Text(" "), Ctrl: &genx.StreamCtrl{StreamID: "reply"}},
			{Role: genx.RoleModel, Part: genx.Text("more【乙】two"), Ctrl: &genx.StreamCtrl{StreamID: "reply"}},
			{Role: genx.RoleModel, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}},
		}}, nil
	})
	tts := muxFunc(func(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
		if pattern == "second" {
			close(secondStarted)
		}
		out := streamkit.NewOutput(streamkit.OutputConfig{})
		go func() {
			defer out.Close()
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
			if pattern == "first" {
				select {
				case <-firstRelease:
				case <-ctx.Done():
					return
				}
			}
			_ = out.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte(pattern + ":" + spoken.String())}, Ctrl: &genx.StreamCtrl{StreamID: pattern, BeginOfStream: true, EndOfStream: true}})
		}()
		return out, nil
	})
	dock, err := New(Config{Agent: agent, TTS: tts, ResolveVoice: fixedVoice("default"), SpeakerVoices: map[string]string{"甲": "first", "乙": "second"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := dock.Transform(t.Context(), emptyStream{})
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	select {
	case <-secondStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("next segment was not prefetched")
	}
	// The first pull is text even while first TTS is blocked.
	chunk, err := out.Next()
	if err != nil || chunk.Part != genx.Text("one") {
		t.Fatalf("text=%v err=%v", chunk, err)
	}
	close(firstRelease)
	var audio []string
	bos, eos := 0, 0
	for _, chunk := range readAll(t, out) {
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			if len(blob.Data) > 0 {
				audio = append(audio, string(blob.Data))
			}
			if chunk.IsBeginOfStream() {
				bos++
			}
			if chunk.IsEndOfStream() {
				eos++
			}
		}
	}
	if strings.Join(audio, ",") != "first:one more,second:two" || bos != 1 || eos != 1 {
		t.Fatalf("audio=%v BOS=%d EOS=%d", audio, bos, eos)
	}
}

func TestSpeakerSegmentsInterruptCancelsPrefetch(t *testing.T) {
	input := streamkit.NewOutput(streamkit.OutputConfig{})
	model := streamkit.NewOutput(streamkit.OutputConfig{})
	agent := transformerFunc(func(ctx context.Context, source genx.Stream) (genx.Stream, error) {
		go func() {
			for {
				if _, err := source.Next(); err != nil {
					return
				}
			}
		}()
		return model, nil
	})
	started := make(chan string, 3)
	canceled := make(chan string, 3)
	tts := muxFunc(func(ctx context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
		started <- pattern
		<-ctx.Done()
		canceled <- pattern
		return nil, ctx.Err()
	})
	dock, err := New(Config{Agent: agent, TTS: tts, ResolveVoice: fixedVoice("default"), SpeakerVoices: map[string]string{"甲": "first", "乙": "second", "丙": "third"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := dock.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	_ = model.Push(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("【甲】one【乙】two【丙】three"), Ctrl: &genx.StreamCtrl{StreamID: "old", BeginOfStream: true}})
	for range 2 {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("prefetch startup timeout")
		}
	}
	if _, err := out.Next(); err != nil {
		t.Fatal(err)
	}
	_ = input.Push(&genx.MessageChunk{Role: genx.RoleUser, Ctrl: &genx.StreamCtrl{StreamID: "replacement", BeginOfStream: true}})
	for range 2 {
		select {
		case <-canceled:
		case <-time.After(3 * time.Second):
			t.Fatal("interrupt did not cancel current and prefetched TTS")
		}
	}
	select {
	case voice := <-started:
		t.Fatalf("queued voice started after interrupt: %s", voice)
	default:
	}
	_ = model.Close()
	_ = input.Close()
	for _, chunk := range readAll(t, out) {
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			t.Fatal("stale audio after interrupt")
		}
	}
}

func TestSpeakerSegmentsReuseSameVoiceSession(t *testing.T) {
	for _, marker := range []string{"甲", "乙"} {
		t.Run(marker, func(t *testing.T) {
			agent := transformerFunc(func(context.Context, genx.Stream) (genx.Stream, error) {
				return &sliceStream{chunks: []*genx.MessageChunk{
					{Role: genx.RoleModel, Part: genx.Text("【甲】one "), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: true}},
					{Role: genx.RoleModel, Part: genx.Text("【" + marker), Ctrl: &genx.StreamCtrl{StreamID: "reply"}},
					{Role: genx.RoleModel, Part: genx.Text("】two"), Ctrl: &genx.StreamCtrl{StreamID: "reply", EndOfStream: true}},
				}}, nil
			})
			var calls atomic.Int32
			tts := muxFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
				calls.Add(1)
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
				return &sliceStream{chunks: []*genx.MessageChunk{
					{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte(pattern + ":" + spoken.String())}, Ctrl: &genx.StreamCtrl{StreamID: "tts", BeginOfStream: true, EndOfStream: true}},
				}}, nil
			})
			dock, err := New(Config{Agent: agent, TTS: tts, ResolveVoice: fixedVoice("default"), SpeakerVoices: map[string]string{"甲": "shared", "乙": "shared"}})
			if err != nil {
				t.Fatal(err)
			}
			out, err := dock.Transform(t.Context(), emptyStream{})
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			var text, audio strings.Builder
			bos, eos := 0, 0
			for _, chunk := range readAll(t, out) {
				if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
					t.Fatalf("stream error: %s", chunk.Ctrl.Error)
				}
				switch part := chunk.Part.(type) {
				case genx.Text:
					text.WriteString(string(part))
				case *genx.Blob:
					audio.Write(part.Data)
					if chunk.IsBeginOfStream() {
						bos++
					}
					if chunk.IsEndOfStream() {
						eos++
					}
				}
			}
			if calls.Load() != 1 || text.String() != "one two" || audio.String() != "shared:one two" || bos != 1 || eos != 1 {
				t.Fatalf("mux calls=%d text=%q audio=%q BOS=%d EOS=%d", calls.Load(), text.String(), audio.String(), bos, eos)
			}
		})
	}
}
