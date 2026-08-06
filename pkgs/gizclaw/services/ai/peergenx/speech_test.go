package peergenx

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type aliasModels struct {
	fakeModels
	aliases map[string]string
}

func (m aliasModels) ResolveModelAlias(alias string) (string, bool) {
	value, ok := m.aliases[alias]
	return value, ok
}

func (m aliasModels) GetCanonicalModel(ctx context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	return m.fakeModels.GetModel(ctx, request)
}

type aliasVoices struct {
	fakeVoices
	aliases map[string]string
}

func (v aliasVoices) ResolveVoiceAlias(alias string) (string, bool) {
	value, ok := v.aliases[alias]
	return value, ok
}

func (v aliasVoices) GetCanonicalVoice(ctx context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	return v.fakeVoices.GetVoice(ctx, request)
}

func TestRuntimeAliasesRejectNonExactCanonicalIDs(t *testing.T) {
	t.Parallel()
	svc := New(Service{
		Models: aliasModels{aliases: map[string]string{"chat": " model-id "}},
		Voices: aliasVoices{aliases: map[string]string{"narrator": " voice-id "}},
	})
	if _, err := svc.resolveModelAliasID("chat"); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("resolveModelAliasID() error = %v, want exact ID rejection", err)
	}
	if _, err := svc.resolveVoiceAliasID("narrator"); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "surrounding whitespace") {
		t.Fatalf("resolveVoiceAliasID() error = %v, want exact ID rejection", err)
	}
}

func TestExtractResolvesRuntimeProfileAliasesAndValidatesResult(t *testing.T) {
	events := []string{}
	builder := &speechExtractionBuilder{
		events:    &events,
		arguments: `{"phone":"123","name":"Alice"}`,
	}
	svc := New(Service{
		Models: speechExtractionModels{
			events:  &events,
			aliases: map[string]string{"asr": "canonical-asr", "extract": "canonical-extract"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         builder,
	})

	result, err := svc.Extract(context.Background(), SpeechExtractionRequest{
		ASRModelAlias:       "asr",
		ExtractModelAlias:   "extract",
		Language:            "en",
		SchemaJSON:          `{"type":"object","properties":{"name":{"type":"string"},"phone":{"type":"string"}},"required":["name","phone"],"additionalProperties":false}`,
		Instruction:         "extract the contact",
		Input:               newTextStream("Alice 123"),
		MaxSchemaDepth:      16,
		MaxSchemaProperties: 8,
		MaxTranscriptBytes:  1024,
		MaxResultBytes:      1024,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if result.Transcript != "Alice 123" || result.ResultJSON != `{"name":"Alice","phone":"123"}` {
		t.Fatalf("Extract() = %+v", result)
	}
	if builder.tool == nil || builder.tool.Name != "extract" || builder.tool.Argument == nil {
		t.Fatalf("extraction tool = %+v", builder.tool)
	}
	inspected, err := genx.InspectModelContext(builder.modelContext)
	if err != nil {
		t.Fatalf("InspectModelContext() error = %v", err)
	}
	if !strings.Contains(inspected, "extract the contact") || !strings.Contains(inspected, "Alice 123") {
		t.Fatalf("model context = %q", inspected)
	}
	wantEvents := []string{
		"get:model:canonical-asr",
		"get:tenant:volc:main",
		"get:credential:volc-token",
		"build:transformer:model:canonical-asr",
		"call:transformer:model/asr?language=en",
		"get:model:canonical-extract",
		"get:tenant:openai:main",
		"get:credential:openai-key",
		"build:generator:canonical-extract",
		"call:invoke:model/extract",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
}

func TestExtractRejectsSchemaInvalidProviderResult(t *testing.T) {
	events := []string{}
	svc := New(Service{
		Models: speechExtractionModels{
			events:  &events,
			aliases: map[string]string{"asr": "canonical-asr", "extract": "canonical-extract"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder: &speechExtractionBuilder{
			events:    &events,
			arguments: `{"name":12}`,
		},
	})
	_, err := svc.Extract(context.Background(), SpeechExtractionRequest{
		ASRModelAlias:       "asr",
		ExtractModelAlias:   "extract",
		SchemaJSON:          `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
		Input:               newTextStream("Alice"),
		MaxSchemaDepth:      16,
		MaxSchemaProperties: 8,
		MaxTranscriptBytes:  1024,
		MaxResultBytes:      1024,
	})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Extract() error = %v, want %v", err, ErrInvalidOutput)
	}
}

func TestExtractPreservesJSONNumberPrecision(t *testing.T) {
	events := []string{}
	svc := newSpeechExtractionTestService(&events, &speechExtractionBuilder{
		events:    &events,
		arguments: `{"id":9007199254740993}`,
	})
	request := validSpeechExtractionRequest()
	request.SchemaJSON = `{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`

	result, err := svc.Extract(context.Background(), request)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if result.ResultJSON != `{"id":9007199254740993}` {
		t.Fatalf("ResultJSON = %q", result.ResultJSON)
	}
}

func TestExtractValidatesPrecisionPreservedInteger(t *testing.T) {
	events := []string{}
	svc := newSpeechExtractionTestService(&events, &speechExtractionBuilder{
		events:    &events,
		arguments: `{"id":9007199254740993}`,
	})
	request := validSpeechExtractionRequest()
	request.SchemaJSON = `{"type":"object","properties":{"id":{"type":"integer","maximum":9007199254740992}},"required":["id"]}`

	_, err := svc.Extract(context.Background(), request)
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Extract() error = %v, want %v", err, ErrInvalidOutput)
	}
}

func TestExtractPreservesFractionalAndExponentNumberPrecision(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
	}{
		{name: "fractional", arguments: `{"score":9007199254740993.0}`},
		{name: "exponent", arguments: `{"score":1e-1}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			svc := newSpeechExtractionTestService(&events, &speechExtractionBuilder{
				events:    &events,
				arguments: test.arguments,
			})
			request := validSpeechExtractionRequest()
			request.SchemaJSON = `{"type":"object","properties":{"score":{"type":"number"}},"required":["score"]}`

			result, err := svc.Extract(context.Background(), request)
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			if result.ResultJSON != test.arguments {
				t.Fatalf("ResultJSON = %q, want %q", result.ResultJSON, test.arguments)
			}
		})
	}
}

func TestExtractValidatesArbitraryPrecisionNumberConstraints(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		maximum   string
	}{
		{name: "fractional", arguments: `{"score":0.1}`, maximum: "0.099999999999999999999999999999"},
		{name: "exponent", arguments: `{"score":1e-1}`, maximum: "9e-2"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			svc := newSpeechExtractionTestService(&events, &speechExtractionBuilder{
				events:    &events,
				arguments: test.arguments,
			})
			request := validSpeechExtractionRequest()
			request.SchemaJSON = `{"type":"object","properties":{"score":{"type":"number","maximum":` + test.maximum + `}},"required":["score"]}`

			_, err := svc.Extract(context.Background(), request)
			if !errors.Is(err, ErrInvalidOutput) {
				t.Fatalf("Extract() error = %v, want %v", err, ErrInvalidOutput)
			}
		})
	}
}

func newSpeechExtractionTestService(events *[]string, builder Builder) *Service {
	return New(Service{
		Models: speechExtractionModels{
			events:  events,
			aliases: map[string]string{"asr": "canonical-asr", "extract": "canonical-extract"},
		},
		Credentials:     fakeCredentials{events: events},
		ProviderTenants: fakeTenants{events: events},
		Builder:         builder,
	})
}

func validSpeechExtractionRequest() SpeechExtractionRequest {
	return SpeechExtractionRequest{
		ASRModelAlias:       "asr",
		ExtractModelAlias:   "extract",
		SchemaJSON:          `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
		Input:               newTextStream("Alice"),
		MaxSchemaDepth:      16,
		MaxSchemaProperties: 8,
		MaxTranscriptBytes:  1024,
		MaxResultBytes:      1024,
	}
}

func TestExtractRejectsOversizedTranscriptBeforeLLMInvocation(t *testing.T) {
	events := []string{}
	builder := &speechExtractionBuilder{
		events:    &events,
		arguments: `{"name":"Alice"}`,
	}
	svc := newSpeechExtractionTestService(&events, builder)
	request := validSpeechExtractionRequest()
	request.Input = newTextStream("oversized")
	request.MaxTranscriptBytes = 4

	_, err := svc.Extract(context.Background(), request)
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Extract() error = %v, want %v", err, ErrInvalidOutput)
	}
	for _, event := range events {
		if strings.HasPrefix(event, "build:generator:") || strings.HasPrefix(event, "call:invoke:") {
			t.Fatalf("LLM invoked for oversized transcript: events = %#v", events)
		}
	}
}

func TestExtractRejectsInvalidInvocationOutputs(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		invoke    func(context.Context, *genx.FuncTool) (*genx.FuncCall, error)
	}{
		{
			name: "missing invocation",
			invoke: func(context.Context, *genx.FuncTool) (*genx.FuncCall, error) {
				return nil, nil
			},
		},
		{name: "malformed arguments", arguments: `{`},
		{name: "non object arguments", arguments: `[]`},
		{name: "oversized arguments", arguments: strings.Repeat(" ", 1024) + `{"name":"Alice"}`},
		{
			name: "wrong tool",
			invoke: func(_ context.Context, tool *genx.FuncTool) (*genx.FuncCall, error) {
				call := tool.NewFuncCall(`{"name":"Alice"}`)
				call.Name = "other"
				return call, nil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			builder := &speechExtractionBuilder{
				events:    &events,
				arguments: test.arguments,
				invoke:    test.invoke,
			}
			svc := newSpeechExtractionTestService(&events, builder)
			_, err := svc.Extract(context.Background(), validSpeechExtractionRequest())
			if !errors.Is(err, ErrInvalidOutput) {
				t.Fatalf("Extract() error = %v, want %v", err, ErrInvalidOutput)
			}
		})
	}
}

func TestExtractRejectsCanonicalResultOverByteLimit(t *testing.T) {
	events := []string{}
	builder := &speechExtractionBuilder{
		events:    &events,
		arguments: `{"name":"<"}`,
	}
	svc := newSpeechExtractionTestService(&events, builder)
	request := validSpeechExtractionRequest()
	request.MaxResultBytes = len(builder.arguments)
	_, err := svc.Extract(context.Background(), request)
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("Extract() error = %v, want %v", err, ErrInvalidOutput)
	}
}

func TestExtractPropagatesProviderFailureAndCancellation(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *genx.FuncTool) (*genx.FuncCall, error)
		want error
	}{
		{
			name: "provider failure",
			run: func(context.Context, *genx.FuncTool) (*genx.FuncCall, error) {
				return nil, errors.New("secret provider failure")
			},
			want: errors.New("secret provider failure"),
		},
		{
			name: "cancellation",
			run: func(ctx context.Context, _ *genx.FuncTool) (*genx.FuncCall, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
			want: context.Canceled,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			builder := &speechExtractionBuilder{events: &events, invoke: test.run}
			svc := newSpeechExtractionTestService(&events, builder)
			ctx := context.Background()
			cancel := func() {}
			if test.name == "cancellation" {
				var cancelContext context.CancelFunc
				ctx, cancelContext = context.WithCancel(ctx)
				cancel = cancelContext
				cancel()
			}
			defer cancel()
			_, err := svc.Extract(ctx, validSpeechExtractionRequest())
			if test.name == "provider failure" {
				if err == nil || err.Error() != test.want.Error() {
					t.Fatalf("Extract() error = %v, want %v", err, test.want)
				}
				return
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("Extract() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestExtractRejectsUnknownAliasAndWrongModelKind(t *testing.T) {
	tests := []struct {
		name        string
		aliases     map[string]string
		extractKind apitypes.ModelKind
		want        error
	}{
		{
			name:    "unknown extraction alias",
			aliases: map[string]string{"asr": "canonical-asr"},
			want:    ErrNotFound,
		},
		{
			name:        "wrong extraction model kind",
			aliases:     map[string]string{"asr": "canonical-asr", "extract": "canonical-extract"},
			extractKind: apitypes.ModelKindAsr,
			want:        ErrInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			events := []string{}
			svc := New(Service{
				Models: speechExtractionModels{
					events:      &events,
					aliases:     test.aliases,
					extractKind: test.extractKind,
				},
				Credentials:     fakeCredentials{events: &events},
				ProviderTenants: fakeTenants{events: &events},
				Builder:         &speechExtractionBuilder{events: &events, arguments: `{"name":"Alice"}`},
			})
			_, err := svc.Extract(context.Background(), validSpeechExtractionRequest())
			if !errors.Is(err, test.want) {
				t.Fatalf("Extract() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestParseSpeechExtractionSchemaRejectsInvalidOrUnboundedSchemas(t *testing.T) {
	tests := []struct {
		name          string
		schema        string
		maxDepth      int
		maxProperties int
	}{
		{name: "malformed", schema: `{`, maxDepth: 16, maxProperties: 8},
		{name: "non object root", schema: `{"type":"array"}`, maxDepth: 16, maxProperties: 8},
		{name: "external reference", schema: `{"type":"object","properties":{"item":{"$ref":"https://example.com/schema.json"}}}`, maxDepth: 16, maxProperties: 8},
		{name: "depth", schema: `{"type":"object","properties":{"item":{"type":"object"}}}`, maxDepth: 2, maxProperties: 8},
		{name: "properties", schema: `{"type":"object","properties":{"a":{"type":"string"},"b":{"type":"string"}}}`, maxDepth: 16, maxProperties: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := parseSpeechExtractionSchema(test.schema, test.maxDepth, test.maxProperties)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("parseSpeechExtractionSchema() error = %v, want %v", err, ErrInvalid)
			}
		})
	}
}

func TestTranscribeResolvesASRAliasAndRejectsCanonicalID(t *testing.T) {
	events := []string{}
	svc := New(Service{
		Models: aliasModels{
			fakeModels: fakeModels{events: &events, modelKind: apitypes.ModelKindAsr, providerKind: string(apitypes.ModelProviderKindVolcTenant)},
			aliases:    map[string]string{"asr-model": "canonical-asr"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	transcript, err := svc.Transcribe(context.Background(), "asr-model", "zh-CN", newTextStream("hello"))
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if transcript != "hello" {
		t.Fatalf("Transcribe() = %q", transcript)
	}
	wantPrefix := []string{"get:model:canonical-asr", "get:tenant:volc:main", "get:credential:volc-token"}
	if !reflect.DeepEqual(events[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("events = %#v, want prefix %#v", events, wantPrefix)
	}
	if _, err := svc.Transcribe(context.Background(), "canonical-asr", "", newTextStream("hello")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("canonical Transcribe() error = %v, want %v", err, ErrNotFound)
	}
}

type speechExtractionModels struct {
	events      *[]string
	aliases     map[string]string
	extractKind apitypes.ModelKind
}

func (m speechExtractionModels) ResolveModelAlias(alias string) (string, bool) {
	value, ok := m.aliases[alias]
	return value, ok
}

func (m speechExtractionModels) GetModel(_ context.Context, request adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	*m.events = append(*m.events, "get:model:"+request.Id)
	model := apitypes.Model{
		Id:   request.Id,
		Kind: apitypes.ModelKindLlm,
		Provider: apitypes.ModelProvider{
			Kind: apitypes.ModelProviderKindOpenaiTenant,
			Id:   "main",
		},
	}
	if request.Id == "canonical-asr" {
		model.Kind = apitypes.ModelKindAsr
		model.Provider.Kind = apitypes.ModelProviderKindVolcTenant
	} else if m.extractKind != "" {
		model.Kind = m.extractKind
	}
	return adminhttp.GetModel200JSONResponse(model), nil
}

type speechExtractionBuilder struct {
	events       *[]string
	arguments    string
	tool         *genx.FuncTool
	modelContext genx.ModelContext
	invoke       func(context.Context, *genx.FuncTool) (*genx.FuncCall, error)
}

func (b *speechExtractionBuilder) BuildGenerator(_ context.Context, cfg GeneratorConfig) (genx.Generator, error) {
	*b.events = append(*b.events, "build:generator:"+cfg.Model.Id)
	return speechExtractionGenerator{builder: b}, nil
}

func (b *speechExtractionBuilder) BuildTransformer(_ context.Context, cfg TransformerConfig) (genx.Transformer, error) {
	*b.events = append(*b.events, "build:transformer:model:"+cfg.Model.Id)
	return speechExtractionTransformer{events: b.events, pattern: cfg.Pattern}, nil
}

type speechExtractionGenerator struct {
	builder *speechExtractionBuilder
}

func (g speechExtractionGenerator) GenerateStream(context.Context, string, genx.ModelContext) (genx.Stream, error) {
	return nil, errors.New("unexpected GenerateStream")
}

func (g speechExtractionGenerator) Invoke(ctx context.Context, pattern string, modelContext genx.ModelContext, tool *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	*g.builder.events = append(*g.builder.events, "call:invoke:"+pattern)
	g.builder.tool = tool
	g.builder.modelContext = modelContext
	if g.builder.invoke != nil {
		call, err := g.builder.invoke(ctx, tool)
		return genx.Usage{}, call, err
	}
	return genx.Usage{}, tool.NewFuncCall(g.builder.arguments), nil
}

type speechExtractionTransformer struct {
	events  *[]string
	pattern string
}

func (t speechExtractionTransformer) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	*t.events = append(*t.events, "call:transformer:"+t.pattern)
	return input, nil
}

func TestTranscribeRejectsWrongModelKind(t *testing.T) {
	events := []string{}
	svc := New(Service{
		Models: aliasModels{
			fakeModels: fakeModels{events: &events, modelKind: apitypes.ModelKindTts, providerKind: string(apitypes.ModelProviderKindVolcTenant)},
			aliases:    map[string]string{"wrong": "canonical-tts"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, err := svc.Transcribe(context.Background(), "wrong", "", newTextStream("audio")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Transcribe() error = %v, want %v", err, ErrInvalid)
	}
}

func TestSynthesizeResolvesVoiceAliasAndRejectsCanonicalID(t *testing.T) {
	events := []string{}
	svc := New(Service{
		Voices: aliasVoices{
			fakeVoices: fakeVoices{events: &events},
			aliases:    map[string]string{"narrator": "canonical-voice"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	result, err := svc.Synthesize(context.Background(), "narrator", "hello", []string{"audio/ogg"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	_ = result.Stream.Close()
	if result.ContentType != "audio/ogg" || result.SampleRateHz != nil || result.Channels != nil {
		t.Fatalf("Synthesize() metadata = %+v", result)
	}
	if len(events) == 0 || events[0] != "get:voice:canonical-voice" {
		t.Fatalf("events = %#v", events)
	}
	if _, err := svc.Synthesize(context.Background(), "canonical-voice", "hello", []string{"audio/ogg"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("canonical Synthesize() error = %v, want %v", err, ErrNotFound)
	}
}

func TestSynthesizeReportsConfiguredMiniMaxPCMSampleRate(t *testing.T) {
	events := []string{}
	sampleRate := 24000
	svc := New(Service{
		Voices: aliasVoices{
			fakeVoices: fakeVoices{
				events:       &events,
				providerKind: apitypes.VoiceProviderKindMinimaxTenant,
				sampleRate:   &sampleRate,
			},
			aliases: map[string]string{"narrator": "canonical-voice"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	result, err := svc.Synthesize(context.Background(), "narrator", "hello", []string{"audio/pcm"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	_ = result.Stream.Close()
	if result.SampleRateHz == nil || *result.SampleRateHz != int32(sampleRate) || result.Channels == nil || *result.Channels != 1 {
		t.Fatalf("Synthesize() metadata = %+v, want %d Hz mono", result, sampleRate)
	}
}

func TestSynthesizeRejectsInvalidMiniMaxPCMSampleRate(t *testing.T) {
	events := []string{}
	sampleRate := 0
	svc := New(Service{
		Voices: aliasVoices{
			fakeVoices: fakeVoices{
				events:       &events,
				providerKind: apitypes.VoiceProviderKindMinimaxTenant,
				sampleRate:   &sampleRate,
			},
			aliases: map[string]string{"narrator": "canonical-voice"},
		},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, err := svc.Synthesize(context.Background(), "narrator", "hello", []string{"audio/pcm"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Synthesize() error = %v, want %v", err, ErrInvalid)
	}
}

func TestSelectSpeechSynthesisFormatHonorsOrderedPreference(t *testing.T) {
	format, contentType, raw, err := selectSpeechSynthesisFormat(
		string(apitypes.VoiceProviderKindVolcTenant),
		[]string{"audio/mpeg", "audio/ogg"},
	)
	if err != nil || format != "mp3" || contentType != "audio/mpeg" || raw {
		t.Fatalf("selectSpeechSynthesisFormat() = (%q, %q, %t, %v)", format, contentType, raw, err)
	}

	format, contentType, raw, err = selectSpeechSynthesisFormat(
		string(apitypes.VoiceProviderKindMinimaxTenant),
		[]string{"audio/ogg", "audio/pcm"},
	)
	if err != nil || format != "pcm" || contentType != "audio/pcm" || !raw {
		t.Fatalf("selectSpeechSynthesisFormat(raw) = (%q, %q, %t, %v)", format, contentType, raw, err)
	}

	if _, _, _, err := selectSpeechSynthesisFormat(
		string(apitypes.VoiceProviderKindMinimaxTenant),
		[]string{"audio/ogg"},
	); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("selectSpeechSynthesisFormat(unsupported) error = %v, want %v", err, ErrUnsupported)
	}
}
