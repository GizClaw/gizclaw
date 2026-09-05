package doubaoast

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/gorilla/websocket"
)

type observedSDKSession struct {
	doubaoASTTranslateSession
	received chan struct{}
}

func (s observedSDKSession) Recv() iter.Seq2[*doubaospeech.ASTTranslateEvent, error] {
	return func(yield func(*doubaospeech.ASTTranslateEvent, error) bool) {
		defer close(s.received)
		for event, err := range s.doubaoASTTranslateSession.Recv() {
			terminal := err != nil || event != nil && event.Type == doubaospeech.ASTEventSessionFailed
			more := yield(event, err)
			if terminal {
				return
			}
			if !more {
				return
			}
		}
	}
}

func TestSDKSessionFailureIsVisible(t *testing.T) {
	for _, mode := range []InputMode{InputModeRealtime, InputModePushToTalk} {
		for _, code := range []int{0, websocket.CloseNormalClosure, websocket.CloseGoingAway} {
			t.Run(fmt.Sprintf("%s/close_%d", mode, code), func(t *testing.T) {
				testSDKSessionFailureIsVisible(t, mode, code)
			})
		}
	}
}

func testSDKSessionFailureIsVisible(t *testing.T, mode InputMode, closeCode int) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
		// AST protobuf response field 2: SessionStarted (150).
		if err = conn.WriteMessage(websocket.BinaryMessage, []byte{0x10, 0x96, 0x01}); err != nil {
			return
		}
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
		if closeCode != 0 {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(closeCode, ""), time.Now().Add(time.Second))
			return
		}
		// AST protobuf response field 2: SessionFailed (153).
		if err = conn.WriteMessage(websocket.BinaryMessage, []byte{0x10, 0x99, 0x01}); err != nil {
			return
		}
		for {
			if _, _, err = conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	ctx := t.Context()
	input := newBufferStream(4)
	client := doubaospeech.NewClient("test", doubaospeech.WithWebSocketURL("ws"+strings.TrimPrefix(server.URL, "http")))
	tr := newTransformer(client, withInputMode(mode), withRealtimePacing(false))
	received := make(chan struct{})
	opened := make(chan doubaoASTTranslateSession, 1)
	tr.newSession = func(ctx context.Context, cfg doubaospeech.ASTTranslateConfig) (doubaoASTTranslateSession, error) {
		s, err := client.ASTTranslate.OpenSession(ctx, &cfg)
		if err != nil {
			return nil, err
		}
		opened <- s
		return observedSDKSession{s, received}, nil
	}
	out, err := tr.transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer out.CloseWithError(context.Canceled)
	defer input.CloseWithError(context.Canceled)
	_ = input.Push(genx.NewBeginOfStream("sdk-error"))
	_ = input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "sdk-error"}})
	select {
	case s := <-opened:
		defer s.Close()
	case <-time.After(time.Second):
		t.Fatal("SDK session not opened")
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("SDK receiver did not finish")
	}
	result := make(chan error, 1)
	go func() { _, err := out.Next(); result <- err }()
	select {
	case err := <-result:
		var providerErr *doubaospeech.Error
		if closeCode != 0 {
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("expected incomplete session error, got %v", err)
			}
		} else if !errors.As(err, &providerErr) {
			t.Fatalf("expected SDK provider error, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("real SDK stopped receiving but Runtime output remained blocked")
	}
}
