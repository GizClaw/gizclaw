package doubaorealtime

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestPCMOpusContinuousFramesAndSingleTail(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	pcm := bytes.Repeat([]byte{1, 2, 3, 4}, 240) // 30ms at 16k mono.
	convert := func(chunkBytes int) [][]byte {
		input := newDoubaoRealtimeAudioInput("speech_opus", 16000, 1, false)
		defer input.close()
		var frames [][]byte
		for offset := 0; offset < len(pcm); offset += chunkBytes {
			blob := &genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: pcm[offset:min(offset+chunkBytes, len(pcm))], ConcealedAudio: offset/chunkBytes%2 == 1}
			if !blob.ConcealedAudio {
				// Invalid encoded bytes prove that real frames use the same PCM
				// encoder as fractional concealment, never original passthrough.
				blob.OriginalAudio = &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}}
			}
			part, err := input.prepareFrames(blob)
			if err != nil {
				t.Fatal(err)
			}
			frames = append(frames, part...)
		}
		if len(frames) != 1 {
			t.Fatalf("30ms input emitted %d full frames before EOS, want 1", len(frames))
		}
		tail, err := input.flushFrames()
		if err != nil || len(tail) != 1 {
			t.Fatalf("tail flush = %d frames, %v", len(tail), err)
		}
		frames = append(frames, tail...)
		if tail, err := input.flushFrames(); err != nil || len(tail) != 0 {
			t.Fatalf("duplicate EOS flushed %d frames, %v", len(tail), err)
		}
		return frames
	}
	want := convert(len(pcm))
	for _, chunkBytes := range []int{80, 320} { // 2.5ms and 10ms.
		t.Run(fmt.Sprint(chunkBytes), func(t *testing.T) {
			got := convert(chunkBytes)
			for i := range want {
				if !bytes.Equal(got[i], want[i]) {
					t.Fatalf("chunking or provenance changed continuous encoder frame %d", i)
				}
			}
		})
	}
}

func TestPCMOpusLargeChunkRetainsOnlyOwnedTail(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	for _, format := range []struct{ rate, channels int }{{16000, 1}, {48000, 2}} {
		for _, tailSamples := range []int{0, 7} {
			t.Run(fmt.Sprintf("%d_%d_tail%d", format.rate, format.channels, tailSamples), func(t *testing.T) {
				const wholeFrames = 128
				frameSamples := format.rate / 50
				frameBytes := frameSamples * format.channels * 2
				tailBytes := tailSamples * format.channels * 2
				source := make([]byte, wholeFrames*frameBytes+tailBytes)
				state := uint32(17)
				for offset := 0; offset < len(source); offset += 2 {
					state = state*1664525 + 1013904223
					binary.LittleEndian.PutUint16(source[offset:], uint16(state>>16))
				}
				input := newDoubaoRealtimeAudioInput("speech_opus", format.rate, format.channels, false)
				defer input.close()
				mimeType := fmt.Sprintf("audio/pcm;rate=%d;channels=%d", format.rate, format.channels)
				prefix := bytes.Clone(source[:frameBytes/4])
				if frames, err := input.prepareFrames(&genx.Blob{MIMEType: mimeType, Data: prefix}); err != nil || len(frames) != 0 {
					t.Fatalf("partial prefix = %d frames, %v", len(frames), err)
				}
				clear(prefix) // The partial frame must already own its bytes.
				chunk := bytes.Clone(source[len(prefix):])
				frames, err := input.prepareFrames(&genx.Blob{MIMEType: mimeType, Data: chunk})
				if err != nil || len(frames) != wholeFrames {
					t.Fatalf("large chunk = %d frames, %v; want %d", len(frames), err, wholeFrames)
				}
				if len(input.pendingPCM) != tailBytes || cap(input.pendingPCM) > frameBytes {
					t.Fatalf("retained PCM len/cap = %d/%d, want %d/at most %d", len(input.pendingPCM), cap(input.pendingPCM), tailBytes, frameBytes)
				}
				clear(chunk) // The retained tail must not alias the large input.
				if !bytes.Equal(input.pendingPCM, source[wholeFrames*frameBytes:]) {
					t.Fatal("retained PCM changed after caller reused its input")
				}
				tail, err := input.flushFrames()
				wantTail := 0
				if tailBytes > 0 {
					wantTail = 1
				}
				if err != nil || len(tail) != wantTail {
					t.Fatalf("tail flush = %d frames, %v; want %d", len(tail), err, wantTail)
				}
				frames = append(frames, tail...)
				if again, err := input.flushFrames(); err != nil || len(again) != 0 {
					t.Fatalf("duplicate flush = %d frames, %v", len(again), err)
				}

				// Compare against an independently driven native encoder, using its
				// full output capacity and one continuous state for the whole route.
				reference, err := opus.NewEncoder(format.rate, format.channels, opus.ApplicationAudio)
				if err != nil {
					t.Fatal(err)
				}
				defer reference.Close()
				samples := make([]int16, frameSamples*format.channels)
				for index, frame := range frames {
					clear(samples)
					data := source[index*frameBytes : min((index+1)*frameBytes, len(source))]
					for i := 0; i < len(data)/2; i++ {
						samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
					}
					want, err := reference.Encode(samples, frameSamples)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(frame, want) {
						t.Fatalf("frame %d differs from the continuous native encoding", index)
					}
				}
			})
		}
	}
}

func TestSharedAudioInputsFlushesOnlyExistingRoutes(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	inputs := NewSharedAudioInputs("speech_opus", 16000, 1, false)
	defer inputs.Close()
	for _, id := range []string{"first", "second", "canceled"} {
		input := inputs.Stream(id)
		if frames, err := input.PrepareFrames(&genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 80)}); err != nil || len(frames) != 0 {
			t.Fatalf("%s partial input = %d frames, %v", id, len(frames), err)
		}
	}
	if frames, err := inputs.FlushStream("missing"); err != nil || len(frames) != 0 || len(inputs.inputs) != 3 {
		t.Fatalf("unknown-route flush = %d frames, %v; registered routes = %d", len(frames), err, len(inputs.inputs))
	}
	if frames, err := inputs.FlushStream("first"); err != nil || len(frames) != 1 {
		t.Fatalf("first route flush = %d frames, %v", len(frames), err)
	}
	inputs.CloseStream("canceled")
	if frames, err := inputs.Flush(); err != nil || len(frames) != 1 {
		t.Fatalf("outer EOF flush = %d frames, %v; want only the second route", len(frames), err)
	}
	if frames, err := inputs.Flush(); err != nil || len(frames) != 0 {
		t.Fatalf("duplicate outer EOF flush = %d frames, %v", len(frames), err)
	}
	inputs.Close()
	if frames, err := inputs.FlushStream("first"); err != nil || len(frames) != 0 {
		t.Fatalf("closed collection flush = %d frames, %v", len(frames), err)
	}
}

func TestPCMInputAdaptsConfiguredRateAndChannels(t *testing.T) {
	for _, tc := range []struct{ rate, channels int }{{8000, 1}, {8000, 2}, {16000, 2}} {
		t.Run(fmt.Sprintf("%d_%d", tc.rate, tc.channels), func(t *testing.T) {
			input := newDoubaoRealtimeAudioInput("pcm", tc.rate, tc.channels, true)
			defer input.close()
			var pcm []byte
			for range 40 {
				frames, err := input.prepareFrames(&genx.Blob{MIMEType: "audio/pcm;rate=16000;channels=1", Data: make([]byte, 80)})
				if err != nil {
					t.Fatal(err)
				}
				for _, frame := range frames {
					pcm = append(pcm, frame...)
				}
			}
			tail, err := input.flushFrames()
			if err != nil {
				t.Fatal(err)
			}
			for _, frame := range tail {
				pcm = append(pcm, frame...)
			}
			if len(pcm) != tc.rate*tc.channels*2/10 {
				t.Fatalf("100ms PCM output = %d bytes", len(pcm))
			}
		})
	}
}

func TestPCMPartialFramesAreDiscardedOnRouteReplacement(t *testing.T) {
	inputs := newDoubaoRealtimeAudioInputs("speech_opus", 16000, 1, true)
	defer inputs.close()
	old := inputs.stream("old")
	if frames, err := old.prepareFrames(&genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 80)}); err != nil || len(frames) != 0 {
		t.Fatalf("fractional input emitted %d frames, %v", len(frames), err)
	}
	inputs.closeStream("old")
	if frames, err := old.flushFrames(); err != nil || len(frames) != 0 {
		t.Fatalf("canceled route flushed %d frames, %v", len(frames), err)
	}
	if frames, err := inputs.flushStream("old"); err != nil || len(frames) != 0 || len(inputs.streams) != 0 {
		t.Fatal("flush reopened a retired route")
	}
	fresh := inputs.stream("new")
	if frames, err := fresh.prepareFrames(&genx.Blob{MIMEType: "audio/pcm", Data: make([]byte, 560)}); err != nil || len(frames) != 0 {
		t.Fatalf("replacement inherited old partial frame: %d frames, %v", len(frames), err)
	}
	if frames, err := fresh.flushFrames(); err != nil || len(frames) != 1 {
		t.Fatalf("replacement tail flush = %d frames, %v", len(frames), err)
	}
}

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
