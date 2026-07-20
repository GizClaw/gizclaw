package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
)

func TestRPCSpeechTranscribeStreamsUploadBeforeEOS(t *testing.T) {
	firstAudio := make(chan []byte, 1)
	service := speechServiceFuncs{
		transcribe: func(_ context.Context, alias, language string, input genx.Stream) (string, error) {
			if alias != "asr-main" || language != "zh-CN" {
				t.Fatalf("transcription metadata = (%q, %q)", alias, language)
			}
			chunk, err := input.Next()
			if err != nil {
				return "", err
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok {
				return "", errors.New("first input is not audio")
			}
			firstAudio <- append([]byte(nil), blob.Data...)
			for {
				chunk, err = input.Next()
				if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
					return "hello", nil
				}
				if err != nil {
					return "", err
				}
				if chunk != nil && chunk.IsEndOfStream() {
					return "hello", nil
				}
			}
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "transcribe", rpcapi.RPCMethodServerSpeechTranscribe,
		rpcapi.SpeechTranscribeRequest{ModelAlias: "asr-main", ContentType: "audio/L16;rate=16000;channels=1", Language: new("zh-CN")},
		(*rpcapi.RPCPayload).FromSpeechTranscribeRequest)
	if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2}}); err != nil {
		t.Fatalf("WriteFrame(first audio) error = %v", err)
	}
	select {
	case got := <-firstAudio:
		if !bytes.Equal(got, []byte{1, 2}) {
			t.Fatalf("first audio = %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("provider did not receive audio before request EOS")
	}
	if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{3, 4}}); err != nil {
		t.Fatalf("WriteFrame(second audio) error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error != nil {
		t.Fatalf("response error = %+v", response.Error)
	}
	result, err := response.Result.AsSpeechTranscribeResponse()
	if err != nil || result.Transcript != "hello" {
		t.Fatalf("transcription = (%+v, %v)", result, err)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechAcceptsLeadingDigitRuntimeAliases(t *testing.T) {
	t.Parallel()
	if _, err := validateSpeechTranscribeRequest(rpcapi.SpeechTranscribeRequest{
		ModelAlias: "2fa-asr", ContentType: "audio/L16;rate=16000;channels=1",
	}); err != nil {
		t.Fatalf("validateSpeechTranscribeRequest() error = %v", err)
	}
	if _, err := validateSpeechSynthesizeRequest(rpcapi.SpeechSynthesizeRequest{
		VoiceAlias: "2fa-voice", Text: "hello", AcceptedContentTypes: []string{"audio/pcm"},
	}, rpcSpeechMaxTextBytes); err != nil {
		t.Fatalf("validateSpeechSynthesizeRequest() error = %v", err)
	}
}

func TestRPCSpeechTranscribeLimitIsBadRequest(t *testing.T) {
	service := speechServiceFuncs{
		transcribe: func(_ context.Context, _, _ string, input genx.Stream) (string, error) {
			for {
				chunk, err := input.Next()
				if err != nil {
					return "", err
				}
				if chunk != nil && chunk.IsEndOfStream() {
					return "unexpected", nil
				}
			}
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{TranscriptionMaxAudioBytes: 2})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "limit", rpcapi.RPCMethodServerSpeechTranscribe,
		rpcapi.SpeechTranscribeRequest{ModelAlias: "asr-main", ContentType: "audio/L16;rate=16000;channels=1"},
		(*rpcapi.RPCPayload).FromSpeechTranscribeRequest)
	if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2, 3, 4}}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	// The server rejects the oversized frame immediately, before request EOS.
	// Read the full-duplex response instead of synchronously writing against it
	// on the unbuffered net.Pipe.
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("response = %+v", response)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechTranscribeRejectsEmptyAudio(t *testing.T) {
	service := speechServiceFuncs{
		transcribe: func(_ context.Context, _, _ string, input genx.Stream) (string, error) {
			_, err := input.Next()
			return "", err
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "empty", rpcapi.RPCMethodServerSpeechTranscribe,
		rpcapi.SpeechTranscribeRequest{ModelAlias: "asr-main", ContentType: "audio/L16;rate=16000;channels=1"},
		(*rpcapi.RPCPayload).FromSpeechTranscribeRequest)
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("response = %+v", response)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechTranscribeRejectsUnsupportedMIME(t *testing.T) {
	client, serverDone := startSpeechRPCServer(t, speechServiceFuncs{}, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "mime", rpcapi.RPCMethodServerSpeechTranscribe,
		rpcapi.SpeechTranscribeRequest{ModelAlias: "asr-main", ContentType: "audio/ogg"},
		(*rpcapi.RPCPayload).FromSpeechTranscribeRequest)
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("response = %+v", response)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechTranscribeSanitizesProviderError(t *testing.T) {
	service := speechServiceFuncs{
		transcribe: func(_ context.Context, _, _ string, input genx.Stream) (string, error) {
			for {
				chunk, err := input.Next()
				if err != nil {
					return "", err
				}
				if chunk != nil && chunk.IsEndOfStream() {
					return "", errors.New("secret upstream failure")
				}
			}
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "provider", rpcapi.RPCMethodServerSpeechTranscribe,
		rpcapi.SpeechTranscribeRequest{ModelAlias: "asr-main", ContentType: "audio/L16;rate=16000;channels=1"},
		(*rpcapi.RPCPayload).FromSpeechTranscribeRequest)
	if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2}}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError || response.Error.Message != "speech provider failed" {
		t.Fatalf("response = %+v", response)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechTranscribeTimeoutInterruptsStalledUpload(t *testing.T) {
	service := speechServiceFuncs{
		transcribe: func(_ context.Context, _, _ string, input genx.Stream) (string, error) {
			_, err := input.Next()
			return "", err
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{TranscriptionRequestTimeout: 25 * time.Millisecond})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "timeout", rpcapi.RPCMethodServerSpeechTranscribe,
		rpcapi.SpeechTranscribeRequest{ModelAlias: "asr-main", ContentType: "audio/L16;rate=16000;channels=1"},
		(*rpcapi.RPCPayload).FromSpeechTranscribeRequest)
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError || response.Error.Message != "speech request timed out" {
		t.Fatalf("response = %+v", response)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechTranscribeEarlyErrorUnblocksBufferedUpload(t *testing.T) {
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	service := speechServiceFuncs{
		transcribe: func(context.Context, string, string, genx.Stream) (string, error) {
			close(providerStarted)
			<-releaseProvider
			return "", errors.New("unknown ASR alias")
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "early-error", rpcapi.RPCMethodServerSpeechTranscribe,
		rpcapi.SpeechTranscribeRequest{ModelAlias: "missing", ContentType: "audio/L16;rate=16000;channels=1"},
		(*rpcapi.RPCPayload).FromSpeechTranscribeRequest)
	<-providerStarted
	if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2}}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	close(releaseProvider)
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError {
		t.Fatalf("response = %+v", response)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechTranscribeEarlyErrorCancelsStalledUpload(t *testing.T) {
	service := speechServiceFuncs{
		transcribe: func(context.Context, string, string, genx.Stream) (string, error) {
			return "", errors.New("unknown ASR alias")
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "stalled-early-error", rpcapi.RPCMethodServerSpeechTranscribe,
		rpcapi.SpeechTranscribeRequest{ModelAlias: "missing", ContentType: "audio/L16;rate=16000;channels=1"},
		(*rpcapi.RPCPayload).FromSpeechTranscribeRequest)

	responseDone := make(chan struct {
		response *rpcapi.RPCResponse
		err      error
	}, 1)
	go func() {
		response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
		responseDone <- struct {
			response *rpcapi.RPCResponse
			err      error
		}{response: response, err: err}
	}()
	select {
	case result := <-responseDone:
		if result.err != nil {
			t.Fatalf("ReadResponse() error = %v", result.err)
		}
		if result.response.Error == nil || result.response.Error.Code != rpcapi.RPCErrorCodeInternalError {
			t.Fatalf("response = %+v", result.response)
		}
	case <-time.After(time.Second):
		t.Fatal("early transcription error waited for request EOS")
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechSynthesizeStreamsAudioBeforeEOS(t *testing.T) {
	release := make(chan struct{})
	service := speechServiceFuncs{
		synthesize: func(_ context.Context, alias, text string, accepted []string) (peergenx.SpeechSynthesis, error) {
			if alias != "narrator" || text != "hello" {
				t.Fatalf("synthesis request = (%q, %q)", alias, text)
			}
			if len(accepted) != 1 || accepted[0] != "audio/pcm" {
				t.Fatalf("accepted content types = %#v", accepted)
			}
			sampleRate, channels := int32(16000), int32(1)
			return peergenx.SpeechSynthesis{
				Stream: &gatedSpeechStream{release: release}, ContentType: "audio/pcm",
				SampleRateHz: &sampleRate, Channels: &channels,
			}, nil
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "synthesize", rpcapi.RPCMethodServerSpeechSynthesize,
		rpcapi.SpeechSynthesizeRequest{VoiceAlias: "narrator", Text: "hello", AcceptedContentTypes: []string{"audio/pcm"}},
		(*rpcapi.RPCPayload).FromSpeechSynthesizeRequest)
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechSynthesize)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error != nil {
		t.Fatalf("response error = %+v", response.Error)
	}
	metadata, err := response.Result.AsSpeechSynthesizeResponse()
	if err != nil || metadata.ContentType != "audio/pcm" {
		t.Fatalf("metadata = (%+v, %v)", metadata, err)
	}
	if metadata.SampleRateHz == nil || *metadata.SampleRateHz != 16000 || metadata.Channels == nil || *metadata.Channels != 1 {
		t.Fatalf("raw audio metadata = %+v", metadata)
	}
	frame, err := stream.ReadFrame()
	if err != nil || frame.Type != rpcapi.FrameTypeBinary || !bytes.Equal(frame.Payload, []byte{1, 2}) {
		t.Fatalf("first audio frame = (%+v, %v)", frame, err)
	}
	close(release)
	frame, err = stream.ReadFrame()
	if err != nil || frame.Type != rpcapi.FrameTypeBinary || !bytes.Equal(frame.Payload, []byte{3, 4}) {
		t.Fatalf("second audio frame = (%+v, %v)", frame, err)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechSynthesizeTimeoutInterruptsMissingEOS(t *testing.T) {
	service := speechServiceFuncs{
		synthesize: func(context.Context, string, string, []string) (peergenx.SpeechSynthesis, error) {
			return peergenx.SpeechSynthesis{}, errors.New("provider must not be called before request EOS")
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{SynthesisRequestTimeout: 25 * time.Millisecond})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "timeout", rpcapi.RPCMethodServerSpeechSynthesize,
		rpcapi.SpeechSynthesizeRequest{VoiceAlias: "narrator", Text: "hello", AcceptedContentTypes: []string{"audio/pcm"}},
		(*rpcapi.RPCPayload).FromSpeechSynthesizeRequest)
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechSynthesize)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeInternalError || response.Error.Message != "speech request timed out" {
		t.Fatalf("response = %+v", response)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechSynthesizeRejectsUnsupportedFormat(t *testing.T) {
	service := speechServiceFuncs{
		synthesize: func(context.Context, string, string, []string) (peergenx.SpeechSynthesis, error) {
			return peergenx.SpeechSynthesis{}, peergenx.ErrUnsupported
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "format", rpcapi.RPCMethodServerSpeechSynthesize,
		rpcapi.SpeechSynthesizeRequest{VoiceAlias: "narrator", Text: "hello", AcceptedContentTypes: []string{"audio/ogg"}},
		(*rpcapi.RPCPayload).FromSpeechSynthesizeRequest)
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechSynthesize)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("response = %+v", response)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechSynthesizeOutputLimitAbortsWithoutSuccessEOS(t *testing.T) {
	service := speechServiceFuncs{
		synthesize: func(context.Context, string, string, []string) (peergenx.SpeechSynthesis, error) {
			return peergenx.SpeechSynthesis{Stream: &sliceSpeechStream{chunks: [][]byte{{1, 2, 3, 4}}}, ContentType: "audio/ogg"}, nil
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{SynthesisMaxOutputBytes: 2})
	defer func() {
		_ = client.Close()
		select {
		case err := <-serverDone:
			if err == nil {
				t.Fatal("server completed truncated synthesis as success")
			}
		case <-time.After(time.Second):
			t.Fatal("speech RPC server did not stop after output limit")
		}
	}()

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "limit", rpcapi.RPCMethodServerSpeechSynthesize,
		rpcapi.SpeechSynthesizeRequest{VoiceAlias: "narrator", Text: "hello", AcceptedContentTypes: []string{"audio/ogg"}},
		(*rpcapi.RPCPayload).FromSpeechSynthesizeRequest)
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechSynthesize)
	if err != nil || response.Error != nil {
		t.Fatalf("metadata response = (%+v, %v)", response, err)
	}
	if _, err := stream.ReadFrame(); err == nil {
		t.Fatal("truncated synthesis returned a normal frame or EOS")
	}
}

func TestValidateSpeechSynthesizeRequestRejectsDuplicateMediaTypes(t *testing.T) {
	_, err := validateSpeechSynthesizeRequest(rpcapi.SpeechSynthesizeRequest{
		VoiceAlias: "narrator",
		Text:       "hello",
		AcceptedContentTypes: []string{
			"audio/pcm",
			"audio/pcm;rate=16000",
		},
	}, rpcSpeechMaxTextBytes)
	if err == nil {
		t.Fatal("validateSpeechSynthesizeRequest() accepted duplicate media types")
	}
}

type speechServiceFuncs struct {
	transcribe func(context.Context, string, string, genx.Stream) (string, error)
	synthesize func(context.Context, string, string, []string) (peergenx.SpeechSynthesis, error)
}

func (s speechServiceFuncs) Transcribe(ctx context.Context, alias, language string, input genx.Stream) (string, error) {
	if s.transcribe == nil {
		return "", errors.New("unexpected transcription")
	}
	return s.transcribe(ctx, alias, language, input)
}

func (s speechServiceFuncs) Synthesize(ctx context.Context, alias, text string, accepted []string) (peergenx.SpeechSynthesis, error) {
	if s.synthesize == nil {
		return peergenx.SpeechSynthesis{}, errors.New("unexpected synthesis")
	}
	return s.synthesize(ctx, alias, text, accepted)
}

func (speechServiceFuncs) Say(context.Context, peergenx.SayRequest) (peergenx.SayResponse, error) {
	return peergenx.SayResponse{}, errors.New("unexpected say request")
}

type gatedSpeechStream struct {
	index   int
	release <-chan struct{}
}

func (s *gatedSpeechStream) Next() (*genx.MessageChunk, error) {
	switch s.index {
	case 0:
		s.index++
		return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}}, nil
	case 1:
		s.index++
		<-s.release
		return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{3, 4}}}, nil
	default:
		return nil, genx.ErrDone
	}
}

func (*gatedSpeechStream) Close() error               { return nil }
func (*gatedSpeechStream) CloseWithError(error) error { return nil }

type sliceSpeechStream struct {
	chunks [][]byte
	index  int
}

func (s *sliceSpeechStream) Next() (*genx.MessageChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, genx.ErrDone
	}
	chunk := s.chunks[s.index]
	s.index++
	return &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg", Data: chunk}}, nil
}

func (*sliceSpeechStream) Close() error               { return nil }
func (*sliceSpeechStream) CloseWithError(error) error { return nil }

func startSpeechRPCServer(t *testing.T, service speechServiceFuncs, limits SpeechLimits) (net.Conn, <-chan error) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- (&rpcServer{serverGenX: service, speechLimits: limits}).Handle(serverSide)
		_ = serverSide.Close()
	}()
	return clientSide, done
}

func finishSpeechRPCServer(t *testing.T, client net.Conn, done <-chan error) {
	t.Helper()
	if err := client.Close(); err != nil {
		t.Fatalf("client Close() error = %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("speech RPC server did not stop")
	}
}

func newSpeechClientStream(t *testing.T, conn net.Conn) *rpcStream {
	t.Helper()
	stream, err := newRPCStream(context.Background(), conn)
	if err != nil {
		t.Fatalf("newRPCStream() error = %v", err)
	}
	return stream
}

func writeSpeechRequest[T any](t *testing.T, stream *rpcStream, id string, method rpcapi.RPCMethod, value T, encode func(*rpcapi.RPCPayload, T) error) {
	t.Helper()
	params, err := newRPCRequestParams(value, encode)
	if err != nil {
		t.Fatalf("newRPCRequestParams() error = %v", err)
	}
	if err := stream.WriteRequest(newRPCRequest(id, method, params)); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
}

func readSpeechEOS(t *testing.T, stream *rpcStream) {
	t.Helper()
	frame, err := stream.ReadFrame()
	if err != nil || frame.Type != rpcapi.FrameTypeEOS {
		t.Fatalf("response EOS = (%+v, %v)", frame, err)
	}
}
