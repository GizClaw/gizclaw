package audiodock

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// speakerParser retains prefixes of configured markers. When explicitly enabled,
// it also recognizes bounded unknown markers at paragraph starts. Other text
// passes through immediately; incomplete markers flush literally on EOS.
// Its state belongs exclusively to the model reader for one response, or one
// publisher within a response when no configured speaker segmentation is used.
type speakerParser struct {
	pending string
	history strings.Builder
	pattern string
	segment int
	// normalizeUnknown is opt-in; the zero value preserves existing parsing.
	normalizeUnknown bool
	lineHasText      bool
}

type speakerText struct {
	text    string
	pattern string
	segment int
}

func (p *speakerParser) feed(text string, final bool, voices map[string]string) []speakerText {
	var result []speakerText
	emit := func(text string) {
		if text == "" {
			return
		}
		if p.normalizeUnknown {
			for _, r := range text {
				if r == '\n' || r == '\r' {
					p.lineHasText = false
				} else if !unicode.IsSpace(r) {
					p.lineHasText = true
				}
			}
		}
		if len(result) > 0 && result[len(result)-1].segment == p.segment {
			result[len(result)-1].text += text
		} else {
			result = append(result, speakerText{text: text, pattern: p.pattern, segment: p.segment})
		}
	}
	text = p.pending + text
	original := text
	defer func() { p.history.WriteString(original[:len(original)-len(p.pending)]) }()
	p.pending = ""
	for text != "" {
		start := strings.Index(text, "【")
		if start < 0 {
			if !final {
				for i := max(0, len(text)-utf8.UTFMax+1); i < len(text); i++ {
					if !utf8.FullRuneInString(text[i:]) {
						p.pending = text[i:]
						text = text[:i]
						break
					}
				}
			}
			emit(text)
			break
		}
		emit(text[:start])
		text = text[start:]
		matched, prefix := "", false
		for name := range voices {
			marker := "【" + name + "】"
			if strings.HasPrefix(text, marker) {
				matched = name
				break
			}
			if strings.HasPrefix(marker, text) {
				prefix = true
			}
		}
		if matched != "" {
			pattern := voices[matched]
			if p.pattern != pattern {
				p.segment++
				p.pattern = pattern
			}
			text = text[len("【"+matched+"】"):]
			continue
		}
		if prefix && !final {
			p.pending = text
			break
		}
		if p.normalizeUnknown && !p.lineHasText {
			length, incomplete := unknownSpeakerMarker(text)
			if length != 0 {
				if p.pattern != "" {
					p.segment++
					p.pattern = ""
				}
				text = text[length:]
				continue
			}
			if incomplete && !final {
				p.pending = text
				break
			}
		}
		if p.pattern != "" {
			p.segment++
			p.pattern = ""
		}
		emit("【")
		text = text[len("【"):]
	}
	return result
}

// The opt-in syntax reserves paragraph-leading 【name】 for speakers. Thirty-two
// code points bound an unknown name to at most 128 UTF-8 bytes. Known names keep
// their configured semantics, including names longer than this unknown bound.
const maxUnknownSpeakerRunes = 32

// unknownSpeakerMarker examines at most maxUnknownSpeakerRunes+1 code points.
// A nonzero length is a complete marker; incomplete retains a possible prefix.
// Empty labels, punctuation used in prose, nesting, and line breaks are literal.
func unknownSpeakerMarker(text string) (length int, incomplete bool) {
	nameRunes, hasName := 0, false
	for offset := len("【"); offset < len(text); {
		if !utf8.FullRuneInString(text[offset:]) {
			return 0, true
		}
		r, size := utf8.DecodeRuneInString(text[offset:])
		if r == utf8.RuneError && size == 1 {
			return 0, false
		}
		if r == '】' {
			if hasName {
				return offset + size, false
			}
			return 0, false
		}
		if nameRunes == maxUnknownSpeakerRunes || !unknownSpeakerRune(r) {
			return 0, false
		}
		nameRunes++
		hasName = hasName || r != ' '
		offset += size
	}
	return 0, true
}

func unknownSpeakerRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) ||
		unicode.IsSymbol(r) || unicode.Is(unicode.Pi, r) || unicode.Is(unicode.Pf, r) {
		return true
	}
	switch r {
	case ' ', '-', '_', '.', '·', '\u200d', '\'', '"':
		return true
	}
	return false
}

// takeHistoryText releases only text whose presentation has been emitted. A
// split or marker-only prefix stays buffered until actual text is available.
func (p *speakerParser) takeHistoryText() genx.Text {
	text := genx.Text(p.history.String())
	p.history.Reset()
	return text
}
