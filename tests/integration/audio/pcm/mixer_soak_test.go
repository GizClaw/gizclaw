//go:build manual

package pcm_integration_test

import (
	"errors"
	"io"
	"runtime"
	"testing"
	"time"

	audiopcm "github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
)

func TestMixerSoakRepeatedBlockedReadCloseNoStuckReaders(t *testing.T) {
	const (
		rounds          = 300
		readersPerRound = 4
	)

	baseG := runtime.NumGoroutine()

	for i := 0; i < rounds; i++ {
		mx := audiopcm.NewMixer(audiopcm.L16Mono16K)
		errCh := make(chan error, readersPerRound)

		for j := 0; j < readersPerRound; j++ {
			go func() {
				_, err := mx.Read(make([]byte, 320))
				errCh <- err
			}()
		}

		time.Sleep(1 * time.Millisecond)
		if err := mx.CloseWrite(); err != nil {
			t.Fatalf("round=%d CloseWrite() error: %v", i, err)
		}

		deadline := time.After(2 * time.Second)
		for j := 0; j < readersPerRound; j++ {
			select {
			case err := <-errCh:
				if !errors.Is(err, io.EOF) {
					t.Fatalf("round=%d reader=%d err=%v, want EOF", i, j, err)
				}
			case <-deadline:
				t.Fatalf("round=%d reader=%d not released (possible leak)", i, j)
			}
		}
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	afterG := runtime.NumGoroutine()
	if afterG > baseG+32 {
		t.Fatalf("goroutine count grew too much: base=%d after=%d", baseG, afterG)
	}
}
