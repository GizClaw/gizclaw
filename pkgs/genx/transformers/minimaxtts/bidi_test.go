package minimaxtts

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/GizClaw/minimax-go"
	"github.com/coder/websocket"
)

func newBidiTransformerServer(t *testing.T, requests chan<- string, connections *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/v1/t2a_v2_bidi" {
			t.Errorf("unexpected bidi path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		connections.Add(1)
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		audio := hex.EncodeToString(make([]byte, 640))
		write := func(text string) bool { return conn.Write(r.Context(), websocket.MessageText, []byte(text)) == nil }
		if !write(`{"event":"connected_success"}`) {
			return
		}
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			var event struct{ Event, Text string }
			if err := json.Unmarshal(data, &event); err != nil {
				t.Error(err)
				return
			}
			switch event.Event {
			case "task_start":
				if !write(`{"event":"task_started"}`) {
					return
				}
			case "task_continue":
				requests <- event.Text
				if !write(`{"event":"sentence_start"}`) ||
					!write(fmt.Sprintf(`{"event":"task_continued","data":{"audio":"%s"},"is_final":true,"extra_info":{"usage_characters":37}}`, audio)) ||
					!write(`{"event":"sentence_end"}`) {
					return
				}
			case "task_flush":
				if !write(`{"event":"task_flushed"}`) {
					return
				}
			case "task_finish":
				write(`{"event":"task_finished"}`)
				return
			default:
				t.Errorf("unexpected bidi event: %s", event.Event)
				return
			}
		}
	}))
}

func TestBidiStreamsIncompleteInputAndKeepsFinalAudioEventsOpen(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	requests := make(chan string, 8)
	var connections atomic.Int32
	server := newBidiTransformerServer(t, requests, &connections)
	defer server.Close()
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	transformer, err := New(Config{Transport: TransportBidi, Client: client, Model: "speech-2.6-turbo", VoiceID: "voice", Format: "pcm"})
	if err != nil {
		t.Fatal(err)
	}
	if transformer.transport != TransportBidi {
		t.Fatal("explicit bidi transport was not selected")
	}
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 1)
	output, err := transformer.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	add := func(text, message string, end bool) {
		t.Helper()
		if err := input.Add(&genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{StreamID: "route", MessageID: message, MessageEnd: end}}); err != nil {
			t.Fatal(err)
		}
	}
	add("1,", "first", false)
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatal(err)
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			break
		}
	}
	if got := <-requests; got != "1," {
		t.Fatalf("client waited for or changed a segment: %q", got)
	}
	add("000元。他说：“Hello", "first", false)
	add(" world。”", "first", true)
	add("第二轮", "second", false)
	if err := input.Done(genx.Usage{}); err != nil {
		t.Fatal(err)
	}
	var first, second int
	for {
		chunk, err := output.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && len(blob.Data) > 0 {
			switch chunk.Ctrl.MessageID {
			case "first":
				first++
			case "second":
				second++
			default:
				t.Fatalf("audio has wrong message identity: %+v", chunk.Ctrl)
			}
		}
	}
	if connections.Load() != 1 || first == 0 || second != 1 {
		t.Fatalf("connections=%d first=%d second=%d", connections.Load(), first, second)
	}
}

func TestBidiOggFlushSealsEachMessageWithoutReconnecting(t *testing.T) {
	requests := make(chan string, 4)
	var connections atomic.Int32
	server := newBidiTransformerServer(t, requests, &connections)
	defer server.Close()
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	transformer, err := New(Config{Transport: TransportBidi, Client: client, Model: "speech-model", VoiceID: "voice", Format: FormatOggOpus})
	if err != nil {
		t.Fatal(err)
	}
	input := &configTestStream{chunks: []*genx.MessageChunk{
		{Part: genx.Text("第一轮"), Ctrl: &genx.StreamCtrl{StreamID: "route", MessageID: "first", MessageEnd: true}},
		{Part: genx.Text("第二轮"), Ctrl: &genx.StreamCtrl{StreamID: "route", MessageID: "second", EndOfStream: true}},
	}}
	output, err := transformer.Transform(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var audio bytes.Buffer
	for {
		chunk, err := output.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok {
			audio.Write(blob.Data)
		}
	}
	bos, eos := 0, 0
	data := audio.Bytes()
	for len(data) > 0 {
		if len(data) < 27 || string(data[:4]) != "OggS" {
			t.Fatal("invalid Ogg page")
		}
		count := int(data[26])
		size := 27 + count
		for _, lace := range data[27:size] {
			size += int(lace)
		}
		if data[5]&2 != 0 {
			bos++
		}
		if data[5]&4 != 0 {
			eos++
		}
		data = data[size:]
	}
	if bos != 2 || eos != 2 || connections.Load() != 1 {
		t.Fatalf("logical streams BOS=%d EOS=%d connections=%d", bos, eos, connections.Load())
	}
}

func TestBidiCloseCancelsBlockedDeliveryBeforeJoiningReader(t *testing.T) {
	requests := make(chan string, 1)
	var connections atomic.Int32
	server := newBidiTransformerServer(t, requests, &connections)
	defer server.Close()
	client, err := minimax.NewClient(minimax.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	transformer, err := New(Config{Transport: TransportBidi, Client: client, Model: "speech-model", VoiceID: "voice", Format: "pcm"})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	session, err := transformer.openBidi(t.Context(), streamkit.TTSMeta{}, func(ctx context.Context, _ []byte) error {
		entered <- struct{}{}
		<-ctx.Done()
		return context.Cause(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := session.SendText(t.Context(), "正文"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("provider did not reach delivery")
	}
	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close joined the reader before cancelling blocked delivery")
	}
}
