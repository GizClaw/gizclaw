package gizcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

// Protocol sink only: package extraction and publication are tested in GizOS.
type archiveSink struct {
	bytes   int
	chunks  int
	finish  bool
	closed  bool
	outcome error
	wait    bool
	entered chan struct{}
}

func (s *archiveSink) Write(_ context.Context, data []byte) error {
	s.bytes += len(data)
	s.chunks++
	return nil
}
func (s *archiveSink) Finish(ctx context.Context) (*rpcpb.ClientLuaAppInstallResponse, error) {
	if s.wait {
		close(s.entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	s.finish = true
	return &rpcpb.ClientLuaAppInstallResponse{App: &rpcpb.LuaAppInfo{AppId: "demo", Version: "1.0.0"}}, nil
}
func (s *archiveSink) Close(err error) { s.closed = true; s.outcome = err }

func TestLuaAppBinaryLifecycle(t *testing.T) {
	archive := bytes.Repeat([]byte{0xaa}, 524288) // 512 KiB boundary, protocol data only.
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	for _, tc := range []struct {
		name        string
		body        []byte
		sha         string
		unsupported bool
		ok          bool
	}{
		{"large", archive, digest, false, true},
		{"truncated", archive[:len(archive)-1], digest, false, false},
		{"overlong", append(bytes.Clone(archive), 0), digest, false, false},
		{"wrong_sha", archive, hex.EncodeToString(make([]byte, 32)), false, false},
		{"unsupported", archive, digest, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &archiveSink{}
			device := &Client{}
			var calls atomic.Int32
			if !tc.unsupported {
				_ = device.HandleDeviceControl(DeviceControlHandlers{InstallLuaApp: func(context.Context, *rpcpb.ClientLuaAppInstallStreamRequest) (LuaAppInstallSession, error) {
					calls.Add(1)
					return sink, nil
				}})
			}
			caller, receiver := net.Pipe()
			done := make(chan error, 1)
			go func() { defer receiver.Close(); done <- (&rpcClient{peer: device}).Handle(receiver) }()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			result, err := rpcapi.UploadLuaApp(ctx, caller, &rpcpb.ClientLuaAppInstallStreamRequest{ContentLength: uint32(len(archive)), Sha256: tc.sha}, io.NopCloser(bytes.NewReader(tc.body)))
			<-done
			if tc.ok {
				if err != nil {
					t.Fatal(err)
				}
				if result.GetApp().GetAppId() != "demo" || !sink.finish || sink.chunks < 3 || sink.bytes != len(archive) {
					t.Fatalf("result=%v sink=%+v", result, sink)
				}
			} else if err == nil || sink.finish {
				t.Fatalf("accepted invalid stream: %v %+v", err, sink)
			}
			if tc.unsupported {
				var rpcErr rpcapi.Error
				if !errors.As(err, &rpcErr) || rpcErr.Code != rpcapi.StatusCodeUnimplemented {
					t.Fatalf("unsupported error = %v", err)
				}
				if calls.Load() != 0 {
					t.Fatal("unexpected begin")
				}
			} else if !sink.closed || calls.Load() != 1 {
				t.Fatalf("lifecycle %+v calls=%d", sink, calls.Load())
			}
		})
	}
}

func TestLuaAppBinaryCancelDuringFinish(t *testing.T) {
	archive := []byte("archive bytes")
	sum := sha256.Sum256(archive)
	sink := &archiveSink{wait: true, entered: make(chan struct{})}
	device := &Client{}
	_ = device.HandleDeviceControl(DeviceControlHandlers{InstallLuaApp: func(context.Context, *rpcpb.ClientLuaAppInstallStreamRequest) (LuaAppInstallSession, error) {
		return sink, nil
	}})
	caller, receiver := net.Pipe()
	done := make(chan error, 1)
	go func() { defer receiver.Close(); done <- (&rpcClient{peer: device}).Handle(receiver) }()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	outcome := make(chan error, 1)
	go func() {
		_, err := rpcapi.UploadLuaApp(ctx, caller, &rpcpb.ClientLuaAppInstallStreamRequest{ContentLength: uint32(len(archive)), Sha256: hex.EncodeToString(sum[:])}, io.NopCloser(bytes.NewReader(archive)))
		outcome <- err
	}()
	<-sink.entered
	cancel()
	if err := <-outcome; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	<-done
	if !sink.closed || sink.outcome == nil || sink.finish {
		t.Fatalf("abort lifecycle %+v", sink)
	}
}
