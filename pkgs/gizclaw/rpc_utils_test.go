package gizclaw

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestRPCClientPingSingleRequestResponse(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	reqCh := make(chan *rpcapi.RPCRequest, 1)
	serverErrCh := make(chan error, 1)

	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		reqCh <- req

		resp, err := newRPCPingResponse(req.Id, rpcapi.PingResponse{ServerTime: rpcServerTimeForID(req.Id)})
		if err != nil {
			serverErrCh <- err
			return
		}
		if err := writeRPCResponseWithEOS(serverSide, req.Method, resp); err != nil {
			serverErrCh <- err
			return
		}

		serverErrCh <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ping, err := callRPCPing(ctx, clientSide, "req-1")
	if err != nil {
		t.Fatalf("Ping(req-1) error: %v", err)
	}
	if ping.ServerTime != rpcServerTimeForID("req-1") {
		t.Fatalf("Ping(req-1) server_time = %d", ping.ServerTime)
	}

	if req := <-reqCh; req.Id == "" {
		t.Fatal("request missing id")
	} else {
		assertRPCPingRequestHasTimestamp(t, req)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server goroutine error: %v", err)
	}
}

func TestRPCServerHandleReusesStreamByDefault(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- (&rpcServer{}).Handle(serverSide)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	first, err := callRPCPing(ctx, clientSide, "req-1")
	if err != nil {
		t.Fatalf("Ping(req-1) error = %v", err)
	}
	second, err := callRPCPing(ctx, clientSide, "req-2")
	if err != nil {
		t.Fatalf("Ping(req-2) error = %v", err)
	}
	if first.ServerTime <= 0 || second.ServerTime <= 0 {
		t.Fatalf("server times = %d, %d; want positive", first.ServerTime, second.ServerTime)
	}

	if err := clientSide.Close(); err != nil {
		t.Fatalf("client close error = %v", err)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server Handle error = %v", err)
	}
}

func TestRPCServerLogsDomainFailureOnce(t *testing.T) {
	capture := captureSlog(t)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	t.Cleanup(func() { _ = clientSide.Close() })

	serverErrCh := make(chan error, 1)
	server := &rpcServer{
		callerPublicKey: giznet.PublicKey{1},
		serverResources: &peerresource.Server{
			Caller: giznet.PublicKey{1},
			RuntimeProfile: func() *apitypes.RuntimeProfile {
				workflows := map[string]string{"chat": "workflow-a"}
				return &apitypes.RuntimeProfile{Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Workflows: &workflows}}}
			},
			Workspaces: invalidWorkspaceAdminService{},
			Workflows: fixedWorkflowAdminService{value: apitypes.Workflow{
				Name: "workflow-a",
				Spec: apitypes.WorkflowSpec{Driver: apitypes.WorkflowDriverChatroom},
			}},
		},
	}
	go func() { serverErrCh <- server.Handle(serverSide) }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := callRPC(ctx, clientSide, &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     "request-1",
		Method: rpcapi.RPCMethodServerWorkspaceCreate,
		Params: mustRPCParams(rpcapi.WorkspaceCreateRequest{
			Name: "workspace-a", WorkflowName: "workflow-a",
		}, (*rpcapi.RPCPayload).FromWorkspaceCreateRequest),
	})
	if err != nil {
		t.Fatalf("callRPC() error = %v", err)
	}
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeBadRequest {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Error.Message != "unchanged client message" {
		t.Fatalf("response message = %q", resp.Error.Message)
	}
	if err := clientSide.Close(); err != nil {
		t.Fatalf("client Close() error = %v", err)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server Handle() error = %v", err)
	}

	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level.String() != "WARN" {
		t.Fatalf("level = %s, want WARN", record.Level)
	}
	for key, want := range map[string]any{
		"transport": "rpc", "surface": "peer-rpc", "operation": "server.workspace.create",
		"result": "client_error", "rpc_code": int64(400), "request_id": "request-1",
		"error_code": "INVALID_WORKSPACE", "workspace_name": "workspace-a", "workflow_name": "workflow-a",
	} {
		if got := attrs[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

func TestRPCServerCleanEOFDoesNotLogCompletion(t *testing.T) {
	capture := captureSlog(t)
	serverSide, clientSide := net.Pipe()
	serverErrCh := make(chan error, 1)
	go func() { serverErrCh <- (&rpcServer{}).Handle(serverSide) }()

	if err := clientSide.Close(); err != nil {
		t.Fatalf("client Close() error = %v", err)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server Handle() error = %v", err)
	}
	capture.mu.Lock()
	defer capture.mu.Unlock()
	if len(capture.records) != 0 {
		t.Fatalf("records = %d, want 0", len(capture.records))
	}
}

func TestRPCServerLogsMalformedRequestAfterFirstFrame(t *testing.T) {
	capture := captureSlog(t)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	serverErrCh := make(chan error, 1)
	go func() { serverErrCh <- (&rpcServer{}).Handle(serverSide) }()

	if err := rpcapi.WriteFrame(clientSide, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{0xff}}); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	if err := clientSide.Close(); err != nil {
		t.Fatalf("client Close() error = %v", err)
	}
	if err := <-serverErrCh; err == nil {
		t.Fatal("server Handle() error = nil, want malformed request error")
	}

	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level.String() != "ERROR" {
		t.Fatalf("level = %s, want ERROR", record.Level)
	}
	for key, want := range map[string]any{
		"transport": "rpc", "surface": "peer-rpc", "operation": "unknown",
		"result": "transport_error", "status_class": "unknown",
	} {
		if got := attrs[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	for _, key := range []string{"rpc_code", "request_id", "error_message"} {
		if _, ok := attrs[key]; ok {
			t.Errorf("unexpected %s = %#v", key, attrs[key])
		}
	}
}

func TestRPCServerLogsExistingParseErrorResponseAsWarn(t *testing.T) {
	capture := captureSlog(t)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- handleRPCWithStreamObserved(serverSide, func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
			return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeParseError, Message: "secret parse text"}.RPCResponse(), nil
		}, nil, &rpcObservationOptions{peerPublicKey: "peer-key"})
	}()

	resp, err := callRPC(context.Background(), clientSide, &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: "parse-1", Method: rpcapi.RPCMethodAllPing,
	})
	if err != nil {
		t.Fatalf("callRPC() error = %v", err)
	}
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeParseError {
		t.Fatalf("response = %#v", resp)
	}
	_ = clientSide.Close()
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server error = %v", err)
	}
	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level.String() != "WARN" || attrs["rpc_code"] != int64(rpcapi.RPCErrorCodeParseError) || attrs["result"] != "client_error" {
		t.Fatalf("record = (%s, %#v)", record.Level, attrs)
	}
	if strings.Contains(fmt.Sprint(attrs), "secret parse text") {
		t.Fatalf("response text leaked into attrs: %#v", attrs)
	}
}

func TestRPCServerLogsRethrownPanic(t *testing.T) {
	capture := captureSlog(t)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	stream, err := newRPCStream(context.Background(), serverSide)
	if err != nil {
		t.Fatalf("newRPCStream() error = %v", err)
	}
	defer stream.Close()

	writerErr := make(chan error, 1)
	go func() {
		err := rpcapi.WriteRequest(clientSide, &rpcapi.RPCRequest{
			V: rpcapi.RPCVersionV1, Id: "panic-1", Method: rpcapi.RPCMethodAllPing,
		})
		if err == nil {
			err = rpcapi.WriteEOS(clientSide)
		}
		writerErr <- err
	}()

	var panicValue any
	func() {
		defer func() { panicValue = recover() }()
		_, _ = handleRPCStreamRequestObserved(
			stream,
			func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
				panic("secret panic value")
			},
			nil,
			&rpcObservationOptions{peerPublicKey: "peer-key"},
		)
	}()
	if panicValue == nil {
		t.Fatal("RPC handler did not rethrow panic")
	}
	if err := <-writerErr; err != nil {
		t.Fatalf("write request error = %v", err)
	}

	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level.String() != "ERROR" || attrs["result"] != "panic" || attrs["status_class"] != "unknown" {
		t.Fatalf("record = (%s, %#v)", record.Level, attrs)
	}
	if attrs["operation"] != string(rpcapi.RPCMethodAllPing) || attrs["request_id"] != "panic-1" || attrs["peer_public_key"] != "peer-key" {
		t.Fatalf("record = %#v", attrs)
	}
	if _, ok := attrs["rpc_code"]; ok {
		t.Fatalf("panic fabricated rpc_code: %#v", attrs)
	}
	if strings.Contains(fmt.Sprint(attrs), "secret panic value") {
		t.Fatalf("panic value leaked into attrs: %#v", attrs)
	}
}

func TestRPCObservationResultMapsHTTPStyleServerCodes(t *testing.T) {
	if got := rpcObservationResult(false, 500, nil); got != observability.ResultServerError {
		t.Fatalf("rpcObservationResult(500) = %q", got)
	}
}

func TestRPCServerLogsStreamingDispatchCompletionOnce(t *testing.T) {
	capture := captureSlog(t)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- handleRPCWithStreamObserved(
			serverSide,
			func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
				t.Error("ordinary dispatch was called")
				return nil, nil
			},
			func(_ context.Context, stream *rpcStream, req *rpcapi.RPCRequest) (bool, error) {
				if err := stream.ReadEOS(); err != nil {
					return false, err
				}
				response, err := newRPCPingResponse(req.Id, rpcapi.PingResponse{ServerTime: 123})
				if err != nil {
					return false, err
				}
				if _, err := stream.WriteResponseEnvelopeForMethod(req.Method, response); err != nil {
					return false, err
				}
				if err := stream.WriteEOS(); err != nil {
					return false, err
				}
				return true, nil
			},
			&rpcObservationOptions{peerPublicKey: "peer-key"},
		)
	}()

	resp, err := callRPC(context.Background(), clientSide, &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: "stream-1", Method: rpcapi.RPCMethodAllPing,
	})
	if err != nil {
		t.Fatalf("callRPC() error = %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("response = %#v", resp)
	}
	_ = clientSide.Close()
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server error = %v", err)
	}
	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level.String() != "INFO" || attrs["result"] != "success" || attrs["operation"] != string(rpcapi.RPCMethodAllPing) {
		t.Fatalf("record = (%s, %#v)", record.Level, attrs)
	}
	if _, ok := attrs["rpc_code"]; ok {
		t.Fatalf("success fabricated rpc_code: %#v", attrs)
	}
}

func TestRPCServerLogsStreamingErrorResponse(t *testing.T) {
	capture := captureSlog(t)
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- handleRPCWithStreamObserved(
			serverSide,
			func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) { return nil, nil },
			func(_ context.Context, stream *rpcStream, req *rpcapi.RPCRequest) (bool, error) {
				if err := stream.ReadEOS(); err != nil {
					return false, err
				}
				return true, writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "secret invalid params")
			},
			&rpcObservationOptions{peerPublicKey: "peer-key"},
		)
	}()

	resp, err := callRPC(context.Background(), clientSide, &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: "stream-error-1", Method: rpcapi.RPCMethodAllSpeedTestRun,
	})
	if err != nil {
		t.Fatalf("callRPC() error = %v", err)
	}
	if resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
		t.Fatalf("response = %#v", resp)
	}
	_ = clientSide.Close()
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server error = %v", err)
	}
	record, attrs := onlyCapturedRecord(t, capture)
	if record.Level.String() != "WARN" || attrs["result"] != "client_error" || attrs["rpc_code"] != int64(rpcapi.RPCErrorCodeInvalidParams) {
		t.Fatalf("record = (%s, %#v)", record.Level, attrs)
	}
	if strings.Contains(fmt.Sprint(attrs), "secret invalid params") {
		t.Fatalf("response text leaked into attrs: %#v", attrs)
	}
}

type invalidWorkspaceAdminService struct{}

type fixedWorkflowAdminService struct {
	workflow.WorkflowAdminService
	value apitypes.Workflow
}

func (s fixedWorkflowAdminService) GetWorkflow(context.Context, adminhttp.GetWorkflowRequestObject) (adminhttp.GetWorkflowResponseObject, error) {
	return adminhttp.GetWorkflow200JSONResponse(s.value), nil
}

func (invalidWorkspaceAdminService) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	return nil, nil
}

func (invalidWorkspaceAdminService) CreateWorkspace(context.Context, adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	return adminhttp.CreateWorkspace400JSONResponse(apitypes.NewErrorResponse("INVALID_WORKSPACE", "unchanged client message")), nil
}

func (invalidWorkspaceAdminService) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return nil, nil
}

func (invalidWorkspaceAdminService) GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return nil, nil
}

func (invalidWorkspaceAdminService) PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	return nil, nil
}

func TestRPCServerStreamDispatchUsesConsumedContinuationEOS(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- handleRPCWithStream(
			serverSide,
			func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
				t.Error("non-stream dispatch should not be called")
				return nil, nil
			},
			func(_ context.Context, stream *rpcStream, req *rpcapi.RPCRequest) (bool, error) {
				if err := stream.ReadEOS(); err != nil {
					return false, err
				}
				resp, err := newRPCPingResponse(req.Id, rpcapi.PingResponse{ServerTime: 123})
				if err != nil {
					return false, err
				}
				if _, err := stream.WriteResponseEnvelopeForMethod(req.Method, resp); err != nil {
					return false, err
				}
				if err := stream.WriteEOS(); err != nil {
					return false, err
				}
				return true, nil
			},
		)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	clientStream, err := newRPCStream(ctx, clientSide)
	if err != nil {
		t.Fatalf("newRPCStream(client) error = %v", err)
	}
	defer clientStream.Close()

	req := &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     string(bytes.Repeat([]byte("r"), rpcapi.MaxFrameSize+1024)),
		Method: rpcapi.RPCMethodAllPing,
	}
	if err := clientStream.WriteRequestEnvelope(req); err != nil {
		t.Fatalf("WriteRequestEnvelope() error = %v", err)
	}
	if err := clientStream.WriteEOS(); err != nil {
		t.Fatalf("WriteEOS() error = %v", err)
	}

	resp, responseEOS, err := clientStream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodAllPing)
	if err != nil {
		t.Fatalf("ReadResponseEnvelopeForMethod() error = %v", err)
	}
	if !responseEOS {
		if err := clientStream.ReadEOS(); err != nil {
			t.Fatalf("ReadEOS() error = %v", err)
		}
	}
	if resp.Id != req.Id {
		t.Fatalf("response id = %q, want %q", resp.Id, req.Id)
	}
	got, err := resp.Result.AsPingResponse()
	if err != nil {
		t.Fatalf("AsPingResponse() error = %v", err)
	}
	if got.ServerTime != 123 {
		t.Fatalf("server_time = %d, want 123", got.ServerTime)
	}
	if err := clientSide.Close(); err != nil {
		t.Fatalf("client close error = %v", err)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func TestRPCClientCallContextTimeout(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	readDone := make(chan *rpcapi.RPCRequest, 1)
	go func() {
		req, _ := rpcapi.ReadRequest(serverSide)
		readDone <- req
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	params, err := newRPCPingRequestParams(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()})
	if err != nil {
		t.Fatalf("newRPCPingRequestParams() error = %v", err)
	}

	_, err = callRPC(ctx, clientSide, &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     "timeout",
		Method: rpcapi.RPCMethodAllPing,
		Params: params,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call timeout err = %v, want %v", err, context.DeadlineExceeded)
	}

	if req := <-readDone; req == nil || req.Id != "timeout" {
		t.Fatalf("server received request = %+v", req)
	} else {
		assertRPCPingRequestHasTimestamp(t, req)
	}
}

func TestRPCClientCallContextCancel(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	readDone := make(chan *rpcapi.RPCRequest, 1)
	go func() {
		req, _ := rpcapi.ReadRequest(serverSide)
		readDone <- req
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if req := <-readDone; req != nil {
			cancel()
		}
	}()

	params, err := newRPCPingRequestParams(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()})
	if err != nil {
		t.Fatalf("newRPCPingRequestParams() error = %v", err)
	}

	_, err = callRPC(ctx, clientSide, &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     "cancel",
		Method: rpcapi.RPCMethodAllPing,
		Params: params,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Call cancel err = %v, want %v", err, context.Canceled)
	}
}

func TestRPCClientCallValidatesRequest(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	if _, err := callRPC(context.Background(), nil, &rpcapi.RPCRequest{Id: "nil-conn"}); err == nil || err.Error() != "rpc: nil conn" {
		t.Fatalf("call(nil conn) err = %v", err)
	}
	if _, err := callRPC(context.Background(), clientSide, nil); err == nil || err.Error() != "rpc: nil request" {
		t.Fatalf("call(nil) err = %v", err)
	}
	if _, err := callRPC(context.Background(), clientSide, &rpcapi.RPCRequest{Method: rpcapi.RPCMethodAllPing}); err == nil || err.Error() != "rpc: request id required" {
		t.Fatalf("call(empty id) err = %v", err)
	}
}

func TestRPCClientPingErrorPaths(t *testing.T) {
	t.Run("error response", func(t *testing.T) {
		serverSide, clientSide := net.Pipe()
		defer serverSide.Close()
		defer clientSide.Close()

		go func() {
			req, _ := readRPCRequestWithEOS(serverSide)
			_ = writeRPCResponseWithEOS(serverSide, req.Method, rpcapi.Error{RequestID: req.Id, Code: -1, Message: "boom"}.RPCResponse())
		}()

		_, err := callRPCPing(context.Background(), clientSide, "ping-error")
		if err == nil || err.Error() != "rpc: boom" {
			t.Fatalf("Ping(error response) err = %v", err)
		}
		var rpcErr rpcapi.Error
		if !errors.As(err, &rpcErr) {
			t.Fatalf("Ping(error response) err = %T, want rpcapi.Error", err)
		}
		if rpcErr.RequestID != "ping-error" || rpcErr.Code != -1 || rpcErr.Message != "boom" {
			t.Fatalf("Ping(error response) rpc error = %+v", rpcErr)
		}
	})

	t.Run("missing result", func(t *testing.T) {
		serverSide, clientSide := net.Pipe()
		defer serverSide.Close()
		defer clientSide.Close()

		go func() {
			req, _ := readRPCRequestWithEOS(serverSide)
			_ = writeRPCResponseWithEOS(serverSide, req.Method, &rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: req.Id})
		}()

		_, err := callRPCPing(context.Background(), clientSide, "ping-missing")
		if err == nil || err.Error() != "rpc: missing ping result" {
			t.Fatalf("Ping(missing result) err = %v", err)
		}
	})
}

func rpcServerTimeForID(id string) int64 {
	if id == "req-1" {
		return 1
	}
	return 2
}

func readRPCRequestWithEOS(conn net.Conn) (*rpcapi.RPCRequest, error) {
	req, err := rpcapi.ReadRequest(conn)
	if err != nil {
		return nil, err
	}
	if err := rpcapi.ReadEOS(conn); err != nil {
		return nil, err
	}
	return req, nil
}

func writeRPCResponseWithEOS(conn net.Conn, method rpcapi.RPCMethod, resp *rpcapi.RPCResponse) error {
	if err := rpcapi.WriteResponseForMethod(conn, method, resp); err != nil {
		return err
	}
	return rpcapi.WriteEOS(conn)
}

func assertRPCPingRequestHasTimestamp(t *testing.T, req *rpcapi.RPCRequest) {
	t.Helper()
	if req.Params == nil {
		t.Fatal("ping request params missing")
	}
	params, err := req.Params.AsPingRequest()
	if err != nil {
		t.Fatalf("ping request params decode error = %v", err)
	}
	if params.ClientSendTime <= 0 {
		t.Fatalf("ping request client_send_time = %d", params.ClientSendTime)
	}
}
