package voiceprint

import (
	"fmt"
	"math"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/agent/ncnn"
)

const (
	defaultSegmentFrames = 300
	defaultHopFrames     = 150
)

// ncnnModel implements model with the ncnn inference engine.
type ncnnModel struct {
	mu       sync.RWMutex
	net      *ncnn.Net
	dim      int
	fbankCfg fbankConfig
	closed   bool

	inputName     string
	outputName    string
	segmentFrames int
	hopFrames     int
}

type ncnnSegmentConfig struct {
	SegmentFrames int
	HopFrames     int
}

type ncnnModelConfig struct {
	FbankCfg     *fbankConfig
	EmbeddingDim int
	InputName    string
	OutputName   string
	Segment      *ncnnSegmentConfig
}

func (m *ncnnModel) frontend(audio []byte) ([][]float32, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("voiceprint: model is closed")
	}
	features := computeFbank(audio, m.fbankCfg)
	if len(features) == 0 {
		return nil, fmt.Errorf("voiceprint: audio too short for fbank extraction")
	}
	cmvn(features, m.fbankCfg)
	return features, nil
}

func (m *ncnnModel) infer(features [][]float32) ([]float32, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("voiceprint: model is closed")
	}
	if m.net == nil {
		return nil, fmt.Errorf("voiceprint: model net is nil")
	}

	if m.segmentFrames <= 0 || len(features) <= m.segmentFrames {
		emb, err := m.extractSegment(m.net, features)
		if err != nil {
			return nil, err
		}
		l2Normalize(emb)
		return emb, nil
	}

	var (
		embeddings [][]float32
		lastStart  int
	)
	for start := 0; start+m.segmentFrames <= len(features); start += m.hopFrames {
		emb, err := m.extractSegment(m.net, features[start:start+m.segmentFrames])
		if err != nil {
			continue
		}
		l2Normalize(emb)
		embeddings = append(embeddings, emb)
		lastStart = start
	}

	if tail := len(features) - m.segmentFrames; tail > lastStart {
		emb, err := m.extractSegment(m.net, features[tail:])
		if err == nil {
			l2Normalize(emb)
			embeddings = append(embeddings, emb)
		}
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("voiceprint: all segments failed")
	}

	avg := make([]float32, m.dim)
	for _, emb := range embeddings {
		for i, v := range emb {
			avg[i] += v
		}
	}
	n := float32(len(embeddings))
	for i := range avg {
		avg[i] /= n
	}
	l2Normalize(avg)
	return avg, nil
}

// Extract converts PCM16 audio into one normalized speaker embedding.
func (m *ncnnModel) Extract(audio []byte) ([]float32, error) {
	features, err := m.frontend(audio)
	if err != nil {
		return nil, err
	}
	return m.infer(features)
}

func (m *ncnnModel) extractSegment(net *ncnn.Net, features [][]float32) ([]float32, error) {
	if len(features) == 0 || len(features[0]) == 0 {
		return nil, fmt.Errorf("voiceprint: empty feature segment")
	}

	input, err := ncnn.NewMat2D(len(features[0]), len(features), flatten(features))
	if err != nil {
		return nil, fmt.Errorf("voiceprint: create input mat: %w", err)
	}
	defer func() {
		_ = input.Close()
	}()

	ex, err := net.NewExtractor()
	if err != nil {
		return nil, fmt.Errorf("voiceprint: create extractor: %w", err)
	}
	defer func() {
		_ = ex.Close()
	}()

	if err := ex.SetInput(m.inputName, input); err != nil {
		return nil, fmt.Errorf("voiceprint: %w", err)
	}

	output, err := ex.Extract(m.outputName)
	if err != nil {
		return nil, fmt.Errorf("voiceprint: %w", err)
	}
	defer func() {
		_ = output.Close()
	}()

	data := output.FloatData()
	if len(data) == 0 {
		return nil, fmt.Errorf("voiceprint: ncnn output data is empty")
	}

	n := min(len(data), m.dim)
	embedding := make([]float32, m.dim)
	copy(embedding, data[:n])
	return embedding, nil
}

func l2Normalize(v []float32) {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if norm <= 0 {
		return
	}
	scale := float32(1.0 / norm)
	for i := range v {
		v[i] *= scale
	}
}

// Dimension implements model.
func (m *ncnnModel) Dimension() int {
	return m.dim
}

// Close implements model.
func (m *ncnnModel) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true
	if m.net != nil {
		_ = m.net.Close()
		m.net = nil
	}
	return nil
}
