package audiodock

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

type speakerRun struct{ text, pattern string }

func parserRuns(parts []speakerText) []speakerRun {
	var result []speakerRun
	for _, part := range parts {
		if len(result) > 0 && result[len(result)-1].pattern == part.pattern {
			result[len(result)-1].text += part.text
		} else {
			result = append(result, speakerRun{part.text, part.pattern})
		}
	}
	return result
}

func TestUnknownSpeakerParser(t *testing.T) {
	voices := map[string]string{"旁白": "narrator", "狐": "fox", strings.Repeat("长", 40): "long", strings.Repeat("🦊", 40): "longemoji"}
	cases := []struct {
		name, input string
		want        []speakerRun
	}{
		{"unknown first", "【陌生人】你好", []speakerRun{{"你好", ""}}},
		{"known unknown known", "【狐】你好\n【陌生人】见面\n【旁白】故事", []speakerRun{{"你好\n", "fox"}, {"见面\n", ""}, {"故事", "narrator"}}},
		{"inline annotation", "【狐】正文【注释】原文", []speakerRun{{"正文", "fox"}, {"【注释】原文", ""}}},
		{"paragraph indentation", "\n\t 【陌生人】你好", []speakerRun{{"\n\t 你好", ""}}},
		{"CRLF", "【狐】甲\r\n【陌生人】乙", []speakerRun{{"甲\r\n", "fox"}, {"乙", ""}}},
		{"punctuated annotation", "【注释：请注意】原文", []speakerRun{{"【注释：请注意】原文", ""}}},
		{"ordinary brackets", "(甲)[乙]《丙》〔丁〕（戊）", []speakerRun{{"(甲)[乙]《丙》〔丁〕（戊）", ""}}},
		{"empty", "【】原文", []speakerRun{{"【】原文", ""}}},
		{"blank", "【   】原文", []speakerRun{{"【   】原文", ""}}},
		{"incomplete", "【狐】甲\n【陌生", []speakerRun{{"甲\n", "fox"}, {"【陌生", ""}}},
		{"unknown interrupted by newline", "【陌\n生】原文", []speakerRun{{"【陌\n生】原文", ""}}},
		{"nested", "【外【陌生】原文", []speakerRun{{"【外【陌生】原文", ""}}},
		{"adjacent unknown known", "【陌生人】【狐】甲", []speakerRun{{"甲", "fox"}}},
		{"adjacent known unknown", "【狐】【陌生人】甲", []speakerRun{{"甲", ""}}},
		{"markers without text", "【狐】【陌生人】【旁白】", nil},
		{"Unicode and spaces", "【Dr. Élodie 猫👩‍🚀】Hello", []speakerRun{{"Hello", ""}}},
		{"quoted annotation", "【伙伴替“甲”说话】你好", []speakerRun{{"你好", ""}}},
		{"combining", "【A\u0308猫】Hello", []speakerRun{{"Hello", ""}}},
		{"max unknown", "【" + strings.Repeat("猫", 32) + "】甲", []speakerRun{{"甲", ""}}},
		{"overlong unknown", "【" + strings.Repeat("猫", 33) + "】甲", []speakerRun{{"【" + strings.Repeat("猫", 33) + "】甲", ""}}},
		{"configured above unknown byte bound", "【" + strings.Repeat("🦊", 40) + "】甲", []speakerRun{{"甲", "longemoji"}}},
		{"long configured", "【" + strings.Repeat("长", 40) + "】甲", []speakerRun{{"甲", "long"}}},
		{"whitespace EOS", " \t\n\u3000", []speakerRun{{" \t\n\u3000", ""}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for split := 0; split <= len(tc.input); split++ {
				p := speakerParser{normalizeUnknown: true}
				parts := p.feed(tc.input[:split], false, voices)
				var canonical strings.Builder
				if len(parts) != 0 {
					canonical.WriteString(string(p.takeHistoryText()))
				}
				last := p.feed(tc.input[split:], true, voices)
				if len(last) != 0 {
					canonical.WriteString(string(p.takeHistoryText()))
				}
				parts = append(parts, last...)
				wantHistory := tc.input
				if tc.want == nil {
					wantHistory = ""
				}
				if canonical.String() != wantHistory {
					t.Fatalf("split %d: semantic history=%q want %q", split, canonical.String(), wantHistory)
				}
				if !slices.Equal(parserRuns(parts), tc.want) {
					t.Fatalf("split %d: got %#v want %#v", split, parserRuns(parts), tc.want)
				}
				if p.pending != "" {
					t.Fatalf("pending after EOS %q", p.pending)
				}
			}
			// Every byte as an independent chunk stresses split Unicode and labels.
			p := speakerParser{normalizeUnknown: true}
			var parts []speakerText
			var canonical strings.Builder
			for i := 0; i < len(tc.input); i++ {
				next := p.feed(tc.input[i:i+1], false, voices)
				if len(next) != 0 {
					canonical.WriteString(string(p.takeHistoryText()))
				}
				parts = append(parts, next...)
			}
			last := p.feed("", true, voices)
			if len(last) != 0 {
				canonical.WriteString(string(p.takeHistoryText()))
			}
			parts = append(parts, last...)
			wantHistory := tc.input
			if tc.want == nil {
				wantHistory = ""
			}
			if canonical.String() != wantHistory {
				t.Fatalf("byte chunks: semantic history=%q want %q", canonical.String(), wantHistory)
			}
			if !slices.Equal(parserRuns(parts), tc.want) {
				t.Fatalf("byte chunks: got %#v want %#v", parserRuns(parts), tc.want)
			}
			for _, part := range parts {
				if !utf8.ValidString(part.text) {
					t.Fatalf("invalid UTF-8 in emitted chunk %q", part.text)
				}
			}
		})
	}
}

func TestUnknownSpeakerPromptPolicyIsOptIn(t *testing.T) {
	p := speakerParser{}
	parts := p.feed("【陌生人】你好", false, nil)
	if got := parserRuns(parts); len(got) != 1 || got[0].text != "【陌生人】你好" {
		t.Fatalf("zero-value changed: %#v", got)
	}
}

func TestUnknownSpeakerPrefixHasBoundedLookahead(t *testing.T) {
	p := speakerParser{normalizeUnknown: true}
	if got := p.feed("before\n", false, nil); len(got) != 1 || got[0].text != "before\n" {
		t.Fatalf("ordinary prefix was buffered: %#v", got)
	}
	if got := p.feed("【"+strings.Repeat("🦊", maxUnknownSpeakerRunes), false, nil); len(got) != 0 {
		t.Fatalf("possible marker prematurely emitted: %#v", got)
	}
	if len(p.pending) > len("【")+maxUnknownSpeakerRunes*utf8.UTFMax {
		t.Fatalf("pending exceeds bound %d", len(p.pending))
	}
	got := p.feed("🦊", false, nil)
	if len(got) != 1 || got[0].text != "【"+strings.Repeat("🦊", maxUnknownSpeakerRunes+1) || p.pending != "" {
		t.Fatalf("overlong prefix did not flush before EOS: %#v pending=%q", got, p.pending)
	}
	got = p.feed(strings.Repeat("x", 1<<20), false, nil)
	if len(got) != 1 || len(got[0].text) != 1<<20 || p.pending != "" {
		t.Fatal("body was buffered")
	}
}

func TestUnknownSpeakerBodyEmitsBeforeEOS(t *testing.T) {
	p := speakerParser{normalizeUnknown: true}
	for _, part := range []string{"【", "陌", "生", "人", "】"} {
		if got := p.feed(part, false, nil); len(got) != 0 {
			t.Fatalf("marker leaked: %#v", got)
		}
	}
	got := p.feed("第一字", false, nil)
	if len(got) != 1 || got[0].text != "第一字" || got[0].pattern != "" {
		t.Fatalf("body waits for EOS: %#v", got)
	}
}

func FuzzUnknownSpeakerChunkBoundaries(f *testing.F) {
	for _, input := range []string{"【狐】你好\n【新猫】喵", "【注释：保留】正文", "【未知👩‍🚀】你好", "【", "正文【括号】\n【新狐】你好"} {
		f.Add(input, uint8(3))
	}
	f.Fuzz(func(t *testing.T, input string, width uint8) {
		if !utf8.ValidString(input) || len(input) > 1<<16 {
			t.Skip()
		}
		voices := map[string]string{"狐": "fox", "旁白": "narrator"}
		full := speakerParser{normalizeUnknown: true}
		want := parserRuns(full.feed(input, true, voices))
		p := speakerParser{normalizeUnknown: true}
		var parts []speakerText
		size := int(width)%31 + 1
		for start := 0; start < len(input); start += size {
			parts = append(parts, p.feed(input[start:min(start+size, len(input))], false, voices)...)
		}
		parts = append(parts, p.feed("", true, voices)...)
		if got := parserRuns(parts); !slices.Equal(got, want) {
			t.Fatalf("width %d got %#v want %#v", size, got, want)
		}
		if p.pending != "" {
			t.Fatal("pending after EOS")
		}
	})
}

func TestUnknownSpeakerInvalidUTF8IsLiteral(t *testing.T) {
	input := "【" + string([]byte{0xff}) + "】body"
	p := speakerParser{normalizeUnknown: true}
	var actual strings.Builder
	for _, part := range p.feed(input, true, nil) {
		actual.WriteString(part.text)
	}
	if actual.String() != input {
		t.Fatalf("invalid UTF-8 input was removed: %q", actual.String())
	}
}
