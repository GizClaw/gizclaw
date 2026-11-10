//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/netemtrace"
)

// TestAudioNetemRoundTrip uses the multi-server SDK helpers against a separate,
// local-only Server/Edge project. It never opens the multi-server TiKV fixture.
// The controller owns injection; the test receives neither Docker nor tc access.
func TestAudioNetemRoundTrip(t *testing.T) {
	endpoint := os.Getenv("GIZCLAW_NETEM_ENDPOINT")
	if endpoint == "" {
		t.Skip("run with run_audio_netem_tests.sh in its isolated Docker project")
	}
	if endpoint != "edge:9821" {
		t.Fatal("audio netem requires the isolated edge:9821 endpoint")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancel()
	info := fetchServer(t, endpoint)
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	client := connectAndServe(t, key, info, info.PublicKey, "netem-client")
	defer client.Close()
	if _, err := client.Register(ctx, "netem-register", "netem-local-fixture"); err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateWorkspace(ctx, "netem-create", rpcapi.WorkspaceCreateRequest{
		Name: "netem-audio", WorkflowName: "rtp-bos-eino",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_, _ = client.StopServerRun(cleanup, "netem-stop")
		_, _ = client.DeleteWorkspace(cleanup, "netem-delete", rpcapi.WorkspaceDeleteRequest{Name: created.Name})
		_, _ = client.DeletePeer(cleanup, "netem-delete-peer", rpcapi.ServerPeerDeleteRequest{})
	}()
	if _, err := client.ReloadServerRunWorkspaceWithOptions(ctx, "netem-start", rpcapi.ServerReloadRunWorkspaceWithOptionsRequest{
		WorkspaceName: &created.Name,
		Parameters:    &rpcapi.WorkspaceParametersPatch{Input: new(rpcapi.WorkspaceInputModePushToTalk)},
	}); err != nil {
		t.Fatal(err)
	}
	stream, err := client.OpenPeerStream(512)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	stopRead := context.AfterFunc(ctx, func() { _ = stream.Close() })
	defer stopRead()
	type observation struct {
		Text             string  `json:"text"`
		AudioBytes       int     `json:"audio_bytes"`
		AudioPackets     int     `json:"audio_packets"`
		AudioEOS         bool    `json:"audio_eos"`
		TextEOS          bool    `json:"text_eos"`
		IgnoredPostEOS   uint64  `json:"ignored_post_eos"`
		FirstAudioNS     int64   `json:"first_audio_ns"`
		InputEOSNS       int64   `json:"input_eos_ns"`
		Error            string  `json:"error,omitempty"`
		NonSilentPackets int     `json:"non_silent_packets"`
		DecodeErrors     int     `json:"decode_errors"`
		MaxRMS           float64 `json:"max_rms"`
		OriginalPackets  int     `json:"original_packets"`
		ConcealedSamples int     `json:"concealed_samples"`
		PCMBytes         int     `json:"pcm_bytes"`
	}
	result := make(chan observation, 1)
	go func() {
		var value observation
		decoder, err := opus.NewDecoder(16000, 1)
		if err != nil {
			value.Error = err.Error()
			result <- value
			return
		}
		defer decoder.Close()
		for {
			chunk, err := stream.Next()
			if err != nil {
				value.Error = err.Error()
				break
			}
			if chunk.Ctrl != nil && chunk.Ctrl.Error != "" {
				value.Error = chunk.Ctrl.Error
				break
			}
			if text, ok := chunk.Part.(genx.Text); ok {
				value.Text += string(text)
				value.TextEOS = value.TextEOS || chunk.IsEndOfStream()
			}
			if blob, ok := chunk.Part.(*genx.Blob); ok && strings.HasPrefix(blob.MIMEType, "audio/") {
				if chunk.IsBeginOfStream() {
					netemtrace.Record("client_audio_bos", nil, chunk.Ctrl.StreamID, -1, -1)
				}
				if chunk.IsEndOfStream() {
					netemtrace.Record("client_audio_eos", nil, chunk.Ctrl.StreamID, -1, -1)
				}
				if len(blob.Data) > 0 {
					var pcm []int16
					var decodeErr error
					if blob.MIMEType == "audio/pcm;rate=16000;channels=1" {
						if len(blob.Data)%2 != 0 || blob.ConcealedAudio == (blob.OriginalAudio != nil && len(blob.OriginalAudio.Data) > 0) {
							decodeErr = fmt.Errorf("invalid recovered PCM provenance")
						} else {
							pcm = make([]int16, len(blob.Data)/2)
							for i := range pcm {
								pcm[i] = int16(binary.LittleEndian.Uint16(blob.Data[i*2:]))
							}
							value.PCMBytes += len(blob.Data)
							if blob.ConcealedAudio {
								value.ConcealedSamples += len(pcm)
							} else {
								value.OriginalPackets++
							}
						}
					} else if blob.MIMEType == "audio/opus" {
						pcm, decodeErr = decoder.Decode(blob.Data, 960, false)
						value.OriginalPackets++
					} else {
						decodeErr = fmt.Errorf("unsupported received audio format %q", blob.MIMEType)
					}
					if decodeErr != nil {
						value.DecodeErrors++
					} else if len(pcm) > 0 {
						var energy float64
						for _, sample := range pcm {
							energy += float64(sample) * float64(sample)
						}
						rms := math.Sqrt(energy / float64(len(pcm)))
						if rms > 50 {
							value.NonSilentPackets++
						}
						value.MaxRMS = max(value.MaxRMS, rms)
					}
					if value.FirstAudioNS == 0 {
						value.FirstAudioNS = time.Now().UnixNano()
					}
					value.AudioPackets++
					value.AudioBytes += len(blob.Data)
				}
				value.AudioEOS = value.AudioEOS || chunk.IsEndOfStream()
			}
			if value.AudioEOS && value.TextEOS {
				break
			}
		}
		result <- value
	}()
	id := "netem-fixed-input"
	push := func(data []byte, bos, eos bool) {
		t.Helper()
		chunk := &genx.MessageChunk{Role: genx.RoleUser, Part: &genx.Blob{MIMEType: "audio/opus", Data: data},
			Ctrl: &genx.StreamCtrl{StreamID: id, Label: "user", BeginOfStream: bos, EndOfStream: eos, InputMode: "push-to-talk"}}
		if err := stream.Push(ctx, chunk); err != nil {
			t.Fatal(err)
		}
		stage := "client_sent"
		if bos {
			stage = "client_bos"
		}
		if eos {
			stage = "client_eos"
		}
		netemtrace.Record(stage, data, id, -1, -1)
	}
	// Same mono 16 kHz / 20 ms encoding as opusTonePackets. A seeded low-level
	// noise component makes every real Opus payload identifiable across the hop.
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	rng := rand.New(rand.NewPCG(7919, 104729))
	packets := make([][]byte, 160)
	for i := range packets {
		pcm := make([]int16, 320)
		for j := range pcm {
			pcm[j] = int16(10000*math.Sin(2*math.Pi*440*float64(i*320+j)/16000) + float64(rng.IntN(4001)-2000))
		}
		packets[i], err = encoder.Encode(pcm, 320)
		if err != nil {
			t.Fatal(err)
		}
	}
	push(nil, true, false)
	start := time.Now()
	for i, packet := range packets {
		delay := time.Until(start.Add(time.Duration(i) * 20 * time.Millisecond))
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				t.Fatal(ctx.Err())
			}
		}
		push(packet, false, false)
	}
	// Exactly one media interval between the final frame and release, matching
	// the existing multi-server PTT sender. No extra EOS drain or reorder fix.
	timer := time.NewTimer(time.Until(start.Add(160 * 20 * time.Millisecond)))
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		t.Fatal(ctx.Err())
	}
	inputEOS := time.Now().UnixNano()
	push(nil, false, true)
	var value observation
	select {
	case value = <-result:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Keep the subscription open after EOS to count actual late raw audio.
	tail := time.NewTimer(750 * time.Millisecond)
	select {
	case <-tail.C:
	case <-ctx.Done():
		tail.Stop()
	}
	value.IgnoredPostEOS = stream.IgnoredPostEOSAudioPackets()
	value.InputEOSNS = inputEOS
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("GIZCLAW_NETEM_REPORT"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("audio packets=%d bytes=%d eos=%v text_eos=%v late=%d", value.AudioPackets, value.AudioBytes, value.AudioEOS, value.TextEOS, value.IgnoredPostEOS)
	if value.Error != "" || !value.AudioEOS || !value.TextEOS || value.AudioBytes == 0 || value.NonSilentPackets == 0 || value.DecodeErrors != 0 {
		t.Fatal(fmt.Sprintf("incomplete audible response: %+v", value))
	}
}
