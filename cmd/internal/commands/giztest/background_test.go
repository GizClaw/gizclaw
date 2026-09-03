package giztest

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func backgroundListenDocument(steps []Step) *Document {
	return &Document{
		Name: "background-listen", Path: "background-listen.giztest.yaml", Repeat: 1,
		Variables: map[string]VariableSpec{
			"received": {Direction: "output", Type: "audio", MaxBytes: 65536, MediaType: "audio/ogg", Codec: "opus"},
		},
		Clients: map[string]ClientSpec{"peer": {}},
		Steps:   steps,
	}
}

// fakeClientRunOptions hands out the given streams in order; background steps
// open their PeerStream concurrently, so the cursor is atomic.
func fakeClientRunOptions(streams ...*fakeRelayStream) runOptions {
	var index atomic.Int64
	return runOptions{
		connectClients: func(context.Context, map[string]ClientSpec, []Step, *variables) (*clientSet, error) {
			return &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}, nil
		},
		openPeerStream: func(*gizcli.Client) peerStreamOpener {
			return func() (peerStream, error) {
				return streams[int(index.Add(1)-1)%len(streams)], nil
			}
		},
	}
}

func TestRunTaskAwaitAppliesBackgroundListenResult(t *testing.T) {
	stream := newFakeRelayStream()
	_, packets := testOggOpus(t)
	minimum := float64(1)
	doc := backgroundListenDocument([]Step{
		{ID: "listen", Client: "peer", Background: true, PeerStream: &PeerStreamOperation{Mode: "listen", Duration: "300ms"}},
		{ID: "wait", Await: "listen", Timeout: "5s", Capture: map[string]string{"received": "/audio"}, Expect: map[string]Expectation{
			"/audio_bytes": {Minimum: &minimum},
			"/packets":     {Equals: len(packets)},
		}},
	})
	go func() {
		for _, packet := range packets {
			stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "remote", Label: "participant"}}
		}
	}()
	result := runTask(context.Background(), task{doc: doc}, fakeClientRunOptions(stream))
	if result.Status != "passed" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps = %#v", result.Steps)
	}
	started := result.Steps[0]
	if started.Status != "started" || started.Operation != "peer_stream" || started.Evidence["await"] != "wait" || started.Evidence["background"] != true {
		t.Fatalf("background step report = %#v", started)
	}
	awaited := result.Steps[1]
	if awaited.Status != "passed" || awaited.Operation != "await" || awaited.Client != "peer" || awaited.Evidence["step"] != "listen" || awaited.Evidence["packets"] != len(packets) {
		t.Fatalf("await step report = %#v", awaited)
	}
	if _, ok := awaited.Evidence["background_duration_ms"]; !ok {
		t.Fatalf("await evidence lacks background duration: %#v", awaited.Evidence)
	}
	select {
	case <-stream.closed:
	default:
		t.Fatal("background PeerStream was not closed after await")
	}
}

func TestRunTaskAwaitSurfacesBackgroundFailure(t *testing.T) {
	stream := newFakeRelayStream()
	doc := backgroundListenDocument([]Step{
		{ID: "listen", Client: "peer", Background: true, PeerStream: &PeerStreamOperation{Mode: "listen", Duration: "5s"}},
		{ID: "wait", Await: "listen", Timeout: "5s"},
	})
	close(stream.in)
	result := runTask(context.Background(), task{doc: doc}, fakeClientRunOptions(stream))
	if result.Status != "failed" || len(result.Steps) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.Steps[0].Status != "started" {
		t.Fatalf("background step report = %#v", result.Steps[0])
	}
	awaited := result.Steps[1]
	if awaited.Status != "failed" || !strings.Contains(awaited.Error, "closed before the listen duration") || awaited.Evidence["step"] != "listen" {
		t.Fatalf("await step report = %#v", awaited)
	}
}

func TestRunTaskAwaitTimeoutCancelsBackgroundAndReportsUnawaited(t *testing.T) {
	first := newFakeRelayStream()
	second := newFakeRelayStream()
	doc := backgroundListenDocument([]Step{
		{ID: "first", Client: "peer", Background: true, PeerStream: &PeerStreamOperation{Mode: "listen", Duration: "1m"}},
		{ID: "second", Client: "peer", Background: true, PeerStream: &PeerStreamOperation{Mode: "listen", Duration: "1m"}},
		{ID: "wait_first", Await: "first", Timeout: "100ms"},
		{ID: "wait_second", Await: "second", Timeout: "5s"},
	})
	started := time.Now()
	result := runTask(context.Background(), task{doc: doc}, fakeClientRunOptions(first, second))
	if time.Since(started) > 5*time.Second {
		t.Fatal("aborted task waited for the full listen duration")
	}
	if result.Status != "failed" || len(result.Steps) != 4 {
		t.Fatalf("result = %#v", result)
	}
	awaited := result.Steps[2]
	if awaited.Status != "failed" || !strings.Contains(awaited.Error, "cancelled before it finished") || awaited.Evidence["deadline"] != "timeout" {
		t.Fatalf("await step report = %#v", awaited)
	}
	cancelled := result.Steps[3]
	if cancelled.ID != "second" || cancelled.Status != "cancelled" || cancelled.Stage != "background" || !strings.Contains(cancelled.Error, "cancelled before await step wait_second") {
		t.Fatalf("cancelled step report = %#v", cancelled)
	}
	for name, stream := range map[string]*fakeRelayStream{"first": first, "second": second} {
		select {
		case <-stream.closed:
		default:
			t.Fatalf("%s PeerStream was not closed", name)
		}
	}
}

func TestRunTaskRejectsBackgroundStepsInPlayMode(t *testing.T) {
	doc := backgroundListenDocument([]Step{
		{ID: "listen", Client: "peer", Background: true, PeerStream: &PeerStreamOperation{Mode: "listen", Duration: "1s"}},
		{ID: "wait", Await: "listen"},
	})
	opts := fakeClientRunOptions(newFakeRelayStream())
	opts.audioObserver = func(string, string, []byte, bool) error { return nil }
	result := runTask(context.Background(), task{doc: doc}, opts)
	if result.Status != "failed" || len(result.Steps) != 1 || !strings.Contains(result.Steps[0].Error, "play audio") {
		t.Fatalf("result = %#v", result)
	}
}

func TestLoadDocumentAcceptsBackgroundAwaitListen(t *testing.T) {
	doc, err := loadDocument(writeTestDocument(t, validDocument+`  - id: listen
    client: peer
    background: true
    peer_stream:
      mode: listen
      duration: 3s
  - id: wait
    await: listen
    timeout: 10s
    expect:
      /audio_bytes:
        equals: 0
`))
	if err != nil {
		t.Fatalf("background listen document rejected: %v", err)
	}
	listen, wait := doc.Steps[1], doc.Steps[2]
	if !listen.Background || listen.PeerStream.Mode != "listen" || listen.PeerStream.Duration != "3s" {
		t.Fatalf("listen step = %#v", listen)
	}
	if wait.Await != "listen" || wait.operation() != "await" || operationNeedsClient(wait.operation()) {
		t.Fatalf("await step = %#v", wait)
	}
}

func TestLoadDocumentRejectsInvalidBackgroundAwait(t *testing.T) {
	listen := func(extra string) string {
		return "  - id: listen\n    client: peer\n    background: true\n    peer_stream:\n      mode: listen\n      duration: 3s\n" + extra
	}
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"background never awaited":  {body: listen(""), want: "must be awaited exactly once"},
		"await unknown step":        {body: listen("  - id: wait\n    await: other\n"), want: "not an earlier background step"},
		"await before background":   {body: "  - id: wait\n    await: listen\n" + listen(""), want: "not an earlier background step"},
		"await twice":               {body: listen("  - id: wait\n    await: listen\n  - id: again\n    await: listen\n"), want: "already awaited"},
		"await with client":         {body: listen("  - id: wait\n    client: peer\n    await: listen\n"), want: "takes its client"},
		"await with operation":      {body: listen("  - id: wait\n    await: listen\n    rpc:\n      method: all.ping\n      request: {}\n"), want: "schema validation"},
		"await with retry":          {body: listen("  - id: wait\n    await: listen\n    retry:\n      attempts: 2\n"), want: "does not support retry"},
		"background rpc":            {body: "  - id: bg\n    client: peer\n    background: true\n    rpc:\n      method: all.ping\n      request: {}\n  - id: wait\n    await: bg\n", want: "requires a peer_stream"},
		"background expect":         {body: "  - id: listen\n    client: peer\n    background: true\n    peer_stream:\n      mode: listen\n      duration: 3s\n    expect:\n      /audio_bytes:\n        equals: 0\n  - id: wait\n    await: listen\n", want: "cannot capture, expect, or save_as"},
		"background retry":          {body: "  - id: listen\n    client: peer\n    background: true\n    retry:\n      attempts: 2\n    peer_stream:\n      mode: listen\n      duration: 3s\n  - id: wait\n    await: listen\n", want: "cannot retry"},
		"background session":        {body: "  - id: turn\n    client: peer\n    background: true\n    peer_stream:\n      mode: realtime\n      input: ${endpoint}\n      session: mic\n      keep_open: true\n  - id: wait\n    await: turn\n", want: "cannot use a session"},
		"background in finally":     {body: listen("  - id: wait\n    await: listen\n") + "finally:\n  - id: late\n    client: peer\n    background: true\n    peer_stream:\n      mode: listen\n      duration: 1s\n", want: "schema validation"},
		"background and await":      {body: listen("  - id: wait\n    await: listen\n    background: true\n"), want: "cannot be both background and await"},
		"await in finally":          {body: listen("  - id: wait\n    await: listen\n") + "finally:\n  - id: late\n    await: listen\n", want: "schema validation"},
		"await before finally only": {body: listen("") + "finally:\n  - id: cleanup\n    client: peer\n    rpc:\n      method: all.ping\n      request: {}\n", want: "must be awaited exactly once"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadDocument(writeTestDocument(t, validDocument+tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

// stuckPeerStreamOptions hands out an opener that blocks until release is
// closed, modeling a PeerStream that ignores cancellation: invocation.run
// never returns, so every wait on the background goroutine must be bounded.
func stuckPeerStreamOptions(release <-chan struct{}, opened chan<- struct{}) runOptions {
	var once sync.Once
	return runOptions{
		connectClients: func(context.Context, map[string]ClientSpec, []Step, *variables) (*clientSet, error) {
			return &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}, nil
		},
		openPeerStream: func(*gizcli.Client) peerStreamOpener {
			return func() (peerStream, error) {
				once.Do(func() { close(opened) })
				<-release
				return nil, context.Canceled
			}
		},
		backgroundCancelGrace: 100 * time.Millisecond,
	}
}

func TestRunTaskAwaitTimeoutIsBoundedWhenStreamIgnoresCancellation(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	opened := make(chan struct{})
	doc := backgroundListenDocument([]Step{
		{ID: "listen", Client: "peer", Background: true, PeerStream: &PeerStreamOperation{Mode: "listen", Duration: "1m"}},
		{ID: "wait", Await: "listen", Timeout: "100ms"},
	})
	started := time.Now()
	result := runTask(context.Background(), task{doc: doc}, stuckPeerStreamOptions(release, opened))
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("await waited %s for a stream that ignores cancellation", elapsed)
	}
	<-opened
	if result.Status != "failed" || len(result.Steps) != 2 {
		t.Fatalf("result = %#v", result)
	}
	awaited := result.Steps[1]
	if awaited.Status != "failed" || !strings.Contains(awaited.Error, "did not stop within") {
		t.Fatalf("await step report = %#v", awaited)
	}
	if awaited.Evidence["unfinished"] != true || awaited.Evidence["deadline"] != "timeout" {
		t.Fatalf("await evidence = %#v", awaited.Evidence)
	}
}

// A background goroutine that still owns the PeerStream also owns the task's
// shared clients, so finalizers must not run and the clients must not be torn
// down underneath it.
func TestRunTaskSkipsFinalizersWhileBackgroundStepStillOwnsStream(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	opened := make(chan struct{})
	doc := backgroundListenDocument([]Step{
		{ID: "listen", Client: "peer", Background: true, PeerStream: &PeerStreamOperation{Mode: "listen", Duration: "1m"}},
		{ID: "wait", Await: "listen", Timeout: "100ms"},
	})
	doc.Finally = []Step{{ID: "cleanup", Output: &OutputOperation{Variable: "received"}}}
	result := runTask(context.Background(), task{doc: doc}, stuckPeerStreamOptions(release, opened))
	<-opened
	if result.Status != "failed" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Cleanup) != 0 {
		t.Fatalf("finally steps ran while the PeerStream was still owned: %#v", result.Cleanup)
	}
	if !strings.Contains(result.Error, "still hold their PeerStream") {
		t.Fatalf("result error = %q, want the retained PeerStream report", result.Error)
	}
}
