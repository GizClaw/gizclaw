package main

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"runtime/cgo"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
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
	if method == rpcMethodsGet && response != nil {
		return fmt.Errorf("%s is answered from the installed providers and takes no response", rpcMethodsGet)
	}
	if method == toolInvoke {
		if _, err := scriptedTool(response); err != nil {
			return err
		}
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
	if status := rpcapi.StatusCode(code); !status.Valid() || status == rpcapi.StatusCodeOK {
		return 0, "", fmt.Errorf("error_code must be a canonical status code other than OK, got %d", code)
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

// toolInvoke is answered through a registered C SDK Tool handler rather than
// rpc_provider, and is not advertised by client.rpc.methods.get.
const toolInvoke = "client.tool.invoke"

// scriptedTool reads the Go runner's `{name, result}` Tool response form;
// `unavailable: true` scripts a device without that Tool.
func scriptedTool(response any) (map[string]any, error) {
	object, ok := response.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tool response must be an object")
	}
	if name, _ := object["name"].(string); name == "" {
		return nil, fmt.Errorf("tool response requires name")
	}
	if unavailable, _ := object["unavailable"].(bool); unavailable {
		if _, hasResult := object["result"]; hasResult {
			return nil, fmt.Errorf("unavailable tool response cannot set result")
		}
	}
	return object, nil
}

// toolName reports the scripted Tool name the C session must register, or ""
// when the document scripts no Tool.
func (p *clientRPCProvider) toolName() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	object, _ := p.responses[toolInvoke].(map[string]any)
	name, _ := object["name"].(string)
	return name
}

// rpcMethodsGet is answered from the scripted providers rather than from a
// scripted response, the way every device SDK derives it.
const rpcMethodsGet = "client.rpc.methods.get"

// supportedMethods lists the client.* methods this provider answers, sorted,
// ending with client.rpc.methods.get itself.
func (p *clientRPCProvider) supportedMethods() []any {
	p.mu.Lock()
	names := make([]string, 0, len(p.responses))
	for name := range p.responses {
		if name != rpcMethodsGet && name != toolInvoke {
			names = append(names, name)
		}
	}
	p.mu.Unlock()
	sort.Strings(names)
	methods := make([]any, 0, len(names)+1)
	for _, name := range names {
		methods = append(methods, name)
	}
	return append(methods, rpcMethodsGet)
}

// answer builds the encoded response payload for one inbound client.* call.
func (p *clientRPCProvider) answer(id rpcpb.RpcMethod, requestPayload []byte) ([]byte, int32, string, error) {
	name, info, err := lookupMethodByID(id)
	if err != nil {
		return nil, 0, "", err
	}
	if name == rpcMethodsGet {
		// A real C device's provider answers this whether or not a document
		// scripted it; a step only counts the calls.
		_, _ = p.lookup(name)
		payload, err := encodePayload(info.response, map[string]any{"methods": p.supportedMethods()})
		if err != nil {
			return nil, 0, "", err
		}
		return payload, 0, "", nil
	}
	response, ok := p.lookup(name)
	if !ok {
		// An unscripted method must look unimplemented, so the Server maps it
		// to 501 DEVICE_UNSUPPORTED exactly as it does for real firmware.
		return nil, int32(rpcpb.StatusCode_STATUS_CODE_UNIMPLEMENTED), "method not found", nil
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
	switch name {
	case toolInvoke:
		object, _ := value.(map[string]any)
		if unavailable, _ := object["unavailable"].(bool); unavailable {
			return nil, int32(rpcpb.StatusCode_STATUS_CODE_UNIMPLEMENTED), "Tool unavailable", nil
		}
		data, marshalErr := json.Marshal(object["result"])
		if marshalErr != nil {
			return nil, 0, "", marshalErr
		}
		value = map[string]any{"data_json": string(data)}
	case "client.device.volume.set":
		value, err = echoVolume(info.request, requestPayload, response)
	case "client.device.settings.set":
		value, err = overlaySettings(info.request, requestPayload, value)
	}
	if err != nil {
		return nil, 0, "", err
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

// overlaySettings applies the members a settings patch carries over the
// scripted settings, so an HTTP PATCH round trip observes what it asked for
// next to what it left unchanged. Absent optional members stay absent in the
// decoded patch, so only the sent members replace scripted ones.
func overlaySettings(requestMessage string, requestPayload []byte, response any) (any, error) {
	patch, err := decodePayload(requestMessage, requestPayload)
	if err != nil {
		return nil, err
	}
	settings := map[string]any{}
	if object, ok := response.(map[string]any); ok {
		maps.Copy(settings, object)
	}
	// decodePayload unwraps a request that carries a value; an empty patch
	// decodes to the wrapper with no value, which changes nothing.
	if value, ok := patch["value"]; ok {
		if value == nil {
			return settings, nil
		}
		patch, _ = value.(map[string]any)
	}
	maps.Copy(settings, patch)
	return settings, nil
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
		code = int32(rpcpb.StatusCode_STATUS_CODE_INTERNAL)
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
