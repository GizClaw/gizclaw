package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

var errRPCMissingResult = errors.New("rpc: missing result")

func handleRPC(conn net.Conn, dispatch func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error)) error {
	req, err := rpcapi.ReadRequest(conn)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}

	ctx, stop := rpcConnContext(conn)
	defer stop()

	resp, err := dispatch(ctx, req)
	if err != nil {
		if ctx.Err() != nil {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
		}
		return err
	}
	if resp == nil {
		resp = &rpcapi.RPCResponse{V: rpcapi.RPCVersionV1, Id: req.Id}
	}
	if resp.Id == "" {
		resp.Id = req.Id
	}
	if resp.V == 0 {
		resp.V = rpcapi.RPCVersionV1
	}
	if err := rpcapi.WriteResponse(conn, resp); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
	return nil
}

func handleRPCPing(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	if _, err := req.Params.AsPingRequest(); err != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "invalid params"}.RPCResponse(), nil
	}
	return newRPCPingResponse(req.Id, rpcapi.PingResponse{ServerTime: time.Now().UnixMilli()})
}

func rpcConnContext(conn net.Conn) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		var b [1]byte
		_, err := conn.Read(b[:])
		if err == nil {
			cancel(io.ErrUnexpectedEOF)
			return
		}
		select {
		case <-done:
		default:
			cancel(err)
		}
	}()

	stop := func() {
		close(done)
		_ = conn.SetReadDeadline(time.Now())
		<-stopped
		_ = conn.SetReadDeadline(time.Time{})
		cancel(nil)
	}
	return ctx, stop
}

func callRPC(ctx context.Context, conn net.Conn, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if conn == nil {
		return nil, errors.New("rpc: nil conn")
	}
	if req == nil {
		return nil, errors.New("rpc: nil request")
	}
	if req.Id == "" {
		return nil, errors.New("rpc: request id required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	stopCancel := make(chan struct{})
	cancelDone := make(chan struct{})
	defer func() {
		close(stopCancel)
		<-cancelDone
		_ = conn.SetDeadline(time.Time{})
	}()
	go func() {
		defer close(cancelDone)
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-stopCancel:
		}
	}()

	if err := rpcapi.WriteRequest(conn, req); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if hasDeadline && time.Now().After(deadline) {
			return nil, context.DeadlineExceeded
		}
		return nil, err
	}

	resp, err := rpcapi.ReadResponse(conn)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if hasDeadline && time.Now().After(deadline) {
			return nil, context.DeadlineExceeded
		}
		return nil, err
	}
	return resp, nil
}

func callRPCPing(ctx context.Context, conn net.Conn, id string) (*rpcapi.PingResponse, error) {
	params, err := newRPCPingRequestParams(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()})
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodPeerPing, params), rpcapi.RPCResponse_Result.AsPingResponse)
	if err != nil {
		return nil, wrapRPCResultError("ping", err)
	}
	return result, nil
}

func newRPCRequest(id string, method rpcapi.RPCMethod, params *rpcapi.RPCRequest_Params) *rpcapi.RPCRequest {
	return &rpcapi.RPCRequest{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Method: method,
		Params: params,
	}
}

func callRPCResult[T any](
	ctx context.Context,
	conn net.Conn,
	req *rpcapi.RPCRequest,
	decode func(rpcapi.RPCResponse_Result) (T, error),
) (*T, error) {
	resp, err := callRPC(ctx, conn, req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("rpc: %w", rpcapi.Error{
			RequestID: resp.Id,
			Code:      resp.Error.Code,
			Message:   resp.Error.Message,
		})
	}
	if resp.Result == nil {
		return nil, errRPCMissingResult
	}
	result, err := decode(*resp.Result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func wrapRPCResultError(name string, err error) error {
	if errors.Is(err, errRPCMissingResult) {
		return fmt.Errorf("rpc: missing %s result", name)
	}
	var rpcErr rpcapi.Error
	if errors.As(err, &rpcErr) {
		return err
	}
	return fmt.Errorf("rpc: decode %s result: %w", name, err)
}

func newRPCPingRequestParams(request rpcapi.PingRequest) (*rpcapi.RPCRequest_Params, error) {
	var params rpcapi.RPCRequest_Params
	if err := params.FromPingRequest(request); err != nil {
		return nil, err
	}
	return &params, nil
}

func newRPCPingResponse(id string, response rpcapi.PingResponse) (*rpcapi.RPCResponse, error) {
	return newRPCResultResponse(id, response, (*rpcapi.RPCResponse_Result).FromPingResponse)
}

func newRPCResultResponse[T any](id string, result T, encode func(*rpcapi.RPCResponse_Result, T) error) (*rpcapi.RPCResponse, error) {
	var body rpcapi.RPCResponse_Result
	if err := encode(&body, result); err != nil {
		return nil, err
	}
	return &rpcapi.RPCResponse{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Result: &body,
	}, nil
}

func newRPCRequestParams[T any](request T, encode func(*rpcapi.RPCRequest_Params, T) error) (*rpcapi.RPCRequest_Params, error) {
	var params rpcapi.RPCRequest_Params
	if err := encode(&params, request); err != nil {
		return nil, err
	}
	return &params, nil
}

func validateRPCParams[T any](params *rpcapi.RPCRequest_Params, decode func(rpcapi.RPCRequest_Params) (T, error)) error {
	if params == nil {
		return nil
	}
	_, err := decode(*params)
	return err
}

func rpcInvalidParams(id string) *rpcapi.RPCResponse {
	return rpcapi.Error{RequestID: id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "invalid params"}.RPCResponse()
}

func rpcAPIError(id string, statusCode int, body apitypes.ErrorResponse) *rpcapi.RPCResponse {
	message := body.Error.Message
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return rpcapi.Error{RequestID: id, Code: rpcapi.RPCErrorCode(statusCode), Message: message}.RPCResponse()
}

func rpcUnexpectedResponse(id string, response any) *rpcapi.RPCResponse {
	return rpcapi.Error{
		RequestID: id,
		Code:      rpcapi.RPCErrorCodeInternalError,
		Message:   fmt.Sprintf("unexpected gear service response: %T", response),
	}.RPCResponse()
}

func convertRPCType[T any](value any) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
