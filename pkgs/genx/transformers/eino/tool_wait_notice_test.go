package eino

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/agentkit/audiodock"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/internal/streamkit"
	"github.com/cloudwego/eino/schema"
)

type noticeTestTTS struct{}

func (noticeTestTTS) Transform(ctx context.Context, _ string, input genx.Stream) (genx.Stream, error) {
	return streamkit.NewTTSStream(ctx, input, streamkit.OutputConfig{}, "audio/opus",
		func(_ context.Context, text string, _ streamkit.TTSMeta, _ string, emit func([]byte) error) error {
			if strings.TrimSpace(text) == "我先找找。" {
				return emit([]byte{1})
			}
			return emit([]byte{2})
		}), nil
}

type noticeTestInvoker struct {
	*einoToolInvoker
	alias string
}

func (invoker noticeTestInvoker) ResolveTools(ctx context.Context) ([]genx.ToolDefinition, error) {
	definitions, err := invoker.einoToolInvoker.ResolveTools(ctx)
	if err == nil {
		definitions[0].Alias = invoker.alias
	}
	return definitions, err
}

func TestToolWaitNotice(t *testing.T) {
	const alias = "litelink.call_tool.search_catalog"
	const cue = "我先找找。 "
	for _, scenario := range []struct {
		name   string
		alias  string
		wait   time.Duration
		verify bool
		want   bool
	}{
		{name: "quick search", alias: alias, wait: 100 * time.Millisecond},
		{name: "slow search", alias: alias, wait: 2 * time.Second, want: true},
		{name: "unrelated tool", alias: "music.play", wait: 2 * time.Second},
		{name: "verified slow search", alias: alias, wait: 2 * time.Second, want: true, verify: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				searches := 0
				var firstSearchReturned atomic.Bool
				invoker := noticeTestInvoker{alias: scenario.alias, einoToolInvoker: &einoToolInvoker{
					invoke: func(value string) (any, error) {
						searches++
						time.Sleep(scenario.wait)
						if searches == 1 {
							firstSearchReturned.Store(true)
						}
						return map[string]string{"found": value}, nil
					},
				}}
				search := func(id string) *schema.Message {
					return schema.AssistantMessage("", []schema.ToolCall{
						{ID: id, Type: "function", Function: schema.FunctionCall{Name: "lookup", Arguments: `{"value":"小鸟飞飞"}`}},
					})
				}
				chat := &scriptedChatModel{rounds: [][]*schema.Message{
					{search("first")}, {search("second")}, {schema.AssistantMessage("找到了。", nil)},
				}}
				config := chatConfig(&componentMapResolver{chat: chat})
				config.ToolInvoker = invoker
				for _, node := range config.Graph.Nodes {
					if node.ChatModel != nil {
						node.ChatModel.ToolWaitNotice = &ToolWaitNotice{
							ToolAlias: alias, Delay: time.Second, Messages: []string{strings.TrimSpace(cue)},
						}
					}
				}
				if scenario.verify {
					config.VerifyToolResponse = func(_ context.Context, _ genx.ToolConversation, reply string) (string, error) {
						if reply != "找到了。" {
							t.Errorf("verified reply = %q", reply)
						}
						return "", nil
					}
				}
				transformer, err := New(t.Context(), config)
				if err != nil {
					t.Fatal(err)
				}
				defer transformer.Close()
				dock, err := audiodock.New(audiodock.Config{
					Agent: transformer, TTS: noticeTestTTS{},
					ResolveVoice: func(context.Context, audiodock.VoiceRequest) (string, error) { return "voice/test", nil },
				})
				if err != nil {
					t.Fatal(err)
				}
				output, err := dock.Transform(t.Context(), textInput("播放小鸟飞飞"))
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				firstNotice, cueAudio, final := 0, 0, false
				for {
					chunk, err := output.Next()
					if err != nil {
						break
					}
					switch part := chunk.Part.(type) {
					case genx.Text:
						if part == genx.Text(cue) {
							firstNotice++
							synctest.Wait()
							if firstSearchReturned.Load() {
								t.Error("notice arrived after the search returned")
							}
						}
						final = final || part == genx.Text("找到了。")
					case *genx.Blob:
						if len(part.Data) > 0 && part.Data[0] == 1 {
							cueAudio++
							synctest.Wait()
							if firstSearchReturned.Load() {
								t.Error("notice audio arrived after the search returned")
							}
						}
					}
				}
				wantNotices := 0
				if scenario.want {
					wantNotices = 1
				}
				if !final || searches != 2 || firstNotice != wantNotices || cueAudio != wantNotices {
					t.Fatalf("final=%t, tool calls=%d, notices=%d, audio=%d", final, searches, firstNotice, cueAudio)
				}
			})
		})
	}
}

func TestToolWaitNoticeCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		stop := scheduleToolWaitNotice(ctx, time.Second, func() error {
			t.Error("notice ran after cancellation")
			return nil
		})
		cancel()
		time.Sleep(2 * time.Second)
		if sent, err := stop(); sent || err != nil {
			t.Fatalf("sent=%t err=%v", sent, err)
		}
	})
}
