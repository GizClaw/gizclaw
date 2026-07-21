package gizcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"iter"
	"net"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const rpcMaxEnvelopeSize = rpcapi.MaxFrameSize * 16

type rpcStream struct {
	ctx  context.Context
	conn net.Conn

	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

type rpcRequest struct {
	*rpcStream
	Envelope *rpcapi.RPCRequest
}

type rpcResponse struct {
	*rpcStream
	Envelope *rpcapi.RPCResponse
}

func newRPCStream(ctx context.Context, conn net.Conn) (*rpcStream, error) {
	if conn == nil {
		return nil, errors.New("rpc: nil conn")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s := &rpcStream{
		ctx:  ctx,
		conn: conn,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}

	go func() {
		defer close(s.done)
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-s.stop:
		}
	}()

	if err := ctx.Err(); err != nil {
		_ = conn.SetDeadline(time.Now())
		return s, nil
	}
	return s, nil
}

func (s *rpcStream) Context() context.Context {
	if s == nil || s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *rpcStream) Close() error {
	if s == nil {
		return nil
	}
	s.stopOnce.Do(func() {
		close(s.stop)
		<-s.done
		_ = s.conn.SetDeadline(time.Time{})
	})
	return nil
}

func (s *rpcStream) WriteEOS() error {
	return s.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeEOS})
}

func (s *rpcStream) ReadEOS() error {
	frame, err := s.ReadFrame()
	if err != nil {
		return err
	}
	if frame.Type != rpcapi.FrameTypeEOS {
		return errors.New("rpc: expected EOS frame")
	}
	return nil
}

func (s *rpcStream) ReadFrame() (rpcapi.Frame, error) {
	if err := s.Context().Err(); err != nil {
		return rpcapi.Frame{}, err
	}
	frame, err := rpcapi.ReadFrame(s.conn)
	if err != nil {
		return rpcapi.Frame{}, s.normalizeIOError(err)
	}
	return frame, nil
}

func (s *rpcStream) WriteFrame(frame rpcapi.Frame) error {
	if err := s.Context().Err(); err != nil {
		return err
	}
	if err := rpcapi.WriteFrame(s.conn, frame); err != nil {
		return s.normalizeIOError(err)
	}
	return nil
}

func (s *rpcStream) Frames() iter.Seq2[rpcapi.Frame, error] {
	return func(yield func(rpcapi.Frame, error) bool) {
		for {
			frame, err := s.ReadFrame()
			if err != nil {
				yield(rpcapi.Frame{}, err)
				return
			}
			if frame.Type == rpcapi.FrameTypeEOS {
				return
			}
			if !yield(frame, nil) {
				return
			}
		}
	}
}

func (s *rpcStream) WriteFrames(frames iter.Seq2[rpcapi.Frame, error]) error {
	for frame, err := range frames {
		if err != nil {
			return err
		}
		if err := s.WriteFrame(frame); err != nil {
			return err
		}
	}
	return nil
}

func (s *rpcStream) ReadRequest() (*rpcapi.RPCRequest, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, err
	}
	return rpcapi.DecodeRequestFrame(frame)
}

func (s *rpcStream) ReadRequestEnvelope() (*rpcapi.RPCRequest, bool, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, false, err
	}
	req, consumedEOS, err := s.decodeRequestEnvelope(frame)
	if err != nil {
		return nil, consumedEOS, err
	}
	return req, consumedEOS, nil
}

func (s *rpcStream) WriteRequest(req *rpcapi.RPCRequest) error {
	frame, err := rpcapi.NewRequestFrame(req)
	if err != nil {
		return err
	}
	return s.WriteFrame(frame)
}

func (s *rpcStream) WriteRequestEnvelope(req *rpcapi.RPCRequest) error {
	_, err := s.writeRequestEnvelope(req)
	return err
}

func (s *rpcStream) writeRequestEnvelope(req *rpcapi.RPCRequest) (bool, error) {
	frame, err := rpcapi.NewRequestFrame(req)
	if err != nil {
		return false, err
	}
	return s.writeProtobufEnvelope(frame.Payload)
}

func (s *rpcStream) ReadResponse() (*rpcapi.RPCResponse, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, err
	}
	return rpcapi.DecodeResponseFrame(frame)
}

func (s *rpcStream) ReadResponseForMethod(method rpcapi.RPCMethod) (*rpcapi.RPCResponse, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, err
	}
	return rpcapi.DecodeResponseFrameForMethod(method, frame)
}

func (s *rpcStream) ReadResponseEnvelope() (*rpcapi.RPCResponse, bool, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, false, err
	}
	resp, consumedEOS, err := s.decodeResponseEnvelope(frame)
	if err != nil {
		return nil, consumedEOS, err
	}
	return resp, consumedEOS, nil
}

func (s *rpcStream) ReadResponseEnvelopeForMethod(method rpcapi.RPCMethod) (*rpcapi.RPCResponse, bool, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, false, err
	}
	resp, consumedEOS, err := s.decodeResponseEnvelopeForMethod(method, frame)
	if err != nil {
		return nil, consumedEOS, err
	}
	return resp, consumedEOS, nil
}

func (s *rpcStream) WriteResponse(resp *rpcapi.RPCResponse) error {
	frame, err := rpcapi.NewResponseFrame(resp)
	if err != nil {
		return err
	}
	return s.WriteFrame(frame)
}

func (s *rpcStream) WriteResponseForMethod(method rpcapi.RPCMethod, resp *rpcapi.RPCResponse) error {
	frame, err := rpcapi.NewResponseFrameForMethod(method, resp)
	if err != nil {
		return err
	}
	return s.WriteFrame(frame)
}

func (s *rpcStream) WriteResponseEnvelope(resp *rpcapi.RPCResponse) (bool, error) {
	frame, err := rpcapi.NewResponseFrame(resp)
	if err != nil {
		return false, err
	}
	return s.writeProtobufEnvelope(frame.Payload)
}

func (s *rpcStream) WriteResponseEnvelopeForMethod(method rpcapi.RPCMethod, resp *rpcapi.RPCResponse) (bool, error) {
	frame, err := rpcapi.NewResponseFrameForMethod(method, resp)
	if err != nil {
		return false, err
	}
	return s.writeProtobufEnvelope(frame.Payload)
}

func (s *rpcStream) Responses() iter.Seq2[*rpcapi.RPCResponse, error] {
	return func(yield func(*rpcapi.RPCResponse, error) bool) {
		for frame, err := range s.Frames() {
			if err != nil {
				yield(nil, err)
				return
			}
			resp, err := rpcapi.DecodeResponseFrame(frame)
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(resp, nil) {
				return
			}
		}
	}
}

func (s *rpcStream) writeProtobufEnvelope(data []byte) (bool, error) {
	if len(data) <= rpcapi.MaxFrameSize {
		return false, s.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: data})
	}
	for len(data) > 0 {
		n := min(len(data), rpcapi.MaxFrameSize)
		if err := s.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeText, Payload: data[:n]}); err != nil {
			return false, err
		}
		data = data[n:]
	}
	return true, nil
}

func (s *rpcStream) decodeRequestEnvelope(first rpcapi.Frame) (*rpcapi.RPCRequest, bool, error) {
	switch first.Type {
	case rpcapi.FrameTypeBinary:
		req, err := rpcapi.DecodeRequestFrame(first)
		return req, false, err
	case rpcapi.FrameTypeText:
		payload, err := s.readProtobufEnvelopeContinuation(first)
		if err != nil {
			return nil, false, err
		}
		req, err := rpcapi.DecodeRequestFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload})
		return req, true, err
	default:
		return nil, false, fmt.Errorf("rpc: expected protobuf binary frame, got type %d", first.Type)
	}
}

func (s *rpcStream) decodeResponseEnvelope(first rpcapi.Frame) (*rpcapi.RPCResponse, bool, error) {
	switch first.Type {
	case rpcapi.FrameTypeBinary:
		resp, err := rpcapi.DecodeResponseFrame(first)
		return resp, false, err
	case rpcapi.FrameTypeText:
		payload, err := s.readProtobufEnvelopeContinuation(first)
		if err != nil {
			return nil, false, err
		}
		resp, err := rpcapi.DecodeResponseFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload})
		return resp, true, err
	default:
		return nil, false, fmt.Errorf("rpc: expected protobuf binary frame, got type %d", first.Type)
	}
}

func (s *rpcStream) decodeResponseEnvelopeForMethod(method rpcapi.RPCMethod, first rpcapi.Frame) (*rpcapi.RPCResponse, bool, error) {
	switch first.Type {
	case rpcapi.FrameTypeBinary:
		resp, err := rpcapi.DecodeResponseFrameForMethod(method, first)
		return resp, false, err
	case rpcapi.FrameTypeText:
		payload, err := s.readProtobufEnvelopeContinuation(first)
		if err != nil {
			return nil, false, err
		}
		resp, err := rpcapi.DecodeResponseFrameForMethod(method, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload})
		return resp, true, err
	default:
		return nil, false, fmt.Errorf("rpc: expected protobuf binary frame, got type %d", first.Type)
	}
}

func (s *rpcStream) readProtobufEnvelopeContinuation(first rpcapi.Frame) ([]byte, error) {
	if len(first.Payload) > rpcMaxEnvelopeSize {
		return nil, fmt.Errorf("rpc: protobuf envelope too large: %d", len(first.Payload))
	}
	var buf bytes.Buffer
	buf.Write(first.Payload)
	for {
		frame, err := s.ReadFrame()
		if err != nil {
			return nil, err
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			return buf.Bytes(), nil
		}
		if frame.Type != rpcapi.FrameTypeText {
			return nil, fmt.Errorf("rpc: expected protobuf continuation frame, got type %d", frame.Type)
		}
		if buf.Len()+len(frame.Payload) > rpcMaxEnvelopeSize {
			return nil, fmt.Errorf("rpc: protobuf envelope too large: %d", buf.Len()+len(frame.Payload))
		}
		buf.Write(frame.Payload)
	}
}

func (s *rpcStream) normalizeIOError(err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := s.Context().Err(); ctxErr != nil {
		return ctxErr
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		if deadline, ok := s.Context().Deadline(); ok && !time.Now().Before(deadline) {
			return context.DeadlineExceeded
		}
	}
	return err
}
