package doubaorealtime

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestRealtimeAudioHelpers(t *testing.T) {
	if got := realtimeBaseMIME(" Audio/L16; Rate=16000 "); got != "audio/l16" {
		t.Fatalf("realtimeBaseMIME() = %q", got)
	}
	if got := realtimeAudioFormat(" Speech_Opus "); got != "speech_opus" {
		t.Fatalf("realtimeAudioFormat() = %q", got)
	}
	if got := realtimeAudioSampleRate(-1); got != 16000 {
		t.Fatalf("realtimeAudioSampleRate() = %d", got)
	}
	if got := realtimeAudioChannels(0); got != 1 {
		t.Fatalf("realtimeAudioChannels() = %d", got)
	}
	if got := realtimeStreamKey(" "); got != "default" {
		t.Fatalf("realtimeStreamKey() = %q", got)
	}
	if !isRealtimeOpusMIME("audio/ogg; codecs=opus") || !isRealtimePCMInputMIME("audio/x-pcm") ||
		!isRealtimeMP3InputMIME("audio/x-mp3") {
		t.Fatal("audio MIME aliases were not recognized")
	}
	if isRealtimeOpusMIME("audio/pcm") || isRealtimePCMInputMIME("audio/mpeg") || isRealtimeMP3InputMIME("audio/opus") {
		t.Fatal("unrelated audio MIME was recognized")
	}
	if got := realtimePCM16LE(nil); got != nil {
		t.Fatalf("realtimePCM16LE(nil) = %v", got)
	}
	if got := blobMIMEType(nil); got != "" {
		t.Fatalf("blobMIMEType(nil) = %q", got)
	}
	err := (&doubaoRealtimeStreamMIMEChangeError{StreamID: "s", From: "audio/pcm", To: "audio/mpeg"}).Error()
	if !strings.Contains(err, `stream "s" changed MIME type`) {
		t.Fatalf("MIME change error = %q", err)
	}
}

func TestRealtimeAudioInputsLifecycle(t *testing.T) {
	var nilInputs *doubaoRealtimeAudioInputs
	if input := nilInputs.stream("route"); input == nil || input.format != "pcm" {
		t.Fatalf("nil stream() = %#v", input)
	}
	if input, err := nilInputs.streamForBlob("route", nil); err != nil || input == nil {
		t.Fatalf("nil streamForBlob() = (%#v, %v)", input, err)
	}
	nilInputs.closeStream("route")
	nilInputs.close()

	inputs := newDoubaoRealtimeAudioInputs("pcm", 16000, 1, false)
	first := inputs.stream(" route ")
	if first != inputs.stream("route") {
		t.Fatal("stream() did not cache by canonical StreamID")
	}
	if _, err := inputs.streamForBlob("route", nil); err != nil {
		t.Fatalf("streamForBlob(nil) error = %v", err)
	}
	if _, err := inputs.streamForBlob("route", &genx.Blob{MIMEType: " Audio/PCM; rate=16000 "}); err != nil {
		t.Fatalf("streamForBlob(first MIME) error = %v", err)
	}
	if _, err := inputs.streamForBlob("route", &genx.Blob{MIMEType: "audio/mpeg"}); err == nil {
		t.Fatal("streamForBlob() accepted a MIME change")
	}
	inputs.closeStream("route")
	if len(inputs.streams) != 0 || len(inputs.mimeTypes) != 0 {
		t.Fatalf("closeStream() retained state: streams=%d MIME=%d", len(inputs.streams), len(inputs.mimeTypes))
	}
	inputs.stream("one")
	inputs.streamForBlob("two", &genx.Blob{MIMEType: "audio/pcm"})
	inputs.close()
	if len(inputs.streams) != 0 || len(inputs.mimeTypes) != 0 {
		t.Fatalf("close() retained state: streams=%d MIME=%d", len(inputs.streams), len(inputs.mimeTypes))
	}
}

func TestRealtimeAudioInputPrepareBranches(t *testing.T) {
	pcm := []byte{1, 0, 2, 0}
	for _, format := range []string{"pcm", "pcm_s16le", "unknown"} {
		input := newDoubaoRealtimeAudioInput(format, 16000, 1, false)
		got, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/pcm", Data: pcm})
		if err != nil || len(got) != 1 || !bytes.Equal(got[0], pcm) {
			t.Fatalf("%s prepareFrames(PCM) = (%v, %v)", format, got, err)
		}
	}

	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	defer input.close()
	if got, err := input.prepareFrames(nil); err != nil || got != nil {
		t.Fatalf("prepareFrames(nil) = (%v, %v)", got, err)
	}
	if got, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/pcm"}); err != nil || got != nil {
		t.Fatalf("prepareFrames(empty) = (%v, %v)", got, err)
	}
	if _, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/pcm", Data: []byte{1}}); err == nil || !strings.Contains(err.Error(), "must be even") {
		t.Fatalf("odd PCM error = %v", err)
	}
	if _, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/ogg", Data: []byte{1}}); err == nil || !strings.Contains(err.Error(), "raw Opus") {
		t.Fatalf("Ogg input error = %v", err)
	}
	if _, err := input.prepareFrames(&genx.Blob{MIMEType: "application/octet-stream", Data: []byte{1}}); err == nil || !strings.Contains(err.Error(), "requires audio/opus, PCM, or MP3") {
		t.Fatalf("unknown input error = %v", err)
	}
	if _, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/mpeg", Data: []byte("not mp3")}); err == nil || !strings.Contains(err.Error(), "decode mp3") {
		t.Fatalf("invalid MP3 error = %v", err)
	}

	oneFramePCM := make([]byte, input.frameSize*input.channels*2)
	oneFrame, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/pcm", Data: oneFramePCM})
	if err != nil || len(oneFrame) != 1 || len(oneFrame[0]) == 0 {
		t.Fatalf("prepareFrames(one PCM frame) = (%d, %v)", len(oneFrame), err)
	}
	if got, err := input.prepare(&genx.Blob{MIMEType: "audio/pcm", Data: oneFramePCM}); err != nil || len(got) == 0 {
		t.Fatalf("prepare(one PCM frame) = (%d bytes, %v)", len(got), err)
	}
	twoFramePCM := make([]byte, len(oneFramePCM)*2)
	if _, err := input.prepare(&genx.Blob{MIMEType: "audio/pcm", Data: twoFramePCM}); err == nil || !strings.Contains(err.Error(), "produced 2 frames") {
		t.Fatalf("prepare(two frames) error = %v", err)
	}

	rawOpus := oneFrame[0]
	passthrough, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/opus", Data: rawOpus})
	if err != nil || len(passthrough) != 1 || !bytes.Equal(passthrough[0], rawOpus) {
		t.Fatalf("prepareFrames(raw Opus) = (%v, %v)", passthrough, err)
	}
	transcoding := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, true)
	defer transcoding.close()
	if got, err := transcoding.prepareFrames(&genx.Blob{MIMEType: "audio/opus", Data: rawOpus}); err != nil || len(got) != 1 || len(got[0]) == 0 {
		t.Fatalf("prepareFrames(transcoded Opus) = (%v, %v)", got, err)
	}
	if _, err := transcoding.transcodeOpus([]byte{0xff}); err == nil {
		t.Fatal("transcodeOpus() accepted an invalid packet")
	}

	pcmOutput := newDoubaoRealtimeAudioInput("pcm", 16000, 1, false)
	defer pcmOutput.close()
	decoded, err := pcmOutput.prepareFrames(&genx.Blob{MIMEType: "audio/opus", Data: rawOpus})
	if err != nil || len(decoded) != 1 || len(decoded[0]) != len(oneFramePCM) {
		t.Fatalf("prepareFrames(Opus to PCM) = (%v, %v)", decoded, err)
	}
	if _, err := pcmOutput.prepareFrames(&genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}}); err == nil {
		t.Fatal("PCM output accepted an invalid Opus packet")
	}
	if _, err := pcmOutput.prepareFrames(&genx.Blob{MIMEType: "audio/mpeg", Data: []byte("not mp3")}); err == nil {
		t.Fatal("PCM output accepted invalid MP3")
	}

	ogg := newDoubaoRealtimeAudioInput("ogg_opus", 16000, 1, false)
	if got, err := ogg.prepareFrames(&genx.Blob{MIMEType: "audio/ogg", Data: []byte("page")}); err != nil || len(got) != 1 {
		t.Fatalf("Ogg passthrough = (%v, %v)", got, err)
	}
	if _, err := ogg.prepareFrames(&genx.Blob{MIMEType: "audio/opus", Data: rawOpus}); err == nil || !strings.Contains(err.Error(), "Ogg/Opus pages") {
		t.Fatalf("raw Opus to Ogg error = %v", err)
	}
	if got, err := ogg.prepareFrames(&genx.Blob{MIMEType: "application/octet-stream", Data: []byte("page")}); err != nil || len(got) != 1 {
		t.Fatalf("Ogg default passthrough = (%v, %v)", got, err)
	}
}

func TestRealtimeAudioInputCodecBoundaries(t *testing.T) {
	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	defer input.close()
	if frames, err := input.silenceFrames(0); err != nil || frames != nil {
		t.Fatalf("silenceFrames(0) = (%v, %v)", frames, err)
	}
	if frame, err := input.encodeOpus(nil); err != nil || frame != nil {
		t.Fatalf("encodeOpus(nil) = (%v, %v)", frame, err)
	}
	if _, err := input.encodeOpusSamples(make([]int16, input.frameSize-1)); err == nil || !strings.Contains(err.Error(), "want") {
		t.Fatalf("encodeOpusSamples(short) error = %v", err)
	}

	invalid := &doubaoRealtimeAudioInput{format: "speech_opus", frameSize: 0, channels: 1}
	if _, err := invalid.encodeOpusFrames([]byte{0, 0}); err == nil || !strings.Contains(err.Error(), "invalid opus frame size") {
		t.Fatalf("encodeOpusFrames(invalid frame) error = %v", err)
	}
	invalid.close()

	ogg := newDoubaoRealtimeAudioInput("ogg_opus", 16000, 1, false)
	if _, err := ogg.silenceFrames(1); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Ogg silence error = %v", err)
	}
	pcm := newDoubaoRealtimeAudioInput("pcm", 16000, 2, false)
	frames, err := pcm.silenceFrames(2)
	if err != nil || len(frames) != 2 || len(frames[0]) != pcm.frameSize*pcm.channels*2 {
		t.Fatalf("PCM silenceFrames() = (%d frames, %v)", len(frames), err)
	}
	pcm.close()
	pcm.close()
}

func TestRealtimeAudioInputOpusKeepaliveAvoidsDTXPackets(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	defer input.close()
	frames, err := input.keepaliveFrames(5)
	if err != nil {
		t.Fatalf("keepaliveFrames() error = %v", err)
	}
	if len(frames) != 5 {
		t.Fatalf("keepaliveFrames() count = %d, want 5", len(frames))
	}
	for index, frame := range frames {
		if len(frame) <= 3 {
			t.Fatalf("keepalive frame %d length = %d, want non-DTX Opus packet", index+1, len(frame))
		}
	}
}

func TestRealtimeAudioInputPrepareFramesReportsSignal(t *testing.T) {
	loud := make([]int16, 320)
	for i := range loud {
		if i%2 == 0 {
			loud[i] = 4000
		} else {
			loud[i] = -4000
		}
	}
	quiet := make([]int16, 320)
	for i := range quiet {
		quiet[i] = int16(i % 7)
	}
	if !realtimePCMHasSignal(loud, doubaoRealtimeSignalRMS) || realtimePCMHasSignal(quiet, doubaoRealtimeSignalRMS) || realtimePCMHasSignal(nil, 1) {
		t.Fatal("realtimePCMHasSignal() did not separate speech-level audio from the noise floor")
	}

	pcmInput := newDoubaoRealtimeAudioInput("pcm", 16000, 1, false)
	frames, signal, err := pcmInput.prepareFramesWithSignal(&genx.Blob{MIMEType: "audio/pcm", Data: pcm16LE(loud)})
	if err != nil || len(frames) != 1 || signal != 20*time.Millisecond {
		t.Fatalf("loud pcm prepareFramesWithSignal() = (%d frames, %s, %v), want one 20ms signal frame", len(frames), signal, err)
	}
	frames, signal, err = pcmInput.prepareFramesWithSignal(&genx.Blob{MIMEType: "audio/pcm", Data: pcm16LE(quiet)})
	if err != nil || len(frames) != 1 || signal != 0 {
		t.Fatalf("quiet pcm prepareFramesWithSignal() = (%d frames, %s, %v), want one silent frame", len(frames), signal, err)
	}
	if _, signal, err := pcmInput.prepareFramesWithSignal(nil); err != nil || signal != 0 {
		t.Fatalf("nil blob prepareFramesWithSignal() = (%s, %v)", signal, err)
	}

	opusInput := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, true)
	defer opusInput.close()
	enc, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder() error = %v", err)
	}
	defer enc.Close()
	packet, err := enc.Encode(loud, 320)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	frames, signal, err = opusInput.prepareFramesWithSignal(&genx.Blob{MIMEType: "audio/opus", Data: packet})
	if err != nil || len(frames) != 1 || signal != 20*time.Millisecond {
		t.Fatalf("transcoded opus prepareFramesWithSignal() = (%d frames, %s, %v), want one 20ms signal frame", len(frames), signal, err)
	}

	passthrough := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
	frames, signal, err = passthrough.prepareFramesWithSignal(&genx.Blob{MIMEType: "audio/opus", Data: []byte{1, 2, 3}})
	if err != nil || len(frames) != 1 || signal != 20*time.Millisecond {
		t.Fatalf("passthrough opus prepareFramesWithSignal() = (%d frames, %s, %v), want undecoded audio counted as signal", len(frames), signal, err)
	}
}
