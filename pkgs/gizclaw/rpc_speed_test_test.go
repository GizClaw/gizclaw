package gizclaw

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRPCSpeedTest(t *testing.T) {
	for _, tc := range []struct {
		name       string
		upLength   int64
		downLength int64
	}{
		{name: "binary payloads", upLength: 96 * 1024, downLength: 128 * 1024},
		{name: "zero length payloads", upLength: 0, downLength: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientSide, serverSide := net.Pipe()
			defer clientSide.Close()
			defer serverSide.Close()

			serverErr := make(chan error, 1)
			go func() {
				serverErr <- (&rpcServer{}).Handle(serverSide)
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result, err := callRPCSpeedTest(ctx, clientSide, "speed", rpcapi.SpeedTestRequest{
				UpContentLength:   tc.upLength,
				DownContentLength: tc.downLength,
			})
			if err != nil {
				t.Fatalf("callRPCSpeedTest error = %v", err)
			}
			if result.UpBytes != result.UpContentLength {
				t.Fatalf("UpBytes = %d, want %d", result.UpBytes, result.UpContentLength)
			}
			if result.DownBytes != result.DownContentLength {
				t.Fatalf("DownBytes = %d, want %d", result.DownBytes, result.DownContentLength)
			}
			if result.Duration <= 0 {
				t.Fatalf("Duration = %v, want positive", result.Duration)
			}
			if tc.upLength > 0 && result.UpDuration <= 0 {
				t.Fatalf("UpDuration = %v, want positive", result.UpDuration)
			}
			if tc.downLength > 0 && result.DownDuration <= 0 {
				t.Fatalf("DownDuration = %v, want positive", result.DownDuration)
			}
			if tc.upLength == 0 && result.UpDuration != 0 {
				t.Fatalf("UpDuration = %v, want zero", result.UpDuration)
			}
			if tc.downLength == 0 && result.DownDuration != 0 {
				t.Fatalf("DownDuration = %v, want zero", result.DownDuration)
			}
			if err := clientSide.Close(); err != nil {
				t.Fatalf("client close error = %v", err)
			}
			if err := <-serverErr; err != nil {
				t.Fatalf("server Handle error = %v", err)
			}
		})
	}
}

func TestRPCSpeedTestServerEOSAcknowledgesConsumedUpload(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- (&rpcServer{}).Handle(serverSide)
	}()
	stream, err := newRPCStream(context.Background(), clientSide)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	params, err := newRPCRequestParams(
		rpcapi.SpeedTestRequest{UpContentLength: 1},
		(*rpcapi.RPCPayload).FromSpeedTestRequest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.WriteRequest(newRPCRequest("speed-eos", rpcapi.RPCMethodAllSpeedTestRun, params)); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.ReadResponseForMethod(rpcapi.RPCMethodAllSpeedTestRun); err != nil {
		t.Fatal(err)
	}

	if err := clientSide.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if frame, err := stream.ReadFrame(); err == nil {
		t.Fatalf("server sent frame %v before consuming upload", frame.Type)
	}
	if err := clientSide.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}

	if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := stream.WriteEOS(); err != nil {
		t.Fatal(err)
	}
	if err := stream.ReadEOS(); err != nil {
		t.Fatal(err)
	}
	if err := clientSide.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server Handle error = %v", err)
	}
}

func TestRPCSpeedTestRejectsInvalidLength(t *testing.T) {
	_, err := callRPCSpeedTest(context.Background(), nil, "speed", rpcapi.SpeedTestRequest{
		UpContentLength: -1,
	})
	if err == nil {
		t.Fatal("callRPCSpeedTest should reject invalid length")
	}
}

func TestRPCSpeedTestStopsUploadWhenServerReturnsError(t *testing.T) {
	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	serverErr := make(chan error, 1)
	go func() {
		stream, err := newRPCStream(context.Background(), serverSide)
		if err != nil {
			serverErr <- err
			return
		}
		defer stream.Close()
		req, err := stream.ReadRequest()
		if err != nil {
			serverErr <- err
			return
		}
		if err := writeRPCErrorResponse(stream, req.Id, rpcapi.RPCErrorCodeInvalidParams, "server rejected speed test"); err != nil {
			serverErr <- err
			return
		}
		serverErr <- serverSide.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := callRPCSpeedTest(ctx, clientSide, "speed", rpcapi.SpeedTestRequest{
		UpContentLength:   1024 * 1024,
		DownContentLength: 1024,
	})
	if err == nil {
		t.Fatal("callRPCSpeedTest should return server error")
	}
	if !strings.Contains(err.Error(), "server rejected speed test") {
		t.Fatalf("callRPCSpeedTest error = %v, want server rejection", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error = %v", err)
	}
}
