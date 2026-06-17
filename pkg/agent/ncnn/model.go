package ncnn

import (
	"fmt"
	"slices"
	"sync"
)

// ModelID identifies a built-in ncnn model.
type ModelID string

const (
	minModelBinDataBytes = 4

	// ModelSpeakerERes2Net is the speaker embedding model.
	ModelSpeakerERes2Net ModelID = "speaker-eres2net"

	// ModelSpeakerECAPA is the ECAPA-TDNN speaker embedding model.
	ModelSpeakerECAPA ModelID = "speaker-ecapa"

	// ModelVADSilero is the voice activity detection model.
	ModelVADSilero ModelID = "vad-silero"

	// ModelDenoiseNSNet2 is the noise suppression model.
	ModelDenoiseNSNet2 ModelID = "denoise-nsnet2"
)

// ModelInfo describes a registered model payload.
type ModelInfo struct {
	ID        ModelID
	ParamData []byte
	BinData   []byte
}

var (
	registryMu sync.RWMutex
	registry   = make(map[ModelID]*ModelInfo)
)

// RegisterModel registers or replaces a model definition.
func RegisterModel(id ModelID, paramData, binData []byte) {
	registryMu.Lock()
	defer registryMu.Unlock()

	registry[id] = &ModelInfo{
		ID:        id,
		ParamData: append([]byte(nil), paramData...),
		BinData:   append([]byte(nil), binData...),
	}
}

// LoadModel creates a ready-to-use Net for a registered model ID.
func LoadModel(id ModelID) (*Net, error) {
	registryMu.RLock()
	info, ok := registry[id]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("ncnn: model %q not registered", id)
	}

	opt := NewOption()
	if opt == nil {
		return nil, fmt.Errorf("ncnn: option_create failed for model %q", id)
	}
	defer func() {
		_ = opt.Close()
	}()
	opt.SetFP16(false)

	net, err := NewNetFromMemory(info.ParamData, info.BinData, opt)
	if err != nil {
		return nil, err
	}
	return net, nil
}

// ListModels returns all registered model IDs in lexical order.
func ListModels() []ModelID {
	registryMu.RLock()
	ids := make([]ModelID, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	registryMu.RUnlock()
	slices.Sort(ids)
	return ids
}

// GetModelInfo returns a copy of registered model info.
func GetModelInfo(id ModelID) *ModelInfo {
	registryMu.RLock()
	info, ok := registry[id]
	registryMu.RUnlock()
	if !ok {
		return nil
	}

	return &ModelInfo{
		ID:        info.ID,
		ParamData: append([]byte(nil), info.ParamData...),
		BinData:   append([]byte(nil), info.BinData...),
	}
}
