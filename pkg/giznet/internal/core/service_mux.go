package core

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"

	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/kcp"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/internal/noise"
	gokcp "github.com/xtaci/kcp-go/v5"
)

var (
	ErrServiceMuxClosed    = kcp.ErrServiceMuxClosed
	ErrAcceptQueueClosed   = kcp.ErrAcceptQueueClosed
	ErrServiceRejected     = kcp.ErrServiceRejected
	ErrInboundQueueFull    = kcp.ErrInboundQueueFull
	ErrInvalidServiceFrame = kcp.ErrInvalidServiceFrame
)

var afterServiceMuxDirectReadHook func()

const (
	serviceMuxFrameOpen byte = iota
	serviceMuxFrameData
	serviceMuxFrameClose
	serviceMuxFrameCloseAck
)

const (
	serviceStreamCloseReasonClose byte = iota
	serviceStreamCloseReasonAbort
	serviceStreamCloseReasonInvalid
)

type ServiceMuxConfig struct {
	IsClient            bool
	Output              func(peer noise.PublicKey, service uint64, protocol byte, data []byte) error
	OnOutputError       func(peer noise.PublicKey, service uint64, err error)
	OnNewService        func(peer noise.PublicKey, service uint64) bool
	DefaultKcpMuxConfig kcp.KcpMuxConfig
}

type serviceState struct {
	serviceID        uint64
	mux              *kcp.KcpMux
	closed           bool
	acceptingStopped bool
}

type ServiceMux struct {
	config ServiceMuxConfig
	peer   noise.PublicKey

	mu       sync.RWMutex
	services map[uint64]*serviceState
	closeCh  chan struct{}
	directCh chan protoPacket

	closed    atomic.Bool
	closeOnce sync.Once

	outputErrors atomic.Uint64
}

func NewServiceMux(peer noise.PublicKey, cfg ServiceMuxConfig) *ServiceMux {
	if cfg.OnNewService == nil {
		cfg.OnNewService = func(_ noise.PublicKey, service uint64) bool {
			return service == 0
		}
	}
	return &ServiceMux{
		config:   cfg,
		peer:     peer,
		services: make(map[uint64]*serviceState),
		closeCh:  make(chan struct{}),
		directCh: make(chan protoPacket, InboundChanSize),
	}
}

func (m *ServiceMux) InputKCP(service uint64, data []byte) error {
	if m.closed.Load() {
		return ErrServiceMuxClosed
	}

	state, ok := m.getService(service)
	if ok {
		if state.closed {
			return m.rejectUnknownService(service, data)
		}
		if state.acceptingStopped && shouldRejectStoppedService(data) {
			return m.rejectUnknownService(service, data)
		}
		return m.inputKCPToService(state, data)
	}

	if !m.config.OnNewService(m.peer, service) {
		return m.rejectUnknownService(service, data)
	}

	state, err := m.getOrCreateService(service)
	if err != nil {
		return err
	}
	return m.inputKCPToService(state, data)
}

func (m *ServiceMux) InputPacket(protocol byte, data []byte) error {
	if m.closed.Load() {
		return ErrServiceMuxClosed
	}
	if protocol == ProtocolKCP {
		return ErrKCPMustUseStream
	}
	select {
	case m.directCh <- protoPacket{protocol: protocol, payload: data}:
		return nil
	default:
		return ErrInboundQueueFull
	}
}

func (m *ServiceMux) Read(buf []byte) (protocol byte, n int, err error) {
	if m.closed.Load() {
		return 0, 0, ErrServiceMuxClosed
	}

	select {
	case pkt, ok := <-m.directCh:
		n, err = m.copyDirectPacket(pkt, ok, buf)
		if err != nil {
			return 0, 0, err
		}
		return pkt.protocol, n, nil
	case <-m.closeCh:
		return 0, 0, ErrServiceMuxClosed
	}
}

func (m *ServiceMux) Write(protocol byte, data []byte) (n int, err error) {
	if m.closed.Load() {
		return 0, ErrServiceMuxClosed
	}

	switch protocol {
	case ProtocolKCP:
		return 0, ErrKCPMustUseStream
	}

	if m.config.Output == nil {
		return 0, ErrNoSession
	}
	if err := m.config.Output(m.peer, 0, protocol, data); err != nil {
		m.reportOutputError(0, err)
		return 0, err
	}
	return len(data), nil
}

func (m *ServiceMux) OpenStream(service uint64) (net.Conn, error) {
	if m.closed.Load() {
		return nil, ErrServiceMuxClosed
	}
	state, err := m.getOrCreateService(service)
	if err != nil {
		return nil, err
	}
	mux, err := m.ensureKcpMux(state)
	if err != nil {
		return nil, err
	}
	return mux.Open()
}

func (m *ServiceMux) AcceptStream(service uint64) (net.Conn, error) {
	if m.closed.Load() {
		return nil, ErrServiceMuxClosed
	}
	state, err := m.getOrCreateService(service)
	if err != nil {
		return nil, err
	}
	mux, err := m.ensureKcpMux(state)
	if err != nil {
		return nil, err
	}
	return mux.Accept()
}

func (m *ServiceMux) CloseService(service uint64) error {
	if m.closed.Load() {
		return ErrServiceMuxClosed
	}

	m.mu.Lock()
	state, ok := m.services[service]
	if !ok {
		state = &serviceState{
			serviceID: service,
		}
		m.services[service] = state
	}
	state.closed = true
	state.acceptingStopped = true
	m.mu.Unlock()
	if state.mux != nil {
		return state.mux.Close()
	}
	return nil
}

func (m *ServiceMux) StopAcceptingService(service uint64) error {
	if m.closed.Load() {
		return ErrServiceMuxClosed
	}

	m.mu.Lock()
	state, ok := m.services[service]
	if !ok {
		state = &serviceState{
			serviceID: service,
		}
		m.services[service] = state
	}
	state.acceptingStopped = true
	m.mu.Unlock()
	if state.mux == nil {
		return nil
	}
	return state.mux.StopAccepting()
}

func (m *ServiceMux) Close() error {
	m.closeOnce.Do(func() {
		m.closed.Store(true)
		close(m.closeCh)

		m.mu.Lock()
		states := make([]*serviceState, 0, len(m.services))
		for _, state := range m.services {
			states = append(states, state)
		}
		m.services = make(map[uint64]*serviceState)
		m.mu.Unlock()

		for _, state := range states {
			if state.mux != nil {
				_ = state.mux.Close()
			}
		}
	})
	return nil
}

func (m *ServiceMux) IsClosed() bool {
	return m == nil || m.closed.Load()
}

func (m *ServiceMux) NumServices() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.services)
}

func (m *ServiceMux) NumStreams() int {
	m.mu.RLock()
	states := make([]*serviceState, 0, len(m.services))
	for _, state := range m.services {
		states = append(states, state)
	}
	m.mu.RUnlock()

	total := 0
	for _, state := range states {
		if state.mux != nil {
			total += state.mux.NumStreams()
		}
	}
	return total
}

func (m *ServiceMux) OutputErrorCount() uint64 {
	return m.outputErrors.Load()
}

func (m *ServiceMux) getService(service uint64) (*serviceState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.services[service]
	return state, ok
}

func (m *ServiceMux) getOrCreateService(service uint64) (*serviceState, error) {
	if m.closed.Load() {
		return nil, ErrServiceMuxClosed
	}

	m.mu.RLock()
	state, ok := m.services[service]
	m.mu.RUnlock()
	if ok {
		if state.closed {
			return nil, ErrServiceMuxClosed
		}
		return state, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed.Load() {
		return nil, ErrServiceMuxClosed
	}
	if state, ok := m.services[service]; ok {
		if state.closed {
			return nil, ErrServiceMuxClosed
		}
		return state, nil
	}

	state = &serviceState{
		serviceID: service,
	}
	m.services[service] = state
	return state, nil
}

func (m *ServiceMux) ensureKcpMux(state *serviceState) (*kcp.KcpMux, error) {
	m.mu.RLock()
	if state.mux != nil {
		mux := state.mux
		m.mu.RUnlock()
		return mux, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed.Load() {
		return nil, ErrServiceMuxClosed
	}
	if state.mux != nil {
		return state.mux, nil
	}
	if state.closed {
		return nil, ErrServiceMuxClosed
	}

	state.mux = kcp.NewKcpMux(
		state.serviceID,
		m.config.IsClient,
		m.config.DefaultKcpMuxConfig,
		func(service uint64, data []byte) error {
			if m.config.Output == nil {
				return ErrNoSession
			}
			return m.config.Output(m.peer, service, ProtocolKCP, data)
		},
		m.reportOutputError,
	)
	if state.acceptingStopped {
		if err := state.mux.StopAccepting(); err != nil {
			return nil, err
		}
	}
	return state.mux, nil
}

func (m *ServiceMux) reportOutputError(service uint64, err error) {
	if err == nil {
		return
	}
	m.outputErrors.Add(1)
	if m.config.OnOutputError != nil {
		m.config.OnOutputError(m.peer, service, err)
	}
}

func (m *ServiceMux) rejectUnknownService(service uint64, data []byte) error {
	streamID, frameType, err := decodeServiceMuxFrame(data)
	if err != nil {
		return err
	}

	switch frameType {
	case serviceMuxFrameOpen:
		m.sendServiceControlFrame(service, streamID, serviceMuxFrameClose, []byte{serviceStreamCloseReasonAbort})
	case serviceMuxFrameClose:
		m.sendServiceControlFrame(service, streamID, serviceMuxFrameCloseAck, nil)
	case serviceMuxFrameCloseAck:
		return nil
	default:
		m.sendServiceControlFrame(service, streamID, serviceMuxFrameClose, []byte{serviceStreamCloseReasonInvalid})
	}
	return nil
}

func (m *ServiceMux) sendServiceControlFrame(service uint64, streamID uint64, frameType byte, payload []byte) {
	frame := []byte{frameType}
	frame = binary.LittleEndian.AppendUint32(frame, uint32(streamID))
	frame = append(frame, payload...)
	if m.config.Output == nil {
		return
	}
	if err := m.config.Output(m.peer, service, ProtocolKCP, frame); err != nil {
		m.reportOutputError(service, err)
	}
}

func decodeServiceMuxFrame(data []byte) (uint64, byte, error) {
	if len(data) == 0 {
		return 0, 0, ErrInvalidServiceFrame
	}
	frameType := data[0]
	if frameType == serviceMuxFrameData {
		if len(data[1:]) < gokcp.IKCP_OVERHEAD {
			return 0, 0, ErrInvalidServiceFrame
		}
		return uint64(binary.LittleEndian.Uint32(data[1:])), frameType, nil
	}

	if len(data) < 5 {
		return 0, 0, ErrInvalidServiceFrame
	}
	streamID := uint64(binary.LittleEndian.Uint32(data[1:]))
	return streamID, frameType, nil
}

func shouldRejectStoppedService(data []byte) bool {
	_, frameType, err := decodeServiceMuxFrame(data)
	return err == nil && frameType == serviceMuxFrameOpen
}

func (m *ServiceMux) copyDirectPacket(pkt protoPacket, ok bool, buf []byte) (int, error) {
	if !ok {
		return 0, ErrServiceMuxClosed
	}
	if afterServiceMuxDirectReadHook != nil {
		afterServiceMuxDirectReadHook()
	}
	if m.closed.Load() {
		return 0, ErrServiceMuxClosed
	}
	return copy(buf, pkt.payload), nil
}

func (m *ServiceMux) inputKCPToService(state *serviceState, data []byte) error {
	mux, err := m.ensureKcpMux(state)
	if err != nil {
		return err
	}
	return mux.Input(data)
}
