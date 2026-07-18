//go:build cgo && ((linux && (amd64 || arm64)) || (darwin && (amd64 || arm64)))

package opus

/*
	#include <opus/opus.h>
	#include <stdlib.h>

	static int gizclaw_opus_encoder_set_complexity(OpusEncoder *enc, int complexity) {
		return opus_encoder_ctl(enc, OPUS_SET_COMPLEXITY(complexity));
	}
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"
)

// Version returns the linked libopus version string.
func Version() string {
	return C.GoString(C.opus_get_version_string())
}

// Encoder wraps an Opus encoder instance.
type Encoder struct {
	enc        *C.OpusEncoder
	sampleRate int
	channels   int
}

// NewEncoder creates a new Opus encoder.
func NewEncoder(sampleRate, channels int, app Application) (*Encoder, error) {
	if err := validateSampleRate(sampleRate); err != nil {
		return nil, err
	}
	if err := validateChannels(channels); err != nil {
		return nil, err
	}
	if err := validateApplication(app); err != nil {
		return nil, err
	}

	errCode := C.int(0)
	enc := C.opus_encoder_create(C.opus_int32(sampleRate), C.int(channels), C.int(app), &errCode)
	if enc == nil {
		return nil, codecError("encoder_create", errCode)
	}
	if errCode != C.OPUS_OK {
		C.opus_encoder_destroy(enc)
		return nil, codecError("encoder_create", errCode)
	}

	e := &Encoder{enc: enc, sampleRate: sampleRate, channels: channels}
	runtime.SetFinalizer(e, (*Encoder).Close)
	return e, nil
}

// SampleRate returns encoder sample rate in Hz.
func (e *Encoder) SampleRate() int {
	if e == nil {
		return 0
	}
	return e.sampleRate
}

// Channels returns encoder channel count.
func (e *Encoder) Channels() int {
	if e == nil {
		return 0
	}
	return e.channels
}

// SetComplexity sets libopus encoder complexity in the range [0, 10].
func (e *Encoder) SetComplexity(complexity int) error {
	if e == nil || e.enc == nil {
		return fmt.Errorf("opus: encoder is nil")
	}
	if err := validateComplexity(complexity); err != nil {
		return err
	}
	ret := C.gizclaw_opus_encoder_set_complexity(e.enc, C.int(complexity))
	if ret != C.OPUS_OK {
		return codecError("encoder_set_complexity", ret)
	}
	return nil
}

// Encode encodes one PCM frame into one Opus packet.
func (e *Encoder) Encode(pcm []int16, frameSize int) ([]byte, error) {
	return e.EncodeWithMaxDataBytes(pcm, frameSize, DefaultMaxPacketSize)
}

// EncodeWithMaxDataBytes encodes one PCM frame with caller-specified output cap.
func (e *Encoder) EncodeWithMaxDataBytes(pcm []int16, frameSize, maxDataBytes int) ([]byte, error) {
	if e == nil || e.enc == nil {
		return nil, fmt.Errorf("opus: encoder is nil")
	}
	if err := validateFrameSize(e.sampleRate, frameSize); err != nil {
		return nil, err
	}
	if err := validateMaxDataBytes(maxDataBytes); err != nil {
		return nil, err
	}
	if err := validatePCM(pcm, frameSize, e.channels); err != nil {
		return nil, err
	}

	out := make([]byte, maxDataBytes)
	ret := C.opus_encode(
		e.enc,
		(*C.opus_int16)(unsafe.Pointer(&pcm[0])),
		C.int(frameSize),
		(*C.uchar)(unsafe.Pointer(&out[0])),
		C.opus_int32(maxDataBytes),
	)
	if ret < 0 {
		return nil, codecError("encode", ret)
	}

	return out[:int(ret)], nil
}

// Close releases encoder resources. It is safe to call repeatedly.
func (e *Encoder) Close() error {
	if e != nil && e.enc != nil {
		C.opus_encoder_destroy(e.enc)
		e.enc = nil
		runtime.SetFinalizer(e, nil)
	}
	return nil
}

// Decoder wraps an Opus decoder instance.
type Decoder struct {
	dec        *C.OpusDecoder
	sampleRate int
	channels   int
}

// NewDecoder creates a new Opus decoder.
func NewDecoder(sampleRate, channels int) (*Decoder, error) {
	if err := validateSampleRate(sampleRate); err != nil {
		return nil, err
	}
	if err := validateChannels(channels); err != nil {
		return nil, err
	}

	errCode := C.int(0)
	dec := C.opus_decoder_create(C.opus_int32(sampleRate), C.int(channels), &errCode)
	if dec == nil {
		return nil, codecError("decoder_create", errCode)
	}
	if errCode != C.OPUS_OK {
		C.opus_decoder_destroy(dec)
		return nil, codecError("decoder_create", errCode)
	}

	d := &Decoder{dec: dec, sampleRate: sampleRate, channels: channels}
	runtime.SetFinalizer(d, (*Decoder).Close)
	return d, nil
}

// SampleRate returns decoder sample rate in Hz.
func (d *Decoder) SampleRate() int {
	if d == nil {
		return 0
	}
	return d.sampleRate
}

// Channels returns decoder channel count.
func (d *Decoder) Channels() int {
	if d == nil {
		return 0
	}
	return d.channels
}

// Decode decodes one Opus packet into PCM samples.
// Returned sample count is interleaved and equals decodedSamplesPerChannel * channels.
func (d *Decoder) Decode(packet []byte, frameSize int, fec bool) ([]int16, error) {
	if d == nil || d.dec == nil {
		return nil, fmt.Errorf("opus: decoder is nil")
	}
	if err := validateDecodeFrameSize(d.sampleRate, frameSize); err != nil {
		return nil, err
	}

	need, ok := checkedMul(frameSize, d.channels)
	if !ok {
		return nil, fmt.Errorf("opus: decode frame shape overflow: frameSize=%d channels=%d", frameSize, d.channels)
	}
	out := make([]int16, need)

	var pktPtr *C.uchar
	if len(packet) > 0 {
		pktPtr = (*C.uchar)(unsafe.Pointer(&packet[0]))
	}

	fecFlag := C.int(0)
	if fec {
		fecFlag = 1
	}

	ret := C.opus_decode(
		d.dec,
		pktPtr,
		C.opus_int32(len(packet)),
		(*C.opus_int16)(unsafe.Pointer(&out[0])),
		C.int(frameSize),
		fecFlag,
	)
	if ret < 0 {
		return nil, codecError("decode", ret)
	}

	decoded := int(ret) * d.channels
	if decoded < 0 {
		decoded = 0
	}
	if decoded > len(out) {
		decoded = len(out)
	}
	return out[:decoded], nil
}

// Close releases decoder resources. It is safe to call repeatedly.
func (d *Decoder) Close() error {
	if d != nil && d.dec != nil {
		C.opus_decoder_destroy(d.dec)
		d.dec = nil
		runtime.SetFinalizer(d, nil)
	}
	return nil
}

func codecError(operation string, code C.int) error {
	msg := C.GoString(C.opus_strerror(code))
	if msg == "" {
		msg = "unknown error"
	}
	return fmt.Errorf("opus: %s failed: %s (%d)", operation, msg, int(code))
}
