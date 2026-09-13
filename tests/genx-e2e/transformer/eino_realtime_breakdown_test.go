//go:build gizclaw_genx_e2e

package transformer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoasr"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	einofactory "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/eino"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"sigs.k8s.io/yaml"
)

// Run from the repository root:
//
// go test -tags gizclaw_genx_e2e -count=1 -run TestEinoRealtimeFirstResponseBreakdown -v ./tests/genx-e2e/transformer/
//
// Uses loadGenXE2EEnv's existing tests/genx-e2e/.env contract, plus
// GIZCLAW_GENX_E2E_VOLC_ARK_API_KEY (Ark, not the speech API key).
// This is a measurement, with no latency or transcript-accuracy gate. It uses
// the production Factory, graph YAML, model YAML, resolver and provider builder;
// resource getters replace catalog storage only. No WebRTC, deployed history,
// memory backend or client playback is included. Each trial has a fresh agent.
func TestEinoRealtimeFirstResponseBreakdown(t *testing.T) {
	runEinoBreakdown(t, false)
}

// TestEinoRealtimeConcurrentBreakdown measures ten fresh sessions with a shared
// playback deadline. Run with -run '^TestEinoRealtimeConcurrentBreakdown$'
// -parallel 10 -timeout 3m and the build tag/environment documented above.
func TestEinoRealtimeConcurrentBreakdown(t *testing.T) {
	parallel, err := strconv.Atoi(flag.Lookup("test.parallel").Value.String())
	if err != nil || parallel < 10 {
		t.Fatal("concurrent breakdown requires -parallel 10 or greater")
	}
	runEinoBreakdown(t, true)
}

func runEinoBreakdown(t *testing.T, concurrent bool) {
	loadGenXE2EEnv(t)
	arkKey := firstEnv("GIZCLAW_GENX_E2E_VOLC_ARK_API_KEY")
	if arkKey == "" {
		t.Fatal("set GIZCLAW_GENX_E2E_VOLC_ARK_API_KEY in tests/genx-e2e/.env")
	}
	resources := breakdownResources{models: map[string]apitypes.Model{}}
	for alias, file := range map[string]string{"asr": "02-volc-asr.yaml", "llm": "04-doubao-mini-chat.yaml"} {
		var m apitypes.Model
		breakdownResource(t, "03-models/"+file, &m)
		resources.models[alias] = m
	}
	var spec apitypes.WorkflowSpec
	breakdownResource(t, "04-workflows/31-eino-concurrency.yaml", &spec)
	resources.credential.Id = "breakdown-credential"
	if err := resources.credential.Body.FromVolcCredentialBody(apitypes.VolcCredentialBody{
		ArkApiKey: &arkKey, SpeechAppId: new(firstEnv(doubaoAppIDEnv)), SpeechApiKey: new(firstEnv(doubaoAPIKeyEnv)),
	}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"id":"volc-tenant:volc-main:zh_female_vv_uranus_bigtts","provider":{"kind":"volc-tenant","id":"volc-main"},"provider_data":{"voice_id":"zh_female_vv_uranus_bigtts"}}`), &resources.voice); err != nil {
		t.Fatal(err)
	}
	service := peergenx.New(peergenx.Service{Models: resources, Voices: resources, Credentials: resources, ProviderTenants: resources})
	// Synthesize once, outside all measured trials; every group replays identical PCM.
	synth, err := service.BuildTransformer(t.Context(), "voice/assistant-voice")
	if err != nil {
		t.Fatal(err)
	}
	encoded := collectTTSAudioE2E(t, synth, "breakdown-fixture", "Giztest audio input", "audio/ogg")
	var pcm bytes.Buffer
	if _, err := codecconv.OggToPCM(&pcm, bytes.NewReader(encoded), opus.SampleRate16K); err != nil {
		t.Fatal(err)
	}
	if pcm.Len() == 0 {
		t.Fatal("empty synthesized input")
	}
	t.Logf("input=%q pcm_bytes=%d duration=%s; speech_end=last speech frame pushed; trailing synthesis silence retained", "Giztest audio input", pcm.Len(), time.Duration(pcm.Len())*time.Second/32000)
	t.Log("queue snapshot includes all PCM offered before SDK open returned: RealtimeStream, audiodock and adapter-held first frame; drain is the successful send crossing that fixed byte watermark (packet rounding included), not an empty live queue or server acknowledgement")
	t.Log("boundaries are local provider-adapter reads; signed durations allow partial/EOS before speech_end; n/a means unavailable; first_text_out is Factory output, not client receipt")
	if concurrent {
		// Parallel subtests start after this parent returns. Each owns its agent,
		// streams and probes; fixture synthesis is intentionally outside the cohort.
		start := make(chan struct{})
		ready := make(chan struct{}, 10)
		finished := make(chan struct{})
		t.Cleanup(func() { close(finished) })
		go func() {
			for range 10 {
				select {
				case <-ready:
				case <-finished:
					return
				}
			}
			close(start)
		}()
		for lane := range 10 {
			t.Run(fmt.Sprintf("lane_%02d", lane+1), func(t *testing.T) {
				t.Parallel()
				stamps := &breakdownStamps{origin: time.Now(), speechReady: make(chan struct{}), start: start, ready: ready}
				trial := *service
				trial.Builder = breakdownBuilder{stamps: stamps, production: true}
				transcript := runBreakdownTrial(t, &trial, spec, pcm.Bytes(), stamps)
				t.Logf("recognized=%q", transcript)
			})
		}
		return
	}
	// EndWindowSize is milliseconds, minimum 200 (SDK ASRV2RequestConfig).
	// The production builder supplies its defaults (500/1000) when all endpoint fields
	// are absent. Keep that separate from the SDK's nil/provider semantic default.
	groups := []struct {
		name       string
		window     *int
		production bool
	}{
		{name: "provider_default"}, {name: "end_window_500ms", window: new(500)},
		{name: "end_window_300ms", window: new(300)}, {name: "production_default", production: true},
	}
	baseline := ""
	for round := range 5 {
		// Rotate serial order to reduce systematic provider warm-up/time drift bias.
		for offset := range groups {
			group := groups[(round+offset)%len(groups)]
			t.Run(fmt.Sprintf("%s/round_%d", group.name, round+1), func(t *testing.T) {
				stamps := &breakdownStamps{origin: time.Now(), speechReady: make(chan struct{})}
				builder := breakdownBuilder{stamps: stamps, window: group.window, production: group.production}
				trialService := *service
				trialService.Builder = builder
				transcript := runBreakdownTrial(t, &trialService, spec, pcm.Bytes(), stamps)
				if baseline == "" && group.name == "provider_default" {
					baseline = transcript
				}
				normalized, reference := breakdownNormalize(transcript), breakdownNormalize(baseline)
				t.Logf("recognized=%q default_reference=%q changed=%t possible_truncation=%t (shorter normalized text; heuristic, inspect transcript)", transcript, baseline, normalized != reference, len([]rune(normalized)) < len([]rune(reference)))
			})
		}
	}
}

func runBreakdownTrial(t *testing.T, service *peergenx.Service, workflow apitypes.WorkflowSpec, pcm []byte, stamps *breakdownStamps) string {
	t.Helper()
	defer stamps.arrive()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	var parameters apitypes.WorkspaceParameters
	if err := json.Unmarshal([]byte(`{"input":"realtime"}`), &parameters); err != nil {
		t.Fatal(err)
	}
	agent, err := (einofactory.Factory{GenX: service}).NewAgent(ctx, agenthost.Spec{
		Workspace: apitypes.Workspace{Id: "breakdown-" + t.Name(), Name: "breakdown", Parameters: &parameters},
		Workflow:  apitypes.Workflow{Id: "eino-concurrency-assistant", Spec: workflow},
	})
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := agent.(io.Closer); ok {
		defer closer.Close()
	}
	stamps.mark(&stamps.streamOpen)
	input := genx.NewRealtimeStream() // Preserve production reorder delay.
	defer input.Close()
	output, err := agent.Transform(ctx, &breakdownTap{Stream: input, observe: func(c *genx.MessageChunk) {
		if b, ok := c.Part.(*genx.Blob); ok && b != nil && len(b.Data) > 0 {
			stamps.mark(&stamps.reordered)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	stop := context.AfterFunc(ctx, func() { _ = input.CloseWithError(ctx.Err()); _ = output.CloseWithError(ctx.Err()) })
	defer stop()
	feedCtx, stopFeed := context.WithCancel(ctx)
	feedDone := make(chan error, 1)
	go func() {
		if stamps.start != nil {
			stamps.arrive()
			select {
			case <-stamps.start:
			case <-feedCtx.Done():
				feedDone <- feedCtx.Err()
				return
			}
		}
		feedDone <- breakdownFeed(feedCtx, input, pcm, stamps)
	}()
	defer func() {
		stopFeed()
		if err := <-feedDone; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("feed: %v", err)
		}
	}()
	defer func() {
		t.Logf("stream_open→asr_session_open=%s asr_dial=%s first_audio_in→first_audio_sent_to_asr=%s first_audio_in→asr_dial_start=%s queued_before_open_bytes=%d queued_audio=%s open→backlog_sent=%s sent_bytes=%d",
			stamps.delta(&stamps.streamOpen, &stamps.sessionOpen), stamps.delta(&stamps.dialStart, &stamps.sessionOpen), stamps.delta(&stamps.firstInput, &stamps.firstSent), stamps.delta(&stamps.firstInput, &stamps.dialStart), stamps.queued.Load(), time.Duration(stamps.queued.Load())*time.Second/32000, stamps.delta(&stamps.sessionOpen, &stamps.drained), stamps.sent.Load())
		t.Logf("first_audio_in→realtime_dequeue=%s realtime_dequeue→asr_dial_start=%s", stamps.delta(&stamps.firstInput, &stamps.reordered), stamps.delta(&stamps.reordered, &stamps.dialStart))
	}()
	var text strings.Builder
	audioBytes := 0
	defer func() {
		t.Logf("speech_end→asr_first_partial=%s speech_end→asr_definite(EOS)=%s asr_definite→llm_first_token=%s llm_first_token→first_text_out=%s first_text_out→tts_first_audio=%s speech_end→first_text=%s speech_end→first_audio=%s tts_audio→output_audio=%s response=%q audio_bytes=%d",
			stamps.delta(&stamps.speechEnd, &stamps.partial), stamps.delta(&stamps.speechEnd, &stamps.definite), stamps.delta(&stamps.definite, &stamps.token), stamps.delta(&stamps.token, &stamps.text), stamps.delta(&stamps.text, &stamps.ttsAudio), stamps.delta(&stamps.speechEnd, &stamps.text), stamps.delta(&stamps.speechEnd, &stamps.audio), stamps.delta(&stamps.ttsAudio, &stamps.audio), text.String(), audioBytes)
	}()
	// Read through both response EOS events so the following trial does not
	// overlap a still-generating LLM/TTS request from this one.
	textDone, audioDone := false, false
	for !textDone || !audioDone {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("read response: %v (text=%q audio_bytes=%d)", err, text.String(), audioBytes)
		}
		if err := speechChunkError(chunk); err != nil {
			t.Fatal(err)
		}
		if chunk == nil || chunk.Role != genx.RoleModel {
			continue
		}
		switch part := chunk.Part.(type) {
		case genx.Text:
			if len(part) > 0 {
				stamps.mark(&stamps.text)
				text.WriteString(string(part))
			}
			textDone = textDone || chunk.IsEndOfStream()
		case *genx.Blob:
			if part != nil && strings.HasPrefix(part.MIMEType, "audio/") {
				if len(part.Data) > 0 {
					stamps.mark(&stamps.audio)
					audioBytes += len(part.Data)
				}
				audioDone = audioDone || chunk.IsEndOfStream()
			}
		}
	}
	select {
	case <-stamps.speechReady:
	case <-ctx.Done():
		t.Fatalf("finish input playback: %v", ctx.Err())
	}
	if strings.TrimSpace(text.String()) == "" || audioBytes == 0 {
		t.Fatalf("empty response: text=%q audio_bytes=%d", text.String(), audioBytes)
	}

	transcript := stamps.transcript.Load()
	if transcript == nil {
		return ""
	}
	return *transcript
}

func breakdownFeed(ctx context.Context, input *genx.RealtimeStream, pcm []byte, stamps *breakdownStamps) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for frame := 0; ; frame++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		data := make([]byte, 640) // 16 kHz, mono, signed 16-bit PCM, then live silence.
		offset := frame * 640
		if offset < len(pcm) {
			copy(data, pcm[offset:min(offset+640, len(pcm))])
		}
		stamps.mark(&stamps.firstInput)
		stamps.inputBytes.Add(int64(len(data)))
		err := input.Push(ctx, &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/pcm", Data: data}, Ctrl: &genx.StreamCtrl{StreamID: "microphone", BeginOfStream: frame == 0, Timestamp: time.Now().UnixMilli()}})
		if err != nil {
			return err
		}
		if offset < len(pcm) && offset+640 >= len(pcm) {
			stamps.mark(&stamps.speechEnd)
			close(stamps.speechReady)
		}
		// Never send input EOS: that would measure forced finalization, not VAD.
	}
}

type breakdownStamps struct {
	arrival                                                                       sync.Once
	start                                                                         <-chan struct{}
	ready                                                                         chan<- struct{}
	reordered, streamOpen, dialStart, sessionOpen, firstInput, firstSent, drained atomic.Int64
	inputBytes, queued, sent                                                      atomic.Int64
	speechReady                                                                   chan struct{}
	origin                                                                        time.Time
	speechEnd, partial, definite, token, text, ttsAudio, audio                    atomic.Int64
	transcript                                                                    atomic.Pointer[string]
}

func (s *breakdownStamps) arrive() {
	if s.ready != nil {
		s.arrival.Do(func() { s.ready <- struct{}{} })
	}
}

func (s *breakdownStamps) mark(dst *atomic.Int64) {
	dst.CompareAndSwap(0, time.Since(s.origin).Nanoseconds())
}
func (s *breakdownStamps) delta(a, b *atomic.Int64) string {
	x, y := a.Load(), b.Load()
	if x == 0 || y == 0 {
		return "n/a"
	}
	return time.Duration(y - x).String()
}

type breakdownTap struct {
	genx.Stream
	observe func(*genx.MessageChunk)
}

func (s *breakdownTap) Next() (*genx.MessageChunk, error) {
	c, err := s.Stream.Next()
	if err == nil && c != nil {
		s.observe(c)
	}
	return c, err
}

type breakdownTransformer struct {
	genx.Transformer
	observe func(*genx.MessageChunk)
}

func (s breakdownTransformer) Transform(ctx context.Context, input genx.Stream) (genx.Stream, error) {
	out, err := s.Transformer.Transform(ctx, input)
	if err != nil {
		return nil, err
	}
	return &breakdownTap{Stream: out, observe: s.observe}, nil
}

type breakdownGenerator struct {
	genx.Generator
	stamps *breakdownStamps
}

func (g breakdownGenerator) GenerateStream(ctx context.Context, p string, m genx.ModelContext) (genx.Stream, error) {
	out, err := g.Generator.GenerateStream(ctx, p, m)
	if err != nil {
		return nil, err
	}
	return &breakdownTap{Stream: out, observe: func(c *genx.MessageChunk) {
		if text, ok := c.Part.(genx.Text); ok && len(text) > 0 {
			g.stamps.mark(&g.stamps.token)
		}
	}}, nil
}

type breakdownBuilder struct {
	peergenx.DefaultBuilder
	stamps     *breakdownStamps
	window     *int
	production bool
}

func (b breakdownBuilder) BuildGenerator(ctx context.Context, cfg peergenx.GeneratorConfig) (genx.Generator, error) {
	g, err := b.DefaultBuilder.BuildGenerator(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return breakdownGenerator{Generator: g, stamps: b.stamps}, nil
}
func (b breakdownBuilder) BuildTransformer(ctx context.Context, cfg peergenx.TransformerConfig) (genx.Transformer, error) {
	var tr genx.Transformer
	var err error
	asr := cfg.Model != nil && cfg.Model.Kind == apitypes.ModelKindAsr
	if asr && !b.production && b.window == nil {
		// The production builder injects forced endpoint defaults. Construct only
		// this control ASR directly to leave all three SDK endpoint pointers nil.
		body, e := cfg.Credential.Body.AsVolcCredentialBody()
		if e != nil {
			return nil, e
		}
		data, e := cfg.Model.ProviderData.AsVolcTenantModelProviderData()
		if e != nil {
			return nil, e
		}
		tr, err = doubaoasr.New(doubaoasr.Config{Client: doubaospeech.NewClient(*body.SpeechAppId, doubaospeech.WithAPIKey(*body.SpeechApiKey)), ResourceID: *data.ResourceId, EmitInterim: true, RealtimePacing: new(false)})
	} else {
		if asr && b.window != nil {
			cfg.Params = maps.Clone(cfg.Params)
			if cfg.Params == nil {
				cfg.Params = map[string]any{}
			}
			cfg.Params["end_window_size"] = *b.window
		}
		tr, err = b.DefaultBuilder.BuildTransformer(ctx, cfg)
	}
	if err != nil {
		return nil, err
	}
	if !asr {
		return breakdownTransformer{Transformer: tr, observe: func(c *genx.MessageChunk) {
			if blob, ok := c.Part.(*genx.Blob); ok && blob != nil && len(blob.Data) > 0 {
				b.stamps.mark(&b.stamps.ttsAudio)
			}
		}}, nil
	}
	asrTransformer, ok := tr.(*doubaoasr.Transformer)
	if !ok {
		return nil, fmt.Errorf("breakdown: expected doubaoasr, got %T", tr)
	}
	tr = asrTransformer.ObserveSessionForE2E(
		func() { b.stamps.mark(&b.stamps.dialStart) },
		func() {
			if b.stamps.sessionOpen.Load() == 0 {
				b.stamps.queued.Store(b.stamps.inputBytes.Load())
				b.stamps.mark(&b.stamps.sessionOpen)
			}
		},
		func(n int) {
			b.stamps.mark(&b.stamps.firstSent)
			if b.stamps.sent.Add(int64(n)) >= b.stamps.queued.Load() {
				b.stamps.mark(&b.stamps.drained)
			}
		},
	)
	// ASR emits full replacement hypotheses, then a definite replacement and EOS.
	// A lone text followed by EOS has no observable partial: report n/a.
	var candidate int64
	count := 0
	latest := ""
	var segments []string
	return breakdownTransformer{Transformer: tr, observe: func(c *genx.MessageChunk) {
		if c.Name != "transcript" {
			return
		}
		if text, ok := c.Part.(genx.Text); ok && len(text) > 0 {
			if count == 0 {
				candidate = time.Since(b.stamps.origin).Nanoseconds()
			}
			count++
			latest = string(text)
		}
		if c.IsEndOfStream() && c.Ctrl.Error == "" {
			if count > 1 && b.stamps.definite.Load() == 0 {
				b.stamps.partial.CompareAndSwap(0, candidate)
			}
			b.stamps.mark(&b.stamps.definite)
			segments = append(segments, latest)
			joined := strings.Join(segments, " ")
			b.stamps.transcript.Store(&joined)
			count = 0
			latest = ""
		}
	}}, nil
}

// Catalog fixtures delegate runtime behavior to peergenx, not to substitute providers.
type breakdownResources struct {
	peergenx.ProviderTenantGetter
	models     map[string]apitypes.Model
	voice      apitypes.Voice
	credential apitypes.Credential
}

func (r breakdownResources) GetModel(_ context.Context, q adminhttp.GetModelRequestObject) (adminhttp.GetModelResponseObject, error) {
	m, ok := r.models[q.Id]
	if !ok {
		return nil, fmt.Errorf("unknown model %q", q.Id)
	}
	return adminhttp.GetModel200JSONResponse(m), nil
}
func (r breakdownResources) GetVoice(_ context.Context, q adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	if q.Id != "assistant-voice" {
		return nil, fmt.Errorf("unknown voice %q", q.Id)
	}
	return adminhttp.GetVoice200JSONResponse(r.voice), nil
}
func (r breakdownResources) GetCredential(_ context.Context, _ adminhttp.GetCredentialRequestObject) (adminhttp.GetCredentialResponseObject, error) {
	return adminhttp.GetCredential200JSONResponse(r.credential), nil
}
func (r breakdownResources) GetVolcTenant(_ context.Context, q adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
	return adminhttp.GetVolcTenant200JSONResponse(apitypes.VolcTenant{Id: q.Id, CredentialId: r.credential.Id}), nil
}

func breakdownResource(t *testing.T, relative string, dst any) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test resources")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../gizclaw-e2e/testdata/resources", relative))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Metadata struct {
			ID string `json:"id"`
		} `json:"metadata"`
		Spec json.RawMessage `json:"spec"`
	}
	if err := yaml.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(envelope.Spec, dst); err != nil {
		t.Fatal(err)
	}
	if m, ok := dst.(*apitypes.Model); ok {
		m.Id = envelope.Metadata.ID
	}
}
func breakdownNormalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }
