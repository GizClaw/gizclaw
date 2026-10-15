package streamkit

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

type ttsSegmentationSample struct {
	name      string
	input     string
	spoken    string
	protected []string
}

// These samples specify speech quality targets independently of the current
// punctuation rules. Segments may be combined differently for each chunking;
// their text must be preserved and boundaries must not split protected spans.
// The length samples require complete words and graphemes around the hard rune
// limit, without silently relaxing that limit to preserve protected spans.
func ttsSegmentationSamples() []ttsSegmentationSample {
	plain := func(name, text string, protected ...string) ttsSegmentationSample {
		return ttsSegmentationSample{name: name, input: text, spoken: text, protected: protected}
	}
	samples := []ttsSegmentationSample{
		plain("numbers/grouped", "这一笔金额是1,000元。", "1,000"),
		plain("numbers/multiple_groups", "账户余额共有1,000,000元。", "1,000,000"),
		plain("numbers/grouped_decimal", "这一笔金额是1,234.56元。", "1,234.56"),
		plain("numbers/negative_decimal", "当前温度降低到-12.5度。", "-12.5"),
		plain("numbers/leading_decimal", "这一项系数等于0.001。", "0.001"),
		plain("numbers/percentage", "这个月的增长率是12.5%。", "12.5%"),
		plain("numbers/clock", "这次会议安排在10:15开始。", "10:15"),
		plain("numbers/clock_seconds", "记录的准确时间是10:15:30。", "10:15:30"),
		plain("numbers/fullwidth_colon", "这次会议安排在10：15开始。", "10：15"),
		plain("numbers/fullwidth_digits", "这一笔金额是１,０００元。", "１,０００"),
		plain("numbers/arabic_digits", "这一笔金额是١٬٠٠٠元。", "١٬٠٠٠"),
		plain("numbers/narrow_space", "这一笔金额是1\u202f000元。", "1\u202f000"),
		plain("numbers/scientific", "这一项结果等于6.02e23。", "6.02e23"),
		plain("numbers/date", "今天的日期是2026-10-14。", "2026-10-14"),
		plain("numbers/version", "当前使用的版本是v1.2.3。", "v1.2.3"),
		plain("numbers/ipv4", "服务器地址为192.168.1.1。", "192.168.1.1"),
		plain("numbers/ipv6", "服务器地址为2001:db8::1。", "2001:db8::1"),
		plain("words/english_space", "Please keep these words separate.", "these words"),
		plain("words/mixed_language", "请使用Go语言和OpenAI服务。", "OpenAI"),
		plain("words/contraction", "Please don't stop speaking.", "don't"),
		plain("words/hyphenated", "Please check the state-of-the-art system.", "state-of-the-art"),
		plain("words/title", "Please ask Dr. Smith today.", "Dr. Smith"),
		plain("words/initials", "Please ask J. R. R. Tolkien today.", "J. R. R. Tolkien"),
		plain("words/acronym", "Please read the U.S. policy carefully.", "U.S."),
		plain("words/latin_abbreviation", "Please use e.g. apples today.", "e.g."),
		plain("words/hostname", "请记住域名example.com。", "example.com"),
		plain("words/email", "请联系用户alice@example.com。", "alice@example.com"),
		plain("punctuation/chinese_quote", "他说：“这件事已经完成。”我们明天再聊。", "完成。”"),
		plain("punctuation/english_quote", "She said, \"Please wait here.\" Then left.", "here.\""),
		plain("punctuation/parenthesis", "解释如下（这件事已经完成。）我们明天再聊。", "完成。）"),
		plain("punctuation/chinese_ellipsis", "他说这件事情还需要等待……然后离开。", "……"),
		plain("punctuation/ascii_ellipsis", "Please wait a moment... then continue.", "..."),
		plain("punctuation/repeated_terminator", "这件事情真的已经完成了吗？！我们再确认。", "？！"),
		plain("punctuation/chinese_comma", "先把这件事情完成，再继续下一件事情。"),
		plain("punctuation/chinese_list", "需要准备苹果、香蕉和梨。"),
		plain("unicode/combining_mark", "请读出这个组合字符e\u0301。", "e\u0301"),
		plain("unicode/zwj_emoji", "今天介绍的职业是👩‍💻。", "👩‍💻"),
		plain("unicode/flag", "今天展示的旗帜是🇸🇬。", "🇸🇬"),
		plain("unicode/skin_tone", "我为这个好消息鼓掌👍🏽。", "👍🏽"),
		plain("unicode/japanese", "今日はいい天気です。明日も晴れです。"),
		plain("unicode/korean", "오늘은 좋은 날입니다. 내일 만나요."),
		plain("unicode/hindi", "आज मौसम अच्छा है। कल फिर मिलेंगे।"),
		plain("unicode/arabic", "هذا اختبار جديد؟ نعم بالتأكيد."),
		plain("normalization/non_tag_angle", "数学关系是3<5，6>4。"),
		{name: "normalization/repeated_spaces", input: "Please   keep\tthese\nwords together.", spoken: "Please keep these words together.", protected: []string{"these words"}},
		{name: "normalization/outer_space", input: "  这是一段完整的说明。  ", spoken: "这是一段完整的说明。"},
		{name: "normalization/self_closing_tag", input: `<node id="answer"/>这是可朗读的正文。`, spoken: "这是可朗读的正文。"},
		{name: "normalization/control_block", input: `<node id="tool_call">内部控制内容。不要朗读。</node>这是可朗读的正文。`, spoken: "这是可朗读的正文。"},
		{name: "normalization/nested_control", input: `前面的正文。<node><block>内部控制内容。</block></node>后面的正文。`, spoken: "前面的正文。后面的正文。"},
		{name: "normalization/url", input: "链接是https://example.com/a?b=1。接着继续说明。", spoken: "链接是。接着继续说明。"},
		{name: "normalization/bracket_url", input: "请看（https://example.com/a?b=1），接着继续说明。", spoken: "请看，接着继续说明。"},
		{name: "normalization/www_url", input: "链接是www.example.com/path。接着继续说明。", spoken: "链接是。接着继续说明。"},
		{name: "normalization/only_control", input: `<node id="tool_call"><function>内部控制内容。</function></node>`, spoken: ""},
		{name: "normalization/empty", input: "", spoken: ""},
		{name: "normalization/only_space", input: " \t\r\n ", spoken: ""},
		plain("length/exact_limit", strings.Repeat("甲", 80)),
		plain("length/over_limit", strings.Repeat("甲", 161)),
		plain("length/long_numeric_token", strings.Repeat("1", 161)),
		plain("length/grouped_at_limit", strings.Repeat("甲", 78)+"1,000元。", "1,000"),
		plain("length/decimal_at_limit", strings.Repeat("甲", 78)+"3.14元。", "3.14"),
		plain("length/word_at_limit", strings.Repeat("甲", 78)+"OpenAI服务。", "OpenAI"),
		plain("length/combining_at_limit", strings.Repeat("甲", 79)+"e\u0301。", "e\u0301"),
		plain("length/emoji_at_limit", strings.Repeat("甲", 79)+"👩‍💻。", "👩‍💻"),
		plain("length/flag_at_limit", strings.Repeat("甲", 79)+"🇸🇬。", "🇸🇬"),
	}
	return append(samples, ttsMultilingualSegmentationSamples()...)
}

type ttsChunking struct {
	name   string
	chunks []string
}

func ttsSegmentationChunkings(text string) []ttsChunking {
	runes := []rune(text)
	chunkings := []ttsChunking{{name: "whole", chunks: []string{text}}}
	for _, width := range []int{1, 2, 3, 7, 16} {
		var chunks []string
		for start := 0; start < len(runes); start += width {
			chunks = append(chunks, string(runes[start:min(start+width, len(runes))]))
		}
		chunkings = append(chunkings, ttsChunking{name: fmt.Sprintf("width_%d", width), chunks: chunks})
	}
	for split := 1; split < len(runes); split++ {
		chunkings = append(chunkings, ttsChunking{
			name:   fmt.Sprintf("split_%03d", split),
			chunks: []string{string(runes[:split]), string(runes[split:])},
		})
	}
	for seed := range 4 {
		chunkings = append(chunkings, ttsChunking{
			name:   fmt.Sprintf("mixed_%d", seed),
			chunks: ttsMixedChunks(text, uint64(seed)),
		})
	}
	// Transport chunks can also split the UTF-8 encoding of a single rune.
	byteChunks := make([]string, len(text))
	for index := range len(text) {
		byteChunks[index] = text[index : index+1]
	}
	chunkings = append(chunkings, ttsChunking{name: "bytewise", chunks: byteChunks})
	return chunkings
}

func ttsMixedChunks(text string, seed uint64) []string {
	random := rand.New(rand.NewPCG(seed, 17))
	runes := []rune(text)
	var chunks []string
	for start := 0; start < len(runes); {
		end := min(start+1+random.IntN(11), len(runes))
		chunks = append(chunks, string(runes[start:end]))
		start = end
	}
	return chunks
}

func ttsCollectSegments(chunks []string) []string {
	segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
	var segments []string
	for _, chunk := range chunks {
		segmenter.WriteString(chunk)
		segments = append(segments, segmenter.Segments(false)...)
	}
	return append(segments, segmenter.Segments(true)...)
}

func ttsCheckSegments(t *testing.T, segments []string, spoken string, protected []string, limits ...int) {
	t.Helper()
	limit := defaultTTSSegmentMaxRunes
	if len(limits) != 0 {
		limit = limits[0]
	}
	for index, segment := range segments {
		if segment == "" || !utf8.ValidString(segment) || utf8.RuneCountInString(segment) > limit {
			t.Errorf("invalid segment %d: %q", index, segment)
		}
	}
	if got := strings.Join(segments, ""); got != spoken {
		t.Errorf("spoken text changed: got %q, want %q; segments=%q", got, spoken, segments)
		return
	}
	boundary := 0
	for _, segment := range segments {
		boundary += len(segment)
		for _, token := range protected {
			for start := 0; start < len(spoken); {
				index := strings.Index(spoken[start:], token)
				if index < 0 {
					break
				}
				index += start
				if index < boundary && boundary < index+len(token) {
					t.Errorf("boundary at byte %d splits protected span %q; segments=%q", boundary, token, segments)
				}
				start = index + len(token)
			}
		}
	}
}

func TestTTSSentenceSegmenterQualityCorpus(t *testing.T) {
	for _, sample := range ttsSegmentationSamples() {
		t.Run(sample.name, func(t *testing.T) {
			for _, token := range sample.protected {
				if token == "" || !strings.Contains(sample.spoken, token) {
					t.Fatalf("invalid corpus: protected span %q is absent from expected text", token)
				}
			}
			for _, chunking := range ttsSegmentationChunkings(sample.input) {
				t.Run(chunking.name, func(t *testing.T) {
					ttsCheckSegments(t, ttsCollectSegments(chunking.chunks), sample.spoken, sample.protected)
				})
			}
		})
	}
}

func TestTTSSentenceSegmenterCommitsCompletePhrase(t *testing.T) {
	for _, phrase := range []string{
		"这是一个已经完成的句子。",
		"Please wait here.",
		"他说：“现在已经完成了。”",
		"今日はいい天気です。",
		"आज मौसम अच्छा है।",
		"هذا اختبار جديد؟",
	} {
		t.Run(phrase, func(t *testing.T) {
			segmenter := newTTSSentenceSegmenter(defaultTTSSegmentMaxRunes)
			segmenter.WriteString(phrase + "后")
			if got := strings.Join(segmenter.Segments(false), ""); got != phrase {
				t.Fatalf("complete phrase before EOS: got %q, want %q", got, phrase)
			}
			if got := strings.Join(segmenter.Segments(true), ""); got != "后" {
				t.Fatalf("remaining text: got %q, want %q", got, "后")
			}
			if got := segmenter.Segments(true); len(got) != 0 {
				t.Fatalf("repeated flush duplicated text: %q", got)
			}
		})
	}
}

func FuzzTTSSentenceSegmenterPlainText(f *testing.F) {
	for _, text := range []string{
		"这一笔金额是1,000元。",
		"账户余额共有1,234,567.89元。",
		"这次会议安排在10:15:30开始。",
		"今天的日期是2026-10-14。",
		"今天介绍的职业是工程师。",
		strings.Repeat("甲", 161),
	} {
		f.Add(text, uint64(0))
	}
	f.Fuzz(func(t *testing.T, text string, seed uint64) {
		if len(text) > 1024 || !utf8.ValidString(text) {
			t.Skip()
		}
		for _, r := range text {
			if !unicode.IsLetter(r) && !unicode.IsNumber(r) && !strings.ContainsRune(".,:：，；。？！…～!?;-", r) {
				t.Skip()
			}
		}
		if strings.Contains(strings.ToLower(text), "www.") {
			t.Skip()
		}
		ttsCheckSegments(t, ttsCollectSegments(ttsMixedChunks(text, seed)), text, nil)
	})
}
