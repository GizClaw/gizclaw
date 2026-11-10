package gizcli

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
)

func peerAudioOutputMode() opusmedia.OutputMode {
	if !opus.IsRuntimeSupported() {
		return opusmedia.OutputEncoded
	}
	return opusmedia.OutputPCM
}

// The physical event reader fences its own connection's receiver before
// publishing EOS. Local subscribers only drain their independently owned queues;
// their progress must never reset the decoder or loss clock for newer media.
func (s *peerEventSession) flushOutputAudio(event *eventpb.PeerEvent) error {
	if event.Type != eventpb.PeerEventType_PEER_EVENT_TYPE_EOS || event.StreamKindValue() != eventpb.StreamKind_STREAM_KIND_AUDIO ||
		strings.TrimSpace(event.Label()) == "user" || !isOpusBlob(peerStreamEventBlobPart(event)) {
		return nil
	}
	s.mu.Lock()
	active := !s.closed && s.audioStartID != "" && s.audioStartID == event.StreamID()
	s.mu.Unlock()
	if !active {
		return nil
	}
	reader := s.mediaReader.Load()
	if reader == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Flush dispatches only media already owned by this receiver. It neither
	// waits for local Next calls nor reads future packets during a grace period.
	err := reader.Flush(ctx)
	if errors.Is(err, io.ErrClosedPipe) {
		return nil
	}
	return err
}
