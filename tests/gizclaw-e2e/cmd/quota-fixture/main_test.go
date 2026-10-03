package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
)

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
