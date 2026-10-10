package doubaochat

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestGenerateStreamKeepsReplyAndTranscriptSeparate(t *testing.T) {
	for _, reply := range []string{"八。", "{\"answer\":8}", "<asr>literal business text</asr>", "<b>标题</b>"} {
		t.Run(reply, func(t *testing.T) {
			next := &recordingGenerator{deltas: []string{reply}, transcription: []string{`{"transcript":"三加五等于几"}`}}
			stream, err := New(next).GenerateStream(t.Context(), "model/audio", audioContext(t, testOpusPackets(t, 2)...))
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			body, transcripts, err := drainAudio(stream)
			if err != nil || body != reply || !slices.Equal(transcripts, []string{"三加五等于几"}) || len(next.Requests()) != 2 {
				t.Fatalf("body=%q transcripts=%q error=%v calls=%d", body, transcripts, err, len(next.Requests()))
			}
		})
	}
}

func TestGenerateStreamConvertsAudioAndOwnsASRRequest(t *testing.T) {
	next := &recordingGenerator{deltas: []string{"八。"}}
	stream, err := New(next).GenerateStream(t.Context(), "model/audio", audioContext(t, testOpusPackets(t, 2)...))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if _, _, err := drainAudio(stream); err != nil {
		t.Fatal(err)
	}
	if len(next.Requests()) != 2 {
		t.Fatalf("calls=%d", len(next.Requests()))
	}
	for _, request := range next.Requests() {
		messages := slices.Collect(request.Messages())
		contents := messages[len(messages)-1].Payload.(genx.Contents)
		if len(contents) != 1 {
			t.Fatalf("current user parts=%d", len(contents))
		}
		if frames := assertWAV(t, contents[0].(*genx.Blob), 16000, 1); frames != 640 {
			t.Fatalf("frames=%d", frames)
		}
		prompts := slices.Collect(request.Prompts())
		if isAudioTranscriptRequest(request) {
			if len(messages) != 1 || len(prompts) != 1 || prompts[0].Text != audioTranscriptInstruction {
				t.Fatal("ASR inherited caller state")
			}
		} else if len(prompts) != 1 || prompts[0].Text != "be brief" {
			t.Fatal("reply prompt was modified")
		}
	}
}

func TestGenerateStreamPassesTextRequestsThrough(t *testing.T) {
	next := &recordingGenerator{deltas: []string{"<asr>kept</asr>"}}
	builder := &genx.ModelContextBuilder{}
	builder.UserText("", "hello")
	request := builder.Build()
	stream, err := New(next).GenerateStream(t.Context(), "model/audio", request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	events, err := drainEvents(stream)
	if err != nil || !slices.Equal(events, []string{"text:<asr>kept</asr>"}) {
		t.Fatalf("events=%q error=%v", events, err)
	}
	if requests := next.Requests(); len(requests) != 1 || requests[0] != request {
		t.Fatal("text request was rebuilt or transcribed")
	}
}

func TestEarlierAudioIsConvertedWithoutTranscriptRequest(t *testing.T) {
	next := &recordingGenerator{deltas: []string{"reply"}}
	builder := &genx.ModelContextBuilder{}
	builder.UserBlob("", "audio/mp3", []byte("ab"))
	builder.ModelText("", "earlier reply")
	builder.UserText("", "now text")
	stream, err := New(next).GenerateStream(t.Context(), "model/audio", builder.Build())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	events, err := drainEvents(stream)
	if err != nil || !slices.Equal(events, []string{"text:reply"}) || len(next.Requests()) != 1 {
		t.Fatalf("events=%q error=%v calls=%d", events, err, len(next.Requests()))
	}
}

func TestRequestAudioBlobFormats(t *testing.T) {
	t.Parallel()
	packets := testOpusPackets(t, 5)
	var ogg bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&ogg, 16000, 1, packets); err != nil {
		t.Fatal(err)
	}
	oggBytes := ogg.Bytes()
	for _, testCase := range []struct {
		name  string
		audio []*genx.Blob
	}{
		{name: "raw packets", audio: blobs("audio/opus", packets...)},
		{name: "ogg pages", audio: blobs("audio/ogg; codecs=opus", oggBytes[:len(oggBytes)/2], oggBytes[len(oggBytes)/2:])},
	} {
		blob, err := requestAudioBlob(testCase.audio)
		if err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		if frames := assertWAV(t, blob, 16000, 1); frames != 5*320 {
			t.Fatalf("%s: decoded frames = %d, want %d", testCase.name, frames, 5*320)
		}
	}

	// 100 ms of 24 kHz stereo PCM becomes about 100 ms of 16 kHz mono.
	stereo := make([]byte, 2400*4)
	for frame := range 2400 {
		sample := int16(8000 * math.Sin(2*math.Pi*440*float64(frame)/24000))
		for channel := range 2 {
			binary.LittleEndian.PutUint16(stereo[frame*4+channel*2:], uint16(sample))
		}
	}
	pcmBlob, err := requestAudioBlob(blobs("audio/L16; rate=24000; channels=2", stereo[:len(stereo)/2], stereo[len(stereo)/2:]))
	if err != nil {
		t.Fatal(err)
	}
	if frames := assertWAV(t, pcmBlob, 16000, 1); frames < 1568 || frames > 1632 {
		t.Fatalf("resampled PCM frames = %d, want about 1600", frames)
	}
	mono := []byte{1, 0, 2, 0}
	passed16k, err := requestAudioBlob(blobs("audio/pcm", mono))
	if err != nil || !bytes.Equal(passed16k.Data[44:], mono) {
		t.Fatalf("16 kHz mono PCM = %#v, %v", passed16k, err)
	}
	mp3, err := requestAudioBlob(blobs("audio/mp3", []byte("ab"), []byte("cd")))
	if err != nil || mp3.MIMEType != "audio/mpeg" || string(mp3.Data) != "abcd" {
		t.Fatalf("MP3 = %#v, %v", mp3, err)
	}
	wav := pcm.L16Mono16K.WAV([]byte{9, 0})
	passed, err := requestAudioBlob(blobs("audio/wav", wav))
	if err != nil || passed.MIMEType != "audio/wav" || !bytes.Equal(passed.Data, wav) {
		t.Fatalf("WAV = %#v, %v", passed, err)
	}
}

func TestRequestAudioBlobDecodesLongOpusPackets(t *testing.T) {
	t.Parallel()
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationVoIP)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	var frames [2][]byte
	for index := range frames {
		samples := make([]int16, 960)
		for sample := range samples {
			samples[sample] = int16(8000 * math.Sin(2*math.Pi*440*float64(index*960+sample)/16000))
		}
		packet, err := encoder.Encode(samples, len(samples))
		if err != nil {
			t.Fatal(err)
		}
		if packet[0]&0x03 != 0 || len(packet)-1 >= 252 {
			t.Fatalf("60 ms packet TOC %#x size %d is not one short frame", packet[0], len(packet))
		}
		frames[index] = packet
	}
	// RFC 6716 code 3: one TOC, a VBR frame-count byte for two frames, the
	// first frame length, then both 60 ms frames: one 120 ms packet.
	packet := []byte{frames[0][0] | 0x03, 0x80 | 2, byte(len(frames[0]) - 1)}
	packet = append(packet, frames[0][1:]...)
	packet = append(packet, frames[1][1:]...)
	blob, err := requestAudioBlob(blobs("audio/opus", packet))
	if err != nil {
		t.Fatalf("120 ms packet: %v", err)
	}
	if decoded := assertWAV(t, blob, 16000, 1); decoded != 1920 {
		t.Fatalf("decoded frames = %d, want 1920", decoded)
	}
}

func TestRequestAudioBlobRejectsUnusableAudio(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name  string
		audio []*genx.Blob
		want  string
	}{
		{name: "mixed", audio: append(blobs("audio/opus", []byte{1}), blobs("audio/mpeg", []byte{2})...), want: "mixes"},
		{name: "two WAV files", audio: blobs("audio/wav", []byte("a"), []byte("b")), want: "WAV parts"},
		{name: "unsupported codec", audio: blobs("audio/flac", []byte("a")), want: "unsupported"},
		{name: "unsupported PCM rate", audio: blobs("audio/pcm; rate=11025", []byte{0, 0}), want: "unsupported L16"},
		{name: "empty Opus", audio: blobs("audio/opus", []byte{}), want: "no Opus audio"},
	} {
		if _, err := requestAudioBlob(testCase.audio); err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Fatalf("%s: error = %v, want %q", testCase.name, err, testCase.want)
		}
	}
}

type recordingGenerator struct {
	mu            sync.Mutex
	deltas        []string
	transcription []string
	requests      []genx.ModelContext
}

func (g *recordingGenerator) GenerateStream(_ context.Context, _ string, mctx genx.ModelContext) (genx.Stream, error) {
	g.mu.Lock()
	g.requests = append(g.requests, mctx)
	g.mu.Unlock()
	deltas := g.deltas
	if isAudioTranscriptRequest(mctx) {
		deltas = g.transcription
		if deltas == nil {
			deltas = []string{`{"transcript":"heard"}`}
		}
	}
	builder := genx.NewGrowableStreamBuilder(mctx, 8)
	for _, delta := range deltas {
		if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(delta)}); err != nil {
			return nil, err
		}
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return builder.Stream(), nil
}

func (g *recordingGenerator) Requests() []genx.ModelContext {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.requests)
}

func (*recordingGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("unexpected tool invocation")
}

func isAudioTranscriptRequest(mctx genx.ModelContext) bool {
	for prompt := range mctx.Prompts() {
		if prompt.Name == audioTranscriptPromptName && prompt.Text == audioTranscriptInstruction {
			return true
		}
	}
	return false
}

func drainAudio(stream genx.Stream) (string, []string, error) {
	events, err := drainEvents(stream)
	var body strings.Builder
	var transcripts []string
	for _, event := range events {
		if text, ok := strings.CutPrefix(event, "text:"); ok {
			body.WriteString(text)
		} else if text, ok := strings.CutPrefix(event, "asr:"); ok {
			transcripts = append(transcripts, text)
		}
	}
	return body.String(), transcripts, err
}

// drainEvents renders reply text as "text:" and transcripts as "asr:".
func drainEvents(stream genx.Stream) ([]string, error) {
	var events []string
	for {
		chunk, err := stream.Next()
		if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
			return events, nil
		}
		if err != nil {
			return events, err
		}
		if text, ok := genx.InputTranscript(chunk); ok {
			events = append(events, "asr:"+text)
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok && text != "" {
			events = append(events, "text:"+string(text))
		}
	}
}

func audioContext(t testing.TB, packets ...[]byte) genx.ModelContext {
	t.Helper()
	builder := &genx.ModelContextBuilder{}
	builder.PromptText("system", "be brief")
	builder.UserText("", "earlier")
	builder.ModelText("", "reply")
	for _, packet := range packets {
		builder.UserBlob("", "audio/opus", packet)
	}
	return builder.Build()
}

func blobs(mimeType string, payloads ...[]byte) []*genx.Blob {
	out := make([]*genx.Blob, 0, len(payloads))
	for _, payload := range payloads {
		out = append(out, &genx.Blob{MIMEType: mimeType, Data: payload})
	}
	return out
}

// testOpusPackets encodes count 20 ms frames of a 440 Hz tone.
func testOpusPackets(t testing.TB, count int) [][]byte {
	t.Helper()
	encoder, err := opus.NewEncoder(16000, 1, opus.ApplicationVoIP)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	packets := make([][]byte, 0, count)
	for frame := range count {
		samples := make([]int16, 320)
		for index := range samples {
			phase := 2 * math.Pi * 440 * float64(frame*320+index) / 16000
			samples[index] = int16(8000 * math.Sin(phase))
		}
		packet, err := encoder.Encode(samples, len(samples))
		if err != nil {
			t.Fatal(err)
		}
		packets = append(packets, packet)
	}
	return packets
}

// assertWAV checks a canonical PCM WAV header and returns its frame count.
func assertWAV(t testing.TB, blob *genx.Blob, sampleRate, channels int) int {
	t.Helper()
	if blob == nil || blob.MIMEType != "audio/wav" || len(blob.Data) < 44 {
		t.Fatalf("blob = %#v, want WAV", blob)
	}
	data := blob.Data
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" || string(data[36:40]) != "data" {
		t.Fatalf("WAV header = % x", data[:44])
	}
	if got := int(binary.LittleEndian.Uint16(data[22:24])); got != channels {
		t.Fatalf("WAV channels = %d, want %d", got, channels)
	}
	if got := int(binary.LittleEndian.Uint32(data[24:28])); got != sampleRate {
		t.Fatalf("WAV sample rate = %d, want %d", got, sampleRate)
	}
	size := int(binary.LittleEndian.Uint32(data[40:44]))
	if size != len(data)-44 {
		t.Fatalf("WAV data size = %d, payload %d", size, len(data)-44)
	}
	return size / (2 * channels)
}
