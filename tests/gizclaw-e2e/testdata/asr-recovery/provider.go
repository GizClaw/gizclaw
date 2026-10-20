// Package peergenx adds a process-owned, loopback-only Doubao protocol fixture
// through the disposable Docker build overlay. Production builds exclude it.
package peergenx

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers/doubaoasr"
	"github.com/gorilla/websocket"
)

var recoveryFixture = sync.OnceValue(func() *httptest.Server {
	var attempts atomic.Int32
	var failedAudio atomic.Pointer[[32]byte]
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		attempt := int(attempts.Add(1))
		var audio []byte
		for {
			_, packet, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if len(packet) < 8 || packet[1]>>4 != 2 {
				continue
			}
			payload := packet[8:]
			if packet[2]&15 == 1 {
				reader, err := gzip.NewReader(bytes.NewReader(payload))
				if err != nil {
					return
				}
				payload, err = io.ReadAll(io.LimitReader(reader, 2<<20))
				_ = reader.Close()
				if err != nil {
					return
				}
			}
			if len(audio)+len(payload) > 2<<20 {
				return
			}
			audio = append(audio, payload...)
			if packet[1]&15 != 2 {
				continue
			}
			if attempt == 8 {
				digest := sha256.Sum256(audio)
				failedAudio.Store(&digest)
				message := []byte("OperatorWrapper Process failed: big asr recv err. rpc timeout: CallWithTimeout: timeout in business code, timeout_config=3s, actual=3.000894888s")
				frame := []byte{0x11, 0xf0, 0, 0}
				frame = binary.BigEndian.AppendUint32(frame, 55000000)
				frame = binary.BigEndian.AppendUint32(frame, uint32(len(message)))
				_ = conn.WriteMessage(websocket.BinaryMessage, append(frame, message...))
				return
			}
			turn := attempt
			if attempt > 8 {
				turn--
			}
			// The replacement must receive exactly the eighth recording once.
			// All nine turns use the same deterministic Opus input fixture.
			if len(audio) == 0 {
				return
			}
			if attempt == 9 {
				digest := sha256.Sum256(audio)
				if original := failedAudio.Load(); original == nil || *original != digest {
					return
				}
				slog.Warn("asr recovery fixture: replay verified", "audio_bytes", len(audio))
			}
			payload, err = json.Marshal(map[string]any{"result": map[string]any{"text": fmt.Sprintf("answer %d", turn), "utterances": []map[string]any{{"text": fmt.Sprintf("answer %d", turn), "start_time": 0, "end_time": len(audio) * 1000 / 32000, "definite": true}}}, "audio_info": map[string]any{"duration": len(audio) * 1000 / 32000}})
			if err != nil {
				return
			}
			frame := []byte{0x11, 0x93, 0x10, 0}
			frame = binary.BigEndian.AppendUint32(frame, ^uint32(0))
			frame = binary.BigEndian.AppendUint32(frame, uint32(len(payload)))
			_ = conn.WriteMessage(websocket.BinaryMessage, append(frame, payload...))
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			return
		}
	}))
})

func newASRRecoveryFixture(cfg TransformerConfig) (genx.Transformer, error) {
	provider := recoveryFixture()
	client := doubaospeech.NewClient("local-fixture", doubaospeech.WithAPIKey("local-fixture"), doubaospeech.WithWebSocketURL("ws"+strings.TrimPrefix(provider.URL, "http")))
	return doubaoasr.New(doubaoasr.Config{Client: client, EmitInterim: cfg.Params["emit_interim"] == true || cfg.Params["emit_interim"] == "true"})
}
