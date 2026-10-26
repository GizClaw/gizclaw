package agenthost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

type gatedOutputHistoryStore struct {
	memory.HistoryStore
	entered chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (s *gatedOutputHistoryStore) Append(ctx context.Context, entries []memory.HistoryEntry) error {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return s.HistoryStore.Append(ctx, entries)
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func waitForHistoryAppend(t testing.TB, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("History append was not reached")
	}
}

func TestHistoryOutputDoesNotWaitForTranscriptPersistence(t *testing.T) {
	for _, audio := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "audio"}[audio], func(t *testing.T) {
			for _, coalesced := range []bool{false, true} {
				t.Run(map[bool]string{false: "streaming", true: "terminal carries data"}[coalesced], func(t *testing.T) {
					testHistoryOutputPersistenceGate(t, coalesced, audio)
				})
			}
		})
	}
}

func testHistoryOutputPersistenceGate(t *testing.T, coalesced, audio bool) {
	t.Helper()
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	store := &gatedOutputHistoryStore{HistoryStore: history.Records, entered: make(chan struct{}), release: release}
	history.Records = store
	chunks := []*genx.MessageChunk{
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("question"), Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript", BeginOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "input", Label: "transcript", EndOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", BeginOfStream: true}},
		&genx.MessageChunk{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "reply", Label: "assistant", EndOfStream: true}},
	}
	if audio {
		chunks[2].Part = &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xf8, 0xff, 0xfe}}
		chunks[3].Part = &genx.Blob{MIMEType: "audio/opus"}
	}
	if coalesced {
		chunks[0].Ctrl.EndOfStream = true
		chunks[2].Ctrl.EndOfStream = true
		chunks = []*genx.MessageChunk{chunks[0], chunks[2]}
	}
	chunks = append(chunks, &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "after-reply"}})
	agent := wrapHistoryAgent(historyTestAgent{output: historyStreamFromChunks(chunks...)}, history)
	ctx, cancel := context.WithCancel(withHistoryGearID(t.Context(), "gear"))
	defer cancel()
	output, err := agent.Transform(ctx, historyStreamFromChunks())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		unblock()
		_ = output.Close()
	}()
	firstBody := make(chan struct{})
	marker := make(chan struct{})
	terminals := make(chan struct{}, 4)
	begins := make(map[genx.Role]int)
	completed := make(chan error, 1)
	go func() {
		for {
			chunk, err := output.Next()
			if chunk != nil && chunk.Role == genx.RoleModel {
				switch part := chunk.Part.(type) {
				case genx.Text:
					if part != "" {
						close(firstBody)
					}
				case *genx.Blob:
					if len(part.Data) != 0 {
						close(firstBody)
					}
				}
			}
			if chunk != nil && chunk.Ctrl != nil {
				if chunk.IsBeginOfStream() {
					begins[chunk.Role]++
				}
				if chunk.Ctrl.StreamID == "after-reply" {
					close(marker)
				}
				if chunk.IsEndOfStream() {
					terminals <- struct{}{}
				}
			}
			if err != nil {
				completed <- err
				return
			}
		}
	}()
	select {
	case <-store.entered:
	case <-time.After(time.Second):
		unblock()
		t.Fatal("History append was not reached")
	}
	select {
	case <-firstBody:
	case <-time.After(time.Second):
		unblock()
		<-completed
		t.Fatal("slow transcript persistence blocked already available assistant data")
	}
	select {
	case <-marker:
	case <-time.After(time.Second):
		unblock()
		t.Fatal("unrelated output waited behind History completion")
	}
	select {
	case <-terminals:
		unblock()
		t.Fatal("completion became visible before durable History")
	default:
	}
	unblock()
	if err := <-completed; !IsStreamDone(err) {
		t.Fatal(err)
	}
	if begins[genx.RoleUser] != 1 || begins[genx.RoleModel] != 1 || len(terminals) != 2 {
		t.Fatalf("text lifecycle changed: begin=%v terminals=%d", begins, len(terminals))
	}
	page, err := agent.ListHistory(ctx, apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	wantText := "answer"
	if audio {
		wantText = ""
	}
	if len(page.Items) != 2 || page.Items[0].Text != "question" || page.Items[1].Text != wantText {
		t.Fatalf("History lost its exact input/reply order: %v", page.Items)
	}
	if audio && !page.Items[1].ReplayAvailable {
		t.Fatal("History lost the delivered audio asset")
	}
}

func TestHistoryPersistenceKeepsPriorTextBarrier(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	store := &gatedOutputHistoryStore{HistoryStore: history.Records, entered: make(chan struct{}), release: release}
	history.Records = store
	r := newHistoryRecorder(history, "gear", nil)
	r.startPersistence(t.Context())
	defer func() {
		unblock()
		r.persistence.close(t.Context())
	}()
	r.record(t.Context(), &genx.MessageChunk{Role: genx.RoleUser, Name: "transcript", Part: genx.Text("question"), Ctrl: &genx.StreamCtrl{StreamID: "input", BeginOfStream: true, EndOfStream: true}})
	waitForHistoryAppend(t, store.entered)
	if err := r.waitForPriorText(t.Context(), "input"); err != nil {
		unblock()
		t.Fatal(err)
	}
	barrier := make(chan error, 1)
	go func() { barrier <- r.waitForPriorText(t.Context(), "next-input") }()
	select {
	case err := <-barrier:
		unblock()
		t.Fatalf("next turn read uncommitted History: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	if err := <-barrier; err != nil {
		t.Fatal(err)
	}
}

func TestHistoryPersistenceCloseCancelsBlockedAppend(t *testing.T) {
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	store := &gatedOutputHistoryStore{HistoryStore: history.Records, entered: make(chan struct{}), release: make(chan struct{})}
	history.Records = store
	r := newHistoryRecorder(history, "gear", nil)
	r.startPersistence(t.Context())
	defer func() {
		r.persistence.abort()
		<-r.persistence.done
	}()
	r.record(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: "reply", BeginOfStream: true, EndOfStream: true}})
	waitForHistoryAppend(t, store.entered)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	r.persistence.close(ctx)
	select {
	case <-r.persistence.done:
	default:
		t.Fatal("persistence writer survived close")
	}
	r.mu.Lock()
	remaining := len(r.pendingText)
	r.mu.Unlock()
	if remaining != 0 {
		t.Fatal("cancelled persistence left a pending History read barrier")
	}
}

func TestHistoryPersistenceBoundsBacklogWithoutBlockingProducer(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	history := newTestWorkspaceHistory(t, newTestObjectStore(t))
	store := &gatedOutputHistoryStore{HistoryStore: history.Records, entered: make(chan struct{}), release: release}
	history.Records = store
	r := newHistoryRecorder(history, "gear", nil)
	r.startPersistence(t.Context())
	defer func() {
		unblock()
		r.persistence.close(t.Context())
	}()
	for i := 0; i <= historyPersistenceMaxEntries; i++ {
		r.record(t.Context(), &genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("answer"), Ctrl: &genx.StreamCtrl{StreamID: fmt.Sprint(i), BeginOfStream: true, EndOfStream: true}})
		if i == 0 {
			waitForHistoryAppend(t, store.entered)
		}
	}
	if err := r.waitForPriorText(t.Context(), "next-input"); !errors.Is(err, errHistoryPersistenceBacklog) {
		t.Fatalf("recording overload was hidden from the next History reader: %v", err)
	}
	r.mu.Lock()
	entries := r.persistence.entries
	r.mu.Unlock()
	if entries != historyPersistenceMaxEntries {
		t.Fatalf("unbounded queued records: %d", entries)
	}
}
