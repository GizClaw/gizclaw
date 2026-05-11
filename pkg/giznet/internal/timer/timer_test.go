package timer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTimerResetFiresLatestDeadline(t *testing.T) {
	fired := make(chan struct{}, 1)
	tm := New(func() {
		fired <- struct{}{}
	})
	defer tm.Close()

	if !tm.Reset(time.Now().Add(time.Hour)) {
		t.Fatal("Reset() returned false before Close")
	}
	if !tm.Reset(time.Now().Add(10 * time.Millisecond)) {
		t.Fatal("Reset() returned false before Close")
	}

	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("timer did not fire")
	}
}

func TestTimerResetZeroDisarms(t *testing.T) {
	var calls atomic.Int64
	tm := New(func() {
		calls.Add(1)
	})
	defer tm.Close()

	if !tm.Reset(time.Now().Add(10 * time.Millisecond)) {
		t.Fatal("Reset() returned false before Close")
	}
	if !tm.Reset(time.Time{}) {
		t.Fatal("Reset(zero) returned false before Close")
	}

	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("timer fired after Reset(zero), calls=%d", got)
	}
}

func TestTimerConcurrentResetAndClose(t *testing.T) {
	var calls atomic.Int64
	tm := New(func() {
		calls.Add(1)
	})

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				tm.Reset(time.Now().Add(time.Millisecond))
			}
		}()
	}
	wg.Wait()

	tm.Close()
	tm.Close()

	if tm.Reset(time.Now()) {
		t.Fatal("Reset() returned true after Close")
	}
	_ = calls.Load()
}

func TestTimerResetRacesWithClose(t *testing.T) {
	var calls atomic.Int64
	tm := New(func() {
		calls.Add(1)
	})

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 1_000 {
				tm.Reset(time.Now().Add(time.Millisecond))
			}
		}()
	}

	closeDone := make(chan struct{})
	go func() {
		<-start
		tm.Close()
		close(closeDone)
	}()

	close(start)

	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close() blocked during concurrent Reset() calls")
	}

	wg.Wait()
	tm.Close()

	if tm.Reset(time.Now()) {
		t.Fatal("Reset() returned true after racing Close")
	}
	_ = calls.Load()
}

func TestTimerCloseDoesNotWaitForCallback(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	tm := New(func() {
		close(started)
		<-release
	})

	if !tm.Reset(time.Now()) {
		t.Fatal("Reset() returned false before Close")
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timer callback did not start")
	}

	closed := make(chan struct{})
	go func() {
		tm.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close() waited for callback")
	}

	close(release)
	tm.Close()
}
