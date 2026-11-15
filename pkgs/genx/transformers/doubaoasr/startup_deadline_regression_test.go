package doubaoasr

import (
	"context"
	"fmt"
	"io"
	"testing"
	"testing/synctest"
	"time"
)

func TestOpeningRecoveryDeadlineEndsWhenReplacementOpens(t *testing.T) {
	for _, continuous := range []bool{false, true} {
		t.Run(fmt.Sprint("continuous=", continuous), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				provider := newRecoveryProvider()
				var opens int
				session, err := newRecoveringSession(ctx, doubaoASRSessionConfig{format: "pcm", sampleRate: 16000, channels: 1, bits: 16}, newDoubaoASRRouteState(nil), continuous, time.Second,
					func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
						opens++
						if opens == 1 {
							return nil, fmt.Errorf("opening provider: %w", io.EOF)
						}
						return provider, nil
					})
				if err != nil {
					t.Fatal(err)
				}
				defer session.Close()
				// A healthy open need not produce speech or a definite result.
				time.Sleep(2 * time.Second)
				if err := session.SendAudio(ctx, []byte{1, 2}, false); err != nil {
					t.Fatalf("quiet replacement retained its opening deadline: %v", err)
				}
				if sent := awaitRecovery(t, provider.sent); len(sent.data) != 2 {
					t.Fatal("healthy replacement did not accept source audio")
				}
				// Speech may continue beyond the former recovery deadline as well.
				time.Sleep(2 * time.Second)
				if err := session.SendAudio(ctx, []byte{3, 4}, false); err != nil {
					t.Fatalf("active replacement still waited for a definite result: %v", err)
				}
				if opens != 2 {
					t.Fatalf("provider opens=%d, want exactly one startup retry", opens)
				}
			})
		})
	}
}
