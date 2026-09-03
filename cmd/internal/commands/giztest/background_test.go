package giztestcmd

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func backgroundListenDocument(steps []giztest.Step) *giztest.Document {
	return &giztest.Document{
		Name: "background-listen", Path: "background-listen.giztest.yaml", Repeat: 1,
		Variables: map[string]giztest.VariableSpec{
			"received": {Direction: "output", Type: "audio", MaxBytes: 65536, MediaType: "audio/ogg", Codec: "opus"},
		},
		Clients: map[string]giztest.ClientSpec{"peer": {}},
		Steps:   steps,
	}
}

// fakeClientDriver hands out the given streams in order; background steps
// open their PeerStream concurrently, so the cursor is atomic.
func fakeClientDriver(streams ...*fakeRelayStream) *driver {
	var index atomic.Int64
	return &driver{
		speechCache: newSpeechFixtureCache(),
		connectClients: func(context.Context, map[string]giztest.ClientSpec, []giztest.Step, *giztest.Variables) (*clientSet, error) {
			return &clientSet{clients: map[string]*gizcli.Client{"peer": {}}}, nil
		},
		openPeerStream: func(*gizcli.Client) peerStreamOpener {
			return func() (peerStream, error) {
				return streams[int(index.Add(1)-1)%len(streams)], nil
			}
		},
	}
}

// runSingleTask runs doc once through the engine with the driver's
// substituted transports and returns its task report.
func runSingleTask(t *testing.T, doc *giztest.Document, d *driver) giztest.TaskReport {
	t.Helper()
	report := giztest.Run(context.Background(), []*giztest.Document{doc}, giztest.Options{Driver: d, Parallel: 1, Out: io.Discard})
	if len(report.Tasks) != 1 {
		t.Fatalf("report = %#v", report)
	}
	return report.Tasks[0]
}

func TestRunAwaitAppliesBackgroundListenResult(t *testing.T) {
	stream := newFakeRelayStream()
	_, packets := testOggOpus(t)
	minimum := float64(1)
	doc := backgroundListenDocument([]giztest.Step{
		{ID: "listen", Client: "peer", Background: true, PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Duration: "300ms"}},
		{ID: "wait", Await: "listen", Timeout: "5s", Capture: map[string]string{"received": "/audio"}, Expect: map[string]giztest.Expectation{
			"/audio_bytes": {Minimum: &minimum},
			"/packets":     {Equals: len(packets)},
		}},
	})
	go func() {
		for _, packet := range packets {
			stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "remote", Label: "participant"}}
		}
	}()
	result := runSingleTask(t, doc, fakeClientDriver(stream))
	if result.Status != "passed" || len(result.Steps) != 2 {
		t.Fatalf("result = %#v", result)
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

func TestRunAwaitSurfacesBackgroundListenFailure(t *testing.T) {
	stream := newFakeRelayStream()
	doc := backgroundListenDocument([]giztest.Step{
		{ID: "listen", Client: "peer", Background: true, PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Duration: "5s"}},
		{ID: "wait", Await: "listen", Timeout: "5s"},
	})
	close(stream.in)
	result := runSingleTask(t, doc, fakeClientDriver(stream))
	if result.Status != "failed" || len(result.Steps) != 2 || result.Steps[0].Status != "started" {
		t.Fatalf("result = %#v", result)
	}
	awaited := result.Steps[1]
	if awaited.Status != "failed" || !strings.Contains(awaited.Error, "closed before the listen duration") || awaited.Evidence["step"] != "listen" {
		t.Fatalf("await step report = %#v", awaited)
	}
}

func TestRunAwaitTimeoutClosesBackgroundListenStreams(t *testing.T) {
	first := newFakeRelayStream()
	second := newFakeRelayStream()
	doc := backgroundListenDocument([]giztest.Step{
		{ID: "first", Client: "peer", Background: true, PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Duration: "1m"}},
		{ID: "second", Client: "peer", Background: true, PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Duration: "1m"}},
		{ID: "wait_first", Await: "first", Timeout: "100ms"},
		{ID: "wait_second", Await: "second", Timeout: "5s"},
	})
	started := time.Now()
	result := runSingleTask(t, doc, fakeClientDriver(first, second))
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

func TestRunRejectsBackgroundStepsInPlayMode(t *testing.T) {
	doc := backgroundListenDocument([]giztest.Step{
		{ID: "listen", Client: "peer", Background: true, PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Duration: "1s"}},
		{ID: "wait", Await: "listen"},
	})
	d := fakeClientDriver(newFakeRelayStream())
	d.audioObserver = func(string, string, []byte, bool) error { return nil }
	result := runSingleTask(t, doc, d)
	if result.Status != "failed" || len(result.Steps) != 1 || !strings.Contains(result.Steps[0].Error, "play audio") {
		t.Fatalf("result = %#v", result)
	}
}

// The await step owns the capture map of a background step, so its /audio
// bound is what the background PeerStream enforces.
func TestPrepareBackgroundUsesAwaiterCaptureBound(t *testing.T) {
	d := fakeClientDriver(newFakeRelayStream())
	sess := testDriverSession(d, &clientSet{clients: map[string]*gizcli.Client{"peer": {}}})
	vars := mustVariables(t, map[string]giztest.VariableSpec{
		"received": {Direction: "output", Type: "audio", MaxBytes: 4096, MediaType: "audio/ogg", Codec: "opus"},
	})
	listen := giztest.Step{ID: "listen", Client: "peer", Background: true, PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Duration: "1s"}}
	await := giztest.Step{ID: "wait", Await: "listen", Capture: map[string]string{"received": "/audio"}}
	background, ok := sess.(giztest.BackgroundSession)
	if !ok {
		t.Fatal("CLI session does not implement giztest.BackgroundSession")
	}
	run, err := background.PrepareBackground(giztest.StepRequest{Step: listen, Vars: vars, Awaiter: &await})
	if err != nil {
		t.Fatal(err)
	}
	invocation, ok := run.(peerStreamInvocation)
	if !ok || invocation.audioCaptureMaxBytes != 4096 {
		t.Fatalf("prepared invocation = %#v", run)
	}
	if _, err := background.PrepareBackground(giztest.StepRequest{Step: giztest.Step{ID: "ping", Client: "peer", Background: true, RPC: &giztest.RPCOperation{Method: "all.ping"}}, Vars: vars, Awaiter: &await}); err == nil {
		t.Fatal("non peer_stream background step was prepared")
	}
}
