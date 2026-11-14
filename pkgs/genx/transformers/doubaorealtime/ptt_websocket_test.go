package doubaorealtime

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	speech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/gorilla/websocket"
)

// pttWireProvider speaks the actual Dialogue binary protocol to the pinned SDK.
// It only uses loopback, synthetic PCM and dummy credentials; no cloud service.
type pttWireProvider struct {
	t                *testing.T
	mu               sync.Mutex
	connections      int
	turns            int
	audioBytes       int
	fault            string
	server           *httptest.Server
	replacementReady chan struct{}
}

func newPTTWireProvider(t *testing.T, fault string) *pttWireProvider {
	p := &pttWireProvider{t: t, fault: fault, replacementReady: make(chan struct{})}
	p.server = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.server.Close)
	return p
}

func (p *pttWireProvider) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
	if err != nil {
		p.t.Error(err)
		return
	}
	defer conn.Close()
	p.mu.Lock()
	p.connections++
	connection := p.connections
	p.mu.Unlock()
	stalled := false
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		event, session, payload, err := parsePTTClientFrame(raw)
		if err != nil {
			p.t.Error(err)
			return
		}
		send := func(event int, payload string) bool {
			return conn.WriteMessage(websocket.BinaryMessage, pttServerFrame(event, session, []byte(payload))) == nil
		}
		switch event {
		case 1:
			if !send(50, `{}`) {
				return
			}
		case 100:
			if !send(150, `{"dialog_id":"local-docker-dialog"}`) {
				return
			}
			if connection == 2 {
				close(p.replacementReady)
			}
		case 200:
			p.mu.Lock()
			p.audioBytes += len(payload)
			turns := p.turns
			p.mu.Unlock()
			if connection == 1 && p.fault == "capture599" && turns == 1 && !stalled {
				p.t.Log("INJECT: 599 after first frame while input is still Capturing")
				if !send(599, `{"status_code":"MODEL_BUSY","message":"local capture failure"}`) {
					return
				}
				stalled = true
			}
		case 400:
			p.mu.Lock()
			p.turns++
			turn := p.turns
			bytes := p.audioBytes
			p.mu.Unlock()
			p.t.Logf("wire connection=%d EndASR turn=%d cumulative_received_audio_bytes=%d", connection, turn, bytes)
			if stalled {
				continue
			}
			asr := fmt.Sprintf(`{"text":"synthetic turn %d","question_id":"q%d"}`, turn, turn)
			if !send(451, asr) {
				return
			}
			if turn == 2 && (p.fault == "missing459" || p.fault == "closeEventAttempt") {
				p.t.Log("INJECT: permanently omit turn 2 ASREnded and response; later turns return their own 451/459/reply")
				continue
			}
			if !send(459, asr) {
				return
			}
			if turn == 2 && strings.HasPrefix(p.fault, "599") {
				p.t.Log("INJECT: event599 MODEL_BUSY; this connection then accepts audio without recognition/reply")
				if !send(599, `{"status_code":"MODEL_BUSY","message":"local injected model failure"}`) {
					return
				}
				stalled = true
				continue
			}
			identity := fmt.Sprintf(`"question_id":"q%d","reply_id":"r%d"`, turn, turn)
			if !send(350, fmt.Sprintf(`{"text":"answer %d",%s}`, turn, identity)) ||
				!send(550, fmt.Sprintf(`{"content":"answer %d",%s}`, turn, identity)) ||
				!send(559, `{`+identity+`}`) || !send(359, `{`+identity+`}`) {
				return
			}
		case 102:
			if connection == 1 && p.fault == "closeEventAttempt" {
				// Attempt old events after the finish request. The SDK may close
				// the socket before reading them; this case does not prove delivery
				// or filtering of already-received events.
				if !send(451, `{"text":"late old transcript","question_id":"q2"}`) ||
					!send(459, `{"question_id":"q2"}`) ||
					!send(350, `{"text":"late old answer","question_id":"q2","reply_id":"r2"}`) {
					return
				}
			}
			if !send(152, `{}`) {
				return
			}
		case 2:
			return
		case 515:
			// Interruption cannot manufacture the permanently missing 459.
		default:
			p.t.Errorf("unexpected client event %d", event)
			return
		}
	}
}

func parsePTTClientFrame(raw []byte) (event int, session string, payload []byte, err error) {
	if len(raw) < 12 || raw[0] != 0x11 || raw[1]&0xf != 4 || raw[2]&0xf != 0 {
		return 0, "", nil, fmt.Errorf("unsupported local client frame")
	}
	event = int(binary.BigEndian.Uint32(raw[4:8]))
	raw = raw[8:]
	field := func() ([]byte, error) {
		if len(raw) < 4 {
			return nil, io.ErrUnexpectedEOF
		}
		n := int(binary.BigEndian.Uint32(raw[:4]))
		raw = raw[4:]
		if n > len(raw) {
			return nil, io.ErrUnexpectedEOF
		}
		value := raw[:n]
		raw = raw[n:]
		return value, nil
	}
	if event != 1 && event != 2 {
		id, e := field()
		if e != nil {
			return 0, "", nil, e
		}
		session = string(id)
	}
	payload, err = field()
	return
}

func pttServerFrame(event int, session string, payload []byte) []byte {
	raw := []byte{0x11, 0x94, 0x10, 0}
	raw = binary.BigEndian.AppendUint32(raw, uint32(event))
	if event == 50 {
		session = "local-connect"
	}
	raw = binary.BigEndian.AppendUint32(raw, uint32(len(session)))
	raw = append(raw, session...)
	raw = binary.BigEndian.AppendUint32(raw, uint32(len(payload)))
	return append(raw, payload...)
}

func TestPTTWebSocketRecovery(t *testing.T) {
	for _, fault := range []string{"none", "missing459", "599", "closeEventAttempt", "599TrailingEOS"} {
		t.Run(fault, func(t *testing.T) {
			p := newPTTWireProvider(t, fault)
			client := speech.NewClient("local-only", speech.WithAPIKey("local-only"), speech.WithWebSocketURL("ws"+strings.TrimPrefix(p.server.URL, "http")))
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			tr := newTransformer(client, withMode(ModePushToTalk), withModel("2.2.0.0"), withInputFormat("pcm"), withInputTranscode(false), withFormat("pcm"))
			input := newBufferStream(64, ctx)
			defer input.Close()
			output, err := tr.Transform(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			chunks := make(chan *genx.MessageChunk, 256)
			outputError := make(chan error, 1)
			go func() {
				defer close(chunks)
				for {
					chunk, err := output.Next()
					if err != nil {
						outputError <- err
						return
					}
					chunks <- chunk
				}
			}()
			for turn := 1; turn <= 5; turn++ {
				id := fmt.Sprintf("turn-%d", turn)
				for _, chunk := range []*genx.MessageChunk{
					{Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}},
					{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 640)}, Ctrl: &genx.StreamCtrl{StreamID: id}},
					{Part: &genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 640)}, Ctrl: &genx.StreamCtrl{StreamID: id}},
					{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}},
					{Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}},
				} {
					if turn == 2 && fault == "599TrailingEOS" && chunk.Part == nil && chunk.IsEndOfStream() {
						// Deliver the containing route EOS only after observing the
						// failure/replacement. It must not fail the new session.
						continue
					}
					if err := input.Push(chunk); err != nil {
						t.Fatal(err)
					}
				}
				transcript, answer, recorded := "", "", 0
				finished, textEnded, audioEnded := false, false, false
				timer := time.NewTimer(500 * time.Millisecond)
			observe:
				for {
					select {
					case chunk, ok := <-chunks:
						if !ok {
							t.Fatalf("output ended before all turns: %v", <-outputError)
						}
						if chunk.Ctrl == nil || chunk.Ctrl.StreamID != id && chunk.Ctrl.SourceStreamID != id {
							continue
						}
						if text, ok := chunk.Part.(genx.Text); ok {
							if chunk.Role == genx.RoleUser {
								transcript += string(text)
							}
							if chunk.Role == genx.RoleModel {
								answer += string(text)
							}
						}
						if blob, ok := chunk.Part.(*genx.Blob); ok && chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
							recorded += len(blob.Data)
						}
						if chunk.Role == genx.RoleModel && chunk.Ctrl.EndOfStream {
							switch chunk.Part.(type) {
							case genx.Text:
								textEnded = true
							case *genx.Blob:
								audioEnded = true
							}
							if answer != "" && textEnded && audioEnded {
								finished = true
								break observe
							}
						}
					case <-timer.C:
						break observe
					}
				}
				timer.Stop()
				p.mu.Lock()
				connections, bytes := p.connections, p.audioBytes
				p.mu.Unlock()
				t.Logf("OBSERVE turn=%d transcript=%q answer=%q history_audio_bytes=%d wire_audio_bytes=%d provider_connections=%d completed=%t", turn, transcript, answer, recorded, bytes, connections, finished)
				if recorded != 1280 {
					t.Errorf("turn %d recorded bytes=%d, want 1280", turn, recorded)
				}
				if turn == 2 && fault != "none" {
					if fault == "599TrailingEOS" {
						if err := input.Push(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}}); err != nil {
							t.Fatal(err)
						}
					}
					if answer != "" {
						t.Errorf("faulted turn unexpectedly produced answer %q", answer)
					}
					continue
				}
				if transcript != fmt.Sprintf("synthetic turn %d", turn) || answer != fmt.Sprintf("answer %d", turn) || !finished {
					t.Errorf("turn %d did not recover: transcript=%q answer=%q", turn, transcript, answer)
				}
			}
			p.mu.Lock()
			connections, bytes, turns := p.connections, p.audioBytes, p.turns
			p.mu.Unlock()
			wantConnections := 2
			if fault == "none" {
				wantConnections = 1
			}
			if connections != wantConnections || bytes != 6400 || turns != 5 {
				t.Errorf("connections=%d (want %d), received bytes=%d (want 6400), EndASR=%d (want 5); recovery must not replay audio", connections, wantConnections, bytes, turns)
			}
		})
	}
}
