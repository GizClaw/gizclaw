package main

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"time"
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
func errorResponse(response any) (int32, string, error) {
	object, ok := response.(map[string]any)
	if !ok {
		return 0, "", errNotScripted
	}
	raw, ok := object["error_code"]
	if !ok {
		return 0, "", errNotScripted
	}
	code, err := scriptedErrorCode(raw)
	if err != nil {
		return 0, "", err
	}
	message, _ := object["error_message"].(string)
	if message == "" {
		message = "scripted client RPC error"
	}
	return code, message, nil
}

// errNotScripted marks a response that carries no scripted RPC error, so the
// provider encodes it as a normal result instead.
var errNotScripted = errors.New("response carries no error_code")

// scriptedErrorCode reads one RPC error code from a decoded scenario value. A
// YAML document decodes a negative code as int and a non-negative one as
// uint64, and a JSON round trip decodes either as float64, so every integral
// form is accepted. A value that is not integral, or that does not fit the
// int32 wire field, is rejected rather than silently becoming another code.
func scriptedErrorCode(raw any) (int32, error) {
	var value int64
	switch v := raw.(type) {
	case int:
		value = int64(v)
	case int32:
		return v, nil
	case int64:
		value = v
	case uint:
		if uint64(v) > math.MaxInt64 {
			return 0, fmt.Errorf("error_code must fit in int32, got %d", v)
		}
		value = int64(v)
	case uint32:
		value = int64(v)
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("error_code must fit in int32, got %d", v)
		}
		value = int64(v)
	case float64:
		if v != math.Trunc(v) {
			return 0, fmt.Errorf("error_code must be an integer, got %v", v)
		}
		value = int64(v)
	default:
		return 0, fmt.Errorf("error_code must be an integer, got %T", raw)
	}
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, fmt.Errorf("error_code must fit in int32, got %d", value)
	}
	return int32(value), nil
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
	switch code, message, scriptErr := errorResponse(response); {
	case scriptErr == nil:
		return nil, code, message, nil
	case !errors.Is(scriptErr, errNotScripted):
		return nil, 0, "", scriptErr
	}
	value, delay, err := scriptedDelay(response)
	if err != nil {
		return nil, 0, "", err
	}
	if delay > 0 {
		// The Server is waiting on this RPC, so a slow device is a slow
		// answer. Blocking the poll thread is what makes the wait observable.
		time.Sleep(delay)
	}
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

// maxScriptedDelayMs bounds a scripted device delay. Node clamps a larger
// setTimeout delay to a single millisecond, which would turn a scenario
// written to exercise a timeout into one that passes on an immediate answer.
// Every runner rejects the same values so a document behaves identically.
const maxScriptedDelayMs = 2147483647

// scriptedDelay splits an optional delay_ms out of a scripted response. The
// key is a runner instruction, not a payload field, so it is removed before
// the rest is encoded as the method's response message.
func scriptedDelay(response any) (any, time.Duration, error) {
	object, ok := response.(map[string]any)
	if !ok || object["delay_ms"] == nil {
		return response, 0, nil
	}
	value, err := scriptedErrorCode(object["delay_ms"])
	if err != nil || value < 0 {
		return nil, 0, fmt.Errorf("delay_ms must be a non-negative integer")
	}
	if value > maxScriptedDelayMs {
		return nil, 0, fmt.Errorf("delay_ms must be at most %d", maxScriptedDelayMs)
	}
	remaining := make(map[string]any, len(object))
	maps.Copy(remaining, object)
	delete(remaining, "delay_ms")
	return remaining, time.Duration(value) * time.Millisecond, nil
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
	limit := min(int(cap)-1, len(text))
	target := unsafe.Slice((*byte)(unsafe.Pointer(out)), int(cap))
	copy(target[:limit], text)
	target[limit] = 0
}
