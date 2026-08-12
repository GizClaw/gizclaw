package gizwebrtc

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v4"
)

// NativeChannelOptions selects the SCTP delivery semantics for a labeled
// DataChannel. Both partial-reliability options must be nil for reliable
// delivery, and WebRTC forbids setting both options at once.
type NativeChannelOptions struct {
	Ordered           bool
	MaxPacketLifeTime *uint16
	MaxRetransmits    *uint16
}

const nativeChannelRemoteCloseDrainTimeout = time.Second

// NativeChannel is one detached, labeled WebRTC DataChannel. Reliable callers
// use its net.Conn surface; packet callers use ReadMessage and WriteMessage to
// retain DataChannel message boundaries.
type NativeChannel struct {
	label             string
	ordered           bool
	maxPacketLifeTime *uint16
	maxRetransmits    *uint16
	stream            *dataChannelConn
	raw               datachannel.ReadWriteCloserDeadliner
	messageReadMu     sync.Mutex
	messageWriteMu    sync.Mutex
}

// Label returns the immutable DCEP label that declared this channel.
func (c *NativeChannel) Label() string {
	if c == nil {
		return ""
	}
	return c.label
}

// Ordered reports the negotiated ordered-delivery setting.
func (c *NativeChannel) Ordered() bool {
	return c != nil && c.ordered
}

// MaxPacketLifeTime reports the negotiated packet lifetime in milliseconds.
// Nil means that time-limited partial reliability is disabled.
func (c *NativeChannel) MaxPacketLifeTime() *uint16 {
	if c == nil || c.maxPacketLifeTime == nil {
		return nil
	}
	value := *c.maxPacketLifeTime
	return &value
}

// MaxRetransmits reports the negotiated retransmission limit. Nil means that
// retransmission-limited partial reliability is disabled.
func (c *NativeChannel) MaxRetransmits() *uint16 {
	if c == nil || c.maxRetransmits == nil {
		return nil
	}
	value := *c.maxRetransmits
	return &value
}

// ReadMessage reads exactly one DataChannel message.
func (c *NativeChannel) ReadMessage(buf []byte) (int, error) {
	if c == nil || c.raw == nil {
		return 0, giznet.ErrConnClosed
	}
	c.messageReadMu.Lock()
	defer c.messageReadMu.Unlock()
	n, _, err := c.raw.ReadDataChannel(buf)
	if nativeChannelErrorIsTerminal(err) {
		_ = c.Close()
		err = normalizeNativeChannelTerminalError(err)
	}
	return n, err
}

// WriteMessage writes exactly one binary DataChannel message.
func (c *NativeChannel) WriteMessage(payload []byte) (int, error) {
	if c == nil || c.raw == nil {
		return 0, giznet.ErrConnClosed
	}
	if len(payload) > maxPacketMessageSize {
		return 0, giznet.ErrPacketTooLarge
	}
	c.messageWriteMu.Lock()
	defer c.messageWriteMu.Unlock()
	n, err := c.raw.WriteDataChannel(payload, false)
	if nativeChannelErrorIsTerminal(err) {
		_ = c.Close()
		err = normalizeNativeChannelTerminalError(err)
	}
	return n, err
}

func (c *NativeChannel) Read(buf []byte) (int, error) {
	if c == nil || c.stream == nil {
		return 0, giznet.ErrConnClosed
	}
	n, err := c.stream.Read(buf)
	if nativeChannelErrorIsTerminal(err) {
		_ = c.Close()
		err = normalizeNativeChannelTerminalError(err)
	}
	return n, err
}

func (c *NativeChannel) Write(buf []byte) (int, error) {
	if c == nil || c.stream == nil {
		return 0, giznet.ErrConnClosed
	}
	n, err := c.stream.Write(buf)
	if nativeChannelErrorIsTerminal(err) {
		_ = c.Close()
		err = normalizeNativeChannelTerminalError(err)
	}
	return n, err
}

func (c *NativeChannel) Close() error {
	if c == nil || c.stream == nil {
		return nil
	}
	return c.stream.Close()
}

func (c *NativeChannel) LocalAddr() net.Addr {
	if c == nil || c.stream == nil {
		return nil
	}
	return c.stream.LocalAddr()
}

func (c *NativeChannel) RemoteAddr() net.Addr {
	if c == nil || c.stream == nil {
		return nil
	}
	return c.stream.RemoteAddr()
}

func (c *NativeChannel) SetDeadline(deadline time.Time) error {
	if c == nil || c.stream == nil {
		return giznet.ErrConnClosed
	}
	return c.stream.SetDeadline(deadline)
}

func (c *NativeChannel) SetReadDeadline(deadline time.Time) error {
	if c == nil || c.stream == nil {
		return giznet.ErrConnClosed
	}
	return c.stream.SetReadDeadline(deadline)
}

func (c *NativeChannel) SetWriteDeadline(deadline time.Time) error {
	if c == nil || c.stream == nil {
		return giznet.ErrConnClosed
	}
	return c.stream.SetWriteDeadline(deadline)
}

// SetWriteBudgets applies shared outstanding-byte budgets to reliable writes.
// It must be called before the first write. Each successful write remains
// reserved until the DataChannel BufferedAmount reports the bytes drained.
func (c *NativeChannel) SetWriteBudgets(budgets ...*WriteBudget) error {
	if c == nil || c.stream == nil {
		return giznet.ErrConnClosed
	}
	return c.stream.setWriteBudgets(budgets...)
}

func nativeChannelErrorIsTerminal(err error) bool {
	if err == nil || errors.Is(err, io.ErrShortBuffer) || errors.Is(err, os.ErrDeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return !errors.As(err, &netErr) || !netErr.Timeout()
}

func normalizeNativeChannelTerminalError(err error) error {
	if err == nil || errors.Is(err, giznet.ErrConnClosed) || errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) {
		return err
	}
	return errors.Join(giznet.ErrConnClosed, err)
}

// RegisterNativeChannelHandler claims one incoming label prefix. The returned
// function stops admission and closes channels owned by that registration.
func (c *Conn) RegisterNativeChannelHandler(
	prefix string,
	handler func(*NativeChannel),
) (func(), error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if prefix == "" || len(prefix) > maxNativeChannelLabelSize || handler == nil {
		return nil, errors.New("gizwebrtc: invalid native channel handler")
	}
	c.nativeMu.Lock()
	if c.nativeHandler != nil {
		c.nativeMu.Unlock()
		return nil, errors.New("gizwebrtc: native channel handler already registered")
	}
	c.nativePrefix = prefix
	c.nativeHandler = handler
	c.nativeGeneration++
	generation := c.nativeGeneration
	c.nativeMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			c.nativeMu.Lock()
			if c.nativeGeneration != generation || c.nativePrefix != prefix {
				c.nativeMu.Unlock()
				return
			}
			c.nativeHandler = nil
			c.nativePrefix = ""
			channels := make([]*NativeChannel, 0, len(c.nativeChannels))
			for channel := range c.nativeChannels {
				channels = append(channels, channel)
			}
			c.nativeMu.Unlock()
			for _, channel := range channels {
				_ = channel.Close()
			}
		})
	}, nil
}

// OpenNativeChannel opens one labeled DataChannel and waits for DCEP open. It
// does not imply that the remote application accepted the label.
func (c *Conn) OpenNativeChannel(
	ctx context.Context,
	label string,
	options NativeChannelOptions,
) (*NativeChannel, error) {
	if ctx == nil {
		return nil, errors.New("gizwebrtc: nil native channel context")
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	if err := validateNativeChannelLabel(label); err != nil {
		return nil, err
	}
	init := &webrtc.DataChannelInit{Ordered: &options.Ordered}
	if options.MaxPacketLifeTime != nil {
		value := *options.MaxPacketLifeTime
		init.MaxPacketLifeTime = &value
	}
	if options.MaxRetransmits != nil {
		value := *options.MaxRetransmits
		init.MaxRetransmits = &value
	}
	dc, err := c.pc.CreateDataChannel(label, init)
	if err != nil {
		return nil, err
	}
	raw, err := detachWhenOpen(ctx, dc, c.closeCh, c.parentCloseError)
	if err != nil {
		_ = dc.Close()
		return nil, err
	}
	channel := c.newNativeChannel(dc, raw)
	if err := c.trackNativeChannel(dc, channel, nil); err != nil {
		_ = channel.Close()
		return nil, err
	}
	return channel, nil
}

func validateNativeChannelLabel(label string) error {
	if label == "" || len(label) > maxNativeChannelLabelSize || strings.IndexByte(label, 0) >= 0 {
		return errors.New("gizwebrtc: invalid native channel label")
	}
	return nil
}

func (c *Conn) handleNativeDataChannel(dc *webrtc.DataChannel) bool {
	label := dc.Label()
	c.nativeMu.Lock()
	handler := c.nativeHandler
	prefix := c.nativePrefix
	generation := c.nativeGeneration
	if handler == nil || !strings.HasPrefix(label, prefix) {
		c.nativeMu.Unlock()
		return false
	}
	if len(label) > maxNativeChannelLabelSize {
		c.nativeMu.Unlock()
		_ = dc.Close()
		return true
	}
	if c.nativeInbound == nil {
		c.nativeInbound = make(map[*webrtc.DataChannel]struct{})
	}
	if len(c.nativeInbound) >= maxInboundNativeChannels {
		c.nativeMu.Unlock()
		_ = dc.Close()
		return true
	}
	c.nativeInbound[dc] = struct{}{}
	c.nativeMu.Unlock()

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			c.nativeMu.Lock()
			delete(c.nativeInbound, dc)
			c.nativeMu.Unlock()
		})
	}
	dc.OnClose(release)
	dc.OnError(func(error) { release() })
	dc.OnOpen(func() {
		raw, err := dc.DetachWithDeadline()
		if err != nil {
			release()
			_ = dc.Close()
			return
		}
		channel := c.newNativeChannel(dc, raw)
		if err := c.trackNativeChannel(dc, channel, release); err != nil {
			release()
			_ = channel.Close()
			return
		}
		c.nativeMu.Lock()
		current := c.nativeHandler
		currentPrefix := c.nativePrefix
		currentGeneration := c.nativeGeneration
		c.nativeMu.Unlock()
		if current == nil || currentGeneration != generation || currentPrefix != prefix {
			_ = channel.Close()
			return
		}
		handler(channel)
	})
	return true
}

func (c *Conn) newNativeChannel(
	dc *webrtc.DataChannel,
	raw datachannel.ReadWriteCloserDeadliner,
) *NativeChannel {
	stream := newDataChannelConn(raw, dc, c.localAddr, c.remoteAddr)
	stream.rx = &c.rxBytes
	stream.tx = &c.txBytes
	channel := &NativeChannel{
		label:   dc.Label(),
		ordered: dc.Ordered(),
		stream:  stream,
		raw:     raw,
	}
	if value := dc.MaxPacketLifeTime(); value != nil {
		copyValue := *value
		channel.maxPacketLifeTime = &copyValue
	}
	if value := dc.MaxRetransmits(); value != nil {
		copyValue := *value
		channel.maxRetransmits = &copyValue
	}
	return channel
}

func (c *Conn) trackNativeChannel(
	dc *webrtc.DataChannel,
	channel *NativeChannel,
	releaseInbound func(),
) error {
	if channel == nil || channel.stream == nil {
		return giznet.ErrNilConn
	}
	c.nativeMu.Lock()
	if c.closed.Load() {
		c.nativeMu.Unlock()
		return c.parentCloseError()
	}
	if c.nativeChannels == nil {
		c.nativeChannels = make(map[*NativeChannel]struct{})
	}
	c.nativeChannels[channel] = struct{}{}
	previousOnClose := channel.stream.onClose
	channel.stream.onClose = func() {
		if previousOnClose != nil {
			previousOnClose()
		}
		if releaseInbound != nil {
			releaseInbound()
		}
		c.nativeMu.Lock()
		delete(c.nativeChannels, channel)
		c.nativeMu.Unlock()
	}
	c.nativeMu.Unlock()
	// A remote close can be reported before final queued messages have been read
	// from the detached channel. Give the application reader a bounded drain
	// window, then release orphaned channels that have no active reader.
	dc.OnClose(func() {
		time.AfterFunc(nativeChannelRemoteCloseDrainTimeout, func() { _ = channel.Close() })
	})
	dc.OnError(func(error) { _ = channel.Close() })
	return nil
}

var (
	_ net.Conn  = (*NativeChannel)(nil)
	_ io.Reader = (*NativeChannel)(nil)
)
