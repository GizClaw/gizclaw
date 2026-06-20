package transformers

import "testing"

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
