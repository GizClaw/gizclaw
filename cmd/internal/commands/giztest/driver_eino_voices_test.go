package giztestcmd

import "testing"

func TestEinoMultiVoiceGiztest(t *testing.T) {
	for _, fault := range []string{"", "wrong-voice", "stall", "overlap"} {
		name := fault
		if name == "" {
			name = "success"
		}
		t.Run(name, func(t *testing.T) { runVoiceGiztest(t, "eino", "eino-voices", fault) })
	}
}
