package streamkit

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTTSSentenceSegmenterCommitsChineseCommaWithLookahead(t *testing.T) {
	for _, phrase := range []string{"首先把门窗全部关好，", "（首先把门窗全部关好，"} {
		t.Run(phrase, func(t *testing.T) {
			segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
			for _, r := range phrase {
				segmenter.WriteString(string(r))
				if got := segmenter.Segments(false); len(got) != 0 {
					t.Fatalf("unconfirmed phrase was emitted: %q", got)
				}
			}
			segmenter.WriteString("再")
			if got := segmenter.Segments(false); !slices.Equal(got, []string{phrase}) {
				t.Fatalf("first phrase with one-character lookahead = %q, want %q", got, phrase)
			}
			if got := segmenter.Segments(true); !slices.Equal(got, []string{"再"}) {
				t.Fatalf("remaining lookahead = %q", got)
			}
		})
	}
}

func TestTTSSentenceSegmenterRetainsChinesePunctuationRun(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
	for _, text := range []string{"他说：“这件事真的完成了吗？", "！", "”"} {
		segmenter.WriteString(text)
		if got := segmenter.Segments(false); len(got) != 0 {
			t.Fatalf("unfinished punctuation run was emitted: %q", got)
		}
	}
	segmenter.WriteString("接")
	phrase := "他说：“这件事真的完成了吗？！”"
	if got := segmenter.Segments(false); !slices.Equal(got, []string{phrase}) {
		t.Fatalf("confirmed phrase = %q, want %q", got, phrase)
	}
}

func TestTTSSentenceSegmenterResetsLexicalStateAtMessageEnd(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
	for _, unfinished := range []string{"<node>隐藏内容", "<node attr=\"unfinished", "https://example.com/unfinished"} {
		segmenter.WriteString(unfinished)
		if got := segmenter.Segments(true); len(got) != 0 {
			t.Fatalf("unfinished control text leaked: %q", got)
		}
		segmenter.WriteString("下一条消息可以正常朗读。")
		if got := strings.Join(segmenter.Segments(true), ""); got != "下一条消息可以正常朗读。" {
			t.Fatalf("lexical state crossed a message boundary: %q", got)
		}
	}
}

func TestTTSSentenceSegmenterUsesObservedWhitespaceWithoutSplittingAbbreviations(t *testing.T) {
	for _, abbreviation := range []string{"Dr.", "Prof.", "Capt.", "Mrs.", "J."} {
		t.Run(abbreviation, func(t *testing.T) {
			segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
			prefix := "Please ask " + abbreviation + " "
			segmenter.WriteString(prefix)
			if got := segmenter.Segments(false); len(got) != 0 {
				t.Fatalf("observed space split an abbreviation: %q", got)
			}
			segmenter.WriteString("Smith today. ")
			want := prefix + "Smith today."
			if got := strings.Join(segmenter.Segments(false), ""); got != want {
				t.Fatalf("completed sentence with observed space = %q, want %q", got, want)
			}
		})
	}
}

func TestTTSSentenceSegmenterUsesWhitespaceAfterClosingPunctuation(t *testing.T) {
	for _, phrase := range []string{"他说：“这件事已经完成。”", "She said, \"Please wait here.\"", "आज मौसम अच्छा है।", "هذا اختبار جديد؟"} {
		t.Run(phrase, func(t *testing.T) {
			segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
			segmenter.WriteString(phrase + " ")
			if got := strings.Join(segmenter.Segments(false), ""); got != phrase {
				t.Fatalf("observed space did not commit the complete phrase: got %q, want %q", got, phrase)
			}
		})
	}
}

func TestTTSSentenceSegmenterKeepsNonbreakingNumberAtBudget(t *testing.T) {
	prefix := strings.Repeat("甲", 79)
	chunks := []string{prefix + "1\u202f", "000元。"}
	ttsCheckSegments(t, ttsCollectSegments(chunks), prefix+"1\u202f000元。", []string{"1\u202f000"})
}

func FuzzTTSSentenceSegmenterNormalizedText(f *testing.F) {
	for _, sample := range ttsSegmentationSamples() {
		f.Add(sample.input, uint64(0))
	}
	f.Fuzz(func(t *testing.T, text string, seed uint64) {
		if len(text) > 1024 || !utf8.ValidString(text) {
			t.Skip()
		}
		ttsCheckSegments(t, ttsCollectSegments(ttsMixedChunks(text, seed)), normalizeTTSSpokenText(text), nil)
	})
}

func BenchmarkTTSSentenceSegmenter(b *testing.B) {
	for _, sample := range []struct{ name, text string }{
		{"chinese", "首先把门窗全部关好，再检查厨房电源。今天的温度是23.5度，账户余额是1,234.56元。"},
		{"english", "Please ask Dr. Smith about the U.S. policy before 10:15 today."},
		{"control", `<node id="tool_call">隐藏的控制内容。</node>这是可以朗读的正文。请看（https://example.com）。`},
	} {
		for _, width := range []int{1, 7} {
			b.Run(fmt.Sprintf("%s/width_%d", sample.name, width), func(b *testing.B) {
				runes := []rune(sample.text)
				var chunks []string
				for start := 0; start < len(runes); start += width {
					chunks = append(chunks, string(runes[start:min(start+width, len(runes))]))
				}
				b.SetBytes(int64(len(sample.text)))
				b.ReportAllocs()
				for b.Loop() {
					segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
					for _, chunk := range chunks {
						segmenter.WriteString(chunk)
						segmenter.Segments(false)
					}
					segmenter.Segments(true)
				}
			})
		}
	}
}
