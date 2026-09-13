package doubaorealtime

import (
	"errors"
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPTTQueueConcurrentProgress(t *testing.T) {
	for _, outcome := range []string{"success", "cancel", "output-error"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				testPTTQueueLifecycle(t, outcome, true)
			})
		})
	}
}

func TestPTTQueueSameResourceSerialControl(t *testing.T) {
	for _, outcome := range []string{"success", "cancel", "output-error"} {
		t.Run(outcome, func(t *testing.T) {
			testPTTQueueLifecycle(t, outcome, false)
		})
	}
}

// Both schedules reuse one queue and one input turn for every round. Only the
// receiver mutates response terminal flags, matching the production ownership.
// The concurrent schedule keeps that receiver in flight while the input binds
// the next response, then releases add and finish from the same barrier.
func testPTTQueueLifecycle(t *testing.T, outcome string, concurrent bool) {
	t.Helper()
	var queue doubaoRealtimePTTResponses
	var turn doubaoRealtimePTTTurn
	output := &recordingRealtimeOutput{}
	bind := func(index int, identity doubaoRealtimePTTResponseIdentity) (*doubaoRealtimePTTResponse, error) {
		turn.begin(output, fmt.Sprintf("turn-%d", index), doubaoRealtimeAssistantLabel, 0, 0)
		if err := turn.commitText(); err != nil {
			return nil, err
		}
		response := turn.bindResponseFor(turn.currentGeneration(), uint64(index+1), identity)
		if response == nil {
			return nil, errors.New("text response was not bound")
		}
		return response, nil
	}
	old, err := bind(0, doubaoRealtimePTTResponseIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	queue.add(old)
	for index := 1; index <= 32; index++ {
		identity := doubaoRealtimePTTResponseIdentity{replyID: fmt.Sprintf("reply-%d", index)}
		if got := queue.startAudio(identity); got != old {
			t.Fatalf("round %d: audio owner = %p, want %p", index, got, old)
		}
		if outcome != "success" {
			if outcome == "output-error" {
				// Exercise a real output error before the caller discards the turn.
				failure := errors.New("output rejected")
				old.output.output = pttQueueErrorOutput{err: failure}
				if err := old.push(&genx.MessageChunk{Part: genx.Text("answer")}); !errors.Is(err, failure) {
					t.Fatalf("output error = %v, want %v", err, failure)
				}
				if id, active, committed := turn.discardResponse(old); id != old.streamID || !active || !committed {
					t.Fatalf("discardResponse = %q, %t, %t", id, active, committed)
				}
			} else {
				// Turn cancellation discards its output; late provider terminals
				// still drain its queue entry below, before the next reply.
				if id, active, committed := turn.discard(); id != old.streamID || !active || !committed {
					t.Fatalf("discard = %q, %t, %t", id, active, committed)
				}
			}
			select {
			case <-old.completion.done:
			default:
				t.Fatal("discard did not release the response waiter")
			}
		}

		// Receiver observes the old owner before the new, unidentified response
		// is published. Publication must not redirect its untagged binary audio.
		match := func() error {
			if queue.match(identity) != old || queue.matchAudio(doubaoRealtimePTTResponseIdentity{}) != old {
				return errors.New("receiver lost the old text or binary audio owner")
			}
			return nil
		}
		finish := func() error {
			if err := match(); err != nil {
				return err
			}
			old.chatEnded = true
			queue.finish(old)
			if queue.match(identity) != old {
				return errors.New("ChatEnded removed response before TTSFinished")
			}
			old.ttsFinished = true
			queue.finish(old)
			queue.finish(old) // Duplicate terminal must not remove the next round.
			return nil
		}
		var next *doubaoRealtimePTTResponse
		if concurrent {
			matched := make(chan struct{})
			bound := make(chan struct{})
			publishAndFinish := make(chan struct{})
			inputDone := make(chan error, 1)
			receiverDone := make(chan error, 1)
			go func() {
				matchErr := match()
				close(matched)
				<-publishAndFinish
				if matchErr == nil {
					matchErr = finish()
				}
				receiverDone <- matchErr
			}()
			go func() {
				<-matched
				var bindErr error
				next, bindErr = bind(index, doubaoRealtimePTTResponseIdentity{})
				close(bound)
				<-publishAndFinish
				if bindErr == nil {
					queue.add(next)
				}
				inputDone <- bindErr
			}()
			// Wait proves both workers reached the barrier, rather than relying
			// on scheduler luck or a sleep. A deadlock fails the synctest bubble.
			synctest.Wait()
			select {
			case <-bound:
			default:
				t.Fatal("input did not bind while the receiver was in flight")
			}
			close(publishAndFinish)
			if err := <-inputDone; err != nil {
				t.Fatal(err)
			}
			if err := <-receiverDone; err != nil {
				t.Fatal(err)
			}
		} else {
			if err := match(); err != nil {
				t.Fatal(err)
			}
			next, err = bind(index, doubaoRealtimePTTResponseIdentity{})
			if err != nil {
				t.Fatal(err)
			}
			queue.add(next)
			if err := finish(); err != nil {
				t.Fatal(err)
			}
		}
		select {
		case <-old.completion.done:
		default:
			t.Fatal("finished response waiter is blocked")
		}
		if len(queue.items) != 1 || queue.items[0] != next {
			t.Fatalf("round %d: completed response remains in queue: %#v", index, queue.items)
		}
		if queue.matchAudio(doubaoRealtimePTTResponseIdentity{}) != nil {
			t.Fatal("late untagged audio was assigned to the next input")
		}
		if queue.match(doubaoRealtimePTTResponseIdentity{}) != next || next.streamID != fmt.Sprintf("turn-%d", index) || next.epoch != uint64(index+1) {
			t.Fatal("next response has incorrect input ownership")
		}
		select {
		case <-next.completion.done:
			t.Fatal("old terminal completed the next response")
		default:
		}
		old = next
	}
	old.chatEnded, old.ttsFinished = true, true
	queue.finish(old)
	if len(queue.items) != 0 || queue.match(doubaoRealtimePTTResponseIdentity{}) != nil {
		t.Fatal("queue is not empty after the final response")
	}
}

type pttQueueErrorOutput struct{ err error }

func (o pttQueueErrorOutput) Push(*genx.MessageChunk) error { return o.err }
