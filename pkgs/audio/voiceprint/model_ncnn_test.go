package voiceprint

import (
	"math"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/agent/ncnn"
)

func loadEmbeddedSpeakerNet(t *testing.T, id ncnn.ModelID) *ncnn.Net {
	t.Helper()

	net, err := ncnn.LoadModel(id)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			t.Skip(err)
		}
		t.Fatalf("LoadModel: %v", err)
	}
	return net
}

func makeSineWavePCM(freq float64, samples, sampleRate int) []byte {
	audio := make([]byte, samples*2)
	for i := range samples {
		v := math.Sin(2 * math.Pi * freq * float64(i) / float64(sampleRate))
		s := int16(v * 30000)
		audio[i*2] = byte(s)
		audio[i*2+1] = byte(s >> 8)
	}
	return audio
}

//go:fix inline
func ptrToFbankConfig(cfg fbankConfig) *fbankConfig {
	return new(cfg)
}

func newTestERes2Model(cfg ncnnModelConfig) *ncnnModel {
	m := &ncnnModel{
		dim:           512,
		fbankCfg:      eres2netFbankConfig(),
		inputName:     "in0",
		outputName:    "out0",
		segmentFrames: defaultSegmentFrames,
		hopFrames:     defaultHopFrames,
	}
	if cfg.FbankCfg != nil {
		m.fbankCfg = *cfg.FbankCfg
	}
	if cfg.EmbeddingDim > 0 {
		m.dim = cfg.EmbeddingDim
	}
	if cfg.InputName != "" {
		m.inputName = cfg.InputName
	}
	if cfg.OutputName != "" {
		m.outputName = cfg.OutputName
	}
	if cfg.Segment != nil {
		m.segmentFrames = cfg.Segment.SegmentFrames
		m.hopFrames = cfg.Segment.HopFrames
	}
	if m.segmentFrames > 0 && m.hopFrames <= 0 {
		m.hopFrames = m.segmentFrames
	}
	if err := validateFbankConfig(m.fbankCfg); err != nil {
		panic(err)
	}
	return m
}

func newTestLoadedERes2Model(cfg ncnnModelConfig) (*ncnnModel, error) {
	net, err := ncnn.LoadModel(ncnn.ModelSpeakerERes2Net)
	if err != nil {
		return nil, err
	}
	m := newTestERes2Model(cfg)
	m.net = net
	return m, nil
}

func newTestECAPModel(cfg ncnnModelConfig) (*ncnnModel, error) {
	net, err := ncnn.LoadModel(ncnn.ModelSpeakerECAPA)
	if err != nil {
		return nil, err
	}
	m := &ncnnModel{
		net:           net,
		dim:           192,
		fbankCfg:      fbankConfig{SampleRate: 16000, NumMels: 80, FrameLength: 400, FrameShift: 160, FFTSize: 400, PreEmphasis: 0, EnergyFloor: 1e-10, LowFreq: 0, HighFreq: 8000, RemoveDC: false, Center: true, PoveyWindow: false, UseLog10: true, TopDB: 80, MeanNorm: true, StdNorm: false, NormalizePCM: true},
		inputName:     "in0",
		outputName:    "out0",
		segmentFrames: 0,
		hopFrames:     0,
	}
	if cfg.FbankCfg != nil {
		m.fbankCfg = *cfg.FbankCfg
	}
	if cfg.EmbeddingDim > 0 {
		m.dim = cfg.EmbeddingDim
	}
	if cfg.InputName != "" {
		m.inputName = cfg.InputName
	}
	if cfg.OutputName != "" {
		m.outputName = cfg.OutputName
	}
	if cfg.Segment != nil {
		m.segmentFrames = cfg.Segment.SegmentFrames
		m.hopFrames = cfg.Segment.HopFrames
	}
	if m.segmentFrames > 0 && m.hopFrames <= 0 {
		m.hopFrames = m.segmentFrames
	}
	if err := validateFbankConfig(m.fbankCfg); err != nil {
		panic(err)
	}
	return m, nil
}

func newTestModelFromMemory(t *testing.T, paramData, binData []byte, cfg ncnnModelConfig) *ncnnModel {
	t.Helper()
	if len(paramData) == 0 {
		t.Fatal("empty param data")
	}
	if len(binData) == 0 {
		t.Fatal("empty bin data")
	}
	opt := ncnn.NewOption()
	if opt == nil {
		t.Fatal("create ncnn option")
	}
	defer func() {
		_ = opt.Close()
	}()
	opt.SetFP16(false)

	net, err := ncnn.NewNetFromMemory(paramData, binData, opt)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			t.Skip(err)
		}
		t.Fatalf("NewNetFromMemory: %v", err)
	}
	m := newTestERes2Model(cfg)
	m.net = net
	return m
}

func TestERes2NetFbankConfig(t *testing.T) {
	cfg := eres2netFbankConfig()
	if cfg.SampleRate != 16000 || cfg.NumMels != 80 || !cfg.PoveyWindow || !cfg.RemoveDC || cfg.Center || cfg.UseLog10 || !cfg.MeanNorm || !cfg.StdNorm {
		t.Fatalf("unexpected eres2net fbank config: %+v", cfg)
	}
}

func TestECAPAFbankConfig(t *testing.T) {
	cfg := fbankConfig{
		SampleRate:   16000,
		NumMels:      80,
		FrameLength:  400,
		FrameShift:   160,
		FFTSize:      400,
		PreEmphasis:  0,
		EnergyFloor:  1e-10,
		LowFreq:      0,
		HighFreq:     8000,
		RemoveDC:     false,
		Center:       true,
		PoveyWindow:  false,
		UseLog10:     true,
		TopDB:        80,
		MeanNorm:     true,
		StdNorm:      false,
		NormalizePCM: true,
	}
	if cfg.SampleRate != 16000 ||
		cfg.NumMels != 80 ||
		cfg.FFTSize != 400 ||
		cfg.PoveyWindow ||
		cfg.RemoveDC ||
		cfg.PreEmphasis != 0 ||
		cfg.LowFreq != 0 ||
		cfg.HighFreq != 8000 ||
		!cfg.Center ||
		!cfg.UseLog10 ||
		cfg.TopDB != 80 ||
		!cfg.MeanNorm ||
		cfg.StdNorm {
		t.Fatalf("unexpected ECAPA fbank config: %+v", cfg)
	}
}

func TestNewERes2ModelConfig(t *testing.T) {
	m := newTestERes2Model(ncnnModelConfig{
		EmbeddingDim: 256,
		InputName:    "input",
		OutputName:   "output",
		Segment: &ncnnSegmentConfig{
			SegmentFrames: 0,
			HopFrames:     0,
		},
	})
	if m.Dimension() != 256 {
		t.Fatalf("Dimension() = %d, want 256", m.Dimension())
	}
	if m.inputName != "input" || m.outputName != "output" {
		t.Fatalf("unexpected blob names: %q %q", m.inputName, m.outputName)
	}
	if m.segmentFrames != 0 || m.hopFrames != 0 {
		t.Fatalf("unexpected segment config: %d/%d", m.segmentFrames, m.hopFrames)
	}
}

func TestNewERes2ModelConfigRejectsInvalidFbankConfig(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid fbank config")
		}
	}()
	badCfg := fbankConfig{}
	_ = newTestERes2Model(ncnnModelConfig{
		FbankCfg: &badCfg,
	})
}

func TestNCNNModelExtractRejectsNilNet(t *testing.T) {
	model := newTestERes2Model(ncnnModelConfig{})
	_, err := model.Extract(makeSineWavePCM(440, 6400, 16000))
	if err == nil {
		t.Fatal("expected nil-net error")
	}
}

func TestDefaultNCNNModelExtract(t *testing.T) {
	model, err := newTestLoadedERes2Model(ncnnModelConfig{})
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			t.Skip(err)
		}
		t.Fatalf("newTestLoadedERes2Model: %v", err)
	}
	defer model.Close()

	audio := makeSineWavePCM(440, 6400, 16000)
	embedding, err := model.Extract(audio)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(embedding) != 512 {
		t.Fatalf("embedding length = %d, want 512", len(embedding))
	}

	var norm float64
	for _, v := range embedding {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.001 {
		t.Fatalf("embedding norm = %.6f, want near 1.0", norm)
	}
}

func TestECAPANCNNModelExtract(t *testing.T) {
	model, err := newTestECAPModel(ncnnModelConfig{})
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			t.Skip(err)
		}
		t.Fatalf("newTestECAPModel: %v", err)
	}
	defer model.Close()

	audio := makeSineWavePCM(440, 6400, 16000)
	embedding, err := model.Extract(audio)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(embedding) != 192 {
		t.Fatalf("embedding length = %d, want 192", len(embedding))
	}

	var norm float64
	for _, v := range embedding {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 0.001 {
		t.Fatalf("embedding norm = %.6f, want near 1.0", norm)
	}
}

func TestDefaultNCNNModelExtractLongAudio(t *testing.T) {
	model, err := newTestLoadedERes2Model(ncnnModelConfig{})
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			t.Skip(err)
		}
		t.Fatalf("newTestLoadedERes2Model: %v", err)
	}
	defer model.Close()

	audio := makeSineWavePCM(440, 64000, 16000)
	embedding, err := model.Extract(audio)
	if err != nil {
		t.Fatalf("Extract long audio: %v", err)
	}
	if len(embedding) != 512 {
		t.Fatalf("embedding length = %d, want 512", len(embedding))
	}
}

func TestECAPANCNNModelFromMemoryMatchesBuiltin(t *testing.T) {
	info := ncnn.GetModelInfo(ncnn.ModelSpeakerECAPA)
	if info == nil {
		t.Fatal("expected embedded ECAPA model info")
	}

	defaultModel, err := newTestECAPModel(ncnnModelConfig{})
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			t.Skip(err)
		}
		t.Fatalf("newTestECAPModel: %v", err)
	}
	defer defaultModel.Close()

	memModel := newTestModelFromMemory(t, info.ParamData, info.BinData, ncnnModelConfig{
		EmbeddingDim: 192,
		FbankCfg: new(fbankConfig{
			SampleRate:   16000,
			NumMels:      80,
			FrameLength:  400,
			FrameShift:   160,
			FFTSize:      400,
			PreEmphasis:  0,
			EnergyFloor:  1e-10,
			LowFreq:      0,
			HighFreq:     8000,
			RemoveDC:     false,
			Center:       true,
			PoveyWindow:  false,
			UseLog10:     true,
			TopDB:        80,
			MeanNorm:     true,
			StdNorm:      false,
			NormalizePCM: true,
		}),
		Segment: &ncnnSegmentConfig{
			SegmentFrames: 0,
			HopFrames:     0,
		},
	})
	defer memModel.Close()

	audio := makeSineWavePCM(440, 6400, 16000)

	emb1, err := defaultModel.Extract(audio)
	if err != nil {
		t.Fatalf("default Extract: %v", err)
	}
	emb2, err := memModel.Extract(audio)
	if err != nil {
		t.Fatalf("memory Extract: %v", err)
	}

	for i := range emb1 {
		if emb1[i] != emb2[i] {
			t.Fatalf("embedding[%d]: default=%f memory=%f", i, emb1[i], emb2[i])
		}
	}
}

func TestNCNNModelFromMemoryMatchesDefaultModel(t *testing.T) {
	info := ncnn.GetModelInfo(ncnn.ModelSpeakerERes2Net)
	if info == nil {
		t.Fatal("expected embedded speaker model info")
	}

	defaultModel, err := newTestLoadedERes2Model(ncnnModelConfig{})
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			t.Skip(err)
		}
		t.Fatalf("newTestLoadedERes2Model: %v", err)
	}
	defer defaultModel.Close()

	memModel := newTestModelFromMemory(t, info.ParamData, info.BinData, ncnnModelConfig{})
	defer memModel.Close()

	audio := makeSineWavePCM(440, 6400, 16000)

	emb1, err := defaultModel.Extract(audio)
	if err != nil {
		t.Fatalf("default Extract: %v", err)
	}
	emb2, err := memModel.Extract(audio)
	if err != nil {
		t.Fatalf("memory Extract: %v", err)
	}

	for i := range emb1 {
		if emb1[i] != emb2[i] {
			t.Fatalf("embedding[%d]: default=%f memory=%f", i, emb1[i], emb2[i])
		}
	}
}

func TestNCNNModelClose(t *testing.T) {
	net := loadEmbeddedSpeakerNet(t, ncnn.ModelSpeakerERes2Net)
	model := newTestERes2Model(ncnnModelConfig{})
	model.net = net

	if err := model.Close(); err != nil {
		t.Fatal(err)
	}
	if err := model.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Extract(makeSineWavePCM(440, 6400, 16000)); err == nil {
		t.Fatal("expected error after Close")
	}
}

func TestL2NormalizeZeroVector(t *testing.T) {
	v := []float32{0, 0, 0}
	l2Normalize(v)
	if v[0] != 0 || v[1] != 0 || v[2] != 0 {
		t.Fatalf("zero vector should remain unchanged: %v", v)
	}
}
