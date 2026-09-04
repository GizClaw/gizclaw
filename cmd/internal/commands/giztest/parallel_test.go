package giztestcmd

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func parallelListenDocument(steps []giztest.Step) *giztest.Document {
	return &giztest.Document{
		Name: "parallel-listen", Path: "parallel-listen.giztest.yaml", Repeat: 1,
		Variables: map[string]giztest.VariableSpec{
			"received": {Direction: "output", Type: "audio", MaxBytes: 65536, MediaType: "audio/ogg", Codec: "opus"},
		},
		Clients: map[string]giztest.ClientSpec{"peer": {}, "other": {}},
		Steps:   steps,
	}
}

// fakeClientDriver gives client "peer" the first stream and client "other"
// the second, so a test can tell the two children of one parallel step apart
// no matter which order their goroutines opened their PeerStream in.
func fakeClientDriver(streams ...*fakeRelayStream) *driver {
	clients := map[string]*gizcli.Client{"peer": {}, "other": {}}
	byClient := map[*gizcli.Client]*fakeRelayStream{
		clients["peer"]:  streams[0],
		clients["other"]: streams[len(streams)-1],
	}
	return &driver{
		speechCache: newSpeechFixtureCache(),
		connectClients: func(context.Context, map[string]giztest.ClientSpec, []giztest.Step, *giztest.Variables) (*clientSet, error) {
			return &clientSet{clients: clients}, nil
		},
		openPeerStream: func(client *gizcli.Client) peerStreamOpener {
			return func() (peerStream, error) { return byClient[client], nil }
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

func listenChild(id, client, duration string) giztest.Step {
	return giztest.Step{ID: id, Client: client, PeerStream: &giztest.PeerStreamOperation{Mode: "listen", Duration: duration}}
}

func TestRunParallelAppliesChildListenResults(t *testing.T) {
	first, second := newFakeRelayStream(), newFakeRelayStream()
	_, packets := testOggOpus(t)
	minimum := float64(1)
	doc := parallelListenDocument([]giztest.Step{{
		ID: "listen_together", Timeout: "5s",
		Parallel: []giztest.Step{listenChild("one", "peer", "300ms"), listenChild("two", "other", "300ms")},
		Capture:  map[string]string{"received": "/one/audio"},
		Expect: map[string]giztest.Expectation{
			"/one/audio_bytes": {Minimum: &minimum},
			"/one/packets":     {Equals: len(packets)},
			"/two/packets":     {Equals: len(packets)},
		},
	}})
	for _, stream := range []*fakeRelayStream{first, second} {
		go func() {
			for _, packet := range packets {
				stream.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "remote", Label: "participant"}}
			}
		}()
	}
	result := runSingleTask(t, doc, fakeClientDriver(first, second))
	if result.Status != "passed" || len(result.Steps) != 1 {
		t.Fatalf("result = %#v", result)
	}
	step := result.Steps[0]
	if step.Operation != "parallel" || step.Status != "passed" || len(step.Children) != 2 {
		t.Fatalf("parallel step report = %#v", step)
	}
	for _, child := range step.Children {
		if child.Status != "passed" || child.Evidence["packets"] != len(packets) {
			t.Fatalf("child report = %#v", child)
		}
	}
	for name, stream := range map[string]*fakeRelayStream{"one": first, "two": second} {
		select {
		case <-stream.closed:
		default:
			t.Fatalf("%s PeerStream was not closed after the parallel step", name)
		}
	}
}

func TestRunParallelSurfacesOneChildFailure(t *testing.T) {
	first, second := newFakeRelayStream(), newFakeRelayStream()
	_, packets := testOggOpus(t)
	doc := parallelListenDocument([]giztest.Step{{
		ID: "listen_together", Timeout: "5s",
		Parallel: []giztest.Step{listenChild("one", "peer", "300ms"), listenChild("two", "other", "5s")},
	}})
	go func() {
		for _, packet := range packets {
			first.in <- &genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}, Ctrl: &genx.StreamCtrl{StreamID: "remote", Label: "participant"}}
		}
	}()
	// "two" fails: its PeerStream ends before its listen window does.
	close(second.in)
	result := runSingleTask(t, doc, fakeClientDriver(first, second))
	if result.Status != "failed" || len(result.Steps) != 1 {
		t.Fatalf("result = %#v", result)
	}
	step := result.Steps[0]
	if !strings.Contains(step.Error, "closed before the listen duration") {
		t.Fatalf("parallel step report = %#v", step)
	}
	if step.Children[0].Status != "passed" || step.Children[0].ID != "one" ||
		step.Children[1].Status != "failed" || step.Children[1].ID != "two" {
		t.Fatalf("child reports = %#v", step.Children)
	}
}

func TestRunParallelTimeoutClosesEveryChildStream(t *testing.T) {
	first, second := newFakeRelayStream(), newFakeRelayStream()
	doc := parallelListenDocument([]giztest.Step{{
		ID: "listen_together", Timeout: "100ms",
		Parallel: []giztest.Step{listenChild("one", "peer", "1m"), listenChild("two", "other", "1m")},
	}})
	started := time.Now()
	result := runSingleTask(t, doc, fakeClientDriver(first, second))
	if time.Since(started) > 10*time.Second {
		t.Fatal("timed-out parallel step waited for the full listen duration")
	}
	if result.Status != "failed" || len(result.Steps) != 1 {
		t.Fatalf("result = %#v", result)
	}
	step := result.Steps[0]
	if !strings.Contains(step.Error, "cancelled its children before they finished") || len(step.Children) != 2 {
		t.Fatalf("parallel step report = %#v", step)
	}
	for name, stream := range map[string]*fakeRelayStream{"one": first, "two": second} {
		select {
		case <-stream.closed:
		default:
			t.Fatalf("%s PeerStream was not closed", name)
		}
	}
}

func TestRunRejectsParallelStepsInPlayMode(t *testing.T) {
	doc := parallelListenDocument([]giztest.Step{{
		ID: "listen_together", Parallel: []giztest.Step{listenChild("one", "peer", "1s"), listenChild("two", "other", "1s")},
	}})
	d := fakeClientDriver(newFakeRelayStream())
	d.audioObserver = func(string, string, []byte, bool) error { return nil }
	result := runSingleTask(t, doc, d)
	if result.Status != "failed" || len(result.Steps) != 1 || !strings.Contains(result.Steps[0].Error, "play audio") {
		t.Fatalf("result = %#v", result)
	}
}

// The parallel step owns the capture map, so the /audio bound it declared for
// one child is what that child's PeerStream enforces.
func TestPrepareParallelUsesParentCaptureBound(t *testing.T) {
	d := fakeClientDriver(newFakeRelayStream())
	sess := testDriverSession(d, &clientSet{clients: map[string]*gizcli.Client{"peer": {}}})
	vars := mustVariables(t, map[string]giztest.VariableSpec{
		"received": {Direction: "output", Type: "audio", MaxBytes: 4096, MediaType: "audio/ogg", Codec: "opus"},
	})
	child := listenChild("one", "peer", "1s")
	parent := giztest.Step{ID: "listen_together", Parallel: []giztest.Step{child}, Capture: map[string]string{"received": "/one/audio"}}
	parallel, ok := sess.(giztest.ParallelSession)
	if !ok {
		t.Fatal("CLI session does not implement giztest.ParallelSession")
	}
	run, err := parallel.PrepareParallel(giztest.StepRequest{Step: child, Vars: vars, Parent: &parent})
	if err != nil {
		t.Fatal(err)
	}
	invocation, ok := run.(peerStreamInvocation)
	if !ok || invocation.audioCaptureMaxBytes != 4096 {
		t.Fatalf("prepared invocation = %#v", run)
	}
	ping := giztest.Step{ID: "ping", Client: "peer", RPC: &giztest.RPCOperation{Method: "all.ping"}}
	if _, err := parallel.PrepareParallel(giztest.StepRequest{Step: ping, Vars: vars, Parent: &parent}); err == nil {
		t.Fatal("non peer_stream parallel child was prepared")
	}
}
