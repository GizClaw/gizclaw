package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/minimax-go"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

func TestInitializeQuotaServerWithoutExternalLogSink(t *testing.T) {
	t.Chdir(filepath.Join("..", "..", "..", ".."))
	dir := t.TempDir()
	if err := initialize(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "server", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, external := range []string{"monitor-tls", "monitor-logs", "log-sink", "fixture-secret"} {
		if strings.Contains(string(data), external) {
			t.Fatalf("quota startup still depends on Monitor logging: %s", external)
		}
	}
	if !strings.Contains(string(data), "peer-usage") || !strings.Contains(string(data), "stderr") {
		t.Fatal("quota configuration lost usage reporting or stderr warnings")
	}
}

func TestSpeechFixtureUsesCurrentHTTPProtocol(t *testing.T) {
	f := &fixture{}
	server := httptest.NewServer(http.HandlerFunc(f.speech))
	defer server.Close()
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Speech.OpenStream(t.Context(), minimax.SpeechStreamRequest{
		Model: "billing-speech", VoiceID: "fixture-voice", Text: "short",
		StreamOptions: &minimax.SpeechStreamOptions{ExcludeAggregatedAudio: true},
		AudioSetting:  &minimax.SpeechAudioSetting{Format: "pcm"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var size int
	var billed int64
	for {
		chunk, err := stream.Next()
		if err != nil {
			t.Fatal(err)
		}
		size += len(chunk.Audio)
		if chunk.UsageCharacters != nil {
			billed = *chunk.UsageCharacters
		}
		if chunk.Done {
			break
		}
	}
	if size != 320 || billed != 37 || f.providerCalls.Load() != 1 {
		t.Fatalf("speech bytes=%d billed=%d calls=%d", size, billed, f.providerCalls.Load())
	}
}

func TestChatFixtureCompletionProtocols(t *testing.T) {
	f := &fixture{}
	server := httptest.NewServer(http.HandlerFunc(f.chat))
	defer server.Close()
	client := openai.NewClient(option.WithBaseURL(server.URL+"/"), option.WithAPIKey("fixture"))
	request := openai.ChatCompletionNewParams{Model: "billing-chat", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("Hello")}}
	completion, err := client.Chat.Completions.New(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(completion.Choices) != 1 || completion.Choices[0].Message.Content != "quota fixture answer" || completion.Usage.TotalTokens != 9 {
		t.Fatalf("unexpected JSON completion: %+v", completion)
	}
	request.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: param.NewOpt(true)}
	stream := client.Chat.Completions.NewStreaming(t.Context(), request)
	defer stream.Close()
	var content string
	var totalTokens int64
	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			content += choice.Delta.Content
		}
		if chunk.Usage.TotalTokens != 0 {
			totalTokens = chunk.Usage.TotalTokens
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if content != completion.Choices[0].Message.Content || totalTokens != completion.Usage.TotalTokens {
		t.Fatalf("SSE completion differs from JSON: content=%q, total tokens=%d", content, totalTokens)
	}
}
