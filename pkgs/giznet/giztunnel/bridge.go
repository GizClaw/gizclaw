package giztunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const bridgeStreamDrainTimeout = 30 * time.Second

const (
	bridgePathService = "service"
	bridgePathPacket  = "packet"

	bridgeDirectionLeftToRight = "left_to_right"
	bridgeDirectionRightToLeft = "right_to_left"

	bridgePhaseAcceptSource     = "accept_source"
	bridgePhaseReadSource       = "read_source"
	bridgePhaseWriteDestination = "write_destination"

	bridgeErrorClassClean            = "clean"
	bridgeErrorClassEOF              = "eof"
	bridgeErrorClassClosed           = "closed"
	bridgeErrorClassConnectionClosed = "connection_closed"
	bridgeErrorClassServiceMuxClosed = "service_mux_closed"
	bridgeErrorClassBufferLimit      = "buffer_limit"
	bridgeErrorClassContextCanceled  = "context_canceled"
	bridgeErrorClassDeadlineExceeded = "deadline_exceeded"
	bridgeErrorClassOther            = "other"
)

// BridgeObservation is a bounded, payload-free summary of the first
// connection-level bridge terminal and any preceding destination-open
// rejections. Empty capacity fields mean that the rejecting boundary did not
// own an exact established-session capacity snapshot.
type BridgeObservation struct {
	Path                        string
	Direction                   string
	Phase                       string
	ErrorClass                  string
	OpenRejectionCount          uint64
	FirstOpenRejectionDirection string
	FirstOpenRejectionClass     string
	LastOpenRejectionDirection  string
	LastOpenRejectionClass      string
	CapacityScope               string
	ActiveChannels              int
	ChannelLimit                int
}

type bridgeLoopResult struct {
	path      string
	direction string
	phase     string
	err       error
}

type bridgeObservationState struct {
	mu          sync.Mutex
	observation BridgeObservation
}

// Bridge transparently forwards service streams and packets between two Giznet
// connections until either side closes.
func Bridge(left, right giznet.Conn) error {
	_, err := BridgeWithObservation(left, right)
	return err
}

// BridgeWithObservation transparently forwards service streams and packets
// between two Giznet connections and returns a bounded diagnostic summary.
// Its returned error and connection-close behavior are compatible with Bridge.
func BridgeWithObservation(left, right giznet.Conn) (BridgeObservation, error) {
	if enabler, ok := left.(giznet.ServiceAcceptEnabler); ok {
		enabler.EnableServiceAccept()
	}
	if enabler, ok := right.(giznet.ServiceAcceptEnabler); ok {
		enabler.EnableServiceAccept()
	}
	leftServices, ok := left.(giznet.ServiceAcceptor)
	if !ok {
		return BridgeObservation{}, errors.New("giztunnel: left connection cannot accept aggregate services")
	}
	rightServices, ok := right.(giznet.ServiceAcceptor)
	if !ok {
		return BridgeObservation{}, errors.New("giztunnel: right connection cannot accept aggregate services")
	}
	state := &bridgeObservationState{}
	resultCh := make(chan bridgeLoopResult, 4)
	go func() {
		resultCh <- bridgeServices(leftServices, right, bridgeDirectionLeftToRight, state)
	}()
	go func() {
		resultCh <- bridgeServices(rightServices, left, bridgeDirectionRightToLeft, state)
	}()
	go func() {
		resultCh <- bridgePackets(left, right, bridgeDirectionLeftToRight)
	}()
	go func() {
		resultCh <- bridgePackets(right, left, bridgeDirectionRightToLeft)
	}()
	result := <-resultCh
	_ = left.Close()
	_ = right.Close()
	observation := state.finish(result)
	if isClosedError(result.err) {
		return observation, nil
	}
	return observation, result.err
}

func bridgeServices(
	source giznet.ServiceAcceptor,
	destination giznet.Conn,
	direction string,
	state *bridgeObservationState,
) bridgeLoopResult {
	for {
		service, sourceStream, err := source.AcceptService()
		if err != nil {
			return bridgeLoopResult{
				path:      bridgePathService,
				direction: direction,
				phase:     bridgePhaseAcceptSource,
				err:       err,
			}
		}
		destinationStream, err := destination.Dial(service)
		if err != nil {
			_ = sourceStream.Close()
			state.recordOpenRejection(direction, err)
			continue
		}
		go bridgeStream(sourceStream, destinationStream)
	}
}

func bridgeStream(left, right net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(left, right)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(right, left)
		done <- struct{}{}
	}()
	<-done
	// A full-close transport can report EOF after its final writes while the
	// opposite endpoint is still consuming those bytes. Let that endpoint close
	// in response before forcing teardown, otherwise its queued response can be
	// discarded by an eager DataChannel close.
	timer := time.NewTimer(bridgeStreamDrainTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	_ = left.Close()
	_ = right.Close()
}

func bridgePackets(source, destination giznet.Conn, direction string) bridgeLoopResult {
	buf := make([]byte, 64*1024)
	for {
		protocol, n, err := source.Read(buf)
		if err != nil {
			return bridgeLoopResult{
				path:      bridgePathPacket,
				direction: direction,
				phase:     bridgePhaseReadSource,
				err:       err,
			}
		}
		if _, err := destination.Write(protocol, buf[:n]); err != nil {
			return bridgeLoopResult{
				path:      bridgePathPacket,
				direction: direction,
				phase:     bridgePhaseWriteDestination,
				err:       err,
			}
		}
	}
}

func (s *bridgeObservationState) recordOpenRejection(direction string, err error) {
	if s == nil {
		return
	}
	errorClass := bridgeErrorClass(err)
	s.mu.Lock()
	s.observation.OpenRejectionCount++
	if s.observation.FirstOpenRejectionDirection == "" {
		s.observation.FirstOpenRejectionDirection = direction
		s.observation.FirstOpenRejectionClass = errorClass
	}
	s.observation.LastOpenRejectionDirection = direction
	s.observation.LastOpenRejectionClass = errorClass
	if s.observation.CapacityScope == "" {
		if capacity, ok := channelCapacityFromError(err); ok {
			s.observation.CapacityScope = capacity.scope
			s.observation.ActiveChannels = capacity.active
			s.observation.ChannelLimit = capacity.limit
		}
	}
	s.mu.Unlock()
}

func (s *bridgeObservationState) finish(result bridgeLoopResult) BridgeObservation {
	s.mu.Lock()
	s.observation.Path = result.path
	s.observation.Direction = result.direction
	s.observation.Phase = result.phase
	s.observation.ErrorClass = bridgeErrorClass(result.err)
	observation := s.observation
	s.mu.Unlock()
	return observation
}

func bridgeErrorClass(err error) string {
	switch {
	case err == nil:
		return bridgeErrorClassClean
	case errors.Is(err, context.Canceled):
		return bridgeErrorClassContextCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return bridgeErrorClassDeadlineExceeded
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return bridgeErrorClassEOF
	case errors.Is(err, giznet.ErrConnClosed):
		return bridgeErrorClassConnectionClosed
	case errors.Is(err, giznet.ErrServiceMuxClosed):
		return bridgeErrorClassServiceMuxClosed
	case errors.Is(err, net.ErrClosed), errors.Is(err, io.ErrClosedPipe), errors.Is(err, giznet.ErrClosed):
		return bridgeErrorClassClosed
	case errors.Is(err, ErrBufferLimit):
		return bridgeErrorClassBufferLimit
	default:
		return bridgeErrorClassOther
	}
}

func isClosedError(err error) bool {
	return err == nil ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, giznet.ErrClosed) ||
		errors.Is(err, giznet.ErrConnClosed) ||
		errors.Is(err, giznet.ErrServiceMuxClosed)
}
