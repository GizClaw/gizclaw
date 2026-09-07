package doubaorealtime

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func initiativeTestEvents() []*doubaospeech.RealtimeEvent {
	return []*doubaospeech.RealtimeEvent{
		{Type: doubaospeech.EventChatResponse, Text: "你好呀"},
		{Type: doubaospeech.EventTTSStarted, Text: "你好呀"},
		{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2, 3, 4}},
		{Type: doubaospeech.EventChatEnded},
		{Type: doubaospeech.EventTTSFinished},
	}
}

// requireNoInitiativeLeak asserts the hidden query never reached output and no
// user-role chunk was published for the opening turn.
func requireNoInitiativeLeak(t *testing.T, chunks []*genx.MessageChunk, query string) {
	t.Helper()
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if chunk.Role == genx.RoleUser {
			t.Fatalf("initiative published a user chunk: %#v", chunk)
		}
		if text, ok := chunk.Part.(genx.Text); ok && strings.Contains(string(text), query) {
			t.Fatalf("initiative query leaked into output: %#v", chunk)
		}
	}
}

func TestTransformerInitiativeSendsHiddenQueryPerMode(t *testing.T) {
	for _, tc := range []struct {
		mode     Mode
		streamID string
	}{
		{mode: ModePushToTalk, streamID: doubaoRealtimeInitiativeStreamID},
		{mode: ModeRealtime, streamID: doubaoRealtimeInitiativeStreamID + ":rt:1"},
		{mode: ModeText, streamID: doubaoRealtimeInitiativeStreamID},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			querySent := make(chan struct{})
			eventsDrained := make(chan struct{})
			session := &fakeTransformerSession{
				beforeRecv:       querySent,
				firstTextSent:    querySent,
				eventsDrained:    eventsDrained,
				blockAfterEvents: make(chan struct{}),
				events:           initiativeTestEvents(),
			}
			opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
			tfr := newTransformer(nil,
				withDoubaoRealtimeOpener(opener),
				withMode(tc.mode),
				withFormat("pcm"),
				withInitiative(InitiativeOnReload, "  请先开口  "),
			)
			input := &gatedRealtimeStream{gate: eventsDrained}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			output, err := tfr.transform(ctx, input)
			if err != nil {
				t.Fatalf("transform() error = %v", err)
			}
			chunks := drainRealtimeTestOutput(t, output)
			if got := session.textMessages(); !slices.Equal(got, []string{"请先开口"}) {
				t.Fatalf("hidden queries = %q, want exactly one trimmed query", got)
			}
			if got := session.endASRCount(); got != 0 {
				t.Fatalf("EndASR calls = %d, want 0", got)
			}
			if got := session.interruptCount(); got != 0 {
				t.Fatalf("Interrupt calls = %d, want 0", got)
			}
			requireNoInitiativeLeak(t, chunks, "请先开口")
			requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 2)
			for _, chunk := range chunks {
				if chunk.Role != genx.RoleModel || chunk.Ctrl == nil {
					continue
				}
				if chunk.Ctrl.StreamID != tc.streamID {
					t.Fatalf("assistant StreamID = %q, want %q", chunk.Ctrl.StreamID, tc.streamID)
				}
				if chunk.Ctrl.Error != "" {
					t.Fatalf("assistant route ended with error %q: %#v", chunk.Ctrl.Error, chunk)
				}
			}
			if !hasRealtimeTestText(chunks, genx.RoleModel, "你好呀") {
				t.Fatalf("missing opening text: %#v", chunks)
			}
			if !hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
				t.Fatalf("missing opening audio: %#v", chunks)
			}
		})
	}
}

func TestTransformerInitiativeDisabledSendsNothing(t *testing.T) {
	eventsDrained := make(chan struct{})
	session := &fakeTransformerSession{
		eventsDrained:    eventsDrained,
		blockAfterEvents: make(chan struct{}),
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	tfr := newTransformer(nil, withDoubaoRealtimeOpener(opener), withMode(ModePushToTalk), withFormat("pcm"))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &gatedRealtimeStream{gate: eventsDrained})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := session.textMessages(); len(got) != 0 {
		t.Fatalf("hidden queries = %q, want none", got)
	}
	if len(chunks) != 0 {
		t.Fatalf("output = %#v, want empty", chunks)
	}
}

func TestTransformerInitiativeResendsAfterLossBeforeResponse(t *testing.T) {
	firstQuery := make(chan struct{})
	first := &fakeTransformerSession{
		beforeRecv:    firstQuery,
		firstTextSent: firstQuery,
		recvErr:       errors.New("provider lost"),
	}
	secondQuery := make(chan struct{})
	eventsDrained := make(chan struct{})
	second := &fakeTransformerSession{
		beforeRecv:       secondQuery,
		firstTextSent:    secondQuery,
		eventsDrained:    eventsDrained,
		blockAfterEvents: make(chan struct{}),
		events:           initiativeTestEvents(),
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModePushToTalk),
		withFormat("pcm"),
		withInitiative(InitiativeOnReload, ""),
	)
	tfr.retryWait = func(context.Context, <-chan struct{}, time.Duration) bool { return true }
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &gatedRealtimeStream{gate: eventsDrained})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := first.textMessages(); !slices.Equal(got, []string{DefaultInitiativeQuery}) {
		t.Fatalf("first session queries = %q", got)
	}
	if got := second.textMessages(); !slices.Equal(got, []string{DefaultInitiativeQuery}) {
		t.Fatalf("replacement session queries = %q, want the opening query again", got)
	}
	requireNoInitiativeLeak(t, chunks, DefaultInitiativeQuery)
	if !hasRealtimeTestText(chunks, genx.RoleModel, "你好呀") || !hasRealtimeTestBlob(chunks, genx.RoleModel, "audio/pcm") {
		t.Fatalf("replacement opening not published: %#v", chunks)
	}
	var successful int
	for _, chunk := range chunks {
		if chunk.Role == genx.RoleModel && chunk.Ctrl != nil && chunk.Ctrl.EndOfStream && chunk.Ctrl.Error == "" {
			successful++
		}
	}
	if successful != 2 {
		t.Fatalf("successful assistant EOS = %d, want text and audio: %#v", successful, chunks)
	}
}

func TestTransformerInitiativeDoesNotResendAfterResponseStarted(t *testing.T) {
	firstQuery := make(chan struct{})
	first := &fakeTransformerSession{
		beforeRecv:    firstQuery,
		firstTextSent: firstQuery,
		events: []*doubaospeech.RealtimeEvent{
			{Type: doubaospeech.EventTTSStarted, Text: "你好"},
			{Type: doubaospeech.EventTTSAudioData, Audio: []byte{1, 2}},
		},
		recvErr: errors.New("provider lost"),
	}
	secondOpened := make(chan struct{})
	second := &fakeTransformerSession{
		eventsDrained:    secondOpened,
		blockAfterEvents: make(chan struct{}),
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModePushToTalk),
		withFormat("pcm"),
		withInitiative(InitiativeOnReload, ""),
	)
	tfr.retryWait = func(context.Context, <-chan struct{}, time.Duration) bool { return true }
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &gatedRealtimeStream{gate: secondOpened})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := first.textMessages(); !slices.Equal(got, []string{DefaultInitiativeQuery}) {
		t.Fatalf("first session queries = %q", got)
	}
	if got := second.textMessages(); len(got) != 0 {
		t.Fatalf("replacement session queries = %q, want none after the opening started", got)
	}
	requireNoInitiativeLeak(t, chunks, DefaultInitiativeQuery)
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 2)
	for _, chunk := range chunks {
		if chunk.Ctrl != nil && chunk.Ctrl.EndOfStream && chunk.Ctrl.Error == "" {
			t.Fatalf("lost opening ended without error: %#v", chunk)
		}
	}
}

func TestTransformerInitiativePushToTalkBargeInInterruptsOpening(t *testing.T) {
	querySent := make(chan struct{})
	eventPaused := make(chan struct{})
	session := &fakeTransformerSession{
		beforeRecv:       querySent,
		firstTextSent:    querySent,
		events:           initiativeTestEvents(),
		pauseBeforeEvent: 3,
		eventPaused:      eventPaused,
		resumeEvents:     make(chan struct{}),
		blockAfterEvents: make(chan struct{}),
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: session}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModePushToTalk),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
		withInitiative(InitiativeOnReload, ""),
	)
	input := &gatedRealtimeStream{
		gate: eventPaused,
		rest: []*genx.MessageChunk{{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}}},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, input)
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := session.interruptCount(); got != 1 {
		t.Fatalf("Interrupt calls = %d, want 1", got)
	}
	if got := session.textMessages(); !slices.Equal(got, []string{DefaultInitiativeQuery}) {
		t.Fatalf("hidden queries = %q, want exactly one", got)
	}
	requireNoInitiativeLeak(t, chunks, DefaultInitiativeQuery)
	if !hasRealtimeInterruptedEOS(chunks, doubaoRealtimeInitiativeStreamID, genx.RoleModel, false) ||
		!hasRealtimeInterruptedEOS(chunks, doubaoRealtimeInitiativeStreamID, genx.RoleModel, true) {
		t.Fatalf("missing interrupted opening EOS: %#v", chunks)
	}
	requireRealtimeOwnedRouteLifecycles(t, chunks, genx.RoleModel, doubaoRealtimeAssistantLabel, 2)
}

func TestTransformerInitiativeRealtimeBargeInHandsOffToReplacement(t *testing.T) {
	querySent := make(chan struct{})
	eventPaused := make(chan struct{})
	first := &fakeTransformerSession{
		beforeRecv:       querySent,
		firstTextSent:    querySent,
		events:           initiativeTestEvents(),
		pauseBeforeEvent: 3,
		eventPaused:      eventPaused,
		resumeEvents:     make(chan struct{}),
		blockAfterEvents: make(chan struct{}),
	}
	secondOpened := make(chan struct{})
	second := &fakeTransformerSession{
		eventsDrained:    secondOpened,
		blockAfterEvents: make(chan struct{}),
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModeRealtime),
		withInputFormat("pcm"),
		withInputTranscode(false),
		withFormat("pcm"),
		withInitiative(InitiativeOnReload, ""),
	)
	input := &gatedRealtimeStream{
		gate: eventPaused,
		rest: []*genx.MessageChunk{{Ctrl: &genx.StreamCtrl{StreamID: "mic", BeginOfStream: true}}},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, input)
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := first.interruptCount(); got != 0 {
		t.Fatalf("realtime handoff must not send ClientInterrupt, got %d", got)
	}
	if got := second.textMessages(); len(got) != 0 {
		t.Fatalf("replacement session queries = %q, want none after Peer barge-in", got)
	}
	requireNoInitiativeLeak(t, chunks, DefaultInitiativeQuery)
	streamID := doubaoRealtimeInitiativeStreamID + ":rt:1"
	if !hasRealtimeInterruptedEOS(chunks, streamID, genx.RoleModel, false) ||
		!hasRealtimeInterruptedEOS(chunks, streamID, genx.RoleModel, true) {
		t.Fatalf("missing interrupted opening EOS: %#v", chunks)
	}
}

func TestTransformerInitiativeSendFailureRetriesOnReplacement(t *testing.T) {
	first := &fakeTransformerSession{sendTextErr: errors.New("send failed"), sendTextErrAt: 1}
	secondQuery := make(chan struct{})
	eventsDrained := make(chan struct{})
	second := &fakeTransformerSession{
		beforeRecv:       secondQuery,
		firstTextSent:    secondQuery,
		eventsDrained:    eventsDrained,
		blockAfterEvents: make(chan struct{}),
		events:           initiativeTestEvents(),
	}
	opener := &fakeTransformerOpener{results: []fakeTransformerOpenResult{{session: first}, {session: second}}}
	tfr := newTransformer(nil,
		withDoubaoRealtimeOpener(opener),
		withMode(ModePushToTalk),
		withFormat("pcm"),
		withInitiative(InitiativeOnReload, ""),
	)
	tfr.retryWait = func(context.Context, <-chan struct{}, time.Duration) bool { return true }
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	output, err := tfr.transform(ctx, &gatedRealtimeStream{gate: eventsDrained})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	chunks := drainRealtimeTestOutput(t, output)
	if got := second.textMessages(); !slices.Equal(got, []string{DefaultInitiativeQuery}) {
		t.Fatalf("replacement session queries = %q, want the opening query", got)
	}
	if !hasRealtimeTestText(chunks, genx.RoleModel, "你好呀") {
		t.Fatalf("replacement opening not published: %#v", chunks)
	}
}

func TestNewRejectsUnknownInitiative(t *testing.T) {
	_, err := New(Config{Client: &doubaospeech.Client{}, Model: "model", Initiative: InitiativePolicy("sometimes")})
	if err == nil || !strings.Contains(err.Error(), "unsupported Initiative") {
		t.Fatalf("New() error = %v, want unsupported Initiative", err)
	}
}
