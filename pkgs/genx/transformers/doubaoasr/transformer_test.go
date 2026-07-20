package doubaoasr

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"iter"
	"slices"
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

type fakeDoubaoASRSend struct {
	data   []byte
	isLast bool
}

type fakeDoubaoASRSession struct {
	sends     []fakeDoubaoASRSend
	result    chan *doubaospeech.ASRV2Result
	sendAudio func(context.Context, []byte, bool) error
}

type fakeDoubaoASROpen struct {
	cfg     doubaoASRSessionConfig
	session *fakeDoubaoASRSession
}

func TestConfigValidationDefaultsAndCopies(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("New() accepted a nil client")
	}
	client := doubaospeech.NewClient("app")
	hotwords := []string{"first"}
	disabled := false
	transformer, err := New(Config{
		Client:            client,
		EnableITN:         &disabled,
		EnablePunctuation: &disabled,
		Hotwords:          hotwords,
		RealtimePacing:    &disabled,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	hotwords[0] = "changed"
	if transformer.format != "pcm" || transformer.sampleRate != 16000 || transformer.channels != 1 || transformer.bits != 16 || transformer.language != "zh-CN" {
		t.Fatalf("audio defaults = %#v", transformer)
	}
	if transformer.enableITN || transformer.enablePunc || transformer.realtimePacing {
		t.Fatalf("explicit false values were replaced by defaults: %#v", transformer)
	}
	if len(transformer.hotwords) != 1 || transformer.hotwords[0] != "first" {
		t.Fatalf("hotwords were not defensively copied: %#v", transformer.hotwords)
	}
	if transformer.resultType != "single" || transformer.resourceID != doubaospeech.ResourceASRStream {
		t.Fatalf("provider defaults = %#v", transformer)
	}
	if transformer.chunkSize != 0 || transformer.emitInterim {
		t.Fatalf("stream defaults = %#v", transformer)
	}

	enabled := true
	configured, err := New(Config{
		Client:            client,
		Format:            "wav",
		SampleRate:        24000,
		Channels:          2,
		Bits:              24,
		Language:          "en-US",
		EnableITN:         &enabled,
		EnablePunctuation: &enabled,
		Hotwords:          []string{"hello"},
		ResultType:        "full",
		EmitInterim:       true,
		ResourceID:        "resource",
		ChunkSize:         2048,
		RealtimePacing:    &enabled,
	})
	if err != nil {
		t.Fatalf("New(custom) error = %v", err)
	}
	if configured.format != "wav" || configured.sampleRate != 24000 || configured.channels != 2 || configured.bits != 24 || configured.language != "en-US" || !configured.enableITN || !configured.enablePunc || configured.resultType != "full" || !configured.emitInterim || configured.resourceID != "resource" || configured.chunkSize != 2048 || !configured.realtimePacing {
		t.Fatalf("custom config = %#v", configured)
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func collectTransformerChunks(t *testing.T, stream genx.Stream) []*genx.MessageChunk {
	t.Helper()
	var chunks []*genx.MessageChunk
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
			return chunks
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		chunks = append(chunks, chunk)
	}
}

func newFakeDoubaoASRSession() *fakeDoubaoASRSession {
	return &fakeDoubaoASRSession{result: make(chan *doubaospeech.ASRV2Result, 1)}
}

func (s *fakeDoubaoASRSession) SendAudio(_ context.Context, data []byte, isLast bool) error {
	if s.sendAudio != nil {
		return s.sendAudio(context.Background(), data, isLast)
	}
	s.sends = append(s.sends, fakeDoubaoASRSend{data: slices.Clone(data), isLast: isLast})
	if isLast {
		s.result <- &doubaospeech.ASRV2Result{
			Text:    "recognized text",
			IsFinal: true,
			Utterances: []doubaospeech.ASRV2Utterance{
				{Text: "recognized text", EndTime: 100},
			},
		}
		close(s.result)
	}
	return nil
}

func (s *fakeDoubaoASRSession) Recv() iter.Seq2[*doubaospeech.ASRV2Result, error] {
	return func(yield func(*doubaospeech.ASRV2Result, error) bool) {
		for result := range s.result {
			if !yield(result, nil) {
				return
			}
		}
	}
}

func (s *fakeDoubaoASRSession) Close() error {
	return nil
}

func TestTransformerSendsLastNonEmptyAudioFrame(t *testing.T) {
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{Format: "ogg_opus"})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		return session, nil
	}

	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte("first")}}); err != nil {
		t.Fatalf("push first audio = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg", Data: []byte("second")}}); err != nil {
		t.Fatalf("push second audio = %v", err)
	}
	if err := input.Push(genx.NewEndOfStream("audio/ogg")); err != nil {
		t.Fatalf("push eos = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	chunk := nextNonHistoryChunk(t, output)
	if got := chunk.Part.(genx.Text); got != "recognized text" {
		t.Fatalf("output text = %q, want recognized text", got)
	}
	chunk = nextNonHistoryChunk(t, output)
	if chunk == nil || !chunk.IsEndOfStream() {
		t.Fatalf("output eos chunk = %#v", chunk)
	}

	if len(session.sends) != 2 {
		t.Fatalf("SendAudio calls = %#v, want two non-empty frames", session.sends)
	}
	if got := string(session.sends[0].data); got != "first" || session.sends[0].isLast {
		t.Fatalf("first SendAudio = data %q last %t, want first/false", got, session.sends[0].isLast)
	}
	if got := string(session.sends[1].data); got != "second" || !session.sends[1].isLast {
		t.Fatalf("second SendAudio = data %q last %t, want second/true", got, session.sends[1].isLast)
	}
}

func TestTransformerUsesWAVFormatForWAVInput(t *testing.T) {
	session := newFakeDoubaoASRSession()
	var openCfg doubaoASRSessionConfig
	transformer := newTransformer(Config{Format: "ogg_opus"})
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		openCfg = cfg
		return session, nil
	}

	input := newBufferStream(2)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	wav := []byte("RIFF----WAVEfmt data")
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/wav", Data: wav}}); err != nil {
		t.Fatalf("push wav audio = %v", err)
	}
	if err := input.Push(genx.NewEndOfStream("audio/wav")); err != nil {
		t.Fatalf("push eos = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}
	_ = nextNonHistoryChunk(t, output)
	_ = nextNonHistoryChunk(t, output)

	if openCfg.format != "wav" {
		t.Fatalf("session format = %q, want wav", openCfg.format)
	}
	if openCfg.sampleRate != 16000 || openCfg.channels != 1 || openCfg.bits != 16 {
		t.Fatalf("session audio config = rate %d channels %d bits %d, want 16000/1/16", openCfg.sampleRate, openCfg.channels, openCfg.bits)
	}
	if len(session.sends) != 1 {
		t.Fatalf("SendAudio calls = %#v, want one", session.sends)
	}
	if !bytes.Equal(session.sends[0].data, wav) || !session.sends[0].isLast {
		t.Fatalf("SendAudio = data %#v last %t, want original wav/true", session.sends[0].data, session.sends[0].isLast)
	}
}

func TestTransformerPushToTalkKeepsHistoryStreamIDAcrossEOS(t *testing.T) {
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{Format: "pcm", RealtimePacing: boolPointer(false)})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		return session, nil
	}

	input := newBufferStream(3)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 0, 2, 0}},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
	}); err != nil {
		t.Fatalf("push audio = %v", err)
	}
	if err := input.Push(genx.NewEndOfStream("audio/pcm")); err != nil {
		t.Fatalf("push eos = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	chunks := collectTransformerChunks(t, output)
	history := historyAudioChunks(chunks)
	if len(history) != 2 {
		t.Fatalf("history chunks = %d, want audio and eos: %#v", len(history), history)
	}
	for i, chunk := range history {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "turn-1" {
			t.Fatalf("history[%d] ctrl = %#v, want stream turn-1", i, chunk.Ctrl)
		}
	}
	nonHistory := nonHistoryChunks(chunks)
	if len(nonHistory) != 2 {
		t.Fatalf("non-history chunks = %d, want text and eos: %#v", len(nonHistory), nonHistory)
	}
	for i, chunk := range nonHistory {
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != "turn-1" || chunk.Ctrl.Label != "transcript" {
			t.Fatalf("non-history[%d] ctrl = %#v, want transcript stream turn-1", i, chunk.Ctrl)
		}
	}
}

func TestTransformerDecodesOggToPCMWhenConfiguredPCM(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}

	const sampleRate = 16000
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{Format: "pcm", SampleRate: sampleRate, Channels: 1, Bits: 16})
	var opens []fakeDoubaoASROpen
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		opens = append(opens, fakeDoubaoASROpen{cfg: cfg, session: session})
		return session, nil
	}

	inputAudio := buildASROGGOpusStream(t, sampleRate, 1, buildASRAudioFrame(sampleRate/50, 1))
	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg", Data: inputAudio}}); err != nil {
		t.Fatalf("push ogg audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	_ = nextNonHistoryChunk(t, output)
	expectTranscriptEOS(t, output)
	expectNoMoreNonHistoryChunks(t, output)

	if len(session.sends) != 1 {
		t.Fatalf("SendAudio calls = %#v, want one final pcm frame", session.sends)
	}
	if len(opens) != 1 || !opens[0].cfg.isPCM() {
		t.Fatalf("open session config = %#v, want pcm", opens)
	}
	got := session.sends[0].data
	if !session.sends[0].isLast {
		t.Fatalf("SendAudio final flag = false, want true")
	}
	if len(got) == 0 {
		t.Fatal("decoded pcm is empty")
	}
	if bytes.Equal(got, inputAudio) {
		t.Fatal("SendAudio received raw ogg data")
	}
	if bytes.HasPrefix(got, []byte("OggS")) {
		t.Fatal("SendAudio received ogg page bytes")
	}
}

func TestTransformerDecodesRawOpusToPCMSession(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}

	const (
		sampleRate = 16000
		channels   = 1
	)
	enc, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationVoIP)
	if err != nil {
		t.Fatalf("NewEncoder() error = %v", err)
	}
	defer func() { _ = enc.Close() }()
	packet, err := enc.Encode(buildASRAudioFrame(sampleRate/50, channels), sampleRate/50)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{Format: "pcm", SampleRate: sampleRate, Channels: channels, Bits: 16})
	var opens []fakeDoubaoASROpen
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		opens = append(opens, fakeDoubaoASROpen{cfg: cfg, session: session})
		return session, nil
	}

	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}}); err != nil {
		t.Fatalf("push raw opus audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	_ = nextNonHistoryChunk(t, output)
	expectTranscriptEOS(t, output)
	expectNoMoreNonHistoryChunks(t, output)

	if len(opens) != 1 || !opens[0].cfg.isPCM() {
		t.Fatalf("open session config = %#v, want pcm", opens)
	}
	if len(session.sends) != 1 {
		t.Fatalf("SendAudio calls = %#v, want one final pcm frame", session.sends)
	}
	got := session.sends[0].data
	if !session.sends[0].isLast {
		t.Fatal("SendAudio final flag = false, want true")
	}
	if len(got) == 0 {
		t.Fatal("decoded pcm is empty")
	}
	if bytes.Equal(got, packet) {
		t.Fatal("SendAudio received raw opus packet")
	}
}

func TestTransformerDecodesMP3ToPCMSession(t *testing.T) {
	const sampleRate = 16000

	inputAudio := buildASRMP3Stream(t, sampleRate, 1, buildASRAudioFrame(sampleRate/2, 1))
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{SampleRate: sampleRate, Channels: 1, Bits: 16})
	var opens []fakeDoubaoASROpen
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		opens = append(opens, fakeDoubaoASROpen{cfg: cfg, session: session})
		return session, nil
	}

	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/mpeg", Data: inputAudio}}); err != nil {
		t.Fatalf("push mp3 audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	_ = nextNonHistoryChunk(t, output)
	expectTranscriptEOS(t, output)
	expectNoMoreNonHistoryChunks(t, output)

	if len(opens) != 1 {
		t.Fatalf("open sessions = %#v, want one", opens)
	}
	if !opens[0].cfg.isPCM() || opens[0].cfg.sampleRate != sampleRate || opens[0].cfg.channels != 1 || opens[0].cfg.bits != 16 {
		t.Fatalf("open session config = %#v, want 16k mono pcm16", opens[0].cfg)
	}
	if len(session.sends) == 0 {
		t.Fatal("expected decoded pcm audio sends")
	}
	got := session.sends[len(session.sends)-1].data
	if len(got) == 0 {
		t.Fatal("decoded pcm is empty")
	}
	if bytes.Equal(got, inputAudio) || bytes.HasPrefix(got, []byte("ID3")) || bytes.HasPrefix(got, []byte{0xff, 0xf3}) {
		t.Fatal("SendAudio received mp3 data")
	}
}

func TestTransformerEmitsDefiniteUtterancesWithNonMonotonicTimes(t *testing.T) {
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{Format: "pcm", RealtimePacing: boolPointer(false)})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		return session, nil
	}
	session.sendAudio = func(_ context.Context, _ []byte, isLast bool) error {
		if isLast {
			session.result <- &doubaospeech.ASRV2Result{
				Utterances: []doubaospeech.ASRV2Utterance{
					{Text: "first", StartTime: 100, EndTime: 200, Definite: true},
				},
			}
			session.result <- &doubaospeech.ASRV2Result{
				Utterances: []doubaospeech.ASRV2Utterance{
					{Text: "second", StartTime: 0, EndTime: 100, Definite: true},
				},
			}
			close(session.result)
		}
		return nil
	}

	input := newBufferStream(2)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}}); err != nil {
		t.Fatalf("push audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	chunk := nextNonHistoryChunk(t, output)
	if got := chunk.Part.(genx.Text); got != "first" {
		t.Fatalf("first output = %q", got)
	}
	chunk = nextNonHistoryChunk(t, output)
	if got := chunk.Part.(genx.Text); got != "second" {
		t.Fatalf("second output = %q", got)
	}
}

func TestTransformerEmitInterimControlsNonDefiniteUtterances(t *testing.T) {
	tests := []struct {
		name        string
		emitInterim bool
		want        []struct {
			text  string
			label string
			bos   bool
			eos   bool
		}
	}{
		{
			name:        "enabled",
			emitInterim: true,
			want: []struct {
				text  string
				label string
				bos   bool
				eos   bool
			}{
				{label: "transcript", bos: true},
				{text: "partial text", label: "transcript"},
				{text: "final text", label: "transcript"},
				{label: "transcript", eos: true},
			},
		},
		{
			name: "disabled",
			want: []struct {
				text  string
				label string
				bos   bool
				eos   bool
			}{
				{text: "final text", label: "transcript"},
				{label: "transcript", eos: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := newFakeDoubaoASRSession()
			transformer := newTransformer(Config{
				Format:         "pcm",
				RealtimePacing: boolPointer(false),
				EmitInterim:    tt.emitInterim,
			})
			transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
				return session, nil
			}
			session.sendAudio = func(_ context.Context, _ []byte, isLast bool) error {
				if isLast {
					session.result <- &doubaospeech.ASRV2Result{
						Text: "partial text",
						Utterances: []doubaospeech.ASRV2Utterance{
							{Text: "partial text", StartTime: 0, EndTime: 100, Definite: false},
						},
					}
					session.result <- &doubaospeech.ASRV2Result{
						Utterances: []doubaospeech.ASRV2Utterance{
							{Text: "final text", StartTime: 0, EndTime: 200, Definite: true},
						},
					}
					close(session.result)
				}
				return nil
			}

			input := newBufferStream(2)
			output, err := transformer.Transform(context.Background(), input)
			if err != nil {
				t.Fatalf("Transform() error = %v", err)
			}
			if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}}); err != nil {
				t.Fatalf("push audio = %v", err)
			}
			if err := input.Close(); err != nil {
				t.Fatalf("close input = %v", err)
			}

			for i, want := range tt.want {
				chunk := nextNonHistoryChunk(t, output)
				if chunk.IsBeginOfStream() != want.bos {
					t.Fatalf("output[%d] BOS = %t, want %t", i, chunk.IsBeginOfStream(), want.bos)
				}
				if chunk.IsEndOfStream() != want.eos {
					t.Fatalf("output[%d] EOS = %t, want %t", i, chunk.IsEndOfStream(), want.eos)
				}
				if want.text != "" {
					if got := chunk.Part.(genx.Text); string(got) != want.text {
						t.Fatalf("output[%d] text = %q, want %q", i, got, want.text)
					}
				}
				gotLabel := ""
				if chunk.Ctrl != nil {
					gotLabel = chunk.Ctrl.Label
				}
				if gotLabel != want.label {
					t.Fatalf("output[%d] label = %q, want %q", i, gotLabel, want.label)
				}
			}
		})
	}
}

func TestTransformerEmitInterimSplitsDefiniteUtteranceStreamIDs(t *testing.T) {
	const sampleRate = 16000
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{
		Format:         "pcm",
		SampleRate:     sampleRate,
		RealtimePacing: boolPointer(false),
		EmitInterim:    true,
	})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		return session, nil
	}
	session.sendAudio = func(_ context.Context, _ []byte, isLast bool) error {
		if isLast {
			session.result <- &doubaospeech.ASRV2Result{
				Utterances: []doubaospeech.ASRV2Utterance{
					{Text: "first text", StartTime: 0, EndTime: 100, Definite: true},
				},
			}
			session.result <- &doubaospeech.ASRV2Result{
				Utterances: []doubaospeech.ASRV2Utterance{
					{Text: "second text", StartTime: 200, EndTime: 300, Definite: true},
				},
			}
			close(session.result)
		}
		return nil
	}

	input := newBufferStream(2)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{
		Part: &genx.Blob{MIMEType: "audio/pcm", Data: pcm16LE(buildASRAudioFrame(sampleRate*3/10, 1))},
		Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
	}); err != nil {
		t.Fatalf("push audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	chunks := collectTransformerChunks(t, output)
	nonHistory := nonHistoryChunks(chunks)
	want := []struct {
		text     string
		streamID string
		bos      bool
		eos      bool
	}{
		{streamID: "turn-1", bos: true},
		{text: "first text", streamID: "turn-1"},
		{streamID: "turn-1", eos: true},
		{streamID: "turn-1:asr:2", bos: true},
		{text: "second text", streamID: "turn-1:asr:2"},
		{streamID: "turn-1:asr:2", eos: true},
	}
	if len(nonHistory) != len(want) {
		t.Fatalf("non-history chunks = %d, want %d: %#v", len(nonHistory), len(want), nonHistory)
	}
	for i, wantChunk := range want {
		chunk := nonHistory[i]
		if chunk.IsBeginOfStream() != wantChunk.bos {
			t.Fatalf("output[%d] BOS = %t, want %t", i, chunk.IsBeginOfStream(), wantChunk.bos)
		}
		if chunk.IsEndOfStream() != wantChunk.eos {
			t.Fatalf("output[%d] EOS = %t, want %t", i, chunk.IsEndOfStream(), wantChunk.eos)
		}
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != wantChunk.streamID {
			t.Fatalf("output[%d] stream id = %#v, want %q", i, chunk.Ctrl, wantChunk.streamID)
		}
		if wantChunk.text != "" {
			if got := chunk.Part.(genx.Text); string(got) != wantChunk.text {
				t.Fatalf("output[%d] text = %q, want %q", i, got, wantChunk.text)
			}
		}
	}

	history := historyAudioChunks(chunks)
	if len(history) != 4 {
		t.Fatalf("history audio chunks = %d, want 4: %#v", len(history), history)
	}
	wantHistory := []struct {
		streamID string
		dataLen  int
		eos      bool
	}{
		{streamID: "turn-1", dataLen: sampleRate / 10 * 2},
		{streamID: "turn-1", eos: true},
		{streamID: "turn-1:asr:2", dataLen: sampleRate / 10 * 2},
		{streamID: "turn-1:asr:2", eos: true},
	}
	for i, wantChunk := range wantHistory {
		chunk := history[i]
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != wantChunk.streamID || chunk.Ctrl.Label != genx.HistoryUserAudioLabel {
			t.Fatalf("history[%d] ctrl = %#v, want stream %q history label", i, chunk.Ctrl, wantChunk.streamID)
		}
		if chunk.IsEndOfStream() != wantChunk.eos {
			t.Fatalf("history[%d] eos = %t, want %t", i, chunk.IsEndOfStream(), wantChunk.eos)
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok {
			t.Fatalf("history[%d] part = %#v, want blob", i, chunk.Part)
		}
		if wantChunk.dataLen > 0 && len(blob.Data) != wantChunk.dataLen {
			t.Fatalf("history[%d] data len = %d, want %d", i, len(blob.Data), wantChunk.dataLen)
		}
	}
}

func TestTransformerEmitInterimUsesTimestampedOpusBlocksForHistory(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}
	const sampleRate = 16000
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{
		Format:         "ogg_opus",
		SampleRate:     sampleRate,
		RealtimePacing: boolPointer(false),
		EmitInterim:    true,
	})
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		if cfg.isPCM() {
			t.Fatalf("open session cfg = %#v, want compressed provider upload", cfg)
		}
		return session, nil
	}
	session.sendAudio = func(_ context.Context, data []byte, isLast bool) error {
		session.sends = append(session.sends, fakeDoubaoASRSend{data: slices.Clone(data), isLast: isLast})
		if isLast {
			session.result <- &doubaospeech.ASRV2Result{
				Utterances: []doubaospeech.ASRV2Utterance{
					{Text: "first text", StartTime: 0, EndTime: 20, Definite: true},
				},
			}
			session.result <- &doubaospeech.ASRV2Result{
				Utterances: []doubaospeech.ASRV2Utterance{
					{Text: "second text", StartTime: 20, EndTime: 40, Definite: true},
				},
			}
			close(session.result)
		}
		return nil
	}

	firstPacket := buildASRRawOpusPacket(t, sampleRate, 1, buildASRAudioFrame(sampleRate/50, 1))
	secondPacket := buildASRRawOpusPacket(t, sampleRate, 1, buildASRAudioFrame(sampleRate/50, 1))
	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	for i, packet := range [][]byte{firstPacket, secondPacket} {
		if err := input.Push(&genx.MessageChunk{
			Part: &genx.Blob{MIMEType: "audio/opus", Data: packet},
			Ctrl: &genx.StreamCtrl{StreamID: "turn-1", Timestamp: 10_000 + int64(i*20)},
		}); err != nil {
			t.Fatalf("push opus audio = %v", err)
		}
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	chunks := collectTransformerChunks(t, output)
	if len(session.sends) != 2 {
		t.Fatalf("SendAudio calls = %#v, want one send per opus packet", session.sends)
	}
	if !bytes.Equal(session.sends[0].data, firstPacket) || !bytes.Equal(session.sends[1].data, secondPacket) {
		t.Fatalf("provider sends = %#v, want original opus packets", session.sends)
	}

	history := historyAudioChunks(chunks)
	want := []struct {
		streamID string
		data     []byte
		eos      bool
	}{
		{streamID: "turn-1", data: firstPacket},
		{streamID: "turn-1", eos: true},
		{streamID: "turn-1:asr:2", data: secondPacket},
		{streamID: "turn-1:asr:2", eos: true},
	}
	if len(history) != len(want) {
		t.Fatalf("history audio chunks = %d, want %d: %#v", len(history), len(want), history)
	}
	for i, wantChunk := range want {
		chunk := history[i]
		if chunk.Ctrl == nil || chunk.Ctrl.StreamID != wantChunk.streamID || chunk.Ctrl.Label != genx.HistoryUserAudioLabel {
			t.Fatalf("history[%d] ctrl = %#v, want stream %q history label", i, chunk.Ctrl, wantChunk.streamID)
		}
		if chunk.IsEndOfStream() != wantChunk.eos {
			t.Fatalf("history[%d] eos = %t, want %t", i, chunk.IsEndOfStream(), wantChunk.eos)
		}
		blob, ok := chunk.Part.(*genx.Blob)
		if !ok || blob.MIMEType != "audio/opus" {
			t.Fatalf("history[%d] part = %#v, want audio/opus blob", i, chunk.Part)
		}
		if wantChunk.data != nil && !bytes.Equal(blob.Data, wantChunk.data) {
			t.Fatalf("history[%d] data = %v, want %v", i, blob.Data, wantChunk.data)
		}
	}
}

func TestTransformerEmitInterimDoesNotDuplicateFinalTextResult(t *testing.T) {
	session := newFakeDoubaoASRSession()
	transformer := newTransformer(Config{
		Format:         "pcm",
		RealtimePacing: boolPointer(false),
		EmitInterim:    true,
	})
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		return session, nil
	}
	session.sendAudio = func(_ context.Context, _ []byte, isLast bool) error {
		if isLast {
			session.result <- &doubaospeech.ASRV2Result{Text: "final text", IsFinal: true}
			close(session.result)
		}
		return nil
	}

	input := newBufferStream(2)
	output, err := transformer.Transform(context.Background(), input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}}); err != nil {
		t.Fatalf("push audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	chunks := collectTransformerChunks(t, output)
	chunks = nonHistoryChunks(chunks)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want BOS/text/EOS: %#v", len(chunks), chunks)
	}
	if !chunks[0].IsBeginOfStream() {
		t.Fatalf("chunk[0] = %#v, want BOS", chunks[0])
	}
	if got := chunks[1].Part.(genx.Text); got != "final text" {
		t.Fatalf("chunk[1] text = %q, want final text", got)
	}
	if !chunks[2].IsEndOfStream() {
		t.Fatalf("chunk[2] = %#v, want EOS", chunks[2])
	}
}

func nextNonHistoryChunk(t *testing.T, output genx.Stream) *genx.MessageChunk {
	t.Helper()
	for {
		chunk, err := output.Next()
		if err != nil {
			t.Fatalf("output.Next() = %v", err)
		}
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != genx.HistoryUserAudioLabel {
			return chunk
		}
	}
}

func expectNoMoreNonHistoryChunks(t *testing.T, output genx.Stream) {
	t.Helper()
	for {
		chunk, err := output.Next()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, genx.ErrDone) {
				return
			}
			t.Fatalf("output.Next() = %v", err)
		}
		if chunk == nil || chunk.Ctrl == nil || chunk.Ctrl.Label != genx.HistoryUserAudioLabel {
			t.Fatalf("unexpected non-history chunk = %#v", chunk)
		}
	}
}

func expectTranscriptEOS(t *testing.T, output genx.Stream) {
	t.Helper()
	chunk := nextNonHistoryChunk(t, output)
	if chunk == nil || !chunk.IsEndOfStream() {
		t.Fatalf("output eos chunk = %#v, want transcript EOS", chunk)
	}
	if chunk.Role != genx.RoleUser || chunk.Name != "transcript" || chunk.Ctrl == nil || chunk.Ctrl.Label != "transcript" {
		t.Fatalf("output eos chunk = %#v, want user transcript EOS", chunk)
	}
}

func nonHistoryChunks(chunks []*genx.MessageChunk) []*genx.MessageChunk {
	filtered := make([]*genx.MessageChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			continue
		}
		filtered = append(filtered, chunk)
	}
	return filtered
}

func historyAudioChunks(chunks []*genx.MessageChunk) []*genx.MessageChunk {
	filtered := make([]*genx.MessageChunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.Label == genx.HistoryUserAudioLabel {
			filtered = append(filtered, chunk)
		}
	}
	return filtered
}

func buildASROGGOpusStream(t *testing.T, sampleRate, channels int, frame []int16) []byte {
	t.Helper()

	enc, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()

	packet, err := enc.Encode(frame, len(frame)/channels)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	var out bytes.Buffer
	sw, err := ogg.NewStreamWriter(&out, 77)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	packets := [][]byte{
		asrOpusHeadPacket(sampleRate, channels),
		asrOpusTagsPacket("asr-test"),
		packet,
	}
	for i, packet := range packets {
		if _, err := sw.WritePacket(packet, uint64(i), i == len(packets)-1); err != nil {
			t.Fatalf("WritePacket %d: %v", i, err)
		}
	}
	return out.Bytes()
}

func buildASRRawOpusPacket(t *testing.T, sampleRate, channels int, frame []int16) []byte {
	t.Helper()
	enc, err := opus.NewEncoder(sampleRate, channels, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	packet, err := enc.Encode(frame, len(frame)/channels)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(packet) == 0 {
		t.Fatal("encoded opus packet is empty")
	}
	return packet
}

func buildASRAudioFrame(frameSize, channels int) []int16 {
	frame := make([]int16, frameSize*channels)
	for i := range frame {
		frame[i] = int16((i * 97) % 24000)
	}
	return frame
}

func buildASRMP3Stream(t *testing.T, sampleRate, channels int, frame []int16) []byte {
	t.Helper()

	var out bytes.Buffer
	enc, err := mp3.NewEncoder(&out, sampleRate, channels, mp3.WithBitrate(64))
	if err != nil {
		t.Skipf("mp3 encoder unavailable: %v", err)
	}
	if _, err := enc.Write(pcm16LE(frame)); err != nil {
		t.Fatalf("write mp3 encoder: %v", err)
	}
	if err := enc.Flush(); err != nil {
		t.Fatalf("flush mp3 encoder: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("close mp3 encoder: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("encoded mp3 is empty")
	}
	return out.Bytes()
}

func asrOpusHeadPacket(sampleRate, channels int) []byte {
	packet := make([]byte, 19)
	copy(packet[:8], "OpusHead")
	packet[8] = 1
	packet[9] = byte(channels)
	binary.LittleEndian.PutUint32(packet[12:16], uint32(sampleRate))
	return packet
}

func asrOpusTagsPacket(vendor string) []byte {
	vendorBytes := []byte(vendor)
	packet := make([]byte, 8+4+len(vendorBytes)+4)
	copy(packet[:8], "OpusTags")
	binary.LittleEndian.PutUint32(packet[8:12], uint32(len(vendorBytes)))
	copy(packet[12:12+len(vendorBytes)], vendorBytes)
	return packet
}
