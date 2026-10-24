package doubaoasr

import (
	"errors"
	"io"
	"testing"
)

func TestRecoveringSessionCompletedSendWinsSessionClose(t *testing.T) {
	rejected := errors.New("provider rejected the final send")
	for _, tc := range []struct {
		name   string
		result error
	}{{name: "success"}, {name: "provider-error", result: rejected}} {
		t.Run(tc.name, func(t *testing.T) {
			for attempt := range 128 {
				session := &recoveringSession{sends: make(chan asrSend), done: make(chan struct{}), err: tc.result}
				workerDone := make(chan struct{})
				go func() {
					defer close(workerDone)
					request := <-session.sends
					request.done <- tc.result
					close(session.done)
				}()
				err := session.SendAudio(t.Context(), []byte{1, 0}, true)
				<-workerDone
				if !errors.Is(err, tc.result) {
					t.Fatalf("accepted final send attempt %d returned %v, want %v", attempt, err, tc.result)
				}
			}
		})
	}
}

func TestRecoveringSessionSendAfterCompletionReturnsClosedPipe(t *testing.T) {
	session := &recoveringSession{sends: make(chan asrSend), done: make(chan struct{})}
	close(session.done)
	if err := session.SendAudio(t.Context(), []byte{1, 0}, true); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("send to completed session=%v, want closed pipe", err)
	}
}
