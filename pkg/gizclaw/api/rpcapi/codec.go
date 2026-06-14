package rpcapi

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net"
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

// WriteRequest writes an RPC request as one JSON frame.
func WriteRequest(w io.Writer, req *RPCRequest) error {
	frame, err := NewJSONFrame(req)
	if err != nil {
		return err
	}
	return WriteFrame(w, frame)
}

// ReadRequest reads an RPC request from one JSON frame.
func ReadRequest(r io.Reader) (*RPCRequest, error) {
	frame, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	var req RPCRequest
	if err := DecodeJSONFrame(frame, &req); err != nil {
		return nil, fmt.Errorf("rpc: unmarshal request: %w", err)
	}
	return &req, nil
}

// ReadResponse reads an RPC response from one JSON frame.
func ReadResponse(r io.Reader) (*RPCResponse, error) {
	frame, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	var resp RPCResponse
	if err := DecodeJSONFrame(frame, &resp); err != nil {
		return nil, fmt.Errorf("rpc: unmarshal response: %w", err)
	}
	return &resp, nil
}

// WriteResponse writes an RPC response as one JSON frame.
func WriteResponse(w io.Writer, resp *RPCResponse) error {
	frame, err := NewJSONFrame(resp)
	if err != nil {
		return err
	}
	return WriteFrame(w, frame)
}

// ReadResponses reads JSON RPC response frames until EOF.
func ReadResponses(r io.Reader) iter.Seq2[*RPCResponse, error] {
	return func(yield func(*RPCResponse, error) bool) {
		for frame, err := range ReadFrames(r) {
			if err != nil {
				yield(nil, err)
				return
			}
			var resp RPCResponse
			if err := DecodeJSONFrame(frame, &resp); err != nil {
				yield(nil, fmt.Errorf("rpc: unmarshal response: %w", err))
				return
			}
			if !yield(&resp, nil) {
				return
			}
		}
	}
}

// WriteResponses writes each RPC response as a JSON frame.
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
