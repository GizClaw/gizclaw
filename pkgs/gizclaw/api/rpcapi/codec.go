package rpcapi

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

// MaxFrameSize is the largest RPC frame payload accepted by ReadFrame.
const MaxFrameSize = 65535

// FrameType identifies the payload encoding used by an RPC frame.
type FrameType uint16

const (
	FrameTypeEOS    FrameType = 0
	FrameTypeJSON   FrameType = 1
	FrameTypeBinary FrameType = 2
	FrameTypeText   FrameType = 3
)

// Valid reports whether the frame type is known by this protocol version.
func (t FrameType) Valid() bool {
	switch t {
	case FrameTypeEOS, FrameTypeJSON, FrameTypeBinary, FrameTypeText:
		return true
	default:
		return false
	}
}

// Frame is one typed payload in an RPC stream.
type Frame struct {
	Type    FrameType
	Payload []byte
}

// WriteFrame writes a typed RPC frame.
func WriteFrame(w io.Writer, frame Frame) error {
	if !frame.Type.Valid() {
		return fmt.Errorf("rpc: unknown frame type: %d", frame.Type)
	}
	if len(frame.Payload) > MaxFrameSize {
		return fmt.Errorf("rpc: frame too large: %d", len(frame.Payload))
	}
	if frame.Type == FrameTypeEOS && len(frame.Payload) != 0 {
		return fmt.Errorf("rpc: EOS frame must be empty")
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint16(hdr[0:2], uint16(len(frame.Payload)))
	binary.LittleEndian.PutUint16(hdr[2:4], uint16(frame.Type))
	if len(frame.Payload) == 0 {
		return writeFull(w, hdr[:])
	}
	if bw, ok := w.(buffersWriter); ok {
		total := int64(len(hdr) + len(frame.Payload))
		n, err := bw.WriteBuffers(net.Buffers{hdr[:], frame.Payload})
		if err != nil {
			return err
		}
		if n != total {
			return io.ErrShortWrite
		}
		return nil
	}
	if err := writeFull(w, hdr[:]); err != nil {
		return err
	}
	return writeFull(w, frame.Payload)
}

type buffersWriter interface {
	WriteBuffers(net.Buffers) (int64, error)
}

// ReadFrame reads a typed RPC frame.
func ReadFrame(r io.Reader) (Frame, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	length := binary.LittleEndian.Uint16(hdr[0:2])
	frameType := FrameType(binary.LittleEndian.Uint16(hdr[2:4]))
	if !frameType.Valid() {
		return Frame{}, fmt.Errorf("rpc: unknown frame type: %d", frameType)
	}
	if frameType == FrameTypeEOS && length != 0 {
		return Frame{}, fmt.Errorf("rpc: EOS frame must be empty")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return Frame{}, err
	}
	return Frame{Type: frameType, Payload: buf}, nil
}

// ReadFrames reads frames until EOS. EOF before EOS is returned as an error.
func ReadFrames(r io.Reader) iter.Seq2[Frame, error] {
	return func(yield func(Frame, error) bool) {
		for {
			frame, err := ReadFrame(r)
			if err != nil {
				yield(Frame{}, err)
				return
			}
			if frame.Type == FrameTypeEOS {
				return
			}
			if !yield(frame, nil) {
				return
			}
		}
	}
}

// WriteEOS writes the end-of-stream frame for one RPC frame sequence.
func WriteEOS(w io.Writer) error {
	return WriteFrame(w, Frame{Type: FrameTypeEOS})
}

// ReadEOS reads and validates one end-of-stream frame.
func ReadEOS(r io.Reader) error {
	frame, err := ReadFrame(r)
	if err != nil {
		return err
	}
	if frame.Type != FrameTypeEOS {
		return fmt.Errorf("rpc: expected EOS frame, got type %d", frame.Type)
	}
	return nil
}

// WriteFrames writes frames from the sequence until it is exhausted or errors.
func WriteFrames(w io.Writer, frames iter.Seq2[Frame, error]) error {
	for frame, err := range frames {
		if err != nil {
			return err
		}
		if err := WriteFrame(w, frame); err != nil {
			return err
		}
	}
	return nil
}

// NewJSONFrame marshals a value into one compact JSON frame.
func NewJSONFrame(v any) (Frame, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: FrameTypeJSON, Payload: data}, nil
}

// DecodeJSONFrame unmarshals one JSON frame into v.
func DecodeJSONFrame(frame Frame, v any) error {
	if frame.Type != FrameTypeJSON {
		return fmt.Errorf("rpc: expected JSON frame, got type %d", frame.Type)
	}
	if err := json.Unmarshal(frame.Payload, v); err != nil {
		return err
	}
	return nil
}

// NewProtobufFrame marshals a protobuf message into one binary frame.
func NewProtobufFrame(m proto.Message) (Frame, error) {
	data, err := proto.Marshal(m)
	if err != nil {
		return Frame{}, err
	}
	return Frame{Type: FrameTypeBinary, Payload: data}, nil
}

// DecodeProtobufFrame unmarshals one binary protobuf frame into m.
func DecodeProtobufFrame(frame Frame, m proto.Message) error {
	if frame.Type != FrameTypeBinary {
		return fmt.Errorf("rpc: expected protobuf binary frame, got type %d", frame.Type)
	}
	if err := proto.Unmarshal(frame.Payload, m); err != nil {
		return err
	}
	return nil
}

// NewRequestFrame marshals an RPC request into one protobuf binary frame.
func NewRequestFrame(req *RPCRequest) (Frame, error) {
	msg, err := EncodeRPCRequest(req)
	if err != nil {
		return Frame{}, err
	}
	return NewProtobufFrame(msg)
}

// DecodeRequestFrame unmarshals one protobuf binary frame into an RPC request.
func DecodeRequestFrame(frame Frame) (*RPCRequest, error) {
	var msg rpcpb.RpcRequest
	if err := DecodeProtobufFrame(frame, &msg); err != nil {
		return nil, err
	}
	return DecodeRPCRequest(&msg)
}

// NewResponseFrame marshals an RPC response into one protobuf binary frame.
func NewResponseFrame(resp *RPCResponse) (Frame, error) {
	msg, err := EncodeRPCResponse(resp)
	if err != nil {
		return Frame{}, err
	}
	return NewProtobufFrame(msg)
}

// DecodeResponseFrame unmarshals one protobuf binary frame into an RPC response.
func DecodeResponseFrame(frame Frame) (*RPCResponse, error) {
	var msg rpcpb.RpcResponse
	if err := DecodeProtobufFrame(frame, &msg); err != nil {
		return nil, err
	}
	return DecodeRPCResponse(&msg)
}

// WriteRequest writes an RPC request as one protobuf binary frame.
func WriteRequest(w io.Writer, req *RPCRequest) error {
	frame, err := NewRequestFrame(req)
	if err != nil {
		return err
	}
	return WriteFrame(w, frame)
}

// ReadRequest reads an RPC request from one protobuf binary frame.
func ReadRequest(r io.Reader) (*RPCRequest, error) {
	frame, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	req, err := DecodeRequestFrame(frame)
	if err != nil {
		return nil, fmt.Errorf("rpc: unmarshal request: %w", err)
	}
	return req, nil
}

// ReadResponse reads an RPC response from one protobuf binary frame.
func ReadResponse(r io.Reader) (*RPCResponse, error) {
	frame, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	resp, err := DecodeResponseFrame(frame)
	if err != nil {
		return nil, fmt.Errorf("rpc: unmarshal response: %w", err)
	}
	return resp, nil
}

// WriteResponse writes an RPC response as one protobuf binary frame.
func WriteResponse(w io.Writer, resp *RPCResponse) error {
	frame, err := NewResponseFrame(resp)
	if err != nil {
		return err
	}
	return WriteFrame(w, frame)
}

// ReadResponses reads protobuf RPC response frames until EOS.
func ReadResponses(r io.Reader) iter.Seq2[*RPCResponse, error] {
	return func(yield func(*RPCResponse, error) bool) {
		for frame, err := range ReadFrames(r) {
			if err != nil {
				yield(nil, err)
				return
			}
			resp, err := DecodeResponseFrame(frame)
			if err != nil {
				yield(nil, fmt.Errorf("rpc: unmarshal response: %w", err))
				return
			}
			if !yield(resp, nil) {
				return
			}
		}
	}
}

// WriteResponses writes each RPC response as a protobuf binary frame.
func WriteResponses(w io.Writer, responses iter.Seq2[*RPCResponse, error]) error {
	for resp, err := range responses {
		if err != nil {
			return err
		}
		if err := WriteResponse(w, resp); err != nil {
			return err
		}
	}
	return WriteEOS(w)
}

// EncodeRPCRequest converts the public typed RPC request into the protobuf wire envelope.
func EncodeRPCRequest(req *RPCRequest) (*rpcpb.RpcRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("rpc: nil request")
	}
	method, err := ProtoMethod(req.Method)
	if err != nil {
		return nil, err
	}
	var payload []byte
	if req.Params != nil {
		payload = append([]byte(nil), req.Params.union...)
	}
	return &rpcpb.RpcRequest{
		Id:      req.Id,
		Method:  method,
		Payload: payload,
	}, nil
}

// DecodeRPCRequest converts the protobuf wire envelope into the public typed RPC request.
func DecodeRPCRequest(msg *rpcpb.RpcRequest) (*RPCRequest, error) {
	if msg == nil {
		return nil, fmt.Errorf("rpc: nil protobuf request")
	}
	method, err := MethodFromProto(msg.GetMethod())
	if err != nil {
		return nil, err
	}
	var params *RPCRequest_Params
	if len(msg.GetPayload()) > 0 {
		params = &RPCRequest_Params{union: append([]byte(nil), msg.GetPayload()...)}
	}
	return &RPCRequest{
		V:      RPCVersionV1,
		Id:     msg.GetId(),
		Method: method,
		Params: params,
	}, nil
}

// EncodeRPCResponse converts the public typed RPC response into the protobuf wire envelope.
func EncodeRPCResponse(resp *RPCResponse) (*rpcpb.RpcResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("rpc: nil response")
	}
	msg := &rpcpb.RpcResponse{Id: resp.Id}
	switch {
	case resp.Error != nil:
		msg.Body = &rpcpb.RpcResponse_Error{Error: &rpcpb.RpcError{
			Code:    rpcpb.RpcErrorCode(resp.Error.Code),
			Message: resp.Error.Message,
		}}
	case resp.Result != nil:
		msg.Body = &rpcpb.RpcResponse_Payload{Payload: append([]byte(nil), resp.Result.union...)}
	}
	return msg, nil
}

// DecodeRPCResponse converts the protobuf wire envelope into the public typed RPC response.
func DecodeRPCResponse(msg *rpcpb.RpcResponse) (*RPCResponse, error) {
	if msg == nil {
		return nil, fmt.Errorf("rpc: nil protobuf response")
	}
	resp := &RPCResponse{
		V:  RPCVersionV1,
		Id: msg.GetId(),
	}
	if rpcErr := msg.GetError(); rpcErr != nil {
		resp.Error = &RPCError{
			Code:    RPCErrorCode(rpcErr.GetCode()),
			Message: rpcErr.GetMessage(),
		}
		return resp, nil
	}
	if payload := msg.GetPayload(); payload != nil {
		resp.Result = &RPCResponse_Result{union: append([]byte(nil), payload...)}
	}
	return resp, nil
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

// Error is a structured RPC error that can be returned as a Go error or encoded
// into an RPC response envelope.
type Error struct {
	RequestID string
	Code      RPCErrorCode
	Message   string
}

// Error returns the RPC error message.
func (e Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("rpc error %d", e.Code)
	}
	return e.Message
}

// RPCResponse converts the error to an RPC response envelope.
func (e Error) RPCResponse() *RPCResponse {
	message := e.Message
	if message == "" {
		message = e.Error()
	}
	return &RPCResponse{
		V:  RPCVersionV1,
		Id: e.RequestID,
		Error: &RPCError{
			Code:    e.Code,
			Message: message,
		},
	}
}
