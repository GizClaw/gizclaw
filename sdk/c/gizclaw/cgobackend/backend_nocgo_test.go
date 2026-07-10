//go:build !cgo

package cgobackend

import (
	"testing"
	"time"
)

type testEventSink struct{}

func (testEventSink) RemoteChannel(int, string, bool, bool) {}
func (testEventSink) ChannelState(int, int)                 {}
func (testEventSink) ChannelMessage(int, []byte, bool)      {}

func TestCloseWakesBlockedPoll(t *testing.T) {
	backend := New()
	backend.SetEventSink(testEventSink{})
	pollDone := make(chan struct{})
	go func() {
		backend.Poll(2_000)
		close(pollDone)
	}()
	time.Sleep(20 * time.Millisecond)

	closeDone := make(chan struct{})
	go func() {
		backend.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Close blocked behind Poll timeout")
	}
	select {
	case <-pollDone:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Poll was not woken by Close")
	}
}
