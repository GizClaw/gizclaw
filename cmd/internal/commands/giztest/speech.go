package giztest

import (
	"bytes"
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func invokeSpeech(ctx context.Context, client *gizcli.Client, step Step, request, input any, inputSpec, outputSpec VariableSpec) (operationResult, error) {
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
		audio, err := speechPCMInput(audio, inputSpec)
		if err != nil {
			return operationResult{}, err
		}
		var req rpcapi.SpeechTranscribeRequest
		if err := decodeRequest(request, &req); err != nil {
			return operationResult{}, err
		}
		result, err := client.TranscribeSpeech(ctx, step.ID, req, bytes.NewReader(audio))
		if err != nil {
			return operationResult{}, err
		}
		object, err := jsonObject(result)
		return operationResult{assertion: object, saved: object, evidence: map[string]any{"method": op.Method}}, err
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
