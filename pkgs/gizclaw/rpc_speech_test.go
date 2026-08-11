package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
)

func TestNormalizedSpeechLimitsCannotRaiseExtractionWireBounds(t *testing.T) {
	server := rpcServer{speechLimits: SpeechLimits{
		ExtractionMaxSchemaBytes:      rpcSpeechMaxSchemaBytes + 1,
		ExtractionMaxSchemaDepth:      rpcSpeechMaxSchemaDepth + 1,
		ExtractionMaxSchemaProperties: rpcSpeechMaxSchemaProperties + 1,
		ExtractionMaxInstructionBytes: rpcSpeechMaxInstructionBytes + 1,
		ExtractionMaxResultBytes:      rpcSpeechMaxResultBytes + 1,
		ExtractionRequestTimeout:      rpcSpeechExtractTimeout + time.Second,
	}}
	limits := server.normalizedSpeechLimits()
	if limits.ExtractionMaxSchemaBytes != rpcSpeechMaxSchemaBytes ||
		limits.ExtractionMaxSchemaDepth != rpcSpeechMaxSchemaDepth ||
		limits.ExtractionMaxSchemaProperties != rpcSpeechMaxSchemaProperties ||
		limits.ExtractionMaxInstructionBytes != rpcSpeechMaxInstructionBytes ||
		limits.ExtractionMaxResultBytes != rpcSpeechMaxResultBytes ||
		limits.ExtractionRequestTimeout != rpcSpeechExtractTimeout {
		t.Fatalf("normalized extraction wire limits = %+v", limits)
	}
}

func TestRPCSpeechExtractStreamsUploadAndReturnsValidatedResult(t *testing.T) {
	firstAudio := make(chan []byte, 1)
	service := speechServiceFuncs{
		extract: func(_ context.Context, request peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
			if request.ASRModelAlias != "journey.asr" ||
				request.ExtractModelAlias != "journey.extract" ||
				request.Language != "zh-CN" ||
				request.SchemaJSON != `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}` ||
				request.Instruction != "extract the contact" {
				t.Fatalf("extraction request = %+v", request)
			}
			chunk, err := request.Input.Next()
			if err != nil {
				return peergenx.SpeechExtraction{}, err
			}
			blob, ok := chunk.Part.(*genx.Blob)
			if !ok {
				return peergenx.SpeechExtraction{}, errors.New("first input is not audio")
			}
			firstAudio <- append([]byte(nil), blob.Data...)
			for {
				chunk, err = request.Input.Next()
				if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return peergenx.SpeechExtraction{}, err
				}
				if chunk != nil && chunk.IsEndOfStream() {
					break
				}
			}
			return peergenx.SpeechExtraction{
				Transcript: "Alice",
				ResultJSON: `{"name":"Alice"}`,
			}, nil
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "extract", rpcapi.RPCMethodServerSpeechExtract,
		rpcapi.SpeechExtractRequest{
			ASRModelName:     "journey.asr",
			ExtractModelName: "journey.extract",
			ContentType:      "audio/L16;rate=16000;channels=1",
			Language:         new("zh-CN"),
			SchemaJSON:       `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
			Instruction:      new("extract the contact"),
		},
		(*rpcapi.RPCPayload).FromSpeechExtractRequest)
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
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechExtract)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if response.Error != nil {
		t.Fatalf("response error = %+v", response.Error)
	}
	result, err := response.Result.AsSpeechExtractResponse()
	if err != nil || result.Transcript != "Alice" || result.ResultJSON != `{"name":"Alice"}` {
		t.Fatalf("extraction = (%+v, %v)", result, err)
	}
	readSpeechEOS(t, stream)
}

func TestRPCSpeechExtractSplitResponseClosesRequestChannel(t *testing.T) {
	service := speechServiceFuncs{
		extract: func(_ context.Context, request peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
			for {
				chunk, err := request.Input.Next()
				if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return peergenx.SpeechExtraction{}, err
				}
				if chunk != nil && chunk.IsEndOfStream() {
					break
				}
			}
			return peergenx.SpeechExtraction{
				Transcript: "Alice",
				ResultJSON: `{"name":"Alice"}`,
			}, nil
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	params, err := newRPCRequestParams(rpcapi.SpeechExtractRequest{
		ASRModelName:     "asr-main",
		ExtractModelName: "extract-main",
		ContentType:      "audio/L16;rate=16000;channels=1",
		SchemaJSON:       `{"type":"object","properties":{"name":{"type":"string"}}}`,
	}, (*rpcapi.RPCPayload).FromSpeechExtractRequest)
	if err != nil {
		t.Fatalf("newRPCRequestParams() error = %v", err)
	}
	largeID := string(bytes.Repeat([]byte("r"), rpcapi.MaxFrameSize+1024))
	if err := stream.WriteRequestEnvelope(newRPCRequest(largeID, rpcapi.RPCMethodServerSpeechExtract, params)); err != nil {
		t.Fatalf("WriteRequestEnvelope() error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS(request envelope) error = %v", err)
	}
	if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2}}); err != nil {
		t.Fatalf("WriteFrame(audio) error = %v", err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS(audio) error = %v", err)
	}

	response, responseEOS, err := stream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodServerSpeechExtract)
	if err != nil {
		t.Fatalf("ReadResponseEnvelopeForMethod() error = %v", err)
	}
	if !responseEOS {
		t.Fatal("split response did not consume its terminal EOS")
	}
	if response.Id != largeID || response.Error != nil {
		t.Fatalf("response = %+v", response)
	}
	_, err = stream.ReadFrame()
	if err == nil {
		t.Fatal("frame after split response error = nil, want closed channel")
	}
}

func TestRPCSpeechExtractMapsAndSanitizesTerminalErrors(t *testing.T) {
	tests := []struct {
		name    string
		service error
		code    rpcapi.RPCErrorCode
		message string
	}{
		{name: "unknown alias", service: peergenx.ErrNotFound, code: rpcapi.RPCErrorCodeNotFound, message: "speech alias not found"},
		{name: "wrong model kind", service: peergenx.ErrInvalid, code: rpcapi.RPCErrorCodeBadRequest, message: "speech extraction request is not supported"},
		{name: "invalid invocation output", service: peergenx.ErrInvalidOutput, code: rpcapi.RPCErrorCodeInternalError, message: "speech extraction failed"},
		{name: "provider failure", service: errors.New("secret upstream credential"), code: rpcapi.RPCErrorCodeInternalError, message: "speech extraction provider failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := speechServiceFuncs{
				extract: func(context.Context, peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
					return peergenx.SpeechExtraction{}, test.service
				},
			}
			client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
			defer finishSpeechRPCServer(t, client, serverDone)

			stream := newSpeechClientStream(t, client)
			defer stream.Close()
			writeStandardSpeechExtractRequest(t, stream, "terminal-error")
			response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechExtract)
			if err != nil {
				t.Fatalf("ReadResponse() error = %v", err)
			}
			if response.Error == nil ||
				response.Error.Code != test.code ||
				response.Error.Message != test.message {
				t.Fatalf("response = %+v", response)
			}
			if strings.Contains(response.Error.Message, "credential") {
				t.Fatalf("response leaked provider details: %+v", response.Error)
			}
			readSpeechEOS(t, stream)
		})
	}
}

func TestRPCSpeechExtractTimeoutCancelsUploadAndInvocation(t *testing.T) {
	tests := []struct {
		name    string
		extract func(context.Context, peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error)
		upload  bool
	}{
		{
			name: "stalled upload",
			extract: func(_ context.Context, request peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
				_, err := request.Input.Next()
				return peergenx.SpeechExtraction{}, err
			},
		},
		{
			name: "stalled invocation",
			extract: func(ctx context.Context, request peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
				for {
					chunk, err := request.Input.Next()
					if err != nil {
						return peergenx.SpeechExtraction{}, err
					}
					if chunk != nil && chunk.IsEndOfStream() {
						break
					}
				}
				<-ctx.Done()
				return peergenx.SpeechExtraction{}, ctx.Err()
			},
			upload: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, serverDone := startSpeechRPCServer(t, speechServiceFuncs{extract: test.extract}, SpeechLimits{
				ExtractionRequestTimeout: 25 * time.Millisecond,
			})
			defer finishSpeechRPCServer(t, client, serverDone)

			stream := newSpeechClientStream(t, client)
			defer stream.Close()
			writeStandardSpeechExtractRequest(t, stream, "timeout")
			if test.upload {
				if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2}}); err != nil {
					t.Fatalf("WriteFrame() error = %v", err)
				}
				if err := stream.WriteEOS(); err != nil {
					t.Fatalf("WriteEOS() error = %v", err)
				}
			}
			response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechExtract)
			if err != nil {
				t.Fatalf("ReadResponse() error = %v", err)
			}
			if response.Error == nil ||
				response.Error.Code != rpcapi.RPCErrorCodeInternalError ||
				response.Error.Message != "speech extraction timed out" {
				t.Fatalf("response = %+v", response)
			}
			readSpeechEOS(t, stream)
		})
	}
}

func TestRPCSpeechExtractRejectsInputAndOutputLimits(t *testing.T) {
	t.Run("empty audio", func(t *testing.T) {
		service := speechServiceFuncs{
			extract: func(_ context.Context, request peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
				_, err := request.Input.Next()
				return peergenx.SpeechExtraction{}, err
			},
		}
		client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{})
		defer finishSpeechRPCServer(t, client, serverDone)
		stream := newSpeechClientStream(t, client)
		defer stream.Close()
		writeStandardSpeechExtractRequest(t, stream, "empty")
		if err := stream.WriteEOS(); err != nil {
			t.Fatalf("WriteEOS() error = %v", err)
		}
		response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechExtract)
		if err != nil {
			t.Fatalf("ReadResponse() error = %v", err)
		}
		if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeBadRequest {
			t.Fatalf("response = %+v", response)
		}
		readSpeechEOS(t, stream)
	})

	t.Run("oversized audio", func(t *testing.T) {
		service := speechServiceFuncs{
			extract: func(_ context.Context, request peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
				for {
					if _, err := request.Input.Next(); err != nil {
						return peergenx.SpeechExtraction{}, err
					}
				}
			},
		}
		client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{TranscriptionMaxAudioBytes: 2})
		defer finishSpeechRPCServer(t, client, serverDone)
		stream := newSpeechClientStream(t, client)
		defer stream.Close()
		writeStandardSpeechExtractRequest(t, stream, "audio-limit")
		if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2, 3, 4}}); err != nil {
			t.Fatalf("WriteFrame() error = %v", err)
		}
		response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechExtract)
		if err != nil {
			t.Fatalf("ReadResponse() error = %v", err)
		}
		if response.Error == nil || response.Error.Code != rpcapi.RPCErrorCodeBadRequest {
			t.Fatalf("response = %+v", response)
		}
		readSpeechEOS(t, stream)
	})

	t.Run("oversized result", func(t *testing.T) {
		service := speechServiceFuncs{
			extract: func(_ context.Context, request peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
				for {
					chunk, err := request.Input.Next()
					if err != nil {
						return peergenx.SpeechExtraction{}, err
					}
					if chunk != nil && chunk.IsEndOfStream() {
						return peergenx.SpeechExtraction{
							Transcript: "Alice",
							ResultJSON: `{"name":"Alice"}`,
						}, nil
					}
				}
			},
		}
		client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{ExtractionMaxResultBytes: 4})
		defer finishSpeechRPCServer(t, client, serverDone)
		stream := newSpeechClientStream(t, client)
		defer stream.Close()
		writeStandardSpeechExtractRequest(t, stream, "result-limit")
		if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2}}); err != nil {
			t.Fatalf("WriteFrame() error = %v", err)
		}
		if err := stream.WriteEOS(); err != nil {
			t.Fatalf("WriteEOS() error = %v", err)
		}
		response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechExtract)
		if err != nil {
			t.Fatalf("ReadResponse() error = %v", err)
		}
		if response.Error == nil ||
			response.Error.Code != rpcapi.RPCErrorCodeInternalError ||
			response.Error.Message != "speech extraction returned an invalid result" {
			t.Fatalf("response = %+v", response)
		}
		readSpeechEOS(t, stream)
	})
}

func TestValidateSpeechExtractRequestRejectsBoundedMetadata(t *testing.T) {
	tests := []struct {
		name    string
		request rpcapi.SpeechExtractRequest
	}{
		{
			name: "oversized schema",
			request: rpcapi.SpeechExtractRequest{
				ASRModelName: "asr", ExtractModelName: "extract",
				ContentType: "audio/L16;rate=16000;channels=1",
				SchemaJSON:  strings.Repeat("x", rpcSpeechMaxSchemaBytes+1),
			},
		},
		{
			name: "oversized instruction",
			request: rpcapi.SpeechExtractRequest{
				ASRModelName: "asr", ExtractModelName: "extract",
				ContentType: "audio/L16;rate=16000;channels=1",
				SchemaJSON:  `{"type":"object"}`,
				Instruction: new(strings.Repeat("x", rpcSpeechMaxInstructionBytes+1)),
			},
		},
		{
			name: "invalid UTF-8 schema",
			request: rpcapi.SpeechExtractRequest{
				ASRModelName: "asr", ExtractModelName: "extract",
				ContentType: "audio/L16;rate=16000;channels=1",
				SchemaJSON:  string([]byte{0xff}),
			},
		},
		{
			name: "invalid UTF-8 instruction",
			request: rpcapi.SpeechExtractRequest{
				ASRModelName: "asr", ExtractModelName: "extract",
				ContentType: "audio/L16;rate=16000;channels=1",
				SchemaJSON:  `{"type":"object"}`,
				Instruction: new(string([]byte{0xff})),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := validateSpeechExtractRequest(test.request, DefaultSpeechLimits())
			if !errors.Is(err, errSpeechBadRequest) {
				t.Fatalf("validateSpeechExtractRequest() error = %v, want BAD_REQUEST", err)
			}
			if code, _ := speechExtractRPCError(err); code != rpcapi.RPCErrorCodeBadRequest {
				t.Fatalf("speechExtractRPCError() code = %v, want BAD_REQUEST", code)
			}
		})
	}
}

func TestRPCSpeechTranscribeStreamsUploadBeforeEOS(t *testing.T) {
	firstAudio := make(chan []byte, 1)
	service := speechServiceFuncs{
		transcribe: func(_ context.Context, alias, language string, input genx.Stream) (string, error) {
			if alias != "journey.asr" || language != "zh-CN" {
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
		rpcapi.SpeechTranscribeRequest{ModelName: "journey.asr", ContentType: "audio/L16;rate=16000;channels=1", Language: new("zh-CN")},
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

func TestRPCSpeechAcceptsRuntimeAliasGrammar(t *testing.T) {
	t.Parallel()
	for _, alias := range []string{"2fa-asr", "journey.asr"} {
		if _, err := validateSpeechTranscribeRequest(rpcapi.SpeechTranscribeRequest{
			ModelName: alias, ContentType: "audio/L16;rate=16000;channels=1",
		}); err != nil {
			t.Fatalf("validateSpeechTranscribeRequest(%q) error = %v", alias, err)
		}
	}
	if _, err := validateSpeechSynthesizeRequest(rpcapi.SpeechSynthesizeRequest{
		VoiceName: "journey.narrator", Text: "hello", AcceptedContentTypes: []string{"audio/pcm"},
	}, rpcSpeechMaxTextBytes); err != nil {
		t.Fatalf("validateSpeechSynthesizeRequest() error = %v", err)
	}
	if _, _, _, err := validateSpeechExtractRequest(rpcapi.SpeechExtractRequest{
		ASRModelName:     "journey.asr",
		ExtractModelName: "journey.extract",
		ContentType:      "audio/L16;rate=16000;channels=1",
		SchemaJSON:       `{"type":"object"}`,
	}, DefaultSpeechLimits()); err != nil {
		t.Fatalf("validateSpeechExtractRequest() error = %v", err)
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
		rpcapi.SpeechTranscribeRequest{ModelName: "asr-main", ContentType: "audio/L16;rate=16000;channels=1"},
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
		rpcapi.SpeechTranscribeRequest{ModelName: "asr-main", ContentType: "audio/L16;rate=16000;channels=1"},
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
		rpcapi.SpeechTranscribeRequest{ModelName: "asr-main", ContentType: "audio/ogg"},
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
		rpcapi.SpeechTranscribeRequest{ModelName: "asr-main", ContentType: "audio/L16;rate=16000;channels=1"},
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
		rpcapi.SpeechTranscribeRequest{ModelName: "asr-main", ContentType: "audio/L16;rate=16000;channels=1"},
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
		rpcapi.SpeechTranscribeRequest{ModelName: "missing", ContentType: "audio/L16;rate=16000;channels=1"},
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
		rpcapi.SpeechTranscribeRequest{ModelName: "missing", ContentType: "audio/L16;rate=16000;channels=1"},
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
			if alias != "journey.narrator" || text != "hello" {
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
		rpcapi.SpeechSynthesizeRequest{VoiceName: "journey.narrator", Text: "hello", AcceptedContentTypes: []string{"audio/pcm"}},
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

func TestRPCSpeechSynthesizeSplitsOversizedProviderChunk(t *testing.T) {
	audio := bytes.Repeat([]byte{0x5a}, rpcapi.MaxFrameSize+17)
	service := speechServiceFuncs{
		synthesize: func(context.Context, string, string, []string) (peergenx.SpeechSynthesis, error) {
			return peergenx.SpeechSynthesis{
				Stream:      &sliceSpeechStream{chunks: [][]byte{audio}},
				ContentType: "audio/ogg",
			}, nil
		},
	}
	client, serverDone := startSpeechRPCServer(t, service, SpeechLimits{SynthesisMaxOutputBytes: int64(len(audio))})
	defer finishSpeechRPCServer(t, client, serverDone)

	stream := newSpeechClientStream(t, client)
	defer stream.Close()
	writeSpeechRequest(t, stream, "large-chunk", rpcapi.RPCMethodServerSpeechSynthesize,
		rpcapi.SpeechSynthesizeRequest{VoiceName: "narrator", Text: "hello", AcceptedContentTypes: []string{"audio/ogg"}},
		(*rpcapi.RPCPayload).FromSpeechSynthesizeRequest)
	if err := stream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	response, err := stream.ReadResponseForMethod(rpcapi.RPCMethodServerSpeechSynthesize)
	if err != nil || response.Error != nil {
		t.Fatalf("metadata response = (%+v, %v)", response, err)
	}

	first, err := stream.ReadFrame()
	if err != nil || first.Type != rpcapi.FrameTypeBinary || len(first.Payload) != rpcapi.MaxFrameSize {
		t.Fatalf("first audio frame = (type=%v, bytes=%d, err=%v)", first.Type, len(first.Payload), err)
	}
	second, err := stream.ReadFrame()
	if err != nil || second.Type != rpcapi.FrameTypeBinary || len(second.Payload) != 17 {
		t.Fatalf("second audio frame = (type=%v, bytes=%d, err=%v)", second.Type, len(second.Payload), err)
	}
	if got := append(append([]byte(nil), first.Payload...), second.Payload...); !bytes.Equal(got, audio) {
		t.Fatal("reassembled audio differs from provider output")
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
		rpcapi.SpeechSynthesizeRequest{VoiceName: "narrator", Text: "hello", AcceptedContentTypes: []string{"audio/pcm"}},
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
		rpcapi.SpeechSynthesizeRequest{VoiceName: "narrator", Text: "hello", AcceptedContentTypes: []string{"audio/ogg"}},
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
		rpcapi.SpeechSynthesizeRequest{VoiceName: "narrator", Text: "hello", AcceptedContentTypes: []string{"audio/ogg"}},
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
		VoiceName: "narrator",
		Text:      "hello",
		AcceptedContentTypes: []string{
			"audio/pcm",
			"audio/pcm;rate=16000",
		},
	}, rpcSpeechMaxTextBytes)
	if err == nil {
		t.Fatal("validateSpeechSynthesizeRequest() accepted duplicate media types")
	}
	if !errors.Is(err, errSpeechBadRequest) {
		t.Fatalf("validateSpeechSynthesizeRequest() error = %v, want errSpeechBadRequest", err)
	}
	if code, _ := speechRPCError(err); code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("speechRPCError() code = %v, want BAD_REQUEST", code)
	}
}

type speechServiceFuncs struct {
	transcribe func(context.Context, string, string, genx.Stream) (string, error)
	extract    func(context.Context, peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error)
	synthesize func(context.Context, string, string, []string) (peergenx.SpeechSynthesis, error)
}

func (s speechServiceFuncs) Transcribe(ctx context.Context, alias, language string, input genx.Stream) (string, error) {
	if s.transcribe == nil {
		return "", errors.New("unexpected transcription")
	}
	return s.transcribe(ctx, alias, language, input)
}

func (s speechServiceFuncs) Extract(ctx context.Context, request peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error) {
	if s.extract == nil {
		return peergenx.SpeechExtraction{}, errors.New("unexpected extraction")
	}
	return s.extract(ctx, request)
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

func writeStandardSpeechExtractRequest(t *testing.T, stream *rpcStream, id string) {
	t.Helper()
	writeSpeechRequest(t, stream, id, rpcapi.RPCMethodServerSpeechExtract,
		rpcapi.SpeechExtractRequest{
			ASRModelName:     "asr-main",
			ExtractModelName: "extract-main",
			ContentType:      "audio/L16;rate=16000;channels=1",
			SchemaJSON:       `{"type":"object","properties":{"name":{"type":"string"}}}`,
		},
		(*rpcapi.RPCPayload).FromSpeechExtractRequest)
}

func readSpeechEOS(t *testing.T, stream *rpcStream) {
	t.Helper()
	frame, err := stream.ReadFrame()
	if err != nil || frame.Type != rpcapi.FrameTypeEOS {
		t.Fatalf("response EOS = (%+v, %v)", frame, err)
	}
}
