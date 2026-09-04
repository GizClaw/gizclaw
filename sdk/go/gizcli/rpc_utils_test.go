package gizcli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCClientPingSingleRequestResponse(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	client := &rpcClient{}

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

	ping, err := client.Ping(ctx, clientSide, "req-1")
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

func TestRPCClientRejectsMismatchedResponseID(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	go func() {
		req, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			return
		}
		_ = writeRPCResponseWithEOS(serverSide, req.Method, &rpcapi.RPCResponse{
			V:  rpcapi.RPCVersionV1,
			Id: "another-session",
		})
	}()
	_, err := callRPC(context.Background(), clientSide, &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     "expected-session",
		Method: rpcapi.RPCMethodAllPing,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched response id error = %v", err)
	}
}

func TestRPCClientPingErrorPaths(t *testing.T) {
	t.Run("error response", func(t *testing.T) {
		serverSide, clientSide := net.Pipe()
		defer serverSide.Close()
		defer clientSide.Close()

		client := &rpcClient{}

		go func() {
			req, _ := readRPCRequestWithEOS(serverSide)
			_ = writeRPCResponseWithEOS(serverSide, req.Method, rpcapi.Error{RequestID: req.Id, Code: -1, Message: "boom"}.RPCResponse())
		}()

		_, err := client.Ping(context.Background(), clientSide, "ping-error")
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

		client := &rpcClient{}

		go func() {
			req, _ := readRPCRequestWithEOS(serverSide)
			_ = writeRPCResponseWithEOS(serverSide, req.Method, &rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: req.Id})
		}()

		_, err := client.Ping(context.Background(), clientSide, "ping-missing")
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

// rpcAPIError starts from an HTTP status and used to cast it straight into the
// code field, which put a value outside the canonical set on the wire.
func TestRPCAPIErrorMapsHTTPStatusToCanonicalCode(t *testing.T) {
	for _, test := range []struct {
		status int
		want   rpcapi.StatusCode
	}{
		{http.StatusNotFound, rpcapi.StatusCodeNotFound},
		{http.StatusForbidden, rpcapi.StatusCodePermissionDenied},
		{http.StatusBadRequest, rpcapi.StatusCodeInvalidArgument},
		{http.StatusUnauthorized, rpcapi.StatusCodeUnauthenticated},
		{http.StatusConflict, rpcapi.StatusCodeAborted},
		{http.StatusInternalServerError, rpcapi.StatusCodeInternal},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			response := rpcAPIError("req-1", test.status, apitypes.ErrorResponse{})
			if response.Error == nil {
				t.Fatalf("rpcAPIError(%d) = %#v, want a status", test.status, response)
			}
			if response.Error.Code != test.want {
				t.Fatalf("rpcAPIError(%d) code = %s, want %s", test.status, response.Error.Code, test.want)
			}
			if !response.Error.Code.Valid() {
				t.Fatalf("rpcAPIError(%d) produced a code outside the canonical set", test.status)
			}
		})
	}
}
