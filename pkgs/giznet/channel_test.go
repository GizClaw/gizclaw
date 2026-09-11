package giznet

import "testing"

func TestChannelReliabilityString(t *testing.T) {
	var zero ChannelReliability
	for reliability, want := range map[ChannelReliability]string{
		zero:                  "unknown",
		ChannelReliable:       "reliable",
		ChannelUnreliable:     "unreliable",
		ChannelReliability(9): "unknown",
	} {
		if got := reliability.String(); got != want {
			t.Fatalf("ChannelReliability(%d).String() = %q, want %q", reliability, got, want)
		}
	}
	if zero != ChannelReliabilityUnknown {
		t.Fatal("zero ChannelReliability is not unknown")
	}
}
