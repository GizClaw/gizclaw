package audiodock

import (
	"strings"
	"unicode/utf8"
)

// speakerParser retains only a prefix of a configured marker. All other text
// can pass through immediately, including unmatched and incomplete brackets.
// Its state belongs exclusively to the model reader for one response.
type speakerParser struct {
	pending string
	pattern string
	segment int
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
		if len(result) > 0 && result[len(result)-1].segment == p.segment {
			result[len(result)-1].text += text
		} else {
			result = append(result, speakerText{text: text, pattern: p.pattern, segment: p.segment})
		}
	}
	text = p.pending + text
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
		if p.pattern != "" {
			p.segment++
			p.pattern = ""
		}
		emit("【")
		text = text[len("【"):]
	}
	return result
}
