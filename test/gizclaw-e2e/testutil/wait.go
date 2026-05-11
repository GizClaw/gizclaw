package testutil

import (
	"fmt"
	"net"
	"testing"
	"time"
)

const (
	ReadyTimeout = 30 * time.Second
	ProbeTimeout = time.Second
	PollInterval = 20 * time.Millisecond
)

func AllocateUDPAddr(t testing.TB) string {
	t.Helper()
	for range 20 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("allocateUDPAddr tcp: %v", err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		pc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = pc.Close()
			_ = l.Close()
			return fmt.Sprintf("127.0.0.1:%d", port)
		}
		_ = l.Close()
	}
	t.Fatalf("allocateUDPAddr: could not find a TCP/UDP-free port")
	return ""
}

func WaitUntil(timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := check(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(PollInterval)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("condition not satisfied before timeout")
}
