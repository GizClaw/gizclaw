package gizcli

import (
	"context"
	"fmt"
	"mime"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/opusmedia"
)

// The gate owns one input send operation, including its codec and transport
// writes. Close first cancels transport I/O; the last admitted sender releases
// native state. No internal state mutex is held across codec or network calls.
func (s *PeerStream) initSendGate() {
	s.sendOnce.Do(func() { s.sendGate = make(chan struct{}, 1) })
}

func (s *PeerStream) enterSend(ctx context.Context) error {
	s.initSendGate()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return s.closeErr()
	case s.sendGate <- struct{}{}:
		return nil
	}
}

func (s *PeerStream) leaveSend() {
	<-s.sendGate
	select {
	case <-s.done:
		s.closeIdleSender()
	default:
	}
}

func (s *PeerStream) closePCMInput() {
	if s.pcmInput != nil {
		_ = s.pcmInput.Close()
		s.pcmInput = nil
	}
}

func (s *PeerStream) closeIdleSender() {
	s.initSendGate()
	select {
	case s.sendGate <- struct{}{}:
		s.closePCMInput()
		<-s.sendGate
	default:
		// The active sender's deferred leaveSend owns cleanup after I/O exits.
	}
}

func peerInputPCM(blob *genx.Blob) (bool, error) {
	if blob == nil {
		return false, nil
	}
	base, params, err := mime.ParseMediaType(blob.MIMEType)
	if err != nil || !strings.EqualFold(base, "audio/pcm") {
		return false, nil
	}
	if rate := params["rate"]; rate != "" && rate != "16000" {
		return false, fmt.Errorf("gizclaw: PCM input requires 16000 Hz, got %q", rate)
	}
	if channels := params["channels"]; channels != "" && channels != "1" {
		return false, fmt.Errorf("gizclaw: PCM input requires mono, got %q", channels)
	}
	if bits := params["bits"]; bits != "" && bits != "16" {
		return false, fmt.Errorf("gizclaw: PCM input requires 16 bits, got %q", bits)
	}
	if format := strings.ToLower(params["format"]); format != "" && format != "s16le" && format != "pcm_s16le" {
		return false, fmt.Errorf("gizclaw: PCM input requires s16le format, got %q", params["format"])
	}
	return true, nil
}

func (s *PeerStream) sendInputAudio(ctx context.Context, chunk *genx.MessageChunk, blob *genx.Blob, pcm bool) error {
	if chunk.Part == nil && chunk.IsEndOfStream() && s.sendPCM && s.pcmInput != nil {
		s.inputReadyMu.Lock()
		ready := s.inputReady
		s.inputReadyMu.Unlock()
		if ready != nil && chunk.Ctrl.StreamID == ready.streamID {
			// A whole-route EOS also ends the active PCM channel. Keep it on
			// the normal validated send path so its tail precedes wire EOS.
			blob, pcm = &genx.Blob{}, true
		}
	}
	if blob == nil || !pcm && !isOpusBlob(blob) {
		return nil
	}
	if len(blob.Data) == 0 && (!pcm || s.pcmInput == nil || !chunk.IsEndOfStream()) {
		return nil
	}
	if s.conn == nil {
		return fmt.Errorf("gizclaw: peer stream is not connected")
	}
	s.inputReadyMu.Lock()
	ready := s.inputReady
	s.inputReadyMu.Unlock()
	if ready == nil {
		return fmt.Errorf("gizclaw: audio input requires an acknowledged BOS")
	}
	if chunk.Ctrl == nil || chunk.Ctrl.StreamID != ready.streamID {
		return fmt.Errorf("gizclaw: audio chunk does not belong to input stream %q", ready.streamID)
	}
	if err := s.waitAudioInput(ctx, ready); err != nil {
		return err
	}
	if pcm != s.sendPCM {
		return fmt.Errorf("gizclaw: audio input format cannot change within one epoch")
	}
	packets := [][]byte{blob.Data}
	if pcm {
		if s.pcmInput == nil {
			var err error
			s.pcmInput, err = opusmedia.NewPCMEncoder(16000, 1)
			if err != nil {
				return err
			}
		}
		var err error
		packets, err = s.pcmInput.Push(blob.Data)
		if err != nil {
			return err
		}
		if chunk.IsEndOfStream() {
			tail, err := s.pcmInput.Flush()
			if err != nil {
				return err
			}
			packets = append(packets, tail...)
		}
	} else if s.pcmInput != nil {
		return fmt.Errorf("gizclaw: cannot mix encoded Opus and PCM in one input epoch")
	}
	for _, packet := range packets {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			return s.closeErr()
		default:
		}
		if _, err := s.conn.Write(giznet.ProtocolOpusPacket, packet); err != nil {
			return err
		}
		select {
		case <-s.done:
			return s.closeErr()
		default:
		}
	}
	return nil
}
