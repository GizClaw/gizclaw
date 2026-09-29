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
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestGenerateStreamReportsTranscriptWhereTheModelPlacesIt(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name   string
		deltas []string
		events []string
	}{
		{name: "after first sentence", deltas: []string{"三加五等于八。\n<a", "sr>三加", "五等于几</asr>", "\n还有问题吗？"},
			events: []string{"text:三加五等于八。\n", "asr:三加五等于几", "text:还有问题吗？"}},
		{name: "at the end", deltas: []string{"等于八。\n<asr>三加五</asr>"}, events: []string{"text:等于八。\n", "asr:三加五"}},
		{name: "at the start", deltas: []string{" \n<asr>三加五</asr>\n", "等于八。"}, events: []string{"asr:三加五", "text:等于八。"}},
		{name: "misspelled close", deltas: []string{"好的。<asr>你好</asr]再见"}, events: []string{"text:好的。", "asr:你好", "text:再见"}},
		{name: "unclosed at end", deltas: []string{"好的。\n<asr>你好"}, events: []string{"text:好的。\n", "asr:你好"}},
		{name: "tag-like text", deltas: []string{"价格", "<", "b>便宜</b>"}, events: []string{"text:价格", "text:<b>便宜</b>"}},
		{name: "no transcript", deltas: []string{"等于", "八。<"}, events: []string{"text:等于", "text:八。", "text:<"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			next := &recordingGenerator{deltas: testCase.deltas}
			stream, err := New(next).GenerateStream(t.Context(), "model/audio", audioContext(t, testOpusPackets(t, 2)...))
			if err != nil {
				t.Fatal(err)
			}
			events, err := drainEvents(stream)
			if err != nil {
				t.Fatalf("stream error = %v", err)
			}
			if !slices.Equal(events, testCase.events) {
				t.Fatalf("events = %q, want %q", events, testCase.events)
			}
		})
	}
}

func TestGenerateStreamConvertsAudioAndAddsInstruction(t *testing.T) {
	t.Parallel()
	next := &recordingGenerator{deltas: []string{"八。<asr>三加五</asr>"}}
	stream, err := New(next).GenerateStream(t.Context(), "model/audio", audioContext(t, testOpusPackets(t, 2)...))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := drainEvents(stream); err != nil {
		t.Fatal(err)
	}
	request := next.requests[0]
	var instructed bool
	for prompt := range request.Prompts() {
		instructed = instructed || prompt.Text == transcriptInstruction
	}
	if !instructed {
		t.Fatal("audio request carries no transcript instruction")
	}
	var messages []*genx.Message
	for message := range request.Messages() {
		messages = append(messages, message)
	}
	contents := messages[len(messages)-1].Payload.(genx.Contents)
	if len(contents) != 1 {
		t.Fatalf("latest user contents = %#v, want one WAV Blob", contents)
	}
	if frames := assertWAV(t, contents[0].(*genx.Blob), 16000, 1); frames != 2*320 {
		t.Fatalf("decoded frames = %d, want %d", frames, 2*320)
	}
}

func TestGenerateStreamPassesTextRequestsThrough(t *testing.T) {
	t.Parallel()
	next := &recordingGenerator{deltas: []string{"<asr>kept</asr>"}}
	builder := &genx.ModelContextBuilder{}
	builder.UserText("", "hello")
	request := builder.Build()
	stream, err := New(next).GenerateStream(t.Context(), "model/audio", request)
	if err != nil {
		t.Fatal(err)
	}
	events, err := drainEvents(stream)
	if err != nil || !slices.Equal(events, []string{"text:<asr>kept</asr>"}) {
		t.Fatalf("events=%q err=%v", events, err)
	}
	if next.requests[0] != request {
		t.Fatal("text request was rebuilt")
	}
}

func TestEarlierAudioIsConvertedWithoutTranscriptInstruction(t *testing.T) {
	t.Parallel()
	next := &recordingGenerator{deltas: []string{"reply"}}
	builder := &genx.ModelContextBuilder{}
	builder.UserBlob("", "audio/mp3", []byte("ab"))
	builder.ModelText("", "earlier reply")
	builder.UserText("", "now text")
	stream, err := New(next).GenerateStream(t.Context(), "model/audio", builder.Build())
	if err != nil {
		t.Fatal(err)
	}
	if events, err := drainEvents(stream); err != nil || !slices.Equal(events, []string{"text:reply"}) {
		t.Fatalf("events=%q err=%v", events, err)
	}
	for prompt := range next.requests[0].Prompts() {
		if prompt.Text == transcriptInstruction {
			t.Fatal("text turn carries the transcript instruction")
		}
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

	pcmBlob, err := requestAudioBlob(blobs("audio/L16; rate=24000; channels=2", []byte{1, 0, 2, 0}, []byte{3, 0, 4, 0}))
	if err != nil {
		t.Fatal(err)
	}
	if frames := assertWAV(t, pcmBlob, 24000, 2); frames != 2 || !bytes.Equal(pcmBlob.Data[44:], []byte{1, 0, 2, 0, 3, 0, 4, 0}) {
		t.Fatalf("PCM frames = %d payload = % x", frames, pcmBlob.Data[44:])
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
	deltas   []string
	requests []genx.ModelContext
}

func (g *recordingGenerator) GenerateStream(_ context.Context, _ string, mctx genx.ModelContext) (genx.Stream, error) {
	g.requests = append(g.requests, mctx)
	builder := genx.NewGrowableStreamBuilder(mctx, 8)
	for _, delta := range g.deltas {
		if err := builder.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(delta)}); err != nil {
			return nil, err
		}
	}
	if err := builder.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return builder.Stream(), nil
}

func (g *recordingGenerator) Invoke(context.Context, string, genx.ModelContext, *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	return genx.Usage{}, nil, errors.New("not used")
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

func TestTranscriptFilterToleratesClosingVariants(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		deltas     []string
		transcript string
		reply      string
	}{
		{deltas: []string{"<asr></asr]您好，我没有听清。"}, transcript: "", reply: "您好，我没有听清。"},
		{deltas: []string{"<ASR>hi</asr>", "\nyo"}, transcript: "hi", reply: "yo"},
		{deltas: []string{"<asr>三加五\n", "等于八"}, transcript: "三加五", reply: "等于八"},
		{deltas: []string{"<asr>三加五</as", "r", "]八"}, transcript: "三加五", reply: "八"},
		{deltas: []string{"<asr>三加五</asr", ">\n\n八"}, transcript: "三加五", reply: "八"},
		{deltas: []string{"<asr>\n三加五</asr>八"}, transcript: "三加五", reply: "八"},
	} {
		var filter transcriptFilter
		var reply strings.Builder
		transcript, closed := "", false
		for _, delta := range testCase.deltas {
			before, text, done, after := filter.consume(delta)
			reply.WriteString(before)
			reply.WriteString(after)
			if done {
				transcript, closed = text, true
			}
		}
		if !closed || transcript != testCase.transcript || reply.String() != testCase.reply {
			t.Fatalf("deltas %q: closed=%v transcript=%q reply=%q", testCase.deltas, closed, transcript, reply.String())
		}
	}
}
