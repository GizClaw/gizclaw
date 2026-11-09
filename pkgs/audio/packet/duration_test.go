package packet

import (
	"fmt"
	"testing"
)

func TestOpusDuration48TOCConfigurations(t *testing.T) {
	// RFC 6716 section 3.1: all 32 configuration values, in TOC order.
	frameTicks := [...]uint32{
		480, 960, 1920, 2880, 480, 960, 1920, 2880, 480, 960, 1920, 2880,
		480, 960, 480, 960,
		120, 240, 480, 960, 120, 240, 480, 960, 120, 240, 480, 960, 120, 240, 480, 960,
	}
	for config, ticks := range frameTicks {
		for _, stereo := range []byte{0, 4} {
			for _, tc := range []struct {
				code   byte
				second byte
				frames uint32
			}{
				{code: 0, frames: 1},
				{code: 1, frames: 2},
				{code: 2, frames: 2},
				{code: 3, second: 0xc3, frames: 3},
				{code: 3, second: 48, frames: 48},
				{code: 3, second: 0xff, frames: 63},
			} {
				t.Run(fmt.Sprintf("config%d/stereo%d/code%d/count%d", config, stereo, tc.code, tc.frames), func(t *testing.T) {
					payload := []byte{byte(config<<3) | stereo | tc.code, tc.second}
					if got := OpusDuration48(payload); got != ticks*tc.frames {
						t.Fatalf("OpusDuration48(%x) = %d, want %d", payload, got, ticks*tc.frames)
					}
				})
			}
		}
	}
}

func TestOpusDuration48EmptyAndCountFallback(t *testing.T) {
	for _, payload := range [][]byte{nil, {}} {
		if got := OpusDuration48(payload); got != 0 {
			t.Fatalf("empty payload duration = %d, want 0", got)
		}
	}
	for _, payload := range [][]byte{{0x9b}, {0x9b, 0}, {0x9b, 0xc0}} {
		if got := OpusDuration48(payload); got != 960 {
			t.Fatalf("one-frame compatibility fallback %x = %d, want 960", payload, got)
		}
	}
}
