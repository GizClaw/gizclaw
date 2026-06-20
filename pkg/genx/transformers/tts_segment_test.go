package transformers

import (
	"reflect"
	"testing"
)

func TestTTSSentenceSegmenterSplitsOnSemanticBoundaries(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString("你好，我的朋友。3.14 是一个小数，10:15 是时间")

	got := segmenter.Segments(false)
	want := []string{"你好，我的朋友。3.14 是一个小数，"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(false) = %#v, want %#v", got, want)
	}

	got = segmenter.Segments(true)
	want = []string{"10:15 是时间"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(true) = %#v, want %#v", got, want)
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

func TestTTSSentenceSegmenterWaitsForReadableFirstSegment(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString("好的，")

	got := segmenter.Segments(false)
	if len(got) != 0 {
		t.Fatalf("Segments(false) = %#v, want no short first segment", got)
	}

	segmenter.WriteString("我来讲一个。")
	got = segmenter.Segments(false)
	want := []string{"好的，我来讲一个。"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(false) = %#v, want %#v", got, want)
	}
}

func TestTTSSentenceSegmenterKeepsTailAfterLongestSegment(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString("嘘，都别出声！这")

	got := segmenter.Segments(false)
	want := []string{"嘘，都别出声！"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(false) = %#v, want %#v", got, want)
	}

	got = segmenter.Segments(true)
	want = []string{"这"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Segments(true) = %#v, want %#v", got, want)
	}
}

func TestTTSSentenceSegmenterFindsLongestWeakBoundary(t *testing.T) {
	segmenter := newTTSSentenceSegmenter(256)
	segmenter.WriteString("山雨斜斜打在破庙门前，悟空抬头，后面")

	got := segmenter.Segments(false)
	want := []string{"山雨斜斜打在破庙门前，悟空抬头，"}
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
