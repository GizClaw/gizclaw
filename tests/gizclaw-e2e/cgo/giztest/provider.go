package main

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"maps"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"unsafe"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// statusOK and statusInvalidArgument mirror gzc_status_t across the bridge.
const (
	statusOK              = 0
	statusInvalidArgument = -1
)

// clientRPCProvider answers the server-initiated client.* methods a document
// scripted through client_rpc steps and counts the calls each one received.
//
// The C SDK invokes it from the poll-owning thread; the counters are read from
// the runner goroutine, so both are atomic.
type clientRPCProvider struct {
	mu        sync.Mutex
	responses map[string]any
	calls     map[string]*atomic.Int64
}

func newClientRPCProvider() *clientRPCProvider {
	return &clientRPCProvider{responses: map[string]any{}, calls: map[string]*atomic.Int64{}}
}

// install scripts one method's response. A response carrying `error_code`
// makes the provider answer with that structured RPC error.
func (p *clientRPCProvider) install(method string, response any) error {
	if _, err := lookupMethod(method); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responses[method] = response
	p.calls[method] = &atomic.Int64{}
	return nil
}

// installed reports whether the document scripted method.
func (p *clientRPCProvider) installed(method string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.responses[method]
	return ok
}

// callCount reports how many times the Server has invoked method.
func (p *clientRPCProvider) callCount(method string) int64 {
	p.mu.Lock()
	counter := p.calls[method]
	p.mu.Unlock()
	if counter == nil {
		return 0
	}
	return counter.Load()
}

func (p *clientRPCProvider) lookup(method string) (any, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	response, ok := p.responses[method]
	if ok {
		p.calls[method].Add(1)
	}
	return response, ok
}

// errorResponse reads a scripted structured RPC error, matching the Go
// runner's `response: {error_code: ...}` form.
func errorResponse(response any) (int32, string, bool) {
	object, ok := response.(map[string]any)
	if !ok {
		return 0, "", false
	}
	raw, ok := object["error_code"]
	if !ok {
		return 0, "", false
	}
	code, ok := numeric(raw)
	if !ok {
		return 0, "", false
	}
	message, _ := object["error_message"].(string)
	if message == "" {
		message = "scripted client RPC error"
	}
	return int32(code), message, true
}

func numeric(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}

// answer builds the encoded response payload for one inbound client.* call.
func (p *clientRPCProvider) answer(id rpcpb.RpcMethod, requestPayload []byte) ([]byte, int32, string, error) {
	name, info, err := lookupMethodByID(id)
	if err != nil {
		return nil, 0, "", err
	}
	response, ok := p.lookup(name)
	if !ok {
		// An unscripted method must look unimplemented, so the Server maps it
		// to 501 DEVICE_UNSUPPORTED exactly as it does for real firmware.
		return nil, int32(rpcpb.RpcErrorCode_RPC_ERROR_CODE_METHOD_NOT_FOUND), "method not found", nil
	}
	if code, message, scripted := errorResponse(response); scripted {
		return nil, code, message, nil
	}
	value := response
	if name == "client.device.volume.set" {
		value, err = echoVolume(info.request, requestPayload, response)
		if err != nil {
			return nil, 0, "", err
		}
	}
	payload, err := encodePayload(info.response, value)
	if err != nil {
		return nil, 0, "", err
	}
	return payload, 0, "", nil
}

// echoVolume overlays the requested level and mute state onto the scripted
// status, so an HTTP volume round trip observes what it asked for.
func echoVolume(requestMessage string, requestPayload []byte, response any) (any, error) {
	request, err := decodePayload(requestMessage, requestPayload)
	if err != nil {
		return nil, err
	}
	status := map[string]any{}
	if object, ok := response.(map[string]any); ok {
		maps.Copy(status, object)
	}
	if level, ok := request["level"]; ok {
		status["volume"] = level
	}
	if muted, ok := request["muted"]; ok {
		status["muted"] = muted
	}
	return status, nil
}

//export gztGoProvider
func gztGoProvider(
	handle C.ulonglong,
	method C.int,
	requestPayload unsafe.Pointer,
	requestPayloadLen C.size_t,
	outPayload *unsafe.Pointer,
	outPayloadLen *C.size_t,
	outErrorCode *C.int,
	outErrorMessage *C.char,
	outErrorMessageCap C.size_t,
) C.int {
	provider, ok := cgo.Handle(handle).Value().(*clientRPCProvider)
	if !ok {
		return statusInvalidArgument
	}
	var request []byte
	if requestPayload != nil && requestPayloadLen > 0 {
		request = C.GoBytes(requestPayload, C.int(requestPayloadLen))
	}
	payload, code, message, err := provider.answer(rpcpb.RpcMethod(method), request)
	if err != nil {
		code = int32(rpcpb.RpcErrorCode_RPC_ERROR_CODE_INTERNAL_ERROR)
		message = fmt.Sprintf("provider failed: %v", err)
		payload = nil
	}
	if code != 0 {
		*outErrorCode = C.int(code)
		writeCString(outErrorMessage, outErrorMessageCap, message)
		return statusOK
	}
	*outErrorCode = 0
	if len(payload) > 0 {
		buffer := C.malloc(C.size_t(len(payload)))
		if buffer == nil {
			return statusInvalidArgument
		}
		C.memcpy(buffer, unsafe.Pointer(&payload[0]), C.size_t(len(payload)))
		*outPayload = buffer
		*outPayloadLen = C.size_t(len(payload))
	}
	return statusOK
}

func writeCString(out *C.char, cap C.size_t, text string) {
	if out == nil || cap == 0 {
		return
	}
	limit := int(cap) - 1
	if len(text) < limit {
		limit = len(text)
	}
	target := unsafe.Slice((*byte)(unsafe.Pointer(out)), int(cap))
	copy(target[:limit], text)
	target[limit] = 0
}
