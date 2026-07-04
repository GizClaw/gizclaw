//go:build gizclaw_e2e

package internal

/*
#cgo CFLAGS: -I. -I../../../../c/gizwebrtc/include -I../../../../c/gizwebrtc/generated
#include "bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"time"
	"unsafe"

	"runtime/cgo"
)

func backendFromHandle(handle C.uint64_t) *backend {
	if handle == 0 {
		return nil
	}
	h := cgo.Handle(uintptr(handle))
	b, _ := h.Value().(*backend)
	return b
}

//export gzcGoBackendCreate
func gzcGoBackendCreate(identityDir *C.char) C.uint64_t {
	b, err := newBackend(C.GoString(identityDir))
	if err != nil {
		return 0
	}
	return C.uint64_t(cgo.NewHandle(b))
}

//export gzcGoBackendDestroy
func gzcGoBackendDestroy(handle C.uint64_t) {
	b := backendFromHandle(handle)
	if b != nil {
		b.clearCBackend()
		b.close()
	}
	cgo.Handle(uintptr(handle)).Delete()
}

//export gzcGoBackendSetCBackend
func gzcGoBackendSetCBackend(handle C.uint64_t, cBackend *C.gzc_cgo_backend_t) {
	if b := backendFromHandle(handle); b != nil {
		b.setCBackend(unsafe.Pointer(cBackend))
	}
}

//export gzcGoHTTPPost
func gzcGoHTTPPost(handle C.uint64_t, data *C.uint8_t, length C.size_t, outData **C.uint8_t, outLen *C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil || outData == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	answer, err := b.postOffer(ctx, C.GoBytes(unsafe.Pointer(data), C.int(length)))
	if err != nil {
		return C.GZC_ERR_SIGNALING
	}
	mem := C.CBytes(answer)
	*outData = (*C.uint8_t)(mem)
	*outLen = C.size_t(len(answer))
	return C.GZC_OK
}

//export gzcGoPeerCreate
func gzcGoPeerCreate(handle C.uint64_t) C.int {
	if b := backendFromHandle(handle); b != nil {
		if err := b.createPeer(); err != nil {
			return C.GZC_ERR_WEBRTC
		}
		return C.GZC_OK
	}
	return C.GZC_ERR_INVALID_ARGUMENT
}

//export gzcGoPeerStartOffer
func gzcGoPeerStartOffer(handle C.uint64_t, outSDP **C.char, outLen *C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil || outSDP == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	sdp, err := b.startOffer()
	if err != nil {
		return C.GZC_ERR_WEBRTC
	}
	mem := C.CBytes([]byte(sdp))
	*outSDP = (*C.char)(mem)
	*outLen = C.size_t(len(sdp))
	return C.GZC_OK
}

//export gzcGoPeerSetRemoteSDP
func gzcGoPeerSetRemoteSDP(handle C.uint64_t, sdp *C.char, length C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.setRemoteSDP(string(C.GoBytes(unsafe.Pointer(sdp), C.int(length)))); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoPeerCreateDataChannel
func gzcGoPeerCreateDataChannel(handle C.uint64_t, label *C.char, length C.size_t, channelID C.int, ordered C.bool, reliable C.bool) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.createDataChannel(
		string(C.GoBytes(unsafe.Pointer(label), C.int(length))),
		int(channelID),
		bool(ordered),
		bool(reliable),
	); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoPeerPoll
func gzcGoPeerPoll(handle C.uint64_t, timeoutMS C.int) C.int {
	if backendFromHandle(handle) == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if timeoutMS > 0 {
		time.Sleep(time.Duration(timeoutMS) * time.Millisecond)
	}
	return C.GZC_OK
}

//export gzcGoChannelSend
func gzcGoChannelSend(handle C.uint64_t, channelID C.int, data *C.uint8_t, length C.size_t, isText C.bool) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.send(int(channelID), C.GoBytes(unsafe.Pointer(data), C.int(length)), bool(isText)); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoChannelClose
func gzcGoChannelClose(handle C.uint64_t, channelID C.int) {
	if b := backendFromHandle(handle); b != nil {
		b.closeDataChannel(int(channelID))
	}
}

//export gzcGoPeerClose
func gzcGoPeerClose(handle C.uint64_t) {
	if b := backendFromHandle(handle); b != nil {
		b.close()
	}
}
