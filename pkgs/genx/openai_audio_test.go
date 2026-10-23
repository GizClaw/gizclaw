package genx

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"math"
	"strings"
	"testing"
)

func TestOpenAIAudioInputPreservesMixedEncodedParts(t *testing.T) {
	builder := &ModelContextBuilder{}
	builder.UserBlob("", "audio/mp3", []byte("mp3"))
	builder.UserBlob("", "audio/wav", pcm.L16Mono16K.WAV([]byte{1, 0}))
	request, err := (&OpenAIGenerator{Model: "audio-model"}).chatCompletion(builder.Build(), nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Messages []struct {
			Content []struct {
				Audio struct {
					Format string `json:"format"`
				} `json:"input_audio"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Messages) != 1 || len(decoded.Messages[0].Content) != 2 || decoded.Messages[0].Content[0].Audio.Format != "mp3" || decoded.Messages[0].Content[1].Audio.Format != "wav" {
		t.Fatalf("audio request=%s", encoded)
	}
}

func TestRequestAudioBlobFormats(t *testing.T) {
	t.Parallel()
	packets := openAITestOpusPackets(t, 5)
	var ogg bytes.Buffer
	if err := codecconv.OpusPacketsToOgg(&ogg, 16000, 1, packets); err != nil {
		t.Fatal(err)
	}
	oggBytes := ogg.Bytes()
	for _, testCase := range []struct {
		name  string
		audio []*Blob
	}{
		{name: "raw packets", audio: openAIAudioBlobs("audio/opus", packets...)},
		{name: "ogg pages", audio: openAIAudioBlobs("audio/ogg; codecs=opus", oggBytes[:len(oggBytes)/2], oggBytes[len(oggBytes)/2:])},
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
	pcmBlob, err := requestAudioBlob(openAIAudioBlobs("audio/L16; rate=24000; channels=2", stereo[:len(stereo)/2], stereo[len(stereo)/2:]))
	if err != nil {
		t.Fatal(err)
	}
	if frames := assertWAV(t, pcmBlob, 16000, 1); frames < 1568 || frames > 1632 {
		t.Fatalf("resampled PCM frames = %d, want about 1600", frames)
	}
	mono := []byte{1, 0, 2, 0}
	passed16k, err := requestAudioBlob(openAIAudioBlobs("audio/pcm", mono))
	if err != nil || !bytes.Equal(passed16k.Data[44:], mono) {
		t.Fatalf("16 kHz mono PCM = %#v, %v", passed16k, err)
	}
	mp3, err := requestAudioBlob(openAIAudioBlobs("audio/mp3", []byte("ab"), []byte("cd")))
	if err != nil || mp3.MIMEType != "audio/mpeg" || string(mp3.Data) != "abcd" {
		t.Fatalf("MP3 = %#v, %v", mp3, err)
	}
	wav := pcm.L16Mono16K.WAV([]byte{9, 0})
	passed, err := requestAudioBlob(openAIAudioBlobs("audio/wav", wav))
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
	blob, err := requestAudioBlob(openAIAudioBlobs("audio/opus", packet))
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
		audio []*Blob
		want  string
	}{
		{name: "mixed", audio: append(openAIAudioBlobs("audio/opus", []byte{1}), openAIAudioBlobs("audio/mpeg", []byte{2})...), want: "mixes"},
		{name: "two WAV files", audio: openAIAudioBlobs("audio/wav", []byte("a"), []byte("b")), want: "WAV parts"},
		{name: "unsupported codec", audio: openAIAudioBlobs("audio/flac", []byte("a")), want: "unsupported"},
		{name: "unsupported PCM rate", audio: openAIAudioBlobs("audio/pcm; rate=11025", []byte{0, 0}), want: "unsupported L16"},
		{name: "empty Opus", audio: openAIAudioBlobs("audio/opus", []byte{}), want: "no Opus audio"},
	} {
		if _, err := requestAudioBlob(testCase.audio); err == nil || !strings.Contains(err.Error(), testCase.want) {
			t.Fatalf("%s: error = %v, want %q", testCase.name, err, testCase.want)
		}
	}
}

func openAIAudioBlobs(mimeType string, payloads ...[]byte) []*Blob {
	out := make([]*Blob, 0, len(payloads))
	for _, payload := range payloads {
		out = append(out, &Blob{MIMEType: mimeType, Data: payload})
	}
	return out
}

// testOpusPackets encodes count 20 ms frames of a 440 Hz tone.
func openAITestOpusPackets(t testing.TB, count int) [][]byte {
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
func assertWAV(t testing.TB, blob *Blob, sampleRate, channels int) int {
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
