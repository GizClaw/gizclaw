package gizclaw

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
)

func TestPeerConnOpenAIServiceWithOpenAISDK(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}

	clientConn, serverConn := newTestWebRTCConnPair(t, serverKey, clientKey,
		testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool { return true }},
		testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool { return true }})
	defer clientConn.Close()
	defer serverConn.Close()

	chatReached := make(chan struct{}, 1)
	speechReached := make(chan struct{}, 1)
	transcriptionReached := make(chan struct{}, 1)
	transcriptionStreamReached := make(chan struct{}, 1)
	conversationBackend := newOpenAISDKConversationBackend(t)
	handler := newOpenAIHTTPHandler(&openaiapi.Server{
		Caller:     clientKey.Public,
		Workspaces: conversationBackend,
		Executor:   conversationBackend,
		Responses:  openaiapi.NewResponseRuntime(),
		Models: peerConnModelListerFunc(func(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
			return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{Items: []apitypes.Model{
				{
					Id: "chat",
					Provider: apitypes.ModelProvider{
						Kind: apitypes.ModelProviderKindOpenaiTenant,
						Id:   "test",
					},
				},
				{
					Id:   "asr",
					Kind: apitypes.ModelKindAsr,
					Provider: apitypes.ModelProvider{
						Kind: apitypes.ModelProviderKindVolcTenant,
						Id:   "test",
					},
				},
			}}), nil
		}),
		Generator: openAISDKGeneratorFunc(func(_ context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
			if pattern != "model/chat" {
				t.Fatalf("generator pattern = %q, want model/chat", pattern)
			}
			signalOpenAISDK(chatReached)
			return openAISDKStream(mctx, &genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text("sdk chat ok"),
			}), nil
		}),
		Transformer: openAISDKTransformerFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			switch pattern {
			case "voice/voice-a?format=mp3":
				text, err := openAISDKReadText(input)
				if err != nil {
					t.Fatalf("read speech input: %v", err)
				}
				if text != "hello speech" && text != "sdk response result" {
					return nil, fmt.Errorf("speech input = %q", text)
				}
				signalOpenAISDK(speechReached)
				return openAISDKStream((&genx.ModelContextBuilder{}).Build(), &genx.MessageChunk{
					Part: &genx.Blob{MIMEType: "audio/mpeg", Data: []byte("sdk speech bytes")},
				}), nil
			case "model/asr":
				audio, err := openAISDKReadBlob(input)
				if err != nil {
					t.Fatalf("read transcription input: %v", err)
				}
				switch string(audio) {
				case "sdk audio bytes":
					signalOpenAISDK(transcriptionReached)
					return openAISDKStream((&genx.ModelContextBuilder{}).Build(), &genx.MessageChunk{Part: genx.Text("sdk transcription ok")}), nil
				case "sdk streaming audio bytes":
					signalOpenAISDK(transcriptionStreamReached)
					return openAISDKStream((&genx.ModelContextBuilder{}).Build(),
						&genx.MessageChunk{Part: genx.Text("sdk ")},
						&genx.MessageChunk{Part: genx.Text("streaming transcription ok")},
					), nil
				default:
					t.Fatalf("transcription input = %q, want sdk audio bytes", audio)
				}
			default:
				t.Fatalf("transformer pattern = %q, want voice/voice-a?format=mp3 or model/asr", pattern)
			}
			return nil, nil
		}),
	})
	server := gizhttp.NewServer(serverConn, ServicePeerOpenAI, handler)
	defer server.Shutdown(context.Background())
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Serve()
	}()

	httpClient := gizhttp.NewClient(clientConn, ServicePeerOpenAI)
	sdk := openai.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL("http://gizclaw.test/v1"),
		option.WithHTTPClient(httpClient),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models, err := sdk.Models.List(ctx)
	requireNoOpenAISDKError(t, err)
	if len(models.Data) != 2 || models.Data[0].ID != "chat" || models.Data[1].ID != "asr" {
		t.Fatalf("Models.List data = %#v", models.Data)
	}

	completion, err := sdk.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    shared.ChatModel("chat"),
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello chat")},
	})
	requireNoOpenAISDKError(t, err)
	requireOpenAISDKSignal(t, chatReached, "chat completion did not reach GenX generator")
	if len(completion.Choices) != 1 || completion.Choices[0].Message.Content != "sdk chat ok" {
		t.Fatalf("chat completion = %#v", completion)
	}

	speech, err := sdk.Audio.Speech.New(ctx, openai.AudioSpeechNewParams{
		Input:          "hello speech",
		Model:          openai.SpeechModelTTS1,
		Voice:          openai.AudioSpeechNewParamsVoice("voice-a"),
		ResponseFormat: openai.AudioSpeechNewParamsResponseFormatMP3,
	})
	requireNoOpenAISDKError(t, err)
	defer speech.Body.Close()
	body, err := io.ReadAll(speech.Body)
	if err != nil {
		t.Fatalf("read speech body: %v", err)
	}
	requireOpenAISDKSignal(t, speechReached, "speech request did not reach GenX transformer")
	if string(body) != "sdk speech bytes" {
		t.Fatalf("speech body = %q, want sdk speech bytes", body)
	}

	transcription, err := sdk.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  bytes.NewReader([]byte("sdk audio bytes")),
		Model: openai.AudioModel("asr"),
	})
	requireNoOpenAISDKError(t, err)
	requireOpenAISDKSignal(t, transcriptionReached, "transcription request did not reach GenX transformer")
	if transcription.Text != "sdk transcription ok" {
		t.Fatalf("transcription text = %q, want sdk transcription ok", transcription.Text)
	}

	transcriptionStream := sdk.Audio.Transcriptions.NewStreaming(ctx, openai.AudioTranscriptionNewParams{
		File:  bytes.NewReader([]byte("sdk streaming audio bytes")),
		Model: openai.AudioModel("asr"),
	})
	defer transcriptionStream.Close()
	var transcriptionText string
	for transcriptionStream.Next() {
		event := transcriptionStream.Current()
		switch event.Type {
		case "transcript.text.delta":
			transcriptionText += event.Delta
		case "transcript.text.done":
			if event.Text != "sdk streaming transcription ok" {
				t.Fatalf("streaming transcription done text = %q, want sdk streaming transcription ok", event.Text)
			}
		}
	}
	requireNoOpenAISDKError(t, transcriptionStream.Err())
	requireOpenAISDKSignal(t, transcriptionStreamReached, "streaming transcription request did not reach GenX transformer")
	if transcriptionText != "sdk streaming transcription ok" {
		t.Fatalf("streaming transcription text = %q, want sdk streaming transcription ok", transcriptionText)
	}

	var conversation struct {
		ID string `json:"id"`
	}
	requireNoOpenAISDKError(t, sdk.Post(ctx, "conversations", map[string]any{
		"metadata": map[string]string{"collection": "assistants", "workflow_name": "story"},
	}, &conversation))
	if conversation.ID == "" {
		t.Fatal("generic SDK Conversation create returned no ID")
	}
	var responseIDs []string
	for turn := 1; turn <= 3; turn++ {
		input := fmt.Sprintf("sdk turn %d", turn)
		if turn == 2 {
			input = transcription.Text
		}
		var response struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		requireNoOpenAISDKError(t, sdk.Post(ctx, "responses", map[string]any{
			"conversation": conversation.ID, "input": input,
		}, &response))
		if response.ID == "" || response.Status != "completed" {
			t.Fatalf("generic SDK Response %d = %#v", turn, response)
		}
		responseIDs = append(responseIDs, response.ID)
	}
	var conversationItems struct {
		Data []json.RawMessage `json:"data"`
	}
	requireNoOpenAISDKError(t, sdk.Get(ctx, "conversations/"+conversation.ID+"/items", nil, &conversationItems))
	if len(conversationItems.Data) != 6 {
		t.Fatalf("generic SDK Conversation items = %d, want 6", len(conversationItems.Data))
	}
	var firstInputItems struct {
		Data []json.RawMessage `json:"data"`
	}
	requireNoOpenAISDKError(t, sdk.Get(ctx, "responses/"+responseIDs[0]+"/input_items", nil, &firstInputItems))
	if len(firstInputItems.Data) != 1 {
		t.Fatalf("generic SDK first Response input items = %d, want 1", len(firstInputItems.Data))
	}

	composedSpeech, err := sdk.Audio.Speech.New(ctx, openai.AudioSpeechNewParams{
		Input: "sdk response result", Model: openai.SpeechModelTTS1, Voice: "voice-a", ResponseFormat: openai.AudioSpeechNewParamsResponseFormatMP3,
	})
	requireNoOpenAISDKError(t, err)
	composedAudio, err := io.ReadAll(composedSpeech.Body)
	_ = composedSpeech.Body.Close()
	if err != nil || len(composedAudio) == 0 {
		t.Fatalf("composed Response speech bytes=%d err=%v", len(composedAudio), err)
	}

	var background struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	requireNoOpenAISDKError(t, sdk.Post(ctx, "responses", map[string]any{
		"conversation": conversation.ID, "input": "sdk cancel", "background": true,
	}, &background))
	var cancelled struct {
		Status string `json:"status"`
	}
	requireNoOpenAISDKError(t, sdk.Post(ctx, "responses/"+background.ID+"/cancel", nil, &cancelled))
	if cancelled.Status != "cancelled" {
		t.Fatalf("cancelled Response = %#v", cancelled)
	}

	streamBody, err := json.Marshal(map[string]any{"conversation": conversation.ID, "input": "sdk abort", "stream": true})
	if err != nil {
		t.Fatal(err)
	}
	streamCtx, streamCancel := context.WithCancel(ctx)
	streamRequest, err := http.NewRequestWithContext(streamCtx, http.MethodPost, "http://gizclaw/v1/responses", bytes.NewReader(streamBody))
	if err != nil {
		t.Fatal(err)
	}
	streamRequest.Header.Set("Authorization", "Bearer sdk-test")
	streamRequest.Header.Set("Content-Type", "application/json")
	streamResponse, err := httpClient.Do(streamRequest)
	requireNoOpenAISDKError(t, err)
	scanner := bufio.NewScanner(streamResponse.Body)
	sawDelta := false
	for scanner.Scan() {
		if bytes.Contains(scanner.Bytes(), []byte("response.output_text.delta")) {
			sawDelta = true
			break
		}
	}
	streamCancel()
	_ = streamResponse.Body.Close()
	if !sawDelta {
		t.Fatalf("stream ended before delta: %v", scanner.Err())
	}
	deadline := time.Now().Add(time.Second)
	for {
		var recovered struct {
			Status string `json:"status"`
		}
		err = sdk.Post(ctx, "responses", map[string]any{"conversation": conversation.ID, "input": "sdk recovery"}, &recovered)
		if err == nil && recovered.Status == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("same-Conversation recovery = %#v err=%v", recovered, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	_ = clientConn.Close()
	_ = serverConn.Close()
	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("OpenAI gizhttp server error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OpenAI gizhttp server did not stop")
	}
}

func TestPeerConnOpenAIServiceStreamsChatThroughProxy(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}

	clientConn, serverConn := newTestWebRTCConnPair(t, serverKey, clientKey,
		testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool { return true }},
		testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool { return true }})
	defer clientConn.Close()
	defer serverConn.Close()

	releaseSecond := make(chan struct{})
	handler := newOpenAIHTTPHandler(&openaiapi.Server{
		Caller: clientKey.Public,
		Models: peerConnModelListerFunc(func(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
			return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{Items: []apitypes.Model{{
				Id: "chat",
				Provider: apitypes.ModelProvider{
					Kind: apitypes.ModelProviderKindOpenaiTenant,
					Id:   "test",
				},
			}}}), nil
		}),
		Generator: openAISDKGeneratorFunc(func(_ context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
			if pattern != "model/chat" {
				t.Fatalf("generator pattern = %q, want model/chat", pattern)
			}
			sb := genx.NewStreamBuilder(mctx, 2)
			go func() {
				_ = sb.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("first")})
				<-releaseSecond
				_ = sb.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("second")})
				_ = sb.Done(genx.Usage{})
			}()
			return sb.Stream(), nil
		}),
	})
	server := gizhttp.NewServer(serverConn, ServicePeerOpenAI, handler)
	defer server.Shutdown(context.Background())
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Serve()
	}()

	target := &url.URL{Scheme: "http", Host: "gizclaw"}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = gizhttp.NewRoundTripper(clientConn, ServicePeerOpenAI)
	proxyServer := httptest.NewServer(proxy)
	defer proxyServer.Close()
	sdk := openai.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL(proxyServer.URL+"/v1"),
		option.WithHTTPClient(proxyServer.Client()),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream := sdk.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model:    shared.ChatModel("chat"),
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello chat")},
	})
	defer stream.Close()

	first := make(chan string, 1)
	go func() {
		for stream.Next() {
			for _, choice := range stream.Current().Choices {
				if choice.Delta.Content != "" {
					first <- choice.Delta.Content
					return
				}
			}
		}
		first <- ""
	}()

	select {
	case got := <-first:
		if got != "first" {
			t.Fatalf("first stream delta = %q, want first", got)
		}
	case <-time.After(time.Second):
		close(releaseSecond)
		t.Fatal("timed out waiting for first stream delta through proxy")
	}
	close(releaseSecond)
	var rest string
	for stream.Next() {
		for _, choice := range stream.Current().Choices {
			rest += choice.Delta.Content
		}
	}
	requireNoOpenAISDKError(t, stream.Err())
	if rest != "second" {
		t.Fatalf("remaining stream delta = %q, want second", rest)
	}

	_ = clientConn.Close()
	_ = serverConn.Close()
	select {
	case err := <-serverErrCh:
		if err != nil {
			t.Fatalf("OpenAI gizhttp server error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OpenAI gizhttp server did not stop")
	}
}

type openAISDKGeneratorFunc func(context.Context, string, genx.ModelContext) (genx.Stream, error)

func (f openAISDKGeneratorFunc) GenerateStream(ctx context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
	return f(ctx, pattern, mctx)
}

func (f openAISDKGeneratorFunc) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not implemented")
}

type openAISDKTransformerFunc func(context.Context, string, genx.Stream) (genx.Stream, error)

func (f openAISDKTransformerFunc) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	return f(ctx, pattern, input)
}

func openAISDKStream(mctx genx.ModelContext, chunks ...*genx.MessageChunk) genx.Stream {
	sb := genx.NewStreamBuilder(mctx, len(chunks)+1)
	for _, chunk := range chunks {
		_ = sb.Add(chunk)
	}
	_ = sb.Done(genx.Usage{})
	return sb.Stream()
}

func openAISDKReadText(stream genx.Stream) (string, error) {
	defer stream.Close()
	var out string
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				return out, nil
			}
			return "", err
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			out += string(text)
		}
	}
}

func openAISDKReadBlob(stream genx.Stream) ([]byte, error) {
	defer stream.Close()
	var out []byte
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, err
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			out = append(out, blob.Data...)
		}
	}
}

func requireOpenAISDKSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	default:
		t.Fatal(message)
	}
}

func signalOpenAISDK(signal chan<- struct{}) {
	select {
	case signal <- struct{}{}:
	default:
	}
}

func requireNoOpenAISDKError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("openai sdk request failed: %v", err)
	}
}

type openAISDKConversationBackend struct {
	mu       sync.Mutex
	store    workspace.ObjectRuntimeStore
	items    map[string]apitypes.Workspace
	runtimes map[string]workspace.Runtime
	sequence int
}

func newOpenAISDKConversationBackend(t *testing.T) *openAISDKConversationBackend {
	return &openAISDKConversationBackend{
		store: workspace.NewObjectRuntimeStore(newTestObjectStore(t)),
		items: map[string]apitypes.Workspace{}, runtimes: map[string]workspace.Runtime{},
	}
}

func (b *openAISDKConversationBackend) CreateConversationWorkspace(ctx context.Context, request openaiapi.ConversationWorkspaceRequest) (apitypes.Workspace, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sequence++
	id := fmt.Sprintf("sdk-workspace-%d", b.sequence)
	runtime, err := b.store.PrepareWorkspace(ctx, id)
	if err != nil {
		return apitypes.Workspace{}, err
	}
	if err := request.Initialize(ctx, runtime); err != nil {
		return apitypes.Workspace{}, err
	}
	system := false
	now := time.Now()
	labels := map[string]string{"collection": request.Collection, "openai.conversation": "true"}
	item := apitypes.Workspace{Id: id, Name: request.Name, WorkflowId: request.WorkflowName, Labels: &labels, System: &system, CreatedAt: now, UpdatedAt: now, LastActiveAt: now}
	b.items[item.Name] = item
	b.runtimes[item.Id] = runtime
	return item, nil
}

func (b *openAISDKConversationBackend) GetConversationWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	item, ok := b.items[name]
	if !ok {
		return apitypes.Workspace{}, errors.New("not found")
	}
	return item, nil
}

func (b *openAISDKConversationBackend) GetConversationRuntime(_ context.Context, id string) (workspace.Runtime, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	runtime, ok := b.runtimes[id]
	if !ok {
		return workspace.Runtime{}, errors.New("not found")
	}
	return runtime, nil
}

func (b *openAISDKConversationBackend) AppendConversationHistory(ctx context.Context, id string, request workspace.AppendHistoryRequest) (workspace.HistoryEntry, error) {
	runtime, err := b.GetConversationRuntime(ctx, id)
	if err != nil {
		return workspace.HistoryEntry{}, err
	}
	return runtime.History.Append(ctx, request)
}

func (b *openAISDKConversationBackend) ExecuteWorkspaceText(ctx context.Context, item apitypes.Workspace, input string, delta func(string) error) ([]workspace.HistoryEntry, error) {
	if input == "sdk cancel" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if input == "sdk abort" {
		if delta != nil {
			if err := delta("partial"); err != nil {
				return nil, err
			}
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	text := "sdk response:" + input
	if delta != nil {
		if err := delta(text); err != nil {
			return nil, err
		}
	}
	runtime, err := b.GetConversationRuntime(ctx, item.Id)
	if err != nil {
		return nil, err
	}
	entry, err := runtime.History.Append(ctx, workspace.AppendHistoryRequest{Type: "agent", Origin: workspace.HistoryOriginAgentHost, Name: "assistant", Text: text})
	if err != nil {
		return nil, err
	}
	return []workspace.HistoryEntry{entry}, nil
}
