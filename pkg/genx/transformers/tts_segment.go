package transformers

import (
	"unicode"
	"unicode/utf8"
)

type ttsSentenceSegmenter struct {
	buf                string
	maxRunesPerSegment int
	firstSegment       bool
}

func newTTSSentenceSegmenter(maxRunes int) *ttsSentenceSegmenter {
	if maxRunes <= 0 {
		maxRunes = defaultTTSSegmentMaxRunes
	}
	return &ttsSentenceSegmenter{
		maxRunesPerSegment: maxRunes,
		firstSegment:       true,
	}
}

func (s *ttsSentenceSegmenter) WriteString(text string) {
	s.buf += text
}

func (s *ttsSentenceSegmenter) Reset() {
	s.buf = ""
	s.firstSegment = true
}

func (s *ttsSentenceSegmenter) Segments(all bool) []string {
	s.buf = normalizeTTSSpokenText(s.buf)

	var segments []string
	for s.buf != "" {
		prefix, full := prefixRunes(s.buf, s.maxRunesPerSegment)
		idx := 0
		if s.firstSegment {
			idx = firstSentenceBoundaryIndex(prefix)
		} else {
			idx = lastSentenceBoundaryIndex(prefix)
		}
		switch {
		case idx > 0:
			segments = append(segments, s.buf[:idx])
			s.buf = s.buf[idx:]
			s.firstSegment = false
		case full:
			segments = append(segments, prefix)
			s.buf = s.buf[len(prefix):]
			s.firstSegment = false
		case all:
			segments = append(segments, s.buf)
			s.Reset()
		default:
			return segments
		}
	}
	return segments
}

func prefixRunes(text string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return text, false
	}
	count := 0
	for idx := range text {
		if count == maxRunes {
			return text[:idx], true
		}
		count++
	}
	return text, count >= maxRunes
}

func firstSentenceBoundaryIndex(text string) int {
	return sentenceBoundaryIndex(text, true, defaultTTSFirstSegmentMinRunes)
}

func lastSentenceBoundaryIndex(text string) int {
	return sentenceBoundaryIndex(text, true, 0)
}

func sentenceBoundaryIndex(text string, last bool, minRunes int) int {
	type runeInfo struct {
		value rune
		end   int
	}
	runes := make([]runeInfo, 0, utf8.RuneCountInString(text))
	for idx, r := range text {
		runes = append(runes, runeInfo{value: r, end: idx + utf8.RuneLen(r)})
	}
	found := 0
	for i, info := range runes {
		prev := rune(0)
		if i > 0 {
			prev = runes[i-1].value
		}
		next := rune(0)
		if i < len(runes)-1 {
			next = runes[i+1].value
		}
		if !isTTSSentenceBoundary(info.value, prev, next) {
			continue
		}
		requiredRunes := minRunes
		if requiredRunes > 0 && isTTSStrongSentenceBoundary(info.value) && requiredRunes > 4 {
			requiredRunes = 4
		}
		if requiredRunes > 0 && i+1 < requiredRunes {
			continue
		}
		if !last {
			return info.end
		}
		found = info.end
	}
	return found
}

func isTTSStrongSentenceBoundary(r rune) bool {
	switch r {
	case '。', '？', '！', '…', '?', '!', '\r', '\n':
		return true
	default:
		return false
	}
}

func isTTSSentenceBoundary(r, prev, next rune) bool {
	switch r {
	case '.', ':', ',', '：':
		if unicode.IsNumber(prev) && unicode.IsNumber(next) {
			return false
		}
		return true
	case '，', '；', '。', '？', '！', '…', '～',
		'?', '!', '¿', '¡', ';', '~',
		'\r', '\n', '„', '・':
		return true
	default:
		return false
	}
}
