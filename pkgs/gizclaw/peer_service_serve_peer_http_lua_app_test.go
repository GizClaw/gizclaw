package gizclaw

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

type luaUploadConn struct {
	testGiznetConn
	serve func(net.Conn)
}

func (c *luaUploadConn) Dial(uint64) (net.Conn, error) {
	caller, device := net.Pipe()
	go func() { defer device.Close(); c.serve(device) }()
	return caller, nil
}

func TestLuaAppHTTPForwardsBeforeUploadCompletes(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	data := bytes.Repeat([]byte{0x78, 0x9c, 0xff}, 54828)
	digest := sha256.Sum256(data)
	first := make(chan struct{})
	receipt := make(chan []byte, 1)
	f.manager.SetPeerUp(f.owner, &luaUploadConn{serve: func(conn net.Conn) {
		req, err := rpcapi.ReadRequest(conn)
		if err != nil {
			receipt <- nil
			return
		}
		if req.Method != rpcapi.RPCMethodClientLuaAppInstall {
			receipt <- nil
			return
		}
		var got []byte
		for {
			frame, err := rpcapi.ReadFrame(conn)
			if err != nil {
				receipt <- nil
				return
			}
			if frame.Type == rpcapi.FrameTypeEOS {
				break
			}
			if len(got) == 0 {
				close(first)
			}
			if frame.Type != rpcapi.FrameTypeBinary {
				receipt <- nil
				return
			}
			got = append(got, frame.Payload...)
		}
		receipt <- got
		payload := new(rpcapi.RPCPayload)
		_ = payload.FromClientLuaAppInstallResponse(&rpcpb.ClientLuaAppInstallResponse{App: &rpcpb.LuaAppInfo{AppId: "demo", Version: "1.0.0"}})
		_ = rpcapi.WriteResponseForMethod(conn, req.Method, &rpcapi.RPCResponse{Id: req.Id, Result: payload})
		_ = rpcapi.WriteEOS(conn)
	}})
	body, producer := io.Pipe()
	defer body.Close()
	defer producer.Close()
	request := httptest.NewRequest(http.MethodPost, "/gizclaw/v1/device/lua-app/install?content_length=164484&sha256="+hex.EncodeToString(digest[:]), body)
	request.Header.Set("Authorization", "Bearer "+f.secret)
	request.Header.Set("Content-Type", "application/octet-stream")
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { f.handler.ServeHTTP(response, request); close(done) }()
	if _, err := producer.Write(data[:32768]); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	case <-ctx.Done():
		t.Fatal("Server buffered the upload before forwarding the first chunk")
	}
	if _, err := producer.Write(data[32768:]); err != nil {
		t.Fatal(err)
	}
	_ = producer.Close()
	<-done
	if response.Code != 200 {
		t.Fatalf("%d %s", response.Code, response.Body)
	}
	if !bytes.Equal(<-receipt, data) {
		t.Fatal("forwarded bytes differ")
	}
}

func TestLuaAppHTTPFinalErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		code   rpcapi.StatusCode
		status int
		want   string
	}{
		{rpcapi.StatusCodeUnimplemented, 501, deviceUnsupportedCode},
		{rpcapi.StatusCodeInvalidArgument, 400, deviceRejectedCode},
		{rpcapi.StatusCodeDeadlineExceeded, 504, deviceTimeoutCode},
		{rpcapi.StatusCodeInternal, 502, deviceErrorCode},
	} {
		t.Run(tc.want, func(t *testing.T) {
			f := newDeviceHTTPFixture(t)
			f.manager.SetPeerUp(f.owner, &luaUploadConn{serve: func(conn net.Conn) {
				req, err := rpcapi.ReadRequest(conn)
				if err != nil {
					return
				}
				_ = rpcapi.WriteResponseForMethod(conn, req.Method, rpcapi.Error{RequestID: req.Id, Code: tc.code, Message: "private installer detail"}.RPCResponse())
				_ = rpcapi.WriteEOS(conn)
			}})
			body, producer := io.Pipe()
			defer producer.Close()
			request := httptest.NewRequest("POST", "/gizclaw/v1/device/lua-app/install?content_length=100000&sha256="+hex.EncodeToString(make([]byte, 32)), body)
			request.Header.Set("Authorization", "Bearer "+f.secret)
			request.Header.Set("Content-Type", "application/octet-stream")
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			response := httptest.NewRecorder()
			f.handler.ServeHTTP(response, request.WithContext(ctx))
			if response.Code != tc.status || errorCode(t, response) != tc.want {
				t.Fatalf("%d %s", response.Code, response.Body)
			}
			if bytes.Contains(response.Body.Bytes(), []byte("private installer detail")) {
				t.Fatal("internal detail leaked")
			}
		})
	}
}
