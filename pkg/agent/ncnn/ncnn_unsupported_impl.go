//go:build !cgo || !(linux || darwin) || ((linux || darwin) && cgo && !amd64 && !arm64)

package ncnn

import (
	"fmt"
	"runtime"
)

func unsupportedErr() error {
	return fmt.Errorf("ncnn: unsupported platform %s/%s: phase 1 supports %s", runtime.GOOS, runtime.GOARCH, supportedPlatformDescription)
}

// Version returns a static marker on unsupported platforms.
func Version() string {
	return "unsupported"
}

// Net is unavailable on unsupported platforms.
type Net struct{}

// NewNet is unavailable on unsupported platforms.
func NewNet(paramPath, binPath string) (*Net, error) {
	_ = paramPath
	_ = binPath
	return nil, unsupportedErr()
}

// NewNetFromMemory validates input then returns unsupported platform error.
func NewNetFromMemory(paramData, binData []byte, opts ...*Option) (*Net, error) {
	_ = opts
	if len(paramData) == 0 {
		return nil, fmt.Errorf("ncnn: empty param data")
	}
	if len(binData) == 0 {
		return nil, fmt.Errorf("ncnn: empty bin data")
	}
	if len(binData) < minModelBinDataBytes {
		return nil, fmt.Errorf("ncnn: bin data too short: %d bytes", len(binData))
	}
	return nil, unsupportedErr()
}

// SetOption is a no-op on unsupported platforms.
func (n *Net) SetOption(opt *Option) {
	_ = n
	_ = opt
}

// NewExtractor is unavailable on unsupported platforms.
func (n *Net) NewExtractor() (*Extractor, error) {
	_ = n
	return nil, unsupportedErr()
}

// Close is safe to call repeatedly.
func (n *Net) Close() error {
	_ = n
	return nil
}

// Option exists for API compatibility on unsupported platforms.
type Option struct {
	useFP16    bool
	numThreads int
}

// NewOption allocates a new option.
func NewOption() *Option {
	return &Option{useFP16: true, numThreads: 1}
}

// SetFP16 updates option field.
func (o *Option) SetFP16(enabled bool) *Option {
	if o == nil {
		return nil
	}
	o.useFP16 = enabled
	return o
}

// SetNumThreads updates option field.
func (o *Option) SetNumThreads(n int) *Option {
	if o == nil {
		return nil
	}
	if n < 1 {
		n = 1
	}
	o.numThreads = n
	return o
}

// Close is safe to call repeatedly.
func (o *Option) Close() error {
	_ = o
	return nil
}

// Extractor is unavailable on unsupported platforms.
type Extractor struct{}

// SetInput returns unsupported platform error.
func (e *Extractor) SetInput(name string, mat *Mat) error {
	_ = e
	_ = name
	_ = mat
	return unsupportedErr()
}

// Extract returns unsupported platform error.
func (e *Extractor) Extract(name string) (*Mat, error) {
	_ = e
	_ = name
	return nil, unsupportedErr()
}

// SetOption is a no-op.
func (e *Extractor) SetOption(opt *Option) {
	_ = e
	_ = opt
}

// Close is safe to call repeatedly.
func (e *Extractor) Close() error {
	_ = e
	return nil
}

// Mat stores tensor data on unsupported platforms for local validation.
type Mat struct {
	w      int
	h      int
	c      int
	closed bool
	data   []float32
}

// NewMat2D creates a 2D tensor snapshot.
func NewMat2D(w, h int, data []float32) (*Mat, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("ncnn: NewMat2D called with empty data")
	}
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("ncnn: NewMat2D invalid shape: w=%d h=%d", w, h)
	}
	need, ok := checkedMul(w, h)
	if !ok {
		return nil, fmt.Errorf("ncnn: NewMat2D shape overflow: w=%d h=%d", w, h)
	}
	if len(data) < need {
		return nil, fmt.Errorf("ncnn: NewMat2D data too short: got %d, need %d (w=%d, h=%d)", len(data), need, w, h)
	}
	return &Mat{w: w, h: h, c: 1, data: append([]float32(nil), data[:need]...)}, nil
}

// NewMat3D creates a 3D tensor snapshot.
func NewMat3D(w, h, c int, data []float32) (*Mat, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("ncnn: NewMat3D called with empty data")
	}
	if w <= 0 || h <= 0 || c <= 0 {
		return nil, fmt.Errorf("ncnn: NewMat3D invalid shape: w=%d h=%d c=%d", w, h, c)
	}
	needWH, ok := checkedMul(w, h)
	if !ok {
		return nil, fmt.Errorf("ncnn: NewMat3D shape overflow: w=%d h=%d c=%d", w, h, c)
	}
	need, ok := checkedMul(needWH, c)
	if !ok {
		return nil, fmt.Errorf("ncnn: NewMat3D shape overflow: w=%d h=%d c=%d", w, h, c)
	}
	if len(data) < need {
		return nil, fmt.Errorf("ncnn: NewMat3D data too short: got %d, need %d (w=%d, h=%d, c=%d)", len(data), need, w, h, c)
	}
	return &Mat{w: w, h: h, c: c, data: append([]float32(nil), data[:need]...)}, nil
}

// W returns tensor width.
func (m *Mat) W() int {
	if m == nil {
		return 0
	}
	return m.w
}

// H returns tensor height.
func (m *Mat) H() int {
	if m == nil {
		return 0
	}
	return m.h
}

// C returns tensor channel count.
func (m *Mat) C() int {
	if m == nil {
		return 0
	}
	return m.c
}

// FloatData returns a copy of tensor data.
func (m *Mat) FloatData() []float32 {
	if m == nil || m.closed || len(m.data) == 0 {
		return nil
	}
	return append([]float32(nil), m.data...)
}

// Close releases tensor data.
func (m *Mat) Close() error {
	if m == nil || m.closed {
		return nil
	}
	m.closed = true
	m.data = nil
	m.w = 0
	m.h = 0
	m.c = 0
	return nil
}
