package giztunnel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestOpusOverflowKeepsLogicalControlAndServices(t *testing.T) {
	pair := newTunnelPair(t, Config{}, Config{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	payload := []byte{1, 2, 3, 4}
	frame, err := encodeOpusPacket(pair.declaration.SessionID, payload)
	if err != nil {
		t.Fatal(err)
	}
	for range cap(pair.server.readCh) + 64 {
		if err := pair.serverRouter.HandlePacket(frame); err != nil {
			t.Fatalf("unreliable audio overflow closed the logical session: %v", err)
		}
	}
	if err := pair.server.validate(); err != nil {
		t.Fatalf("control closed under audio pressure: %v", err)
	}
	const service = 77
	listener := pair.server.ListenService(service)
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		stream, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer stream.Close()
		deadline, _ := ctx.Deadline()
		_ = stream.SetDeadline(deadline)
		buffer := make([]byte, 4)
		if _, err = io.ReadFull(stream, buffer); err == nil {
			_, err = stream.Write(buffer)
		}
		result <- err
	}()
	stream, err := pair.edge.DialContext(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	deadline, _ := ctx.Deadline()
	_ = stream.SetDeadline(deadline)
	if _, err = stream.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if _, err = io.ReadFull(stream, buffer); err != nil || string(buffer) != "ping" {
		t.Fatalf("service reply=%q err=%v", buffer, err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	// Once the application resumes, queued audio and later audio remain readable.
	audio := make([]byte, 16)
	protocol, n, err := pair.server.Read(audio)
	if err != nil || protocol != giznet.ProtocolOpusPacket || !bytes.Equal(audio[:n], payload) {
		t.Fatalf("resumed audio protocol=%x payload=%v err=%v", protocol, audio[:n], err)
	}
	fresh := []byte("fresh")
	frame, err = encodeOpusPacket(pair.declaration.SessionID, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if err := pair.serverRouter.HandlePacket(frame); err != nil {
		t.Fatal(err)
	}
	for range cap(pair.server.readCh) - 1 {
		if _, _, err := pair.server.Read(audio); err != nil {
			t.Fatal(err)
		}
	}
	protocol, n, err = pair.server.Read(audio)
	if err != nil || protocol != giznet.ProtocolOpusPacket || !bytes.Equal(audio[:n], fresh) {
		t.Fatalf("fresh audio protocol=%x payload=%v err=%v", protocol, audio[:n], err)
	}
}

func TestDirectPacketOverflowRetainsItsFailureBoundary(t *testing.T) {
	pair := newMemTunnelPair(t, Config{}, Config{})
	for range cap(pair.server.readCh) {
		if err := pair.server.enqueuePacket(0x44, []byte("direct")); err != nil {
			t.Fatal(err)
		}
	}
	if err := pair.server.enqueuePacket(0x44, []byte("excess")); !errors.Is(err, ErrBufferLimit) {
		t.Fatalf("direct overflow error=%v", err)
	}
	if err := pair.server.validate(); !errors.Is(err, ErrBufferLimit) {
		t.Fatalf("direct overflow did not retire its session: %v", err)
	}
}
