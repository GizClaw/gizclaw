package doubaorealtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	speech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type liveProbeOpener struct {
	client *speech.Client
	mu     sync.Mutex
	opens  int
}

func (o *liveProbeOpener) OpenSession(ctx context.Context, cfg *speech.RealtimeConfig) (doubaoRealtimeSession, error) {
	o.mu.Lock()
	o.opens++
	o.mu.Unlock()
	return o.client.Realtime.Connect(ctx, cfg)
}

func (o *liveProbeOpener) count() int { o.mu.Lock(); defer o.mu.Unlock(); return o.opens }

func TestLiveTextMultiTurnProbe(t *testing.T) {
	if os.Getenv("DOUBAO_REALTIME_LIVE_PROBE") != "1" {
		t.Skip("set DOUBAO_REALTIME_LIVE_PROBE=1 with credentials and PCM path")
	}
	var inputs [][]byte
	for _, key := range []string{"DOUBAO_REALTIME_PROBE_PCM_1", "DOUBAO_REALTIME_PROBE_PCM_2", "DOUBAO_REALTIME_PROBE_PCM_3"} {
		path := os.Getenv(key)
		if path == "" {
			t.Fatalf("%s is required", key)
		}
		pcm, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, pcm)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
	defer cancel()
	client := speech.NewClient(os.Getenv("GIZCLAW_VOLC_SPEECH_APP_ID"), speech.WithAPIKey(os.Getenv("GIZCLAW_VOLC_SPEECH_API_KEY")), speech.WithResourceID(speech.ResourceRealtime))
	opener := &liveProbeOpener{client: client}
	enabled := true
	extra := speech.RealtimeDialogExtra{EnableVolcWebsearch: &enabled, VolcWebsearchType: "web", VolcWebsearchAPIKey: os.Getenv("GIZCLAW_VOLC_SEARCH_API_KEY"), VolcWebsearchResultCount: 3, VolcWebsearchNoResultMessage: "没有找到相关搜索结果。"}
	tr := newTransformer(client, withDoubaoRealtimeOpener(opener), withMode(ModePushToTalk), withOutput(OutputText), withModel("2.2.0.0"), withInstructions("你是简洁的中文助手。记住用户提供的暗号，并在询问时准确复述。"), withSpeaker("ICL_uranus_zh_male_lingyunqingnian_tob"), withInputFormat("pcm"), withInputSampleRate(16000), withInputChannels(1), withInputTranscode(false), withDialogExtra(extra), withFormat("pcm"))
	input := newBufferStream(1024, ctx)
	defer input.Close()
	output, err := tr.Transform(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	for turn, pcm := range inputs {
		turn++
		id := fmt.Sprintf("turn-%d", turn)
		for _, chunk := range []*genx.MessageChunk{{Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}, {Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: id, BeginOfStream: true}}} {
			if err := input.Push(chunk); err != nil {
				t.Fatal(err)
			}
		}
		for off := 0; off < len(pcm); off += 640 {
			end := min(off+640, len(pcm))
			if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: pcm[off:end]}, Ctrl: &genx.StreamCtrl{StreamID: id}}); err != nil {
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		for _, chunk := range []*genx.MessageChunk{{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}}, {Ctrl: &genx.StreamCtrl{StreamID: id, EndOfStream: true}}} {
			if err := input.Push(chunk); err != nil {
				t.Fatal(err)
			}
		}
		timer := time.AfterFunc(30*time.Second, func() { _ = output.Close() })
		defer timer.Stop()
		events := 0
		transcript := ""
		answer := ""
		finished := false
		for !finished {
			chunk, err := output.Next()
			if err != nil {
				if err == io.EOF {
					t.Fatalf("turn %d EOF events=%d transcript=%q answer=%q", turn, events, transcript, answer)
				}
				t.Fatalf("turn %d output: %v", turn, err)
			}
			events++
			if chunk == nil {
				continue
			}
			if chunk.Ctrl != nil && chunk.Ctrl.EndOfStream {
				t.Logf("turn=%d eos role=%q stream=%q label=%q", turn, chunk.Role, chunk.Ctrl.StreamID, chunk.Ctrl.Label)
			}
			if txt, ok := chunk.Part.(genx.Text); ok {
				if chunk.Role == genx.RoleUser {
					transcript += string(txt)
				} else if chunk.Role == genx.RoleModel {
					answer += string(txt)
				}
			}
			if chunk.Ctrl != nil && chunk.Role == genx.RoleModel && chunk.Ctrl.Label == doubaoRealtimeAssistantLabel && chunk.Ctrl.EndOfStream {
				_, finished = chunk.Part.(genx.Text)
			}
		}
		timer.Stop()
		t.Logf("turn=%d events=%d first_transcript=%q first_text=%q provider_sessions=%d", turn, events, transcript, answer, opener.count())
		if turn == 3 && !strings.Contains(answer, "蓝色灯塔") {
			t.Fatalf("third-turn memory missing: %q", answer)
		}
	}
	if opener.count() != 1 {
		t.Fatalf("provider sessions=%d, want 1", opener.count())
	}
}
