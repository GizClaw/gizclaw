package giztunnel

import (
	"errors"
	"io"
	"net"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// Bridge transparently forwards service streams and packets between two Giznet
// connections until either side closes.
func Bridge(left, right giznet.Conn) error {
	if enabler, ok := left.(giznet.ServiceAcceptEnabler); ok {
		enabler.EnableServiceAccept()
	}
	if enabler, ok := right.(giznet.ServiceAcceptEnabler); ok {
		enabler.EnableServiceAccept()
	}
	leftServices, ok := left.(giznet.ServiceAcceptor)
	if !ok {
		return errors.New("giztunnel: left connection cannot accept aggregate services")
	}
	rightServices, ok := right.(giznet.ServiceAcceptor)
	if !ok {
		return errors.New("giztunnel: right connection cannot accept aggregate services")
	}
	errCh := make(chan error, 4)
	go func() { errCh <- bridgeServices(leftServices, right) }()
	go func() { errCh <- bridgeServices(rightServices, left) }()
	go func() { errCh <- bridgePackets(left, right) }()
	go func() { errCh <- bridgePackets(right, left) }()
	err := <-errCh
	_ = left.Close()
	_ = right.Close()
	if isClosedError(err) {
		return nil
	}
	return err
}

func bridgeServices(source giznet.ServiceAcceptor, destination giznet.Conn) error {
	for {
		service, sourceStream, err := source.AcceptService()
		if err != nil {
			return err
		}
		destinationStream, err := destination.Dial(service)
		if err != nil {
			_ = sourceStream.Close()
			return err
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
	_ = left.Close()
	_ = right.Close()
}

func bridgePackets(source, destination giznet.Conn) error {
	buf := make([]byte, 64*1024)
	for {
		protocol, n, err := source.Read(buf)
		if err != nil {
			return err
		}
		if _, err := destination.Write(protocol, buf[:n]); err != nil {
			return err
		}
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
