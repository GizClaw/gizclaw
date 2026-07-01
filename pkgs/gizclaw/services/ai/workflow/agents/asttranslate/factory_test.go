package asttranslate

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
)

func TestFactoryMergesWorkflowAndWorkspaceParams(t *testing.T) {
	langPair := "zh/ja"
	mode := apitypes.ASTTranslateModeS2s
	input := apitypes.WorkspaceInputModePushToTalk
	var voice apitypes.ASTTranslateVoiceParameters
	if err := voice.FromASTTranslateInternalSpeakerParameters(apitypes.ASTTranslateInternalSpeakerParameters{
		SpeakerId: "workspace-speaker",
	}); err != nil {
		t.Fatalf("FromASTTranslateInternalSpeakerParameters() error = %v", err)
	}
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromASTTranslateWorkspaceParameters(apitypes.ASTTranslateWorkspaceParameters{
		LangPair: &langPair,
		Mode:     &mode,
		Input:    &input,
		Voice:    &voice,
	}); err != nil {
		t.Fatalf("FromASTTranslateWorkspaceParameters() error = %v", err)
	}
	agent, err := (Factory{Transformer: recordingTransformer{}}).NewAgent(context.Background(), agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo", Parameters: &workspaceParams},
		Workflow: apitypes.WorkflowDocument{
			Metadata: apitypes.WorkflowMetadata{Name: "ast"},
			Spec: apitypes.WorkflowSpec{
				Driver: apitypes.WorkflowDriverAstTranslate,
				AstTranslate: &apitypes.ASTTranslateWorkflowSpec{
					TranslationModel: "ast-model",
					SpeakerId:        stringPtr("workflow-speaker"),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("NewAgent() error = %v", err)
	}
	stream, err := agent.Transform(context.Background(), "ignored", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	got := string(chunk.Part.(genx.Text))
	if !strings.HasPrefix(got, "model/ast-model?") ||
		!strings.Contains(got, "source_language=zh") ||
		!strings.Contains(got, "target_language=ja") ||
		!strings.Contains(got, "mode=s2s") ||
		!strings.Contains(got, "input=push-to-talk") ||
		!strings.Contains(got, "speaker_id=workspace-speaker") ||
		strings.Contains(got, "workflow-speaker") {
		t.Fatalf("pattern = %q, want AST translate params", got)
	}
}

func TestResolveConfigExternalVoiceForcesS2TAndSourceLanguage(t *testing.T) {
	var voice apitypes.ASTTranslateVoiceParameters
	if err := voice.FromASTTranslateExternalVoiceParameters(apitypes.ASTTranslateExternalVoiceParameters{
		TtsVoice: "volc-tenant:e2e-volc-tenant:voice-a",
	}); err != nil {
		t.Fatalf("FromASTTranslateExternalVoiceParameters() error = %v", err)
	}
	langPair := "auto"
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromASTTranslateWorkspaceParameters(apitypes.ASTTranslateWorkspaceParameters{
		LangPair: &langPair,
		Voice:    &voice,
	}); err != nil {
		t.Fatalf("FromASTTranslateWorkspaceParameters() error = %v", err)
	}
	resolved, err := resolveConfig(agenthost.Spec{
		Workspace: apitypes.Workspace{Name: "demo", Parameters: &workspaceParams},
		Workflow: apitypes.WorkflowDocument{
			Metadata: apitypes.WorkflowMetadata{Name: "ast"},
			Spec: apitypes.WorkflowSpec{
				Driver: apitypes.WorkflowDriverAstTranslate,
				AstTranslate: &apitypes.ASTTranslateWorkflowSpec{
					TranslationModel: "ast-model",
					Mode:             astModePtr(apitypes.ASTTranslateModeS2s),
					SpeakerId:        stringPtr("speaker-a"),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolveConfig() error = %v", err)
	}
	if resolved.ttsVoice != "volc-tenant:e2e-volc-tenant:voice-a" {
		t.Fatalf("ttsVoice = %q", resolved.ttsVoice)
	}
	for _, want := range []string{
		"mode=s2t",
		"source_language=zhen",
		"target_language=zhen",
		"enable_source_language_detect=true",
	} {
		if !strings.Contains(resolved.astPattern, want) {
			t.Fatalf("astPattern = %q, missing %q", resolved.astPattern, want)
		}
	}
	for _, notWant := range []string{"speaker_id=", "tts_voice="} {
		if strings.Contains(resolved.astPattern, notWant) {
			t.Fatalf("astPattern = %q, contains %q", resolved.astPattern, notWant)
		}
	}
}

func TestParseLanguagePairRejectsZhenForms(t *testing.T) {
	for _, pair := range []string{"zhen", "zhen/zhen", "zh/zhen", "zhen/en"} {
		if _, _, _, err := parseLanguagePair(pair); err == nil {
			t.Fatalf("parseLanguagePair(%q) succeeded, want error", pair)
		}
	}
	source, target, auto, err := parseLanguagePair("auto")
	if err != nil {
		t.Fatalf("parseLanguagePair(auto) error = %v", err)
	}
	if source != "zhen" || target != "zhen" || !auto {
		t.Fatalf("parseLanguagePair(auto) = %q, %q, %v", source, target, auto)
	}
	source, target, auto, err = parseLanguagePair("zh/en")
	if err != nil {
		t.Fatalf("parseLanguagePair(zh/en) error = %v", err)
	}
	if source != "zh" || target != "en" || auto {
		t.Fatalf("parseLanguagePair(zh/en) = %q, %q, %v", source, target, auto)
	}
	source, target, auto, err = parseLanguagePair("zh/jp")
	if err != nil {
		t.Fatalf("parseLanguagePair(zh/jp) error = %v", err)
	}
	if source != "zh" || target != "ja" || auto {
		t.Fatalf("parseLanguagePair(zh/jp) = %q, %q, %v", source, target, auto)
	}
}

func TestExternalVoiceTransformerForwardsASTTextAndTTSAudio(t *testing.T) {
	transformer := &scriptedTransformer{
		streams: []genx.Stream{
			streamFromChunks(
				&genx.MessageChunk{Ctrl: &genx.StreamCtrl{Label: "assistant"}, Part: genx.Text("bonjour")},
				&genx.MessageChunk{Ctrl: &genx.StreamCtrl{Label: "user"}, Part: genx.Text("ignored")},
			),
			streamFromChunks(
				&genx.MessageChunk{
					Ctrl: &genx.StreamCtrl{StreamID: "tts-1"},
					Part: &genx.Blob{MIMEType: "audio/mpeg; codec=mp3", Data: []byte{1, 2, 3}},
				},
			),
		},
	}
	agent := externalVoiceTransformer{
		Transformer: transformer,
		ASTPattern:  "model/ast?source_language=zh&target_language=en",
		TTSPattern:  "voice/voice-a",
	}
	out, err := agent.Transform(context.Background(), "ignored", emptyStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	chunks, err := collectStream(out)
	if err != nil {
		t.Fatalf("collectStream() error = %v", err)
	}
	if len(transformer.patterns) != 2 || transformer.patterns[0] != agent.ASTPattern || transformer.patterns[1] != agent.TTSPattern {
		t.Fatalf("patterns = %#v", transformer.patterns)
	}
	if len(chunks) != 3 {
		t.Fatalf("output chunks = %d, want 3", len(chunks))
	}
	var sawASTText bool
	var sawTTSAudio bool
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && string(text) == "bonjour" {
			sawASTText = true
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if ok && blob.MIMEType == "audio/mpeg; codec=mp3" && chunk.Ctrl != nil && chunk.Ctrl.Label == "assistant" && chunk.Ctrl.StreamID == "tts-1" {
			sawTTSAudio = true
		}
	}
	if !sawASTText {
		t.Fatalf("output chunks missing AST text: %#v", chunks)
	}
	if !sawTTSAudio {
		t.Fatalf("output chunks missing labeled TTS audio: %#v", chunks)
	}
}

func TestForwardASTTranslateTTSDecodesOggOpus(t *testing.T) {
	raw := marshalOggPackets(t,
		[]byte("OpusHead\x01\x02"),
		[]byte("OpusTags\x01\x02"),
		[]byte{0xaa, 0xbb},
	)
	input := streamFromChunks(&genx.MessageChunk{
		Ctrl: &genx.StreamCtrl{StreamID: "ogg-1", EndOfStream: true},
		Part: &genx.Blob{MIMEType: "audio/ogg; codecs=opus", Data: raw},
	})
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 4)
	if err := forwardASTTranslateTTS(context.Background(), input, output); err != nil {
		t.Fatalf("forwardASTTranslateTTS() error = %v", err)
	}
	if err := output.Done(genx.Usage{}); err != nil {
		t.Fatalf("output.Done() error = %v", err)
	}
	chunks, err := collectStream(output.Stream())
	if err != nil {
		t.Fatalf("collectStream() error = %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("output chunks = %d, want opus frame and EOS", len(chunks))
	}
	frame, ok := chunks[0].Part.(*genx.Blob)
	if !ok || frame.MIMEType != "audio/opus" || string(frame.Data) != string([]byte{0xaa, 0xbb}) {
		t.Fatalf("frame chunk = %#v", chunks[0].Part)
	}
	eos, ok := chunks[1].Part.(*genx.Blob)
	if !ok || eos.MIMEType != "audio/opus" || len(eos.Data) != 0 || !chunks[1].IsEndOfStream() {
		t.Fatalf("EOS chunk = %#v ctrl=%#v", chunks[1].Part, chunks[1].Ctrl)
	}
}

func TestASTTranslateOggOpusFrameDecoder(t *testing.T) {
	raw := marshalOggPackets(t,
		[]byte("OpusHead\x01\x02"),
		[]byte("OpusTags\x01\x02"),
		[]byte{0x11, 0x22, 0x33},
	)
	decoder := newASTTranslateOggOpusFrameDecoder()
	frames, err := decoder.Write(raw[:10])
	if err != nil {
		t.Fatalf("Write(partial) error = %v", err)
	}
	if len(frames) != 0 {
		t.Fatalf("partial frames = %#v, want none", frames)
	}
	frames, err = decoder.Write(raw[10:])
	if err != nil {
		t.Fatalf("Write(rest) error = %v", err)
	}
	if len(frames) != 1 || string(frames[0]) != string([]byte{0x11, 0x22, 0x33}) {
		t.Fatalf("frames = %#v", frames)
	}
	if err := decoder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := newASTTranslateOggOpusFrameDecoder().Write([]byte("bad")); err == nil {
		t.Fatalf("Write(bad prefix) succeeded, want error")
	}
	truncated := newASTTranslateOggOpusFrameDecoder()
	if _, err := truncated.Write([]byte("OggS")); err != nil {
		t.Fatalf("Write(truncated prefix) error = %v", err)
	}
	if err := truncated.Close(); err == nil {
		t.Fatalf("Close(truncated) succeeded, want error")
	}
}

func TestFactoryAndPatternErrors(t *testing.T) {
	if _, err := (Factory{}).NewAgent(context.Background(), agenthost.Spec{}); err == nil {
		t.Fatalf("Factory.NewAgent() without transformer succeeded, want error")
	}
	if _, err := (Factory{Transformer: recordingTransformer{}}).NewAgent(context.Background(), agenthost.Spec{}); err == nil {
		t.Fatalf("Factory.NewAgent() without AST spec succeeded, want error")
	}
	if _, err := (patternTransformer{}).Transform(context.Background(), "", emptyStream{}); err == nil {
		t.Fatalf("patternTransformer.Transform() without transformer succeeded, want error")
	}

	langPair := "zh/en"
	var workspaceParams apitypes.WorkspaceParameters
	if err := workspaceParams.FromASTTranslateWorkspaceParameters(apitypes.ASTTranslateWorkspaceParameters{LangPair: &langPair}); err != nil {
		t.Fatalf("FromASTTranslateWorkspaceParameters() error = %v", err)
	}
	pattern, err := resolvePattern(agenthost.Spec{
		Workspace: apitypes.Workspace{Parameters: &workspaceParams},
		Workflow:  astWorkflow("model-a", nil),
	})
	if err != nil {
		t.Fatalf("resolvePattern() error = %v", err)
	}
	if pattern != "model/model-a?source_language=zh&target_language=en" {
		t.Fatalf("resolvePattern() = %q", pattern)
	}
}

func TestMergeWorkspaceParamsInternalSpeaker(t *testing.T) {
	customSpeaker := true
	speechRate := 15
	var voice apitypes.ASTTranslateVoiceParameters
	if err := voice.FromASTTranslateInternalSpeakerParameters(apitypes.ASTTranslateInternalSpeakerParameters{
		SpeakerId:       "speaker-a",
		IsCustomSpeaker: &customSpeaker,
		TtsResourceId:   stringPtr("tts-resource"),
		SpeechRate:      &speechRate,
	}); err != nil {
		t.Fatalf("FromASTTranslateInternalSpeakerParameters() error = %v", err)
	}
	params := mergeWorkspaceParams(nil, apitypes.ASTTranslateWorkspaceParameters{
		LangPair:      stringPtr(" en / jp "),
		Voice:         &voice,
		Denoise:       &customSpeaker,
		SpeechRate:    &speechRate,
		SpeakerId:     stringPtr("speaker-b"),
		TtsResourceId: stringPtr("tts-resource-b"),
	})
	if err := normalizeLanguagePair(params, true); err != nil {
		t.Fatalf("normalizeLanguagePair() error = %v", err)
	}
	for _, want := range []string{
		"source_language=en",
		"target_language=ja",
		"speaker_id=speaker-b",
		"is_custom_speaker=true",
		"tts_resource_id=tts-resource-b",
		"speech_rate=15",
		"denoise=true",
	} {
		if !strings.Contains(appendPatternParams("model/demo", params), want) {
			t.Fatalf("params = %#v, missing %q", params, want)
		}
	}
}

func TestASTTranslateUtilityBranches(t *testing.T) {
	if got := voicePattern(" voice-a "); got != "voice/voice-a" {
		t.Fatalf("voicePattern(simple) = %q", got)
	}
	if got := voicePattern("volc-tenant:name:voice"); got != "voice/volc-tenant:name:voice" {
		t.Fatalf("voicePattern(colon) = %q", got)
	}
	if got := voicePattern("kind/name"); got != "kind/name" {
		t.Fatalf("voicePattern(path) = %q", got)
	}
	if got := baseMIME(" Audio/OGG ; codecs=opus "); got != "audio/ogg" {
		t.Fatalf("baseMIME() = %q", got)
	}
	if isAssistantTextChunk(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{Label: "assistant"}, Part: &genx.Blob{}}) {
		t.Fatalf("blob assistant chunk reported as assistant text")
	}
	if !isStreamDone(genx.ErrDone) || !isStreamDone(io.EOF) || isStreamDone(errors.New("boom")) {
		t.Fatalf("isStreamDone returned unexpected values")
	}
	if err := normalizeLanguagePair(nil, true); err == nil {
		t.Fatalf("normalizeLanguagePair(nil, required) succeeded, want error")
	}
	params := map[string]any{"lang_pair": " "}
	if err := normalizeLanguagePair(params, false); err != nil {
		t.Fatalf("normalizeLanguagePair(optional empty) error = %v", err)
	}
	if got := appendPatternParams("model/demo?existing=1", map[string]any{"bad": 1.2, "ok": true}); got != "model/demo?existing=1&ok=true" {
		t.Fatalf("appendPatternParams() = %q", got)
	}
}

type recordingTransformer struct{}

func (recordingTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 2)
	_ = builder.Add(&genx.MessageChunk{Part: genx.Text(pattern)})
	_ = builder.Done(genx.Usage{})
	return builder.Stream(), nil
}

type scriptedTransformer struct {
	patterns []string
	streams  []genx.Stream
}

func (t *scriptedTransformer) Transform(_ context.Context, pattern string, _ genx.Stream) (genx.Stream, error) {
	t.patterns = append(t.patterns, pattern)
	if len(t.streams) == 0 {
		return nil, io.EOF
	}
	stream := t.streams[0]
	t.streams = t.streams[1:]
	return stream, nil
}

type emptyStream struct{}

func (emptyStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }
func (emptyStream) Close() error                      { return nil }
func (emptyStream) CloseWithError(error) error        { return nil }

func streamFromChunks(chunks ...*genx.MessageChunk) genx.Stream {
	builder := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), len(chunks)+1)
	_ = builder.Add(chunks...)
	_ = builder.Done(genx.Usage{})
	return builder.Stream()
}

func collectStream(stream genx.Stream) ([]*genx.MessageChunk, error) {
	defer stream.Close()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				return chunks, nil
			}
			return nil, err
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
	}
}

func marshalOggPackets(t *testing.T, packets ...[]byte) []byte {
	t.Helper()
	var pages []*ogg.Page
	for i, packet := range packets {
		packetPages, err := ogg.BuildPacketPages(1, uint32(i), packet, uint64(i), i == 0, i == len(packets)-1)
		if err != nil {
			t.Fatalf("BuildPacketPages(%d): %v", i, err)
		}
		pages = append(pages, packetPages...)
	}
	raw, err := ogg.MarshalPages(pages)
	if err != nil {
		t.Fatalf("MarshalPages(): %v", err)
	}
	return raw
}

func astWorkflow(model string, voice *apitypes.ASTTranslateVoiceParameters) apitypes.WorkflowDocument {
	return apitypes.WorkflowDocument{
		Metadata: apitypes.WorkflowMetadata{Name: "ast"},
		Spec: apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverAstTranslate,
			AstTranslate: &apitypes.ASTTranslateWorkflowSpec{
				TranslationModel: model,
				Voice:            voice,
			},
		},
	}
}

func stringPtr(value string) *string {
	return &value
}

func astModePtr(value apitypes.ASTTranslateMode) *apitypes.ASTTranslateMode {
	return &value
}
