package agenthost

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
)

// levelPCM returns mono 16-bit PCM of duration whose samples alternate
// between +level and -level.
func levelPCM(format pcm.Format, duration time.Duration, level int16) []byte {
	data := make([]byte, format.BytesInDuration(duration))
	for offset := 0; offset+1 < len(data); offset += 2 {
		sample := level
		if offset/2%2 == 1 {
			sample = -level
		}
		binary.LittleEndian.PutUint16(data[offset:], uint16(sample))
	}
	return data
}

func trimAll(t *testing.T, trimmer *audioLeadTrimmer, chunks ...[]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	for _, data := range chunks {
		for _, chunk := range trimmer.trim(pcm.L16Mono16K.DataChunk(data)) {
			if _, err := chunk.WriteTo(&out); err != nil {
				t.Fatal(err)
			}
		}
	}
	return out.Bytes()
}

// Provider TTS opens with a quiet lead-in the listener cannot hear. It is
// dropped up to the preroll ahead of the first audible sample, so the reply
// starts that much sooner, while speech and later pauses play unchanged.
func TestAudioLeadTrimmerDropsQuietLeadIn(t *testing.T) {
	format := pcm.L16Mono16K
	quiet := levelPCM(format, 400*time.Millisecond, 80)
	speech := levelPCM(format, 200*time.Millisecond, 8000)
	pause := levelPCM(format, 300*time.Millisecond, 80)
	var trimmer audioLeadTrimmer
	out := trimAll(t, &trimmer, quiet[:len(quiet)/2], quiet[len(quiet)/2:], speech, pause, speech)

	preroll := int(format.BytesInDuration(audioLeadPreroll))
	want := append(append(append(append([]byte(nil), quiet[len(quiet)-preroll:]...), speech...), pause...), speech...)
	if !bytes.Equal(out, want) {
		t.Fatalf("trimmed audio = %d bytes, want %d: the preroll, then speech and its pause unchanged", len(out), len(want))
	}
	if got := trimmer.dropped; got != 400*time.Millisecond-audioLeadPreroll {
		t.Fatalf("dropped = %s, want %s", got, 400*time.Millisecond-audioLeadPreroll)
	}
}

// Audio that stays quiet past the trim bound is deliberate, so it plays from
// there on instead of being withheld.
func TestAudioLeadTrimmerBoundsDroppedAudio(t *testing.T) {
	format := pcm.L16Mono16K
	quiet := levelPCM(format, 100*time.Millisecond, 50)
	var trimmer audioLeadTrimmer
	var chunks [][]byte
	for range 30 {
		chunks = append(chunks, quiet)
	}
	out := trimAll(t, &trimmer, chunks...)
	if !trimmer.done {
		t.Fatal("trimmer still holding audio after 3s of quiet")
	}
	if trimmer.dropped > audioLeadMaxTrim {
		t.Fatalf("dropped = %s, want at most %s", trimmer.dropped, audioLeadMaxTrim)
	}
	if got, want := format.Duration(int64(len(out)))+trimmer.dropped, 3*time.Second; got != want {
		t.Fatalf("played + dropped = %s, want all %s accounted for", got, want)
	}
}

func TestAudioLeadTrimmerKeepsStereoFramesAligned(t *testing.T) {
	format := pcm.L16Stereo16K
	data := make([]byte, format.BytesInDuration(20*time.Millisecond))
	// Left channel quiet throughout; the right channel of frame 100 is loud.
	onset := 100 * 4
	binary.LittleEndian.PutUint16(data[onset+2:], uint16(9000))
	var trimmer audioLeadTrimmer
	chunks := trimmer.trim(format.DataChunk(data))
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	out := chunks[0].(*pcm.DataChunk).Data
	if len(out)%4 != 0 || !bytes.Equal(out, data) {
		t.Fatalf("stereo output = %d bytes, want the whole frame-aligned chunk kept (onset within the preroll)", len(out))
	}
}

// A route whose provider lead-in precedes speech opens its mixer track only
// once audible audio arrives, so the first mixer frame is the preroll rather
// than the whole lead-in.
func TestAudioOutputTracksTrimRouteLeadIn(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	mimeType := "audio/L16; rate=16000; channels=1"
	quiet := levelPCM(pcm.L16Mono16K, 300*time.Millisecond, 60)
	if err := tracks.consume(pcmOutputChunk("answer", mimeType, quiet, false, "")); err != nil {
		t.Fatalf("consume(lead-in) error = %v", err)
	}
	if got := len(creator.tracks); got != 0 {
		t.Fatalf("tracks after the quiet lead-in = %d, want 0", got)
	}
	speech := levelPCM(pcm.L16Mono16K, 200*time.Millisecond, 8000)
	if err := tracks.consume(pcmOutputChunk("answer", mimeType, speech, true, "")); err != nil {
		t.Fatalf("consume(speech) error = %v", err)
	}
	if got := len(creator.tracks); got != 1 {
		t.Fatalf("tracks after speech = %d, want 1", got)
	}
	var mixed bytes.Buffer
	frame := make([]byte, pcm.L16Mono16K.BytesInDuration(20*time.Millisecond))
	for range 13 {
		if _, err := creator.mixer.Read(frame); err != nil {
			t.Fatalf("mixer.Read() error = %v", err)
		}
		mixed.Write(frame)
	}
	onset := audibleFrameOffset(mixed.Bytes(), 2)
	if got := pcm.L16Mono16K.Duration(int64(onset)); onset < 0 || got > audioLeadPreroll {
		t.Fatalf("first audible sample at %s of mixed audio, want within the %s preroll", got, audioLeadPreroll)
	}
}

// Opening a track without a track creator reports the missing creator rather
// than dereferencing it.
func TestAudioOutputTracksOpenTrackRequiresCreator(t *testing.T) {
	tracks := newAudioOutputTracks(nil)
	channel := &audioOutputChannel{key: audioOutputKey{streamID: "answer", mimeType: "audio/pcm"}}
	err := tracks.writePCM(channel, []pcm.Chunk{pcm.L16Mono16K.DataChunk(levelPCM(pcm.L16Mono16K, 20*time.Millisecond, 8000))})
	if err == nil || !strings.Contains(err.Error(), "audio track creator") {
		t.Fatalf("writePCM() without creator error = %v, want audio track creator", err)
	}
}
