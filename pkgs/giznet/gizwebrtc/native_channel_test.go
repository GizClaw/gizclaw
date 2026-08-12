package gizwebrtc

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/webrtc/v4"
)

func nativeChannelTestPair(t *testing.T) (*Conn, *Conn) {
	return nativeChannelTestPairWithAPI(t, nil)
}

func nativeChannelTestPairWithAPI(t *testing.T, api *webrtc.API) (*Conn, *Conn) {
	t.Helper()
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (&ListenConfig{
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
		API:            api,
	}).Listen(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(listener.SignalingHandler())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	clientListener, client, err := Dial(ctx, clientKey, serverKey.Public, DialConfig{
		SignalingURL:   httpServer.URL + SignalingPath,
		CipherMode:     CipherModePlaintext,
		SecurityPolicy: allowAllPolicy{},
		API:            api,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := acceptConn(t, listener)
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = clientListener.Close()
		_ = listener.Close()
		httpServer.Close()
		cancel()
	})
	return client, server
}

func TestNativeChannelPrefixOwnershipOptionsAndClose(t *testing.T) {
	client, server := nativeChannelTestPair(t)
	accepted := make(chan *NativeChannel, 2)
	unregister, err := server.RegisterNativeChannelHandler("giznet/v2/tunnel/", func(channel *NativeChannel) {
		accepted <- channel
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.RegisterNativeChannelHandler("other/", func(*NativeChannel) {}); err == nil {
		t.Fatal("duplicate native namespace registration succeeded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	zero := uint16(0)
	outbound, err := client.OpenNativeChannel(ctx, "giznet/v2/tunnel/test/packet", NativeChannelOptions{
		Ordered: false, MaxRetransmits: &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	inbound := <-accepted
	if inbound.Label() != outbound.Label() || inbound.Ordered() || inbound.MaxRetransmits() == nil || *inbound.MaxRetransmits() != 0 {
		t.Fatalf("inbound options label=%q ordered=%t retransmits=%v", inbound.Label(), inbound.Ordered(), inbound.MaxRetransmits())
	}
	if _, err := outbound.WriteMessage([]byte("packet")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, err := inbound.ReadMessage(buf)
	if err != nil || string(buf[:n]) != "packet" {
		t.Fatalf("native message = %q, %v", buf[:n], err)
	}
	unregister()
	readDone := make(chan error, 1)
	go func() {
		_, readErr := outbound.ReadMessage(buf)
		readDone <- readErr
	}()
	select {
	case err := <-readDone:
		if err == nil {
			t.Fatal("outbound read succeeded after remote unregister")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("remote unregister did not wake outbound read")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		client.nativeMu.Lock()
		clientChannels := len(client.nativeChannels)
		client.nativeMu.Unlock()
		server.nativeMu.Lock()
		serverChannels := len(server.nativeChannels)
		server.nativeMu.Unlock()
		if clientChannels == 0 && serverChannels == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("native channels after remote reset: client=%d server=%d", clientChannels, serverChannels)
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := server.RegisterNativeChannelHandler("giznet/v2/tunnel/", func(*NativeChannel) {}); err != nil {
		t.Fatalf("register after unregister: %v", err)
	}
}

func TestNativeChannelRejectsInvalidLabel(t *testing.T) {
	client, _ := nativeChannelTestPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, label := range []string{"", string([]byte{'x', 0, 'y'})} {
		if _, err := client.OpenNativeChannel(ctx, label, NativeChannelOptions{Ordered: true}); err == nil {
			t.Fatalf("OpenNativeChannel(%q) succeeded", label)
		}
	}
}

func TestNativeChannelReportsMaxPacketLifeTime(t *testing.T) {
	client, server := nativeChannelTestPair(t)
	accepted := make(chan *NativeChannel, 1)
	if _, err := server.RegisterNativeChannelHandler("giznet/v2/tunnel/", func(channel *NativeChannel) {
		accepted <- channel
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	lifetime := uint16(250)
	outbound, err := client.OpenNativeChannel(ctx, "giznet/v2/tunnel/test/lifetime", NativeChannelOptions{
		Ordered: true, MaxPacketLifeTime: &lifetime,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer outbound.Close()
	inbound := <-accepted
	defer inbound.Close()
	if got := inbound.MaxPacketLifeTime(); got == nil || *got != lifetime {
		t.Fatalf("inbound MaxPacketLifeTime = %v, want %d", got, lifetime)
	}
	if inbound.MaxRetransmits() != nil {
		t.Fatalf("inbound MaxRetransmits = %v, want nil", inbound.MaxRetransmits())
	}
}

func TestNativeChannelDrainsFinalMessageBeforeRemoteClose(t *testing.T) {
	client, server := nativeChannelTestPair(t)
	accepted := make(chan *NativeChannel, 1)
	if _, err := server.RegisterNativeChannelHandler("giznet/v2/tunnel/", func(channel *NativeChannel) {
		accepted <- channel
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outbound, err := client.OpenNativeChannel(ctx, "giznet/v2/tunnel/test/service/1", NativeChannelOptions{Ordered: true})
	if err != nil {
		t.Fatal(err)
	}
	inbound := <-accepted
	first := []byte("final response")
	last := []byte("eos")
	if _, err := inbound.Write(first); err != nil {
		t.Fatal(err)
	}
	if _, err := inbound.Write(last); err != nil {
		t.Fatal(err)
	}
	if err := inbound.Close(); err != nil {
		t.Fatal(err)
	}
	// Let the remote-close callback run before the reader drains the final
	// messages. Closing detached state from that callback loses these frames.
	time.Sleep(100 * time.Millisecond)
	want := append(first, last...)
	got := make([]byte, len(want))
	if _, err := io.ReadFull(outbound, got); err != nil {
		t.Fatalf("read final message: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("final message = %q, want %q", got, want)
	}
}

func TestNativeChannelReadDeadlineDoesNotCloseChannel(t *testing.T) {
	client, server := nativeChannelTestPair(t)
	accepted := make(chan *NativeChannel, 1)
	if _, err := server.RegisterNativeChannelHandler("giznet/v2/tunnel/", func(channel *NativeChannel) {
		accepted <- channel
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outbound, err := client.OpenNativeChannel(ctx, "giznet/v2/tunnel/test/service/3", NativeChannelOptions{Ordered: true})
	if err != nil {
		t.Fatal(err)
	}
	inbound := <-accepted
	if err := inbound.SetReadDeadline(time.Now().Add(10 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := inbound.Read(make([]byte, 1)); err == nil {
		t.Fatal("read before deadline succeeded")
	}
	if err := inbound.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	want := []byte("still open")
	if _, err := outbound.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(inbound, got); err != nil {
		t.Fatalf("read after clearing deadline: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestNativeChannelShortBufferDoesNotCloseChannel(t *testing.T) {
	client, server := nativeChannelTestPair(t)
	accepted := make(chan *NativeChannel, 1)
	if _, err := server.RegisterNativeChannelHandler("giznet/v2/tunnel/", func(channel *NativeChannel) {
		accepted <- channel
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	zero := uint16(0)
	outbound, err := client.OpenNativeChannel(ctx, "giznet/v2/tunnel/test/packet", NativeChannelOptions{MaxRetransmits: &zero})
	if err != nil {
		t.Fatal(err)
	}
	inbound := <-accepted
	want := []byte("packet larger than the first receive buffer")
	if _, err := outbound.WriteMessage(want); err != nil {
		t.Fatal(err)
	}
	if _, err := inbound.ReadMessage(make([]byte, 1)); !errors.Is(err, io.ErrShortBuffer) {
		t.Fatalf("short read error = %v, want %v", err, io.ErrShortBuffer)
	}
	got := make([]byte, len(want))
	n, err := inbound.ReadMessage(got)
	if err != nil {
		t.Fatalf("retry message read: %v", err)
	}
	if string(got[:n]) != string(want) {
		t.Fatalf("message = %q, want %q", got[:n], want)
	}
}

func TestNormalizeNativeChannelTerminalError(t *testing.T) {
	abortErr := errors.New("sctp association aborted")
	if err := normalizeNativeChannelTerminalError(abortErr); !errors.Is(err, giznet.ErrConnClosed) || !errors.Is(err, abortErr) {
		t.Fatalf("normalized abort = %v", err)
	}
	for _, err := range []error{io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe} {
		normalized := normalizeNativeChannelTerminalError(err)
		if !errors.Is(normalized, err) || errors.Is(normalized, giznet.ErrConnClosed) {
			t.Fatalf("normalized %v = %v", err, normalized)
		}
	}
}
