package transformers

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"iter"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/doubao-speech-go"
	mp3codec "github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestDoubaoRealtimeDuplexAudioInputDecodesOpusToPCM(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	const sampleRate = 24000
	const channels = 1
	frameSize := sampleRate / 50
	pcm := make([]int16, frameSize*channels)
	for i := range pcm {
		pcm[i] = int16((i % 64) * 100)
	}
	enc, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	packet, err := enc.Encode(pcm, frameSize)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	input := newDoubaoRealtimeDuplexAudioInput("pcm", sampleRate, channels, false)
	defer input.close()
	got, err := input.prepare(&genx.Blob{MIMEType: "audio/opus", Data: packet})
	if err != nil {
		t.Fatalf("prepare opus: %v", err)
	}
	if len(got) != frameSize*channels*2 {
		t.Fatalf("decoded bytes = %d, want %d", len(got), frameSize*channels*2)
	}
	if bytes.Equal(got, packet) {
		t.Fatal("prepare returned raw opus packet")
	}
}

func TestDoubaoRealtimeDuplexAudioInputPassesPCMThrough(t *testing.T) {
	input := newDoubaoRealtimeDuplexAudioInput("pcm", 16000, 1, false)
	pcm := []byte{1, 0, 2, 0}
	got, err := input.prepare(&genx.Blob{MIMEType: "audio/pcm", Data: pcm})
	if err != nil {
		t.Fatalf("prepare pcm: %v", err)
	}
	if !bytes.Equal(got, pcm) {
		t.Fatalf("prepare pcm = %v, want %v", got, pcm)
	}
}

func TestDoubaoRealtimeDuplexAudioInputPassesSpeechOpusThrough(t *testing.T) {
	input := newDoubaoRealtimeDuplexAudioInput("speech_opus", 16000, 1, false)
	packet := []byte{0x11, 0x22, 0x33}
	got, err := input.prepare(&genx.Blob{MIMEType: "audio/opus", Data: packet})
	if err != nil {
		t.Fatalf("prepare speech_opus: %v", err)
	}
	if !bytes.Equal(got, packet) {
		t.Fatalf("prepare speech_opus = %v, want %v", got, packet)
	}
}

func TestDoubaoRealtimeDuplexAudioInputRejectsOggForSpeechOpus(t *testing.T) {
	input := newDoubaoRealtimeDuplexAudioInput("speech_opus", 16000, 1, false)
	if _, err := input.prepare(&genx.Blob{MIMEType: "audio/ogg", Data: []byte("OggS")}); err == nil {
		t.Fatal("prepare speech_opus audio/ogg error = nil, want error")
	}
}

func TestDoubaoRealtimeDuplexAudioInputRejectsUnknownForSpeechOpus(t *testing.T) {
	input := newDoubaoRealtimeDuplexAudioInput("speech_opus", 16000, 1, false)
	if _, err := input.prepare(&genx.Blob{MIMEType: "application/octet-stream", Data: []byte{1, 2, 3}}); err == nil {
		t.Fatal("prepare speech_opus unknown MIME error = nil, want error")
	}
}

func TestDoubaoRealtimeDuplexAudioInputsArePerStream(t *testing.T) {
	inputs := newDoubaoRealtimeDuplexAudioInputs("speech_opus", 16000, 1, true)
	defer inputs.close()

	a := inputs.stream("a")
	b := inputs.stream("b")
	if a == b {
		t.Fatal("different stream IDs shared the same audio input")
	}
	if again := inputs.stream("a"); again != a {
		t.Fatal("same stream ID did not reuse audio input")
	}
	inputs.closeStream("a")
	if next := inputs.stream("a"); next == a {
		t.Fatal("closed stream ID reused old audio input")
	}
}

func TestChunkInputStreamIDUsesActiveStreamForDirectAudio(t *testing.T) {
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "audio"}}
	if got := doubaoRealtimeDuplexChunkInputStreamID(chunk, "turn-1"); got != "turn-1" {
		t.Fatalf("doubaoRealtimeDuplexChunkInputStreamID(audio) = %q, want active stream", got)
	}
	chunk.Ctrl.StreamID = "turn-2"
	if got := doubaoRealtimeDuplexChunkInputStreamID(chunk, "turn-1"); got != "turn-2" {
		t.Fatalf("doubaoRealtimeDuplexChunkInputStreamID(explicit) = %q, want explicit stream", got)
	}
}

func TestDoubaoRealtimeDuplexStreamIDsSplitRealtimeTranscript(t *testing.T) {
	ids := newDoubaoRealtimeDuplexStreamIDs()
	ids.beginInput("turn-1")
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}}

	if got := ids.serviceInput(chunk); got != "turn-1" {
		t.Fatalf("service input = %q, want base stream", got)
	}
	if got := ids.input(); got != "turn-1:rt:1" {
		t.Fatalf("transcript input = %q, want first realtime segment", got)
	}
	if got := ids.endInputSegment(); got != "turn-1:rt:1" {
		t.Fatalf("ended segment = %q, want first realtime segment", got)
	}
	if got := ids.response(); got != "turn-1:rt:1" {
		t.Fatalf("response stream = %q, want first realtime segment", got)
	}
	if got := ids.input(); got != "turn-1:rt:2" {
		t.Fatalf("next transcript input = %q, want second realtime segment", got)
	}
	if got := ids.endInputSegment(); got != "turn-1:rt:2" {
		t.Fatalf("second ended segment = %q, want second realtime segment", got)
	}
	if got := ids.response(); got != "turn-1:rt:2" {
		t.Fatalf("second response stream = %q, want second realtime segment", got)
	}
}

func TestDoubaoRealtimeDuplexStreamIDsInferRealtimeInputWithoutBOS(t *testing.T) {
	ids := newDoubaoRealtimeDuplexStreamIDs()
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}}

	if got := ids.serviceInput(chunk); got != "turn-1" {
		t.Fatalf("service input = %q, want chunk stream", got)
	}
	if got := ids.input(); got != "turn-1:rt:1" {
		t.Fatalf("transcript input = %q, want chunk-derived realtime segment", got)
	}
}

func TestRealtimeASRResponseEndsSegment(t *testing.T) {
	if !realtimeDuplexASRResponseEndsSegment(&doubaospeech.RealtimeEvent{
		Results: []doubaospeech.RealtimeASRResult{{Text: "第一段", IsInterim: false}},
	}, "第一段") {
		t.Fatal("final ASR result did not end segment")
	}
	if realtimeDuplexASRResponseEndsSegment(&doubaospeech.RealtimeEvent{
		Results: []doubaospeech.RealtimeASRResult{{Text: "第一", IsInterim: true}},
	}, "第一") {
		t.Fatal("interim ASR response ended segment")
	}
	if realtimeDuplexASRResponseEndsSegment(&doubaospeech.RealtimeEvent{IsFinal: true, Text: "。"}, "。") {
		t.Fatal("punctuation-only ASR response ended segment")
	}
	if realtimeDuplexASRResponseEndsSegment(&doubaospeech.RealtimeEvent{Text: "第二段"}, "第二段") {
		t.Fatal("ASR response without final marker ended segment")
	}
}

func TestDoubaoRealtimeDuplexAudioInputTranscodesSpeechOpus(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	const sourceSampleRate = 24000
	const targetSampleRate = 16000
	const channels = 1
	frameSize := sourceSampleRate / 50
	pcm := make([]int16, frameSize*channels)
	for i := range pcm {
		pcm[i] = int16((i % 64) * 100)
	}
	enc, err := opus.NewEncoder(sourceSampleRate, channels, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	packet, err := enc.Encode(pcm, frameSize)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	input := newDoubaoRealtimeDuplexAudioInput("speech_opus", targetSampleRate, channels, true)
	defer input.close()
	got, err := input.prepare(&genx.Blob{MIMEType: "audio/opus", Data: packet})
	if err != nil {
		t.Fatalf("prepare transcode speech_opus: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("prepare transcode returned empty packet")
	}
	if bytes.Equal(got, packet) {
		t.Fatal("prepare transcode returned original packet")
	}
}

func TestDoubaoRealtimeDuplexAudioInputEncodesMP3ToSpeechOpus(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}

	rawPCM := testRealtimePCM16Sine(44100, 2, 0.12, 440)
	var mp3Buf bytes.Buffer
	enc, err := mp3codec.NewEncoder(&mp3Buf, 44100, 2)
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			t.Skipf("native mp3 encoder runtime is not available: %v", err)
		}
		t.Fatalf("NewEncoder: %v", err)
	}
	if _, err := enc.Write(rawPCM); err != nil {
		t.Fatalf("mp3 Write: %v", err)
	}
	if err := enc.Flush(); err != nil {
		t.Fatalf("mp3 Flush: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("mp3 Close: %v", err)
	}

	input := newDoubaoRealtimeDuplexAudioInput("speech_opus", 16000, 1, true)
	defer input.close()
	frames, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/mpeg", Data: mp3Buf.Bytes()})
	if err != nil {
		t.Fatalf("prepareFrames mp3: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("prepareFrames mp3 returned no opus frames")
	}
	for i, frame := range frames {
		if len(frame) == 0 {
			t.Fatalf("opus frame %d is empty", i)
		}
	}
}

func TestDoubaoRealtimeDuplexAudioInputsRejectMIMEChange(t *testing.T) {
	inputs := newDoubaoRealtimeDuplexAudioInputs("speech_opus", 16000, 1, true)
	defer inputs.close()

	if _, err := inputs.streamForBlob("s1", &genx.Blob{MIMEType: "audio/opus; codecs=opus"}); err != nil {
		t.Fatalf("streamForBlob initial: %v", err)
	}
	if _, err := inputs.streamForBlob("s1", &genx.Blob{MIMEType: "audio/opus"}); err != nil {
		t.Fatalf("streamForBlob same base MIME: %v", err)
	}
	if _, err := inputs.streamForBlob("s1", &genx.Blob{MIMEType: "audio/mpeg"}); err == nil {
		t.Fatal("streamForBlob changed MIME error = nil, want error")
	}

	inputs.closeStream("s1")
	if _, err := inputs.streamForBlob("s1", &genx.Blob{MIMEType: "audio/mpeg"}); err != nil {
		t.Fatalf("streamForBlob after EOS: %v", err)
	}
}

func TestDoubaoRealtimeDuplexConfigSetsDuplexSession(t *testing.T) {
	strict := true
	enableMusic := true
	tfr := NewDoubaoRealtimeDuplex(nil,
		WithDoubaoRealtimeDuplexModel("1.2.6.0"),
		WithDoubaoRealtimeDuplexSessionID("workspace-dialog-id"),
		WithDoubaoRealtimeDuplexInstructions("简短回答。"),
		WithDoubaoRealtimeDuplexSpeaker("voice-a"),
		WithDoubaoRealtimeDuplexFormat("ogg_opus"),
		WithDoubaoRealtimeDuplexSampleRate(24000),
		WithDoubaoRealtimeDuplexInputFormat("speech_opus"),
		WithDoubaoRealtimeDuplexInputSampleRate(16000),
		WithDoubaoRealtimeDuplexOutputSpeed(1),
		WithDoubaoRealtimeDuplexOutputLoudness(-1),
		WithDoubaoRealtimeDuplexTools([]doubaospeech.RealtimeDuplexFunctionTool{{
			Type:   "function",
			Name:   "get_weather",
			Strict: &strict,
			Parameters: &doubaospeech.RealtimeDuplexJSONSchema{
				Type:                 "object",
				AdditionalProperties: &strict,
			},
		}}),
		WithDoubaoRealtimeDuplexExtension(&doubaospeech.RealtimeDuplexExtension{
			Dialog: &doubaospeech.RealtimeDuplexDialogExtension{
				Extra: &doubaospeech.RealtimeDuplexDialogExtra{EnableMusic: &enableMusic},
			},
		}),
	)
	cfg := tfr.realtimeConfig()
	if cfg.Session.ID != "workspace-dialog-id" {
		t.Fatalf("session id = %q, want workspace-dialog-id", cfg.Session.ID)
	}
	if cfg.Session.Model != "1.2.6.0" || cfg.Session.Instructions != "简短回答。" {
		t.Fatalf("session model/instructions = %#v", cfg.Session)
	}
	if cfg.Session.Audio.Input.Format.Type != "speech_opus" || cfg.Session.Audio.Input.Format.Rate != 16000 {
		t.Fatalf("input audio = %#v", cfg.Session.Audio.Input.Format)
	}
	if cfg.Session.Audio.Output.Format.Type != "ogg_opus" || cfg.Session.Audio.Output.Format.Rate != 24000 ||
		cfg.Session.Audio.Output.Voice != "voice-a" || cfg.Session.Audio.Output.Speed != 1 || cfg.Session.Audio.Output.Loudness != -1 {
		t.Fatalf("output audio = %#v", cfg.Session.Audio.Output)
	}
	if len(cfg.Session.Tools) != 1 || cfg.Session.Tools[0].Name != "get_weather" {
		t.Fatalf("tools = %#v", cfg.Session.Tools)
	}
	if cfg.Extension == nil || cfg.Extension.Dialog == nil || cfg.Extension.Dialog.Extra == nil ||
		cfg.Extension.Dialog.Extra.EnableMusic == nil || !*cfg.Extension.Dialog.Extra.EnableMusic {
		t.Fatalf("extension = %#v", cfg.Extension)
	}
}

func TestPendingChunkStreamDelegatesClose(t *testing.T) {
	rest := &trackingCloseStream{}
	stream := withDoubaoRealtimeDuplexPendingChunk(rest, &genx.MessageChunk{Part: genx.Text("first")})

	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got, ok := chunk.Part.(genx.Text); !ok || got != "first" {
		t.Fatalf("first chunk = %#v", chunk)
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if rest.closed != 1 {
		t.Fatalf("rest closed = %d, want 1", rest.closed)
	}

	wantErr := errors.New("stop")
	if err := stream.CloseWithError(wantErr); err != nil {
		t.Fatalf("CloseWithError() error = %v", err)
	}
	if rest.closeErr != wantErr {
		t.Fatalf("rest close error = %v, want %v", rest.closeErr, wantErr)
	}
}

func TestPCM16LE(t *testing.T) {
	got := doubaoRealtimeDuplexPCM16LE([]int16{1, -2})
	want := []byte{1, 0, 254, 255}
	if !bytes.Equal(got, want) {
		t.Fatalf("doubaoRealtimeDuplexPCM16LE = %v, want %v", got, want)
	}
}

func testRealtimePCM16Sine(sampleRate, channels int, seconds float64, hz float64) []byte {
	numSamples := int(float64(sampleRate) * seconds)
	out := make([]byte, numSamples*channels*2)
	for i := range numSamples {
		t := float64(i) / float64(sampleRate)
		sample := int16(math.Sin(2*math.Pi*hz*t) * 16000)
		for ch := range channels {
			off := i*channels*2 + ch*2
			binary.LittleEndian.PutUint16(out[off:], uint16(sample))
		}
	}
	return out
}

func TestRealtimeASRTextFromPayload(t *testing.T) {
	payload := []byte(`{"extra":{"origin_text":"你好","soft_finish_paralinguistic":{"asr_text":"你好，能听见我说话吗？"}}}`)
	if got := realtimeDuplexASRText(payload); got != "你好，能听见我说话吗？" {
		t.Fatalf("realtimeDuplexASRText(final) = %q", got)
	}

	payload = []byte(`{"extra":{"origin_text":"你好"}}`)
	if got := realtimeDuplexASRText(payload); got != "你好" {
		t.Fatalf("realtimeDuplexASRText(origin) = %q", got)
	}

	payload = []byte(`{"results":[{"alternatives":[{"text":"候选"}]}]}`)
	if got := realtimeDuplexASRText(payload); got != "候选" {
		t.Fatalf("realtimeDuplexASRText(alternative) = %q", got)
	}
}

func TestRealtimeTextDelta(t *testing.T) {
	if got := realtimeDuplexTextDelta("你好", "你好世界"); got != "世界" {
		t.Fatalf("realtimeDuplexTextDelta prefix = %q", got)
	}
	if got := realtimeDuplexTextDelta("你好", "再见"); got != "再见" {
		t.Fatalf("realtimeDuplexTextDelta replacement = %q", got)
	}
	if got := realtimeDuplexTextDelta("你好", "你好"); got != "" {
		t.Fatalf("realtimeDuplexTextDelta duplicate = %q", got)
	}
	if got := realtimeDuplexTextDelta("你好能听到我说话吗", "你好，能听到我说话吗？"); got != "？" {
		t.Fatalf("realtimeDuplexTextDelta punctuated prefix = %q", got)
	}
	if got := realtimeDuplexTextDelta("嗯今天天气怎么样我想出门走走", "今天天气怎么样？我想出门走走。"); got != "" {
		t.Fatalf("realtimeDuplexTextDelta replacement subset = %q", got)
	}
}

func TestDoubaoRealtimeDuplexOutputAudioBlobsExtractsOggOpusPackets(t *testing.T) {
	var buf bytes.Buffer
	sw, err := ogg.NewStreamWriter(&buf, 77)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	if _, err := sw.WritePacket(testRealtimeOpusHeadPacket(24000, 1), 0, false); err != nil {
		t.Fatalf("write opus head: %v", err)
	}
	if _, err := sw.WritePacket([]byte("OpusTags"), 0, false); err != nil {
		t.Fatalf("write opus tags: %v", err)
	}
	packet := []byte{0x11, 0x22, 0x33}
	if _, err := sw.WritePacket(packet, 960, true); err != nil {
		t.Fatalf("write opus packet: %v", err)
	}

	tfr := NewDoubaoRealtimeDuplex(nil, WithDoubaoRealtimeDuplexFormat("ogg_opus"))
	blobs, err := tfr.outputAudioBlobs(buf.Bytes())
	if err != nil {
		t.Fatalf("outputAudioBlobs: %v", err)
	}
	if len(blobs) != 1 {
		t.Fatalf("outputAudioBlobs len = %d, want 1", len(blobs))
	}
	if blobs[0].MIMEType != "audio/opus" {
		t.Fatalf("outputAudioBlobs MIME = %q, want audio/opus", blobs[0].MIMEType)
	}
	if !bytes.Equal(blobs[0].Data, packet) {
		t.Fatalf("outputAudioBlobs packet = %v, want %v", blobs[0].Data, packet)
	}
}

func TestDoubaoRealtimeDuplexSendsFakeFunctionCallOutputs(t *testing.T) {
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{
				Type: doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
				FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{{
					CallID:    "call-1",
					Name:      "get_weather",
					Arguments: `{"city":"深圳"}`,
				}},
			},
			{Type: doubaospeech.RealtimeDuplexEventSessionClosed},
		},
	}
	opener := &fakeDoubaoRealtimeDuplexOpener{session: session}
	tfr := NewDoubaoRealtimeDuplex(nil, withDoubaoRealtimeDuplexOpener(opener))
	stream, err := tfr.Transform(context.Background(), "", emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for {
		_, err := stream.Next()
		if err == io.EOF || err == genx.ErrDone {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
	}
	if opener.config == nil {
		t.Fatal("OpenSession was not called")
	}
	if len(session.outputs) != 1 {
		t.Fatalf("function call outputs len = %d, want 1", len(session.outputs))
	}
	output := session.outputs[0]
	if output.CallID != "call-1" ||
		!strings.Contains(output.Output, `"source":"gizclaw-internal-fake"`) ||
		!strings.Contains(output.Output, `"tool":"get_weather"`) {
		t.Fatalf("function call output = %#v", output)
	}
	if !session.closed {
		t.Fatal("session was not closed")
	}
}

func TestDoubaoRealtimeDuplexReturnsFunctionCallOutputError(t *testing.T) {
	wantErr := errors.New("send function output failed")
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{
				Type: doubaospeech.RealtimeDuplexEventResponseFunctionCallArgumentsDone,
				FunctionCalls: []doubaospeech.RealtimeDuplexFunctionCall{{
					CallID: "call-1",
					Name:   "get_weather",
				}},
			},
		},
		functionCallErr: wantErr,
	}
	tfr := NewDoubaoRealtimeDuplex(nil)
	_, err := tfr.processLoop(context.Background(), emptyRealtimeStream{}, newBufferStream(1), session)
	if !errors.Is(err, wantErr) {
		t.Fatalf("processLoop() error = %v, want %v", err, wantErr)
	}
	if !session.closed {
		t.Fatal("session was not closed")
	}
}

func TestDoubaoRealtimeDuplexReturnsDuplexErrorEvent(t *testing.T) {
	wantErr := &doubaospeech.Error{Code: 500, Message: "duplex failed"}
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{Type: doubaospeech.RealtimeDuplexEventError, Error: wantErr},
		},
	}
	tfr := NewDoubaoRealtimeDuplex(nil)
	_, err := tfr.processLoop(context.Background(), emptyRealtimeStream{}, newBufferStream(1), session)
	if !errors.Is(err, wantErr) {
		t.Fatalf("processLoop() error = %v, want %v", err, wantErr)
	}
	if !session.closed {
		t.Fatal("session was not closed")
	}
}

func TestDoubaoRealtimeDuplexErrorEventClosesBlockedInput(t *testing.T) {
	wantErr := &doubaospeech.Error{Code: 500, Message: "duplex failed"}
	input := newBlockingRealtimeStream()
	session := &fakeDoubaoRealtimeDuplexSession{
		beforeRecv: input.started,
		events: []*doubaospeech.RealtimeDuplexEvent{
			{Type: doubaospeech.RealtimeDuplexEventError, Error: wantErr},
		},
	}
	tfr := NewDoubaoRealtimeDuplex(nil)

	done := make(chan error, 1)
	go func() {
		_, err := tfr.processLoop(context.Background(), input, newBufferStream(1), session)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, wantErr) {
			t.Fatalf("processLoop() error = %v, want %v", err, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("processLoop() did not return after duplex error closed output")
	}
	if got := input.closeErr(); !errors.Is(got, wantErr) {
		t.Fatalf("input close error = %v, want %v", got, wantErr)
	}
	if !session.closed {
		t.Fatal("session was not closed")
	}
}

func TestDoubaoRealtimeDuplexMapsDuplexEventsToStreamChunks(t *testing.T) {
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{Type: doubaospeech.RealtimeDuplexEventTranscriptionDelta, Delta: "你好"},
			{Type: doubaospeech.RealtimeDuplexEventTranscriptionCompleted, Transcript: "你好"},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDelta, ResponseID: "resp-1", Delta: "收到"},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDone, ResponseID: "resp-1", Text: "收到"},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputAudioStarted, ResponseID: "resp-1"},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputAudioDelta, ResponseID: "resp-1", Audio: []byte{1, 2, 3}},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputAudioDone, ResponseID: "resp-1"},
			{Type: doubaospeech.RealtimeDuplexEventResponseDone, ResponseID: "resp-1"},
			{Type: doubaospeech.RealtimeDuplexEventSessionClosed},
		},
	}
	tfr := NewDoubaoRealtimeDuplex(nil,
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
		WithDoubaoRealtimeDuplexFormat("pcm"),
	)
	stream, err := tfr.Transform(context.Background(), "", emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err == io.EOF || err == genx.ErrDone {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) < 6 {
		t.Fatalf("chunks len = %d, chunks=%#v", len(chunks), chunks)
	}
	if got, ok := chunks[0].Part.(genx.Text); !ok || got != "你好" || chunks[0].Role != genx.RoleUser {
		t.Fatalf("transcript chunk = %#v", chunks[0])
	}
	hasAssistantText := false
	assistantTextCount := 0
	hasAudio := false
	hasAudioEOS := false
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && chunk.Role == genx.RoleModel && text == "收到" {
			hasAssistantText = true
			assistantTextCount++
		}
		if blob, ok := chunk.Part.(*genx.Blob); ok && bytes.Equal(blob.Data, []byte{1, 2, 3}) {
			hasAudio = true
		}
		if chunk.IsEndOfStream() {
			if _, ok := chunk.Part.(*genx.Blob); ok && chunk.Role == genx.RoleModel {
				hasAudioEOS = true
			}
		}
	}
	if !hasAssistantText || !hasAudio || !hasAudioEOS {
		t.Fatalf("assistant text/audio/eos = %t/%t/%t; chunks=%#v", hasAssistantText, hasAudio, hasAudioEOS, chunks)
	}
	if assistantTextCount != 1 {
		t.Fatalf("assistant text chunks = %d, want 1; chunks=%#v", assistantTextCount, chunks)
	}
}

func TestDoubaoRealtimeDuplexInputEOSClosesLocalStream(t *testing.T) {
	inputDrained := make(chan struct{})
	session := &fakeDoubaoRealtimeDuplexSession{
		beforeRecv: inputDrained,
		events:     []*doubaospeech.RealtimeDuplexEvent{{Type: doubaospeech.RealtimeDuplexEventSessionClosed}},
	}
	tfr := NewDoubaoRealtimeDuplexRealtime(nil,
		WithDoubaoRealtimeDuplexInputFormat("pcm"),
		WithDoubaoRealtimeDuplexInputTranscode(false),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
	)
	input := &gatedRealtimeStream{
		first: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		},
		firstDrained: inputDrained,
	}

	err := runDoubaoRealtimeDuplexProcessLoop(t, tfr.DoubaoRealtimeDuplex, input, session)
	if err != nil {
		t.Fatalf("processLoop() error = %v", err)
	}
	if got := session.audioCount(); got != 1 {
		t.Fatalf("SendAudio calls = %d, want 1 before EOS", got)
	}
}

func TestDoubaoRealtimeDuplexSessionClosedWhileWaitingForInputDoesNotDropNextChunk(t *testing.T) {
	bosRead := make(chan struct{})
	firstEventsDrained := make(chan struct{})
	allowAudio := make(chan struct{})
	secondAudioSent := make(chan struct{})
	firstSession := &fakeDoubaoRealtimeDuplexSession{
		beforeRecv:    bosRead,
		events:        []*doubaospeech.RealtimeDuplexEvent{{Type: doubaospeech.RealtimeDuplexEventSessionClosed}},
		eventsDrained: firstEventsDrained,
	}
	secondSession := &fakeDoubaoRealtimeDuplexSession{
		beforeRecv: secondAudioSent,
		events:     []*doubaospeech.RealtimeDuplexEvent{{Type: doubaospeech.RealtimeDuplexEventSessionClosed}},
		audioSent:  secondAudioSent,
	}
	opener := &fakeDoubaoRealtimeDuplexOpener{sessions: []*fakeDoubaoRealtimeDuplexSession{firstSession, secondSession}}
	tfr := NewDoubaoRealtimeDuplexRealtime(nil,
		WithDoubaoRealtimeDuplexInputFormat("pcm"),
		WithDoubaoRealtimeDuplexInputTranscode(false),
		withDoubaoRealtimeDuplexOpener(opener),
	)
	input := &gatedRealtimeStream{
		first: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
		},
		firstDrained: bosRead,
		gate:         allowAudio,
		rest: []*genx.MessageChunk{
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output := newBufferStream(16)
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			if _, err := output.Next(); err != nil {
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() {
		tfr.sessionLoop(ctx, input, output)
		close(done)
	}()

	select {
	case <-firstEventsDrained:
	case <-ctx.Done():
		t.Fatalf("first session did not close after BOS: %v", ctx.Err())
	}
	close(allowAudio)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("sessionLoop() timed out: %v", ctx.Err())
	}
	output.Close()
	<-drainDone
	if got := opener.openCount(); got < 1 || got > 2 {
		t.Fatalf("OpenSession calls = %d, want 1 or 2", got)
	}
	if got := firstSession.audioCount() + secondSession.audioCount(); got != 1 {
		t.Fatalf("total SendAudio calls = %d, want 1", got)
	}
	if got := firstSession.audioCount(); got != 0 {
		t.Fatalf("first session SendAudio calls = %d, want 0", got)
	}
	if got := secondSession.audioCount(); got != 1 {
		t.Fatalf("second session SendAudio calls = %d, want 1", got)
	}
}

func TestDoubaoRealtimeDuplexInputReaderPrioritizesDoneOverBufferedInput(t *testing.T) {
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}}
	reader := newDoubaoRealtimeDuplexInputReader(&sliceRealtimeStream{
		chunks: []*genx.MessageChunk{chunk},
	})
	defer reader.Close()

	deadline := time.After(2 * time.Second)
	for len(reader.results) == 0 {
		select {
		case <-deadline:
			t.Fatal("input reader did not buffer test chunk")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	done := make(chan struct{})
	close(done)

	got, err, closed := reader.NextOrDone(done)
	if got != nil || err != nil || !closed {
		t.Fatalf("NextOrDone() = (%#v, %v, %v), want done without consuming buffered chunk", got, err, closed)
	}
	got, err = reader.Next()
	if err != nil {
		t.Fatalf("Next() after done = %v", err)
	}
	if got != chunk {
		t.Fatalf("Next() after done = %#v, want buffered chunk %#v", got, chunk)
	}
}

func TestDoubaoRealtimeDuplexPendingChunkPrioritizesDone(t *testing.T) {
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "pending", BeginOfStream: true}}
	stream := withDoubaoRealtimeDuplexPendingChunk(emptyRealtimeStream{}, chunk).(doubaoRealtimeDuplexDoneAwareStream)
	done := make(chan struct{})
	close(done)

	got, err, closed := stream.NextOrDone(done)
	if got != nil || err != nil || !closed {
		t.Fatalf("NextOrDone() = (%#v, %v, %v), want done without consuming pending chunk", got, err, closed)
	}
	got, err = stream.Next()
	if err != nil {
		t.Fatalf("Next() after done = %v", err)
	}
	if got != chunk {
		t.Fatalf("Next() after done = %#v, want pending chunk %#v", got, chunk)
	}
}

func TestDoubaoRealtimeDuplexTextDoneAfterAudioDoneAllowsNextTurn(t *testing.T) {
	firstInputDrained := make(chan struct{})
	allowNextInput := make(chan struct{})
	releaseEvents := make(chan struct{})
	eventsDrained := make(chan struct{})
	session := &fakeDoubaoRealtimeDuplexSession{
		beforeRecv: firstInputDrained,
		events: []*doubaospeech.RealtimeDuplexEvent{
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputAudioStarted, ResponseID: "turn-1"},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputAudioDone, ResponseID: "turn-1"},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDelta, ResponseID: "turn-1", Delta: "late text"},
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDone, ResponseID: "turn-1"},
		},
		eventsDrained:    eventsDrained,
		blockAfterEvents: releaseEvents,
	}
	tfr := NewDoubaoRealtimeDuplexRealtime(nil,
		WithDoubaoRealtimeDuplexInputFormat("pcm"),
		WithDoubaoRealtimeDuplexInputTranscode(false),
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
	)
	input := &gatedRealtimeStream{
		first: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", BeginOfStream: true}},
			{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}}, Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}},
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-1", EndOfStream: true}},
		},
		firstDrained: firstInputDrained,
		gate:         allowNextInput,
		rest: []*genx.MessageChunk{
			{Ctrl: &genx.StreamCtrl{StreamID: "turn-2", BeginOfStream: true}},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output := newBufferStream(16)
	defer output.Close()
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for {
			if _, err := output.Next(); err != nil {
				return
			}
		}
	}()
	errCh := make(chan error, 1)
	go func() {
		_, err := tfr.processLoop(ctx, input, output, session)
		errCh <- err
	}()

	select {
	case <-eventsDrained:
	case <-ctx.Done():
		t.Fatalf("events did not drain: %v", ctx.Err())
	}
	close(allowNextInput)
	time.Sleep(50 * time.Millisecond)
	if session.cancelCount() != 0 {
		t.Fatalf("CancelResponse calls = %d, want 0", session.cancelCount())
	}
	close(releaseEvents)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("processLoop() error = %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("processLoop() timed out: %v", ctx.Err())
	}
	output.Close()
	<-drainDone
}

func TestDoubaoRealtimeDuplexUsesDoneTextWhenNoDeltaArrived(t *testing.T) {
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDone, ResponseID: "resp-1", Text: "最终文本"},
			{Type: doubaospeech.RealtimeDuplexEventSessionClosed},
		},
	}
	tfr := NewDoubaoRealtimeDuplex(nil, withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}))
	stream, err := tfr.Transform(context.Background(), "", emptyRealtimeStream{})
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if err == io.EOF || err == genx.ErrDone {
			break
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
	foundText := false
	foundEOS := false
	for _, chunk := range chunks {
		if text, ok := chunk.Part.(genx.Text); ok && chunk.Role == genx.RoleModel && text == "最终文本" {
			foundText = true
		}
		if chunk.Role == genx.RoleModel && chunk.IsEndOfStream() {
			foundEOS = true
		}
	}
	if !foundText || !foundEOS {
		t.Fatalf("done text/eos = %t/%t; chunks=%#v", foundText, foundEOS, chunks)
	}
}

func runDoubaoRealtimeDuplexProcessLoop(t *testing.T, tfr *DoubaoRealtimeDuplex, input genx.Stream, session *fakeDoubaoRealtimeDuplexSession) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := tfr.processLoop(ctx, input, newBufferStream(16), session)
		errCh <- err
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func testRealtimeOpusHeadPacket(sampleRate, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet, []byte("OpusHead"))
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:], uint32(sampleRate))
	return packet
}

type fakeDoubaoRealtimeDuplexOpener struct {
	config   *doubaospeech.RealtimeDuplexConfig
	session  *fakeDoubaoRealtimeDuplexSession
	sessions []*fakeDoubaoRealtimeDuplexSession
	mu       sync.Mutex
	opens    int
}

func (o *fakeDoubaoRealtimeDuplexOpener) OpenSession(_ context.Context, cfg *doubaospeech.RealtimeDuplexConfig) (doubaoRealtimeDuplexSession, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.config = cfg
	if len(o.sessions) > 0 {
		index := o.opens
		if index >= len(o.sessions) {
			index = len(o.sessions) - 1
		}
		o.opens++
		return o.sessions[index], nil
	}
	o.opens++
	return o.session, nil
}

func (o *fakeDoubaoRealtimeDuplexOpener) openCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.opens
}

type fakeDoubaoRealtimeDuplexSession struct {
	events           []*doubaospeech.RealtimeDuplexEvent
	beforeRecv       <-chan struct{}
	eventsDrained    chan<- struct{}
	blockAfterEvents <-chan struct{}
	outputs          []doubaospeech.RealtimeDuplexFunctionCallOutput
	functionCallErr  error
	cancelErr        error
	audioSent        chan<- struct{}
	closed           bool

	mu                sync.Mutex
	audio             [][]byte
	cancels           int
	eventsDrainedOnce sync.Once
	audioSentOnce     sync.Once
}

func (s *fakeDoubaoRealtimeDuplexSession) SendAudio(ctx context.Context, audio []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	frame := append([]byte(nil), audio...)
	s.mu.Lock()
	s.audio = append(s.audio, frame)
	s.mu.Unlock()
	if s.audioSent != nil {
		s.audioSentOnce.Do(func() {
			close(s.audioSent)
		})
	}
	return nil
}

func (s *fakeDoubaoRealtimeDuplexSession) CancelResponse(context.Context) error {
	s.mu.Lock()
	s.cancels++
	s.mu.Unlock()
	return s.cancelErr
}

func (s *fakeDoubaoRealtimeDuplexSession) SendFunctionCallOutputs(_ context.Context, outputs ...doubaospeech.RealtimeDuplexFunctionCallOutput) error {
	if s.functionCallErr != nil {
		return s.functionCallErr
	}
	s.outputs = append(s.outputs, outputs...)
	return nil
}

func (s *fakeDoubaoRealtimeDuplexSession) Recv() iter.Seq2[*doubaospeech.RealtimeDuplexEvent, error] {
	return func(yield func(*doubaospeech.RealtimeDuplexEvent, error) bool) {
		if s.beforeRecv != nil {
			<-s.beforeRecv
		}
		for _, event := range s.events {
			if !yield(event, nil) {
				if s.eventsDrained != nil {
					s.eventsDrainedOnce.Do(func() {
						close(s.eventsDrained)
					})
				}
				return
			}
		}
		if s.eventsDrained != nil {
			s.eventsDrainedOnce.Do(func() {
				close(s.eventsDrained)
			})
		}
		if s.blockAfterEvents != nil {
			<-s.blockAfterEvents
		}
	}
}

func (s *fakeDoubaoRealtimeDuplexSession) Close() error {
	s.closed = true
	return nil
}

func (s *fakeDoubaoRealtimeDuplexSession) audioCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.audio)
}

func (s *fakeDoubaoRealtimeDuplexSession) cancelCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancels
}

type emptyRealtimeStream struct{}

func (emptyRealtimeStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }

func (emptyRealtimeStream) Close() error { return nil }

func (emptyRealtimeStream) CloseWithError(error) error { return nil }

type sliceRealtimeStream struct {
	chunks []*genx.MessageChunk
	index  int
}

func (s *sliceRealtimeStream) Next() (*genx.MessageChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (s *sliceRealtimeStream) Close() error { return nil }

func (s *sliceRealtimeStream) CloseWithError(error) error { return nil }

type gatedRealtimeStream struct {
	first            []*genx.MessageChunk
	rest             []*genx.MessageChunk
	gate             <-chan struct{}
	firstDrained     chan<- struct{}
	firstDrainedOnce sync.Once
	index            int
}

func (s *gatedRealtimeStream) Next() (*genx.MessageChunk, error) {
	if s.index < len(s.first) {
		chunk := s.first[s.index]
		s.index++
		if s.index == len(s.first) && s.firstDrained != nil {
			s.firstDrainedOnce.Do(func() {
				close(s.firstDrained)
			})
		}
		return chunk, nil
	}
	if s.gate != nil {
		<-s.gate
		s.gate = nil
	}
	restIndex := s.index - len(s.first)
	if restIndex >= len(s.rest) {
		return nil, io.EOF
	}
	chunk := s.rest[restIndex]
	s.index++
	return chunk, nil
}

func (s *gatedRealtimeStream) Close() error { return nil }

func (s *gatedRealtimeStream) CloseWithError(error) error { return nil }

type blockingRealtimeStream struct {
	started     chan struct{}
	done        chan struct{}
	startedOnce sync.Once
	doneOnce    sync.Once
	errMu       sync.Mutex
	err         error
}

func newBlockingRealtimeStream() *blockingRealtimeStream {
	return &blockingRealtimeStream{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (s *blockingRealtimeStream) Next() (*genx.MessageChunk, error) {
	s.startedOnce.Do(func() {
		close(s.started)
	})
	<-s.done
	return nil, s.closeErr()
}

func (s *blockingRealtimeStream) Close() error {
	s.close(nil)
	return nil
}

func (s *blockingRealtimeStream) CloseWithError(err error) error {
	s.close(err)
	return nil
}

func (s *blockingRealtimeStream) close(err error) {
	s.doneOnce.Do(func() {
		s.errMu.Lock()
		s.err = err
		s.errMu.Unlock()
		close(s.done)
	})
}

func (s *blockingRealtimeStream) closeErr() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

type trackingCloseStream struct {
	closed   int
	closeErr error
}

func (s *trackingCloseStream) Next() (*genx.MessageChunk, error) {
	return nil, io.EOF
}

func (s *trackingCloseStream) Close() error {
	s.closed++
	return nil
}

func (s *trackingCloseStream) CloseWithError(err error) error {
	s.closeErr = err
	return nil
}
