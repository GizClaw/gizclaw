package gizclaw

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"golang.org/x/sync/errgroup"
)

const (
	rpcSpeedTestFrameSize        = 32 * 1024
	maxRPCSpeedTestContentLength = int64(1 << 30)
)

// SpeedTestResult is measured locally by the caller while one RPC stream sends
// upload frames and receives download frames concurrently. Duration is the
// whole-call wall time; UpDuration and DownDuration measure only their
// respective transfer direction.
type SpeedTestResult struct {
	UpContentLength   int64
	DownContentLength int64
	UpBytes           int64
	DownBytes         int64
	UpDuration        time.Duration
	DownDuration      time.Duration
	Duration          time.Duration
}

func (r SpeedTestResult) UpMbps() float64 {
	duration := r.UpDuration
	if duration == 0 {
		duration = r.Duration
	}
	return mbps(r.UpBytes, duration)
}

func (r SpeedTestResult) DownMbps() float64 {
	duration := r.DownDuration
	if duration == 0 {
		duration = r.Duration
	}
	return mbps(r.DownBytes, duration)
}

func mbps(bytes int64, duration time.Duration) float64 {
	if bytes <= 0 || duration <= 0 {
		return 0
	}
	return float64(bytes*8) / duration.Seconds() / 1_000_000
}

func callRPCSpeedTest(ctx context.Context, conn net.Conn, id string, request rpcapi.SpeedTestRequest) (SpeedTestResult, error) {
	if err := validateSpeedTestRequest(request); err != nil {
		return SpeedTestResult{}, err
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromSpeedTestRequest)
	if err != nil {
		return SpeedTestResult{}, err
	}
	g, groupCtx := errgroup.WithContext(ctx)
	stream, err := newRPCStream(groupCtx, conn)
	if err != nil {
		return SpeedTestResult{}, err
	}
	defer stream.Close()

	if err := stream.WriteRequest(newRPCRequest(id, rpcapi.RPCMethodAllSpeedTestRun, params)); err != nil {
		return SpeedTestResult{}, err
	}

	start := time.Now()
	var upBytes, downBytes int64
	var upStarted, downStarted time.Time
	var responseErr error
	g.Go(func() error {
		upStarted = time.Now()
		n, err := writeBinaryFrames(stream, request.UpContentLength)
		upBytes = n
		return err
	})
	g.Go(func() error {
		stopUpload := func(err error) error {
			if err != nil {
				responseErr = err
				_ = stream.conn.SetDeadline(time.Now())
			}
			return err
		}
		resp, err := stream.ReadResponseForMethod(rpcapi.RPCMethodAllSpeedTestRun)
		if err != nil {
			return stopUpload(err)
		}
		if resp.Error != nil {
			if err := stream.ReadEOS(); err != nil {
				return stopUpload(err)
			}
			return stopUpload(fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message}))
		}
		if resp.Result == nil {
			return stopUpload(errRPCMissingResult)
		}
		ack, err := resp.Result.AsSpeedTestResponse()
		if err != nil {
			return stopUpload(wrapRPCResultError("speed test", err))
		}
		if ack.UpContentLength != request.UpContentLength || ack.DownContentLength != request.DownContentLength {
			return stopUpload(fmt.Errorf("rpc: speed test ack mismatch"))
		}
		downStarted = time.Now()
		n, err := readBinaryFrames(stream)
		downBytes = n
		return stopUpload(err)
	})
	if err := g.Wait(); err != nil {
		if responseErr != nil {
			return SpeedTestResult{}, responseErr
		}
		return SpeedTestResult{}, err
	}
	completed := time.Now()
	var upDuration, downDuration time.Duration
	if request.UpContentLength > 0 {
		// The Server does not send its EOS until it has consumed the upload.
		// Measuring through that completion barrier avoids reporting the local
		// DataChannel send buffer as path throughput.
		upDuration = completed.Sub(upStarted)
	}
	if request.DownContentLength > 0 {
		downDuration = completed.Sub(downStarted)
	}
	return SpeedTestResult{
		UpContentLength:   request.UpContentLength,
		DownContentLength: request.DownContentLength,
		UpBytes:           upBytes,
		DownBytes:         downBytes,
		UpDuration:        upDuration,
		DownDuration:      downDuration,
		Duration:          completed.Sub(start),
	}, nil
}

func (s *rpcServer) handleSpeedTest(ctx context.Context, stream *rpcStream, req *rpcapi.RPCRequest) error {
	if req.Params == nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInvalidArgument, "missing params")
	}
	params, err := req.Params.AsSpeedTestRequest()
	if err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInvalidArgument, "invalid params")
	}
	if err := validateSpeedTestRequest(params); err != nil {
		return writeRPCErrorResponse(stream, req.Id, rpcapi.StatusCodeInvalidArgument, err.Error())
	}

	result, err := newRPCResultResponse(req.Id, rpcapi.SpeedTestResponse{
		UpContentLength:   params.UpContentLength,
		DownContentLength: params.DownContentLength,
	}, (*rpcapi.RPCPayload).FromSpeedTestResponse)
	if err != nil {
		return err
	}
	metadataEOS, err := stream.WriteResponseEnvelopeForMethod(req.Method, result)
	if err != nil {
		return err
	}
	if metadataEOS {
		if err := stream.WriteEOS(); err != nil {
			return err
		}
	}

	var g errgroup.Group
	cancelStream := func(err error) error {
		if err != nil {
			_ = stream.conn.SetDeadline(time.Now())
		}
		return err
	}
	g.Go(func() error {
		n, err := readBinaryFrames(stream)
		if err != nil {
			return cancelStream(err)
		}
		if n != params.UpContentLength {
			return cancelStream(fmt.Errorf("rpc: speed test upload length mismatch: got %d want %d", n, params.UpContentLength))
		}
		return nil
	})
	g.Go(func() error {
		_, err := writeBinaryFramePayload(stream, params.DownContentLength)
		return cancelStream(err)
	})
	if err := g.Wait(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// EOS is also the upload-consumed acknowledgement. Sending it only after
	// both directions finish keeps upload-only measurements tied to remote
	// consumption rather than local DataChannel buffering.
	return stream.WriteEOS()
}

func validateSpeedTestRequest(request rpcapi.SpeedTestRequest) error {
	if request.UpContentLength < 0 {
		return fmt.Errorf("up_content_length must be non-negative")
	}
	if request.DownContentLength < 0 {
		return fmt.Errorf("down_content_length must be non-negative")
	}
	if request.UpContentLength > maxRPCSpeedTestContentLength {
		return fmt.Errorf("up_content_length exceeds %d", maxRPCSpeedTestContentLength)
	}
	if request.DownContentLength > maxRPCSpeedTestContentLength {
		return fmt.Errorf("down_content_length exceeds %d", maxRPCSpeedTestContentLength)
	}
	return nil
}

func writeRPCErrorResponse(stream *rpcStream, id string, code rpcapi.StatusCode, message string) error {
	return writeRPCStatusResponse(stream, id, &rpcapi.RPCStatus{Code: code, Message: message})
}

// writeRPCStatusResponse forwards a whole status, so a reason set by the
// resolver survives the stream. Reducing the status to a code and a message
// silently dropped it.
func writeRPCStatusResponse(stream *rpcStream, id string, status *rpcapi.RPCStatus) error {
	if status == nil {
		return writeRPCErrorResponse(stream, id, rpcapi.StatusCodeInternal, "missing RPC status")
	}
	response := rpcapi.Error{
		RequestID: id, Code: status.Code, Message: status.Message, Reason: status.Reason,
	}.RPCResponse()
	if _, err := stream.WriteResponseEnvelope(response); err != nil {
		return err
	}
	return stream.WriteEOS()
}

func writeBinaryFrames(stream *rpcStream, total int64) (int64, error) {
	written, err := writeBinaryFramePayload(stream, total)
	if err != nil {
		return written, err
	}
	if err := stream.WriteEOS(); err != nil {
		return written, err
	}
	return written, nil
}

func writeBinaryFramePayload(stream *rpcStream, total int64) (int64, error) {
	chunk := make([]byte, rpcSpeedTestFrameSize)
	for i := range chunk {
		chunk[i] = byte(i)
	}
	var written int64
	for written < total {
		size := int64(len(chunk))
		if remaining := total - written; remaining < size {
			size = remaining
		}
		if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: chunk[:size]}); err != nil {
			return written, err
		}
		written += size
	}
	return written, nil
}

func readBinaryFrames(stream *rpcStream) (int64, error) {
	payload := make([]byte, rpcSpeedTestFrameSize)
	var read int64
	for {
		frame, err := stream.ReadFrameInto(payload)
		if err != nil {
			return read, err
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			return read, nil
		}
		if frame.Type != rpcapi.FrameTypeBinary {
			return read, fmt.Errorf("rpc: expected binary frame, got type %d", frame.Type)
		}
		read += int64(len(frame.Payload))
	}
}
