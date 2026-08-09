package doubaoast

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestASTAudioHelpersAndBoundaries(t *testing.T) {
	if got := audioDuration(make([]byte, 32000), doubaoASRSessionConfig{}); got != time.Second {
		t.Fatalf("audioDuration(defaults) = %v", got)
	}
	if got := pcm16LE([]int16{0x0102, -1}); !bytes.Equal(got, []byte{0x02, 0x01, 0xff, 0xff}) {
		t.Fatalf("pcm16LE() = %v", got)
	}
	if _, err := decodeMP3ToPCM([]byte("not mp3"), doubaoASRSessionConfig{sampleRate: 16000, channels: 1}); err == nil || !strings.Contains(err.Error(), "decode mp3") {
		t.Fatalf("decodeMP3ToPCM() error = %v", err)
	}

	var decoder *opus.Decoder
	if got, err := decodeRawOpusToPCM(nil, doubaoASRSessionConfig{}, &decoder); err != nil || got != nil {
		t.Fatalf("decodeRawOpusToPCM(nil) = (%v, %v)", got, err)
	}
	if _, err := decodeRawOpusToPCM([]byte{1}, doubaoASRSessionConfig{sampleRate: 16000, channels: 3}, &decoder); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("decodeRawOpusToPCM(channels) error = %v", err)
	}
	if _, err := decodeRawOpusToPCM([]byte{0xff}, doubaoASRSessionConfig{}, &decoder); err == nil || !strings.Contains(err.Error(), "decode raw opus") {
		t.Fatalf("decodeRawOpusToPCM(invalid) error = %v", err)
	}
	if decoder != nil {
		_ = decoder.Close()
	}

	if got := slices.Collect(splitDoubaoASRAudio([]byte{1, 2, 3}, 0)); len(got) != 1 || !bytes.Equal(got[0], []byte{1, 2, 3}) {
		t.Fatalf("splitDoubaoASRAudio(no split) = %v", got)
	}
	if got := slices.Collect(splitDoubaoASRAudio(nil, 0)); len(got) != 0 {
		t.Fatalf("splitDoubaoASRAudio(empty) = %v", got)
	}
	if got := slices.Collect(splitDoubaoASRAudio([]byte{1, 2, 3, 4, 5}, 2)); len(got) != 3 || !bytes.Equal(got[2], []byte{5}) {
		t.Fatalf("splitDoubaoASRAudio() = %v", got)
	}
	yields := 0
	for range splitDoubaoASRAudio([]byte{1, 2, 3, 4}, 1) {
		yields++
		break
	}
	if yields != 1 {
		t.Fatalf("early-stop yields = %d", yields)
	}
	if !isAudioMIME("audio/pcm") || !isOggAudioMIME("application/ogg") || !isASRMP3MIME("audio/x-mp3") ||
		!isASRPCMMIME("audio/l16; rate=16000") || !isASROpusMIME("audio/opus") {
		t.Fatal("audio MIME helper rejected a supported alias")
	}
	if isAudioMIME("text/plain") || isOggAudioMIME("audio/opus") || isASRMP3MIME("audio/pcm") ||
		isASRPCMMIME("audio/mpeg") || isASROpusMIME("audio/ogg") {
		t.Fatal("audio MIME helper accepted an unrelated type")
	}
	if got := baseAudioMIME(" Audio/L16; rate=16000 "); got != "audio/l16" {
		t.Fatalf("baseAudioMIME() = %q", got)
	}
}

func TestRealtimeTextDeltaBranches(t *testing.T) {
	tests := []struct {
		previous string
		current  string
		want     string
	}{
		{previous: "same", current: "same"},
		{previous: "prefix", current: "prefix suffix", want: " suffix"},
		{previous: "Hello!", current: "hello, world", want: ", world"},
		{previous: "Hello world", current: "hello", want: ""},
		{previous: "old", current: "new", want: "new"},
		{previous: "!!!", current: "new", want: "new"},
	}
	for _, test := range tests {
		if got := realtimeTextDelta(test.previous, test.current); got != test.want {
			t.Fatalf("realtimeTextDelta(%q, %q) = %q, want %q", test.previous, test.current, got, test.want)
		}
	}
	if suffix, ok := realtimeTextSuffixAfterNormalizedPrefix("abc", "a-b-c tail"); !ok || suffix != " tail" {
		t.Fatalf("normalized suffix = (%q, %t)", suffix, ok)
	}
	if suffix, ok := realtimeTextSuffixAfterNormalizedPrefix("abc", "abx"); ok || suffix != "" {
		t.Fatalf("mismatched suffix = (%q, %t)", suffix, ok)
	}
	if got := realtimeNormalizeText(" Hello，世界! 123 "); got != "hello世界123" {
		t.Fatalf("realtimeNormalizeText() = %q", got)
	}
}

func TestASTPrepareAudioBlobBranches(t *testing.T) {
	transformer := &Transformer{}
	var decoder *opus.Decoder
	if data, err := transformer.prepareAudioBlob(nil, &decoder); data != nil || err != nil {
		t.Fatalf("prepareAudioBlob(nil) = (%v, %v)", data, err)
	}
	if data, err := transformer.prepareAudioBlob(&genx.Blob{MIMEType: "audio/pcm"}, &decoder); data != nil || err != nil {
		t.Fatalf("prepareAudioBlob(empty) = (%v, %v)", data, err)
	}
	pcmData := []byte{1, 2}
	if data, err := transformer.prepareAudioBlob(&genx.Blob{MIMEType: "audio/L16; rate=16000", Data: pcmData}, &decoder); err != nil || !bytes.Equal(data, pcmData) {
		t.Fatalf("prepareAudioBlob(PCM) = (%v, %v)", data, err)
	}
	tests := []struct {
		name string
		blob *genx.Blob
		want string
	}{
		{name: "unknown", blob: &genx.Blob{MIMEType: "text/plain", Data: []byte("x")}, want: "requires audio"},
		{name: "MP3", blob: &genx.Blob{MIMEType: "audio/mpeg", Data: []byte("bad")}, want: "decode mp3"},
		{name: "Ogg Opus", blob: &genx.Blob{MIMEType: "audio/ogg", Data: []byte("bad")}, want: "decode ogg opus"},
		{name: "raw Opus", blob: &genx.Blob{MIMEType: "audio/opus", Data: []byte{0xff}}, want: "decode raw opus"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := transformer.prepareAudioBlob(test.blob, &decoder); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("prepareAudioBlob() error = %v, want %q", err, test.want)
			}
		})
	}
	if decoder != nil {
		_ = decoder.Close()
	}
	if transformer.audioChunkSize() != 3200 {
		t.Fatalf("audioChunkSize() = %d", transformer.audioChunkSize())
	}
}

func TestASTRouteLifecycleInterruptionBranches(t *testing.T) {
	var nilLifecycle *astTranslateRouteLifecycle
	nilLifecycle.observe(nil)
	fallback := nilLifecycle.interruptedChunks(" ", true)
	if len(fallback) != 4 || !fallback[0].IsBeginOfStream() || !fallback[1].IsEndOfStream() ||
		!fallback[2].IsBeginOfStream() || !fallback[3].IsEndOfStream() {
		t.Fatalf("fallback interruption lifecycle = %#v", fallback)
	}

	lifecycle := newASTTranslateRouteLifecycle()
	lifecycle.observe(nil)
	lifecycle.observe(&genx.MessageChunk{Part: genx.Text("missing ctrl")})
	lifecycle.observe(&genx.MessageChunk{Part: nil, Ctrl: &genx.StreamCtrl{StreamID: "missing part"}})
	lifecycle.observe(&genx.MessageChunk{
		Role: genx.RoleModel, Name: "assistant", Part: genx.Text(""),
		Ctrl: &genx.StreamCtrl{StreamID: "response", Label: doubaoASTTranslateAssistantLabel, BeginOfStream: true},
	})
	lifecycle.observe(&genx.MessageChunk{
		Role: genx.RoleModel, Name: "assistant", Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: "response", Label: doubaoASTTranslateAssistantLabel, BeginOfStream: true, EndOfStream: true},
	})
	chunks := lifecycle.interruptedChunks("fallback", true)
	if len(chunks) != 1 || !chunks[0].IsEndOfStream() || chunks[0].Ctrl.StreamID != "response" || chunks[0].Ctrl.Error != "interrupted" {
		t.Fatalf("tracked interruption chunks = %#v", chunks)
	}
	if astTranslateSegmentStreamID(" ", 1) != "audio" || astTranslateSegmentStreamID(" route ", 2) != "route:ast:2" {
		t.Fatal("astTranslateSegmentStreamID() boundaries are incorrect")
	}
}

func TestASTTextAndAudioStateBranches(t *testing.T) {
	output := &recordingASTTranslateOutput{}
	state := &astTranslateTextState{role: genx.RoleModel, label: "assistant", streamID: "response"}
	if err := state.close(output, "ignored"); err != nil {
		t.Fatalf("inactive close error = %v", err)
	}
	if err := state.addToken(output, " "); err != nil {
		t.Fatalf("blank addToken error = %v", err)
	}
	if err := state.addToken(output, "hello"); err != nil {
		t.Fatalf("first addToken error = %v", err)
	}
	if err := state.addToken(output, "world"); err != nil {
		t.Fatalf("second addToken error = %v", err)
	}
	if state.text != "hello world" {
		t.Fatalf("token text = %q", state.text)
	}
	if err := state.addFinal(output, "hello, world"); err != nil || state.text != "hello, world" {
		t.Fatalf("normalized addFinal = text %q, error %v", state.text, err)
	}
	if err := state.addFinal(output, "hello"); err != nil || state.text != "hello" {
		t.Fatalf("shorter addFinal = text %q, error %v", state.text, err)
	}
	if err := state.addFinal(output, "new"); err != nil || state.text != "hello new" {
		t.Fatalf("replacement addFinal = text %q, error %v", state.text, err)
	}
	if err := state.close(output, "done"); err != nil || state.active || state.text != "" {
		t.Fatalf("active close = active %t text %q error %v", state.active, state.text, err)
	}
	if !astTranslateNeedsSpace("a", "b") || astTranslateNeedsSpace("a ", "b") ||
		!astTranslateASCIIWordByte('9') || astTranslateASCIIWordByte('-') {
		t.Fatal("ASCII spacing helpers returned unexpected values")
	}

	wantErr := errors.New("push failed")
	failing := astTranslateErrorOutput{err: wantErr}
	if err := (&astTranslateTextState{}).addToken(failing, "text"); !errors.Is(err, wantErr) {
		t.Fatalf("addToken push error = %v", err)
	}
	if err := (&astTranslateTextState{}).addFinal(failing, "text"); !errors.Is(err, wantErr) {
		t.Fatalf("addFinal push error = %v", err)
	}

	audioState := &astTranslateAudioState{streamID: "response", mimeType: "audio/opus"}
	if err := audioState.add(output, nil); err != nil || audioState.active {
		t.Fatalf("empty audio add = active %t, error %v", audioState.active, err)
	}
	if err := audioState.close(output, ""); err != nil {
		t.Fatalf("inactive audio close error = %v", err)
	}
	if err := audioState.finishDecoder(""); err != nil {
		t.Fatalf("nil decoder finish error = %v", err)
	}
	audioState.decoder = newASTOggOpusFrameDecoder()
	if _, err := audioState.decoder.Write([]byte("O")); err != nil {
		t.Fatalf("partial decoder Write() error = %v", err)
	}
	if err := audioState.finishDecoder(""); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("truncated decoder finish error = %v", err)
	}
	audioState.decoder = newASTOggOpusFrameDecoder()
	if _, err := audioState.decoder.Write([]byte("O")); err != nil {
		t.Fatalf("second partial decoder Write() error = %v", err)
	}
	if err := audioState.finishDecoder("provider failed"); err != nil {
		t.Fatalf("provider-error decoder finish error = %v", err)
	}
}

type astTranslateErrorOutput struct{ err error }

func (o astTranslateErrorOutput) Push(*genx.MessageChunk) error { return o.err }
