package ncnn

import (
	"runtime"
	"slices"
	"strings"
	"testing"
)

func isNativeNCNNSupportedRuntime() bool {
	return nativeCGOEnabled && isSupportedPlatform(runtime.GOOS, runtime.GOARCH)
}

func requireNativeNCNNSupportedRuntime(t *testing.T) {
	t.Helper()
	if !isNativeNCNNSupportedRuntime() {
		t.Skipf("requires native ncnn runtime, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func containsModel(models []ModelID, target ModelID) bool {
	return slices.Contains(models, target)
}

func TestListModelsIncludesBuiltins(t *testing.T) {
	models := ListModels()
	for _, required := range []ModelID{ModelSpeakerERes2Net, ModelSpeakerECAPA, ModelVADSilero, ModelDenoiseNSNet2} {
		if !containsModel(models, required) {
			t.Fatalf("ListModels() missing %q, got=%v", required, models)
		}
	}
}

func TestLoadModelCreatesExtractor(t *testing.T) {
	requireNativeNCNNSupportedRuntime(t)

	net, err := LoadModel(ModelSpeakerERes2Net)
	if err != nil {
		t.Fatalf("LoadModel(%q): %v", ModelSpeakerERes2Net, err)
	}
	defer func() {
		_ = net.Close()
	}()

	ex, err := net.NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}
	if err := ex.Close(); err != nil {
		t.Fatalf("Close extractor: %v", err)
	}
}

func TestNewMat2DMinValidInput(t *testing.T) {
	mat, err := NewMat2D(1, 1, []float32{0.1})
	if err != nil {
		t.Fatalf("NewMat2D(1,1): %v", err)
	}
	defer func() {
		_ = mat.Close()
	}()

	if mat.W() != 1 || mat.H() != 1 {
		t.Fatalf("mat dimensions = %dx%d, want 1x1", mat.W(), mat.H())
	}

	out := mat.FloatData()
	if len(out) != 1 {
		t.Fatalf("FloatData len = %d, want 1", len(out))
	}
	if out[0] != 0.1 {
		t.Fatalf("FloatData[0] = %v, want 0.1", out[0])
	}
}

func TestLoadThreeBuiltinModels(t *testing.T) {
	requireNativeNCNNSupportedRuntime(t)

	for _, id := range []ModelID{ModelSpeakerERes2Net, ModelSpeakerECAPA, ModelVADSilero, ModelDenoiseNSNet2} {
		t.Run(string(id), func(t *testing.T) {
			net, err := LoadModel(id)
			if err != nil {
				t.Fatalf("LoadModel(%q): %v", id, err)
			}
			defer func() {
				_ = net.Close()
			}()

			ex, err := net.NewExtractor()
			if err != nil {
				t.Fatalf("NewExtractor(%q): %v", id, err)
			}
			if err := ex.Close(); err != nil {
				t.Fatalf("Close extractor(%q): %v", id, err)
			}
		})
	}
}

func TestNewNetFromMemoryEmptyDataErrors(t *testing.T) {
	_, err := NewNetFromMemory(nil, []byte("bin"))
	if err == nil || !strings.Contains(err.Error(), "empty param data") {
		t.Fatalf("expected empty param data error, got: %v", err)
	}

	_, err = NewNetFromMemory([]byte("param"), nil)
	if err == nil || !strings.Contains(err.Error(), "empty bin data") {
		t.Fatalf("expected empty bin data error, got: %v", err)
	}
}

func TestLoadModelNotRegistered(t *testing.T) {
	_, err := LoadModel("nonexistent")
	if err == nil {
		t.Fatal("expected error for unregistered model")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsupportedPlatformHasClearError(t *testing.T) {
	if isNativeNCNNSupportedRuntime() {
		t.Skipf("native ncnn is supported on %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	_, err := LoadModel(ModelSpeakerERes2Net)
	if err == nil {
		t.Fatal("expected unsupported platform error")
	}
	if !strings.Contains(err.Error(), "unsupported platform") {
		t.Fatalf("unexpected error: %v", err)
	}
}
