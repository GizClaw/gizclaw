package gizcli

import (
	"context"
	"time"
)

// The control fence waits for packet-pump dispatch. Continue consuming that
// dispatch while waiting, so an EOS cannot make its own subscriber look slow.
func (s *PeerStream) flushAudioOutput() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- s.flushAudio(ctx) }()
	audio := s.audio
	for {
		select {
		case err := <-finished:
			// The receiver owns the deadline result. Successful dispatch stays
			// successful even if delivering its frames to Next takes longer.
			return err
		case <-s.done:
			return s.closeErr()
		case blob, ok := <-audio:
			if !ok {
				audio = nil
				continue
			}
			if err := s.pushMergedAudio(blob); err != nil {
				return err
			}
		}
	}
}
