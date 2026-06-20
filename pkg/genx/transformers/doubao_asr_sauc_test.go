package transformers

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
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/mp3"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/ogg"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
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

func TestDoubaoASRSAUCSendsLastNonEmptyAudioFrame(t *testing.T) {
	session := newFakeDoubaoASRSession()
	transformer := NewDoubaoASRSAUC(nil, WithDoubaoASRSAUCFormat("ogg_opus"))
	transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
		return session, nil
	}

	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), "asr", input)
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

	chunk, err := output.Next()
	if err != nil {
		t.Fatalf("output first chunk = %v", err)
	}
	if got := chunk.Part.(genx.Text); got != "recognized text" {
		t.Fatalf("output text = %q, want recognized text", got)
	}
	chunk, err = output.Next()
	if err != nil {
		t.Fatalf("output eos chunk = %v", err)
	}
	if chunk == nil || !chunk.IsEndOfStream() {
		t.Fatalf("output eos chunk = %#v", chunk)
	}
	if _, err := output.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("output final error = %v, want EOF", err)
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

func TestDoubaoASRSAUCDecodesOggToPCMWhenConfiguredPCM(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("native opus runtime is not available")
	}

	const sampleRate = 16000
	session := newFakeDoubaoASRSession()
	transformer := NewDoubaoASRSAUC(nil,
		WithDoubaoASRSAUCFormat("pcm"),
		WithDoubaoASRSAUCSampleRate(sampleRate),
		WithDoubaoASRSAUCChannels(1),
		WithDoubaoASRSAUCBits(16),
	)
	var opens []fakeDoubaoASROpen
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		opens = append(opens, fakeDoubaoASROpen{cfg: cfg, session: session})
		return session, nil
	}

	inputAudio := buildASROGGOpusStream(t, sampleRate, 1, buildASRAudioFrame(sampleRate/50, 1))
	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), "asr", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/ogg", Data: inputAudio}}); err != nil {
		t.Fatalf("push ogg audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	if _, err := output.Next(); err != nil {
		t.Fatalf("output text = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("output final error = %v, want EOF", err)
	}

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

func TestDoubaoASRSAUCDecodesRawOpusToPCMSession(t *testing.T) {
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
	transformer := NewDoubaoASRSAUC(nil,
		WithDoubaoASRSAUCFormat("pcm"),
		WithDoubaoASRSAUCSampleRate(sampleRate),
		WithDoubaoASRSAUCChannels(channels),
		WithDoubaoASRSAUCBits(16),
	)
	var opens []fakeDoubaoASROpen
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		opens = append(opens, fakeDoubaoASROpen{cfg: cfg, session: session})
		return session, nil
	}

	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), "asr", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/opus", Data: packet}}); err != nil {
		t.Fatalf("push raw opus audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	if _, err := output.Next(); err != nil {
		t.Fatalf("output text = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("output final error = %v, want EOF", err)
	}

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

func TestDoubaoASRSAUCDecodesMP3ToPCMSession(t *testing.T) {
	const sampleRate = 16000

	inputAudio := buildASRMP3Stream(t, sampleRate, 1, buildASRAudioFrame(sampleRate/2, 1))
	session := newFakeDoubaoASRSession()
	transformer := NewDoubaoASRSAUC(nil,
		WithDoubaoASRSAUCSampleRate(sampleRate),
		WithDoubaoASRSAUCChannels(1),
		WithDoubaoASRSAUCBits(16),
	)
	var opens []fakeDoubaoASROpen
	transformer.newSession = func(_ context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		opens = append(opens, fakeDoubaoASROpen{cfg: cfg, session: session})
		return session, nil
	}

	input := newBufferStream(4)
	output, err := transformer.Transform(context.Background(), "asr", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/mpeg", Data: inputAudio}}); err != nil {
		t.Fatalf("push mp3 audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	if _, err := output.Next(); err != nil {
		t.Fatalf("output text = %v", err)
	}
	if _, err := output.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("output final error = %v, want EOF", err)
	}

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

func TestDoubaoASRSAUCEmitsDefiniteUtterancesWithNonMonotonicTimes(t *testing.T) {
	session := newFakeDoubaoASRSession()
	transformer := NewDoubaoASRSAUC(nil,
		WithDoubaoASRSAUCFormat("pcm"),
		WithDoubaoASRSAUCRealtimePacing(false),
	)
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
	output, err := transformer.Transform(context.Background(), "asr", input)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if err := input.Push(&genx.MessageChunk{Part: &genx.Blob{MIMEType: "audio/pcm", Data: []byte{1, 2}}}); err != nil {
		t.Fatalf("push audio = %v", err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("close input = %v", err)
	}

	chunk, err := output.Next()
	if err != nil {
		t.Fatalf("first output = %v", err)
	}
	if got := chunk.Part.(genx.Text); got != "first" {
		t.Fatalf("first output = %q", got)
	}
	chunk, err = output.Next()
	if err != nil {
		t.Fatalf("second output = %v", err)
	}
	if got := chunk.Part.(genx.Text); got != "second" {
		t.Fatalf("second output = %q", got)
	}
}

func TestDoubaoASRSAUCEmitInterimControlsNonDefiniteUtterances(t *testing.T) {
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
				{text: "final text"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := newFakeDoubaoASRSession()
			transformer := NewDoubaoASRSAUC(nil,
				WithDoubaoASRSAUCFormat("pcm"),
				WithDoubaoASRSAUCRealtimePacing(false),
				WithDoubaoASRSAUCEmitInterim(tt.emitInterim),
			)
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
			output, err := transformer.Transform(context.Background(), "asr", input)
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
				chunk, err := output.Next()
				if err != nil {
					t.Fatalf("output[%d] = %v", i, err)
				}
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

func TestDoubaoASRSAUCEmitInterimDoesNotDuplicateFinalTextResult(t *testing.T) {
	session := newFakeDoubaoASRSession()
	transformer := NewDoubaoASRSAUC(nil,
		WithDoubaoASRSAUCFormat("pcm"),
		WithDoubaoASRSAUCRealtimePacing(false),
		WithDoubaoASRSAUCEmitInterim(true),
	)
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
	output, err := transformer.Transform(context.Background(), "asr", input)
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
