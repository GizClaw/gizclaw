package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/openaihttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	telemetrypb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/telemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/openaiapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"google.golang.org/protobuf/proto"
)

func TestPeerConnHelpersAndRPCHandle(t *testing.T) {
	t.Run("audio mixer lifecycle", func(t *testing.T) {
		var nilPeer *PeerConn
		if _, err := nilPeer.audioMixer(); err != ErrNilPeerConn {
			t.Fatalf("audioMixer(nil) err = %v, want %v", err, ErrNilPeerConn)
		}

		peer := &PeerConn{}
		if _, err := peer.audioMixer(); err != ErrNilPeerConnMixer {
			t.Fatalf("audioMixer() err = %v, want %v", err, ErrNilPeerConnMixer)
		}

		peer.init()
		if _, err := peer.audioMixer(); err != nil {
			t.Fatalf("audioMixer() after init error = %v", err)
		}

		track, ctrl, err := peer.CreateAudioTrack()
		if err != nil {
			t.Fatalf("CreateAudioTrack() error = %v", err)
		}
		if track == nil || ctrl == nil {
			t.Fatalf("CreateAudioTrack() = (%v, %v)", track, ctrl)
		}
		if err := peer.close(); err != nil {
			t.Fatalf("close() error = %v", err)
		}
		if !peer.isClosed() {
			t.Fatal("peer should be closed")
		}
	})

	t.Run("dispatch missing params", func(t *testing.T) {
		server := &rpcServer{}
		resp, err := server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "missing",
			Method: rpcapi.RPCMethodAllPing,
		})
		if err != nil {
			t.Fatalf("dispatch() error = %v", err)
		}
		if resp == nil || resp.Error == nil || resp.Error.Code != rpcapi.RPCErrorCodeInvalidParams {
			t.Fatalf("dispatch() response = %+v", resp)
		}
	})

	t.Run("dispatch ping and unknown method", func(t *testing.T) {
		server := &rpcServer{}
		params, err := newRPCPingRequestParams(rpcapi.PingRequest{})
		if err != nil {
			t.Fatalf("newRPCPingRequestParams() error = %v", err)
		}
		resp, err := server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "ping",
			Method: rpcapi.RPCMethodAllPing,
			Params: params,
		})
		if err != nil {
			t.Fatalf("dispatch(ping) error = %v", err)
		}
		if resp == nil || resp.Result == nil {
			t.Fatalf("dispatch(ping) response = %+v", resp)
		}
		result, err := resp.Result.AsPingResponse()
		if err != nil {
			t.Fatalf("dispatch(ping) result decode error = %v", err)
		}
		if result.ServerTime <= 0 {
			t.Fatalf("dispatch(ping) response = %+v", result)
		}

		resp, err = server.dispatch(context.Background(), &rpcapi.RPCRequest{
			Id:     "unknown",
			Method: "rpc.unknown",
		})
		if err != nil {
			t.Fatalf("dispatch(unknown) error = %v", err)
		}
		if resp == nil || resp.Error == nil || !strings.Contains(resp.Error.Message, "unknown method") {
			t.Fatalf("dispatch(unknown) response = %+v", resp)
		}
	})

	t.Run("openai handler routes under v1", func(t *testing.T) {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair() error = %v", err)
		}
		var voiceRequests []adminhttp.ListVoicesRequestObject
		handler := newOpenAIHTTPHandler(&openaiapi.Server{
			Caller: keyPair.Public,
			Models: peerConnModelListerFunc(func(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
				return adminhttp.ListModels200JSONResponse(adminhttp.ModelList{Items: []apitypes.Model{
					{Id: "chat", Provider: apitypes.ModelProvider{Name: "main"}},
				}}), nil
			}),
			Authorizer: peerConnAuthorizerFunc(func(_ context.Context, req acl.AuthorizeRequest) error {
				if req.Subject.Id != keyPair.Public.String() {
					t.Fatalf("subject = %q, want current peer public key", req.Subject.Id)
				}
				return nil
			}),
			Voices: peerConnVoiceListerFunc(func(_ context.Context, req adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
				voiceRequests = append(voiceRequests, req)
				return adminhttp.ListVoices200JSONResponse(adminhttp.VoiceList{Items: []apitypes.Voice{
					{
						Id: "voice-a",
						Provider: apitypes.VoiceProvider{
							Kind: apitypes.VoiceProviderKindOpenaiTenant,
							Name: "main",
						},
						Source: apitypes.VoiceSourceManual,
					},
				}}), nil
			}),
		})

		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET /v1/models status = %d body=%s", resp.Code, resp.Body.String())
		}
		var models openaihttp.ListModelsResponse
		if err := json.Unmarshal(resp.Body.Bytes(), &models); err != nil {
			t.Fatalf("decode /v1/models response: %v", err)
		}
		if len(models.Data) != 1 || models.Data[0].Id != "chat" {
			t.Fatalf("/v1/models response = %#v", models)
		}

		req = httptest.NewRequest(http.MethodGet, "/v1/voices?source=sync&providerKind=minimax-tenant&providerName=main&limit=10", nil)
		resp = httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET /v1/voices status = %d body=%s", resp.Code, resp.Body.String())
		}
		var voices struct {
			Object string           `json:"object"`
			Data   []apitypes.Voice `json:"data"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &voices); err != nil {
			t.Fatalf("decode /v1/voices response: %v", err)
		}
		if voices.Object != "list" || len(voices.Data) != 1 || voices.Data[0].Id != "voice-a" {
			t.Fatalf("/v1/voices response = %#v", voices)
		}
		if len(voiceRequests) != 1 {
			t.Fatalf("voice requests = %d, want 1", len(voiceRequests))
		}
		params := voiceRequests[0].Params
		if params.Source == nil || *params.Source != apitypes.VoiceSourceSync {
			t.Fatalf("voice source param = %#v", params.Source)
		}
		if params.ProviderKind == nil || *params.ProviderKind != apitypes.VoiceProviderKindMinimaxTenant {
			t.Fatalf("voice providerKind param = %#v", params.ProviderKind)
		}
		if params.ProviderName == nil || *params.ProviderName != "main" {
			t.Fatalf("voice providerName param = %#v", params.ProviderName)
		}

		req = httptest.NewRequest(http.MethodGet, "/models", nil)
		resp = httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("GET /models status = %d, want 404", resp.Code)
		}
	})
}

func TestPeerConnPacesMixedAudioAtEgress(t *testing.T) {
	mx := pcm.NewMixer(peerConnMixerFormat)
	track, ctrl, err := mx.CreateTrack()
	if err != nil {
		t.Fatalf("CreateTrack() error = %v", err)
	}
	frame := make([]byte, peerConnMixerFormat.BytesInDuration(peerConnOpusFrameDuration))
	for i := range frame {
		frame[i] = byte(i)
	}
	if err := track.Write(peerConnMixerFormat.DataChunk(append(append([]byte(nil), frame...), frame...))); err != nil {
		t.Fatalf("track.Write() error = %v", err)
	}
	if err := track.Write(peerConnMixerFormat.DataChunk(frame)); err != nil {
		t.Fatalf("track.Write() error = %v", err)
	}
	if err := ctrl.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error = %v", err)
	}

	conn := &recordingGiznetConn{written: make(chan struct{}, 3)}
	ticks := make(chan time.Time)
	peer := &PeerConn{Conn: conn, mixer: mx, audioPacing: ticks}
	result := make(chan error, 1)
	go func() {
		_, err := peer.streamMixedAudio(false)
		result <- err
	}()

	for want := 1; want <= 3; want++ {
		ticks <- time.Now()
		select {
		case <-conn.written:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for packet %d", want)
		}
		if got := conn.packetCount(); got != want {
			t.Fatalf("packet count after tick %d = %d, want %d", want, got, want)
		}
		select {
		case <-conn.written:
			t.Fatalf("wrote packet without the next pacing tick")
		case <-time.After(10 * time.Millisecond):
		}
	}
	for index, packet := range conn.recordedPackets() {
		if ticks := codecconv.OpusPacketRTPTicks(packet); ticks != 960 {
			t.Fatalf("packet %d RTP ticks = %d, want 960", index, ticks)
		}
	}
	select {
	case <-ctrl.Done():
		t.Fatal("track was marked drained before the next pacing tick")
	default:
	}

	peer.closed.Store(true)
	close(ticks)
	if err := mx.Close(); err != nil {
		t.Fatalf("mixer.Close() error = %v", err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("streamMixedAudio() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("streamMixedAudio() did not stop")
	}
}

type recordingGiznetConn struct {
	testGiznetConn

	mu      sync.Mutex
	packets [][]byte
	written chan struct{}
}

func (c *recordingGiznetConn) Write(protocol byte, packet []byte) (int, error) {
	if protocol != giznet.ProtocolOpusPacket {
		return 0, errors.New("unexpected protocol")
	}
	c.mu.Lock()
	c.packets = append(c.packets, append([]byte(nil), packet...))
	c.mu.Unlock()
	c.written <- struct{}{}
	return len(packet), nil
}

func (c *recordingGiznetConn) packetCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.packets)
}

func (c *recordingGiznetConn) recordedPackets() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	packets := make([][]byte, len(c.packets))
	for i, packet := range c.packets {
		packets[i] = append([]byte(nil), packet...)
	}
	return packets
}

type peerConnModelListerFunc func(context.Context, adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error)

func (f peerConnModelListerFunc) ListModels(ctx context.Context, req adminhttp.ListModelsRequestObject) (adminhttp.ListModelsResponseObject, error) {
	return f(ctx, req)
}

type peerConnVoiceListerFunc func(context.Context, adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error)

func (f peerConnVoiceListerFunc) ListVoices(ctx context.Context, req adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
	return f(ctx, req)
}

type peerConnAuthorizerFunc func(context.Context, acl.AuthorizeRequest) error

func (f peerConnAuthorizerFunc) Authorize(ctx context.Context, req acl.AuthorizeRequest) error {
	return f(ctx, req)
}

func TestPeerConnCloseClosesConn(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(server) error = %v", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(client) error = %v", err)
	}
	clientConn, serverConn := newTestWebRTCConnPair(t, serverKey, clientKey, testGiznetSecurityPolicy{}, testGiznetSecurityPolicy{})
	defer clientConn.Close()

	peer := &PeerConn{Conn: serverConn}
	if err := peer.close(); err != nil {
		t.Fatalf("PeerConn.close() error = %v", err)
	}
	if err := serverConn.Close(); err != nil && !errors.Is(err, giznet.ErrConnClosed) {
		t.Fatalf("server Conn.Close() after PeerConn.close err=%v, want nil or %v", err, giznet.ErrConnClosed)
	}
}

func TestPeerConnCloseStopsAgentRuntime(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	output := newPeerConnBlockingStream()
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: output},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source: agenthost.StreamSourceFunc(func(context.Context) (genx.Stream, error) {
			return agenthost.NewInputStream(1), nil
		}),
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	if _, err := runtime.Reload(ctx); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}
	peer := &PeerConn{agentHost: runtime}
	if err := peer.close(); err != nil {
		t.Fatalf("close() error = %v", err)
	}
	status, err := runtime.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != apitypes.PeerRunStatusStateStopped {
		t.Fatalf("runtime status after close = %+v", status)
	}
	if !output.closed() {
		t.Fatal("agent output stream was not closed")
	}
}

func TestPeerConnHandleTelemetryPacket(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	peerRun := &peerrun.Server{Store: kv.NewMemory(nil)}
	metricStore := &peerConnFakeMetrics{}
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	manager.PeerRun = peerRun
	manager.Metrics = metricStore
	conn := &testGiznetConn{publicKey: keyPair.Public}
	peerConn := &PeerConn{
		Conn:    conn,
		Service: &PeerService{manager: manager},
	}
	percent := 77.0
	charging := true
	payload, err := proto.Marshal(&telemetrypb.TelemetryFrame{
		ObservedAtUnixMs: time.Unix(300, 0).UnixMilli(),
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
				Percent:  &percent,
				Charging: &charging,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := peerConn.handleTelemetryPacket(ctx, payload); err != nil {
		t.Fatalf("handleTelemetryPacket() error = %v", err)
	}
	if len(metricStore.samples) != 2 {
		t.Fatalf("metrics samples = %d, want 2", len(metricStore.samples))
	}
	if metricStore.samples[0].Name != peertelemetry.MetricBatteryPercent || metricStore.samples[0].Value != 77 {
		t.Fatalf("first metric = %+v", metricStore.samples[0])
	}
	status, err := peerRun.GetStatus(ctx, keyPair.Public)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.BatteryPercent == nil || *status.BatteryPercent != 77 {
		t.Fatalf("BatteryPercent = %#v, want 77", status.BatteryPercent)
	}
	if status.Charging == nil || !*status.Charging {
		t.Fatalf("Charging = %#v, want true", status.Charging)
	}
}

func TestPeerConnServeDirectPacketsDoesNotBlockOnTelemetry(t *testing.T) {
	originalShutdownTimeout := peerConnTelemetryShutdownTimeout
	peerConnTelemetryShutdownTimeout = 50 * time.Millisecond
	t.Cleanup(func() { peerConnTelemetryShutdownTimeout = originalShutdownTimeout })

	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	percent := 77.0
	payload, err := proto.Marshal(&telemetrypb.TelemetryFrame{
		ObservedAtUnixMs: time.Unix(300, 0).UnixMilli(),
		Observations: []*telemetrypb.Observation{{
			Body: &telemetrypb.Observation_Battery{Battery: &telemetrypb.BatteryObservation{
				Percent: &percent,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	packets := []peerConnTestPacket{{protocol: EventStreamTelemetry, payload: payload}}
	for i := 0; i < peerConnTelemetryQueueSize+5; i++ {
		packets = append(packets, peerConnTestPacket{protocol: EventStreamTelemetry, payload: payload})
	}
	packets = append(packets, peerConnTestPacket{protocol: giznet.ProtocolOpusPacket, payload: []byte{1, 2, 3}})
	conn := &peerConnPacketConn{
		testGiznetConn: testGiznetConn{publicKey: keyPair.Public},
		packets:        packets,
	}
	metricStore := newPeerConnBlockingMetrics()
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	manager.PeerRun = &peerrun.Server{Store: kv.NewMemory(nil)}
	manager.Metrics = metricStore
	peerConn := &PeerConn{
		Conn:    conn,
		Service: &PeerService{manager: manager},
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- peerConn.serveDirectPackets()
	}()

	select {
	case <-metricStore.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for telemetry metrics append")
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveDirectPackets() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serveDirectPackets stayed blocked behind telemetry shutdown")
	}
	if got, want := conn.reads, len(packets)+1; got != want {
		t.Fatalf("direct packet reads = %d, want %d", got, want)
	}
	close(metricStore.release)
	select {
	case <-metricStore.finished:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for telemetry metrics append to finish")
	}
	_, err = manager.PeerRun.GetStatus(ctx, keyPair.Public)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
}

func TestManagerTelemetryStatusLockIsScopedByPeer(t *testing.T) {
	first, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(first) error = %v", err)
	}
	second, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(second) error = %v", err)
	}
	manager := NewManager(&peer.Server{Store: kv.NewMemory(nil)})
	if a, b := manager.telemetryStatusLock(first.Public), manager.telemetryStatusLock(first.Public); a == nil || a != b {
		t.Fatalf("same peer status locks = %p and %p, want same non-nil lock", a, b)
	}
	if a, b := manager.telemetryStatusLock(first.Public), manager.telemetryStatusLock(second.Public); a == nil || b == nil || a == b {
		t.Fatalf("different peer status locks = %p and %p, want different non-nil locks", a, b)
	}
	retained := manager.retainTelemetryStatusLock(first.Public, true)
	if retained == nil {
		t.Fatal("retainTelemetryStatusLock returned nil")
	}
	manager.releaseTelemetryStatusLock(first.Public)
	if _, ok := manager.telemetryStatusLocks[first.Public]; ok {
		t.Fatal("releaseTelemetryStatusLock should delete unreferenced peer lock")
	}
}

func TestPeerConnTelemetryStatusSyncSerializesCalls(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	next := &peerConnBlockingStatusSync{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	syncer := peerConnTelemetryStatusSync{
		mu:   &sync.Mutex{},
		next: next,
	}
	errCh := make(chan error, 2)
	go func() {
		errCh <- syncer.SyncTelemetryStatus(context.Background(), keyPair.Public, peertelemetry.StatusPatch{BatteryPercent: peerConnIntPtr(10)})
	}()
	select {
	case <-next.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first status sync")
	}
	go func() {
		errCh <- syncer.SyncTelemetryStatus(context.Background(), keyPair.Public, peertelemetry.StatusPatch{Charging: peerConnBoolPtr(true)})
	}()
	select {
	case <-next.entered:
		t.Fatal("second status sync entered before first released")
	case <-time.After(100 * time.Millisecond):
	}
	close(next.release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("SyncTelemetryStatus() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for status sync to finish")
		}
	}
	if next.calls != 2 {
		t.Fatalf("status sync calls = %d, want 2", next.calls)
	}
}

func TestPeerConnReloadsRuntimeWhenInputIsInactive(t *testing.T) {
	ctx := context.Background()
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair error = %v", err)
	}
	store := &peerrun.Server{Store: kv.NewMemory(nil)}
	if _, err := store.SetRunAgent(ctx, keyPair.Public, apitypes.AgentSelection{WorkspaceName: "demo"}); err != nil {
		t.Fatalf("SetRunAgent() error = %v", err)
	}
	source := newPeerRealtimeSource(genx.WithRealtimeStreamDelay(0))
	received := make(chan *genx.MessageChunk, 1)
	runtime := &agenthost.Service{
		Host:      peerConnTestHost{output: &peerConnBlockingStream{done: make(chan struct{})}},
		PeerRun:   store,
		PublicKey: keyPair.Public,
		Source:    source,
		Consumer: agenthost.StreamConsumerFunc(func(ctx context.Context, _ genx.Stream) error {
			<-ctx.Done()
			return nil
		}),
	}
	peer := &PeerConn{agentHost: runtime, agentInput: source}
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio", BeginOfStream: true}}
	if err := peer.pushAgentInputChunk(ctx, chunk); err != nil {
		t.Fatalf("pushAgentInputChunk() error = %v", err)
	}
	source.mu.RLock()
	input := source.current
	source.mu.RUnlock()
	if input == nil {
		t.Fatal("reload did not open an agent input stream")
	}
	go func() {
		got, _ := input.Next()
		received <- got
	}()
	select {
	case got := <-received:
		if got != chunk {
			t.Fatalf("received chunk = %p, want %p", got, chunk)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pushed chunk")
	}
}

type peerConnFakeMetrics struct {
	samples []metrics.Sample
}

func (s *peerConnFakeMetrics) Append(_ context.Context, samples []metrics.Sample) error {
	s.samples = append(s.samples, samples...)
	return nil
}

func (s *peerConnFakeMetrics) Latest(context.Context, metrics.LatestQuery) (metrics.SeriesSet, error) {
	return nil, nil
}

func (s *peerConnFakeMetrics) Range(context.Context, metrics.RangeQuery) (metrics.SeriesSet, error) {
	return nil, nil
}

func (s *peerConnFakeMetrics) Aggregate(context.Context, metrics.AggregateQuery) (metrics.SeriesSet, error) {
	return nil, nil
}

func (s *peerConnFakeMetrics) Close() error {
	return nil
}

type peerConnBlockingMetrics struct {
	peerConnFakeMetrics
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
	once     sync.Once
	finish   sync.Once
}

func newPeerConnBlockingMetrics() *peerConnBlockingMetrics {
	return &peerConnBlockingMetrics{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
	}
}

func (s *peerConnBlockingMetrics) Append(_ context.Context, samples []metrics.Sample) error {
	s.once.Do(func() { close(s.started) })
	defer s.finish.Do(func() { close(s.finished) })
	select {
	case <-s.release:
	}
	return s.peerConnFakeMetrics.Append(context.Background(), samples)
}

type peerConnTestPacket struct {
	protocol byte
	payload  []byte
}

type peerConnPacketConn struct {
	testGiznetConn
	packets []peerConnTestPacket
	reads   int
}

func (c *peerConnPacketConn) Read(buf []byte) (byte, int, error) {
	c.reads++
	if len(c.packets) == 0 {
		return 0, 0, giznet.ErrClosed
	}
	packet := c.packets[0]
	c.packets = c.packets[1:]
	return packet.protocol, copy(buf, packet.payload), nil
}

type peerConnBlockingStatusSync struct {
	entered chan struct{}
	release chan struct{}
	calls   int
}

func (s *peerConnBlockingStatusSync) SyncTelemetryStatus(context.Context, giznet.PublicKey, peertelemetry.StatusPatch) error {
	s.calls++
	s.entered <- struct{}{}
	<-s.release
	return nil
}

func peerConnBoolPtr(v bool) *bool {
	return &v
}

func peerConnIntPtr(v int) *int {
	return &v
}

func TestPeerConnPCMChunkToInt16(t *testing.T) {
	chunk := &pcm.DataChunk{Data: []byte{0x34, 0x12, 0x78, 0x56}}
	got := peerConnPCMChunkToInt16(chunk)
	if len(got) != 2 {
		t.Fatalf("len(peerConnPCMChunkToInt16()) = %d", len(got))
	}
	if got[0] != 0x1234 || got[1] != 0x5678 {
		t.Fatalf("peerConnPCMChunkToInt16() = %#v", got)
	}
	if out := peerConnPCMChunkToInt16(nil); out != nil {
		t.Fatalf("peerConnPCMChunkToInt16(nil) = %#v", out)
	}
}

type peerConnTestHost struct {
	output genx.Stream
}

func (h peerConnTestHost) Transform(context.Context, string, genx.Stream) (genx.Stream, error) {
	return h.output, nil
}

type peerConnBlockingStream struct {
	done chan struct{}
	once sync.Once
}

func newPeerConnBlockingStream() *peerConnBlockingStream {
	return &peerConnBlockingStream{done: make(chan struct{})}
}

func (s *peerConnBlockingStream) Next() (*genx.MessageChunk, error) {
	<-s.done
	return nil, context.Canceled
}

func (s *peerConnBlockingStream) Close() error {
	return s.CloseWithError(context.Canceled)
}

func (s *peerConnBlockingStream) CloseWithError(error) error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func (s *peerConnBlockingStream) closed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}
