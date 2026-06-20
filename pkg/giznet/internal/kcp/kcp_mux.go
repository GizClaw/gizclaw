package kcp

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"sync"
	"time"

	gokcp "github.com/xtaci/kcp-go/v5"
)

const (
	kcpMuxFrameOpen byte = iota
	kcpMuxFrameData
	kcpMuxFrameClose
	kcpMuxFrameCloseAck
)

// Mux wire format:
// DATA: frame_type(1B) | raw KCP segment(s), routed by segment conv.
// Control: frame_type(1B) | conv(4B little-endian) | payload.

const (
	streamCloseReasonClose byte = iota
	streamCloseReasonAbort
	streamCloseReasonInvalid
)

var (
	errStreamClosedGraceful = errors.New("kcp: stream closed by peer")
	errStreamAborted        = errors.New("kcp: stream aborted by peer")
	errStreamInvalid        = errors.New("kcp: stream closed as invalid")
	errStreamIDExhausted    = errors.New("kcp: stream id exhausted")
)

type KcpMuxConfig struct {
	CloseAckTimeout   time.Duration
	IdleStreamTimeout time.Duration
	AcceptBacklog     int
	MaxActiveStreams  int
}

type streamEntry struct {
	streamID uint64
	conn     *KCPConn

	mu          sync.Mutex
	active      bool
	queued      bool
	localClose  bool
	closeAckCh  chan struct{}
	closeAckSet bool
	idleTimer   *time.Timer
}

type KcpMux struct {
	serviceID uint64
	isClient  bool
	config    KcpMuxConfig
	output    func(service uint64, data []byte) error
	reportErr func(service uint64, err error)

	mu                sync.Mutex
	streams           map[uint64]*streamEntry
	nextLocalStreamID uint64
	activeStreams     int
	acceptCh          chan uint64
	acceptStopCh      chan struct{}
	acceptingRemote   bool
	closeCh           chan struct{}
	closing           bool
	closed            bool
	closeOnce         sync.Once
	acceptStopOnce    sync.Once
}

type kcpStream struct {
	mux     *KcpMux
	service uint64
	stream  uint64
	conn    *KCPConn
}

func defaultKcpMuxConfig(cfg KcpMuxConfig) KcpMuxConfig {
	if cfg.CloseAckTimeout <= 0 {
		cfg.CloseAckTimeout = 15 * time.Second
	}
	if cfg.IdleStreamTimeout <= 0 {
		cfg.IdleStreamTimeout = kcpDefaultIdleTimeout
	}
	if cfg.AcceptBacklog <= 0 {
		cfg.AcceptBacklog = 32
	}
	if cfg.MaxActiveStreams <= 0 {
		cfg.MaxActiveStreams = 32
	}
	return cfg
}

func NewKcpMux(serviceID uint64, isClient bool, cfg KcpMuxConfig, output func(service uint64, data []byte) error, reportErr func(service uint64, err error)) *KcpMux {
	cfg = defaultKcpMuxConfig(cfg)
	m := &KcpMux{
		serviceID:         serviceID,
		isClient:          isClient,
		config:            cfg,
		output:            output,
		reportErr:         reportErr,
		streams:           make(map[uint64]*streamEntry),
		nextLocalStreamID: initialLocalStreamID(isClient),
		acceptCh:          make(chan uint64, cfg.AcceptBacklog),
		acceptStopCh:      make(chan struct{}),
		acceptingRemote:   true,
		closeCh:           make(chan struct{}),
	}
	return m
}

func initialLocalStreamID(isClient bool) uint64 {
	if isClient {
		return 1
	}
	return 0
}

func (m *KcpMux) Open() (net.Conn, error) {
	m.mu.Lock()
	if m.closed || m.closing {
		m.mu.Unlock()
		return nil, ErrServiceMuxClosed
	}

	streamID, err := m.allocateLocalStreamIDLocked()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}

	entry := m.newStreamEntryLocked(streamID)
	entry.active = true
	m.streams[streamID] = entry
	m.activeStreams++
	m.resetIdleTimerLocked(entry)
	m.mu.Unlock()

	if err := m.sendFrame(streamID, kcpMuxFrameOpen, nil); err != nil {
		m.removeStream(streamID, ErrConnClosedLocal)
		return nil, err
	}

	return m.wrapStream(entry), nil
}

func (m *KcpMux) Accept() (net.Conn, error) {
	for {
		select {
		case streamID := <-m.acceptCh:
			conn, ok, err := m.acceptQueued(streamID)
			if err != nil {
				return nil, err
			}
			if ok {
				return conn, nil
			}
		case <-m.acceptStopCh:
			m.mu.Lock()
			closing := m.closing || m.closed
			m.mu.Unlock()
			if closing {
				return nil, ErrServiceMuxClosed
			}
			return nil, ErrAcceptQueueClosed
		case <-m.closeCh:
			return nil, ErrServiceMuxClosed
		}
	}
}

func (m *KcpMux) Input(data []byte) error {
	m.mu.Lock()
	closed := m.closed
	closing := m.closing
	m.mu.Unlock()
	if closed || closing {
		return ErrServiceMuxClosed
	}

	streamID, frameType, payload, err := decodeMuxFrame(data)
	if err != nil {
		return err
	}

	entry, exists := m.getEntry(streamID)
	invalidRemoteID := !m.isRemoteStreamID(streamID)

	switch frameType {
	case kcpMuxFrameCloseAck:
		m.touchStream(streamID)
		if exists {
			m.handleCloseAck(entry)
		}
		return nil
	case kcpMuxFrameClose:
		reason := streamCloseReasonInvalid
		if len(payload) == 1 {
			reason = payload[0]
		}
		m.touchStream(streamID)
		if exists {
			m.handleRemoteClose(entry, reason)
		}
		m.sendCloseAck(streamID)
		return nil
	}

	invalid := !exists && invalidRemoteID
	if frameType == kcpMuxFrameOpen {
		if len(payload) != 0 {
			invalid = true
		}
		if !invalid && exists {
			m.touchStream(streamID)
			return nil
		}
		if invalid {
			if exists {
				m.removeStream(streamID, errStreamInvalid)
			}
			m.sendClose(streamID, streamCloseReasonInvalid)
			return nil
		}
		return m.handleOpen(streamID)
	}

	if frameType != kcpMuxFrameData {
		invalid = true
	}
	if !exists && !invalid && frameType == kcpMuxFrameData {
		if err := m.handleOpen(streamID); err != nil {
			return err
		}
		entry, exists = m.getEntry(streamID)
	}
	if invalid {
		if exists {
			m.removeStream(streamID, errStreamInvalid)
		}
		m.sendClose(streamID, streamCloseReasonInvalid)
		return nil
	}
	if !exists {
		m.sendClose(streamID, streamCloseReasonInvalid)
		return nil
	}
	if err := entry.conn.Input(payload); err != nil {
		m.removeStream(streamID, err)
		m.sendClose(streamID, streamCloseReasonInvalid)
		return nil
	}
	m.touchStream(streamID)
	return nil
}

func (m *KcpMux) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		if m.closed || m.closing {
			m.mu.Unlock()
			return
		}
		m.acceptingRemote = false
		m.acceptStopOnce.Do(func() {
			close(m.acceptStopCh)
		})
		m.closing = true
		close(m.closeCh)
		streamIDs := make([]uint64, 0, len(m.streams))
		for streamID := range m.streams {
			streamIDs = append(streamIDs, streamID)
		}
		m.mu.Unlock()

		for _, streamID := range streamIDs {
			_ = m.closeStream(streamID)
		}

		m.mu.Lock()
		for streamID := range m.streams {
			m.removeStreamLocked(streamID, ErrConnClosedLocal)
		}
		m.closed = true
		m.mu.Unlock()
	})
	return nil
}

func (m *KcpMux) StopAccepting() error {
	m.mu.Lock()
	if m.closed || m.closing || !m.acceptingRemote {
		m.mu.Unlock()
		return nil
	}
	m.acceptingRemote = false
	queued := make([]uint64, 0)
	for streamID, entry := range m.streams {
		entry.mu.Lock()
		isQueued := entry.queued && !entry.active
		entry.mu.Unlock()
		if !isQueued {
			continue
		}
		m.removeStreamLocked(streamID, errStreamAborted)
		queued = append(queued, streamID)
	}
	m.mu.Unlock()

	m.acceptStopOnce.Do(func() {
		close(m.acceptStopCh)
	})
	for _, streamID := range queued {
		m.sendClose(streamID, streamCloseReasonAbort)
	}
	return nil
}

func (m *KcpMux) NumStreams() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.streams)
}

func (m *KcpMux) wrapStream(entry *streamEntry) net.Conn {
	return &kcpStream{
		mux:     m,
		service: m.serviceID,
		stream:  entry.streamID,
		conn:    entry.conn,
	}
}

func (m *KcpMux) acceptQueued(streamID uint64) (net.Conn, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.closing {
		return nil, false, ErrServiceMuxClosed
	}
	if !m.acceptingRemote {
		return nil, false, ErrAcceptQueueClosed
	}
	entry, ok := m.streams[streamID]
	if !ok {
		return nil, false, nil
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if !entry.queued {
		return nil, false, nil
	}
	entry.queued = false
	entry.active = true
	m.activeStreams++
	m.resetIdleTimerLocked(entry)
	return m.wrapStream(entry), true, nil
}

func (m *KcpMux) handleOpen(streamID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed || m.closing || !m.acceptingRemote {
		go m.sendClose(streamID, streamCloseReasonAbort)
		return nil
	}
	if _, exists := m.streams[streamID]; exists {
		return nil
	}
	if m.activeStreams >= m.config.MaxActiveStreams {
		go m.sendClose(streamID, streamCloseReasonAbort)
		return nil
	}
	if len(m.acceptCh) >= cap(m.acceptCh) {
		go m.sendClose(streamID, streamCloseReasonAbort)
		return nil
	}

	entry := m.newStreamEntryLocked(streamID)
	entry.queued = true
	m.streams[streamID] = entry
	m.resetIdleTimerLocked(entry)

	select {
	case m.acceptCh <- streamID:
	default:
		m.removeStreamLocked(streamID, errStreamAborted)
		go m.sendClose(streamID, streamCloseReasonAbort)
	}
	return nil
}

func (m *KcpMux) handleRemoteClose(entry *streamEntry, reason byte) {
	closeErr := errStreamInvalid
	switch reason {
	case streamCloseReasonClose:
		closeErr = errStreamClosedGraceful
	case streamCloseReasonAbort:
		closeErr = errStreamAborted
	}
	m.removeStream(entry.streamID, closeErr)
}

func (m *KcpMux) handleCloseAck(entry *streamEntry) {
	entry.mu.Lock()
	ch := entry.closeAckCh
	shouldCleanup := entry.localClose
	if ch != nil && !entry.closeAckSet {
		close(ch)
		entry.closeAckSet = true
	}
	entry.mu.Unlock()

	if shouldCleanup {
		m.removeStream(entry.streamID, ErrConnClosedLocal)
	}
}

func (m *KcpMux) closeStream(streamID uint64) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	entry, ok := m.streams[streamID]
	if !ok {
		m.mu.Unlock()
		return nil
	}

	entry.mu.Lock()
	if entry.localClose {
		ch := entry.closeAckCh
		entry.mu.Unlock()
		m.mu.Unlock()
		return m.waitForCloseAck(streamID, ch)
	}
	entry.localClose = true
	entry.closeAckCh = make(chan struct{})
	ch := entry.closeAckCh
	m.resetIdleTimerLocked(entry)
	entry.mu.Unlock()
	m.mu.Unlock()

	if err := entry.conn.Drain(time.Now().Add(m.config.CloseAckTimeout)); err != nil {
		entry.conn.closeSignal(err)
		go entry.conn.finalizeClose()
		m.sendClose(streamID, streamCloseReasonAbort)
		return err
	}
	entry.conn.closeSignal(ErrConnClosedLocal)
	go entry.conn.finalizeClose()
	m.sendClose(streamID, streamCloseReasonClose)
	return m.waitForCloseAck(streamID, ch)
}

func (m *KcpMux) waitForCloseAck(streamID uint64, ch chan struct{}) error {
	if ch == nil {
		return nil
	}

	timer := time.NewTimer(m.config.CloseAckTimeout)
	defer timer.Stop()

	select {
	case <-ch:
		return nil
	case <-timer.C:
		m.removeStream(streamID, ErrConnClosedLocal)
		return nil
	case <-m.closeCh:
		return nil
	}
}

func (m *KcpMux) allocateLocalStreamIDLocked() (uint64, error) {
	start := uint32(m.nextLocalStreamID)
	current := start
	for {
		if _, exists := m.streams[uint64(current)]; !exists {
			next := current + 2
			m.nextLocalStreamID = uint64(next)
			return uint64(current), nil
		}
		current += 2
		if current == start {
			break
		}
	}
	return 0, errStreamIDExhausted
}

func (m *KcpMux) newStreamEntryLocked(streamID uint64) *streamEntry {
	entry := &streamEntry{streamID: streamID}
	entry.conn = NewKCPConn(uint32(streamID), func(data []byte) {
		m.touchStream(streamID)
		if err := m.sendFrame(streamID, kcpMuxFrameData, data); err != nil {
			m.reportOutput(err)
		}
	})
	return entry
}

func (m *KcpMux) getEntry(streamID uint64) (*streamEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.streams[streamID]
	return entry, ok
}

func (m *KcpMux) removeStream(streamID uint64, closeErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeStreamLocked(streamID, closeErr)
}

func (m *KcpMux) removeStreamLocked(streamID uint64, closeErr error) {
	entry, ok := m.streams[streamID]
	if !ok {
		return
	}
	delete(m.streams, streamID)

	entry.mu.Lock()
	if entry.idleTimer != nil {
		entry.idleTimer.Stop()
		entry.idleTimer = nil
	}
	if entry.active && m.activeStreams > 0 {
		m.activeStreams--
	}
	if entry.closeAckCh != nil && !entry.closeAckSet {
		close(entry.closeAckCh)
		entry.closeAckSet = true
	}
	entry.mu.Unlock()
	entry.conn.closeSignal(closeErr)
	go entry.conn.finalizeClose()
}

func (m *KcpMux) resetIdleTimerLocked(entry *streamEntry) {
	if m.config.IdleStreamTimeout <= 0 {
		return
	}
	if entry.idleTimer == nil {
		streamID := entry.streamID
		entry.idleTimer = time.AfterFunc(m.config.IdleStreamTimeout, func() {
			m.removeStream(streamID, ErrConnTimeout)
		})
		return
	}
	entry.idleTimer.Reset(m.config.IdleStreamTimeout)
}

func (m *KcpMux) touchStream(streamID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.streams[streamID]
	if !ok {
		return
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	m.resetIdleTimerLocked(entry)
}

func (m *KcpMux) sendFrame(streamID uint64, frameType byte, payload []byte) error {
	frame := []byte{frameType}
	if frameType != kcpMuxFrameData {
		frame = binary.LittleEndian.AppendUint32(frame, uint32(streamID))
	}
	frame = append(frame, payload...)
	if m.output == nil {
		return nil
	}
	if err := m.output(m.serviceID, frame); err != nil {
		m.reportOutput(err)
		return err
	}
	return nil
}

func (m *KcpMux) sendClose(streamID uint64, reason byte) {
	_ = m.sendFrame(streamID, kcpMuxFrameClose, []byte{reason})
}

func (m *KcpMux) sendCloseAck(streamID uint64) {
	_ = m.sendFrame(streamID, kcpMuxFrameCloseAck, nil)
}

func (m *KcpMux) reportOutput(err error) {
	if err == nil {
		return
	}
	if m.reportErr != nil {
		m.reportErr(m.serviceID, err)
	}
}

func (m *KcpMux) isRemoteStreamID(streamID uint64) bool {
	if streamID > math.MaxUint32 {
		return false
	}
	if m.isClient {
		return streamID%2 == 0
	}
	return streamID%2 == 1
}

func decodeMuxFrame(data []byte) (uint64, byte, []byte, error) {
	if len(data) == 0 {
		return 0, 0, nil, ErrInvalidServiceFrame
	}
	frameType := data[0]
	if frameType == kcpMuxFrameData {
		streamID, err := decodeKCPConv(data[1:])
		if err != nil {
			return 0, 0, nil, err
		}
		return streamID, frameType, data[1:], nil
	}

	if len(data) < 5 {
		return 0, 0, nil, ErrInvalidServiceFrame
	}
	streamID := uint64(binary.LittleEndian.Uint32(data[1:]))
	return streamID, frameType, data[5:], nil
}

func decodeKCPConv(data []byte) (uint64, error) {
	if len(data) < gokcp.IKCP_OVERHEAD {
		return 0, ErrInvalidServiceFrame
	}
	return uint64(binary.LittleEndian.Uint32(data)), nil
}

func (s *kcpStream) Read(b []byte) (int, error) {
	n, err := s.conn.Read(b)
	if err == nil {
		return n, nil
	}
	if errors.Is(err, errStreamClosedGraceful) {
		return n, io.EOF
	}
	return n, err
}

func (s *kcpStream) Write(b []byte) (int, error) {
	n, err := s.conn.Write(b)
	if err == nil {
		return n, nil
	}
	if errors.Is(err, errStreamClosedGraceful) {
		return n, io.ErrClosedPipe
	}
	return n, err
}

func (s *kcpStream) Close() error {
	return s.mux.closeStream(s.stream)
}

func (s *kcpStream) LocalAddr() net.Addr {
	return s.conn.LocalAddr()
}

func (s *kcpStream) RemoteAddr() net.Addr {
	return s.conn.RemoteAddr()
}

func (s *kcpStream) SetDeadline(t time.Time) error {
	return s.conn.SetDeadline(t)
}

func (s *kcpStream) SetReadDeadline(t time.Time) error {
	return s.conn.SetReadDeadline(t)
}

func (s *kcpStream) SetWriteDeadline(t time.Time) error {
	return s.conn.SetWriteDeadline(t)
}
