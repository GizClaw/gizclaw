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

type observedStreamLifecycle struct {
	begun    bool
	explicit bool
	ended    bool
	chunks   int
}

type routeLifecycleTracker struct {
	streams map[string]*observedStreamLifecycle
	routes  map[routeLifecycleKey]*observedRouteLifecycle
}

func newRouteLifecycleTracker() *routeLifecycleTracker {
	return &routeLifecycleTracker{
		streams: make(map[string]*observedStreamLifecycle),
		routes:  make(map[routeLifecycleKey]*observedRouteLifecycle),
	}
}

func (tracker *routeLifecycleTracker) observe(chunk *genx.MessageChunk) error {
	if chunk == nil {
		return fmt.Errorf("route lifecycle: nil chunk")
	}
	if chunk.Ctrl != nil && chunk.Ctrl.Error != "" && !chunk.IsEndOfStream() {
		return fmt.Errorf("route lifecycle: error metadata without EOS: %#v", chunk)
	}
	if chunk.Part == nil {
		return tracker.observeControl(chunk)
	}
	mimeType, ok := chunk.MIMEType()
	if !ok {
		return fmt.Errorf("route lifecycle: chunk has no canonical MIME: %#v", chunk)
	}
	if chunk.Ctrl == nil || strings.TrimSpace(chunk.Ctrl.StreamID) == "" {
		return fmt.Errorf("route lifecycle: %s chunk has no StreamID", mimeType)
	}

	streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
	stream := tracker.stream(streamID)
	if stream.ended {
		return fmt.Errorf("route lifecycle: StreamID %q received chunk after route EOS", streamID)
	}
	if chunk.IsBeginOfStream() {
		stream.begun = true
	} else if !stream.begun {
		return fmt.Errorf("route lifecycle: StreamID %q received data or EOS before BOS", streamID)
	}
	stream.chunks++

	key := routeLifecycleKey{streamID: streamID, mimeType: mimeType}
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

func (tracker *routeLifecycleTracker) observeControl(chunk *genx.MessageChunk) error {
	if chunk.Ctrl == nil || (!chunk.IsBeginOfStream() && !chunk.IsEndOfStream()) {
		return nil
	}
	streamID := strings.TrimSpace(chunk.Ctrl.StreamID)
	if streamID == "" {
		return fmt.Errorf("route lifecycle: control boundary has no StreamID")
	}
	state := tracker.stream(streamID)
	if state.ended {
		return fmt.Errorf("route lifecycle: StreamID %q received control after route EOS", streamID)
	}
	if chunk.IsBeginOfStream() {
		if state.begun {
			return fmt.Errorf("route lifecycle: StreamID %q received duplicate route BOS", streamID)
		}
		state.begun = true
		state.explicit = true
	} else if !state.begun {
		return fmt.Errorf("route lifecycle: StreamID %q received route EOS before BOS", streamID)
	}
	if chunk.IsEndOfStream() {
		state.ended = true
		for key, route := range tracker.routes {
			if key.streamID == streamID && route.begun {
				route.ended = true
			}
		}
	}
	state.chunks++
	return nil
}

func (tracker *routeLifecycleTracker) stream(streamID string) *observedStreamLifecycle {
	state := tracker.streams[streamID]
	if state == nil {
		state = &observedStreamLifecycle{}
		tracker.streams[streamID] = state
	}
	return state
}

func (tracker *routeLifecycleTracker) allComplete() bool {
	if tracker == nil || len(tracker.streams) == 0 {
		return false
	}
	for streamID, state := range tracker.streams {
		if !tracker.streamComplete(streamID, state) {
			return false
		}
	}
	for _, state := range tracker.routes {
		if !state.begun || !state.ended {
			return false
		}
	}
	return true
}

func (tracker *routeLifecycleTracker) streamComplete(streamID string, state *observedStreamLifecycle) bool {
	if state == nil || !state.begun {
		return false
	}
	if state.ended {
		return true
	}
	if state.explicit {
		return false
	}
	hasMIME := false
	for key, route := range tracker.routes {
		if key.streamID != streamID {
			continue
		}
		hasMIME = true
		if !route.begun || !route.ended {
			return false
		}
	}
	return hasMIME
}

func (tracker *routeLifecycleTracker) assertComplete(t *testing.T) {
	t.Helper()
	if tracker == nil || len(tracker.streams) == 0 {
		t.Fatal("route lifecycle: no StreamID routes observed")
	}
	for streamID, state := range tracker.streams {
		if !tracker.streamComplete(streamID, state) {
			t.Errorf("route lifecycle: StreamID %q = %#v, want complete route", streamID, state)
		}
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

func TestRouteLifecycleTrackerAcceptsInterleavedExplicitControlRoutes(t *testing.T) {
	tracker := newRouteLifecycleTracker()
	for _, chunk := range []*genx.MessageChunk{
		{Ctrl: &genx.StreamCtrl{StreamID: "audio-turn", BeginOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "text-turn", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-turn", BeginOfStream: true}},
		{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "text-turn", BeginOfStream: true}},
		{Part: &genx.Blob{MIMEType: "audio/opus", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "audio-turn"}},
		{Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: "text-turn"}},
		{Part: &genx.Blob{MIMEType: "audio/opus"}, Ctrl: &genx.StreamCtrl{StreamID: "audio-turn", EndOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "text-turn", EndOfStream: true}},
		{Ctrl: &genx.StreamCtrl{StreamID: "audio-turn", EndOfStream: true}},
	} {
		observeRouteLifecycle(t, tracker, chunk)
	}
	tracker.assertComplete(t)
	if len(tracker.streams) != 2 || len(tracker.routes) != 2 {
		t.Fatalf("lifecycles = streams %d routes %d, want 2/2", len(tracker.streams), len(tracker.routes))
	}
	if route := tracker.route("text-turn", "text/plain"); route == nil || !route.ended {
		t.Fatalf("control EOS did not close text MIME route: %#v", route)
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

	explicit := newRouteLifecycleTracker()
	observeRouteLifecycle(t, explicit, &genx.MessageChunk{
		Ctrl: &genx.StreamCtrl{StreamID: "control", BeginOfStream: true},
	})
	if explicit.allComplete() {
		t.Fatal("explicit control route without EOS reported complete")
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
		{name: "duplicate MIME EOS", chunks: []*genx.MessageChunk{validBOS, validEOS, validEOS.Clone()}},
		{name: "audio after MIME EOS", chunks: []*genx.MessageChunk{
			{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}, Ctrl: &genx.StreamCtrl{StreamID: "turn"}},
		}},
		{name: "metadata changed", chunks: []*genx.MessageChunk{
			{Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "assistant", BeginOfStream: true}},
			{Role: genx.RoleModel, Name: "assistant", Part: genx.Text("late"), Ctrl: &genx.StreamCtrl{StreamID: "turn", Label: "other"}},
		}},
		{name: "missing control", chunks: []*genx.MessageChunk{{Part: genx.Text("late")}}},
		{name: "missing StreamID", chunks: []*genx.MessageChunk{{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{BeginOfStream: true}}}},
		{name: "invalid MIME", chunks: []*genx.MessageChunk{{Part: &genx.Blob{MIMEType: "not a mime"}, Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}}}},
		{name: "control EOS before BOS", chunks: []*genx.MessageChunk{{Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}}}},
		{name: "duplicate control BOS", chunks: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
			{Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
		}},
		{name: "post control EOS", chunks: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
			{Ctrl: &genx.StreamCtrl{StreamID: "turn", EndOfStream: true}},
			{Part: genx.Text("late"), Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true}},
		}},
		{name: "control boundary missing StreamID", chunks: []*genx.MessageChunk{{Ctrl: &genx.StreamCtrl{BeginOfStream: true}}}},
		{name: "error without EOS", chunks: []*genx.MessageChunk{{Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: "turn", BeginOfStream: true, Error: "early"}}}},
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
