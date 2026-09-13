package eino

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	genxeino "github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/einoconfig"
)

func TestStateVoicesAcrossTurns(t *testing.T) {
	var public apitypes.EinoWorkflowSpec
	err := json.Unmarshal([]byte(`{
      "voice_adapter":{"default_voice":"story.default","node_voices":{"answer":"story.node"},"state_voices":{"field":"speaker","voices":{"fox":"story.fox","bird":"story.bird"}}},
      "graph":{"name":"voices","compile":{"node_trigger_mode":"any_predecessor"},
      "state":{"fields":[{"name":"speaker","type":"string","merge":"replace"},{"name":"answer","type":"string","merge":"replace"}]},
      "nodes":[
        {"id":"select","type":"script","inputs":{"text":{"from":"input.text"}},"outputs":{"speaker":"speaker"},"language":"starlark","entrypoint":"run","source":"def run(input):\n  return {\"speaker\": input[\"text\"]}\n","limits":{"timeout":"100ms","max_execution_steps":1000,"max_input_bytes":4096,"max_output_bytes":4096}},
        {"id":"answer","type":"passthrough","inputs":{"value":{"from":"speaker"}},"outputs":{"value":"answer"}}
      ],"edges":[{"from":"start","to":"select"},{"from":"select","to":"answer"},{"from":"answer","to":"end"}],"branches":[],
      "outputs":[{"node":"answer","field":"answer","name":"assistant","mime_type":"text/plain","primary":true}]}}
    `), &public)
	if err != nil {
		t.Fatal(err)
	}
	if err := einoconfig.Validate(public); err != nil {
		t.Fatal(err)
	}
	graph, err := einoconfig.MapGraph(public.Graph)
	if err != nil {
		t.Fatal(err)
	}
	core, err := genxeino.New(t.Context(), genxeino.Config{
		Agent: genxeino.AgentConfig{ID: "voices"}, Graph: graph,
		OutputMetadata: einoOutputMetadata(public.VoiceAdapter),
	})
	if err != nil {
		t.Fatal(err)
	}
	patterns := make(chan string, 8)
	mux := einoTestMux(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
		patterns <- pattern
		output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
		go func() {
			defer input.Close()
			id := genx.NewStreamID()
			_ = output.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}})
			for {
				_, err := input.Next()
				if err != nil {
					break
				}
			}
			_ = output.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{0, 0}}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}})
			_ = output.Done(genx.Usage{})
		}()
		return output.Stream(), nil
	})
	dock, err := wrapAudio(mux, core, *public.VoiceAdapter, public.Graph.Outputs, apitypes.WorkspaceInputModePushToTalk)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 8)
	output, err := dock.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for _, turn := range []struct{ text, want string }{{"fox", "voice/story.fox"}, {"bird", "voice/story.bird"}, {"unknown", "voice/story.node"}, {"fox", "voice/story.fox"}} {
		id := genx.NewStreamID()
		if err := input.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(turn.text), Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}, &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}}); err != nil {
			t.Fatal(err)
		}
		var delivered strings.Builder
		textEnd, audioEnd := false, false
		for !textEnd || !audioEnd {
			chunk, err := output.Next()
			if err != nil {
				t.Fatal(err)
			}
			if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				t.Fatal(chunk.Ctrl.Error)
			}
			if text, ok := chunk.Part.(genx.Text); ok {
				delivered.WriteString(string(text))
			}
			if chunk.IsEndOfStream() {
				mime, _ := chunk.MIMEType()
				textEnd = textEnd || mime == "text/plain"
				audioEnd = audioEnd || mime == "audio/pcm"
			}
		}

		if delivered.String() != turn.text {
			t.Fatalf("delivered = %q, want %q", delivered.String(), turn.text)
		}
		select {
		case pattern := <-patterns:
			if pattern != turn.want {
				t.Fatalf("turn %q pattern = %q, want %q", turn.text, pattern, turn.want)
			}
		default:
			t.Fatalf("turn %q did not start TTS", turn.text)
		}
		select {
		case extra := <-patterns:
			t.Fatalf("extra TTS: %s", extra)
		default:
		}
	}
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	for {
		_, err := output.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestStateVoiceFallbackAndPrimaryOnly(t *testing.T) {
	var voice apitypes.EinoVoiceAdapter
	if err := json.Unmarshal([]byte(`{"state_voices":{"field":"speaker","voices":{"fox":"story.fox"}}}`), &voice); err != nil {
		t.Fatal(err)
	}
	metadata := einoOutputMetadata(&voice)
	for _, test := range []struct {
		name    string
		state   map[string]any
		primary bool
		want    string
	}{
		{"mapped", map[string]any{"speaker": "fox"}, true, "voice/story.fox"},
		{"missing", nil, true, "voice/story.default"},
		{"unmapped", map[string]any{"speaker": "rabbit"}, true, "voice/story.default"},
		{"wrong type", map[string]any{"speaker": 42}, true, "voice/story.default"},
		{"exact value", map[string]any{"speaker": " fox "}, true, "voice/story.default"},
		{"secondary", map[string]any{"speaker": "fox"}, false, "voice/story.default"},
	} {
		chunk := &genx.MessageChunk{Metadata: metadata(genxeino.OutputDefinition{Primary: test.primary}, test.state)}
		got, err := einoVoiceResolver("story.default", nil, nil)(t.Context(), audiodock.VoiceRequest{Chunk: chunk})
		if err != nil || got != test.want {
			t.Fatalf("%s: %q, %v; want %q", test.name, got, err, test.want)
		}
	}
}
