//go:build giznet_e2e

package webrtc_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

func BenchmarkWebRTCHTTPRoundTrip(b *testing.B) {
	for _, size := range []int{128, 1024, 4096} {
		b.Run("size="+itoa(size), func(b *testing.B) {
			serverKey := mustKeyPair(b)
			clientKey := mustKeyPair(b)
			server := startWebRTCServer(b, serverKey, gizwebrtc.CipherModePlaintext)
			defer server.Close()

			clientListener, clientConn := dialWebRTC(b, clientKey, serverKey.Public, server.signalingURL, gizwebrtc.CipherModePlaintext)
			defer clientListener.Close()
			defer clientConn.Close()

			serverConn := acceptConn(b, server.listener)
			defer serverConn.Close()

			srv := gizhttp.NewServer(serverConn, 7, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				defer r.Body.Close()
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write(body)
			}))
			go func() {
				_ = srv.Serve()
			}()
			b.Cleanup(func() {
				_ = srv.Shutdown(context.Background())
			})

			client := gizhttp.NewClient(clientConn, 7)
			payload := bytes.Repeat([]byte("a"), size)
			b.SetBytes(int64(len(payload) * 2))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://gizclaw/echo", bytes.NewReader(payload))
				if err != nil {
					b.Fatal(err)
				}
				resp, err := client.Do(req)
				if err != nil {
					b.Fatal(err)
				}
				got, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					b.Fatal(err)
				}
				if len(got) != len(payload) {
					b.Fatalf("response len=%d want=%d", len(got), len(payload))
				}
			}
		})
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
