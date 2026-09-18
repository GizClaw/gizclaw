package giztestcmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/peergenx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// TestMixedProviderSpeakerVoicesGiztest runs one Eino or Flowcraft reply whose
// narrator Voice natively produces Ogg/Opus and whose speaker_voices character
// Voice natively produces MP3, through AgentHost with Workspace History enabled.
func TestMixedProviderSpeakerVoicesGiztest(t *testing.T) {
	for _, kind := range []string{"eino", "flowcraft"} {
		for _, fault := range []string{"", "ignore-format"} {
			name := fault
			if name == "" {
				name = "success"
			}
			t.Run(kind+"/"+name, func(t *testing.T) { runMixedProviderVoices(t, kind, fault) })
		}
	}
}

func runMixedProviderVoices(t *testing.T, kind, fault string) {
	root := "../../../../tests/gizclaw-e2e/testdata/eino-voices"
	if kind == "flowcraft" {
		root = "../../../../tests/gizclaw-e2e/testdata/multi-role-voices"
	}
	data, err := os.ReadFile(filepath.Join(root, "mixed-providers.json"))
	if err != nil {
		t.Fatal(err)
	}
	packets := map[string][][]byte{
		"story.default": voiceTonePackets(t, 300),
		"story.fox":     voiceTonePackets(t, 500),
	}
	provider := &voiceFixtureProvider{
		packets: packets,
		formats: map[string]string{"story.default": "ogg_opus", "story.fox": "mp3"},
		fault:   fault,
	}
	resources := voiceFixtureResources{}
	service := peergenx.New(peergenx.Service{Models: resources, Voices: resources, Credentials: resources, ProviderTenants: resources, Builder: provider})
	history := newMixedVoiceHistory(t)
	spec := newVoiceFixtureSpec(t, kind, data, apitypes.WorkspaceInputModePushToTalk)
	spec.Runtime.History = history
	host := agenthost.New(fixedSpecResolver{spec: spec})
	if err := host.Register(kind, voiceFixtureFactory(kind, service)); err != nil {
		t.Fatal(err)
	}
	agent, release, err := host.OpenAgent(t.Context(), "voice-fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	input := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 128)
	output, err := agent.Transform(ctx, input.Stream())
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	transport := &voiceFixtureStream{input: input, output: output}
	driver := &voiceFixtureDriver{driver: newDriver(false, nil), stream: transport}
	doc, err := giztest.LoadDocument(filepath.Join(root, "mixed-providers.giztest.yaml"), driver)
	if err != nil {
		t.Fatal(err)
	}
	// The narrator segment and the character segment arrive as one Ogg/Opus
	// audio stream, in reply order.
	var expected []byte
	for _, voice := range []string{"story.default", "story.fox"} {
		var segment bytes.Buffer
		if err := codecconv.OpusPacketsToOgg(&segment, 16000, 1, packets[voice]); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, segment.Bytes()...)
	}
	digest := sha256.Sum256(expected)
	variable := doc.Variables["digest"]
	variable.Value = hex.EncodeToString(digest[:])
	doc.Variables["digest"] = variable

	report := giztest.Run(ctx, []*giztest.Document{doc}, giztest.Options{Driver: driver, Parallel: 1, Out: io.Discard})
	// Ending the input ends the invocation. AgentHost ends its output only
	// after the last History append, so no write outlives the test.
	_ = input.Done(genx.Usage{})
	for {
		if _, err := output.Next(); err != nil {
			break
		}
	}
	encoded, _ := json.Marshal(report)
	if len(report.Tasks) != 1 {
		t.Fatalf("report=%s", encoded)
	}
	if fault == "ignore-format" {
		// A provider that cannot honor the segment format must fail only this
		// reply's audio with a diagnosable error, never mix MIME types.
		if report.Tasks[0].Status == "passed" || !strings.Contains(report.Tasks[0].Error, `differs from response audio "audio/ogg"`) {
			t.Fatalf("mixed provider formats were not rejected: %s", encoded)
		}
		t.Logf("rejected: %s", report.Tasks[0].Error)
		return
	}
	if report.Tasks[0].Status != "passed" {
		t.Fatalf("report=%s", encoded)
	}
	for _, format := range provider.formatRequests() {
		if format != peergenx.SegmentVoiceFormat {
			t.Fatalf("TTS format requests=%q, want %s", provider.formatRequests(), peergenx.SegmentVoiceFormat)
		}
	}
	if provider.calls.Load() != 2 {
		t.Fatalf("TTS calls=%d, want narrator and fox segments", provider.calls.Load())
	}
	entry := waitMixedVoiceHistory(t, history)
	if entry.Text != "The narrator begins. I am the fox." || len(entry.Assets) != 1 || entry.Assets[0].MIMEType != "audio/ogg; codecs=opus" {
		t.Fatalf("history entry = %+v, want reply text with one Ogg/Opus asset", entry)
	}
	reader, err := history.ReadAsset(t.Context(), entry.Assets[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var stored [][]byte
	for packet, err := range codecconv.OggOpusPackets(reader) {
		if err != nil {
			t.Fatal(err)
		}
		stored = append(stored, packet)
	}
	want := append(append([][]byte(nil), packets["story.default"]...), packets["story.fox"]...)
	if voicePacketDigest(stored) != voicePacketDigest(want) {
		t.Fatalf("history audio has %d packets, want narrator then fox (%d packets)", len(stored), len(want))
	}
	t.Logf("%s: narrator Ogg/Opus + fox MP3 Voice -> one audio/ogg stream; History stored %d packets", kind, len(stored))
}

type fixedSpecResolver struct{ spec agenthost.Spec }

func (r fixedSpecResolver) Resolve(context.Context, string) (agenthost.Spec, error) {
	return r.spec, nil
}

func newMixedVoiceHistory(t *testing.T) *workspace.HistoryStore {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	objects, err := objectstore.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	records, err := logstore.NewSQLStoreWithDB(db, "workspace_history")
	if err != nil {
		t.Fatal(err)
	}
	return workspace.NewHistoryStore(records, objects, "voice-fixture")
}

// waitMixedVoiceHistory waits for the agent entry, which AgentHost appends
// once the reply's text and audio routes have both ended.
func waitMixedVoiceHistory(t *testing.T, history *workspace.HistoryStore) workspace.HistoryEntry {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		page, err := history.ListEntries(t.Context(), "", "", 10)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range page.Entries {
			if entry.Type == "agent" {
				return entry
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("history entries = %+v, want the agent reply", page.Entries)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
