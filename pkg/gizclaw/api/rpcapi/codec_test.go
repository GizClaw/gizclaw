package rpcapi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func TestFrameRequestResponseRoundTrip(t *testing.T) {
	var frameBuf bytes.Buffer
	if err := WriteFrame(&frameBuf, []byte("payload")); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	frame, err := ReadFrame(&frameBuf)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if string(frame) != "payload" {
		t.Fatalf("ReadFrame() = %q", frame)
	}

	var reqParams RPCRequest_Params
	if err := reqParams.FromPingRequest(PingRequest{ClientSendTime: 123}); err != nil {
		t.Fatalf("FromPingRequest() error = %v", err)
	}
	var reqBuf bytes.Buffer
	req := &RPCRequest{
		V:      RPCVersionV1,
		Id:     "req-1",
		Method: RPCMethodPeerPing,
		Params: &reqParams,
	}
	if err := WriteRequest(&reqBuf, req); err != nil {
		t.Fatalf("WriteRequest() error = %v", err)
	}
	gotReq, err := ReadRequest(&reqBuf)
	if err != nil {
		t.Fatalf("ReadRequest() error = %v", err)
	}
	if gotReq.Id != req.Id || gotReq.Method != RPCMethodPeerPing || gotReq.Params == nil {
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

func TestGearRPCUnionTypes(t *testing.T) {
	var pingParams RPCRequest_Params
	if err := pingParams.MergePingRequest(PingRequest{ClientSendTime: 100}); err != nil {
		t.Fatalf("MergePingRequest() error = %v", err)
	}
	if got, err := pingParams.AsPingRequest(); err != nil || got.ClientSendTime != 100 {
		t.Fatalf("AsPingRequest() = %+v, %v", got, err)
	}

	assertRequestUnion(t, "GearGetConfig", GearGetConfigRequest{}, (*RPCRequest_Params).FromGearGetConfigRequest, RPCRequest_Params.AsGearGetConfigRequest, (*RPCRequest_Params).MergeGearGetConfigRequest)
	assertRequestUnion(t, "GearGetInfo", GearGetInfoRequest{}, (*RPCRequest_Params).FromGearGetInfoRequest, RPCRequest_Params.AsGearGetInfoRequest, (*RPCRequest_Params).MergeGearGetInfoRequest)
	assertRequestUnion(t, "GearPutInfo", GearPutInfoRequest{Name: stringPtr("gear-1")}, (*RPCRequest_Params).FromGearPutInfoRequest, RPCRequest_Params.AsGearPutInfoRequest, (*RPCRequest_Params).MergeGearPutInfoRequest)
	assertRequestUnion(t, "GearGetRegistration", GearGetRegistrationRequest{}, (*RPCRequest_Params).FromGearGetRegistrationRequest, RPCRequest_Params.AsGearGetRegistrationRequest, (*RPCRequest_Params).MergeGearGetRegistrationRequest)
	assertRequestUnion(t, "GearRegister", GearRegisterRequest{Device: DeviceInfo{Name: stringPtr("gear-1")}}, (*RPCRequest_Params).FromGearRegisterRequest, RPCRequest_Params.AsGearRegisterRequest, (*RPCRequest_Params).MergeGearRegisterRequest)
	assertRequestUnion(t, "GearGetRuntime", GearGetRuntimeRequest{}, (*RPCRequest_Params).FromGearGetRuntimeRequest, RPCRequest_Params.AsGearGetRuntimeRequest, (*RPCRequest_Params).MergeGearGetRuntimeRequest)
	assertRequestUnion(t, "GearRunTest", GearRunTestRequest{Mode: GearTestModeAudioPlay}, (*RPCRequest_Params).FromGearRunTestRequest, RPCRequest_Params.AsGearRunTestRequest, (*RPCRequest_Params).MergeGearRunTestRequest)
	assertRequestUnion(t, "PeerGetInfo", PeerGetInfoRequest{}, (*RPCRequest_Params).FromPeerGetInfoRequest, RPCRequest_Params.AsPeerGetInfoRequest, (*RPCRequest_Params).MergePeerGetInfoRequest)
	assertRequestUnion(t, "PeerGetIdentifiers", PeerGetIdentifiersRequest{}, (*RPCRequest_Params).FromPeerGetIdentifiersRequest, RPCRequest_Params.AsPeerGetIdentifiersRequest, (*RPCRequest_Params).MergePeerGetIdentifiersRequest)
	assertRequestUnion(t, "ServerGetInfo", ServerGetInfoRequest{}, (*RPCRequest_Params).FromServerGetInfoRequest, RPCRequest_Params.AsServerGetInfoRequest, (*RPCRequest_Params).MergeServerGetInfoRequest)

	var pingResult RPCResponse_Result
	if err := pingResult.MergePingResponse(PingResponse{ServerTime: 200}); err != nil {
		t.Fatalf("MergePingResponse() error = %v", err)
	}
	if got, err := pingResult.AsPingResponse(); err != nil || got.ServerTime != 200 {
		t.Fatalf("AsPingResponse() = %+v, %v", got, err)
	}

	now := time.Unix(100, 0).UTC()
	publicKey := string([]byte{1})
	registration := Registration{
		PublicKey: publicKey,
		Role:      GearRoleGear,
		Status:    GearStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	gear := Gear{
		PublicKey:     publicKey,
		Role:          GearRoleGear,
		Status:        GearStatusActive,
		Device:        DeviceInfo{Name: stringPtr("gear-1")},
		Configuration: Configuration{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	assertResponseUnion(t, "GearGetConfig", GearGetConfigResponse{}, (*RPCResponse_Result).FromGearGetConfigResponse, RPCResponse_Result.AsGearGetConfigResponse, (*RPCResponse_Result).MergeGearGetConfigResponse)
	assertResponseUnion(t, "GearGetInfo", GearGetInfoResponse{Name: stringPtr("gear-1")}, (*RPCResponse_Result).FromGearGetInfoResponse, RPCResponse_Result.AsGearGetInfoResponse, (*RPCResponse_Result).MergeGearGetInfoResponse)
	assertResponseUnion(t, "GearPutInfo", GearPutInfoResponse{Name: stringPtr("gear-2")}, (*RPCResponse_Result).FromGearPutInfoResponse, RPCResponse_Result.AsGearPutInfoResponse, (*RPCResponse_Result).MergeGearPutInfoResponse)
	assertResponseUnion(t, "GearGetRegistration", GearGetRegistrationResponse(registration), (*RPCResponse_Result).FromGearGetRegistrationResponse, RPCResponse_Result.AsGearGetRegistrationResponse, (*RPCResponse_Result).MergeGearGetRegistrationResponse)
	assertResponseUnion(t, "GearRegister", GearRegisterResponse{Gear: gear, Registration: registration}, (*RPCResponse_Result).FromGearRegisterResponse, RPCResponse_Result.AsGearRegisterResponse, (*RPCResponse_Result).MergeGearRegisterResponse)
	assertResponseUnion(t, "GearGetRuntime", GearGetRuntimeResponse{Online: true, LastSeenAt: now}, (*RPCResponse_Result).FromGearGetRuntimeResponse, RPCResponse_Result.AsGearGetRuntimeResponse, (*RPCResponse_Result).MergeGearGetRuntimeResponse)
	assertResponseUnion(t, "GearRunTest", GearRunTestResponse{Mode: GearTestModeAudioPlay, Status: GearRunTestResponseStatusStarted}, (*RPCResponse_Result).FromGearRunTestResponse, RPCResponse_Result.AsGearRunTestResponse, (*RPCResponse_Result).MergeGearRunTestResponse)
	assertResponseUnion(t, "PeerGetInfo", PeerGetInfoResponse{Name: stringPtr("gear-1")}, (*RPCResponse_Result).FromPeerGetInfoResponse, RPCResponse_Result.AsPeerGetInfoResponse, (*RPCResponse_Result).MergePeerGetInfoResponse)
	assertResponseUnion(t, "PeerGetIdentifiers", PeerGetIdentifiersResponse{Sn: stringPtr("sn-1")}, (*RPCResponse_Result).FromPeerGetIdentifiersResponse, RPCResponse_Result.AsPeerGetIdentifiersResponse, (*RPCResponse_Result).MergePeerGetIdentifiersResponse)
	assertResponseUnion(t, "ServerGetInfo", ServerGetInfoResponse{PublicKey: "server", ServerTime: 123}, (*RPCResponse_Result).FromServerGetInfoResponse, RPCResponse_Result.AsServerGetInfoResponse, (*RPCResponse_Result).MergeServerGetInfoResponse)
}

func TestRPCMethodValid(t *testing.T) {
	for _, method := range []RPCMethod{
		RPCMethodPeerPing,
		RPCMethodPeerInfoGet,
		RPCMethodPeerIdentifiersGet,
		RPCMethodGearConfigGet,
		RPCMethodGearInfoGet,
		RPCMethodGearInfoPut,
		RPCMethodGearRegistrationGet,
		RPCMethodGearRegistrationRegister,
		RPCMethodGearRuntimeGet,
		RPCMethodGearTestRun,
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
	for _, code := range []RPCErrorCode{RPCErrorCodeInvalidRequest, RPCErrorCodeMethodNotFound, RPCErrorCodeInvalidParams, RPCErrorCodeInternalError, RPCErrorCodeBadRequest, RPCErrorCodeNotFound, RPCErrorCodeConflict} {
		if !code.Valid() {
			t.Fatalf("%d should be valid", code)
		}
	}
	if RPCErrorCode(418).Valid() {
		t.Fatal("unknown RPC error code should be invalid")
	}
	for _, role := range []GearRole{GearRoleAdmin, GearRoleGear, GearRoleServer, GearRoleUnspecified} {
		if !role.Valid() {
			t.Fatalf("%s should be valid", role)
		}
	}
	if GearRole("bad").Valid() {
		t.Fatal("unknown gear role should be invalid")
	}
	for _, status := range []GearStatus{GearStatusActive, GearStatusBlocked, GearStatusUnspecified} {
		if !status.Valid() {
			t.Fatalf("%s should be valid", status)
		}
	}
	if GearStatus("bad").Valid() {
		t.Fatal("unknown gear status should be invalid")
	}
	for _, mode := range []GearTestMode{GearTestModeAudioLoopback, GearTestModeAudioPlay, GearTestModeDownload, GearTestModeUpload} {
		if !mode.Valid() {
			t.Fatalf("%s should be valid", mode)
		}
	}
	if GearTestMode("bad").Valid() {
		t.Fatal("unknown gear test mode should be invalid")
	}
	if !GearRunTestResponseStatusStarted.Valid() {
		t.Fatalf("%s should be valid", GearRunTestResponseStatusStarted)
	}
	if GearRunTestResponseStatus("bad").Valid() {
		t.Fatal("unknown gear run test response status should be invalid")
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
	if err := WriteFrame(errorWriter{err: writeErr}, []byte("payload")); !errors.Is(err, writeErr) {
		t.Fatalf("WriteFrame() err = %v, want %v", err, writeErr)
	}
}

func TestReadRequestAndResponseRejectInvalidJSON(t *testing.T) {
	var reqBuf bytes.Buffer
	if err := WriteFrame(&reqBuf, []byte("{")); err != nil {
		t.Fatalf("WriteFrame(request) error = %v", err)
	}
	if _, err := ReadRequest(&reqBuf); err == nil {
		t.Fatal("ReadRequest() should fail for invalid JSON")
	}

	var respBuf bytes.Buffer
	if err := WriteFrame(&respBuf, []byte("{")); err != nil {
		t.Fatalf("WriteFrame(response) error = %v", err)
	}
	if _, err := ReadResponse(&respBuf); err == nil {
		t.Fatal("ReadResponse() should fail for invalid JSON")
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}

func TestReadFrameRejectsOversizedFrame(t *testing.T) {
	var buf bytes.Buffer
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], MaxFrameSize+1)
	if _, err := buf.Write(hdr[:]); err != nil {
		t.Fatalf("Write(header) error = %v", err)
	}

	_, err := ReadFrame(&buf)
	if err == nil || err.Error() != "rpc: frame too large: 1048577" {
		t.Fatalf("ReadFrame() err = %v", err)
	}
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
