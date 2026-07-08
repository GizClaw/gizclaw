package gizcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

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
	var req rpcapi.RPCRequest
	if err := rpcapi.DecodeJSONFrame(frame, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

func (s *rpcStream) ReadRequestEnvelope() (*rpcapi.RPCRequest, bool, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, false, err
	}
	var req rpcapi.RPCRequest
	consumedEOS, err := s.decodeJSONEnvelope(frame, &req)
	if err != nil {
		return nil, consumedEOS, err
	}
	return &req, consumedEOS, nil
}

func (s *rpcStream) WriteRequest(req *rpcapi.RPCRequest) error {
	frame, err := rpcapi.NewJSONFrame(req)
	if err != nil {
		return err
	}
	return s.WriteFrame(frame)
}

func (s *rpcStream) WriteRequestEnvelope(req *rpcapi.RPCRequest) error {
	return s.writeJSONEnvelope(req)
}

func (s *rpcStream) ReadResponse() (*rpcapi.RPCResponse, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, err
	}
	var resp rpcapi.RPCResponse
	if err := rpcapi.DecodeJSONFrame(frame, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *rpcStream) ReadResponseEnvelope() (*rpcapi.RPCResponse, bool, error) {
	frame, err := s.ReadFrame()
	if err != nil {
		return nil, false, err
	}
	var resp rpcapi.RPCResponse
	consumedEOS, err := s.decodeJSONEnvelope(frame, &resp)
	if err != nil {
		return nil, consumedEOS, err
	}
	return &resp, consumedEOS, nil
}

func (s *rpcStream) WriteResponse(resp *rpcapi.RPCResponse) error {
	frame, err := rpcapi.NewJSONFrame(resp)
	if err != nil {
		return err
	}
	return s.WriteFrame(frame)
}

func (s *rpcStream) WriteResponseEnvelope(resp *rpcapi.RPCResponse) error {
	return s.writeJSONEnvelope(resp)
}

func (s *rpcStream) Responses() iter.Seq2[*rpcapi.RPCResponse, error] {
	return func(yield func(*rpcapi.RPCResponse, error) bool) {
		for frame, err := range s.Frames() {
			if err != nil {
				yield(nil, err)
				return
			}
			var resp rpcapi.RPCResponse
			if err := rpcapi.DecodeJSONFrame(frame, &resp); err != nil {
				yield(nil, err)
				return
			}
			if !yield(&resp, nil) {
				return
			}
		}
	}
}

func (s *rpcStream) writeJSONEnvelope(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(data) <= rpcapi.MaxFrameSize {
		return s.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeJSON, Payload: data})
	}
	for len(data) > 0 {
		n := min(len(data), rpcapi.MaxFrameSize)
		if err := s.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeText, Payload: data[:n]}); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func (s *rpcStream) decodeJSONEnvelope(first rpcapi.Frame, v any) (bool, error) {
	switch first.Type {
	case rpcapi.FrameTypeJSON:
		return false, json.Unmarshal(first.Payload, v)
	case rpcapi.FrameTypeText:
		var buf bytes.Buffer
		buf.Write(first.Payload)
		for {
			frame, err := s.ReadFrame()
			if err != nil {
				return false, err
			}
			if frame.Type == rpcapi.FrameTypeEOS {
				if err := json.Unmarshal(buf.Bytes(), v); err != nil {
					return true, err
				}
				return true, nil
			}
			if frame.Type != rpcapi.FrameTypeText {
				return false, fmt.Errorf("rpc: expected JSON continuation frame, got type %d", frame.Type)
			}
			buf.Write(frame.Payload)
		}
	default:
		return false, fmt.Errorf("rpc: expected JSON frame, got type %d", first.Type)
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
