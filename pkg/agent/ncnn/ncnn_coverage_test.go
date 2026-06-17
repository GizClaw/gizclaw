package ncnn

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustBuiltinModelInfo(t *testing.T, id ModelID) *ModelInfo {
	t.Helper()
	info := GetModelInfo(id)
	if info == nil {
		t.Fatalf("GetModelInfo(%q) returned nil", id)
	}
	if len(info.ParamData) == 0 || len(info.BinData) == 0 {
		t.Fatalf("GetModelInfo(%q) returned empty model data", id)
	}
	return info
}

func TestPlatformMatrix(t *testing.T) {
	if !strings.Contains(supportedPlatformDescription, "darwin/arm64") {
		t.Fatalf("supportedPlatformDescription missing darwin/arm64: %q", supportedPlatformDescription)
	}

	cases := []struct {
		goos   string
		goarch string
		want   bool
	}{
		{goos: "linux", goarch: "amd64", want: true},
		{goos: "linux", goarch: "arm64", want: true},
		{goos: "darwin", goarch: "amd64", want: true},
		{goos: "darwin", goarch: "arm64", want: true},
		{goos: "linux", goarch: "riscv64", want: false},
		{goos: "windows", goarch: "amd64", want: false},
	}

	for _, tc := range cases {
		got := isSupportedPlatform(tc.goos, tc.goarch)
		if got != tc.want {
			t.Fatalf("isSupportedPlatform(%q, %q)=%v, want %v", tc.goos, tc.goarch, got, tc.want)
		}
	}
}

func TestGetModelInfoDefensiveCopy(t *testing.T) {
	info := mustBuiltinModelInfo(t, ModelSpeakerERes2Net)

	param0 := info.ParamData[0]
	bin0 := info.BinData[0]

	info.ParamData[0] ^= 0xFF
	info.BinData[0] ^= 0xFF

	infoAgain := mustBuiltinModelInfo(t, ModelSpeakerERes2Net)
	if infoAgain.ParamData[0] != param0 {
		t.Fatalf("ParamData was mutated through returned copy")
	}
	if infoAgain.BinData[0] != bin0 {
		t.Fatalf("BinData was mutated through returned copy")
	}

	if got := GetModelInfo(ModelID("ncnn-model-not-exist")); got != nil {
		t.Fatalf("GetModelInfo(non-exist)=%v, want nil", got)
	}
}

func TestRegisterModelCopiesInputSlices(t *testing.T) {
	id := ModelID("unit-model-copy-check")
	param := []byte{1, 2, 3}
	bin := []byte{4, 5, 6}

	RegisterModel(id, param, bin)
	param[0] = 99
	bin[0] = 88

	info := GetModelInfo(id)
	if info == nil {
		t.Fatalf("GetModelInfo(%q) returned nil", id)
	}
	if info.ParamData[0] != 1 {
		t.Fatalf("ParamData copy check failed: got %d, want 1", info.ParamData[0])
	}
	if info.BinData[0] != 4 {
		t.Fatalf("BinData copy check failed: got %d, want 4", info.BinData[0])
	}
}

func TestVersionAndOptionHelpers(t *testing.T) {
	requireNativeNCNNSupportedRuntime(t)

	if v := Version(); strings.TrimSpace(v) == "" {
		t.Fatalf("Version() returned empty string")
	}

	var nilOpt *Option
	if got := nilOpt.SetFP16(true); got != nil {
		t.Fatalf("nil option SetFP16 should return nil, got %v", got)
	}
	if got := nilOpt.SetNumThreads(4); got != nil {
		t.Fatalf("nil option SetNumThreads should return nil, got %v", got)
	}
	if err := nilOpt.Close(); err != nil {
		t.Fatalf("nil option Close() error: %v", err)
	}

	opt := NewOption()
	if opt == nil {
		t.Fatal("NewOption() returned nil")
	}

	if got := opt.SetFP16(true); got != opt {
		t.Fatal("SetFP16 should return receiver")
	}
	if got := opt.SetNumThreads(0); got != opt {
		t.Fatal("SetNumThreads should return receiver")
	}

	if err := opt.Close(); err != nil {
		t.Fatalf("Close option: %v", err)
	}
	if err := opt.Close(); err != nil {
		t.Fatalf("Close option second call: %v", err)
	}

	// No-op after close.
	_ = opt.SetFP16(false)
	_ = opt.SetNumThreads(2)
}

func TestNewNetFromFileAndSetOption(t *testing.T) {
	requireNativeNCNNSupportedRuntime(t)

	info := mustBuiltinModelInfo(t, ModelSpeakerERes2Net)
	tempDir := t.TempDir()
	paramPath := filepath.Join(tempDir, "model.param")
	binPath := filepath.Join(tempDir, "model.bin")

	if err := os.WriteFile(paramPath, info.ParamData, 0o600); err != nil {
		t.Fatalf("WriteFile param: %v", err)
	}
	if err := os.WriteFile(binPath, info.BinData, 0o600); err != nil {
		t.Fatalf("WriteFile bin: %v", err)
	}

	net, err := NewNet(paramPath, binPath)
	if err != nil {
		t.Fatalf("NewNet(valid files): %v", err)
	}
	defer func() {
		_ = net.Close()
	}()

	var nilNet *Net
	nilNet.SetOption(nil)
	nilNet.SetOption(NewOption())

	net.SetOption(nil)
	opt := NewOption()
	if opt == nil {
		t.Fatal("NewOption() returned nil")
	}
	net.SetOption(opt.SetNumThreads(2).SetFP16(false))
	_ = opt.Close()

	_, err = NewNet(filepath.Join(tempDir, "missing.param"), binPath)
	if err == nil || !strings.Contains(err.Error(), "load_param") {
		t.Fatalf("expected load_param error, got: %v", err)
	}

	_, err = NewNet(paramPath, filepath.Join(tempDir, "missing.bin"))
	if err == nil || !strings.Contains(err.Error(), "load_model") {
		t.Fatalf("expected load_model error, got: %v", err)
	}
}

func TestNewNetFromMemoryAdditionalPaths(t *testing.T) {
	requireNativeNCNNSupportedRuntime(t)

	info := mustBuiltinModelInfo(t, ModelSpeakerERes2Net)

	closedOpt := NewOption()
	if closedOpt == nil {
		t.Fatal("NewOption() returned nil")
	}
	if err := closedOpt.Close(); err != nil {
		t.Fatalf("Close option: %v", err)
	}

	net, err := NewNetFromMemory(info.ParamData, info.BinData, nil, closedOpt)
	if err != nil {
		t.Fatalf("NewNetFromMemory(valid): %v", err)
	}
	if err := net.Close(); err != nil {
		t.Fatalf("Close net: %v", err)
	}

	_, err = NewNetFromMemory([]byte("not-a-valid-param"), []byte{1, 2, 3, 4})
	if err == nil || !strings.Contains(err.Error(), "load_param_memory") {
		t.Fatalf("expected load_param_memory error, got: %v", err)
	}

	_, err = NewNetFromMemory(info.ParamData, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "bin data too short") {
		t.Fatalf("expected short bin data error, got: %v", err)
	}
}

func TestNetAndExtractorErrorBranches(t *testing.T) {
	nilExtractorErrSubstr := "extractor is nil"
	if !isNativeNCNNSupportedRuntime() {
		nilExtractorErrSubstr = "unsupported platform"
	}

	var nilNet *Net
	if ex, err := nilNet.NewExtractor(); err == nil || ex != nil {
		t.Fatalf("nil net should fail NewExtractor, ex=%v err=%v", ex, err)
	}
	if err := nilNet.Close(); err != nil {
		t.Fatalf("nil net Close() error: %v", err)
	}

	var nilExtractor *Extractor
	if err := nilExtractor.SetInput("in0", &Mat{}); err == nil || !strings.Contains(err.Error(), nilExtractorErrSubstr) {
		t.Fatalf("nil extractor SetInput error mismatch: %v", err)
	}
	if out, err := nilExtractor.Extract("out0"); err == nil || out != nil {
		t.Fatalf("nil extractor Extract should fail, out=%v err=%v", out, err)
	}
	if err := nilExtractor.Close(); err != nil {
		t.Fatalf("nil extractor Close() error: %v", err)
	}

	ex := &Extractor{}
	if err := ex.SetInput("in0", &Mat{}); err == nil || !strings.Contains(err.Error(), nilExtractorErrSubstr) {
		t.Fatalf("empty extractor SetInput error mismatch: %v", err)
	}
	if out, err := ex.Extract("out0"); err == nil || out != nil {
		t.Fatalf("empty extractor Extract should fail, out=%v err=%v", out, err)
	}
	ex.SetOption(nil)
	if err := ex.Close(); err != nil {
		t.Fatalf("empty extractor Close() error: %v", err)
	}

	requireNativeNCNNSupportedRuntime(t)

	net, err := LoadModel(ModelSpeakerERes2Net)
	if err != nil {
		t.Fatalf("LoadModel(%q): %v", ModelSpeakerERes2Net, err)
	}
	ex2, err := net.NewExtractor()
	if err != nil {
		t.Fatalf("NewExtractor: %v", err)
	}

	ex2.SetOption(nil)
	opt := NewOption()
	if opt == nil {
		t.Fatal("NewOption() returned nil")
	}
	ex2.SetOption(opt)
	_ = opt.Close()

	if err := ex2.SetInput("in0", nil); err == nil || !strings.Contains(err.Error(), "input mat is nil") {
		t.Fatalf("expected input mat nil error, got: %v", err)
	}

	mat, err := NewMat2D(1, 1, []float32{0.5})
	if err != nil {
		t.Fatalf("NewMat2D: %v", err)
	}
	defer func() {
		_ = mat.Close()
	}()

	if err := ex2.SetInput("", mat); err == nil {
		t.Fatalf("expected SetInput with empty name to fail")
	}

	// This branch is expected to pass for valid input blob names.
	if err := ex2.SetInput("in0", mat); err != nil {
		t.Fatalf("SetInput valid blob failed: %v", err)
	}

	if out, err := ex2.Extract("__not_exists__"); err == nil || out != nil {
		t.Fatalf("expected Extract invalid output to fail, out=%v err=%v", out, err)
	}

	if err := ex2.Close(); err != nil {
		t.Fatalf("Close extractor: %v", err)
	}
	if err := ex2.Close(); err != nil {
		t.Fatalf("Close extractor second call: %v", err)
	}

	if err := net.Close(); err != nil {
		t.Fatalf("Close net: %v", err)
	}
	if err := net.Close(); err != nil {
		t.Fatalf("Close net second call: %v", err)
	}

	if ex3, err := net.NewExtractor(); err == nil || ex3 != nil {
		t.Fatalf("closed net should fail NewExtractor, ex=%v err=%v", ex3, err)
	}
}

func TestMatCoverageAndCheckedMul(t *testing.T) {
	requireNativeNCNNSupportedRuntime(t)

	if _, err := NewMat2D(0, 1, []float32{1}); err == nil || !strings.Contains(err.Error(), "invalid shape") {
		t.Fatalf("expected NewMat2D invalid shape error, got: %v", err)
	}
	if _, err := NewMat2D(2, 2, []float32{1}); err == nil || !strings.Contains(err.Error(), "data too short") {
		t.Fatalf("expected NewMat2D short data error, got: %v", err)
	}
	if _, err := NewMat2D(math.MaxInt, 2, []float32{1, 2}); err == nil || !strings.Contains(err.Error(), "shape overflow") {
		t.Fatalf("expected NewMat2D overflow error, got: %v", err)
	}

	mat2d, err := NewMat2D(2, 2, []float32{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("NewMat2D valid: %v", err)
	}
	if mat2d.W() != 2 || mat2d.H() != 2 || mat2d.C() != 1 {
		t.Fatalf("mat2d dimensions mismatch: %dx%dx%d", mat2d.W(), mat2d.H(), mat2d.C())
	}
	if got := mat2d.FloatData(); len(got) != 4 || got[0] != 1 || got[3] != 4 {
		t.Fatalf("mat2d FloatData mismatch: %v", got)
	}
	if err := mat2d.Close(); err != nil {
		t.Fatalf("mat2d Close: %v", err)
	}
	if got := mat2d.FloatData(); got != nil {
		t.Fatalf("FloatData after Close should be nil, got=%v", got)
	}
	if err := mat2d.Close(); err != nil {
		t.Fatalf("mat2d Close second call: %v", err)
	}

	if _, err := NewMat3D(0, 1, 1, []float32{1}); err == nil || !strings.Contains(err.Error(), "invalid shape") {
		t.Fatalf("expected NewMat3D invalid shape error, got: %v", err)
	}
	if _, err := NewMat3D(2, 2, 2, []float32{1}); err == nil || !strings.Contains(err.Error(), "data too short") {
		t.Fatalf("expected NewMat3D short data error, got: %v", err)
	}
	if _, err := NewMat3D(math.MaxInt, 2, 2, []float32{1, 2}); err == nil || !strings.Contains(err.Error(), "shape overflow") {
		t.Fatalf("expected NewMat3D overflow error, got: %v", err)
	}

	mat3d, err := NewMat3D(2, 2, 2, []float32{1, 2, 3, 4, 5, 6, 7, 8})
	if err != nil {
		t.Fatalf("NewMat3D valid: %v", err)
	}
	if mat3d.W() != 2 || mat3d.H() != 2 || mat3d.C() != 2 {
		t.Fatalf("mat3d dimensions mismatch: %dx%dx%d", mat3d.W(), mat3d.H(), mat3d.C())
	}
	if got := mat3d.FloatData(); len(got) != 8 || got[0] != 1 || got[7] != 8 {
		t.Fatalf("mat3d FloatData mismatch: %v", got)
	}
	if err := mat3d.Close(); err != nil {
		t.Fatalf("mat3d Close: %v", err)
	}

	var nilMat *Mat
	if nilMat.W() != 0 || nilMat.H() != 0 || nilMat.C() != 0 {
		t.Fatalf("nil mat dimensions should be zero")
	}
	if got := nilMat.FloatData(); got != nil {
		t.Fatalf("nil mat FloatData should be nil, got=%v", got)
	}
	if err := nilMat.Close(); err != nil {
		t.Fatalf("nil mat Close() error: %v", err)
	}

	emptyMat := &Mat{}
	if emptyMat.W() != 0 || emptyMat.H() != 0 || emptyMat.C() != 0 {
		t.Fatalf("empty mat dimensions should be zero")
	}
	if got := emptyMat.FloatData(); got != nil {
		t.Fatalf("empty mat FloatData should be nil, got=%v", got)
	}
	if err := emptyMat.Close(); err != nil {
		t.Fatalf("empty mat Close() error: %v", err)
	}

	if got, ok := checkedMul(-1, 1); ok || got != 0 {
		t.Fatalf("checkedMul(-1,1)=(%d,%v), want (0,false)", got, ok)
	}
	if got, ok := checkedMul(0, 123); !ok || got != 0 {
		t.Fatalf("checkedMul(0,123)=(%d,%v), want (0,true)", got, ok)
	}
	if got, ok := checkedMul(math.MaxInt, 2); ok || got != 0 {
		t.Fatalf("checkedMul(MaxInt,2)=(%d,%v), want (0,false)", got, ok)
	}
	if got, ok := checkedMul(7, 9); !ok || got != 63 {
		t.Fatalf("checkedMul(7,9)=(%d,%v), want (63,true)", got, ok)
	}
}
