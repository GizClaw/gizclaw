package agenthost

import (
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestAudioOutputTracksKeyByStreamAndCanonicalMIME(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	chunks := []*genx.MessageChunk{
		pcmOutputChunk("stream-a", "audio/L16; rate=16000; channels=1", []byte{1, 0}, false, ""),
		pcmOutputChunk("stream-a", "AUDIO/L16; channels=1; rate=16000", []byte{2, 0}, false, ""),
		pcmOutputChunk("stream-a", "audio/L16; rate=24000; channels=1", []byte{3, 0}, false, ""),
		pcmOutputChunk("stream-b", "audio/L16; rate=16000; channels=1", []byte{4, 0}, false, ""),
	}
	for _, chunk := range chunks {
		if err := tracks.consume(chunk); err != nil {
			t.Fatalf("consume(%#v) error = %v", chunk.Ctrl, err)
		}
	}
	if got := len(tracks.channels); got != 3 {
		t.Fatalf("active channels = %d, want 3", got)
	}
	if got := len(creator.tracks); got != 3 {
		t.Fatalf("created tracks = %d, want 3", got)
	}
}

func TestAudioOutputTracksMIMEEOSClosesOnlyMatchingTrack(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	mime16 := "audio/L16; rate=16000; channels=1"
	mime24 := "audio/L16; rate=24000; channels=1"
	for _, chunk := range []*genx.MessageChunk{
		pcmOutputChunk("stream-a", mime16, []byte{1, 0}, false, ""),
		pcmOutputChunk("stream-a", mime24, []byte{2, 0}, false, ""),
		pcmOutputChunk("stream-b", mime16, []byte{3, 0}, false, ""),
		pcmOutputChunk("stream-a", mime16, []byte{4, 0}, true, ""),
	} {
		if err := tracks.consume(chunk); err != nil {
			t.Fatalf("consume() error = %v", err)
		}
	}
	if got := len(tracks.channels); got != 2 {
		t.Fatalf("active channels after MIME EOS = %d, want 2", got)
	}
	if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{5, 0})); err == nil || !strings.Contains(err.Error(), "CloseWrite") {
		t.Fatalf("write after normal EOS error = %v, want CloseWrite", err)
	}
	if err := creator.tracks[1].Write(pcm.L16Mono24K.DataChunk([]byte{6, 0})); err != nil {
		t.Fatalf("unrelated MIME track write error = %v", err)
	}
	if err := creator.tracks[2].Write(pcm.L16Mono16K.DataChunk([]byte{7, 0})); err != nil {
		t.Fatalf("unrelated stream track write error = %v", err)
	}
}

func TestAudioOutputTracksErrorEOSAndRouteEOS(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	mime16 := "audio/L16; rate=16000; channels=1"
	mime24 := "audio/L16; rate=24000; channels=1"
	for _, chunk := range []*genx.MessageChunk{
		pcmOutputChunk("stream-a", mime16, []byte{1, 0}, false, ""),
		pcmOutputChunk("stream-a", mime24, []byte{2, 0}, false, ""),
		pcmOutputChunk("stream-b", mime16, []byte{3, 0}, false, ""),
		pcmOutputChunk("stream-a", mime16, nil, true, "interrupted"),
	} {
		if err := tracks.consume(chunk); err != nil {
			t.Fatalf("consume() error = %v", err)
		}
	}
	if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{4, 0})); err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("write after Error EOS error = %v, want interrupted", err)
	}
	if err := tracks.consume(&genx.MessageChunk{Ctrl: &genx.StreamCtrl{StreamID: "stream-a", EndOfStream: true}}); err != nil {
		t.Fatalf("control EOS error = %v", err)
	}
	if got := len(tracks.channels); got != 1 {
		t.Fatalf("active channels after route EOS = %d, want 1", got)
	}
	if err := creator.tracks[1].Write(pcm.L16Mono24K.DataChunk([]byte{5, 0})); err == nil || !strings.Contains(err.Error(), "CloseWrite") {
		t.Fatalf("route track write error = %v, want CloseWrite", err)
	}
	if err := creator.tracks[2].Write(pcm.L16Mono16K.DataChunk([]byte{6, 0})); err != nil {
		t.Fatalf("other route track write error = %v", err)
	}
}

func TestAudioOutputTracksRejectInvalidPCMWithContext(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	err := tracks.consume(pcmOutputChunk("answer", "audio/L16; rate=44100; channels=1", []byte{1, 0}, false, ""))
	if err == nil || !strings.Contains(err.Error(), `stream_id="answer"`) || !strings.Contains(err.Error(), "44100") {
		t.Fatalf("consume invalid PCM error = %v", err)
	}
	if len(tracks.channels) != 0 {
		t.Fatalf("active channels after invalid PCM = %d, want 0", len(tracks.channels))
	}
}

func TestAudioOutputTracksRejectMalformedAudioMIMEWithContext(t *testing.T) {
	creator := newRecordingAudioTrackCreator()
	tracks := newAudioOutputTracks(creator)
	err := tracks.consume(pcmOutputChunk("answer", "audio/L16; rate", []byte{1, 0}, false, ""))
	if err == nil || !strings.Contains(err.Error(), `stream_id="answer"`) || !strings.Contains(err.Error(), `mime="audio/L16; rate"`) {
		t.Fatalf("consume malformed MIME error = %v", err)
	}
	if len(tracks.channels) != 0 {
		t.Fatalf("active channels after malformed MIME = %d, want 0", len(tracks.channels))
	}
}

func pcmOutputChunk(streamID, mimeType string, data []byte, eos bool, errorText string) *genx.MessageChunk {
	return &genx.MessageChunk{
		Part: &genx.Blob{MIMEType: mimeType, Data: data},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, EndOfStream: eos, Error: errorText},
	}
}

type recordingAudioTrackCreator struct {
	mixer  *pcm.Mixer
	tracks []pcm.Track
}

func newRecordingAudioTrackCreator() *recordingAudioTrackCreator {
	return &recordingAudioTrackCreator{mixer: pcm.NewMixer(pcm.L16Mono16K)}
}

func (c *recordingAudioTrackCreator) CreateAudioTrack(opts ...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	track, ctrl, err := c.mixer.CreateTrack(opts...)
	if err == nil {
		c.tracks = append(c.tracks, track)
	}
	return track, ctrl, err
}

func TestMixerOutputOuterCloseModes(t *testing.T) {
	t.Run("normal completion closes writes", func(t *testing.T) {
		creator := newRecordingAudioTrackCreator()
		output := &sliceStream{
			chunks:  []*genx.MessageChunk{pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, false, "")},
			doneErr: genx.ErrDone,
		}
		if err := (MixerOutput{Tracks: creator}).ConsumeAgentOutput(t.Context(), output); err != nil {
			t.Fatalf("ConsumeAgentOutput() error = %v", err)
		}
		if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{2, 0})); err == nil || !strings.Contains(err.Error(), "CloseWrite") {
			t.Fatalf("write after completion error = %v, want CloseWrite", err)
		}
	})

	t.Run("outer error closes with error", func(t *testing.T) {
		creator := newRecordingAudioTrackCreator()
		wantErr := errors.New("provider failed")
		output := &sliceStream{
			chunks:  []*genx.MessageChunk{pcmOutputChunk("answer", "audio/pcm", []byte{1, 0}, false, "")},
			doneErr: wantErr,
		}
		err := (MixerOutput{Tracks: creator}).ConsumeAgentOutput(t.Context(), output)
		if !errors.Is(err, wantErr) {
			t.Fatalf("ConsumeAgentOutput() error = %v, want %v", err, wantErr)
		}
		if err := creator.tracks[0].Write(pcm.L16Mono16K.DataChunk([]byte{2, 0})); !errors.Is(err, wantErr) {
			t.Fatalf("write after outer error = %v, want %v", err, wantErr)
		}
	})
}
