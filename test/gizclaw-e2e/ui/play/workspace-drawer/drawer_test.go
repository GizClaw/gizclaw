//go:build gizclaw_e2e

// User story: As a Play UI user, I can inspect and test the active workspace
// from the Workspace drawer without using the global OpenAI drawer.
package playui_test

import (
	"encoding/json"
	"fmt"
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
)

func playWorkspaceDrawerStories() []Story {
	return []Story{
		{
			Name: "204-play-workspace-drawer",
			Run: func(t testing.TB, page *Page) {
				page.GotoPlay("/")
				page.ClickRole("button", "Workspace")
				page.ExpectText("Inspect and test the current peer run active workspace.")
				page.ClickRole("button", "Workspace")
				page.ExpectNoText("Inspect and test the current peer run active workspace.")
				page.ClickRole("button", "Workspace")
				page.ExpectText("Inspect and test the current peer run active workspace.")
				page.ClickRole("button", "OpenAI")
				page.ExpectText("Send requests to this gateway through the OpenAI-compatible chat completions endpoint.")
				page.ExpectNoText("Inspect and test the current peer run active workspace.")
				page.ClickRole("button", "OpenAI")
				page.ExpectNoText("Send requests to this gateway through the OpenAI-compatible chat completions endpoint.")
				page.ClickRole("button", "Workspace")
				page.ExpectText("Inspect and test the current peer run active workspace.")
				page.ExpectText(SeedWorkspaceName)
				page.ExpectText("flowcraft")
				page.ExpectText("active")
				page.ExpectText("Selected")
				page.ExpectText("Pending")
				page.ExpectText("Active")

				page.ClickSelector("#scroll-select-active-workspace")
				if err := page.Raw().GetByRole(playwright.AriaRole("option"), playwright.PageGetByRoleOptions{
					Name:  SeedAltWorkspaceName,
					Exact: playwright.Bool(true),
				}).Click(); err != nil {
					t.Fatalf("select workspace option %q: %v", SeedAltWorkspaceName, err)
				}
				if err := WaitUntil(10*time.Second, func() error {
					selectorText, err := page.Raw().Locator("#scroll-select-active-workspace").TextContent()
					if err != nil {
						return err
					}
					if !strings.Contains(selectorText, SeedAltWorkspaceName) {
						return fmt.Errorf("workspace selector text=%q, want %q", selectorText, SeedAltWorkspaceName)
					}
					disabled, err := page.Raw().GetByRole(playwright.AriaRole("button"), playwright.PageGetByRoleOptions{
						Name:  "Set",
						Exact: playwright.Bool(true),
					}).IsDisabled()
					if err != nil {
						return err
					}
					if disabled {
						return fmt.Errorf("workspace Set button is still disabled after selecting %q", SeedAltWorkspaceName)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				page.ClickRole("button", "Set")
				page.ExpectText("Workspace selection updated")
				page.ExpectText(SeedAltWorkspaceName)
				page.ClickRole("button", "Reload")
				page.ExpectText("Workspace runtime reloaded")

				page.ClickRoleLike("tab", "History")
				page.ExpectText("北京今天适合轻松散步")
				page.ClickRoleLike("button", "Play")
				page.ExpectText("History replay started")

				page.ClickRoleLike("tab", "Memory")
				page.ExpectText("Memory")
				page.ExpectText("2416")
				page.ExpectText("flowcraft-memory")

				page.ClickRoleLike("tab", "Recall")
				page.Fill(`input[placeholder="Recall query"]`, "北京适合出去玩吗")
				page.ClickRoleLike("button", "Run Recall")
				page.ExpectText("北京出行建议")
				page.ExpectText("0.910")
			},
		},
		{
			Name: "204-play-workspace-drawer-error",
			Run: func(_ testing.TB, page *Page) {
				page.GotoErrorPlay("/")
				page.ClickRole("button", "Workspace")
				page.ExpectText("no gizclaw client configured for error scenario")
			},
		},
		{
			Name: "204-play-workspace-push-to-talk-hold",
			Run: func(t testing.TB, page *Page) {
				installWorkspaceVoiceBrowserMocks(t, page)
				page.GotoPlay("/")
				page.ClickRole("button", "Workspace")
				page.ExpectText("Conversation")

				button := page.Raw().Locator("#workspace-chat-primary-trigger")
				box, err := button.BoundingBox()
				if err != nil {
					t.Fatalf("push button bounds: %v", err)
				}
				if box == nil {
					t.Fatal("push button has no bounding box")
				}
				x := box.X + box.Width/2
				y := box.Y + box.Height/2
				if err := page.Raw().Mouse().Move(x, y); err != nil {
					t.Fatalf("move mouse to push button: %v", err)
				}
				if err := page.Raw().Mouse().Down(); err != nil {
					t.Fatalf("mouse down push button: %v", err)
				}
				if err := WaitUntil(5*time.Second, func() error {
					state, err := button.GetAttribute("data-state")
					if err != nil {
						return err
					}
					if state != "pressed" {
						return fmt.Errorf("button data-state=%q, want pressed", state)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				page.ExpectText("BOS sent")
				time.Sleep(250 * time.Millisecond)
				events := readWorkspaceVoiceMockEvents(t, page)
				if len(events) != 1 || events[0]["type"] != "bos" {
					t.Fatalf("events while holding = %+v, want only BOS", events)
				}
				if text, err := page.Raw().TextContent("body"); err != nil {
					t.Fatalf("read body while holding: %v", err)
				} else if strings.Contains(text, "EOS sent") {
					t.Fatalf("EOS appeared while still holding: body=%q", text)
				}

				if err := page.Raw().Mouse().Up(); err != nil {
					t.Fatalf("mouse up push button: %v", err)
				}
				page.ExpectText("EOS sent")
				emitWorkspaceVoiceMockEvent(t, page, map[string]any{"kind": "text", "label": "transcript", "stream_id": "audio", "text": "第一轮问题", "type": "text.delta", "v": 1})
				emitWorkspaceVoiceMockEvent(t, page, map[string]any{"kind": "audio", "label": "assistant", "stream_id": "audio", "type": "bos", "v": 1})
				emitWorkspaceVoiceMockEvent(t, page, map[string]any{"kind": "text", "label": "assistant", "stream_id": "audio", "text": "第一轮回复", "type": "text.delta", "v": 1})
				emitWorkspaceVoiceMockEvent(t, page, map[string]any{"kind": "audio", "label": "assistant", "stream_id": "audio", "type": "eos", "v": 1})
				page.ExpectText("第一轮回复")
				if err := WaitUntil(5*time.Second, func() error {
					events := readWorkspaceVoiceMockEvents(t, page)
					if len(events) != 2 {
						return fmt.Errorf("event count=%d, want 2: %+v", len(events), events)
					}
					if events[0]["type"] != "bos" || events[1]["type"] != "eos" {
						return fmt.Errorf("events=%+v, want BOS then EOS", events)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}

				if err := page.Raw().Mouse().Down(); err != nil {
					t.Fatalf("second mouse down push button: %v", err)
				}
				if err := WaitUntil(5*time.Second, func() error {
					state, err := button.GetAttribute("data-state")
					if err != nil {
						return err
					}
					if state != "pressed" {
						return fmt.Errorf("second button data-state=%q, want pressed", state)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if err := page.Raw().Mouse().Up(); err != nil {
					t.Fatalf("second mouse up push button: %v", err)
				}
				emitWorkspaceVoiceMockEvent(t, page, map[string]any{"kind": "text", "label": "transcript", "stream_id": "audio", "text": "第二轮问题", "type": "text.delta", "v": 1})
				emitWorkspaceVoiceMockEvent(t, page, map[string]any{"kind": "text", "label": "assistant", "stream_id": "audio", "text": "第二轮回复", "type": "text.delta", "v": 1})
				emitWorkspaceVoiceMockEvent(t, page, map[string]any{"kind": "audio", "label": "assistant", "stream_id": "audio", "type": "eos", "v": 1})
				page.ExpectText("第二轮回复")
				if err := WaitUntil(5*time.Second, func() error {
					events := readWorkspaceVoiceMockEvents(t, page)
					if len(events) != 4 {
						return fmt.Errorf("event count=%d, want 4: %+v", len(events), events)
					}
					if events[2]["type"] != "bos" || events[3]["type"] != "eos" {
						return fmt.Errorf("second turn events=%+v, want BOS then EOS", events[2:])
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
}

func installWorkspaceVoiceBrowserMocks(t testing.TB, page *Page) {
	t.Helper()
	script := `
(() => {
  window.__gizclawSentEvents = [];
  window.__gizclawEventChannel = null;
  class FakeDataChannel {
    constructor() {
      this.readyState = "open";
      this.onmessage = null;
      window.__gizclawEventChannel = this;
    }
    send(data) {
      window.__gizclawSentEvents.push(JSON.parse(data));
    }
    close() {
      this.readyState = "closed";
    }
    addEventListener() {}
    removeEventListener() {}
  }
  class FakeRTCPeerConnection {
    constructor() {
      this.connectionState = "connected";
      this.iceGatheringState = "complete";
      this.localDescription = null;
      this.onconnectionstatechange = null;
      this.ontrack = null;
    }
    createDataChannel() {
      return new FakeDataChannel();
    }
    addTransceiver() {
      return { sender: { replaceTrack: async () => {} } };
    }
    async createOffer() {
      return { type: "offer", sdp: "fake-offer" };
    }
    async setLocalDescription(desc) {
      this.localDescription = desc;
    }
    async setRemoteDescription() {}
    addEventListener() {}
    removeEventListener() {}
    close() {
      this.connectionState = "closed";
      if (this.onconnectionstatechange) this.onconnectionstatechange();
    }
  }
  Object.defineProperty(window, "RTCPeerConnection", {
    configurable: true,
    value: FakeRTCPeerConnection,
  });
  Object.defineProperty(navigator, "mediaDevices", {
    configurable: true,
    value: {
    getUserMedia: async () => {
      const track = {
        kind: "audio",
        enabled: true,
        readyState: "live",
        stop: () => {
          track.readyState = "ended";
        },
      };
      return {
        getAudioTracks: () => [track],
        getTracks: () => [track],
      };
    },
    },
  });
})();
`
	if err := page.Raw().AddInitScript(playwright.Script{Content: playwright.String(script)}); err != nil {
		t.Fatalf("add voice browser mocks: %v", err)
	}
	if err := page.Raw().Route("**/webrtc/offer", func(route playwright.Route) {
		_ = route.Fulfill(playwright.RouteFulfillOptions{
			Body:        `{"type":"answer","sdp":"fake-answer"}`,
			ContentType: playwright.String("application/json"),
			Status:      playwright.Int(200),
		})
	}); err != nil {
		t.Fatalf("route fake webrtc offer: %v", err)
	}
}

func emitWorkspaceVoiceMockEvent(t testing.TB, page *Page, event map[string]any) {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal mock peer event: %v", err)
	}
	if _, err := page.Raw().Evaluate(`(payload) => {
  const channel = window.__gizclawEventChannel;
  if (!channel || typeof channel.onmessage !== "function") {
    throw new Error("mock event channel is not ready");
  }
  channel.onmessage({ data: payload });
}`, string(payload)); err != nil {
		t.Fatalf("emit mock peer event: %v", err)
	}
}

func readWorkspaceVoiceMockEvents(t testing.TB, page *Page) []map[string]any {
	t.Helper()
	raw, err := page.Raw().Evaluate(`JSON.stringify(window.__gizclawSentEvents || [])`)
	if err != nil {
		t.Fatalf("read sent events: %v", err)
	}
	var events []map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &events); err != nil {
		t.Fatalf("decode sent events: %v", err)
	}
	return events
}
