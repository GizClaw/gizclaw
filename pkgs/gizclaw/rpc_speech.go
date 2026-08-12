package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/runtimeprofile"
)

const (
	rpcSpeechMaxAudioBytes       = 2 * 1024 * 1024
	rpcSpeechMaxAudioDuration    = 60 * time.Second
	rpcSpeechTranscribeTimeout   = 75 * time.Second
	rpcSpeechMaxTranscriptBytes  = 8192
	rpcSpeechMaxSchemaBytes      = 16 * 1024
	rpcSpeechMaxSchemaDepth      = 16
	rpcSpeechMaxSchemaProperties = 128
	rpcSpeechMaxInstructionBytes = 4096
	rpcSpeechMaxResultBytes      = 16 * 1024
	rpcSpeechExtractTimeout      = 120 * time.Second
	rpcSpeechMaxTextBytes        = 4096
	rpcSpeechMaxOutputBytes      = 4 * 1024 * 1024
	rpcSpeechSynthesizeTimeout   = 120 * time.Second
	rpcSpeechMaxAcceptedTypes    = 8
	rpcSpeechMaxContentTypeBytes = 128
	rpcSpeechInputBufferChunks   = 8
)

var errSpeechBadRequest = errors.New("invalid speech request")

type rpcSpeechService interface {
	Transcribe(context.Context, string, string, genx.Stream) (string, error)
	Extract(context.Context, peergenx.SpeechExtractionRequest) (peergenx.SpeechExtraction, error)
	Synthesize(context.Context, string, string, []string) (peergenx.SpeechSynthesis, error)
}

type SpeechLimits struct {
	TranscriptionMaxAudioBytes    int64
	TranscriptionMaxAudioDuration time.Duration
	TranscriptionRequestTimeout   time.Duration
	ExtractionMaxSchemaBytes      int
	ExtractionMaxSchemaDepth      int
	ExtractionMaxSchemaProperties int
	ExtractionMaxInstructionBytes int
	ExtractionMaxResultBytes      int
	ExtractionRequestTimeout      time.Duration
	SynthesisMaxTextBytes         int
	SynthesisMaxOutputBytes       int64
	SynthesisRequestTimeout       time.Duration
}

func DefaultSpeechLimits() SpeechLimits {
	return SpeechLimits{
		TranscriptionMaxAudioBytes:    rpcSpeechMaxAudioBytes,
		TranscriptionMaxAudioDuration: rpcSpeechMaxAudioDuration,
		TranscriptionRequestTimeout:   rpcSpeechTranscribeTimeout,
		ExtractionMaxSchemaBytes:      rpcSpeechMaxSchemaBytes,
		ExtractionMaxSchemaDepth:      rpcSpeechMaxSchemaDepth,
		ExtractionMaxSchemaProperties: rpcSpeechMaxSchemaProperties,
		ExtractionMaxInstructionBytes: rpcSpeechMaxInstructionBytes,
		ExtractionMaxResultBytes:      rpcSpeechMaxResultBytes,
		ExtractionRequestTimeout:      rpcSpeechExtractTimeout,
		SynthesisMaxTextBytes:         rpcSpeechMaxTextBytes,
		SynthesisMaxOutputBytes:       rpcSpeechMaxOutputBytes,
		SynthesisRequestTimeout:       rpcSpeechSynthesizeTimeout,
	}
}

func (s *rpcServer) normalizedSpeechLimits() SpeechLimits {
	limits := s.speechLimits
	defaults := DefaultSpeechLimits()
	if limits.TranscriptionMaxAudioBytes <= 0 {
		limits.TranscriptionMaxAudioBytes = defaults.TranscriptionMaxAudioBytes
	}
	if limits.TranscriptionMaxAudioDuration <= 0 {
		limits.TranscriptionMaxAudioDuration = defaults.TranscriptionMaxAudioDuration
	}
	if limits.TranscriptionRequestTimeout <= 0 {
		limits.TranscriptionRequestTimeout = defaults.TranscriptionRequestTimeout
	}
	if limits.ExtractionMaxSchemaBytes <= 0 ||
		limits.ExtractionMaxSchemaBytes > defaults.ExtractionMaxSchemaBytes {
		limits.ExtractionMaxSchemaBytes = defaults.ExtractionMaxSchemaBytes
	}
	if limits.ExtractionMaxSchemaDepth <= 0 ||
		limits.ExtractionMaxSchemaDepth > defaults.ExtractionMaxSchemaDepth {
		limits.ExtractionMaxSchemaDepth = defaults.ExtractionMaxSchemaDepth
	}
	if limits.ExtractionMaxSchemaProperties <= 0 ||
		limits.ExtractionMaxSchemaProperties > defaults.ExtractionMaxSchemaProperties {
		limits.ExtractionMaxSchemaProperties = defaults.ExtractionMaxSchemaProperties
	}
	if limits.ExtractionMaxInstructionBytes <= 0 ||
		limits.ExtractionMaxInstructionBytes > defaults.ExtractionMaxInstructionBytes {
		limits.ExtractionMaxInstructionBytes = defaults.ExtractionMaxInstructionBytes
	}
	if limits.ExtractionMaxResultBytes <= 0 ||
		limits.ExtractionMaxResultBytes > defaults.ExtractionMaxResultBytes {
		limits.ExtractionMaxResultBytes = defaults.ExtractionMaxResultBytes
	}
	if limits.ExtractionRequestTimeout <= 0 {
		limits.ExtractionRequestTimeout = defaults.ExtractionRequestTimeout
	}
	if limits.ExtractionRequestTimeout > defaults.ExtractionRequestTimeout {
		limits.ExtractionRequestTimeout = defaults.ExtractionRequestTimeout
	}
	if limits.SynthesisMaxTextBytes <= 0 {
		limits.SynthesisMaxTextBytes = defaults.SynthesisMaxTextBytes
	}
	if limits.SynthesisMaxOutputBytes <= 0 {
		limits.SynthesisMaxOutputBytes = defaults.SynthesisMaxOutputBytes
	}
	if limits.SynthesisRequestTimeout <= 0 {
		limits.SynthesisRequestTimeout = defaults.SynthesisRequestTimeout
	}
	return limits
}

func (s *rpcServer) speechService() rpcSpeechService {
	service, _ := s.serverGenX.(rpcSpeechService)
	return service
}

func (s *rpcServer) handleSpeechTranscribe(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsSpeechTranscribeRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	contentType, err := validateSpeechTranscribeRequest(params)
	if err != nil {
		if errors.Is(err, errSpeechBadRequest) {
			code, message := speechRPCError(err)
			return writeRPCErrorResponse(stream, req.Id, code, message)
		}
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, err.Error())
	}
	service := s.speechService()
	if service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "speech service not configured")
	}

	limits := s.normalizedSpeechLimits()
	callCtx, cancel := context.WithTimeout(ctx, limits.TranscriptionRequestTimeout)
	defer cancel()
	callStream, err := newSpeechCallStream(callCtx, stream)
	if err != nil {
		return err
	}
	defer callStream.Close()
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), rpcSpeechInputBufferChunks)
	uploadDone := make(chan error, 1)
	go func() {
		uploadDone <- readSpeechAudio(callStream, builder, contentType, limits)
	}()

	language := ""
	if params.Language != nil {
		language = strings.TrimSpace(*params.Language)
	}
	transcript, callErr := service.Transcribe(callCtx, strings.TrimSpace(params.ModelName), language, builder.Stream())
	if callErr != nil {
		cancel()
		_ = builder.Abort(callErr)
	}
	uploadErr := <-uploadDone
	if callErr == nil {
		callErr = uploadErr
	}
	if callErr != nil {
		if closeErr := callStream.Close(); closeErr != nil {
			return closeErr
		}
		code, message := speechRPCError(callErr)
		return writeRPCErrorResponse(stream, req.Id, code, message)
	}
	if !utf8.ValidString(transcript) {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "speech provider returned invalid transcript")
	}
	if len(transcript) > rpcSpeechMaxTranscriptBytes {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeBadRequest, "speech provider returned an oversized transcript")
	}
	response, err := newRPCResultResponse(req.Id, rpcapi.SpeechTranscribeResponse{Transcript: transcript}, (*rpcapi.RPCPayload).FromSpeechTranscribeResponse)
	if err != nil {
		return err
	}
	metadataEOS, err := callStream.WriteResponseEnvelopeForMethod(req.Method, response)
	if err != nil {
		return err
	}
	if metadataEOS {
		if err := callStream.WriteEOS(); err != nil {
			return err
		}
	}
	return callStream.WriteEOS()
}

func (s *rpcServer) handleSpeechExtract(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if req.Params == nil {
		observability.SetErrorCode(ctx, "SPEECH_EXTRACT_REQUEST_INVALID_INPUT")
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsSpeechExtractRequest()
	if err != nil {
		observability.SetErrorCode(ctx, "SPEECH_EXTRACT_REQUEST_INVALID_INPUT")
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	limits := s.normalizedSpeechLimits()
	contentType, language, instruction, err := validateSpeechExtractRequest(params, limits)
	if err != nil {
		observability.SetErrorCode(ctx, "SPEECH_EXTRACT_REQUEST_INVALID_INPUT")
		if errors.Is(err, errSpeechBadRequest) {
			code, message := speechRPCError(err)
			return writeRPCErrorResponse(stream, req.Id, code, message)
		}
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, err.Error())
	}
	service := s.speechService()
	if service == nil {
		observability.SetErrorCode(ctx, "SPEECH_EXTRACT_REQUEST_NOT_CONFIGURED")
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "speech service not configured")
	}

	callCtx, cancel := context.WithTimeout(ctx, limits.ExtractionRequestTimeout)
	defer cancel()
	callStream, err := newSpeechCallStream(callCtx, stream)
	if err != nil {
		return err
	}
	defer callStream.Close()
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), rpcSpeechInputBufferChunks)
	uploadDone := make(chan error, 1)
	go func() {
		uploadDone <- readSpeechAudio(callStream, builder, contentType, limits)
	}()

	extraction, callErr := service.Extract(callCtx, peergenx.SpeechExtractionRequest{
		ASRModelAlias:       strings.TrimSpace(params.ASRModelName),
		ExtractModelAlias:   strings.TrimSpace(params.ExtractModelName),
		Language:            language,
		SchemaJSON:          params.SchemaJSON,
		Instruction:         instruction,
		Input:               builder.Stream(),
		MaxSchemaDepth:      limits.ExtractionMaxSchemaDepth,
		MaxSchemaProperties: limits.ExtractionMaxSchemaProperties,
		MaxTranscriptBytes:  rpcSpeechMaxTranscriptBytes,
		MaxResultBytes:      limits.ExtractionMaxResultBytes,
	})
	if callErr != nil {
		cancel()
		_ = builder.Abort(callErr)
	}
	uploadErr := <-uploadDone
	if callErr == nil {
		callErr = uploadErr
	}
	if callErr != nil {
		if closeErr := callStream.Close(); closeErr != nil {
			return closeErr
		}
		setSpeechExtractErrorCode(callCtx, callErr)
		code, message := speechExtractRPCError(callErr)
		return writeRPCErrorResponse(stream, req.Id, code, message)
	}
	if !utf8.ValidString(extraction.Transcript) || len(extraction.Transcript) > rpcSpeechMaxTranscriptBytes {
		observability.SetErrorCode(callCtx, "SPEECH_EXTRACT_RESPONSE_INVALID_OUTPUT")
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "speech extraction returned an invalid transcript")
	}
	if !utf8.ValidString(extraction.ResultJSON) || len(extraction.ResultJSON) > limits.ExtractionMaxResultBytes {
		observability.SetErrorCode(callCtx, "SPEECH_EXTRACT_RESPONSE_INVALID_OUTPUT")
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "speech extraction returned an invalid result")
	}
	response, err := newRPCResultResponse(req.Id, rpcapi.SpeechExtractResponse{
		Transcript: extraction.Transcript,
		ResultJSON: extraction.ResultJSON,
	}, (*rpcapi.RPCPayload).FromSpeechExtractResponse)
	if err != nil {
		return err
	}
	metadataEOS, err := callStream.WriteResponseEnvelopeForMethod(req.Method, response)
	if err != nil {
		return err
	}
	if metadataEOS {
		if err := callStream.WriteEOS(); err != nil {
			return err
		}
		return nil
	}
	return callStream.WriteEOS()
}

func setSpeechExtractErrorCode(ctx context.Context, err error) {
	code := peergenx.SpeechExtractionErrorCode(err)
	if code == "" {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			code = "SPEECH_EXTRACT_REQUEST_TIMEOUT"
		case errors.Is(err, context.Canceled):
			code = "SPEECH_EXTRACT_REQUEST_CANCELED"
		default:
			code = "SPEECH_EXTRACT_PROVIDER_FAILURE"
		}
	}
	observability.SetErrorCode(ctx, code)
}

func newSpeechCallStream(ctx context.Context, parent *rpcStream) (*rpcStream, error) {
	stream, err := newRPCStream(ctx, parent.conn)
	if err != nil {
		return nil, err
	}
	stream.requestEOSAlreadyConsumed = parent.requestEOSAlreadyConsumed
	stream.responseObserver = parent.responseObserver
	return stream, nil
}

func readSpeechAudio(stream *rpcStream, builder *genx.StreamBuilder, contentType string, limits SpeechLimits) error {
	var total int64
	for {
		frame, err := stream.ReadFrame()
		if err != nil {
			_ = builder.Abort(err)
			return err
		}
		switch frame.Type {
		case rpcapi.FrameTypeBinary:
			if len(frame.Payload) == 0 {
				continue
			}
			total += int64(len(frame.Payload))
			if total > limits.TranscriptionMaxAudioBytes {
				err = fmt.Errorf("%w: audio exceeds %d bytes", errSpeechBadRequest, limits.TranscriptionMaxAudioBytes)
				_ = builder.Abort(err)
				drainSpeechUpload(stream)
				return err
			}
			if time.Duration(total)*time.Second/(16000*2) > limits.TranscriptionMaxAudioDuration {
				err = fmt.Errorf("%w: audio exceeds %s", errSpeechBadRequest, limits.TranscriptionMaxAudioDuration)
				_ = builder.Abort(err)
				drainSpeechUpload(stream)
				return err
			}
			data := append([]byte(nil), frame.Payload...)
			if err := builder.Add(&genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: contentType, Data: data}}); err != nil {
				drainSpeechUpload(stream)
				return err
			}
		case rpcapi.FrameTypeEOS:
			if total == 0 {
				err = fmt.Errorf("%w: audio is empty", errSpeechBadRequest)
				_ = builder.Abort(err)
				return err
			}
			if total%2 != 0 {
				err = fmt.Errorf("%w: audio must contain complete 16-bit samples", errSpeechBadRequest)
				_ = builder.Abort(err)
				return err
			}
			if err := builder.Add(genx.NewEndOfStream(contentType)); err != nil {
				return err
			}
			return builder.Done(genx.Usage{})
		default:
			err = fmt.Errorf("%w: audio upload accepts only binary frames", errSpeechBadRequest)
			_ = builder.Abort(err)
			drainSpeechUpload(stream)
			return err
		}
	}
}

func drainSpeechUpload(stream *rpcStream) {
	for {
		frame, err := stream.ReadFrame()
		if err != nil || frame.Type == rpcapi.FrameTypeEOS {
			return
		}
	}
}

func (s *rpcServer) handleSpeechSynthesize(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "missing params")
	}
	params, err := req.Params.AsSpeechSynthesizeRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "invalid params")
	}
	limits := s.normalizedSpeechLimits()
	accepted, err := validateSpeechSynthesizeRequest(params, limits.SynthesisMaxTextBytes)
	if err != nil {
		if errors.Is(err, errSpeechBadRequest) {
			code, message := speechRPCError(err)
			return writeRPCErrorResponse(stream, req.Id, code, message)
		}
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, err.Error())
	}
	service := s.speechService()
	if service == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "speech service not configured")
	}

	callCtx, cancel := context.WithTimeout(ctx, limits.SynthesisRequestTimeout)
	defer cancel()
	callStream, err := newSpeechCallStream(callCtx, stream)
	if err != nil {
		return err
	}
	defer callStream.Close()
	if err := callStream.ReadEOS(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			if closeErr := callStream.Close(); closeErr != nil {
				return closeErr
			}
			return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "speech request timed out")
		}
		return err
	}
	synthesis, err := service.Synthesize(callCtx, strings.TrimSpace(params.VoiceName), params.Text, accepted)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			if closeErr := callStream.Close(); closeErr != nil {
				return closeErr
			}
		}
		code, message := speechRPCError(err)
		return writeRPCErrorResponse(stream, req.Id, code, message)
	}
	if synthesis.Stream == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInternalError, "speech provider returned no audio")
	}
	contentType, err := validateSpeechSynthesisMetadata(synthesis, accepted)
	if err != nil {
		code, message := speechRPCError(err)
		return writeRPCErrorResponse(stream, req.Id, code, message)
	}
	output := synthesis.Stream
	defer output.Close()

	first, err := firstSpeechAudioChunk(output, contentType)
	if err != nil {
		code, message := speechRPCError(err)
		return writeRPCErrorResponse(stream, req.Id, code, message)
	}
	response, err := newRPCResultResponse(req.Id, rpcapi.SpeechSynthesizeResponse{
		ContentType: contentType, SampleRateHz: synthesis.SampleRateHz, Channels: synthesis.Channels,
	}, (*rpcapi.RPCPayload).FromSpeechSynthesizeResponse)
	if err != nil {
		return err
	}
	metadataEOS, err := callStream.WriteResponseEnvelopeForMethod(req.Method, response)
	if err != nil {
		return err
	}
	if metadataEOS {
		if err := callStream.WriteEOS(); err != nil {
			return err
		}
	}

	written := 0
	writeChunk := func(chunk *genx.MessageChunk) error {
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || len(blob.Data) == 0 {
			return nil
		}
		mediaType, _, parseErr := mime.ParseMediaType(blob.MIMEType)
		if parseErr != nil || !strings.EqualFold(mediaType, contentType) {
			return errors.New("speech provider changed output content type")
		}
		written += len(blob.Data)
		if int64(written) > limits.SynthesisMaxOutputBytes {
			return fmt.Errorf("synthesized audio exceeds %d bytes", limits.SynthesisMaxOutputBytes)
		}
		for data := blob.Data; len(data) > 0; {
			n := min(len(data), rpcapi.MaxFrameSize)
			if err := callStream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: data[:n]}); err != nil {
				return err
			}
			data = data[n:]
		}
		return nil
	}
	if err := writeChunk(first); err != nil {
		return err
	}
	for {
		chunk, nextErr := output.Next()
		if nextErr != nil {
			if errors.Is(nextErr, genx.ErrDone) || errors.Is(nextErr, io.EOF) {
				break
			}
			return nextErr
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		if err := writeChunk(chunk); err != nil {
			return err
		}
	}
	return callStream.WriteEOS()
}

func firstSpeechAudioChunk(output genx.Stream, expectedContentType string) (*genx.MessageChunk, error) {
	for {
		chunk, err := output.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				return nil, errors.New("speech provider returned empty audio")
			}
			return nil, err
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || len(blob.Data) == 0 {
			continue
		}
		contentType, _, err := mime.ParseMediaType(blob.MIMEType)
		if err != nil {
			return nil, errors.New("speech provider returned invalid content type")
		}
		contentType = strings.ToLower(contentType)
		if contentType != expectedContentType {
			return nil, fmt.Errorf("speech provider content type %q does not match negotiated type %q", contentType, expectedContentType)
		}
		return chunk, nil
	}
}

func validateSpeechSynthesisMetadata(synthesis peergenx.SpeechSynthesis, accepted []string) (string, error) {
	contentType, _, err := mime.ParseMediaType(synthesis.ContentType)
	if err != nil {
		return "", errors.New("speech provider returned invalid content type")
	}
	contentType = strings.ToLower(contentType)
	found := slices.Contains(accepted, contentType)
	if !found {
		return "", fmt.Errorf("%w: synthesized content type %q is not accepted", errSpeechBadRequest, contentType)
	}
	if contentType == "audio/pcm" || contentType == "audio/l16" {
		if synthesis.SampleRateHz == nil || *synthesis.SampleRateHz <= 0 || synthesis.Channels == nil || *synthesis.Channels <= 0 {
			return "", errors.New("speech provider omitted raw audio decoding metadata")
		}
	}
	return contentType, nil
}

func validateSpeechTranscribeRequest(request rpcapi.SpeechTranscribeRequest) (string, error) {
	if !validRuntimeAlias(strings.TrimSpace(request.ModelName)) {
		return "", errors.New("model_name is invalid")
	}
	if len(request.ContentType) == 0 || len(request.ContentType) > rpcSpeechMaxContentTypeBytes {
		return "", errors.New("content_type is invalid")
	}
	mediaType, params, err := mime.ParseMediaType(request.ContentType)
	if err != nil {
		return "", errors.New("content_type is malformed")
	}
	if !strings.EqualFold(mediaType, "audio/L16") || params["rate"] != "16000" || params["channels"] != "1" || len(params) != 2 {
		return "", fmt.Errorf("%w: content_type must be audio/L16;rate=16000;channels=1", errSpeechBadRequest)
	}
	if request.Language != nil && len(*request.Language) > 32 {
		return "", errors.New("language exceeds 32 bytes")
	}
	return "audio/L16;rate=16000;channels=1", nil
}

func validateSpeechExtractRequest(request rpcapi.SpeechExtractRequest, limits SpeechLimits) (contentType, language, instruction string, err error) {
	contentType, err = validateSpeechTranscribeRequest(rpcapi.SpeechTranscribeRequest{
		ModelName:   request.ASRModelName,
		ContentType: request.ContentType,
		Language:    request.Language,
	})
	if err != nil {
		return "", "", "", err
	}
	if !validRuntimeAlias(strings.TrimSpace(request.ExtractModelName)) {
		return "", "", "", errors.New("extract_model_name is invalid")
	}
	if !utf8.ValidString(request.SchemaJSON) {
		return "", "", "", fmt.Errorf("%w: schema_json must be valid UTF-8", errSpeechBadRequest)
	}
	if strings.TrimSpace(request.SchemaJSON) == "" || len(request.SchemaJSON) > limits.ExtractionMaxSchemaBytes {
		return "", "", "", fmt.Errorf("%w: schema_json must contain 1 to %d UTF-8 bytes", errSpeechBadRequest, limits.ExtractionMaxSchemaBytes)
	}
	if request.Instruction != nil {
		if !utf8.ValidString(*request.Instruction) {
			return "", "", "", fmt.Errorf("%w: instruction must be valid UTF-8", errSpeechBadRequest)
		}
		if len(*request.Instruction) > limits.ExtractionMaxInstructionBytes {
			return "", "", "", fmt.Errorf("%w: instruction exceeds %d UTF-8 bytes", errSpeechBadRequest, limits.ExtractionMaxInstructionBytes)
		}
		instruction = *request.Instruction
	}
	if request.Language != nil {
		language = strings.TrimSpace(*request.Language)
	}
	return contentType, language, instruction, nil
}

func validateSpeechSynthesizeRequest(request rpcapi.SpeechSynthesizeRequest, maxTextBytes int) ([]string, error) {
	if !validRuntimeAlias(strings.TrimSpace(request.VoiceName)) {
		return nil, errors.New("voice_name is invalid")
	}
	if !utf8.ValidString(request.Text) {
		return nil, errors.New("text must be valid UTF-8")
	}
	if strings.TrimSpace(request.Text) == "" || len(request.Text) > maxTextBytes {
		return nil, fmt.Errorf("%w: text must contain 1 to %d UTF-8 bytes", errSpeechBadRequest, maxTextBytes)
	}
	if len(request.AcceptedContentTypes) == 0 {
		return nil, errors.New("accepted_content_types is required")
	}
	if len(request.AcceptedContentTypes) > rpcSpeechMaxAcceptedTypes {
		return nil, fmt.Errorf("%w: accepted_content_types must contain at most %d values", errSpeechBadRequest, rpcSpeechMaxAcceptedTypes)
	}
	seen := make(map[string]struct{}, len(request.AcceptedContentTypes))
	accepted := make([]string, 0, len(request.AcceptedContentTypes))
	for _, value := range request.AcceptedContentTypes {
		if len(value) == 0 {
			return nil, errors.New("accepted_content_types contains an empty value")
		}
		if len(value) > rpcSpeechMaxContentTypeBytes {
			return nil, fmt.Errorf("%w: accepted content type exceeds %d bytes", errSpeechBadRequest, rpcSpeechMaxContentTypeBytes)
		}
		mediaType, _, err := mime.ParseMediaType(value)
		if err != nil {
			return nil, errors.New("accepted_content_types contains an invalid media type")
		}
		mediaType = strings.ToLower(mediaType)
		if _, exists := seen[mediaType]; exists {
			return nil, fmt.Errorf("%w: accepted_content_types contains duplicate media type %q", errSpeechBadRequest, mediaType)
		}
		seen[mediaType] = struct{}{}
		accepted = append(accepted, mediaType)
	}
	return accepted, nil
}

func validRuntimeAlias(value string) bool {
	return runtimeprofile.ValidateAlias("speech alias", value) == nil
}

func speechRPCError(err error) (rpcapi.RPCErrorCode, string) {
	switch {
	case errors.Is(err, errSpeechBadRequest):
		return rpcapi.RPCErrorCodeBadRequest, err.Error()
	case errors.Is(err, peergenx.ErrNotFound):
		return rpcapi.RPCErrorCodeNotFound, "speech alias not found"
	case errors.Is(err, peergenx.ErrInvalid), errors.Is(err, peergenx.ErrUnsupported):
		return rpcapi.RPCErrorCodeBadRequest, "speech request is not supported"
	case errors.Is(err, context.DeadlineExceeded):
		return rpcapi.RPCErrorCodeInternalError, "speech request timed out"
	default:
		return rpcapi.RPCErrorCodeInternalError, "speech provider failed"
	}
}

func speechExtractRPCError(err error) (rpcapi.RPCErrorCode, string) {
	switch {
	case errors.Is(err, errSpeechBadRequest):
		return rpcapi.RPCErrorCodeBadRequest, err.Error()
	case errors.Is(err, peergenx.ErrNotFound):
		return rpcapi.RPCErrorCodeNotFound, "speech alias not found"
	case errors.Is(err, peergenx.ErrInvalidOutput):
		return rpcapi.RPCErrorCodeInternalError, "speech extraction failed"
	case errors.Is(err, peergenx.ErrInvalid), errors.Is(err, peergenx.ErrUnsupported):
		return rpcapi.RPCErrorCodeBadRequest, "speech extraction request is not supported"
	case errors.Is(err, context.DeadlineExceeded):
		return rpcapi.RPCErrorCodeInternalError, "speech extraction timed out"
	default:
		return rpcapi.RPCErrorCodeInternalError, "speech extraction provider failed"
	}
}
