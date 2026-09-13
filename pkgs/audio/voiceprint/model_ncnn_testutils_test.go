package voiceprint

import "fmt"

type stubDetectorModel struct {
	dim   int
	byKey map[string][]float32
}

func (m *stubDetectorModel) frontend(audio []byte) ([][]float32, error) {
	frame := make([]float32, len(audio))
	for i, b := range audio {
		frame[i] = float32(b)
	}
	return [][]float32{frame}, nil
}

func (m *stubDetectorModel) infer(features [][]float32) ([]float32, error) {
	if len(features) == 0 {
		return nil, fmt.Errorf("empty features")
	}
	audio := make([]byte, len(features[0]))
	for i, v := range features[0] {
		audio[i] = byte(v)
	}
	emb, ok := m.byKey[string(audio)]
	if !ok {
		return nil, fmt.Errorf("unknown audio sample %v", audio)
	}
	out := make([]float32, len(emb))
	copy(out, emb)
	return out, nil
}

func (m *stubDetectorModel) Extract(audio []byte) ([]float32, error) {
	emb, ok := m.byKey[string(audio)]
	if !ok {
		return nil, fmt.Errorf("unknown audio sample %v", audio)
	}
	out := make([]float32, len(emb))
	copy(out, emb)
	return out, nil
}

func (m *stubDetectorModel) Dimension() int { return m.dim }
func (m *stubDetectorModel) Close() error   { return nil }

func makeStubDetectorModel() *stubDetectorModel {
	return &stubDetectorModel{
		dim: 2,
		byKey: map[string][]float32{
			string([]byte{1, 0}):       {1, 0},
			string([]byte{1, 0, 1, 0}): {1, 0},
			string([]byte{2, 0}):       {0, 1},
			string([]byte{3, 0}):       {-1, 0},
			string([]byte{4, 0}):       {0, -1},
			string([]byte{5, 0}):       {1, 1},
			string([]byte{6, 0}):       {0.98, 0.02},
			string([]byte{7, 0}):       {0.02, 0.98},
		},
	}
}
