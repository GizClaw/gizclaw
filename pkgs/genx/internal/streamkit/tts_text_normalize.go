package streamkit

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	ttsTagBufferBytes    = 1024
	ttsXMLMaxDepth       = 64
	ttsPendingSpaceBytes = 1024
)

// ttsTextNormalizer owns lexical state for one message. It only publishes text
// once it is known not to belong to a URL, XML control block or pending space.
type ttsTextNormalizer struct {
	preserveNewlines bool
	pending          string
	space            string
	started          bool
	last             rune
	inURL            bool
	urlBracket       bool
	bracket          rune
	bracketSpace     string
	inTag            bool
	tag              []byte
	tagQuote         byte
	tagLast          byte
	xml              []string
	xmlOverflow      int
}

func normalizeTTSSpokenText(text string) string {
	var normalizer ttsTextNormalizer
	return normalizer.WriteString(text) + normalizer.Finish()
}

func (n *ttsTextNormalizer) WriteString(text string) string {
	n.pending += text
	return n.take(false)
}

func (n *ttsTextNormalizer) Finish() string {
	text := n.take(true)
	*n = ttsTextNormalizer{preserveNewlines: n.preserveNewlines}
	return text
}

func (n *ttsTextNormalizer) take(final bool) string {
	var out strings.Builder
	for n.pending != "" {
		if n.inTag {
			if !n.consumeTag() {
				break
			}
			continue
		}
		if !utf8.FullRuneInString(n.pending) && !final {
			break
		}
		r, width := utf8.DecodeRuneInString(n.pending)
		if n.inURL {
			if n.urlBracket {
				n.pending = n.pending[width:]
				if r == ')' || r == '）' {
					n.inURL, n.urlBracket = false, false
				}
				continue
			}
			if ttsURLDelimiter(r) {
				n.inURL = false
				continue
			}
			n.pending = n.pending[width:]
			continue
		}
		if n.bracket != 0 {
			if unicode.IsSpace(r) {
				n.bracketSpace = ttsAppendSpace(n.bracketSpace, r)
				n.pending = n.pending[width:]
				continue
			}
			if r == ')' || r == '）' {
				n.bracket, n.bracketSpace = 0, ""
				n.pending = n.pending[width:]
				continue
			}
			url, more := ttsURLPrefix(n.pending)
			if url {
				n.bracket, n.bracketSpace = 0, ""
				n.inURL, n.urlBracket = true, true
				continue
			}
			if more && !final {
				break
			}
			n.emitRune(&out, n.bracket)
			for _, space := range n.bracketSpace {
				n.emitRune(&out, space)
			}
			n.bracket, n.bracketSpace = 0, ""
			continue
		}
		if r == '<' {
			start := 1
			if len(n.pending) > start && n.pending[start] == '/' {
				start++
			}
			if len(n.pending) == start && !final {
				break
			}
			if len(n.pending) > start && ttsASCIIAlpha(n.pending[start]) {
				n.inTag = true
				n.tag = n.tag[:0]
				n.tagQuote, n.tagLast = 0, 0
				continue
			}
		}
		if len(n.xml) != 0 || n.xmlOverflow != 0 {
			n.pending = n.pending[width:]
			continue
		}
		if r == '(' || r == '（' {
			n.bracket = r
			n.pending = n.pending[width:]
			continue
		}
		if n.space != "" || !ttsASCIIWord(n.last) {
			url, more := ttsURLPrefix(n.pending)
			if url {
				n.inURL = true
				continue
			}
			if more && !final {
				break
			}
		}
		n.emitRune(&out, r)
		n.pending = n.pending[width:]
	}
	if final && n.bracket != 0 {
		n.emitRune(&out, n.bracket)
		n.bracket, n.bracketSpace = 0, ""
	}
	return out.String()
}

func (n *ttsTextNormalizer) emitRune(out *strings.Builder, r rune) {
	if n.preserveNewlines && (r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029') {
		if n.started && !(r == '\n' && n.last == '\r') {
			out.WriteByte('\n')
		}
		n.space, n.last = "", r
		return
	}
	if unicode.IsSpace(r) {
		if n.started {
			n.space = ttsAppendSpace(n.space, r)
		}
		return
	}
	if strings.ContainsRune("，。？！：；、", r) {
		n.space = strings.TrimSuffix(n.space, " ")
	}
	out.WriteString(n.space)
	n.space = ""
	out.WriteRune(r)
	n.started, n.last = true, r
}

func ttsAppendSpace(space string, r rune) string {
	if strings.ContainsRune(" \t\r\n", r) {
		r = ' '
		if strings.HasSuffix(space, " ") {
			return space
		}
	}
	if len(space)+utf8.RuneLen(r) > ttsPendingSpaceBytes {
		return space
	}
	return space + string(r)
}

// Attributes can span chunks and contain quoted '>'. Only a bounded prefix is
// retained for the element name; the rest is discarded while tracking quotes.
func (n *ttsTextNormalizer) consumeTag() bool {
	for index := range len(n.pending) {
		b := n.pending[index]
		if len(n.tag) < ttsTagBufferBytes {
			n.tag = append(n.tag, b)
		}
		if n.tagQuote != 0 {
			if b == n.tagQuote {
				n.tagQuote = 0
			}
			continue
		}
		if b == '>' {
			n.finishTag()
			n.pending = n.pending[index+1:]
			n.inTag = false
			return true
		}
		if b == '\'' || b == '"' {
			n.tagQuote = b
		}
		if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
			n.tagLast = b
		}
	}
	n.pending = ""
	return false
}

func (n *ttsTextNormalizer) finishTag() {
	start := 1
	closing := len(n.tag) > start && n.tag[start] == '/'
	if closing {
		start++
	}
	end := start
	for end < len(n.tag) && (ttsASCIIWord(rune(n.tag[end])) || strings.ContainsRune(":-", rune(n.tag[end]))) {
		end++
	}
	name := strings.ToLower(string(n.tag[start:end]))
	if closing {
		if n.xmlOverflow > 0 {
			n.xmlOverflow--
		} else if len(n.xml) > 0 && n.xml[len(n.xml)-1] == name {
			n.xml = n.xml[:len(n.xml)-1]
		}
	} else if n.tagLast != '/' {
		if len(n.xml) >= ttsXMLMaxDepth {
			n.xmlOverflow++
		} else {
			n.xml = append(n.xml, name)
		}
	}
}

func ttsURLPrefix(text string) (url, more bool) {
	for _, prefix := range []string{"https://", "http://", "www."} {
		if len(text) >= len(prefix) {
			if strings.EqualFold(text[:len(prefix)], prefix) {
				return true, false
			}
		} else if strings.EqualFold(text, prefix[:len(text)]) {
			more = true
		}
	}
	return false, more
}

func ttsURLDelimiter(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("<>\"'，。！？；、）)]}", r)
}

func ttsASCIIAlpha(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func ttsASCIIWord(r rune) bool {
	return r < utf8.RuneSelf && (ttsASCIIAlpha(byte(r)) || r >= '0' && r <= '9' || r == '_')
}
