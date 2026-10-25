package doubaoasr

import (
	"context"
	"slices"
	"strings"
	"testing"

	doubaospeech "github.com/GizClaw/doubao-speech-go"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

func TestTransformerInterimDoesNotReplayCommittedResults(t *testing.T) {
	first := doubaospeech.ASRV2Utterance{Text: "first question", StartTime: 0, EndTime: 200, Definite: true}
	repeatedWords := doubaospeech.ASRV2Utterance{Text: "first question", StartTime: 200, EndTime: 400, Definite: true}
	second := doubaospeech.ASRV2Utterance{Text: "second question", StartTime: 200, EndTime: 400, Definite: true}
	for _, test := range []struct {
		name      string
		results   []*doubaospeech.ASRV2Result
		committed []string
		interim   []string
	}{
		{
			name: "repeated committed snapshot",
			results: []*doubaospeech.ASRV2Result{
				{Text: first.Text, Utterances: []doubaospeech.ASRV2Utterance{first}, IsFinal: true},
				{Text: first.Text},
				{Text: first.Text, Utterances: []doubaospeech.ASRV2Utterance{first}, IsFinal: true},
			},
			committed: []string{first.Text},
		},
		{
			name: "text populated after committed utterance",
			results: []*doubaospeech.ASRV2Result{
				{Utterances: []doubaospeech.ASRV2Utterance{first}, IsFinal: true},
				{Text: first.Text},
				{Text: first.Text, Utterances: []doubaospeech.ASRV2Utterance{first}, IsFinal: true},
			},
			committed: []string{first.Text},
		},
		{
			name: "same words in a new timed utterance",
			results: []*doubaospeech.ASRV2Result{
				{Text: first.Text, Utterances: []doubaospeech.ASRV2Utterance{first}, IsFinal: true},
				{Text: repeatedWords.Text, Utterances: []doubaospeech.ASRV2Utterance{repeatedWords}, IsFinal: true},
				{Text: repeatedWords.Text, Utterances: []doubaospeech.ASRV2Utterance{repeatedWords}, IsFinal: true},
			},
			committed: []string{first.Text, repeatedWords.Text},
		},
		{
			name: "new interim repeats earlier words",
			results: []*doubaospeech.ASRV2Result{
				{Text: first.Text, Utterances: []doubaospeech.ASRV2Utterance{first}, IsFinal: true},
				{Text: repeatedWords.Text, Utterances: []doubaospeech.ASRV2Utterance{{Text: repeatedWords.Text, StartTime: 200, EndTime: 350}}},
				{Text: repeatedWords.Text, Utterances: []doubaospeech.ASRV2Utterance{repeatedWords}, IsFinal: true},
			},
			committed: []string{first.Text, repeatedWords.Text},
			interim:   []string{repeatedWords.Text},
		},
		{
			name: "cumulative committed utterances with new interim text",
			results: []*doubaospeech.ASRV2Result{
				{Text: first.Text, Utterances: []doubaospeech.ASRV2Utterance{first}, IsFinal: true},
				{Text: "second interim"},
				{Text: first.Text + " " + second.Text, Utterances: []doubaospeech.ASRV2Utterance{first, second}, IsFinal: true},
				{Text: first.Text + " " + second.Text, Utterances: []doubaospeech.ASRV2Utterance{first, second}, IsFinal: true},
			},
			committed: []string{first.Text, second.Text},
			interim:   []string{"second interim"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// The real SDK marks a result final when any utterance is definite,
			// including positive-sequence provider frames.
			for i, result := range test.results {
				for _, utterance := range result.Utterances {
					if utterance.Definite && !result.IsFinal {
						t.Fatalf("fixture result %d violates SDK definite/final semantics", i)
					}
				}
			}
			session := newFakeDoubaoASRSession()
			transformer := newTransformer(Config{Format: "pcm", RealtimePacing: new(false), EmitInterim: true})
			transformer.newSession = func(context.Context, doubaoASRSessionConfig) (doubaoASRSession, error) {
				return session, nil
			}
			session.sendAudio = func(_ context.Context, _ []byte, last bool) error {
				if last {
					for _, result := range test.results {
						session.result <- result
					}
					close(session.result)
				}
				return nil
			}
			input := newBufferStream(2)
			output, err := transformer.Transform(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			if err := input.Push(&genx.MessageChunk{
				Part: &genx.Blob{MIMEType: "audio/pcm", Data: pcm16LE(buildASRAudioFrame(8000, 1))},
				Ctrl: &genx.StreamCtrl{StreamID: "turn-1"},
			}); err != nil {
				t.Fatal(err)
			}
			if err := input.Close(); err != nil {
				t.Fatal(err)
			}
			chunks := nonHistoryChunks(collectTransformerChunks(t, output))
			var committed, interim []string
			var routes, terminals []string
			for _, chunk := range chunks {
				if chunk.IsBeginOfStream() {
					routes = append(routes, chunk.Ctrl.StreamID)
				}
				if chunk.IsEndOfStream() {
					if err := genx.StreamError(chunk.Ctrl); err != nil {
						t.Errorf("transcript terminal %s: %v", chunk.Ctrl.StreamID, err)
					}
					terminals = append(terminals, chunk.Ctrl.StreamID)
				}
				text, ok := chunk.Part.(genx.Text)
				if !ok || strings.TrimSpace(string(text)) == "" {
					continue
				}
				if chunk.Ctrl.TextInterim {
					interim = append(interim, string(text))
				} else {
					committed = append(committed, string(text))
				}
			}
			if !slices.Equal(committed, test.committed) || !slices.Equal(interim, test.interim) {
				t.Errorf("transcript committed=%q interim=%q, want committed=%q interim=%q", committed, interim, test.committed, test.interim)
			}
			seenRoutes := make(map[string]bool)
			for _, route := range routes {
				if route == "" || seenRoutes[route] {
					t.Errorf("duplicate or unnamed transcript route: %q", route)
				}
				seenRoutes[route] = true
			}
			if len(routes) != len(test.committed) || !slices.Equal(routes, terminals) {
				t.Errorf("transcript routes=%v terminals=%v, want %d independent completed routes", routes, terminals, len(test.committed))
			}
		})
	}
}
