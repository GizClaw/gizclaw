package gizcli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"golang.org/x/sync/errgroup"
)

func TestSpeedTestResultMbps(t *testing.T) {
	result := SpeedTestResult{
		UpBytes:   1_000_000,
		DownBytes: 2_000_000,
		Duration:  time.Second,
	}
	if got := result.UpMbps(); got != 8 {
		t.Fatalf("UpMbps() = %v, want 8", got)
	}
	if got := result.DownMbps(); got != 16 {
		t.Fatalf("DownMbps() = %v, want 16", got)
	}
	if got := (SpeedTestResult{}).UpMbps(); got != 0 {
		t.Fatalf("zero UpMbps() = %v, want 0", got)
	}
}

func TestValidateSpeedTestRequest(t *testing.T) {
	tests := []struct {
		name    string
		request rpcapi.SpeedTestRequest
		wantErr string
	}{
		{name: "ok", request: rpcapi.SpeedTestRequest{UpContentLength: 1, DownContentLength: 2}},
		{name: "negative up", request: rpcapi.SpeedTestRequest{UpContentLength: -1}, wantErr: "up_content_length must be non-negative"},
		{name: "negative down", request: rpcapi.SpeedTestRequest{DownContentLength: -1}, wantErr: "down_content_length must be non-negative"},
		{name: "too large up", request: rpcapi.SpeedTestRequest{UpContentLength: maxRPCSpeedTestContentLength + 1}, wantErr: "up_content_length exceeds 1073741824"},
		{name: "too large down", request: rpcapi.SpeedTestRequest{DownContentLength: maxRPCSpeedTestContentLength + 1}, wantErr: "down_content_length exceeds 1073741824"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSpeedTestRequest(tc.request)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateSpeedTestRequest() error = %v", err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("validateSpeedTestRequest() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestSpeedTestBinaryFrameHelpers(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverStream, err := newRPCStream(context.Background(), serverSide)
	if err != nil {
		t.Fatalf("newRPCStream(server) error = %v", err)
	}
	defer serverStream.Close()
	clientStream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatalf("newRPCStream(client) error = %v", err)
	}
	defer clientStream.Close()

	writeErrCh := make(chan error, 1)
	go func() {
		written, err := writeBinaryFrames(clientStream, rpcSpeedTestFrameSize+7)
		if err != nil {
			writeErrCh <- err
			return
		}
		if written != rpcSpeedTestFrameSize+7 {
			writeErrCh <- errors.New("unexpected written byte count")
			return
		}
		writeErrCh <- nil
	}()

	read, err := readBinaryFrames(serverStream)
	if err != nil {
		t.Fatalf("readBinaryFrames() error = %v", err)
	}
	if read != rpcSpeedTestFrameSize+7 {
		t.Fatalf("readBinaryFrames() = %d, want %d", read, rpcSpeedTestFrameSize+7)
	}
	if err := <-writeErrCh; err != nil {
		t.Fatalf("writeBinaryFrames() error = %v", err)
	}
}

func TestCallRPCSpeedTestRoundTrip(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	serverErrCh := make(chan error, 1)
	go func() {
		stream, err := newRPCStream(context.Background(), serverSide)
		if err != nil {
			serverErrCh <- err
			return
		}
		defer stream.Close()

		req, err := stream.ReadRequest()
		if err != nil {
			serverErrCh <- err
			return
		}
		if req.Method != rpcapi.RPCMethodAllSpeedTestRun {
			serverErrCh <- fmt.Errorf("method = %s, want %s", req.Method, rpcapi.RPCMethodAllSpeedTestRun)
			return
		}
		params, err := req.Params.AsSpeedTestRequest()
		if err != nil {
			serverErrCh <- err
			return
		}
		resp, err := newRPCResultResponse(req.Id, rpcapi.SpeedTestResponse{
			UpContentLength:   params.UpContentLength,
			DownContentLength: params.DownContentLength,
		}, (*rpcapi.RPCResponse_Result).FromSpeedTestResponse)
		if err != nil {
			serverErrCh <- err
			return
		}
		if err := stream.WriteResponseForMethod(req.Method, resp); err != nil {
			serverErrCh <- err
			return
		}

		var g errgroup.Group
		g.Go(func() error {
			n, err := readBinaryFrames(stream)
			if err != nil {
				return err
			}
			if n != params.UpContentLength {
				return fmt.Errorf("upload bytes = %d, want %d", n, params.UpContentLength)
			}
			return nil
		})
		g.Go(func() error {
			n, err := writeBinaryFrames(stream, params.DownContentLength)
			if err != nil {
				return err
			}
			if n != params.DownContentLength {
				return fmt.Errorf("download bytes = %d, want %d", n, params.DownContentLength)
			}
			return nil
		})
		serverErrCh <- g.Wait()
	}()

	result, err := callRPCSpeedTest(context.Background(), clientSide, "speed", rpcapi.SpeedTestRequest{
		UpContentLength:   777,
		DownContentLength: 999,
	})
	if err != nil {
		t.Fatalf("callRPCSpeedTest() error = %v", err)
	}
	if result.UpBytes != 777 || result.DownBytes != 999 {
		t.Fatalf("callRPCSpeedTest() result = %+v", result)
	}
	if err := <-serverErrCh; err != nil {
		t.Fatalf("server error = %v", err)
	}
}
