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
	"google.golang.org/protobuf/types/known/structpb"
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

	var reqParams RPCPayload
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

	var respResult RPCPayload
	if err := respResult.FromPingResponse(PingResponse{ServerTime: 456}); err != nil {
		t.Fatalf("FromPingResponse() error = %v", err)
	}
	var respBuf bytes.Buffer
	resp := &RPCResponse{
		V:      RPCVersionV1,
		Id:     "req-1",
		Result: &respResult,
	}
	if err := WriteResponseForMethod(&respBuf, RPCMethodAllPing, resp); err != nil {
		t.Fatalf("WriteResponseForMethod() error = %v", err)
	}
	gotResp, err := ReadResponseForMethod(&respBuf, RPCMethodAllPing)
	if err != nil {
		t.Fatalf("ReadResponseForMethod() error = %v", err)
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

func TestModelProviderDataOneofRoundTripAndRejectsMultipleValues(t *testing.T) {
	deepSeek := &DeepSeekTenantModelProviderData{
		ApiMode:       DeepSeekTenantModelProviderDataApiModeChatCompletions,
		UpstreamModel: "deepseek-chat",
	}
	response := ModelGetResponse{Value: Model{
		Alias:          "chat",
		Kind:           ModelKindLlm,
		ProviderKind:   ModelProviderKindDeepseekTenant,
		DeepSeekTenant: deepSeek,
	}}
	var payload RPCPayload
	if err := payload.FromModelGetResponse(response); err != nil {
		t.Fatalf("FromModelGetResponse() error = %v", err)
	}
	decoded, err := payload.AsModelGetResponse()
	if err != nil {
		t.Fatalf("AsModelGetResponse() error = %v", err)
	}
	if decoded.Value.ProviderKind != ModelProviderKindDeepseekTenant || decoded.Value.DeepSeekTenant == nil || decoded.Value.DeepSeekTenant.UpstreamModel != "deepseek-chat" || decoded.Value.OpenAITenant != nil {
		t.Fatalf("AsModelGetResponse() = %#v", decoded)
	}
	dashScopeMode := DashScopeTenantModelProviderDataApiModeChatCompletions
	dashScopeResponse := ModelGetResponse{Value: Model{
		Alias:        "qwen",
		Kind:         ModelKindLlm,
		ProviderKind: ModelProviderKindDashscopeTenant,
		DashScopeTenant: &DashScopeTenantModelProviderData{
			ApiMode:       &dashScopeMode,
			UpstreamModel: stringPtr("qwen-plus"),
		},
	}}
	if err := payload.FromModelGetResponse(dashScopeResponse); err != nil {
		t.Fatalf("FromModelGetResponse(DashScope) error = %v", err)
	}
	dashScopeDecoded, err := payload.AsModelGetResponse()
	if err != nil {
		t.Fatalf("AsModelGetResponse(DashScope) error = %v", err)
	}
	if dashScopeDecoded.Value.DashScopeTenant == nil || dashScopeDecoded.Value.DashScopeTenant.ApiMode == nil || *dashScopeDecoded.Value.DashScopeTenant.ApiMode != dashScopeMode {
		t.Fatalf("AsModelGetResponse(DashScope) lost api_mode: %#v", dashScopeDecoded)
	}

	response.Value.OpenAITenant = &OpenAITenantModelProviderData{UpstreamModel: stringPtr("gpt-test")}
	if err := payload.FromModelGetResponse(response); err == nil {
		t.Fatal("FromModelGetResponse() accepted multiple provider_data oneof values")
	}
}

func TestEncodeRPCResponseRejectsResultWithoutMethod(t *testing.T) {
	var result RPCPayload
	if err := result.FromPingResponse(PingResponse{ServerTime: 456}); err != nil {
		t.Fatalf("FromPingResponse() error = %v", err)
	}
	_, err := EncodeRPCResponse(&RPCResponse{
		V:      RPCVersionV1,
		Id:     "req-1",
		Result: &result,
	})
	if err == nil || err.Error() != "rpc: response result requires method-specific encoding" {
		t.Fatalf("EncodeRPCResponse() err = %v", err)
	}
}

func TestReadResponseRejectsGenericSuccessPayload(t *testing.T) {
	var result RPCPayload
	if err := result.FromPingResponse(PingResponse{ServerTime: 456}); err != nil {
		t.Fatalf("FromPingResponse() error = %v", err)
	}
	var buf bytes.Buffer
	if err := WriteResponseForMethod(&buf, RPCMethodAllPing, &RPCResponse{
		V:      RPCVersionV1,
		Id:     "req-1",
		Result: &result,
	}); err != nil {
		t.Fatalf("WriteResponseForMethod() error = %v", err)
	}
	_, err := ReadResponse(&buf)
	if err == nil || err.Error() != "rpc: unmarshal response: rpc: response payload requires method-specific decoding" {
		t.Fatalf("ReadResponse() err = %v", err)
	}
}

func TestRPCUnionTypes(t *testing.T) {
	var pingParams RPCPayload
	if err := pingParams.MergePingRequest(PingRequest{ClientSendTime: 100}); err != nil {
		t.Fatalf("MergePingRequest() error = %v", err)
	}
	if got, err := pingParams.AsPingRequest(); err != nil || got.ClientSendTime != 100 {
		t.Fatalf("AsPingRequest() = %+v, %v", got, err)
	}

	assertRequestUnion(t, "ServerPutInfo", ServerPutInfoRequest{Name: stringPtr("peer-1")}, (*RPCPayload).FromServerPutInfoRequest, RPCPayload.AsServerPutInfoRequest, (*RPCPayload).MergeServerPutInfoRequest)
	assertRequestUnion(t, "ServerGetRuntime", ServerGetRuntimeRequest{}, (*RPCPayload).FromServerGetRuntimeRequest, RPCPayload.AsServerGetRuntimeRequest, (*RPCPayload).MergeServerGetRuntimeRequest)
	assertRequestUnion(t, "ClientGetInfo", ClientGetInfoRequest{}, (*RPCPayload).FromClientGetInfoRequest, RPCPayload.AsClientGetInfoRequest, (*RPCPayload).MergeClientGetInfoRequest)
	assertRequestUnion(t, "ClientGetIdentifiers", ClientGetIdentifiersRequest{}, (*RPCPayload).FromClientGetIdentifiersRequest, RPCPayload.AsClientGetIdentifiersRequest, (*RPCPayload).MergeClientGetIdentifiersRequest)
	assertRequestUnion(t, "ServerGetInfo", ServerGetInfoRequest{}, (*RPCPayload).FromServerGetInfoRequest, RPCPayload.AsServerGetInfoRequest, (*RPCPayload).MergeServerGetInfoRequest)

	var pingResult RPCPayload
	if err := pingResult.MergePingResponse(PingResponse{ServerTime: 200}); err != nil {
		t.Fatalf("MergePingResponse() error = %v", err)
	}
	if got, err := pingResult.AsPingResponse(); err != nil || got.ServerTime != 200 {
		t.Fatalf("AsPingResponse() = %+v, %v", got, err)
	}

	now := time.Unix(100, 0).UTC()
	assertResponseUnion(t, "ServerGetInfo", ServerGetInfoResponse{Name: stringPtr("peer-1")}, (*RPCPayload).FromServerGetInfoResponse, RPCPayload.AsServerGetInfoResponse, (*RPCPayload).MergeServerGetInfoResponse)
	assertResponseUnion(t, "ServerPutInfo", ServerPutInfoResponse{Name: stringPtr("peer-2")}, (*RPCPayload).FromServerPutInfoResponse, RPCPayload.AsServerPutInfoResponse, (*RPCPayload).MergeServerPutInfoResponse)
	assertResponseUnion(t, "ServerGetRuntime", ServerGetRuntimeResponse{Online: true, LastSeenAt: now}, (*RPCPayload).FromServerGetRuntimeResponse, RPCPayload.AsServerGetRuntimeResponse, (*RPCPayload).MergeServerGetRuntimeResponse)
	assertResponseUnion(t, "ClientGetInfo", ClientGetInfoResponse{Model: stringPtr("model-1")}, (*RPCPayload).FromClientGetInfoResponse, RPCPayload.AsClientGetInfoResponse, (*RPCPayload).MergeClientGetInfoResponse)
	assertResponseUnion(t, "ClientGetIdentifiers", ClientGetIdentifiersResponse{Sn: stringPtr("sn-1")}, (*RPCPayload).FromClientGetIdentifiersResponse, RPCPayload.AsClientGetIdentifiersResponse, (*RPCPayload).MergeClientGetIdentifiersResponse)
}

func TestMethodPayloadsUseProtobufBytes(t *testing.T) {
	var params RPCPayload
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

	var result RPCPayload
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

func TestServerPeerRoutePayloadsUseRPCProtoMessages(t *testing.T) {
	var params RPCPayload
	if err := params.FromServerPeerAssignRequest(rpcpb.ServerPeerAssignRequest{
		PeerPublicKey:   "peer-a",
		ExpectedVersion: int64Ptr(7),
	}); err != nil {
		t.Fatalf("FromServerPeerAssignRequest() error = %v", err)
	}
	reqMsg, err := EncodeRPCRequest(&RPCRequest{
		V:      RPCVersionV1,
		Id:     "req-server-peer",
		Method: RPCMethodServerPeerAssign,
		Params: &params,
	})
	if err != nil {
		t.Fatalf("EncodeRPCRequest() error = %v", err)
	}
	var protoReq rpcpb.ServerPeerAssignRequest
	if err := proto.Unmarshal(reqMsg.GetPayload(), &protoReq); err != nil {
		t.Fatalf("protobuf request payload unmarshal error = %v", err)
	}
	if protoReq.GetPeerPublicKey() != "peer-a" || protoReq.GetExpectedVersion() != 7 {
		t.Fatalf("protobuf request payload = %+v", protoReq)
	}

	var result RPCPayload
	if err := result.FromServerPeerAssignResponse(rpcpb.ServerPeerAssignResponse{
		Assignment: &rpcpb.PeerAssignment{
			PeerPublicKey:   "peer-a",
			ServerPublicKey: "server-a",
			ServerEndpoint:  "server:9820",
			Role:            rpcpb.PeerRole_PEER_ROLE_CLIENT,
			Version:         8,
			UpdatedAt:       "2026-07-12T00:00:00Z",
		},
	}); err != nil {
		t.Fatalf("FromServerPeerAssignResponse() error = %v", err)
	}
	respMsg, err := EncodeRPCResponseForMethod(RPCMethodServerPeerAssign, &RPCResponse{
		V:      RPCVersionV1,
		Id:     "req-server-peer",
		Result: &result,
	})
	if err != nil {
		t.Fatalf("EncodeRPCResponseForMethod() error = %v", err)
	}
	var protoResp rpcpb.ServerPeerAssignResponse
	if err := proto.Unmarshal(respMsg.GetPayload(), &protoResp); err != nil {
		t.Fatalf("protobuf response payload unmarshal error = %v", err)
	}
	if protoResp.GetAssignment().GetPeerPublicKey() != "peer-a" || protoResp.GetAssignment().GetVersion() != 8 {
		t.Fatalf("protobuf response payload = %+v", protoResp)
	}

	decoded, err := DecodeRPCResponseForMethod(RPCMethodServerPeerAssign, respMsg)
	if err != nil {
		t.Fatalf("DecodeRPCResponseForMethod() error = %v", err)
	}
	got, err := decoded.Result.AsServerPeerAssignResponse()
	if err != nil {
		t.Fatalf("AsServerPeerAssignResponse() error = %v", err)
	}
	if got.GetAssignment().GetServerPublicKey() != "server-a" {
		t.Fatalf("decoded response = %+v", got)
	}
}

func TestDecodeRPCRequestPreservesMissingPayload(t *testing.T) {
	msg := &rpcpb.RpcRequest{
		Id:     "req-1",
		Method: rpcpb.RpcMethod_RPC_METHOD_SERVER_INFO_PUT,
	}
	req, err := DecodeRPCRequest(msg)
	if err != nil {
		t.Fatalf("DecodeRPCRequest() error = %v", err)
	}
	if req.Params != nil {
		t.Fatalf("DecodeRPCRequest().Params = %+v, want nil", req.Params)
	}

	msg.Payload = []byte{}
	req, err = DecodeRPCRequest(msg)
	if err != nil {
		t.Fatalf("DecodeRPCRequest(empty payload) error = %v", err)
	}
	if req.Params == nil {
		t.Fatal("DecodeRPCRequest(empty payload).Params = nil, want present empty payload")
	}
}

func TestEncodeRPCRequestPreservesEmptyPayloadPresence(t *testing.T) {
	var params RPCPayload
	if err := params.FromPingRequest(PingRequest{}); err != nil {
		t.Fatalf("FromPingRequest() error = %v", err)
	}
	msg, err := EncodeRPCRequest(&RPCRequest{
		V:      RPCVersionV1,
		Id:     "req-empty",
		Method: RPCMethodAllPing,
		Params: &params,
	})
	if err != nil {
		t.Fatalf("EncodeRPCRequest() error = %v", err)
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	var roundTrip rpcpb.RpcRequest
	if err := proto.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("proto.Unmarshal() error = %v", err)
	}
	req, err := DecodeRPCRequest(&roundTrip)
	if err != nil {
		t.Fatalf("DecodeRPCRequest() error = %v", err)
	}
	if req.Params == nil {
		t.Fatal("DecodeRPCRequest(round trip empty payload).Params = nil, want present")
	}
	got, err := req.Params.AsPingRequest()
	if err != nil {
		t.Fatalf("AsPingRequest() error = %v", err)
	}
	if got.ClientSendTime != 0 {
		t.Fatalf("AsPingRequest().ClientSendTime = %d, want 0", got.ClientSendTime)
	}
}

func TestDecodeRPCResponseRejectsGenericPayload(t *testing.T) {
	msg := &rpcpb.RpcResponse{
		Id:   "resp-empty",
		Body: &rpcpb.RpcResponse_Payload{Payload: []byte{}},
	}
	data, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("proto.Marshal() error = %v", err)
	}
	var roundTrip rpcpb.RpcResponse
	if err := proto.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("proto.Unmarshal() error = %v", err)
	}
	_, err = DecodeRPCResponse(&roundTrip)
	if err == nil || err.Error() != "rpc: response payload requires method-specific decoding" {
		t.Fatalf("DecodeRPCResponse() err = %v", err)
	}

	resp, err := DecodeRPCResponseForMethod(RPCMethodAllPing, &roundTrip)
	if err != nil {
		t.Fatalf("DecodeRPCResponseForMethod() error = %v", err)
	}
	if resp.Result == nil {
		t.Fatal("DecodeRPCResponseForMethod(empty payload oneof).Result = nil, want present")
	}
	got, err := resp.Result.AsPingResponse()
	if err != nil {
		t.Fatalf("AsPingResponse() error = %v", err)
	}
	if got.ServerTime != 0 {
		t.Fatalf("AsPingResponse().ServerTime = %d, want 0", got.ServerTime)
	}
}

func TestPayloadCodecMapsGoDTOsDirectlyToProtobuf(t *testing.T) {
	var recall RPCPayload
	if err := recall.FromServerRunWorkspaceRecallRequest(ServerRunWorkspaceRecallRequest{
		Filters: &map[string]interface{}{
			"score":  float64(1),
			"nested": map[string]interface{}{"weight": float64(2)},
		},
	}); err != nil {
		t.Fatalf("FromServerRunWorkspaceRecallRequest() error = %v", err)
	}
	var recallProto rpcpb.ServerRunWorkspaceRecallRequest
	if err := proto.Unmarshal(recall.payload, &recallProto); err != nil {
		t.Fatalf("unmarshal recall payload error = %v", err)
	}
	if recallProto.GetValue().GetFilters().GetFields()["score"].GetNumberValue() != 1 {
		t.Fatalf("recall filters = %+v", recallProto.GetValue().GetFilters())
	}

	var chatParams RPCPayload
	if err := chatParams.encode("ChatRoomWorkspaceParameters", ChatRoomWorkspaceParameters{Input: ptr(WorkspaceInputModePushToTalk)}); err != nil {
		t.Fatalf("encode ChatRoomWorkspaceParameters error = %v", err)
	}
	var chatProto rpcpb.ChatRoomWorkspaceParameters
	if err := proto.Unmarshal(chatParams.payload, &chatProto); err != nil {
		t.Fatalf("unmarshal chatroom payload error = %v", err)
	}
	if chatProto.GetInput() != rpcpb.WorkspaceInputMode_WORKSPACE_INPUT_MODE_PUSH_TO_TALK {
		t.Fatalf("chatroom input = %s", chatProto.GetInput())
	}

	var workspaceCreate RPCPayload
	workspaceToolIDs := []string{"system.toolkit.echo"}
	if err := workspaceCreate.FromWorkspaceCreateRequest(WorkspaceCreateRequest{
		Name: "demo", Collection: "assistants", WorkflowAlias: "chat",
		Toolkit: &ToolkitPolicy{ToolIds: &workspaceToolIDs},
	}); err != nil {
		t.Fatalf("FromWorkspaceCreateRequest() error = %v", err)
	}
	workspaceCreateDecoded, err := workspaceCreate.AsWorkspaceCreateRequest()
	if err != nil {
		t.Fatalf("AsWorkspaceCreateRequest() error = %v", err)
	}
	if workspaceCreateDecoded.Collection != "assistants" || workspaceCreateDecoded.WorkflowAlias != "chat" {
		t.Fatalf("workspace create = %#v", workspaceCreateDecoded)
	}

	var workflowGet RPCPayload
	if err := workflowGet.FromWorkflowGetRequest(WorkflowGetRequest{Alias: "flowcraft-toolkit"}); err != nil {
		t.Fatalf("FromWorkflowGetRequest() error = %v", err)
	}
	workflowGetDecoded, err := workflowGet.AsWorkflowGetRequest()
	if err != nil || workflowGetDecoded.Alias != "flowcraft-toolkit" {
		t.Fatalf("workflow get = %#v, %v", workflowGetDecoded, err)
	}

	var workflowResponse RPCPayload
	workflow := WorkflowGetResponse{
		Value:              Workflow{Alias: "flowcraft-toolkit", Collection: "assistants", Driver: WorkflowDriverFlowcraft, I18n: map[string]AliasI18nText{"en": {DisplayName: "Flowcraft"}}},
		RuntimeProfileName: "default", RuntimeProfileRevision: "revision",
	}
	if err := workflowResponse.FromWorkflowGetResponse(workflow); err != nil {
		t.Fatalf("FromWorkflowGetResponse() error = %v", err)
	}
	workflowDecoded, err := workflowResponse.AsWorkflowGetResponse()
	if err != nil || workflowDecoded.Value.Alias != workflow.Value.Alias {
		t.Fatalf("workflow response = %#v, %v", workflowDecoded, err)
	}

	var statPayload RPCPayload
	if err := statPayload.encode("PetStats", PetStats{Life: 99, Health: 88, Energy: 77}); err != nil {
		t.Fatalf("encode PetStats error = %v", err)
	}
	var statProto rpcpb.PetStats
	if err := proto.Unmarshal(statPayload.payload, &statProto); err != nil {
		t.Fatalf("unmarshal pet stats payload error = %v", err)
	}
	if statProto.GetLife() != 99 || statProto.GetHealth() != 88 || statProto.GetEnergy() != 77 {
		t.Fatalf("stats = %+v", statProto)
	}
}

func TestPayloadCodecMapsProtobufDirectlyToGoDTOs(t *testing.T) {
	firmwareData, err := proto.Marshal(&rpcpb.FirmwareGetRequest{})
	if err != nil {
		t.Fatalf("marshal firmware get payload error = %v", err)
	}
	firmwarePayload := newRPCPayload("FirmwareGetRequest", firmwareData, true)
	_, err = firmwarePayload.AsFirmwareGetRequest()
	if err != nil {
		t.Fatalf("AsFirmwareGetRequest() error = %v", err)
	}

	schemaData, err := proto.Marshal(&rpcpb.DoubaoRealtimeJSONSchema{
		AdditionalProperties: ptr(false),
		AnyOf: []*rpcpb.DoubaoRealtimeJSONSchema{
			{Type: ptr("string")},
		},
		EnumValues: []string{"red", "green"},
		MinLength:  ptr(int64(2)),
	})
	if err != nil {
		t.Fatalf("marshal JSON schema payload error = %v", err)
	}
	schemaPayload := newRPCPayload("DoubaoRealtimeJSONSchema", schemaData, false)
	var schema DoubaoRealtimeJSONSchema
	if err := schemaPayload.decode("DoubaoRealtimeJSONSchema", &schema); err != nil {
		t.Fatalf("decode DoubaoRealtimeJSONSchema error = %v", err)
	}
	if schema.AdditionalProperties == nil || *schema.AdditionalProperties ||
		schema.AnyOf == nil || len(*schema.AnyOf) != 1 ||
		schema.Enum == nil || len(*schema.Enum) != 2 ||
		schema.MinLength == nil || *schema.MinLength != 2 {
		t.Fatalf("schema = %+v", schema)
	}

	recallData, err := proto.Marshal(&rpcpb.ServerRunWorkspaceRecallRequest{
		Value: &rpcpb.PeerRunRecallRequest{
			Filters: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"deleted_at": structpb.NewNullValue(),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal recall payload error = %v", err)
	}
	recallPayload := newRPCPayload("ServerRunWorkspaceRecallRequest", recallData, false)
	recall, err := recallPayload.AsServerRunWorkspaceRecallRequest()
	if err != nil {
		t.Fatalf("AsServerRunWorkspaceRecallRequest() error = %v", err)
	}
	if recall.Filters == nil {
		t.Fatal("recall filters = nil")
	}
	item, ok := (*recall.Filters)["deleted_at"]
	if !ok || item != nil {
		t.Fatalf("recall deleted_at = %#v, present=%v; want explicit nil", item, ok)
	}
}

func TestRPCMethodValid(t *testing.T) {
	for _, method := range []RPCMethod{
		RPCMethodAllPing,
		RPCMethodClientInfoGet,
		RPCMethodClientIdentifiersGet,
		RPCMethodServerInfoGet,
		RPCMethodServerInfoPut,
		RPCMethodServerRegister,
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
	for _, code := range []RPCErrorCode{RPCErrorCodeParseError, RPCErrorCodeInvalidRequest, RPCErrorCodeMethodNotFound, RPCErrorCodeInvalidParams, RPCErrorCodeInternalError, RPCErrorCodeBadRequest, RPCErrorCodeForbidden, RPCErrorCodeNotFound, RPCErrorCodeConflict} {
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
	from func(*RPCPayload, T) error,
	as func(RPCPayload) (T, error),
	merge func(*RPCPayload, T) error,
) {
	t.Helper()
	var params RPCPayload
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
	from func(*RPCPayload, T) error,
	as func(RPCPayload) (T, error),
	merge func(*RPCPayload, T) error,
) {
	t.Helper()
	var result RPCPayload
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

func int64Ptr(value int64) *int64 {
	return &value
}

func ptr[T any](value T) *T {
	return &value
}

func TestWriteFramePropagatesHeaderWriteError(t *testing.T) {
	writeErr := errors.New("write failed")
	if err := WriteFrame(errorWriter{err: writeErr}, Frame{Type: FrameTypeBinary, Payload: []byte("payload")}); !errors.Is(err, writeErr) {
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
	if err := WriteFrame(&reqBuf, Frame{Type: FrameTypeText, Payload: []byte("{}")}); err != nil {
		t.Fatalf("WriteFrame(request) error = %v", err)
	}
	if _, err := ReadRequest(&reqBuf); err == nil || err.Error() != "rpc: unmarshal request: rpc: expected protobuf binary frame, got type 3" {
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
	if len(got) != 2 {
		t.Fatalf("ReadFrames() got %d frames", len(got))
	}
	if got[0].Type != FrameTypeText || string(got[0].Payload) != "hello" {
		t.Fatalf("frame[0] = %+v", got[0])
	}
	if got[1].Type != FrameTypeBinary || !bytes.Equal(got[1].Payload, []byte{1, 2, 3}) {
		t.Fatalf("frame[1] = %+v", got[1])
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

func TestReadWriteResponsesForMethod(t *testing.T) {
	serverTimes := []int64{11, 22}
	ids := []string{"one", "two"}
	var buf bytes.Buffer
	err := WriteResponsesForMethod(&buf, RPCMethodAllPing, func(yield func(*RPCResponse, error) bool) {
		for index, serverTime := range serverTimes {
			var result RPCPayload
			if err := result.FromPingResponse(PingResponse{ServerTime: serverTime}); err != nil {
				t.Fatalf("FromPingResponse() error = %v", err)
			}
			if !yield(&RPCResponse{V: RPCVersionV1, Id: ids[index], Result: &result}, nil) {
				return
			}
		}
	})
	if err != nil {
		t.Fatalf("WriteResponsesForMethod() error = %v", err)
	}

	var got []int64
	for resp, err := range ReadResponsesForMethod(&buf, RPCMethodAllPing) {
		if err != nil {
			t.Fatalf("ReadResponsesForMethod() error = %v", err)
		}
		if resp.Result == nil {
			t.Fatalf("ReadResponsesForMethod() response missing result: %+v", resp)
		}
		result, err := resp.Result.AsPingResponse()
		if err != nil {
			t.Fatalf("AsPingResponse() error = %v", err)
		}
		got = append(got, result.ServerTime)
	}
	if len(got) != len(serverTimes) || got[0] != 11 || got[1] != 22 {
		t.Fatalf("ReadResponsesForMethod() server times = %v", got)
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
	err := WriteFrame(shortWriter{}, Frame{Type: FrameTypeBinary, Payload: []byte("payload")})
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
