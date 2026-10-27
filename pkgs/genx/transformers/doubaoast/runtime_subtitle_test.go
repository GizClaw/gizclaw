package doubaoast

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	genxast "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/asttranslate"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

type subtitleRuntimeResolver struct{ spec agenthost.Spec }

func (r subtitleRuntimeResolver) Resolve(context.Context, string) (agenthost.Spec, error) {
	return r.spec, nil
}

type subtitleRuntimeMux struct{ transformer genx.Transformer }

func (m subtitleRuntimeMux) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	return m.transformer.Transform(ctx, input)
}

func TestRuntimeReopensTranslationSubtitlesWithinOneSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	provider := newTransformer(doubaospeech.NewClient("test"), withRealtimePacing(false))
	provider.newSession = func(context.Context, doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		return &fakeASTTranslateSession{events: []*doubaospeech.ASTTranslateEvent{
			{Type: doubaospeech.ASTEventSourceSubtitleStart},
			{Type: doubaospeech.ASTEventSourceSubtitleEnd, Text: "你好，再见"},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "Hel"},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "lo"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "Hello"},
			{Type: doubaospeech.ASTEventTranslationSubtitleStart},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "good"},
			{Type: doubaospeech.ASTEventTranslationSubtitleResponse, Text: "bye"},
			{Type: doubaospeech.ASTEventTranslationSubtitleEnd, Text: "goodbye"},
			{Type: doubaospeech.ASTEventSessionFinished},
		}}, nil
	}
	var parameters apitypes.WorkspaceParameters
	if err := parameters.FromASTTranslateWorkspaceParameters(apitypes.ASTTranslateWorkspaceParameters{
		AgentType: apitypes.ASTTranslateWorkspaceParametersAgentTypeAstTranslate,
		Input:     new(apitypes.WorkspaceInputModeRealtime),
	}); err != nil {
		t.Fatal(err)
	}
	host := agenthost.New(subtitleRuntimeResolver{spec: agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "runtime-subtitles", Name: "runtime-subtitles", Parameters: &parameters},
		Workflow:  apitypes.Workflow{Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverAstTranslate}},
		AgentType: string(apitypes.WorkflowDriverAstTranslate),
	}})
	if err := host.Register(string(apitypes.WorkflowDriverAstTranslate), agenthost.FactoryFunc(func(context.Context, agenthost.Spec) (genx.Transformer, error) {
		return genxast.New(genxast.Config{Transformer: subtitleRuntimeMux{transformer: provider}, Model: "translation", Params: map[string]any{"lang_pair": "zh/en"}, PrepareOutput: agenthost.PrepareSpeechOutput})
	})); err != nil {
		t.Fatal(err)
	}
	input := genx.NewRealtimeStream(genx.WithRealtimeStreamDelay(0))
	output, err := host.Transform(ctx, "workspace/runtime-subtitles", input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "input", InputMode: "realtime", BeginOfStream: true}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "input"}},
		{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "input", EndOfStream: true}},
	} {
		if err := input.Push(ctx, chunk); err != nil {
			t.Fatal(err)
		}
	}
	_ = input.Close()
	type route struct {
		begins, ends int
		text         string
	}
	routes := make(map[string]*route)
	for {
		chunk, err := output.Next()
		if err != nil {
			if !agenthost.IsStreamDone(err) {
				t.Fatal(err)
			}
			break
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok || chunk.Role != genx.RoleModel {
			continue
		}
		r := routes[chunk.Ctrl.StreamID]
		if r == nil {
			r = &route{}
			routes[chunk.Ctrl.StreamID] = r
		}
		if chunk.IsBeginOfStream() {
			r.begins++
		}
		if chunk.IsEndOfStream() {
			r.ends++
		}
		r.text += string(text)
	}
	if len(routes) != 2 {
		t.Fatalf("translation response count=%d, want two complete subtitles", len(routes))
	}
	seen := make(map[string]bool)
	for id, r := range routes {
		if r.begins != 1 || r.ends != 1 {
			t.Fatalf("subtitle %s BOS=%d EOS=%d text=%q", id, r.begins, r.ends, r.text)
		}
		seen[strings.TrimSpace(r.text)] = true
	}
	if !seen["Hello"] || !seen["goodbye"] {
		t.Fatalf("subtitle content=%v", seen)
	}
}

func TestTextStatePreservesRepeatedFinalOnlySubtitles(t *testing.T) {
	for _, finals := range [][2]string{
		{"Hello", "Hello"},
		{"こんにちは", "こんにちは"},
		{"안녕하세요", "안녕하세요"},
		{"Hello", "Hello!"},
		{"こんにちは", "こんにちは。"},
		{"안녕하세요", "안녕하세요!"},
	} {
		for _, sameSource := range []bool{false, true} {
			t.Run(finals[1]+"/"+map[bool]string{false: "new-source", true: "same-source"}[sameSource], func(t *testing.T) {
				state := astTranslateTextState{role: genx.RoleModel, label: "assistant", streamID: "one"}
				output := &recordingASTTranslateOutput{}
				for _, final := range finals {
					if err := state.startSubtitle(output); err != nil {
						t.Fatal(err)
					}
					if err := state.addFinal(output, final); err != nil {
						t.Fatal(err)
					}
					if sameSource {
						_ = state.endSubtitle(output)
					} else {
						_ = state.close(output, "")
						state.streamID = "two"
					}
				}
				var texts []string
				for _, chunk := range output.snapshot() {
					if text, ok := chunk.Part.(genx.Text); ok && strings.TrimSpace(string(text)) != "" {
						texts = append(texts, strings.TrimSpace(string(text)))
					}
				}
				if len(texts) != 2 || texts[0] != finals[0] || texts[1] != finals[1] {
					t.Fatalf("final-only subtitles=%v", texts)
				}
			})
		}

	}
}

func TestTextStateHandlesFinalOnlyCumulativeSubtitles(t *testing.T) {
	for _, tc := range []struct {
		name, first, cumulative, suffix, joined string
	}{
		{"latin", "Hello", "Hello world.", "world.", "Hello world. next"},
		{"normalized-prefix", "Hello!", "Hello world.", "world.", "Hello! world. next"},
		{"chinese", "你好", "你好世界", "世界", "你好世界 next"},
		{"japanese", "こんにちは", "こんにちは世界", "世界", "こんにちは世界 next"},
		{"korean", "안녕하세요", "안녕하세요 세계", "세계", "안녕하세요 세계 next"},
	} {
		for _, sameSource := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/same-source=%t", tc.name, sameSource), func(t *testing.T) {
				state := astTranslateTextState{role: genx.RoleModel, label: "assistant", streamID: "one"}
				output := &recordingASTTranslateOutput{}
				for _, final := range []string{tc.first, tc.cumulative, tc.cumulative + " next"} {
					if err := state.startSubtitle(output); err != nil {
						t.Fatal(err)
					}
					if err := state.addFinal(output, final); err != nil {
						t.Fatal(err)
					}
					if sameSource {
						_ = state.endSubtitle(output)
					} else {
						_ = state.close(output, "")
						state.streamID = "two"
					}
				}
				var texts []string
				var joined strings.Builder
				for _, chunk := range output.snapshot() {
					if text, ok := chunk.Part.(genx.Text); ok {
						joined.WriteString(string(text))
						if strings.TrimSpace(string(text)) != "" {
							texts = append(texts, strings.TrimSpace(string(text)))
						}
					}
				}
				if len(texts) != 3 || texts[0] != tc.first || texts[1] != tc.suffix || texts[2] != "next" {
					t.Fatalf("final-only cumulative subtitles=%v, want [%q %q %q]", texts, tc.first, tc.suffix, "next")
				}
				if joined.String() != tc.joined {
					t.Fatalf("joined cumulative subtitles=%q, want %q", joined.String(), tc.joined)
				}
			})
		}
	}
}
