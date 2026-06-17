//go:build cgo && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)))

// Package ncnn provides Go bindings for the ncnn neural network inference
// framework via CGo static linking on supported Linux and macOS targets.
package ncnn

/*
#include <ncnn/c_api.h>
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// Version returns the ncnn library version string.
func Version() string {
	return C.GoString(C.ncnn_version())
}

// Net holds a loaded ncnn model.
// A Net is safe for concurrent use by multiple Extractors.
type Net struct {
	net C.ncnn_net_t
}

// NewNet loads a model from .param and .bin files on disk.
func NewNet(paramPath, binPath string) (*Net, error) {
	n := &Net{net: C.ncnn_net_create()}
	if n.net == nil {
		return nil, fmt.Errorf("ncnn: net_create failed")
	}

	cParam := C.CString(paramPath)
	defer C.free(unsafe.Pointer(cParam))
	if ret := C.ncnn_net_load_param(n.net, cParam); ret != 0 {
		C.ncnn_net_destroy(n.net)
		return nil, fmt.Errorf("ncnn: load_param %q: %d", paramPath, ret)
	}

	cBin := C.CString(binPath)
	defer C.free(unsafe.Pointer(cBin))
	if ret := C.ncnn_net_load_model(n.net, cBin); ret != 0 {
		C.ncnn_net_destroy(n.net)
		return nil, fmt.Errorf("ncnn: load_model %q: %d", binPath, ret)
	}

	runtime.SetFinalizer(n, (*Net).Close)
	return n, nil
}

// NewNetFromMemory loads a model from in-memory .param and .bin data.
func NewNetFromMemory(paramData, binData []byte, opts ...*Option) (*Net, error) {
	if len(paramData) == 0 {
		return nil, fmt.Errorf("ncnn: empty param data")
	}
	if len(binData) == 0 {
		return nil, fmt.Errorf("ncnn: empty bin data")
	}
	if len(binData) < minModelBinDataBytes {
		return nil, fmt.Errorf("ncnn: bin data too short: %d bytes", len(binData))
	}

	n := &Net{net: C.ncnn_net_create()}
	if n.net == nil {
		return nil, fmt.Errorf("ncnn: net_create failed")
	}

	for _, opt := range opts {
		if opt != nil && opt.opt != nil {
			C.ncnn_net_set_option(n.net, opt.opt)
		}
	}

	cParam := C.CString(string(paramData))
	defer C.free(unsafe.Pointer(cParam))
	if ret := C.ncnn_net_load_param_memory(n.net, cParam); ret != 0 {
		C.ncnn_net_destroy(n.net)
		return nil, fmt.Errorf("ncnn: load_param_memory: %d", ret)
	}

	if ret := C.ncnn_net_load_model_memory(n.net, (*C.uchar)(unsafe.Pointer(&binData[0]))); ret < 0 {
		C.ncnn_net_destroy(n.net)
		return nil, fmt.Errorf("ncnn: load_model_memory: %d", ret)
	}

	runtime.SetFinalizer(n, (*Net).Close)
	return n, nil
}

// SetOption applies a configured Option to this Net.
func (n *Net) SetOption(opt *Option) {
	if n == nil || opt == nil || n.net == nil || opt.opt == nil {
		return
	}
	C.ncnn_net_set_option(n.net, opt.opt)
}

// Option configures inference behavior for a Net.
type Option struct {
	opt C.ncnn_option_t
}

// NewOption creates a new Option with default settings.
func NewOption() *Option {
	opt := C.ncnn_option_create()
	if opt == nil {
		return nil
	}
	o := &Option{opt: opt}
	runtime.SetFinalizer(o, (*Option).Close)
	return o
}

// SetFP16 enables or disables FP16 optimizations.
func (o *Option) SetFP16(enabled bool) *Option {
	if o == nil || o.opt == nil {
		return o
	}
	v := C.int(0)
	if enabled {
		v = 1
	}
	C.ncnn_option_set_use_fp16_packed(o.opt, v)
	C.ncnn_option_set_use_fp16_storage(o.opt, v)
	C.ncnn_option_set_use_fp16_arithmetic(o.opt, v)
	return o
}

// SetNumThreads sets the number of CPU threads for inference.
func (o *Option) SetNumThreads(n int) *Option {
	if o == nil || o.opt == nil {
		return o
	}
	if n < 1 {
		n = 1
	}
	C.ncnn_option_set_num_threads(o.opt, C.int(n))
	return o
}

// Close releases option resources.
func (o *Option) Close() error {
	if o != nil && o.opt != nil {
		C.ncnn_option_destroy(o.opt)
		o.opt = nil
		runtime.SetFinalizer(o, nil)
	}
	return nil
}

// NewExtractor creates a new inference session for this Net.
func (n *Net) NewExtractor() (*Extractor, error) {
	if n == nil || n.net == nil {
		return nil, fmt.Errorf("ncnn: nil net")
	}
	ex := C.ncnn_extractor_create(n.net)
	if ex == nil {
		return nil, fmt.Errorf("ncnn: extractor_create failed")
	}
	e := &Extractor{ex: ex}
	runtime.SetFinalizer(e, (*Extractor).Close)
	return e, nil
}

// Close releases the ncnn network resources.
func (n *Net) Close() error {
	if n != nil && n.net != nil {
		C.ncnn_net_destroy(n.net)
		n.net = nil
		runtime.SetFinalizer(n, nil)
	}
	return nil
}

// Extractor runs inference on a loaded Net.
type Extractor struct {
	ex C.ncnn_extractor_t
}

// SetInput feeds a Mat as input to the named blob.
func (e *Extractor) SetInput(name string, mat *Mat) error {
	if e == nil || e.ex == nil {
		return fmt.Errorf("ncnn: extractor is nil")
	}
	if mat == nil || mat.mat == nil {
		return fmt.Errorf("ncnn: input mat is nil")
	}
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	if ret := C.ncnn_extractor_input(e.ex, cName, mat.mat); ret != 0 {
		return fmt.Errorf("ncnn: extractor_input %q: %d", name, ret)
	}
	return nil
}

// Extract runs inference and returns the output Mat for the named blob.
func (e *Extractor) Extract(name string) (*Mat, error) {
	if e == nil || e.ex == nil {
		return nil, fmt.Errorf("ncnn: extractor is nil")
	}
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var m C.ncnn_mat_t
	if ret := C.ncnn_extractor_extract(e.ex, cName, &m); ret != 0 {
		return nil, fmt.Errorf("ncnn: extractor_extract %q: %d", name, ret)
	}

	mat := &Mat{mat: m}
	runtime.SetFinalizer(mat, (*Mat).Close)
	return mat, nil
}

// SetOption applies a configured Option to this extractor.
func (e *Extractor) SetOption(opt *Option) {
	if e == nil || e.ex == nil || opt == nil || opt.opt == nil {
		return
	}
	C.ncnn_extractor_set_option(e.ex, opt.opt)
}

// Close releases extractor resources.
func (e *Extractor) Close() error {
	if e != nil && e.ex != nil {
		C.ncnn_extractor_destroy(e.ex)
		e.ex = nil
		runtime.SetFinalizer(e, nil)
	}
	return nil
}

// Mat is an N-dimensional tensor.
type Mat struct {
	mat C.ncnn_mat_t
}

// NewMat2D creates a 2D Mat and copies float32 data into C-managed memory.
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

	mat := C.ncnn_mat_create_2d(
		C.int(w), C.int(h),
		nil,
	)
	if mat == nil {
		return nil, fmt.Errorf("ncnn: mat_create_2d failed")
	}

	ptr := C.ncnn_mat_get_data(mat)
	if ptr == nil {
		C.ncnn_mat_destroy(mat)
		return nil, fmt.Errorf("ncnn: mat_create_2d returned nil data pointer")
	}

	bytes := C.size_t(need) * C.size_t(unsafe.Sizeof(float32(0)))
	C.memcpy(ptr, unsafe.Pointer(&data[0]), bytes)

	m := &Mat{mat: mat}
	runtime.SetFinalizer(m, (*Mat).Close)
	return m, nil
}

// NewMat3D creates a 3D Mat and copies float32 data into C-managed memory.
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

	mat := C.ncnn_mat_create_3d(
		C.int(w), C.int(h), C.int(c),
		nil,
	)
	if mat == nil {
		return nil, fmt.Errorf("ncnn: mat_create_3d failed")
	}

	ptr := C.ncnn_mat_get_data(mat)
	if ptr == nil {
		C.ncnn_mat_destroy(mat)
		return nil, fmt.Errorf("ncnn: mat_create_3d returned nil data pointer")
	}

	bytes := C.size_t(need) * C.size_t(unsafe.Sizeof(float32(0)))
	C.memcpy(ptr, unsafe.Pointer(&data[0]), bytes)

	m := &Mat{mat: mat}
	runtime.SetFinalizer(m, (*Mat).Close)
	return m, nil
}

// W returns the width of the Mat.
func (m *Mat) W() int {
	if m == nil || m.mat == nil {
		return 0
	}
	return int(C.ncnn_mat_get_w(m.mat))
}

// H returns the height of the Mat.
func (m *Mat) H() int {
	if m == nil || m.mat == nil {
		return 0
	}
	return int(C.ncnn_mat_get_h(m.mat))
}

// C returns the channel count of the Mat.
func (m *Mat) C() int {
	if m == nil || m.mat == nil {
		return 0
	}
	return int(C.ncnn_mat_get_c(m.mat))
}

// FloatData copies Mat data into a new float32 slice.
func (m *Mat) FloatData() []float32 {
	if m == nil || m.mat == nil {
		return nil
	}
	ptr := C.ncnn_mat_get_data(m.mat)
	if ptr == nil {
		return nil
	}
	n := m.W() * m.H() * m.C()
	if n <= 0 {
		n = m.W()
	}
	if n <= 0 {
		return nil
	}
	out := make([]float32, n)
	C.memcpy(unsafe.Pointer(&out[0]), ptr, C.size_t(n*4))
	return out
}

// Close releases Mat resources.
func (m *Mat) Close() error {
	if m != nil && m.mat != nil {
		C.ncnn_mat_destroy(m.mat)
		m.mat = nil
		runtime.SetFinalizer(m, nil)
	}
	return nil
}
