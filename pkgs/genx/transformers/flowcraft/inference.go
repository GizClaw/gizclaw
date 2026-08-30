package flowcraft

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GizClaw/flowcraft/core/inference"
	"github.com/GizClaw/flowcraft/core/message"
	"github.com/GizClaw/flowcraft/core/resource"
	flowllm "github.com/GizClaw/flowcraft/sdk/llm"
	flowmodel "github.com/GizClaw/flowcraft/sdk/model"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

const genXInferenceProvider = "gizclaw"

type pendingGeneration struct {
	model   inference.ModelRef
	request inference.GenerateRequest
	llm     *genXLLM
}

func newInferenceAssembly(generator genx.Generator, toolInvoker genx.ToolInvoker, modelNames []string) (*inference.Assembly, error) {
	if generator == nil {
		return nil, fmt.Errorf("flowcraft: Generator is required")
	}
	opener := inference.OpenGenerate(func(_ context.Context, model inference.ModelRef) (inference.GenerateOperations, error) {
		if model.ID.Provider != genXInferenceProvider || strings.TrimSpace(model.ID.Name) == "" || strings.Contains(model.ID.Name, "/") {
			return inference.GenerateOperations{}, fmt.Errorf("flowcraft: invalid model %q/%q", model.ID.Provider, model.ID.Name)
		}
		var pending sync.Map
		compile := inference.GenerateCompiler[string](func(_ context.Context, model inference.ModelRef, request inference.GenerateRequest, shape inference.GenerateExecutionShape) (inference.Compiled[string], error) {
			decisions := make([]inference.Decision, 0, len(request.ActiveFieldsFor(shape)))
			for _, field := range request.ActiveFieldsFor(shape) {
				decisions = append(decisions, inference.Decision{Field: field, Disposition: inference.Native})
			}
			key := genx.NewStreamID()
			pending.Store(key, pendingGeneration{model: model, request: request.Clone(), llm: &genXLLM{
				generator: generator, pattern: "model/" + model.ID.Name, toolInvoker: toolInvoker,
			}})
			return inference.Compiled[string]{
				Wire:   key,
				Report: inference.CompileReport{Operation: inference.OperationGenerate, Decisions: decisions},
			}, nil
		})
		load := func(key string) (pendingGeneration, error) {
			value, ok := pending.LoadAndDelete(key)
			if !ok {
				return pendingGeneration{}, fmt.Errorf("flowcraft: compiled inference request %q is unavailable", key)
			}
			return value.(pendingGeneration), nil
		}
		unaryTransport := inference.Transport[string, inference.GenerateResponse](func(ctx context.Context, key string) (inference.GenerateResponse, error) {
			wire, err := load(key)
			if err != nil {
				return inference.GenerateResponse{}, err
			}
			return generateUnary(ctx, wire)
		})
		unaryDecode := inference.Decoder[inference.GenerateResponse, inference.GenerateResponse](func(_ context.Context, response inference.GenerateResponse) (inference.GenerateResponse, error) {
			return response, nil
		})
		streamTransport := inference.Transport[string, inference.ProviderStream[inference.GenerateStreamEvent]](func(ctx context.Context, key string) (inference.ProviderStream[inference.GenerateStreamEvent], error) {
			wire, err := load(key)
			if err != nil {
				return nil, err
			}
			return generateStream(ctx, wire)
		})
		streamDecode := inference.GenerateStreamDecoder[inference.GenerateStreamEvent](func(_ context.Context, event inference.GenerateStreamEvent) (inference.GenerateStreamEvent, error) {
			return event, nil
		})
		return inference.BindGenerateOperations(compile, unaryTransport, unaryDecode, streamTransport, streamDecode)
	})
	models := make([]inference.ModelImplementation, 0, len(modelNames))
	seen := make(map[string]struct{}, len(modelNames))
	for _, name := range modelNames {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		models = append(models, inference.ModelImplementation{
			Descriptor: inference.ModelDescriptor{ID: inference.ModelID{Provider: genXInferenceProvider, Name: name}},
			Openers:    inference.Openers{Generate: opener},
		})
	}
	definition := inference.ProviderDefinition{ID: genXInferenceProvider, Models: models}
	value, err := (inference.Factory{}).New(context.Background(), resource.Input{
		Deps: map[string]any{"provider." + genXInferenceProvider: definition},
	})
	if err != nil {
		return nil, err
	}
	return value.(*inference.Assembly), nil
}

func generateUnary(ctx context.Context, wire pendingGeneration) (inference.GenerateResponse, error) {
	messages, options, err := legacyGenerateInput(wire.request)
	if err != nil {
		return inference.GenerateResponse{}, err
	}
	response, usage, err := wire.llm.Generate(ctx, messages, options...)
	if err != nil {
		return inference.GenerateResponse{}, err
	}
	return inference.GenerateResponse{
		Message: coreMessage(response), FinishReason: inference.FinishCompleted,
		Usage: inference.Usage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, TotalTokens: usage.TotalTokens},
	}, nil
}

func generateStream(ctx context.Context, wire pendingGeneration) (inference.ProviderStream[inference.GenerateStreamEvent], error) {
	messages, options, err := legacyGenerateInput(wire.request)
	if err != nil {
		return nil, err
	}
	stream, err := wire.llm.GenerateStream(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return &inferenceEventStream{stream: stream}, nil
}

func legacyGenerateInput(request inference.GenerateRequest) ([]flowmodel.Message, []flowllm.GenerateOption, error) {
	messages := make([]flowmodel.Message, 0, len(request.Context)+1)
	for _, item := range append(append([]message.Message(nil), request.Context...), request.Input.Message()) {
		text, err := messageText(item)
		if err != nil {
			return nil, nil, err
		}
		messages = append(messages, flowmodel.NewTextMessage(flowmodel.Role(item.Role), text))
	}
	var options []flowllm.GenerateOption
	if text := request.Input.Content.Intent.Text; text != nil {
		if text.MaxOutputTokens != nil {
			options = append(options, flowllm.WithMaxTokens(int64(*text.MaxOutputTokens)))
		}
		if text.Temperature != nil {
			options = append(options, flowllm.WithTemperature(*text.Temperature))
		}
		if text.TopP != nil {
			options = append(options, flowllm.WithTopP(*text.TopP))
		}
		if text.ReasoningEnabled != nil {
			options = append(options, flowllm.WithThinking(*text.ReasoningEnabled))
		}
		if text.Response != nil {
			switch text.Response.Kind {
			case inference.ResponseJSONObject:
				options = append(options, flowllm.WithJSONMode(true))
			case inference.ResponseJSONSchema:
				options = append(options, flowllm.WithJSONSchema(flowllm.JSONSchemaParam{Name: text.Response.Name, Schema: text.Response.Schema}))
			}
		}
	}
	return messages, options, nil
}

func messageText(item message.Message) (string, error) {
	var text strings.Builder
	for _, raw := range item.Content.Parts {
		part, err := message.NormalizePart(raw)
		if err != nil {
			return "", err
		}
		if data, ok := part.(message.DataPart); ok && data.MediaType == "application/vnd.genx.interruption+json" {
			continue
		}
		value, ok := part.(message.TextPart)
		if !ok {
			return "", fmt.Errorf("flowcraft: unsupported model message part %q", part.Kind())
		}
		text.WriteString(value.Text)
	}
	return text.String(), nil
}

func coreMessage(item flowmodel.Message) message.Message {
	return message.Message{Role: message.Role(item.Role), Content: message.Content{Parts: []message.Part{message.TextPart{Text: item.Content()}}}}
}

type inferenceEventStream struct {
	stream   flowllm.StreamMessage
	finished bool
}

func (s *inferenceEventStream) Next(context.Context) (inference.GenerateStreamEvent, error) {
	if s.finished {
		return inference.GenerateStreamEvent{}, io.EOF
	}
	if s.stream.Next() {
		return inference.GenerateStreamEvent{PartIndex: 0, Delta: inference.TextPartDelta{Text: s.stream.Current().Content}}, nil
	}
	if err := s.stream.Err(); err != nil {
		return inference.GenerateStreamEvent{}, err
	}
	s.finished = true
	usage := s.stream.Usage()
	return inference.GenerateStreamEvent{
		Usage:        &inference.Usage{InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, TotalTokens: usage.InputTokens + usage.OutputTokens},
		FinishReason: inference.FinishCompleted,
	}, nil
}

func (s *inferenceEventStream) Close() error { return s.stream.Close() }
