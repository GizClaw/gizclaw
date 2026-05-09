package rpcapi

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

// MaxFrameSize is the largest RPC frame payload accepted by ReadFrame.
const MaxFrameSize = 1 << 20 // 1 MiB

// WriteFrame writes a length-prefixed RPC frame.
func WriteFrame(w io.Writer, data []byte) error {
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(data)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// ReadFrame reads a length-prefixed RPC frame.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(hdr[:])
	if length > MaxFrameSize {
		return nil, fmt.Errorf("rpc: frame too large: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// WriteRequest writes an RPC request as one JSON frame.
func WriteRequest(w io.Writer, req *RPCRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return WriteFrame(w, data)
}

// ReadRequest reads an RPC request from one JSON frame.
func ReadRequest(r io.Reader) (*RPCRequest, error) {
	data, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	var req RPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("rpc: unmarshal request: %w", err)
	}
	return &req, nil
}

// ReadResponse reads an RPC response from one JSON frame.
func ReadResponse(r io.Reader) (*RPCResponse, error) {
	data, err := ReadFrame(r)
	if err != nil {
		return nil, err
	}
	var resp RPCResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("rpc: unmarshal response: %w", err)
	}
	return &resp, nil
}

// WriteResponse writes an RPC response as one JSON frame.
func WriteResponse(w io.Writer, resp *RPCResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return WriteFrame(w, data)
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
