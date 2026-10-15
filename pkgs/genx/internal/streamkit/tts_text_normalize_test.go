package streamkit

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeTTSSpokenTextRemovesMarkupAndURLs(t *testing.T) {
	got := normalizeTTSSpokenText(`<node id="answer" />请看（https://example.com/a?b=1），访问 https://foo.example/test。`)
	want := "请看，访问。"
	if got != want {
		t.Fatalf("normalizeTTSSpokenText() = %q, want %q", got, want)
	}
}

func TestNormalizeTTSSpokenTextKeepsNonTagAngleText(t *testing.T) {
	got := normalizeTTSSpokenText("3 < 5，6 > 4。")
	want := "3 < 5，6 > 4。"
	if got != want {
		t.Fatalf("normalizeTTSSpokenText() = %q, want %q", got, want)
	}
}

func TestNormalizeTTSSpokenTextRemovesXMLTagContents(t *testing.T) {
	got := normalizeTTSSpokenText("你好<br/>世界，<voice name=\"x\">继续</voice>。")
	want := "你好世界，。"
	if got != want {
		t.Fatalf("normalizeTTSSpokenText() = %q, want %q", got, want)
	}
}

func TestNormalizeTTSSpokenTextRemovesStructuredXMLBlocks(t *testing.T) {
	got := normalizeTTSSpokenText(`<node id="answer">可以。<block><item name="city">上海</item><item name="date">今天</item></block>请稍等。</node>`)
	want := ""
	if got != want {
		t.Fatalf("normalizeTTSSpokenText() = %q, want %q", got, want)
	}
}

func TestTTSTextNormalizerKeepsStateAcrossByteChunks(t *testing.T) {
	for _, sample := range []struct{ name, input, want string }{
		{"spaces", " Please  keep\tthese words. ", "Please keep these words."},
		{"unicode", "这一笔金额是1\u202f000元，职业是👩‍💻。", "这一笔金额是1\u202f000元，职业是👩‍💻。"},
		{"quoted_tag", `<node attr="a>b">隐藏内容。</node>可朗读的正文。`, "可朗读的正文。"},
		{"large_tag", `<node attr="` + strings.Repeat("x", 4096) + `">隐藏内容。</node>可朗读的正文。`, "可朗读的正文。"},
		{"deep_xml", strings.Repeat("<node>", 100) + "隐藏内容。" + strings.Repeat("</node>", 100) + "可朗读的正文。", "可朗读的正文。"},
		{"url", "链接是HTTPS://example.com/a?b=1。接着说。", "链接是。接着说。"},
		{"url_parenthesis", "请看（ https://example.com/path ），接着说。", "请看，接着说。"},
		{"url_parenthesis_long_space", "请看（" + strings.Repeat(" ", 4096) + "https://example.com/path），接着说。", "请看，接着说。"},
		{"parenthesis_text", "解释如下（详见https://example.com）。", "解释如下（详见）。"},
		{"math", "数学关系是3 < 5，6 > 4。", "数学关系是3 < 5，6 > 4。"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			var normalizer ttsTextNormalizer
			var spoken strings.Builder
			for index := range len(sample.input) {
				spoken.WriteString(normalizer.WriteString(sample.input[index : index+1]))
				if len(normalizer.tag) > ttsTagBufferBytes || len(normalizer.xml) > ttsXMLMaxDepth {
					t.Fatal("control state exceeded its bound")
				}
			}
			spoken.WriteString(normalizer.Finish())
			if got := spoken.String(); got != sample.want {
				t.Fatalf("incremental normalization = %q, want %q", got, sample.want)
			}
		})
	}
}

func FuzzTTSTextNormalizerChunking(f *testing.F) {
	for _, sample := range ttsSegmentationSamples() {
		f.Add(sample.input, uint64(0))
	}
	f.Fuzz(func(t *testing.T, text string, seed uint64) {
		if len(text) > 4096 || !utf8.ValidString(text) {
			t.Skip()
		}
		var normalizer ttsTextNormalizer
		var spoken strings.Builder
		random := rand.New(rand.NewPCG(seed, 29))
		for start := 0; start < len(text); {
			end := min(start+1+random.IntN(11), len(text))
			spoken.WriteString(normalizer.WriteString(text[start:end]))
			start = end
		}
		spoken.WriteString(normalizer.Finish())
		if got, want := spoken.String(), normalizeTTSSpokenText(text); got != want {
			t.Fatalf("normalization depends on byte chunking: got %q, want %q", got, want)
		}
	})
}

func TestTTSTextNormalizerBoundsUnfinishedParenthesis(t *testing.T) {
	var normalizer ttsTextNormalizer
	normalizer.WriteString("(")
	for range 4096 {
		normalizer.WriteString(" ")
		if len(normalizer.pending) > len("https://") || len(normalizer.space) > ttsPendingSpaceBytes || len(normalizer.bracketSpace) > ttsPendingSpaceBytes {
			t.Fatal("unfinished parenthesis retained unbounded whitespace")
		}
	}
	if got := normalizer.WriteString("正文") + normalizer.Finish(); !strings.HasSuffix(got, "正文") {
		t.Fatalf("normalization did not resume after the bounded prefix: %q", got)
	}
}
