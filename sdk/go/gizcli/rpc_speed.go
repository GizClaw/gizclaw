package gizcli

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
// upload frames and receives download frames concurrently.
type SpeedTestResult struct {
	UpContentLength   int64
	DownContentLength int64
	UpBytes           int64
	DownBytes         int64
	Duration          time.Duration
}

func (r SpeedTestResult) UpMbps() float64 {
	return mbps(r.UpBytes, r.Duration)
}

func (r SpeedTestResult) DownMbps() float64 {
	return mbps(r.DownBytes, r.Duration)
}

func mbps(bytes int64, duration time.Duration) float64 {
	if bytes <= 0 || duration <= 0 {
		return 0
	}
	return float64(bytes*8) / duration.Seconds() / 1_000_000
}

func (c *Client) SpeedTest(ctx context.Context, id string, request rpcapi.SpeedTestRequest) (SpeedTestResult, error) {
	stream, err := c.rpcConn()
	if err != nil {
		return SpeedTestResult{}, err
	}
	defer func() { _ = stream.Close() }()
	return callRPCSpeedTest(ctx, stream, id, request)
}

func callRPCSpeedTest(ctx context.Context, conn net.Conn, id string, request rpcapi.SpeedTestRequest) (SpeedTestResult, error) {
	if err := validateSpeedTestRequest(request); err != nil {
		return SpeedTestResult{}, err
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCRequest_Params).FromSpeedTestRequest)
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
	var responseErr error
	g.Go(func() error {
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
	return SpeedTestResult{
		UpContentLength:   request.UpContentLength,
		DownContentLength: request.DownContentLength,
		UpBytes:           upBytes,
		DownBytes:         downBytes,
		Duration:          time.Since(start),
	}, nil
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

func writeBinaryFrames(stream *rpcStream, total int64) (int64, error) {
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
	if err := stream.WriteEOS(); err != nil {
		return written, err
	}
	return written, nil
}

func readBinaryFrames(stream *rpcStream) (int64, error) {
	var read int64
	for {
		frame, err := stream.ReadFrame()
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
