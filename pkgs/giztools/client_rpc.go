package giztools

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const (
	// PeerRPCService is the giznet service carrying Peer RPC.
	PeerRPCService = 0

	maxClientRPCEnvelopeBytes = rpcapi.MaxFrameSize * 16
	maxClientRPCArguments     = 64 << 10
	maxClientRPCResult        = 64 << 10
)

var ErrClientToolUnavailable = errors.New("giztools: client Tool unavailable")
var clientToolName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

// ServiceDialer is the current-Peer connection capability needed by
// ClientRPCExecutor. GizClaw owns selecting the concrete Peer.
type ServiceDialer interface {
	Dial(service uint64) (net.Conn, error)
}

// ClientRPCExecutor invokes one canonically named Tool through the current
// Peer's client.tool.invoke entrypoint. It does not resolve Tools or select a
// Peer and never retries an invocation.
type ClientRPCExecutor struct{}

func (ClientRPCExecutor) Invoke(
	ctx context.Context,
	peer ServiceDialer,
	name string,
	args json.RawMessage,
) (json.RawMessage, error) {
	if peer == nil {
		return nil, ErrClientToolUnavailable
	}
	if !clientToolName.MatchString(name) {
		return nil, fmt.Errorf("giztools: invalid client Tool name %q", name)
	}
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if len(args) > maxClientRPCArguments {
		return nil, fmt.Errorf("giztools: client Tool arguments exceed %d bytes", maxClientRPCArguments)
	}
	var parameters map[string]any
	if err := json.Unmarshal(args, &parameters); err != nil {
		return nil, fmt.Errorf("giztools: decode client Tool arguments: %w", err)
	}
	if parameters == nil {
		return nil, errors.New("giztools: client Tool arguments must be a JSON object")
	}
	stream, err := peer.Dial(PeerRPCService)
	if err != nil {
		return nil, ErrClientToolUnavailable
	}
	defer stream.Close()
	stop, err := bindConnContext(ctx, stream)
	if err != nil {
		return nil, err
	}
	defer stop()

	var params rpcapi.RPCPayload
	if err := params.FromToolInvokeRequest(rpcapi.ToolInvokeRequest{InvokeName: name, Args: parameters}); err != nil {
		return nil, fmt.Errorf("giztools: encode client Tool request: %w", err)
	}
	requestID, err := clientRPCRequestID()
	if err != nil {
		return nil, err
	}
	request := &rpcapi.RPCRequest{
		V: rpcapi.RPCVersionV1, Id: requestID,
		Method: rpcapi.RPCMethodClientToolInvoke, Params: &params,
	}
	frame, err := rpcapi.NewRequestFrame(request)
	if err != nil {
		return nil, fmt.Errorf("giztools: encode client Tool RPC envelope: %w", err)
	}
	if err := writeRPCEnvelope(stream, frame.Payload); err != nil {
		return nil, normalizeClientRPCIOError(ctx, "write request", err)
	}
	if err := rpcapi.WriteEOS(stream); err != nil {
		return nil, normalizeClientRPCIOError(ctx, "write request end", err)
	}
	payload, consumedEOS, err := readRPCEnvelope(stream)
	if err != nil {
		return nil, normalizeClientRPCIOError(ctx, "read response", err)
	}
	if !consumedEOS {
		if err := rpcapi.ReadEOS(stream); err != nil {
			return nil, normalizeClientRPCIOError(ctx, "read response end", err)
		}
	}
	response, err := rpcapi.DecodeResponseFrameForMethod(
		rpcapi.RPCMethodClientToolInvoke,
		rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload},
	)
	if err != nil {
		return nil, fmt.Errorf("giztools: decode client Tool response: %w", err)
	}
	if response.Id != requestID {
		return nil, errors.New("giztools: client Tool response ID mismatch")
	}
	if response.Error != nil {
		if response.Error.Code == rpcapi.StatusCodeUnimplemented {
			return nil, ErrClientToolUnavailable
		}
		return nil, fmt.Errorf("giztools: client Tool RPC failed with code %d", response.Error.Code)
	}
	if response.Result == nil {
		return nil, errors.New("giztools: client Tool response is missing result")
	}
	result, err := response.Result.AsToolInvokeResponse()
	if err != nil {
		return nil, fmt.Errorf("giztools: decode client Tool result: %w", err)
	}
	data := []byte(result.DataJson)
	if len(data) == 0 {
		data = []byte(`null`)
	}
	if len(data) > maxClientRPCResult {
		return nil, fmt.Errorf("giztools: client Tool result exceeds %d bytes", maxClientRPCResult)
	}
	if !json.Valid(data) {
		return nil, errors.New("giztools: client Tool result is invalid JSON")
	}
	return json.RawMessage(append([]byte(nil), data...)), nil
}

func writeRPCEnvelope(writer io.Writer, payload []byte) error {
	if len(payload) > maxClientRPCEnvelopeBytes {
		return fmt.Errorf("giztools: RPC envelope exceeds %d bytes", maxClientRPCEnvelopeBytes)
	}
	if len(payload) <= rpcapi.MaxFrameSize {
		return rpcapi.WriteFrame(writer, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload})
	}
	for len(payload) > 0 {
		size := min(len(payload), rpcapi.MaxFrameSize)
		if err := rpcapi.WriteFrame(writer, rpcapi.Frame{
			Type: rpcapi.FrameTypeText, Payload: payload[:size],
		}); err != nil {
			return err
		}
		payload = payload[size:]
	}
	return nil
}

func readRPCEnvelope(reader io.Reader) ([]byte, bool, error) {
	first, err := rpcapi.ReadFrame(reader)
	if err != nil {
		return nil, false, err
	}
	switch first.Type {
	case rpcapi.FrameTypeBinary:
		return first.Payload, false, nil
	case rpcapi.FrameTypeText:
		var buffer bytes.Buffer
		buffer.Write(first.Payload)
		for {
			frame, err := rpcapi.ReadFrame(reader)
			if err != nil {
				return nil, false, err
			}
			if frame.Type == rpcapi.FrameTypeEOS {
				return buffer.Bytes(), true, nil
			}
			if frame.Type != rpcapi.FrameTypeText {
				return nil, false, fmt.Errorf("giztools: expected RPC continuation frame, got %d", frame.Type)
			}
			if buffer.Len()+len(frame.Payload) > maxClientRPCEnvelopeBytes {
				return nil, false, fmt.Errorf("giztools: RPC envelope exceeds %d bytes", maxClientRPCEnvelopeBytes)
			}
			buffer.Write(frame.Payload)
		}
	default:
		return nil, false, fmt.Errorf("giztools: expected RPC protobuf frame, got %d", first.Type)
	}
}

func bindConnContext(ctx context.Context, conn net.Conn) (func(), error) {
	if err := ctx.Err(); err != nil {
		return func() {}, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return func() {}, err
		}
	}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-done:
		}
	}()
	return func() {
		close(done)
		<-stopped
		_ = conn.SetDeadline(time.Time{})
	}, nil
}

func normalizeClientRPCIOError(ctx context.Context, operation string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		if deadline, ok := ctx.Deadline(); ok && !time.Now().Before(deadline) {
			return context.DeadlineExceeded
		}
	}
	return fmt.Errorf("giztools: client Tool %s: %w", operation, err)
}

func clientRPCRequestID() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("giztools: create client Tool request ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
