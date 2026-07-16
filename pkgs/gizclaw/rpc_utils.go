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

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/observability"
)

var errRPCMissingResult = errors.New("rpc: missing result")

type rpcStreamDispatch func(context.Context, *rpcStream, *rpcapi.RPCRequest) (bool, error)

type rpcObservationOptions struct {
	peerPublicKey string
}

func handleRPC(conn net.Conn, dispatch func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error)) error {
	return handleRPCWithStream(conn, dispatch, nil)
}

func handleRPCWithStream(
	conn net.Conn,
	dispatch func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error),
	streamDispatch rpcStreamDispatch,
) error {
	return handleRPCWithStreamObserved(conn, dispatch, streamDispatch, nil)
}

func handleRPCWithStreamObserved(
	conn net.Conn,
	dispatch func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error),
	streamDispatch rpcStreamDispatch,
	observation *rpcObservationOptions,
) error {
	stream, err := newRPCStream(context.Background(), conn)
	if err != nil {
		return err
	}
	defer stream.Close()

	for {
		done, err := handleRPCStreamRequestObserved(stream, dispatch, streamDispatch, observation)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

func handleRPCStreamRequest(
	stream *rpcStream,
	dispatch func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error),
	streamDispatch rpcStreamDispatch,
) (bool, error) {
	return handleRPCStreamRequestObserved(stream, dispatch, streamDispatch, nil)
}

func handleRPCStreamRequestObserved(
	stream *rpcStream,
	dispatch func(context.Context, *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error),
	streamDispatch rpcStreamDispatch,
	observation *rpcObservationOptions,
) (done bool, resultErr error) {
	first, err := stream.ReadFrame()
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return true, nil
		}
		return false, err
	}
	var (
		outcome       *observability.Outcome
		completionCtx = stream.Context()
		response      *rpcapi.RPCResponse
		observedErr   error
		wasCanceled   bool
	)
	if observation != nil {
		outcome = observability.NewOutcome(observability.TransportRPC, observability.SurfacePeerRPC, "unknown")
		outcome.SetPeer(observation.peerPublicKey, "")
		completionCtx = observability.WithOutcome(completionCtx, outcome)
		defer func() {
			panicValue := recover()
			code := 0
			result := observability.ResultSuccess
			if response != nil && response.Error != nil {
				code = int(response.Error.Code)
				result = rpcObservationResult(wasCanceled, code, nil)
			}
			if observedErr != nil {
				result = rpcObservationResult(wasCanceled, code, observedErr)
			}
			if panicValue != nil {
				outcome.MarkPanic()
				result = observability.ResultPanic
			}
			outcome.SetRPC(code, result)
			observability.Log(completionCtx, outcome)
			if panicValue != nil {
				panic(panicValue)
			}
		}()
	}
	req, requestEOS, err := stream.decodeRequestEnvelope(first)
	if err != nil {
		observedErr = err
		wasCanceled = completionCtx.Err() != nil
		return false, err
	}
	if outcome != nil && req != nil {
		outcome.SetOperation(string(req.Method))
		outcome.SetRequestID(req.Id)
	}
	previousResponseObserver := stream.responseObserver
	stream.responseObserver = func(resp *rpcapi.RPCResponse) {
		response = resp
	}
	defer func() {
		stream.responseObserver = previousResponseObserver
	}()
	if streamDispatch != nil {
		previousRequestEOSAlreadyConsumed := stream.requestEOSAlreadyConsumed
		stream.requestEOSAlreadyConsumed = requestEOS
		defer func() {
			stream.requestEOSAlreadyConsumed = previousRequestEOSAlreadyConsumed
		}()
		handled, err := streamDispatch(completionCtx, stream, req)
		if err != nil {
			observedErr = err
			wasCanceled = completionCtx.Err() != nil
			return false, err
		}
		if handled {
			return false, nil
		}
	}
	if !requestEOS {
		if err := stream.ReadEOS(); err != nil {
			observedErr = err
			wasCanceled = completionCtx.Err() != nil
			return false, err
		}
	}

	ctx, stop := rpcConnContext(stream.conn)
	if outcome != nil {
		ctx = observability.WithOutcome(ctx, outcome)
		completionCtx = ctx
	}
	resp, err := dispatch(ctx, req)
	response = resp
	wasCanceled = ctx.Err() != nil
	cause := context.Cause(ctx)
	stop()
	if err != nil {
		observedErr = err
		if wasCanceled && cause != nil {
			return false, cause
		}
		return false, err
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
	if _, err := stream.WriteResponseEnvelopeForMethod(req.Method, resp); err != nil {
		observedErr = err
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return true, nil
		}
		return false, err
	}
	if err := stream.WriteEOS(); err != nil {
		observedErr = err
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func rpcObservationResult(canceled bool, code int, err error) observability.Result {
	if canceled {
		return observability.ResultCanceled
	}
	if err != nil {
		return observability.ResultTransportError
	}
	if code == 0 {
		return observability.ResultSuccess
	}
	if code == int(rpcapi.RPCErrorCodeInternalError) || code >= 500 && code <= 599 {
		return observability.ResultServerError
	}
	return observability.ResultClientError
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

	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	if err := stream.WriteRequestEnvelope(req); err != nil {
		return nil, err
	}
	if err := stream.WriteEOS(); err != nil {
		return nil, err
	}
	resp, responseEOS, err := stream.ReadResponseEnvelopeForMethod(req.Method)
	if err != nil {
		return nil, err
	}
	if !responseEOS {
		if err := stream.ReadEOS(); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func callRPCPing(ctx context.Context, conn net.Conn, id string) (*rpcapi.PingResponse, error) {
	params, err := newRPCPingRequestParams(rpcapi.PingRequest{ClientSendTime: time.Now().UnixMilli()})
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest(id, rpcapi.RPCMethodAllPing, params), rpcapi.RPCPayload.AsPingResponse)
	if err != nil {
		return nil, wrapRPCResultError("ping", err)
	}
	return result, nil
}

func newRPCRequest(id string, method rpcapi.RPCMethod, params *rpcapi.RPCPayload) *rpcapi.RPCRequest {
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
	decode func(rpcapi.RPCPayload) (T, error),
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

func newRPCPingRequestParams(request rpcapi.PingRequest) (*rpcapi.RPCPayload, error) {
	var params rpcapi.RPCPayload
	if err := params.FromPingRequest(request); err != nil {
		return nil, err
	}
	return &params, nil
}

func newRPCPingResponse(id string, response rpcapi.PingResponse) (*rpcapi.RPCResponse, error) {
	return newRPCResultResponse(id, response, (*rpcapi.RPCPayload).FromPingResponse)
}

func newRPCResultResponse[T any](id string, result T, encode func(*rpcapi.RPCPayload, T) error) (*rpcapi.RPCResponse, error) {
	var body rpcapi.RPCPayload
	if err := encode(&body, result); err != nil {
		return nil, err
	}
	return &rpcapi.RPCResponse{
		V:      rpcapi.RPCVersionV1,
		Id:     id,
		Result: &body,
	}, nil
}

func newRPCRequestParams[T any](request T, encode func(*rpcapi.RPCPayload, T) error) (*rpcapi.RPCPayload, error) {
	var params rpcapi.RPCPayload
	if err := encode(&params, request); err != nil {
		return nil, err
	}
	return &params, nil
}

func validateRPCParams[T any](params *rpcapi.RPCPayload, decode func(rpcapi.RPCPayload) (T, error)) error {
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
		Message:   fmt.Sprintf("unexpected server service response: %T", response),
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
