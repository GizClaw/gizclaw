package opusmedia

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// RawReader reads one transport packet. It must honor cancellation without
// closing the connection; the packet pump is its only reader.
type RawReader func(context.Context, []byte) (byte, int, *packet.OpusInfo, error)

type readResult struct {
	frame *Frame
	fence chan error
}

type readCommand struct {
	discard bool
	mode    *OutputMode
	fence   chan error
}

// PacketReader drives the media receiver's deadlines at the connection boundary.
// One packet pump owns Read and Close. Control readers may call Flush/Discard.
// No goroutines are started by this type.
type PacketReader struct {
	raw      RawReader
	receiver *Receiver
	scratch  []byte
	ready    []readResult
	terminal error
	commands chan readCommand
	done     chan struct{}
	closed   bool
	mode     atomic.Uint32

	// Only the interrupt handle is shared with the control reader. Decoder,
	// jitter, and output state belong exclusively to the packet pump.
	mu         sync.Mutex
	readCancel context.CancelFunc
}

// NewPacketReader creates an idle media read adapter. Legacy payload-only Opus
// remains encoded; timed packets use the configured output mode and PCM format.
func NewPacketReader(raw RawReader, config Config) (*PacketReader, error) {
	if raw == nil {
		return nil, errors.New("opusmedia: nil raw reader")
	}
	receiver, err := NewReceiver(config)
	if err != nil {
		return nil, err
	}
	reader := &PacketReader{raw: raw, receiver: receiver, commands: make(chan readCommand, 1), done: make(chan struct{})}
	reader.mode.Store(uint32(config.OutputMode))
	return reader, nil
}

// OutputMode reports the format selected for this receiver. It is safe to read
// from a control owner after SetOutputMode completes.
func (r *PacketReader) OutputMode() OutputMode { return OutputMode(r.mode.Load()) }

// SetOutputMode switches reception at a serialized media boundary. A changed
// mode discards pending packets and decoder history before the fence completes.
// The caller must prevent admission of a new route until this call returns.
// Repeating the current mode preserves pending media and decoder history.
func (r *PacketReader) SetOutputMode(ctx context.Context, mode OutputMode) error {
	if mode != OutputPCM && mode != OutputEncoded {
		return errors.New("opusmedia: invalid output mode")
	}
	return r.synchronize(ctx, readCommand{mode: &mode})
}

// Read returns a transport packet, or recovered Opus media in frame. For media,
// dst receives only the original encoded payload; a concealed frame has n=0.
// The caller must finish dispatching each result before its next Read.
func (r *PacketReader) Read(ctx context.Context, dst []byte) (protocol byte, n int, frame *Frame, err error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, 0, nil, err
		}
		// Finish the bounded output batch before accepting another command.
		// Canceled/retried control waits cannot accumulate decoded batches.
		if len(r.ready) > 0 {
			next := r.ready[0]
			r.ready[0] = readResult{}
			r.ready = r.ready[1:]
			if next.fence != nil {
				next.fence <- r.terminal
				continue
			}
			if len(next.frame.Original) > len(dst) {
				return 0, 0, nil, io.ErrShortBuffer
			}
			return giznet.ProtocolOpusPacket, copy(dst, next.frame.Original), next.frame, nil
		}
		select {
		case command := <-r.commands:
			r.apply(command)
			continue
		default:
		}
		if r.terminal != nil {
			return 0, 0, nil, r.terminal
		}
		frames, popErr := r.receiver.Pop(time.Now())
		r.enqueue(frames)
		if popErr != nil {
			r.terminal = popErr
		}
		if len(r.ready) > 0 || r.terminal != nil {
			continue
		}

		var readCtx context.Context
		var cancel context.CancelFunc
		if deadline, ok := r.receiver.Deadline(); ok {
			readCtx, cancel = context.WithDeadline(ctx, deadline)
		} else {
			readCtx, cancel = context.WithCancel(ctx)
		}
		r.mu.Lock()
		r.readCancel = cancel
		r.mu.Unlock()
		// Publish the interrupt before checking commands to avoid a lost wakeup.
		select {
		case command := <-r.commands:
			cancel()
			r.mu.Lock()
			r.readCancel = nil
			r.mu.Unlock()
			r.apply(command)
			continue
		default:
		}
		// Connection owners already provide a full packet buffer. Borrow it for
		// this read; the receiver copies media before retaining or returning it.
		// Small destinations still need full transport storage so a packet is
		// never silently truncated before the caller gets io.ErrShortBuffer.
		readBuffer := dst
		if len(readBuffer) < 64*1024 {
			if r.scratch == nil {
				r.scratch = make([]byte, 64*1024)
			}
			readBuffer = r.scratch
		} else {
			readBuffer = readBuffer[:64*1024]
		}
		protocol, n, info, readErr := r.raw(readCtx, readBuffer)
		interrupted := readCtx.Err() != nil
		r.mu.Lock()
		r.readCancel = nil
		r.mu.Unlock()
		cancel()
		if readErr != nil {
			if ctx.Err() != nil {
				return 0, 0, nil, ctx.Err()
			}
			if interrupted && (errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded)) {
				continue
			}
			frames, flushErr := r.receiver.Flush()
			r.enqueue(frames)
			r.terminal = errors.Join(readErr, flushErr)
			continue
		}
		if protocol == giznet.ProtocolOpusPacket && info != nil {
			frames, decodeErr := r.receiver.Push(readBuffer[:n], info, time.Now())
			r.enqueue(frames)
			if decodeErr != nil {
				r.terminal = decodeErr
			}
			continue
		}
		if n > len(dst) {
			return 0, 0, nil, io.ErrShortBuffer
		}
		return protocol, copy(dst, readBuffer[:n]), nil, nil
	}
}

// Flush releases only media already owned by this receiver, then resets its
// decoder. It does not read future packets or infer an audio epoch from time.
// The fence completes after the packet pump dispatches this bounded output.
func (r *PacketReader) Flush(ctx context.Context) error {
	return r.synchronize(ctx, readCommand{})
}

// Discard drops pending media and resets decoder history, without a drain wait.
// It is used before admitting a new input BOS whose sender waits for readiness.
func (r *PacketReader) Discard(ctx context.Context) error {
	return r.synchronize(ctx, readCommand{discard: true})
}

func (r *PacketReader) synchronize(ctx context.Context, command readCommand) error {
	fence := make(chan error, 1)
	command.fence = fence
	select {
	case <-r.done:
		return io.ErrClosedPipe
	case <-ctx.Done():
		return ctx.Err()
	case r.commands <- command:
	}
	r.mu.Lock()
	cancel := r.readCancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case err := <-fence:
		return err
	case <-r.done:
		return io.ErrClosedPipe
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close is called by the packet pump after its last Read. It releases the
// decoder and unblocks concurrent control waiters.
func (r *PacketReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	close(r.done)
	return r.receiver.Close()
}

func (r *PacketReader) apply(command readCommand) {
	if command.mode != nil {
		if r.receiver.mode != *command.mode {
			r.receiver.Discard()
			r.receiver.mode = *command.mode
			r.mode.Store(uint32(*command.mode))
		}
		r.ready = append(r.ready, readResult{fence: command.fence})
		return
	}
	if !command.discard {
		frames, err := r.receiver.Flush()
		r.enqueue(frames)
		if err != nil {
			r.terminal = err
		}
	}
	r.receiver.Discard()
	r.ready = append(r.ready, readResult{fence: command.fence})
}

func (r *PacketReader) enqueue(frames []Frame) {
	for i := range frames {
		r.ready = append(r.ready, readResult{frame: &frames[i]})
	}
}
