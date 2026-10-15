package streamkit

import (
	"slices"
	"testing"
)

func TestTTSSentenceSegmenterKeepsQuoteIntroducerWithQuotation(t *testing.T) {
	for _, sample := range []struct{ name, text, quotation string }{
		{"chinese", "“你好。”她说，“明天见。”", "她说，“明天见。”"},
		{"chinese_comma", "“今天先休息吧”，他说，“明天再继续。”", "他说，“明天再继续。”"},
		{"chinese_colon", "讲解到这里，他转过身说：“这就是答案。”然后离开。", "他转过身说：“这就是答案。”"},
		{"ascii", `"Hello." she said, "Goodbye."`, `she said, "Goodbye."`},
		{"german", "Er sagte, „Alles ist fertig.“ Danach ging er nach Hause.", "Er sagte, „Alles ist fertig.“"},
		{"french", "Elle a dit : « Tout est prêt. » Nous pouvons partir.", "Elle a dit : « Tout est prêt. »"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			for _, chunking := range ttsSegmentationChunkings(sample.text) {
				t.Run(chunking.name, func(t *testing.T) {
					ttsCheckSegments(t, ttsCollectSegments(chunking.chunks), sample.text, []string{sample.quotation})
				})
			}
		})
	}
}

func TestTTSSentenceSegmenterQuoteExample(t *testing.T) {
	var chunks []string
	for _, r := range "“你好。”她说，“明天见。”" {
		chunks = append(chunks, string(r))
	}
	got := ttsCollectSegments(chunks)
	want := []string{"“你好。”", "她说，“明天见。”"}
	if !slices.Equal(got, want) {
		t.Fatalf("quoted dialogue = %q, want %q", got, want)
	}
}
