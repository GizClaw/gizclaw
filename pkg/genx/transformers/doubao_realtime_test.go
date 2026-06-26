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
	mp3codec "github.com/GizClaw/gizclaw-go/pkg/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
)

func TestDoubaoRealtimeAudioInputDecodesOpusToPCM(t *testing.T) {
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

	input := newDoubaoRealtimeAudioInput("pcm", sampleRate, channels, false)
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

func TestDoubaoRealtimeAudioInputPassesPCMThrough(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("pcm", 16000, 1, false)
	pcm := []byte{1, 0, 2, 0}
	got, err := input.prepare(&genx.Blob{MIMEType: "audio/pcm", Data: pcm})
	if err != nil {
		t.Fatalf("prepare pcm: %v", err)
	}
	if !bytes.Equal(got, pcm) {
		t.Fatalf("prepare pcm = %v, want %v", got, pcm)
	}
}

func TestDoubaoRealtimeAudioInputPassesSpeechOpusThrough(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	packet := []byte{0x11, 0x22, 0x33}
	got, err := input.prepare(&genx.Blob{MIMEType: "audio/opus", Data: packet})
	if err != nil {
		t.Fatalf("prepare speech_opus: %v", err)
	}
	if !bytes.Equal(got, packet) {
		t.Fatalf("prepare speech_opus = %v, want %v", got, packet)
	}
}

func TestDoubaoRealtimeAudioInputRejectsOggForSpeechOpus(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	if _, err := input.prepare(&genx.Blob{MIMEType: "audio/ogg", Data: []byte("OggS")}); err == nil {
		t.Fatal("prepare speech_opus audio/ogg error = nil, want error")
	}
}

func TestDoubaoRealtimeAudioInputRejectsUnknownForSpeechOpus(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	if _, err := input.prepare(&genx.Blob{MIMEType: "application/octet-stream", Data: []byte{1, 2, 3}}); err == nil {
		t.Fatal("prepare speech_opus unknown MIME error = nil, want error")
	}
}

func TestDoubaoRealtimeAudioInputsArePerStream(t *testing.T) {
	inputs := newDoubaoRealtimeAudioInputs("speech_opus", 16000, 1, true)
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
	if got := chunkInputStreamID(chunk, "turn-1"); got != "turn-1" {
		t.Fatalf("chunkInputStreamID(audio) = %q, want active stream", got)
	}
	chunk.Ctrl.StreamID = "turn-2"
	if got := chunkInputStreamID(chunk, "turn-1"); got != "turn-2" {
		t.Fatalf("chunkInputStreamID(explicit) = %q, want explicit stream", got)
	}
}

func TestDoubaoRealtimeStreamIDsSplitRealtimeTranscript(t *testing.T) {
	ids := newDoubaoRealtimeStreamIDs(DoubaoRealtimeModeRealtime)
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

func TestDoubaoRealtimeStreamIDsKeepPushToTalkInput(t *testing.T) {
	ids := newDoubaoRealtimeStreamIDs(DoubaoRealtimeModePushToTalk)
	ids.beginInput("turn-1")
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}}

	if got := ids.historyInput(chunk); got != "turn-1" {
		t.Fatalf("push-to-talk history input = %q, want base stream", got)
	}
	if got := ids.endInputSegment(); got != "turn-1" {
		t.Fatalf("push-to-talk ended segment = %q, want base stream", got)
	}
	if got := ids.historyInput(chunk); got != "turn-1" {
		t.Fatalf("push-to-talk next history input = %q, want same base stream", got)
	}
}

func TestDoubaoRealtimeStreamIDsInferRealtimeInputWithoutBOS(t *testing.T) {
	ids := newDoubaoRealtimeStreamIDs(DoubaoRealtimeModeRealtime)
	chunk := &genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "turn-1"}}

	if got := ids.serviceInput(chunk); got != "turn-1" {
		t.Fatalf("service input = %q, want chunk stream", got)
	}
	if got := ids.input(); got != "turn-1:rt:1" {
		t.Fatalf("transcript input = %q, want chunk-derived realtime segment", got)
	}
}

func TestRealtimeASRResponseEndsSegment(t *testing.T) {
	if !realtimeASRResponseEndsSegment(&doubaospeech.RealtimeEvent{
		Results: []doubaospeech.RealtimeASRResult{{Text: "第一段", IsInterim: false}},
	}, "第一段") {
		t.Fatal("final ASR result did not end segment")
	}
	if realtimeASRResponseEndsSegment(&doubaospeech.RealtimeEvent{
		Results: []doubaospeech.RealtimeASRResult{{Text: "第一", IsInterim: true}},
	}, "第一") {
		t.Fatal("interim ASR response ended segment")
	}
	if realtimeASRResponseEndsSegment(&doubaospeech.RealtimeEvent{IsFinal: true, Text: "。"}, "。") {
		t.Fatal("punctuation-only ASR response ended segment")
	}
	if realtimeASRResponseEndsSegment(&doubaospeech.RealtimeEvent{Text: "第二段"}, "第二段") {
		t.Fatal("ASR response without final marker ended segment")
	}
}

func TestDoubaoRealtimeAudioInputEncodesSpeechOpusSilence(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	defer input.close()
	frames, err := input.silenceFrames(2)
	if err != nil {
		t.Fatalf("silenceFrames: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("silenceFrames len = %d, want 2", len(frames))
	}
	for i, frame := range frames {
		if len(frame) == 0 {
			t.Fatalf("silence frame %d is empty", i)
		}
		if len(frame) == 640 {
			t.Fatalf("silence frame %d len = 640, looks like PCM", i)
		}
	}
}

func TestDoubaoRealtimeAudioInputTranscodesSpeechOpus(t *testing.T) {
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

	input := newDoubaoRealtimeAudioInput("speech_opus", targetSampleRate, channels, true)
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

func TestDoubaoRealtimeAudioInputEncodesMP3ToSpeechOpus(t *testing.T) {
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

	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, true)
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

func TestDoubaoRealtimeAudioInputsRejectMIMEChange(t *testing.T) {
	inputs := newDoubaoRealtimeAudioInputs("speech_opus", 16000, 1, true)
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

func TestDoubaoRealtimeConfigSetsDuplexSession(t *testing.T) {
	strict := true
	enableMusic := true
	tfr := NewDoubaoRealtime(nil,
		WithDoubaoRealtimeMode(DoubaoRealtimeModeText),
		WithDoubaoRealtimeModel("1.2.6.0"),
		WithDoubaoRealtimeInstructions("简短回答。"),
		WithDoubaoRealtimeSpeaker("voice-a"),
		WithDoubaoRealtimeFormat("ogg_opus"),
		WithDoubaoRealtimeSampleRate(24000),
		WithDoubaoRealtimeInputFormat("speech_opus"),
		WithDoubaoRealtimeInputSampleRate(16000),
		WithDoubaoRealtimeOutputSpeed(1),
		WithDoubaoRealtimeOutputLoudness(-1),
		WithDoubaoRealtimeTools([]doubaospeech.RealtimeDuplexFunctionTool{{
			Type:   "function",
			Name:   "get_weather",
			Strict: &strict,
			Parameters: &doubaospeech.RealtimeDuplexJSONSchema{
				Type:                 "object",
				AdditionalProperties: &strict,
			},
		}}),
		WithDoubaoRealtimeExtension(&doubaospeech.RealtimeDuplexExtension{
			Dialog: &doubaospeech.RealtimeDuplexDialogExtension{
				Extra: &doubaospeech.RealtimeDuplexDialogExtra{EnableMusic: &enableMusic},
			},
		}),
	)
	cfg := tfr.realtimeConfig()
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
	stream := withPendingChunk(rest, &genx.MessageChunk{Part: genx.Text("first")})

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
	got := pcm16LE([]int16{1, -2})
	want := []byte{1, 0, 254, 255}
	if !bytes.Equal(got, want) {
		t.Fatalf("pcm16LE = %v, want %v", got, want)
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
	if got := realtimeASRText(payload); got != "你好，能听见我说话吗？" {
		t.Fatalf("realtimeASRText(final) = %q", got)
	}

	payload = []byte(`{"extra":{"origin_text":"你好"}}`)
	if got := realtimeASRText(payload); got != "你好" {
		t.Fatalf("realtimeASRText(origin) = %q", got)
	}

	payload = []byte(`{"results":[{"alternatives":[{"text":"候选"}]}]}`)
	if got := realtimeASRText(payload); got != "候选" {
		t.Fatalf("realtimeASRText(alternative) = %q", got)
	}
}

func TestRealtimeTextDelta(t *testing.T) {
	if got := realtimeTextDelta("你好", "你好世界"); got != "世界" {
		t.Fatalf("realtimeTextDelta prefix = %q", got)
	}
	if got := realtimeTextDelta("你好", "再见"); got != "再见" {
		t.Fatalf("realtimeTextDelta replacement = %q", got)
	}
	if got := realtimeTextDelta("你好", "你好"); got != "" {
		t.Fatalf("realtimeTextDelta duplicate = %q", got)
	}
	if got := realtimeTextDelta("你好能听到我说话吗", "你好，能听到我说话吗？"); got != "？" {
		t.Fatalf("realtimeTextDelta punctuated prefix = %q", got)
	}
	if got := realtimeTextDelta("嗯今天天气怎么样我想出门走走", "今天天气怎么样？我想出门走走。"); got != "" {
		t.Fatalf("realtimeTextDelta replacement subset = %q", got)
	}
}

func TestDoubaoRealtimeOutputAudioBlobsExtractsOggOpusPackets(t *testing.T) {
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

	tfr := NewDoubaoRealtime(nil, WithDoubaoRealtimeFormat("ogg_opus"))
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

func TestDoubaoRealtimeSendsFakeFunctionCallOutputs(t *testing.T) {
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
	tfr := NewDoubaoRealtime(nil, withDoubaoRealtimeDuplexOpener(opener))
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

func TestDoubaoRealtimeReturnsFunctionCallOutputError(t *testing.T) {
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
	tfr := NewDoubaoRealtime(nil)
	_, err := tfr.processLoop(context.Background(), emptyRealtimeStream{}, newBufferStream(1), session)
	if !errors.Is(err, wantErr) {
		t.Fatalf("processLoop() error = %v, want %v", err, wantErr)
	}
	if !session.closed {
		t.Fatal("session was not closed")
	}
}

func TestDoubaoRealtimeReturnsDuplexErrorEvent(t *testing.T) {
	wantErr := &doubaospeech.Error{Code: 500, Message: "duplex failed"}
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{Type: doubaospeech.RealtimeDuplexEventError, Error: wantErr},
		},
	}
	tfr := NewDoubaoRealtime(nil)
	_, err := tfr.processLoop(context.Background(), emptyRealtimeStream{}, newBufferStream(1), session)
	if !errors.Is(err, wantErr) {
		t.Fatalf("processLoop() error = %v, want %v", err, wantErr)
	}
	if !session.closed {
		t.Fatal("session was not closed")
	}
}

func TestDoubaoRealtimeErrorEventClosesBlockedInput(t *testing.T) {
	wantErr := &doubaospeech.Error{Code: 500, Message: "duplex failed"}
	input := newBlockingRealtimeStream()
	session := &fakeDoubaoRealtimeDuplexSession{
		beforeRecv: input.started,
		events: []*doubaospeech.RealtimeDuplexEvent{
			{Type: doubaospeech.RealtimeDuplexEventError, Error: wantErr},
		},
	}
	tfr := NewDoubaoRealtime(nil)

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

func TestDoubaoRealtimeMapsDuplexEventsToStreamChunks(t *testing.T) {
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
	tfr := NewDoubaoRealtime(nil,
		withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}),
		WithDoubaoRealtimeFormat("pcm"),
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

func TestDoubaoRealtimeUsesDoneTextWhenNoDeltaArrived(t *testing.T) {
	session := &fakeDoubaoRealtimeDuplexSession{
		events: []*doubaospeech.RealtimeDuplexEvent{
			{Type: doubaospeech.RealtimeDuplexEventResponseOutputTextDone, ResponseID: "resp-1", Text: "最终文本"},
			{Type: doubaospeech.RealtimeDuplexEventSessionClosed},
		},
	}
	tfr := NewDoubaoRealtime(nil, withDoubaoRealtimeDuplexOpener(&fakeDoubaoRealtimeDuplexOpener{session: session}))
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

func testRealtimeOpusHeadPacket(sampleRate, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet, []byte("OpusHead"))
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:], uint32(sampleRate))
	return packet
}

type fakeDoubaoRealtimeDuplexOpener struct {
	config  *doubaospeech.RealtimeDuplexConfig
	session *fakeDoubaoRealtimeDuplexSession
}

func (o *fakeDoubaoRealtimeDuplexOpener) OpenSession(_ context.Context, cfg *doubaospeech.RealtimeDuplexConfig) (doubaoRealtimeDuplexSession, error) {
	o.config = cfg
	return o.session, nil
}

type fakeDoubaoRealtimeDuplexSession struct {
	events          []*doubaospeech.RealtimeDuplexEvent
	beforeRecv      <-chan struct{}
	outputs         []doubaospeech.RealtimeDuplexFunctionCallOutput
	functionCallErr error
	closed          bool
}

func (s *fakeDoubaoRealtimeDuplexSession) SendAudio(context.Context, []byte) error { return nil }

func (s *fakeDoubaoRealtimeDuplexSession) CommitAudio(context.Context) error { return nil }

func (s *fakeDoubaoRealtimeDuplexSession) SendSpeechText(context.Context, doubaospeech.RealtimeDuplexSpeechTextRequest) error {
	return nil
}

func (s *fakeDoubaoRealtimeDuplexSession) CancelResponse(context.Context) error { return nil }

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
				return
			}
		}
	}
}

func (s *fakeDoubaoRealtimeDuplexSession) Close() error {
	s.closed = true
	return nil
}

type emptyRealtimeStream struct{}

func (emptyRealtimeStream) Next() (*genx.MessageChunk, error) { return nil, io.EOF }

func (emptyRealtimeStream) Close() error { return nil }

func (emptyRealtimeStream) CloseWithError(error) error { return nil }

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
