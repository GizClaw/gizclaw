package streamkit

import (
	"reflect"
	"slices"
	"testing"
)

func TestTTSSentenceSegmenterSplitsOnSemanticBoundaries(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString("你好，我的朋友。3.14 是一个小数，10:15 是时间")

	got := segmenter.Segments(false)
	want := []string{"你好，", "我的朋友。", "3.14 是一个小数，"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(false) = %#v, want %#v", got, want)
	}

	got = segmenter.Segments(true)
	want = []string{"10:15 是时间"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(true) = %#v, want %#v", got, want)
	}
}

func TestTTSSentenceSegmenterKeepsNumbersAcrossChunks(t *testing.T) {
	for _, number := range []string{"1,000", "1,000,000.25", "3.14", "10:15", "10：15", "１,０００"} {
		t.Run(number, func(t *testing.T) {
			text := "这件商品的数值是" + number + "。"
			runes := []rune(text)
			for split := range len(runes) + 1 {
				segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
				segmenter.WriteString(string(runes[:split]))
				got := segmenter.Segments(false)
				segmenter.WriteString(string(runes[split:]))
				got = append(got, segmenter.Segments(false)...)
				got = append(got, segmenter.Segments(true)...)
				if want := []string{text}; !slices.Equal(got, want) {
					t.Errorf("chunk split at rune %d: segments = %#v, want %#v", split, got, want)
				}
			}
		})
	}
}

func TestTTSSentenceSegmenterResolvesTrailingNumericPunctuation(t *testing.T) {
	for _, punctuation := range []string{",", ".", ":", "："} {
		t.Run(punctuation, func(t *testing.T) {
			prefix := "这次的计算结果是1" + punctuation
			for _, final := range []bool{false, true} {
				segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
				segmenter.WriteString(prefix)
				if got := segmenter.Segments(false); len(got) != 0 {
					t.Fatalf("pending punctuation: segments = %#v, want none", got)
				}
				if !final {
					segmenter.WriteString("接着继续讲")
				}
				if got, want := segmenter.Segments(final), []string{prefix}; !slices.Equal(got, want) {
					t.Fatalf("final=%t: segments = %#v, want %#v", final, got, want)
				}
				var want []string
				if !final {
					want = []string{"接着继续讲"}
				}
				if got := segmenter.Segments(true); !slices.Equal(got, want) {
					t.Fatalf("final=%t: remaining segments = %#v, want %#v", final, got, want)
				}
			}
		})
	}
}

func TestTTSSentenceSegmenterSplitsAtMaxRunes(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(3)
	segmenter.WriteString("一二三四五")

	got := segmenter.Segments(false)
	want := []string{"一二三"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(false) = %#v, want %#v", got, want)
	}

	got = segmenter.Segments(true)
	want = []string{"四五"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(true) = %#v, want %#v", got, want)
	}
}

func TestTTSSentenceSegmenterRetainsUnconfirmedSentenceTail(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString("好的，")

	got := segmenter.Segments(false)
	if len(got) != 0 {
		t.Fatalf("Segments(false) = %#v, want no short first segment", got)
	}

	segmenter.WriteString("我来讲一个。")
	if got := segmenter.Segments(false); !slices.Equal(got, []string{"好的，"}) {
		t.Fatalf("Segments(false) = %#v, want the confirmed comma phrase", got)
	}
	segmenter.WriteString("后")
	got = segmenter.Segments(false)
	want := []string{"我来讲一个。"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(false) = %#v, want %#v", got, want)
	}
	if got := segmenter.Segments(true); !slices.Equal(got, []string{"后"}) {
		t.Fatalf("Segments(true) = %#v, want the lookahead text", got)
	}
}

func TestTTSSentenceSegmenterKeepsUnconfirmedTail(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString("嘘，都别出声！这")

	got := segmenter.Segments(false)
	want := []string{"嘘，", "都别出声！"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(false) = %#v, want %#v", got, want)
	}

	got = segmenter.Segments(true)
	want = []string{"这"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(true) = %#v, want %#v", got, want)
	}
}

func TestTTSSentenceSegmenterReturnsConfirmedWeakBoundaries(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString("山雨斜斜打在破庙门前，悟空抬头，后面")

	got := segmenter.Segments(false)
	want := []string{"山雨斜斜打在破庙门前，", "悟空抬头，"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(false) = %#v, want %#v", got, want)
	}

	got = segmenter.Segments(true)
	want = []string{"后面"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(true) = %#v, want %#v", got, want)
	}
}

func TestTTSSentenceSegmenterNormalizesSpokenText(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString(`<node id="answer" />我没办法实时更新天气信息哦（https://example.com/weather）。`)

	got := segmenter.Segments(true)
	want := []string{"我没办法实时更新天气信息哦。"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(true) = %#v, want %#v", got, want)
	}
}
