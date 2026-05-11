package scenarios

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkStreamAggregateThroughput benchmarks aggregate throughput over
// multiple long-lived streams.
func BenchmarkStreamAggregateThroughput(b *testing.B, payload []byte, parallel int, open StreamOpenFunc, accept StreamAcceptFunc) {
	b.Helper()
	if parallel <= 0 {
		b.Fatalf("parallel must be > 0")
	}

	clientStreams := make([]net.Conn, 0, parallel)
	serverStreams := make([]net.Conn, 0, parallel)
	for i := range parallel {
		c, err := open()
		if err != nil {
			b.Fatalf("open stream[%d]: %v", i, err)
		}
		s, err := accept()
		if err != nil {
			_ = c.Close()
			b.Fatalf("accept stream[%d]: %v", i, err)
		}
		clientStreams = append(clientStreams, c)
		serverStreams = append(serverStreams, s)
	}
	defer func() {
		for _, s := range clientStreams {
			_ = s.Close()
		}
		for _, s := range serverStreams {
			_ = s.Close()
		}
	}()

	var recvOps atomic.Int64
	errCh := make(chan error, parallel)
	var wg sync.WaitGroup

	for i := range parallel {
		wg.Add(1)
		go func(s net.Conn) {
			defer wg.Done()
			buf := make([]byte, len(payload))
			for {
				if _, err := io.ReadFull(s, buf); err != nil {
					if err == io.EOF || err == io.ErrUnexpectedEOF {
						return
					}
					errCh <- err
					return
				}
				recvOps.Add(1)
			}
		}(serverStreams[i])
	}

	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := clientStreams[i%parallel]
		if _, err := s.Write(payload); err != nil {
			b.Fatalf("write[%d]: %v", i, err)
		}
	}

	deadline := time.Now().Add(20 * time.Second)
	for recvOps.Load() < int64(b.N) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := recvOps.Load(); got < int64(b.N) {
		b.Fatalf("delivered ops=%d, want >= %d", got, b.N)
	}
	b.StopTimer()

	for _, s := range clientStreams {
		_ = s.Close()
	}
	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		b.Fatalf("receiver error: %v", err)
	}
}

// BenchmarkRPCStyleMultiplexing benchmarks request/response traffic over
// multiple streams (RPC-like workload).
func BenchmarkRPCStyleMultiplexing(b *testing.B, req []byte, parallel int, open StreamOpenFunc, accept StreamAcceptFunc) {
	b.Helper()
	if parallel <= 0 {
		b.Fatalf("parallel must be > 0")
	}

	clientStreams := make([]net.Conn, 0, parallel)
	serverStreams := make([]net.Conn, 0, parallel)
	for i := range parallel {
		c, err := open()
		if err != nil {
			b.Fatalf("open stream[%d]: %v", i, err)
		}
		s, err := accept()
		if err != nil {
			_ = c.Close()
			b.Fatalf("accept stream[%d]: %v", i, err)
		}
		clientStreams = append(clientStreams, c)
		serverStreams = append(serverStreams, s)
	}
	defer func() {
		for _, s := range clientStreams {
			_ = s.Close()
		}
		for _, s := range serverStreams {
			_ = s.Close()
		}
	}()

	errCh := make(chan error, parallel)
	var wg sync.WaitGroup
	for i := range parallel {
		wg.Add(1)
		go func(idx int, s net.Conn) {
			defer wg.Done()
			buf := make([]byte, len(req))
			for {
				if _, err := io.ReadFull(s, buf); err != nil {
					if err == io.EOF || err == io.ErrUnexpectedEOF {
						return
					}
					errCh <- fmt.Errorf("server stream[%d] read: %w", idx, err)
					return
				}
				if _, err := s.Write(buf); err != nil {
					errCh <- fmt.Errorf("server stream[%d] write: %w", idx, err)
					return
				}
			}
		}(i, serverStreams[i])
	}

	resp := make([]byte, len(req))
	b.SetBytes(int64(len(req) * 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := clientStreams[i%parallel]
		if _, err := s.Write(req); err != nil {
			b.Fatalf("client write[%d]: %v", i, err)
		}
		if _, err := io.ReadFull(s, resp); err != nil {
			b.Fatalf("client read[%d]: %v", i, err)
		}
	}
	b.StopTimer()

	for _, s := range clientStreams {
		_ = s.Close()
	}
	wg.Wait()
	close(errCh)
	if err := <-errCh; err != nil {
		b.Fatal(err)
	}
}
