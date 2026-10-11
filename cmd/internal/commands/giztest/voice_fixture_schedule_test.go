package giztestcmd

import (
	"testing"
	"time"
)

func TestVoiceFixtureSchedulingPauseDoesNotShiftMediaTimeline(t *testing.T) {
	for _, pause := range []time.Duration{0, 220 * time.Millisecond, 900 * time.Millisecond} {
		t.Run(pause.String(), func(t *testing.T) {
			start := time.Unix(1, 0)
			now := start
			schedule := voiceFixturePacketSchedule{}
			periods := []time.Duration{15 * time.Millisecond, 25 * time.Millisecond, 20 * time.Millisecond}
			caughtUp := 0
			for packet := range 160 {
				if packet == 40 {
					now = now.Add(pause)
				}
				delay := schedule.waitDuration(now, periods[packet%len(periods)])
				if packet >= 40 && delay == 0 {
					caughtUp++
				}
				now = now.Add(delay)
			}
			// 53 complete jitter cycles plus the final 15ms frame: 3.195s.
			if elapsed := now.Sub(start); elapsed != 3195*time.Millisecond {
				t.Fatalf("pause %s shifted the 160-packet media timeline to %s", pause, elapsed)
			}
			if pause > 0 && caughtUp == 0 {
				t.Fatal("missed frames were not produced after the scheduling pause")
			}
		})
	}
}
