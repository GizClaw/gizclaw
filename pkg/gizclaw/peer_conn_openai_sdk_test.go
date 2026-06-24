package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"

	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/gizhttp"
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

	serverListener, err := (&giznet.ListenConfig{
		Addr: "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool {
			return true
		}},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen(server) error = %v", err)
	}
	defer serverListener.Close()
	go drainUDP(serverListener.UDP())

	clientListener, err := (&giznet.ListenConfig{
		Addr: "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool {
			return true
		}},
	}).Listen(clientKey)
	if err != nil {
		t.Fatalf("Listen(client) error = %v", err)
	}
	defer clientListener.Close()
	go drainUDP(clientListener.UDP())

	acceptCh := make(chan *giznet.Conn, 1)
	acceptErrCh := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			acceptErrCh <- err
			return
		}
		acceptCh <- conn
	}()

	clientConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer clientConn.Close()

	var serverConn *giznet.Conn
	select {
	case serverConn = <-acceptCh:
	case err := <-acceptErrCh:
		t.Fatalf("Accept error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept timeout")
	}
	defer serverConn.Close()

	var sawChat bool
	var sawSpeech bool
	var sawTranscription bool
	var sawTranscriptionStream bool
	handler := newOpenAIHTTPHandler(&openaiapi.Server{
		Caller: clientKey.Public,
		Models: peerConnModelListerFunc(func(context.Context, adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
			return adminservice.ListModels200JSONResponse(adminservice.ModelList{Items: []apitypes.Model{
				{
					Id: "chat",
					Provider: apitypes.ModelProvider{
						Kind: apitypes.ModelProviderKindOpenaiTenant,
						Name: "test",
					},
				},
				{
					Id:   "asr",
					Kind: apitypes.ModelKindAsr,
					Provider: apitypes.ModelProvider{
						Kind: apitypes.ModelProviderKindVolcTenant,
						Name: "test",
					},
				},
			}}), nil
		}),
		Authorizer: peerConnAuthorizerFunc(func(_ context.Context, req acl.AuthorizeRequest) error {
			if req.Subject.Id != clientKey.Public.String() {
				t.Fatalf("authorize subject = %q, want client public key", req.Subject.Id)
			}
			return nil
		}),
		Generator: openAISDKGeneratorFunc(func(_ context.Context, pattern string, mctx genx.ModelContext) (genx.Stream, error) {
			if pattern != "model/chat" {
				t.Fatalf("generator pattern = %q, want model/chat", pattern)
			}
			sawChat = true
			return openAISDKStream(mctx, &genx.MessageChunk{
				Role: genx.RoleModel,
				Part: genx.Text("sdk chat ok"),
			}), nil
		}),
		Transformer: openAISDKTransformerFunc(func(_ context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
			switch pattern {
			case "voice/voice-a":
				text, err := openAISDKReadText(input)
				if err != nil {
					t.Fatalf("read speech input: %v", err)
				}
				if text != "hello speech" {
					t.Fatalf("speech input = %q, want hello speech", text)
				}
				sawSpeech = true
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
					sawTranscription = true
					return openAISDKStream((&genx.ModelContextBuilder{}).Build(), &genx.MessageChunk{Part: genx.Text("sdk transcription ok")}), nil
				case "sdk streaming audio bytes":
					sawTranscriptionStream = true
					return openAISDKStream((&genx.ModelContextBuilder{}).Build(),
						&genx.MessageChunk{Part: genx.Text("sdk ")},
						&genx.MessageChunk{Part: genx.Text("streaming transcription ok")},
					), nil
				default:
					t.Fatalf("transcription input = %q, want sdk audio bytes", audio)
				}
			default:
				t.Fatalf("transformer pattern = %q, want voice/voice-a or model/asr", pattern)
			}
			return nil, nil
		}),
	})
	server := gizhttp.NewServer(serverConn, ServiceOpenAI, handler)
	defer server.Shutdown(context.Background())
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Serve()
	}()

	httpClient := gizhttp.NewClient(clientConn, ServiceOpenAI)
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
	if !sawChat {
		t.Fatal("chat completion did not reach GenX generator")
	}
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
	if !sawSpeech {
		t.Fatal("speech request did not reach GenX transformer")
	}
	if string(body) != "sdk speech bytes" {
		t.Fatalf("speech body = %q, want sdk speech bytes", body)
	}

	transcription, err := sdk.Audio.Transcriptions.New(ctx, openai.AudioTranscriptionNewParams{
		File:  bytes.NewReader([]byte("sdk audio bytes")),
		Model: openai.AudioModel("asr"),
	})
	requireNoOpenAISDKError(t, err)
	if !sawTranscription {
		t.Fatal("transcription request did not reach GenX transformer")
	}
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
	if !sawTranscriptionStream {
		t.Fatal("streaming transcription request did not reach GenX transformer")
	}
	if transcriptionText != "sdk streaming transcription ok" {
		t.Fatalf("streaming transcription text = %q, want sdk streaming transcription ok", transcriptionText)
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

	serverListener, err := (&giznet.ListenConfig{
		Addr: "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool {
			return true
		}},
	}).Listen(serverKey)
	if err != nil {
		t.Fatalf("Listen(server) error = %v", err)
	}
	defer serverListener.Close()
	go drainUDP(serverListener.UDP())

	clientListener, err := (&giznet.ListenConfig{
		Addr: "127.0.0.1:0",
		SecurityPolicy: testGiznetSecurityPolicy{allowService: func(giznet.PublicKey, uint64) bool {
			return true
		}},
	}).Listen(clientKey)
	if err != nil {
		t.Fatalf("Listen(client) error = %v", err)
	}
	defer clientListener.Close()
	go drainUDP(clientListener.UDP())

	acceptCh := make(chan *giznet.Conn, 1)
	acceptErrCh := make(chan error, 1)
	go func() {
		conn, err := serverListener.Accept()
		if err != nil {
			acceptErrCh <- err
			return
		}
		acceptCh <- conn
	}()

	clientConn, err := clientListener.Dial(serverKey.Public, serverListener.HostInfo().Addr)
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer clientConn.Close()

	var serverConn *giznet.Conn
	select {
	case serverConn = <-acceptCh:
	case err := <-acceptErrCh:
		t.Fatalf("Accept error = %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept timeout")
	}
	defer serverConn.Close()

	releaseSecond := make(chan struct{})
	handler := newOpenAIHTTPHandler(&openaiapi.Server{
		Caller: clientKey.Public,
		Models: peerConnModelListerFunc(func(context.Context, adminservice.ListModelsRequestObject) (adminservice.ListModelsResponseObject, error) {
			return adminservice.ListModels200JSONResponse(adminservice.ModelList{Items: []apitypes.Model{{
				Id: "chat",
				Provider: apitypes.ModelProvider{
					Kind: apitypes.ModelProviderKindOpenaiTenant,
					Name: "test",
				},
			}}}), nil
		}),
		Authorizer: peerConnAuthorizerFunc(func(context.Context, acl.AuthorizeRequest) error { return nil }),
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
	server := gizhttp.NewServer(serverConn, ServiceOpenAI, handler)
	defer server.Shutdown(context.Background())
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Serve()
	}()

	target := &url.URL{Scheme: "http", Host: "gizclaw"}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = gizhttp.NewRoundTripper(clientConn, ServiceOpenAI)
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

func requireNoOpenAISDKError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("openai sdk request failed: %v", err)
	}
}
