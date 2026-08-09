package transformer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type routeLifecycleKey struct {
	streamID string
	mimeType string
}

type observedRouteLifecycle struct {
	begun      bool
	ended      bool
	dataChunks int
	chunks     int
	role       genx.Role
	label      string
}

type routeLifecycleTracker struct {
	routes map[routeLifecycleKey]*observedRouteLifecycle
}

func newRouteLifecycleTracker() *routeLifecycleTracker {
	return &routeLifecycleTracker{routes: make(map[routeLifecycleKey]*observedRouteLifecycle)}
}

func (tracker *routeLifecycleTracker) observe(chunk *genx.MessageChunk) error {
	if chunk == nil {
		return fmt.Errorf("route lifecycle: nil chunk")
	}
	if chunk.Part == nil {
		return nil
	}
	mimeType, ok := chunk.MIMEType()
	if !ok {
		return fmt.Errorf("route lifecycle: chunk has no canonical MIME: %#v", chunk)
	}
	if chunk.Ctrl == nil || strings.TrimSpace(chunk.Ctrl.StreamID) == "" {
		return fmt.Errorf("route lifecycle: %s chunk has no StreamID", mimeType)
	}

	key := routeLifecycleKey{streamID: chunk.Ctrl.StreamID, mimeType: mimeType}
	state := tracker.routes[key]
	if state == nil {
		state = &observedRouteLifecycle{}
		tracker.routes[key] = state
	}
	if state.ended {
		return fmt.Errorf("route lifecycle: %s received chunk after EOS", key)
	}
	if chunk.IsBeginOfStream() {
		if state.begun {
			return fmt.Errorf("route lifecycle: %s received duplicate BOS", key)
		}
		state.begun = true
		state.role = chunk.Role
		state.label = chunk.Ctrl.Label
	} else if !state.begun {
		return fmt.Errorf("route lifecycle: %s received data or EOS before BOS", key)
	} else if chunk.Role != state.role || chunk.Ctrl.Label != state.label {
		return fmt.Errorf(
			"route lifecycle: %s role/label changed from (%q, %q) to (%q, %q)",
			key, state.role, state.label, chunk.Role, chunk.Ctrl.Label,
		)
	}
	// Name identifies the publisher of an individual chunk, not the route.
	// Flowcraft may publish several named nodes under one response StreamID so
	// Audio Dock can resolve a different voice for each publisher.
	if routeChunkHasData(chunk) {
		state.dataChunks++
	}
	if chunk.IsEndOfStream() {
		state.ended = true
	}
	state.chunks++
	return nil
}

func (tracker *routeLifecycleTracker) allComplete() bool {
	if tracker == nil || len(tracker.routes) == 0 {
		return false
	}
	for _, state := range tracker.routes {
		if !state.begun || !state.ended {
			return false
		}
	}
	return true
}

func (tracker *routeLifecycleTracker) assertComplete(t *testing.T) {
	t.Helper()
	if tracker == nil || len(tracker.routes) == 0 {
		t.Fatal("route lifecycle: no MIME routes observed")
	}
	for key, state := range tracker.routes {
		if !state.begun || !state.ended {
			t.Errorf("route lifecycle: %s = %#v, want exactly one BOS and EOS", key, state)
		}
	}
}

func (tracker *routeLifecycleTracker) route(streamID, mimeType string) *observedRouteLifecycle {
	if tracker == nil {
		return nil
	}
	return tracker.routes[routeLifecycleKey{streamID: streamID, mimeType: mimeType}]
}

func (key routeLifecycleKey) String() string {
	return fmt.Sprintf("(%q, %q)", key.streamID, key.mimeType)
}

func routeChunkHasData(chunk *genx.MessageChunk) bool {
	if chunk == nil {
		return false
	}
	switch part := chunk.Part.(type) {
	case genx.Text:
		return len(part) > 0
	case *genx.Blob:
		return part != nil && len(part.Data) > 0
	default:
		return false
	}
}

func observeRouteLifecycle(t *testing.T, tracker *routeLifecycleTracker, chunk *genx.MessageChunk) {
	t.Helper()
	if err := tracker.observe(chunk); err != nil {
		t.Fatal(err)
	}
}

func completeTextRoute(role genx.Role, name, label, streamID, value string) []*genx.MessageChunk {
	metadata := func(begin, end bool) *genx.StreamCtrl {
		return &genx.StreamCtrl{
			StreamID: streamID, Label: label, BeginOfStream: begin, EndOfStream: end,
		}
	}
	return []*genx.MessageChunk{
		{Role: role, Name: name, Part: genx.Text(""), Ctrl: metadata(true, false)},
		{Role: role, Name: name, Part: genx.Text(value), Ctrl: metadata(false, false)},
		{Role: role, Name: name, Part: genx.Text(""), Ctrl: metadata(false, true)},
	}
}

func TestRouteLifecycleTrackerAcceptsCanonicalInterleavedRoutes(t *testing.T) {
	tracker := newRouteLifecycleTracker()
	for _, chunk := range []*genx.MessageChunk{
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "Audio/Opus; rate=48000; codecs=opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
		{Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "turn"}},
		{Part: &genx.Blob{MIMEType: "audio/opus; codecs=opus; rate=48000", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "turn"}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/opus; rate=48000; codecs=opus"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}},
	} {
		observeRouteLifecycle(t, tracker, chunk)
	}
	tracker.assertComplete(t)
	if len(tracker.routes) != 2 {
		t.Fatalf("routes = %#v, want text and canonical audio", tracker.routes)
	}
}

func TestRouteLifecycleTrackerAcceptsEmptyErrorRoute(t *testing.T) {
	tracker := newRouteLifecycleTracker()
	for _, chunk := range []*genx.MessageChunk{
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true, Error: "provider failed"}},
	} {
		observeRouteLifecycle(t, tracker, chunk)
	}
	tracker.assertComplete(t)
	state := tracker.route("turn", "text/plain")
	if state == nil || state.dataChunks != 0 || state.chunks != 2 {
		t.Fatalf("empty error route = %#v, want BOS and error EOS without data", state)
	}
}

func TestRouteLifecycleTrackerAcceptsPublisherNameChanges(t *testing.T) {
	tracker := newRouteLifecycleTracker()
	for _, chunk := range []*genx.MessageChunk{
		{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant", BeginOfStream: true}},
		{Role: genx.RoleModel, Name: "narrator", Part: genx.Text("first"), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant"}},
		{Role: genx.RoleModel, Name: "character", Part: genx.Text("second"), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant"}},
		{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant", EndOfStream: true}},
	} {
		observeRouteLifecycle(t, tracker, chunk)
	}
	tracker.assertComplete(t)
}

func TestRouteLifecycleTrackerReportsIncompleteRoute(t *testing.T) {
	tracker := newRouteLifecycleTracker()
	if tracker.allComplete() {
		t.Fatal("empty tracker reported complete")
	}
	observeRouteLifecycle(t, tracker, &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true},
	})
	if tracker.allComplete() {
		t.Fatal("route without EOS reported complete")
	}
}

func TestRouteLifecycleTrackerRejectsInvalidBoundaries(t *testing.T) {
	validBOS := &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true},
	}
	validEOS := &genx.MessageChunk{
		Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true},
	}
	for _, test := range []struct {
		name   string
		chunks []*genx.MessageChunk
	}{
		{name: "nil chunk", chunks: []*genx.MessageChunk{nil}},
		{name: "data before BOS", chunks: []*genx.MessageChunk{{Part: genx.Text("late"), Ctrl: &genx.StreamCtrl{StreamID: "turn"}}}},
		{name: "duplicate BOS", chunks: []*genx.MessageChunk{validBOS, validBOS.Clone()}},
		{name: "post EOS", chunks: []*genx.MessageChunk{validBOS, validEOS, {Part: genx.Text("late"), Ctrl: &genx.StreamCtrl{StreamID: "turn"}}}},
		{name: "metadata changed", chunks: []*genx.MessageChunk{
			{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("late"), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "other"}},
		}},
		{name: "missing control", chunks: []*genx.MessageChunk{{Part: genx.Text("late")}}},
		{name: "missing StreamID", chunks: []*genx.MessageChunk{{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{BeginOfStream: true}}}},
		{name: "invalid MIME", chunks: []*genx.MessageChunk{{Part: &genx.Blob{MIMEType: "not a mime"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tracker := newRouteLifecycleTracker()
			var gotErr error
			for _, chunk := range test.chunks {
				if gotErr = tracker.observe(chunk); gotErr != nil {
					break
				}
			}
			if gotErr == nil {
				t.Fatalf("observe(%s) error = nil", test.name)
			}
		})
	}
}
