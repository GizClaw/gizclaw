package giztest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const speechPCM16ContentType = "audio/L16;rate=16000;channels=1"

type speechFixtureCache struct {
	mu      sync.Mutex
	entries map[string]*speechFixtureEntry
}

type speechFixtureEntry struct {
	ready  chan struct{}
	result operationResult
	err    error
}

func newSpeechFixtureCache() *speechFixtureCache {
	return &speechFixtureCache{entries: make(map[string]*speechFixtureEntry)}
}

func (c *speechFixtureCache) Do(ctx context.Context, key string, invoke func() (operationResult, error)) (operationResult, bool, error) {
	if c == nil {
		result, err := invoke()
		return result, false, err
	}
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		entry = &speechFixtureEntry{ready: make(chan struct{})}
		c.entries[key] = entry
	}
	c.mu.Unlock()
	if ok {
		select {
		case <-entry.ready:
			return cloneOperationResult(entry.result), true, entry.err
		case <-ctx.Done():
			return operationResult{}, true, context.Cause(ctx)
		}
	}

	result, err := invoke()
	c.mu.Lock()
	entry.result = cloneOperationResult(result)
	entry.err = err
	if err != nil {
		delete(c.entries, key)
	}
	close(entry.ready)
	c.mu.Unlock()
	return cloneOperationResult(result), false, err
}

func speechFixtureKey(documentPath string, step Step, request any, outputSpec VariableSpec) (string, error) {
	payload, err := json.Marshal(struct {
		Document string
		Step     string
		Request  any
		Output   VariableSpec
	}{Document: documentPath, Step: step.ID, Request: request, Output: outputSpec})
	if err != nil {
		return "", fmt.Errorf("encode speech fixture cache key: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(payload)), nil
}

func cloneOperationResult(input operationResult) operationResult {
	result := input
	if value, ok := input.saved.([]byte); ok {
		result.saved = bytes.Clone(value)
	}
	if value, ok := input.assertion.(map[string]any); ok {
		result.assertion = maps.Clone(value)
	}
	result.evidence = maps.Clone(input.evidence)
	return result
}

// invokeSpeech runs one speech operation. fullEvidence adds the recognized
// text of a transcription to the report evidence; the default redacted mode
// keeps only the method because transcripts carry spoken user content.
func invokeSpeech(ctx context.Context, client *gizcli.Client, step Step, request, input any, inputSpec, outputSpec VariableSpec, fullEvidence bool) (operationResult, error) {
	op := step.Speech
	if op == nil {
		return operationResult{}, fmt.Errorf("speech operation required")
	}
	maxBytes := outputSpec.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 16 << 20
	}
	switch op.Method {
	case "server.speech.synthesize":
		var req rpcapi.SpeechSynthesizeRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		buf := &boundedBuffer{max: maxBytes}
		result, err := client.SynthesizeSpeech(ctx, step.ID, req, buf)
		if err != nil {
			return operationResult{}, err
		}
		if step.SaveAs != "" && outputSpec.Type != "audio" {
			return operationResult{}, fmt.Errorf("speech synthesis save_as must target audio output")
		}
		if step.SaveAs != "" && result.Metadata.ContentType != outputSpec.MediaType {
			return operationResult{}, fmt.Errorf("speech synthesis content type %q does not match output media_type %q", result.Metadata.ContentType, outputSpec.MediaType)
		}
		object, err := jsonObject(result.Metadata)
		if err != nil {
			return operationResult{}, err
		}
		object["bytes"] = result.Bytes
		return operationResult{assertion: object, saved: append([]byte(nil), buf.Bytes()...), evidence: map[string]any{"method": op.Method, "bytes": result.Bytes}}, nil
	case "server.speech.transcribe":
		audio, ok := input.([]byte)
		if !ok {
			return operationResult{}, fmt.Errorf("speech transcription input must be audio bytes")
		}
		if inputSpec.Type != "audio" || inputSpec.MediaType == "" || inputSpec.Codec == "" {
			return operationResult{}, fmt.Errorf("speech transcription input requires typed audio media metadata")
		}
		var req rpcapi.SpeechTranscribeRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		audio, req, err := prepareTranscriptionRequest(audio, inputSpec, req)
		if err != nil {
			return operationResult{}, err
		}
		result, err := client.TranscribeSpeech(ctx, step.ID, req, bytes.NewReader(audio))
		if err != nil {
			return operationResult{}, err
		}
		object, err := jsonObject(result)
		evidence := map[string]any{"method": op.Method}
		if fullEvidence {
			evidence["transcript"] = result.Transcript
		}
		return operationResult{assertion: object, saved: object, evidence: evidence}, err
	case "server.speech.extract":
		audio, ok := input.([]byte)
		if !ok {
			return operationResult{}, fmt.Errorf("speech extraction input must be audio bytes")
		}
		if inputSpec.Type != "audio" || inputSpec.MediaType == "" || inputSpec.Codec == "" {
			return operationResult{}, fmt.Errorf("speech extraction input requires typed audio media metadata")
		}
		audio, err := speechPCMInput(audio, inputSpec)
		if err != nil {
			return operationResult{}, err
		}
		var req rpcapi.SpeechExtractRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		result, err := client.ExtractSpeech(ctx, step.ID, req, bytes.NewReader(audio))
		if err != nil {
			return operationResult{}, err
		}
		object, err := jsonObject(result)
		return operationResult{assertion: object, saved: object, evidence: map[string]any{"method": op.Method}}, err
	default:
		return operationResult{}, fmt.Errorf("unsupported speech method %q", op.Method)
	}
}

func prepareTranscriptionRequest(audio []byte, spec VariableSpec, request rpcapi.SpeechTranscribeRequest) ([]byte, rpcapi.SpeechTranscribeRequest, error) {
	if spec.Codec == "opus" {
		if spec.MediaType != "audio/ogg" {
			return nil, rpcapi.SpeechTranscribeRequest{}, fmt.Errorf("speech transcription Opus input media_type must be audio/ogg, got %q", spec.MediaType)
		}
		decoded, err := speechPCMInput(audio, spec)
		if err != nil {
			return nil, rpcapi.SpeechTranscribeRequest{}, err
		}
		request.ContentType = speechPCM16ContentType
		return decoded, request, nil
	}
	if spec.Codec != "pcm_s16le" || spec.MediaType != speechPCM16ContentType {
		return nil, rpcapi.SpeechTranscribeRequest{}, fmt.Errorf("speech transcription input must be Ogg/Opus or 16 kHz mono PCM, got codec %q and media_type %q", spec.Codec, spec.MediaType)
	}
	request.ContentType = speechPCM16ContentType
	return audio, request, nil
}

func speechPCMInput(audio []byte, spec VariableSpec) ([]byte, error) {
	if spec.Codec != "opus" {
		return audio, nil
	}
	output := &boundedBuffer{max: spec.MaxBytes}
	if _, err := codecconv.OggToPCM(output, bytes.NewReader(audio), opus.SampleRate16K); err != nil {
		return nil, fmt.Errorf("decode synthesized Ogg Opus for speech input: %w", err)
	}
	return append([]byte(nil), output.Bytes()...), nil
}
