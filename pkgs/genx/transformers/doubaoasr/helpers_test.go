package doubaoasr

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestDoubaoASRSessionConfigAndAudioTimingBranches(t *testing.T) {
	custom := doubaoASRSessionConfig{sampleRate: 8000, channels: 2, bits: 8}
	pcmConfig := custom.withPCM()
	if pcmConfig.format != "pcm" || pcmConfig.sampleRate != 8000 || pcmConfig.channels != 2 || pcmConfig.bits != 8 {
		t.Fatalf("withPCM(custom) = %#v", pcmConfig)
	}
	wavConfig := custom.withWAV()
	if wavConfig.format != "wav" || wavConfig.sampleRate != 8000 || wavConfig.channels != 2 || wavConfig.bits != 8 {
		t.Fatalf("withWAV(custom) = %#v", wavConfig)
	}
	defaults := (doubaoASRSessionConfig{}).withPCM()
	if defaults.sampleRate != 16000 || defaults.channels != 1 || defaults.bits != 16 || !defaults.isPCM() {
		t.Fatalf("withPCM(defaults) = %#v", defaults)
	}
	if !(doubaoASRSessionConfig{format: " PCM_S16LE "}).isPCM() || (doubaoASRSessionConfig{format: "wav"}).isPCM() {
		t.Fatal("isPCM() format normalization failed")
	}

	transformer := &Transformer{chunkSize: 123}
	if got := transformer.audioChunkSize(defaults); got != 123 {
		t.Fatalf("explicit chunk size = %d", got)
	}
	transformer.chunkSize = 0
	if got := transformer.audioChunkSize(doubaoASRSessionConfig{format: "mp3"}); got != 0 {
		t.Fatalf("compressed chunk size = %d", got)
	}
	if got := transformer.audioChunkSize(doubaoASRSessionConfig{format: "pcm"}); got != 3200 {
		t.Fatalf("default PCM chunk size = %d", got)
	}
	if got := transformer.audioChunkSize(custom.withPCM()); got != 1600 {
		t.Fatalf("custom PCM chunk size = %d", got)
	}
	if got := audioDuration(make([]byte, 3200), doubaoASRSessionConfig{}); got != 100*time.Millisecond {
		t.Fatalf("default audio duration = %v", got)
	}
	if got := audioDuration(make([]byte, 1600), custom); got != 100*time.Millisecond {
		t.Fatalf("custom audio duration = %v", got)
	}
}

func TestDoubaoASRRouteStateNilAndDuplicateBranches(t *testing.T) {
	var nilState *doubaoASRRouteState
	nilState.set("stream")
	nilState.markTranscriptStarted()
	if streamID, generation := nilState.current(); streamID != "" || generation != 0 || nilState.transcriptStarted() {
		t.Fatalf("nil route state = (%q, %d, %t)", streamID, generation, nilState.transcriptStarted())
	}

	state := newDoubaoASRRouteState(nil)
	state.set(" ")
	state.set(" stream ")
	state.set("stream")
	if streamID, generation := state.current(); streamID != "stream" || generation != 1 {
		t.Fatalf("route state = (%q, %d)", streamID, generation)
	}
	if state.transcriptStarted() {
		t.Fatal("transcript unexpectedly started")
	}
	state.markTranscriptStarted()
	if !state.transcriptStarted() {
		t.Fatal("transcript start was not recorded")
	}
	fromChunk := newDoubaoASRRouteState(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "chunk"}})
	if streamID, _ := fromChunk.current(); streamID != "chunk" {
		t.Fatalf("chunk route state = %q", streamID)
	}
}

func TestDoubaoASRHistoryAudioBufferNilAndFormatBranches(t *testing.T) {
	var nilBuffer *doubaoASRHistoryAudioBuffer
	nilBuffer.appendChunk(nil, "", []byte{1}, doubaoASRSessionConfig{})
	nilBuffer.emitSegment(nil, "", 0, 1)
	if nilBuffer.hasOpusAudio() || nilBuffer.opusSegment("", 0, 1) != nil {
		t.Fatal("nil history buffer returned Opus audio")
	}
	if data, mimeType := nilBuffer.segment(0, 1); data != nil || mimeType != "" {
		t.Fatalf("nil segment = (%v, %q)", data, mimeType)
	}
	if nilBuffer.bytesPerSecond() != 0 || nilBuffer.frameBytes() != 0 || nilBuffer.bytesPerSample() != 0 || nilBuffer.mimeType() != "audio/pcm" {
		t.Fatal("nil history buffer helpers returned non-default data")
	}

	buffer := &doubaoASRHistoryAudioBuffer{cfg: doubaoASRSessionConfig{format: "pcm"}}
	if buffer.bytesPerSample() != 2 || buffer.mimeType() != "audio/L16; rate=16000; channels=1" {
		t.Fatalf("history defaults = bytes %d, MIME %q", buffer.bytesPerSample(), buffer.mimeType())
	}
	buffer.appendChunk(nil, "", nil, doubaoASRSessionConfig{format: "pcm"})
	buffer.appendChunk(nil, "", []byte{1}, doubaoASRSessionConfig{format: "mp3"})
	if data, mimeType := buffer.segment(2, 1); data != nil || mimeType != "" {
		t.Fatalf("invalid range segment = (%v, %q)", data, mimeType)
	}
	if alignPCMOffset(5, 0) != 5 || alignPCMOffset(5, 2) != 4 {
		t.Fatal("alignPCMOffset() boundaries are incorrect")
	}
}

func TestDoubaoASRMIMEAndSegmentIDBranches(t *testing.T) {
	if asrSegmentStreamID(" ", 1) != "audio" || asrSegmentStreamID(" stream ", 2) != "stream:asr:2" {
		t.Fatal("asrSegmentStreamID() boundaries are incorrect")
	}
	if !isAudioMIME(" Audio/PCM ; rate=16000") || isAudioMIME("text/plain") ||
		!isOggAudioMIME("application/ogg") || !isASRMP3MIME("audio/x-mp3") ||
		!isASRPCMMIME("audio/x-pcm") || !isASRWAVMIME("audio/x-wav") || !isASROpusMIME("audio/opus") {
		t.Fatal("ASR MIME aliases were not recognized")
	}
	if baseAudioMIME(" Audio/Opus ; rate=16000 ") != "audio/opus" || baseAudioMIME("audio/pcm") != "audio/pcm" {
		t.Fatal("baseAudioMIME() normalization failed")
	}
}
