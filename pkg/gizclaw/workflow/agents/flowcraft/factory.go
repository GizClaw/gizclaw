package flowcraft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sdkworkspace "github.com/GizClaw/flowcraft/sdk/workspace"
	flowclaw "github.com/GizClaw/flowcraft/sdkx/claw"
	"gopkg.in/yaml.v3"

	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/agenthost"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/peergenx"
)

const Type = "flowcraft"

const (
	defaultInputStreamID = "audio"
	transcriptLabel      = "transcript"
	assistantLabel       = "assistant"
)

var clawModelRoles = []struct {
	settingKey string
	modelsKey  string
	required   bool
}{
	{settingKey: "generate_model", modelsKey: "chat", required: true},
	{settingKey: "extract_model", modelsKey: "extractor"},
	{settingKey: "embedding_model", modelsKey: "embedder"},
}

type Factory struct {
	GenX *peergenx.Service
}

func (f Factory) NewAgent(ctx context.Context, spec agenthost.Spec) (genx.Transformer, error) {
	if f.GenX == nil {
		return nil, fmt.Errorf("flowcraft: peergenx service is required")
	}
	cfg, err := parseWorkflowConfig(spec)
	if err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(spec.Runtime.LocalDir) == "" {
		return nil, fmt.Errorf("flowcraft: local workspace directory is required")
	}
	clawConfig, err := buildClawConfig(ctx, f.GenX, spec, cfg)
	if err != nil {
		return nil, err
	}
	if err := validateVoiceAdapterResources(ctx, f.GenX, cfg); err != nil {
		return nil, err
	}
	if err := writeClawConfig(spec.Runtime.LocalDir, clawConfig); err != nil {
		return nil, err
	}
	ws, err := sdkworkspace.NewLocalWorkspace(spec.Runtime.LocalDir)
	if err != nil {
		return nil, err
	}
	claw, err := flowclaw.New(ws)
	if err != nil {
		return nil, err
	}
	return &agent{
		transformers: f.GenX,
		claw:         realClaw{Claw: claw},
		asrModel:     cfg.Spec.VoiceAdapter.ASRModel,
		defaultVoice: cfg.Spec.VoiceAdapter.DefaultVoice,
		nodeVoices:   cfg.Spec.VoiceAdapter.NodeVoices,
	}, nil
}

type workflowConfig struct {
	Spec struct {
		Flowcraft    map[string]any     `json:"flowcraft"`
		VoiceAdapter voiceAdapterConfig `json:"voice_adapter"`
	} `json:"spec"`
}

type voiceAdapterConfig struct {
	ASRModel     string            `json:"asr_model"`
	DefaultVoice string            `json:"default_voice"`
	NodeVoices   map[string]string `json:"node_voices"`
}

func parseWorkflowConfig(spec agenthost.Spec) (workflowConfig, error) {
	data, err := json.Marshal(spec.Workflow)
	if err != nil {
		return workflowConfig{}, fmt.Errorf("flowcraft: encode workflow: %w", err)
	}
	var cfg workflowConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return workflowConfig{}, fmt.Errorf("flowcraft: decode workflow: %w", err)
	}
	cfg.Spec.VoiceAdapter.ASRModel = strings.TrimSpace(cfg.Spec.VoiceAdapter.ASRModel)
	cfg.Spec.VoiceAdapter.DefaultVoice = strings.TrimSpace(cfg.Spec.VoiceAdapter.DefaultVoice)
	for rawNodeID, voice := range cfg.Spec.VoiceAdapter.NodeVoices {
		nodeID := strings.TrimSpace(rawNodeID)
		voice = strings.TrimSpace(voice)
		delete(cfg.Spec.VoiceAdapter.NodeVoices, rawNodeID)
		if nodeID == "" || voice == "" {
			continue
		}
		cfg.Spec.VoiceAdapter.NodeVoices[nodeID] = voice
	}
	return cfg, nil
}

func (c workflowConfig) validate() error {
	if strings.TrimSpace(c.Spec.VoiceAdapter.ASRModel) == "" {
		return fmt.Errorf("flowcraft: voice_adapter.asr_model is required")
	}
	if strings.TrimSpace(c.Spec.VoiceAdapter.DefaultVoice) == "" {
		return fmt.Errorf("flowcraft: voice_adapter.default_voice is required")
	}
	return nil
}

type agent struct {
	transformers transformerProvider
	claw         clawClient
	asrModel     string
	defaultVoice string
	nodeVoices   map[string]string
}

type transformerProvider interface {
	Transformer() genx.Transformer
}

type clawClient interface {
	RoundTrip(flowclaw.Request) (clawResponse, error)
	CloseContext(context.Context) error
}

type clawResponse interface {
	Next() (flowclaw.Event, error)
}

type realClaw struct {
	Claw *flowclaw.Claw
}

func (c realClaw) RoundTrip(req flowclaw.Request) (clawResponse, error) {
	return c.Claw.RoundTrip(req)
}

func (c realClaw) CloseContext(ctx context.Context) error {
	if c.Claw == nil {
		return nil
	}
	return c.Claw.CloseContext(ctx)
}

func (a *agent) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	if a == nil {
		return nil, fmt.Errorf("flowcraft: agent is nil")
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	go a.run(ctx, input, output)
	return output.Stream(), nil
}

func (a *agent) run(ctx context.Context, input genx.Stream, output *genx.StreamBuilder) {
	defer func() {
		if a.claw != nil {
			_ = a.claw.CloseContext(context.Background())
		}
	}()
	if err := a.runTurn(ctx, input, output); err != nil {
		_ = output.Unexpected(genx.Usage{}, err)
		return
	}
	_ = output.Done(genx.Usage{})
}

func (a *agent) runTurn(ctx context.Context, input genx.Stream, output *genx.StreamBuilder) error {
	transcript, streamID, err := a.transcribeInputTurn(ctx, input, output)
	if err != nil {
		return err
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return fmt.Errorf("flowcraft: ASR produced empty transcript")
	}

	resp, err := a.claw.RoundTrip(flowclaw.Request{Context: ctx, Text: transcript})
	if err != nil {
		return fmt.Errorf("flowcraft: claw round trip: %w", err)
	}
	var nodeOrder []string
	ttsByNode := make(map[string]*ttsSession)
	for {
		ev, err := resp.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("flowcraft: read claw event: %w", err)
		}
		if ev.Type == flowclaw.EventError || ev.IsError {
			if ev.Err == "" {
				ev.Err = ev.Content
			}
			return fmt.Errorf("flowcraft: claw event error: %s", ev.Err)
		}
		if ev.Type != flowclaw.EventToken || ev.Content == "" {
			continue
		}
		nodeID := strings.TrimSpace(ev.NodeID)
		if nodeID == "" {
			nodeID = assistantLabel
		}
		if err := output.Add(textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, ev.Content, false)); err != nil {
			return err
		}
		tts := ttsByNode[nodeID]
		if tts == nil {
			nodeOrder = append(nodeOrder, nodeID)
			voice := a.voiceForNode(nodeID)
			tts, err = a.startTTS(ctx, streamID, nodeID, voice, output)
			if err != nil {
				return err
			}
			ttsByNode[nodeID] = tts
		}
		if err := tts.AddText(ev.Content); err != nil {
			return err
		}
	}
	if err := output.Add(textChunk(genx.RoleModel, assistantLabel, streamID, assistantLabel, "", true)); err != nil {
		return err
	}
	for _, nodeID := range nodeOrder {
		tts := ttsByNode[nodeID]
		if tts == nil {
			continue
		}
		if err := tts.CloseInput(); err != nil {
			return err
		}
		if err := tts.Wait(); err != nil {
			return err
		}
	}
	return nil
}

func (a *agent) transcribeInputTurn(ctx context.Context, input genx.Stream, output *genx.StreamBuilder) (string, string, error) {
	transformer := a.transformers.Transformer()
	asrInput := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	asr, err := transformer.Transform(ctx, "model/"+a.asrModel, asrInput.Stream())
	if err != nil {
		return "", "", fmt.Errorf("flowcraft: start ASR: %w", err)
	}
	defer func() { _ = asr.Close() }()

	streamIDState := &lockedString{value: defaultInputStreamID}
	feedDone := make(chan feedASRResult, 1)
	go func() {
		result := feedASRInput(ctx, input, asrInput, streamIDState)
		feedDone <- result
	}()

	var transcript string
	transcriptEOS := false
	for {
		chunk, err := asr.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) {
				break
			}
			result := <-feedDone
			if result.err != nil {
				return "", result.streamID, result.err
			}
			return "", result.streamID, fmt.Errorf("flowcraft: read ASR: %w", err)
		}
		text, ok := chunk.Part.(genx.Text)
		if chunk.IsEndOfStream() {
			transcriptEOS = true
			if text != "" {
				part := string(text)
				transcript = mergeTranscript(transcript, part)
				if err := output.Add(textChunk(genx.RoleUser, transcriptLabel, streamIDState.Get(), transcriptLabel, part, false)); err != nil {
					return "", streamIDState.Get(), err
				}
			}
			if err := output.Add(textChunk(genx.RoleUser, transcriptLabel, streamIDState.Get(), transcriptLabel, "", true)); err != nil {
				return "", streamIDState.Get(), err
			}
			continue
		}
		if !ok || text == "" {
			continue
		}
		part := string(text)
		transcript = mergeTranscript(transcript, part)
		if err := output.Add(textChunk(genx.RoleUser, transcriptLabel, streamIDState.Get(), transcriptLabel, part, false)); err != nil {
			return "", streamIDState.Get(), err
		}
	}
	result := <-feedDone
	if result.err != nil {
		return "", result.streamID, result.err
	}
	if result.streamID == "" {
		result.streamID = defaultInputStreamID
	}
	if !transcriptEOS {
		if err := output.Add(textChunk(genx.RoleUser, transcriptLabel, result.streamID, transcriptLabel, "", true)); err != nil {
			return "", result.streamID, err
		}
	}
	return transcript, result.streamID, nil
}

func (a *agent) synthesize(ctx context.Context, streamID, nodeID, voice, text string, output *genx.StreamBuilder) error {
	transformer := a.transformers.Transformer()
	input := []*genx.MessageChunk{
		textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, text, false),
		textChunk(genx.RoleModel, nodeID, streamID, assistantLabel, "", true),
	}
	tts, err := transformer.Transform(ctx, "voice/"+voice, &sliceStream{chunks: input})
	if err != nil {
		return fmt.Errorf("flowcraft: start TTS voice %q: %w", voice, err)
	}
	defer func() { _ = tts.Close() }()
	return drainTTSOutput(streamID, nodeID, voice, tts, output)
}

type ttsSession struct {
	input    *genx.StreamBuilder
	done     chan error
	streamID string
	nodeID   string
}

func (a *agent) startTTS(ctx context.Context, streamID, nodeID, voice string, output *genx.StreamBuilder) (*ttsSession, error) {
	transformer := a.transformers.Transformer()
	input := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 64)
	tts, err := transformer.Transform(ctx, "voice/"+voice, input.Stream())
	if err != nil {
		return nil, fmt.Errorf("flowcraft: start TTS voice %q: %w", voice, err)
	}
	session := &ttsSession{
		input:    input,
		done:     make(chan error, 1),
		streamID: streamID,
		nodeID:   nodeID,
	}
	go func() {
		defer func() { _ = tts.Close() }()
		session.done <- drainTTSOutput(streamID, nodeID, voice, tts, output)
	}()
	return session, nil
}

func (s *ttsSession) AddText(text string) error {
	if s == nil {
		return fmt.Errorf("flowcraft: TTS session is nil")
	}
	return s.input.Add(textChunk(genx.RoleModel, s.nodeID, s.streamID, assistantLabel, text, false))
}

func (s *ttsSession) CloseInput() error {
	if s == nil {
		return nil
	}
	if err := s.input.Add(textChunk(genx.RoleModel, s.nodeID, s.streamID, assistantLabel, "", true)); err != nil {
		return err
	}
	return s.input.Done(genx.Usage{})
}

func (s *ttsSession) Wait() error {
	if s == nil {
		return nil
	}
	return <-s.done
}

func drainTTSOutput(streamID, nodeID, voice string, tts genx.Stream, output *genx.StreamBuilder) error {
	var oggAudio bytes.Buffer
	for {
		chunk, err := tts.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) {
				break
			}
			return fmt.Errorf("flowcraft: read TTS voice %q: %w", voice, err)
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || len(blob.Data) == 0 {
			continue
		}
		switch baseMIME(blob.MIMEType) {
		case "audio/opus":
			if err := output.Add(audioChunk(nodeID, streamID, blob.Data, false)); err != nil {
				return err
			}
		case "audio/ogg", "application/ogg":
			_, _ = oggAudio.Write(blob.Data)
		default:
			return fmt.Errorf("flowcraft: unsupported TTS audio MIME %q; want audio/ogg or audio/opus", blob.MIMEType)
		}
	}
	if oggAudio.Len() > 0 {
		frames, err := opusFramesFromOgg(oggAudio.Bytes())
		if err != nil {
			return fmt.Errorf("flowcraft: decode TTS ogg opus: %w", err)
		}
		for _, frame := range frames {
			if err := output.Add(audioChunk(nodeID, streamID, frame, false)); err != nil {
				return err
			}
		}
	}
	return output.Add(audioChunk(nodeID, streamID, nil, true))
}

func (a *agent) voiceForNode(nodeID string) string {
	if a.nodeVoices != nil {
		if voice := strings.TrimSpace(a.nodeVoices[nodeID]); voice != "" {
			return voice
		}
	}
	return a.defaultVoice
}

type feedASRResult struct {
	streamID string
	err      error
}

func feedASRInput(ctx context.Context, input genx.Stream, asrInput *genx.StreamBuilder, streamIDState *lockedString) feedASRResult {
	streamID := defaultInputStreamID
	fail := func(err error) feedASRResult {
		_ = asrInput.Unexpected(genx.Usage{}, err)
		return feedASRResult{streamID: streamID, err: err}
	}
	if input == nil {
		return fail(fmt.Errorf("flowcraft: input stream is required"))
	}

	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		chunk, err := input.Next()
		if err != nil {
			if agenthost.IsStreamDone(err) || errors.Is(err, io.EOF) {
				if err := asrInput.Done(genx.Usage{}); err != nil {
					return feedASRResult{streamID: streamID, err: err}
				}
				return feedASRResult{streamID: streamID}
			}
			return fail(err)
		}
		if chunk == nil {
			continue
		}
		if chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
			streamID = chunk.Ctrl.StreamID
			streamIDState.Set(streamID)
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && isAudioMIME(blob.MIMEType) && len(blob.Data) > 0 {
			if err := asrInput.Add(chunk.Clone()); err != nil {
				return feedASRResult{streamID: streamID, err: err}
			}
		}
		if chunk.IsEndOfStream() {
			eos := chunk.Clone()
			if _, ok := eos.Part.(*genx.Blob); !ok {
				eos.Part = &genx.Blob{MIMEType: "audio/pcm"}
			}
			if eos.Ctrl == nil {
				eos.Ctrl = &genx.StreamCtrl{}
			}
			if eos.Ctrl.StreamID == "" {
				eos.Ctrl.StreamID = streamID
			}
			eos.Ctrl.EndOfStream = true
			if err := asrInput.Add(eos); err != nil {
				return feedASRResult{streamID: streamID, err: err}
			}
			if err := asrInput.Done(genx.Usage{}); err != nil {
				return feedASRResult{streamID: streamID, err: err}
			}
			return feedASRResult{streamID: streamID}
		}
	}
}

type lockedString struct {
	mu    sync.RWMutex
	value string
}

func (s *lockedString) Set(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = value
}

func (s *lockedString) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.value
}

func textChunk(role genx.Role, name, streamID, label, text string, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: role,
		Name: name,
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: eos},
	}
}

func audioChunk(name, streamID string, data []byte, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: genx.RoleModel,
		Name: name,
		Part: &genx.Blob{MIMEType: "audio/opus", Data: data},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: assistantLabel, EndOfStream: eos},
	}
}

func mergeTranscript(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" || next == "" {
		if current != "" {
			return current
		}
		return next
	}
	if strings.HasPrefix(next, current) {
		return next
	}
	if strings.HasPrefix(current, next) {
		return current
	}
	return current + next
}

func opusFramesFromOgg(raw []byte) ([][]byte, error) {
	var frames [][]byte
	for packet, err := range ogg.Packets(bytes.NewReader(raw)) {
		if err != nil {
			return nil, err
		}
		if codecconv.IsOpusHeadPacket(packet.Data) || codecconv.IsOpusTagsPacket(packet.Data) || len(packet.Data) == 0 {
			continue
		}
		frames = append(frames, append([]byte(nil), packet.Data...))
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no opus audio packets found")
	}
	return frames, nil
}

func baseMIME(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

func isAudioMIME(mimeType string) bool {
	return strings.HasPrefix(baseMIME(mimeType), "audio/")
}

type sliceStream struct {
	chunks []*genx.MessageChunk
	err    error
}

func (s *sliceStream) Next() (*genx.MessageChunk, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.chunks) == 0 {
		return nil, genx.Done(genx.Usage{})
	}
	chunk := s.chunks[0]
	s.chunks = s.chunks[1:]
	return chunk, nil
}

func (s *sliceStream) Close() error {
	s.chunks = nil
	return nil
}

func (s *sliceStream) CloseWithError(err error) error {
	s.err = err
	s.chunks = nil
	return nil
}

func writeClawConfig(root string, cfg map[string]any) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("flowcraft: create workspace dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("flowcraft: encode claw config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), data, 0o600); err != nil {
		return fmt.Errorf("flowcraft: write claw config: %w", err)
	}
	return nil
}

func buildClawConfig(ctx context.Context, genxService *peergenx.Service, spec agenthost.Spec, cfg workflowConfig) (map[string]any, error) {
	out := deepCopyMap(cfg.Spec.Flowcraft)
	if out == nil {
		out = map[string]any{}
	}
	var workspaceParams *apitypes.FlowcraftWorkspaceParameters
	if spec.Workspace.Parameters != nil {
		typed, err := spec.Workspace.Parameters.AsFlowcraftWorkspaceParameters()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode workspace parameters: %w", err)
		}
		workspaceParams = &typed
	}
	settings := ensureMap(out, "settings")
	models := ensureMap(out, "models")
	llm := ensureMap(models, "llm")
	var accessibleModels map[string]peergenx.GeneratorConfig
	for _, role := range clawModelRoles {
		modelID, ok, err := modelIDForRole(workspaceParams, out, role.settingKey, role.required)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if accessibleModels == nil {
			accessibleModels, err = accessibleGeneratorModels(ctx, genxService)
			if err != nil {
				return nil, err
			}
		}
		generatorCfg, ok := accessibleModels[modelID]
		if !ok {
			return nil, fmt.Errorf("flowcraft: model %q is not accessible as a generator", modelID)
		}
		modelCfg, err := clawModelConfig(generatorCfg)
		if err != nil {
			return nil, err
		}
		settings[role.settingKey] = modelID
		models[role.modelsKey] = role.settingKey
		llm[modelID] = modelCfg
	}
	ensureDefaultAgent(out)
	return out, nil
}

func accessibleGeneratorModels(ctx context.Context, genxService *peergenx.Service) (map[string]peergenx.GeneratorConfig, error) {
	if genxService == nil {
		return nil, fmt.Errorf("flowcraft: peergenx service is required")
	}
	configs, err := genxService.ListAccessibleGeneratorConfigs(ctx)
	if err != nil {
		return nil, fmt.Errorf("flowcraft: list accessible generator models: %w", err)
	}
	out := make(map[string]peergenx.GeneratorConfig, len(configs))
	for _, cfg := range configs {
		out[string(cfg.Model.Id)] = cfg
	}
	return out, nil
}

func validateVoiceAdapterResources(ctx context.Context, genxService *peergenx.Service, cfg workflowConfig) error {
	if genxService == nil {
		return fmt.Errorf("flowcraft: peergenx service is required")
	}
	if _, err := genxService.ResolveTransformer(ctx, "model/"+cfg.Spec.VoiceAdapter.ASRModel); err != nil {
		return fmt.Errorf("flowcraft: resolve ASR model %q: %w", cfg.Spec.VoiceAdapter.ASRModel, err)
	}
	voices := make([]string, 0, len(cfg.Spec.VoiceAdapter.NodeVoices)+1)
	seen := map[string]bool{}
	addVoice := func(voice string) {
		voice = strings.TrimSpace(voice)
		if voice == "" || seen[voice] {
			return
		}
		seen[voice] = true
		voices = append(voices, voice)
	}
	addVoice(cfg.Spec.VoiceAdapter.DefaultVoice)
	for _, voice := range cfg.Spec.VoiceAdapter.NodeVoices {
		addVoice(voice)
	}
	for _, voice := range voices {
		if _, err := genxService.ResolveTransformer(ctx, "voice/"+voice); err != nil {
			return fmt.Errorf("flowcraft: resolve voice %q: %w", voice, err)
		}
	}
	return nil
}

func modelIDForRole(parameters *apitypes.FlowcraftWorkspaceParameters, cfg map[string]any, key string, required bool) (string, bool, error) {
	if parameters != nil {
		var value *string
		switch key {
		case "generate_model":
			value = parameters.GenerateModel
		case "extract_model":
			value = parameters.ExtractModel
		case "embedding_model":
			value = parameters.EmbeddingModel
		}
		if value != nil {
			if text := strings.TrimSpace(*value); text != "" {
				return strings.TrimSpace(text), true, nil
			}
		}
	}
	if settings, ok := cfg["settings"].(map[string]any); ok {
		if text, ok := settings[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), true, nil
		}
	}
	if required {
		return "", false, fmt.Errorf("flowcraft: %s is required", key)
	}
	return "", false, nil
}

func clawModelConfig(cfg peergenx.GeneratorConfig) (map[string]any, error) {
	switch cfg.Tenant.Kind {
	case string(apitypes.ModelProviderKindOpenaiTenant):
		return resolveOpenAIClawModelConfig(cfg)
	case string(apitypes.ModelProviderKindVolcTenant):
		return resolveVolcClawModelConfig(cfg)
	default:
		return nil, fmt.Errorf("flowcraft: model %q provider %q is not supported by claw config generation", cfg.Model.Id, cfg.Tenant.Kind)
	}
}

func resolveOpenAIClawModelConfig(cfg peergenx.GeneratorConfig) (map[string]any, error) {
	if cfg.Tenant.OpenAI == nil {
		return nil, fmt.Errorf("flowcraft: openai tenant is required")
	}
	var providerData apitypes.OpenAITenantModelProviderData
	if cfg.Model.ProviderData != nil {
		var err error
		providerData, err = cfg.Model.ProviderData.AsOpenAITenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode openai provider_data: %w", err)
		}
	}
	upstream := firstString(providerData.UpstreamModel, string(cfg.Model.Id))
	body, err := cfg.Credential.Body.AsOpenAICredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ApiKey, body.Token)
	if apiKey == "" {
		return nil, fmt.Errorf("flowcraft: credential %q missing api_key", cfg.Credential.Name)
	}
	out := map[string]any{
		"provider": "openai",
		"model":    upstream,
		"api_key":  apiKey,
	}
	if extra := openAIThinkingConfig(providerData); len(extra) > 0 {
		out["config"] = extra
	}
	if baseURL := firstString(cfg.Tenant.OpenAI.BaseUrl, body.BaseUrl); baseURL != "" {
		out["base_url"] = baseURL
	}
	return out, nil
}

func resolveVolcClawModelConfig(cfg peergenx.GeneratorConfig) (map[string]any, error) {
	if cfg.Tenant.Volc == nil {
		return nil, fmt.Errorf("flowcraft: volc tenant is required")
	}
	var providerData apitypes.VolcTenantModelProviderData
	if cfg.Model.ProviderData != nil {
		var err error
		providerData, err = cfg.Model.ProviderData.AsVolcTenantModelProviderData()
		if err != nil {
			return nil, fmt.Errorf("flowcraft: decode volc provider_data: %w", err)
		}
	}
	model := firstString(providerData.UpstreamModel, string(cfg.Model.Id))
	if model == "" {
		return nil, fmt.Errorf("flowcraft: model %q missing upstream model", cfg.Model.Id)
	}
	body, err := cfg.Credential.Body.AsVolcCredentialBody()
	if err != nil {
		return nil, err
	}
	apiKey := firstString(body.ArkApiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("flowcraft: credential %q missing ark_api_key", cfg.Credential.Name)
	}
	out := map[string]any{
		"provider": "bytedance",
		"model":    model,
		"api_key":  apiKey,
	}
	if extra := volcThinkingConfig(providerData); len(extra) > 0 {
		out["config"] = extra
	}
	if baseURL := firstString(cfg.Tenant.Volc.Endpoint); baseURL != "" {
		out["base_url"] = baseURL
	}
	if region := firstString(cfg.Tenant.Volc.Region); region != "" {
		out["region"] = region
	}
	return out, nil
}

func openAIThinkingConfig(data apitypes.OpenAITenantModelProviderData) map[string]any {
	param := firstString(data.ThinkingParam, data.ThinkingLevelParam)
	level := firstString(data.DefaultThinkingLevel)
	if param == "" || level == "" {
		return nil
	}
	out := map[string]any{}
	setNestedConfigValue(out, param, openAIThinkingConfigValue(param, level))
	return out
}

func volcThinkingConfig(data apitypes.VolcTenantModelProviderData) map[string]any {
	param := firstString(data.ThinkingParam, data.ThinkingLevelParam)
	level := firstString(data.DefaultThinkingLevel)
	if param == "" || level == "" {
		return nil
	}
	out := map[string]any{}
	setNestedConfigValue(out, param, openAIThinkingConfigValue(param, level))
	return out
}

func openAIThinkingConfigValue(param, level string) any {
	if strings.EqualFold(strings.TrimSpace(param), "enable_thinking") {
		return !isDisabledThinkingLevel(level)
	}
	return level
}

func isDisabledThinkingLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "disabled", "disable", "off", "false", "0", "none", "no":
		return true
	default:
		return false
	}
}

func setNestedConfigValue(out map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return
	}
	current := out
	for _, raw := range parts[:len(parts)-1] {
		part := strings.TrimSpace(raw)
		if part == "" {
			return
		}
		next, _ := current[part].(map[string]any)
		if next == nil {
			next = map[string]any{}
			current[part] = next
		}
		current = next
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last != "" {
		current[last] = value
	}
}

func ensureDefaultAgent(cfg map[string]any) {
	agent := ensureMap(cfg, "agent")
	if _, ok := agent["id"]; !ok {
		agent["id"] = "claw"
	}
	if _, ok := agent["name"]; !ok {
		agent["name"] = "Claw"
	}
	if _, ok := agent["model"]; !ok {
		agent["model"] = "generate_model"
	}
	if _, ok := agent["system_prompt"]; !ok {
		agent["system_prompt"] = "你是一个简短、自然的中文语音聊天助手。"
	}
	if _, ok := agent["max_iterations"]; !ok {
		agent["max_iterations"] = 8
	}
}

func ensureMap(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}
	if existing, ok := values[key].(map[string]any); ok {
		return existing
	}
	next := map[string]any{}
	values[key] = next
	return next
}

func deepCopyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	data, err := json.Marshal(values)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func mapString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case *string:
			if typed != nil && strings.TrimSpace(*typed) != "" {
				return strings.TrimSpace(*typed)
			}
		}
	}
	return ""
}
