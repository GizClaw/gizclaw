package main

/*
#cgo CFLAGS: -I. -I../../../../sdk/c/gizclaw/include -I../../../../sdk/c/gizclaw/generated -I../../../../sdk/c/gizclaw_control/include -I../../../../sdk/c/gizclaw_control/src -I../../../../third_party/nanopb/upstream
#include "bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"unsafe"

	_ "github.com/GizClaw/gizclaw-go/sdk/c/gizclaw/cgobackend"
)

const errorBufferSize = 512

// cSession owns one connected device client inside the C SDK. Every method
// must run on the goroutine that owns polling, which the Go client serializes
// through a single worker per Giztest task.
type cSession struct {
	handle   *C.gzt_session_t
	provider cgo.Handle
}

// bridgeError carries a failure the C bridge reported.
type bridgeError struct {
	operation string
	code      int
	message   string
}

func (e *bridgeError) Error() string {
	return fmt.Sprintf("%s: %s (rc=%d)", e.operation, e.message, e.code)
}

func newErrorBuffer() (*C.char, func()) {
	buf := (*C.char)(C.calloc(errorBufferSize, 1))
	return buf, func() { C.free(unsafe.Pointer(buf)) }
}

func bridgeFailure(operation string, rc C.int, buf *C.char) error {
	return &bridgeError{operation: operation, code: int(rc), message: C.GoString(buf)}
}

// openSession dials one device Peer. provider answers server-initiated
// client.* methods and may be nil when the document installs none.
func openSession(endpoint, privateKey string, provider *clientRPCProvider) (*cSession, error) {
	cEndpoint := C.CString(endpoint)
	defer C.free(unsafe.Pointer(cEndpoint))
	cKey := C.CString(privateKey)
	defer C.free(unsafe.Pointer(cKey))
	errbuf, freeErr := newErrorBuffer()
	defer freeErr()

	session := &cSession{}
	var providerHandle C.ulonglong
	if provider != nil {
		session.provider = cgo.NewHandle(provider)
		providerHandle = C.ulonglong(session.provider)
	}
	rc := C.gzt_session_open(cEndpoint, cKey, providerHandle, &session.handle, errbuf, errorBufferSize)
	if rc != 0 {
		if session.provider != 0 {
			session.provider.Delete()
		}
		return nil, bridgeFailure("open C SDK session", rc, errbuf)
	}
	return session, nil
}

func (s *cSession) Close() {
	if s == nil || s.handle == nil {
		return
	}
	C.gzt_session_close(s.handle)
	s.handle = nil
	if s.provider != 0 {
		s.provider.Delete()
		s.provider = 0
	}
}

// Poll advances the transport so inbound RPCs and queued writes make progress.
func (s *cSession) Poll(timeoutMS int) error {
	errbuf, freeErr := newErrorBuffer()
	defer freeErr()
	if rc := C.gzt_session_poll(s.handle, C.int(timeoutMS), errbuf, errorBufferSize); rc != 0 {
		return bridgeFailure("poll C SDK session", rc, errbuf)
	}
	return nil
}

// rpcResult is one settled unary RPC.
type rpcResult struct {
	payload   []byte
	errorCode int32
	message   string
}

// CallRPC sends one encoded server.* request and returns the encoded response
// or the structured RPC error the Server reported.
func (s *cSession) CallRPC(method uint32, payload []byte, timeoutMS int) (rpcResult, error) {
	errbuf, freeErr := newErrorBuffer()
	defer freeErr()
	messageBuf, freeMessage := newErrorBuffer()
	defer freeMessage()

	var request *C.uchar
	if len(payload) > 0 {
		request = (*C.uchar)(unsafe.Pointer(&payload[0]))
	}
	var out *C.uchar
	var outLen C.ulong
	var rpcCode C.int
	rc := C.gzt_session_call_rpc(
		s.handle, C.uint(method), request, C.ulong(len(payload)), C.int(timeoutMS), &out, &outLen,
		&rpcCode, messageBuf, errorBufferSize, errbuf, errorBufferSize)
	if out != nil {
		defer C.gzt_free(unsafe.Pointer(out))
	}
	if rc != 0 {
		return rpcResult{}, bridgeFailure("call RPC", rc, errbuf)
	}
	result := rpcResult{errorCode: int32(rpcCode), message: C.GoString(messageBuf)}
	if out != nil && outLen > 0 {
		result.payload = C.GoBytes(unsafe.Pointer(out), C.int(outLen))
	}
	return result, nil
}

// cControl hosts the controller SDK with its own HTTP backend, so a
// `/gizclaw/v1` request never blocks the device client's poll loop.
type cControl struct{ handle *C.gzt_control_t }

func openControl() (*cControl, error) {
	errbuf, freeErr := newErrorBuffer()
	defer freeErr()
	control := &cControl{}
	if rc := C.gzt_control_open(&control.handle, errbuf, errorBufferSize); rc != 0 {
		return nil, bridgeFailure("open controller SDK host", rc, errbuf)
	}
	return control, nil
}

func (c *cControl) Close() {
	if c == nil || c.handle == nil {
		return
	}
	C.gzt_control_close(c.handle)
	c.handle = nil
}

// controlResult is one settled `/gizclaw/v1` call.
type controlResult struct {
	status int
	body   []byte
	// kind is the gzc_control_error_kind_t the controller SDK classified.
	kind int
}

// ControlRequest sends one controller-SDK request. An unsupported route is
// reported as an error so a step never passes without being executed.
func (c *cControl) Request(baseURL, apiKey, method, path, requestJSON string, timeoutMS int) (controlResult, error) {
	args := []*C.char{
		C.CString(baseURL), C.CString(apiKey), C.CString(method), C.CString(path), C.CString(requestJSON),
	}
	defer func() {
		for _, arg := range args {
			C.free(unsafe.Pointer(arg))
		}
	}()
	errbuf, freeErr := newErrorBuffer()
	defer freeErr()

	var status, kind C.int
	var body *C.uchar
	var bodyLen C.ulong
	rc := C.gzt_control_request(
		c.handle, args[0], args[1], args[2], args[3], args[4], C.int(timeoutMS), &status, &body,
		&bodyLen, &kind, errbuf, errorBufferSize)
	if body != nil {
		defer C.gzt_free(unsafe.Pointer(body))
	}
	if rc != 0 {
		return controlResult{}, bridgeFailure("control request", rc, errbuf)
	}
	result := controlResult{status: int(status), kind: int(kind)}
	if body != nil && bodyLen > 0 {
		result.body = C.GoBytes(unsafe.Pointer(body), C.int(bodyLen))
	}
	return result, nil
}
