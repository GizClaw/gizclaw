package gizlog

import (
	"errors"
	"log"
	"log/slog"
	"sync"
	"testing"
	"time"

	lksdk "github.com/livekit/server-sdk-go/v2"
)

// The real SDK logs connection-state changes while holding its own mutex.
// InstallDefault also routes standard log through this queue, under log.outMu.
func TestFullStoreQueueDoesNotBlockLiveKitDisconnect(t *testing.T) {
	room := lksdk.NewRoom(nil)
	store := &gatedLogStore{entered: make(chan struct{}), release: make(chan struct{})}
	cleanup, err := InstallDefault(Config{Sinks: []SinkConfig{{Kind: SinkStore, Store: "logs"}}}, gatedLogResolver{store})
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	unblock := func() { once.Do(func() { close(store.release) }) }
	t.Cleanup(func() { unblock(); _ = cleanup() })
	slog.Info("gate worker")
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter Store")
	}
	for range storeQueueCapacity {
		slog.Info("fill queue")
	}
	done := make(chan struct{})
	go func() { room.Disconnect(); log.Print("unrelated standard log caller"); close(done) }()
	select {
	case <-done:
		unblock()
		if err := cleanup(); !errors.Is(err, ErrStoreQueueFull) {
			t.Fatalf("cleanup did not report rejected SDK logs: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		unblock()
		<-done
		t.Fatal("LiveKit Disconnect or standard log waited for Store capacity")
	}
}
