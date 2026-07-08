package rpcapi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestFrameRequestResponseRoundTrip(t *testing.T) {
	var frameBuf bytes.Buffer
	if err := WriteFrame(&frameBuf, Frame{Type: FrameTypeBinary, Payload: []byte("payload")}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	frame, err := ReadFrame(&frameBuf)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if frame.Type != FrameTypeBinary || string(frame.Payload) != "payload" {
		t.Fatalf("ReadFrame() = %+v", frame)
	}

	var reqParams RPCRequest_Params
	if err := reqParams.FromPingRequest(PingRequest{ClientSendTime: 123}); err != nil {
		t.Fatalf("FromPingRequest() error = %v", err)
	}
	var reqBuf bytes.Buffer
	req := &RPCRequest{
		V:      RPCVersionV1,
		Id:     "req-1",
		Method: RPCMethodAllPing,
		Params: &reqParams,
	}
	if err := WriteRequest(&reqBuf, req); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	gotReq, err := ReadRequest(&reqBuf)
	if err != nil {
		t.Fatalf("ReadRequest() error = %v", err)
	}
	if gotReq.Id != req.Id || gotReq.Method != RPCMethodAllPing || gotReq.Params == nil {
		t.Fatalf("ReadRequest() = %+v", gotReq)
	}
	gotReqParams, err := gotReq.Params.AsPingRequest()
	if err != nil {
		t.Fatalf("AsPingRequest() error = %v", err)
	}
	if gotReqParams.ClientSendTime != 123 {
		t.Fatalf("AsPingRequest().ClientSendTime = %d", gotReqParams.ClientSendTime)
	}

	var respResult RPCResponse_Result
	if err := respResult.FromPingResponse(PingResponse{ServerTime: 456}); err != nil {
		t.Fatalf("FromPingResponse() error = %v", err)
	}
	var respBuf bytes.Buffer
	resp := &RPCResponse{
		V:      RPCVersionV1,
		Id:     "req-1",
		Result: &respResult,
	}
	if err := WriteResponse(&respBuf, resp); err != nil {
		t.Fatalf("WriteResponse() error = %v", err)
	}
	gotResp, err := ReadResponse(&respBuf)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if gotResp.Id != resp.Id || gotResp.Result == nil {
		t.Fatalf("ReadResponse() = %+v", gotResp)
	}
	gotRespResult, err := gotResp.Result.AsPingResponse()
	if err != nil {
		t.Fatalf("AsPingResponse() error = %v", err)
	}
	if gotRespResult.ServerTime != 456 {
		t.Fatalf("AsPingResponse().ServerTime = %d", gotRespResult.ServerTime)
	}
}

func TestRPCUnionTypes(t *testing.T) {
	var pingParams RPCRequest_Params
	if err := pingParams.MergePingRequest(PingRequest{ClientSendTime: 100}); err != nil {
		t.Fatalf("MergePingRequest() error = %v", err)
	}
	if got, err := pingParams.AsPingRequest(); err != nil || got.ClientSendTime != 100 {
		t.Fatalf("AsPingRequest() = %+v, %v", got, err)
	}

	assertRequestUnion(t, "ServerPutInfo", ServerPutInfoRequest{Name: stringPtr("peer-1")}, (*RPCRequest_Params).FromServerPutInfoRequest, RPCRequest_Params.AsServerPutInfoRequest, (*RPCRequest_Params).MergeServerPutInfoRequest)
	assertRequestUnion(t, "ServerGetRuntime", ServerGetRuntimeRequest{}, (*RPCRequest_Params).FromServerGetRuntimeRequest, RPCRequest_Params.AsServerGetRuntimeRequest, (*RPCRequest_Params).MergeServerGetRuntimeRequest)
	assertRequestUnion(t, "ClientGetInfo", ClientGetInfoRequest{}, (*RPCRequest_Params).FromClientGetInfoRequest, RPCRequest_Params.AsClientGetInfoRequest, (*RPCRequest_Params).MergeClientGetInfoRequest)
	assertRequestUnion(t, "ClientGetIdentifiers", ClientGetIdentifiersRequest{}, (*RPCRequest_Params).FromClientGetIdentifiersRequest, RPCRequest_Params.AsClientGetIdentifiersRequest, (*RPCRequest_Params).MergeClientGetIdentifiersRequest)
	assertRequestUnion(t, "ServerGetInfo", ServerGetInfoRequest{}, (*RPCRequest_Params).FromServerGetInfoRequest, RPCRequest_Params.AsServerGetInfoRequest, (*RPCRequest_Params).MergeServerGetInfoRequest)

	var pingResult RPCResponse_Result
	if err := pingResult.MergePingResponse(PingResponse{ServerTime: 200}); err != nil {
		t.Fatalf("MergePingResponse() error = %v", err)
	}
	if got, err := pingResult.AsPingResponse(); err != nil || got.ServerTime != 200 {
		t.Fatalf("AsPingResponse() = %+v, %v", got, err)
	}

	now := time.Unix(100, 0).UTC()
	assertResponseUnion(t, "ServerGetInfo", ServerGetInfoResponse{Name: stringPtr("peer-1")}, (*RPCResponse_Result).FromServerGetInfoResponse, RPCResponse_Result.AsServerGetInfoResponse, (*RPCResponse_Result).MergeServerGetInfoResponse)
	assertResponseUnion(t, "ServerPutInfo", ServerPutInfoResponse{Name: stringPtr("peer-2")}, (*RPCResponse_Result).FromServerPutInfoResponse, RPCResponse_Result.AsServerPutInfoResponse, (*RPCResponse_Result).MergeServerPutInfoResponse)
	assertResponseUnion(t, "ServerGetRuntime", ServerGetRuntimeResponse{Online: true, LastSeenAt: now}, (*RPCResponse_Result).FromServerGetRuntimeResponse, RPCResponse_Result.AsServerGetRuntimeResponse, (*RPCResponse_Result).MergeServerGetRuntimeResponse)
	assertResponseUnion(t, "ClientGetInfo", ClientGetInfoResponse{Name: stringPtr("peer-1")}, (*RPCResponse_Result).FromClientGetInfoResponse, RPCResponse_Result.AsClientGetInfoResponse, (*RPCResponse_Result).MergeClientGetInfoResponse)
	assertResponseUnion(t, "ClientGetIdentifiers", ClientGetIdentifiersResponse{Sn: stringPtr("sn-1")}, (*RPCResponse_Result).FromClientGetIdentifiersResponse, RPCResponse_Result.AsClientGetIdentifiersResponse, (*RPCResponse_Result).MergeClientGetIdentifiersResponse)
}

func TestMethodPayloadsUseProtobufBytes(t *testing.T) {
	var params RPCRequest_Params
	if err := params.FromPingRequest(PingRequest{ClientSendTime: 123}); err != nil {
		t.Fatalf("FromPingRequest() error = %v", err)
	}
	reqMsg, err := EncodeRPCRequest(&RPCRequest{
		V:      RPCVersionV1,
		Id:     "req-1",
		Method: RPCMethodAllPing,
		Params: &params,
	})
	if err != nil {
		t.Fatalf("EncodeRPCRequest() error = %v", err)
	}
	if bytes.Contains(reqMsg.GetPayload(), []byte("client_send_time")) {
		t.Fatalf("request payload is JSON: %q", reqMsg.GetPayload())
	}
	var protoReq rpcpb.PingRequest
	if err := proto.Unmarshal(reqMsg.GetPayload(), &protoReq); err != nil {
		t.Fatalf("protobuf request payload unmarshal error = %v", err)
	}
	if protoReq.GetClientSendTime() != 123 {
		t.Fatalf("protobuf request payload = %+v", protoReq)
	}

	var result RPCResponse_Result
	if err := result.FromPingResponse(PingResponse{ServerTime: 456}); err != nil {
		t.Fatalf("FromPingResponse() error = %v", err)
	}
	respMsg, err := EncodeRPCResponseForMethod(RPCMethodAllPing, &RPCResponse{
		V:      RPCVersionV1,
		Id:     "req-1",
		Result: &result,
	})
	if err != nil {
		t.Fatalf("EncodeRPCResponseForMethod() error = %v", err)
	}
	if bytes.Contains(respMsg.GetPayload(), []byte("server_time")) {
		t.Fatalf("response payload is JSON: %q", respMsg.GetPayload())
	}
	var protoResp rpcpb.PingResponse
	if err := proto.Unmarshal(respMsg.GetPayload(), &protoResp); err != nil {
		t.Fatalf("protobuf response payload unmarshal error = %v", err)
	}
	if protoResp.GetServerTime() != 456 {
		t.Fatalf("protobuf response payload = %+v", protoResp)
	}

	decoded, err := DecodeRPCResponseForMethod(RPCMethodAllPing, respMsg)
	if err != nil {
		t.Fatalf("DecodeRPCResponseForMethod() error = %v", err)
	}
	got, err := decoded.Result.AsPingResponse()
	if err != nil {
		t.Fatalf("AsPingResponse() error = %v", err)
	}
	if got.ServerTime != 456 {
		t.Fatalf("decoded response = %+v", got)
	}
}

func TestRPCMethodValid(t *testing.T) {
	for _, method := range []RPCMethod{
		RPCMethodAllPing,
		RPCMethodClientInfoGet,
		RPCMethodClientIdentifiersGet,
		RPCMethodServerInfoGet,
		RPCMethodServerInfoPut,
		RPCMethodServerRuntimeGet,
		RPCMethodServerInfoGet,
	} {
		if !method.Valid() {
			t.Fatalf("%s should be valid", method)
		}
	}
	if RPCMethod("peer.unknown").Valid() {
		t.Fatal("unknown RPC method should be invalid")
	}
	if !RPCVersionV1.Valid() {
		t.Fatal("RPC version 1 should be valid")
	}
	if RPCVersion(2).Valid() {
		t.Fatal("unknown RPC version should be invalid")
	}
	for _, code := range []RPCErrorCode{RPCErrorCodeInvalidRequest, RPCErrorCodeMethodNotFound, RPCErrorCodeInvalidParams, RPCErrorCodeInternalError, RPCErrorCodeBadRequest, RPCErrorCodeForbidden, RPCErrorCodeNotFound, RPCErrorCodeConflict} {
		if !code.Valid() {
			t.Fatalf("%d should be valid", code)
		}
	}
	if RPCErrorCode(418).Valid() {
		t.Fatal("unknown RPC error code should be invalid")
	}
}

func TestProtoMethodRegistry(t *testing.T) {
	if err := ValidateProtoMethodRegistry(); err != nil {
		t.Fatalf("ValidateProtoMethodRegistry() error = %v", err)
	}
	protoMethod, err := ProtoMethod(RPCMethodAllPing)
	if err != nil {
		t.Fatalf("ProtoMethod() error = %v", err)
	}
	method, err := MethodFromProto(protoMethod)
	if err != nil {
		t.Fatalf("MethodFromProto() error = %v", err)
	}
	if method != RPCMethodAllPing {
		t.Fatalf("MethodFromProto() = %q, want %q", method, RPCMethodAllPing)
	}
}

func assertRequestUnion[T any](
	t *testing.T,
	name string,
	value T,
	from func(*RPCRequest_Params, T) error,
	as func(RPCRequest_Params) (T, error),
	merge func(*RPCRequest_Params, T) error,
) {
	t.Helper()
	var params RPCRequest_Params
	if err := from(&params, value); err != nil {
		t.Fatalf("From%sRequest() error = %v", name, err)
	}
	if _, err := as(params); err != nil {
		t.Fatalf("As%sRequest() error = %v", name, err)
	}
	if err := merge(&params, value); err != nil {
		t.Fatalf("Merge%sRequest() error = %v", name, err)
	}
}

func assertResponseUnion[T any](
	t *testing.T,
	name string,
	value T,
	from func(*RPCResponse_Result, T) error,
	as func(RPCResponse_Result) (T, error),
	merge func(*RPCResponse_Result, T) error,
) {
	t.Helper()
	var result RPCResponse_Result
	if err := from(&result, value); err != nil {
		t.Fatalf("From%sResponse() error = %v", name, err)
	}
	if _, err := as(result); err != nil {
		t.Fatalf("As%sResponse() error = %v", name, err)
	}
	if err := merge(&result, value); err != nil {
		t.Fatalf("Merge%sResponse() error = %v", name, err)
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestWriteFramePropagatesHeaderWriteError(t *testing.T) {
	writeErr := errors.New("write failed")
	if err := WriteFrame(errorWriter{err: writeErr}, Frame{Type: FrameTypeJSON, Payload: []byte("payload")}); !errors.Is(err, writeErr) {
		t.Fatalf("WriteFrame() err = %v, want %v", err, writeErr)
	}
}

func TestWriteFrameUsesBuffersWriter(t *testing.T) {
	var writer recordingBuffersWriter
	if err := WriteFrame(&writer, Frame{Type: FrameTypeBinary, Payload: []byte("payload")}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	if writer.writeCalls != 0 {
		t.Fatalf("Write() calls = %d, want 0", writer.writeCalls)
	}
	if writer.writeBuffersCalls != 1 {
		t.Fatalf("WriteBuffers() calls = %d, want 1", writer.writeBuffersCalls)
	}
	frame, err := ReadFrame(bytes.NewReader(writer.buf.Bytes()))
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if frame.Type != FrameTypeBinary || string(frame.Payload) != "payload" {
		t.Fatalf("ReadFrame() = %+v", frame)
	}
}

func TestWriteFramePropagatesBuffersWriterError(t *testing.T) {
	writeErr := errors.New("write buffers failed")
	writer := recordingBuffersWriter{writeBuffersErr: writeErr}
	err := WriteFrame(&writer, Frame{Type: FrameTypeBinary, Payload: []byte("payload")})
	if !errors.Is(err, writeErr) {
		t.Fatalf("WriteFrame() err = %v, want %v", err, writeErr)
	}
	if writer.writeCalls != 0 {
		t.Fatalf("Write() calls = %d, want 0", writer.writeCalls)
	}
	if writer.writeBuffersCalls != 1 {
		t.Fatalf("WriteBuffers() calls = %d, want 1", writer.writeBuffersCalls)
	}
}

func TestWriteFrameRejectsBuffersWriterShortWrite(t *testing.T) {
	shortWrite := int64(3)
	writer := recordingBuffersWriter{writeBuffersResult: &shortWrite}
	err := WriteFrame(&writer, Frame{Type: FrameTypeBinary, Payload: []byte("payload")})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("WriteFrame() err = %v, want %v", err, io.ErrShortWrite)
	}
	if writer.writeCalls != 0 {
		t.Fatalf("Write() calls = %d, want 0", writer.writeCalls)
	}
	if writer.writeBuffersCalls != 1 {
		t.Fatalf("WriteBuffers() calls = %d, want 1", writer.writeBuffersCalls)
	}
}

type recordingBuffersWriter struct {
	buf                bytes.Buffer
	writeCalls         int
	writeBuffersCalls  int
	writeBuffersResult *int64
	writeBuffersErr    error
}

func (w *recordingBuffersWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	return w.buf.Write(p)
}

func (w *recordingBuffersWriter) WriteBuffers(buffers net.Buffers) (int64, error) {
	w.writeBuffersCalls++
	if w.writeBuffersErr != nil || w.writeBuffersResult != nil {
		var n int64
		if w.writeBuffersResult != nil {
			n = *w.writeBuffersResult
		}
		return n, w.writeBuffersErr
	}
	var total int64
	for _, buf := range buffers {
		n, err := w.buf.Write(buf)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func TestReadRequestAndResponseRejectInvalidProtobuf(t *testing.T) {
	var reqBuf bytes.Buffer
	if err := WriteFrame(&reqBuf, Frame{Type: FrameTypeBinary, Payload: []byte("{")}); err != nil {
		t.Fatalf("WriteFrame(request) error = %v", err)
	}
	if _, err := ReadRequest(&reqBuf); err == nil {
		t.Fatal("ReadRequest() should fail for invalid protobuf")
	}

	var respBuf bytes.Buffer
	if err := WriteFrame(&respBuf, Frame{Type: FrameTypeBinary, Payload: []byte("{")}); err != nil {
		t.Fatalf("WriteFrame(response) error = %v", err)
	}
	if _, err := ReadResponse(&respBuf); err == nil {
		t.Fatal("ReadResponse() should fail for invalid protobuf")
	}
}

func TestReadRequestAndResponseRejectNonProtobufFrames(t *testing.T) {
	var reqBuf bytes.Buffer
	if err := WriteFrame(&reqBuf, Frame{Type: FrameTypeJSON, Payload: []byte("{}")}); err != nil {
		t.Fatalf("WriteFrame(request) error = %v", err)
	}
	if _, err := ReadRequest(&reqBuf); err == nil || err.Error() != "rpc: unmarshal request: rpc: expected protobuf binary frame, got type 1" {
		t.Fatalf("ReadRequest() err = %v", err)
	}

	var respBuf bytes.Buffer
	if err := WriteFrame(&respBuf, Frame{Type: FrameTypeText, Payload: []byte("{}")}); err != nil {
		t.Fatalf("WriteFrame(response) error = %v", err)
	}
	if _, err := ReadResponse(&respBuf); err == nil || err.Error() != "rpc: unmarshal response: rpc: expected protobuf binary frame, got type 3" {
		t.Fatalf("ReadResponse() err = %v", err)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

func TestWriteFrameRejectsOversizedFrame(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), MaxFrameSize+1)
	var buf bytes.Buffer
	err := WriteFrame(&buf, Frame{Type: FrameTypeBinary, Payload: payload})
	if err == nil || err.Error() != "rpc: frame too large: 65536" {
		t.Fatalf("WriteFrame() err = %v", err)
	}
}

func TestReadFrameRejectsUnknownType(t *testing.T) {
	var buf bytes.Buffer
	var hdr [4]byte
	binary.LittleEndian.PutUint16(hdr[0:2], 0)
	binary.LittleEndian.PutUint16(hdr[2:4], 99)
	if _, err := buf.Write(hdr[:]); err != nil {
		t.Fatalf("Write(header) error = %v", err)
	}

	_, err := ReadFrame(&buf)
	if err == nil || err.Error() != "rpc: unknown frame type: 99" {
		t.Fatalf("ReadFrame() err = %v", err)
	}
}

func TestReadFrameRejectsTruncatedPayload(t *testing.T) {
	var buf bytes.Buffer
	var hdr [4]byte
	binary.LittleEndian.PutUint16(hdr[0:2], 4)
	binary.LittleEndian.PutUint16(hdr[2:4], uint16(FrameTypeText))
	if _, err := buf.Write(hdr[:]); err != nil {
		t.Fatalf("Write(header) error = %v", err)
	}
	if _, err := buf.Write([]byte("xy")); err != nil {
		t.Fatalf("Write(payload) error = %v", err)
	}

	_, err := ReadFrame(&buf)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadFrame() err = %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

func TestReadWriteFrames(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFrames(&buf, func(yield func(Frame, error) bool) {
		yield(Frame{Type: FrameTypeJSON, Payload: []byte("{}")}, nil)
		yield(Frame{Type: FrameTypeText, Payload: []byte("hello")}, nil)
		yield(Frame{Type: FrameTypeBinary, Payload: []byte{1, 2, 3}}, nil)
	})
	if err != nil {
		t.Fatalf("WriteFrames() error = %v", err)
	}
	if err := WriteEOS(&buf); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}

	var got []Frame
	for frame, err := range ReadFrames(&buf) {
		if err != nil {
			t.Fatalf("ReadFrames() error = %v", err)
		}
		got = append(got, frame)
	}
	if len(got) != 3 {
		t.Fatalf("ReadFrames() got %d frames", len(got))
	}
	if got[0].Type != FrameTypeJSON || string(got[0].Payload) != "{}" {
		t.Fatalf("frame[0] = %+v", got[0])
	}
	if got[1].Type != FrameTypeText || string(got[1].Payload) != "hello" {
		t.Fatalf("frame[1] = %+v", got[1])
	}
	if got[2].Type != FrameTypeBinary || !bytes.Equal(got[2].Payload, []byte{1, 2, 3}) {
		t.Fatalf("frame[2] = %+v", got[2])
	}
}

func TestWriteFramesPropagatesSequenceError(t *testing.T) {
	seqErr := errors.New("sequence failed")
	var buf bytes.Buffer
	err := WriteFrames(&buf, func(yield func(Frame, error) bool) {
		yield(Frame{}, seqErr)
	})
	if !errors.Is(err, seqErr) {
		t.Fatalf("WriteFrames() err = %v, want %v", err, seqErr)
	}
}

func TestReadWriteResponses(t *testing.T) {
	responses := []string{"one", "two"}
	var buf bytes.Buffer
	err := WriteResponses(&buf, func(yield func(*RPCResponse, error) bool) {
		for _, id := range responses {
			if !yield(&RPCResponse{V: RPCVersionV1, Id: id}, nil) {
				return
			}
		}
	})
	if err != nil {
		t.Fatalf("WriteResponses() error = %v", err)
	}

	var got []string
	for resp, err := range ReadResponses(&buf) {
		if err != nil {
			t.Fatalf("ReadResponses() error = %v", err)
		}
		got = append(got, resp.Id)
	}
	if len(got) != len(responses) || got[0] != "one" || got[1] != "two" {
		t.Fatalf("ReadResponses() ids = %v", got)
	}
}

func TestWriteResponsesPropagatesSequenceError(t *testing.T) {
	seqErr := errors.New("response failed")
	var buf bytes.Buffer
	err := WriteResponses(&buf, func(yield func(*RPCResponse, error) bool) {
		yield(nil, seqErr)
	})
	if !errors.Is(err, seqErr) {
		t.Fatalf("WriteResponses() err = %v, want %v", err, seqErr)
	}
}

func TestReadResponsesRejectsInvalidProtobufFrame(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, Frame{Type: FrameTypeBinary, Payload: []byte("{}")}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	for _, err := range ReadResponses(&buf) {
		if err == nil {
			t.Fatalf("ReadResponses() err = %v", err)
		}
		return
	}
	t.Fatal("ReadResponses() did not yield error")
}

func TestReadFramesReturnsEOFBeforeEOS(t *testing.T) {
	var buf bytes.Buffer
	for _, err := range ReadFrames(&buf) {
		if !errors.Is(err, io.EOF) {
			t.Fatalf("ReadFrames() err = %v, want %v", err, io.EOF)
		}
		return
	}
	t.Fatal("ReadFrames() did not yield EOF before EOS")
}

func TestReadFramesStopsOnEOS(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEOS(&buf); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	for range ReadFrames(&buf) {
		t.Fatal("ReadFrames() should not yield EOS")
	}
}

func TestReadWriteEOS(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteEOS(&buf); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}
	if err := ReadEOS(&buf); err != nil {
		t.Fatalf("ReadEOS() error = %v", err)
	}
}

func TestEOSFrameMustBeEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, Frame{Type: FrameTypeEOS, Payload: []byte("x")}); err == nil || err.Error() != "rpc: EOS frame must be empty" {
		t.Fatalf("WriteFrame(EOS payload) err = %v", err)
	}

	var hdr [4]byte
	binary.LittleEndian.PutUint16(hdr[0:2], 1)
	binary.LittleEndian.PutUint16(hdr[2:4], uint16(FrameTypeEOS))
	if _, err := buf.Write(hdr[:]); err != nil {
		t.Fatalf("Write(header) error = %v", err)
	}
	_, err := ReadFrame(&buf)
	if err == nil || err.Error() != "rpc: EOS frame must be empty" {
		t.Fatalf("ReadFrame(EOS payload) err = %v", err)
	}
}

func TestWriteFrameRejectsUnknownType(t *testing.T) {
	var buf bytes.Buffer
	err := WriteFrame(&buf, Frame{Type: FrameType(99), Payload: []byte("payload")})
	if err == nil || err.Error() != "rpc: unknown frame type: 99" {
		t.Fatalf("WriteFrame() err = %v", err)
	}
}

func TestWriteFramePropagatesShortWrite(t *testing.T) {
	err := WriteFrame(shortWriter{}, Frame{Type: FrameTypeJSON, Payload: []byte("payload")})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("WriteFrame() err = %v, want %v", err, io.ErrShortWrite)
	}
}

type shortWriter struct{}

func (shortWriter) Write(_ []byte) (int, error) {
	return 0, nil
}

func TestErrorImplementsErrorAndBuildsRPCResponse(t *testing.T) {
	rpcErr := Error{RequestID: "req-1", Code: RPCErrorCodeInvalidParams, Message: "missing params"}
	var err error = rpcErr
	if err.Error() != "missing params" {
		t.Fatalf("Error() = %q", err.Error())
	}

	errResp := rpcErr.RPCResponse()
	if errResp.V != RPCVersionV1 || errResp.Id != "req-1" || errResp.Error == nil {
		t.Fatalf("RPCResponse() = %+v", errResp)
	}
	if errResp.Error.Code != RPCErrorCodeInvalidParams || errResp.Error.Message != "missing params" {
		t.Fatalf("RPCResponse().Error = %+v", errResp.Error)
	}
}

func TestErrorUsesFallbackMessage(t *testing.T) {
	rpcErr := Error{Code: RPCErrorCode(-1)}
	if rpcErr.Error() != "rpc error -1" {
		t.Fatalf("Error() = %q", rpcErr.Error())
	}

	errResp := rpcErr.RPCResponse()
	if errResp.Error == nil || errResp.Error.Message != "rpc error -1" {
		t.Fatalf("RPCResponse().Error = %+v", errResp.Error)
	}
}
