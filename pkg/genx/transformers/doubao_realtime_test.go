package transformers

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

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

func testRealtimeOpusHeadPacket(sampleRate, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet, []byte("OpusHead"))
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:], uint32(sampleRate))
	return packet
}
