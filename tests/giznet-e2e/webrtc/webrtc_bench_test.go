//go:build giznet_e2e

package webrtc_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
)

const throughputBytesPerStream = 16 * 1024 * 1024

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

func BenchmarkWebRTCServiceThroughput(b *testing.B) {
	for _, tc := range []struct {
		name        string
		connections int
		streams     int
	}{
		{name: "one_peer_connection/one_data_channel", connections: 1, streams: 1},
		{name: "three_peer_connections", connections: 3, streams: 3},
		{name: "ten_peer_connections", connections: 10, streams: 10},
		{name: "one_peer_connection/three_data_channels", connections: 1, streams: 3},
		{name: "one_peer_connection/ten_data_channels", connections: 1, streams: 10},
	} {
		b.Run(tc.name, func(b *testing.B) {
			streams := openThroughputStreams(b, tc.connections, tc.streams)
			payload := bytes.Repeat([]byte("giznet-throughput"), throughputBytesPerStream/len("giznet-throughput")+1)
			payload = payload[:throughputBytesPerStream]

			b.SetBytes(int64(len(payload) * len(streams)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				observation, err := transferConcurrently(streams, payload)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(observation.minMbps, "min-client-Mbps")
				b.ReportMetric(observation.maxMbps, "max-client-Mbps")
			}
		})
	}
}

type throughputStream struct {
	client io.ReadWriteCloser
	server io.ReadWriteCloser
}

type throughputObservation struct {
	minMbps float64
	maxMbps float64
}

func openThroughputStreams(tb testing.TB, connections, streamCount int) []throughputStream {
	tb.Helper()
	serverKey := mustKeyPair(tb)
	server := startWebRTCServer(tb, serverKey, gizwebrtc.CipherModePlaintext)
	tb.Cleanup(server.Close)

	clientConns := make([]giznet.Conn, connections)
	serverConns := make([]giznet.Conn, connections)
	for i := range connections {
		clientKey := mustKeyPair(tb)
		clientListener, clientConn := dialWebRTC(tb, clientKey, serverKey.Public, server.signalingURL, gizwebrtc.CipherModePlaintext)
		tb.Cleanup(func() {
			_ = clientConn.Close()
			_ = clientListener.Close()
		})
		clientConns[i] = clientConn
		serverConns[i] = acceptConn(tb, server.listener)
		tb.Cleanup(func() { _ = serverConns[i].Close() })
	}

	streams := make([]throughputStream, streamCount)
	for i := range streamCount {
		connIndex := i % connections
		service := serverConns[connIndex].ListenService(echoService)
		serverStreamCh := make(chan struct {
			stream io.ReadWriteCloser
			err    error
		}, 1)
		go func() {
			stream, err := service.Accept()
			serverStreamCh <- struct {
				stream io.ReadWriteCloser
				err    error
			}{stream: stream, err: err}
		}()
		clientStream, err := clientConns[connIndex].Dial(echoService)
		if err != nil {
			tb.Fatalf("Dial throughput stream %d: %v", i, err)
		}
		var serverStream io.ReadWriteCloser
		select {
		case accepted := <-serverStreamCh:
			if accepted.err != nil {
				tb.Fatalf("Accept throughput stream %d: %v", i, accepted.err)
			}
			serverStream = accepted.stream
		case <-time.After(5 * time.Second):
			tb.Fatalf("Accept throughput stream %d timed out", i)
		}
		tb.Cleanup(func() {
			_ = clientStream.Close()
			_ = serverStream.Close()
		})
		streams[i] = throughputStream{client: clientStream, server: serverStream}
	}
	return streams
}

func transferConcurrently(streams []throughputStream, payload []byte) (throughputObservation, error) {
	var wg sync.WaitGroup
	errCh := make(chan error, len(streams)*2)
	start := make(chan struct{})
	durations := make([]time.Duration, len(streams))
	for i, stream := range streams {
		wg.Go(func() {
			<-start
			started := time.Now()
			if _, err := io.CopyN(io.Discard, stream.server, int64(len(payload))); err != nil {
				errCh <- fmt.Errorf("stream %d read: %w", i, err)
			}
			durations[i] = time.Since(started)
		})
		wg.Go(func() {
			<-start
			if _, err := io.Copy(stream.client, bytes.NewReader(payload)); err != nil {
				errCh <- fmt.Errorf("stream %d write: %w", i, err)
			}
		})
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		return throughputObservation{}, err
	}
	observation := throughputObservation{}
	for i, duration := range durations {
		mbps := float64(len(payload)*8) / duration.Seconds() / 1_000_000
		if i == 0 || mbps < observation.minMbps {
			observation.minMbps = mbps
		}
		if mbps > observation.maxMbps {
			observation.maxMbps = mbps
		}
	}
	return observation, nil
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
