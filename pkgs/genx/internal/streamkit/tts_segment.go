package streamkit

import (
	"bufio"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
	"github.com/clipperhouse/uax29/v2/sentences"
	"github.com/clipperhouse/uax29/v2/words"
)

const ttsBoundaryLookaheadRunes = 32

type ttsSentenceSegmenter struct {
	buf                string
	normalizer         ttsTextNormalizer
	maxRunesPerSegment int
	quotes             ttsQuoteState
}

type ttsTextSegment struct {
	text      string
	sentence  bool
	quoteOpen bool
}

func newTTSSentenceSegmenter(maxRunes int) *ttsSentenceSegmenter {
	if maxRunes <= 0 {
		maxRunes = defaultTTSSegmentMaxRunes
	}
	return &ttsSentenceSegmenter{maxRunesPerSegment: maxRunes}
}

func (s *ttsSentenceSegmenter) WriteString(text string) {
	s.buf += s.normalizer.WriteString(text)
}

func (s *ttsSentenceSegmenter) Reset() {
	s.buf = ""
	s.normalizer = ttsTextNormalizer{}
	s.quotes = ttsQuoteState{}
}

func (s *ttsSentenceSegmenter) Segments(all bool) []string {
	var segments []string
	for {
		segment, ok := s.nextSegment(all)
		if !ok {
			return segments
		}
		segments = append(segments, segment.text)
	}
}

func (s *ttsSentenceSegmenter) nextSegment(all bool) (ttsTextSegment, bool) {
	if all {
		s.buf += s.normalizer.Finish()
	}
	if end := s.boundary(all); end > 0 {
		text := s.buf[:end]
		s.quotes = s.quotes.after(s.buf[:end])
		s.buf = s.buf[end:]
		return ttsTextSegment{text: text, sentence: ttsSentencePause(text), quoteOpen: s.quotes.depth != 0}, true
	}
	if all {
		s.Reset()
	}
	return ttsTextSegment{}, false
}

func (s *ttsSentenceSegmenter) boundary(all bool) int {
	// Scan only a bounded prefix with real lookahead. A chunk boundary is never
	// passed to UAX #29 as EOF; punctuation, words and graphemes can continue.
	window, _ := prefixRunes(s.buf, s.maxRunesPerSegment+ttsBoundaryLookaheadRunes)
	final := all && len(window) == len(s.buf)
	// Withheld whitespace is real lookahead even though it is not published
	// until the next visible character. This matters when a model pauses here.
	if len(window) == len(s.buf) && s.normalizer.space != "" {
		_, width := utf8.DecodeRuneInString(s.normalizer.space)
		window += s.normalizer.space[:width]
	}
	data := []byte(window)
	limit, full := prefixRunes(s.buf, s.maxRunesPerSegment)
	wordEnds := ttsBreaks(data, final, words.SplitFunc)
	graphemeEnds := ttsBreaks(data, final, graphemes.SplitFunc)
	graphemeBoundary := make(map[int]bool, len(graphemeEnds))
	for _, end := range graphemeEnds {
		graphemeBoundary[end] = true
	}
	found := 0
	for _, end := range ttsBreaks(data, final, sentences.SplitFunc) {
		end = s.quoteBoundary(window, end, final)
		if end == 0 || end > len(limit) || !graphemeBoundary[end] || ttsSuppressSentenceBoundary(window, end) {
			continue
		}
		if found == 0 || end < found {
			found = end
		}
	}
	// TTS can pause at a completed terminator followed by observed whitespace,
	// without waiting for another word or pretending that a chunk is EOF.
	if len(s.buf) <= len(limit) && strings.HasPrefix(s.normalizer.space, " ") &&
		graphemeBoundary[len(s.buf)] && ttsSentencePause(s.buf) && !ttsSuppressSentenceBoundary(s.buf, len(s.buf)) &&
		s.quoteBoundary(window, len(s.buf), final) == len(s.buf) {
		if found == 0 || len(s.buf) < found {
			found = len(s.buf)
		}
	}
	for _, end := range wordEnds {
		r, width := utf8.DecodeLastRuneInString(window[:end])
		if !strings.ContainsRune("，,；;：:…～~", r) || ttsInsideTechnicalWord(window, end) {
			continue
		}
		if strings.ContainsRune(",:：", r) {
			prev, _ := utf8.DecodeLastRuneInString(window[:end-width])
			next, _ := utf8.DecodeRuneInString(window[end:])
			if unicode.IsNumber(prev) && (end == len(window) || unicode.IsNumber(next)) {
				continue
			}
		}
		// Keep right quotes with the phrase; leave a left quote for its text.
		quotes := s.quotes.after(window[:end])
		for end < len(window) {
			next, size := utf8.DecodeRuneInString(window[end:])
			if quotes.opens(next) || !ttsClosingPunctuation(next) && !strings.ContainsRune("，,；;：:…！？!?～~", next) {
				break
			}
			end += size
			following, _ := utf8.DecodeRuneInString(window[end:])
			quotes.consume(next, following)
		}
		// A comma/colon before a quotation introduces that quotation. Wait
		// for visible lookahead so whitespace split across chunks is harmless.
		lookahead := strings.TrimLeftFunc(window[end:], unicode.IsSpace)
		if lookahead == "" && !final {
			continue
		}
		next, _ := utf8.DecodeRuneInString(lookahead)
		if quotes.opens(next) {
			continue
		}
		end = s.quoteBoundary(window, end, final)
		if end == 0 || end > len(limit) || !graphemeBoundary[end] || !final && end == len(window) {
			continue
		}
		if found == 0 || end < found {
			found = end
		}
	}
	if found > 0 {
		return found
	}
	if !full {
		if all {
			return len(s.buf)
		}
		return 0
	}

	// Prefer a complete word below the budget. Technical tokens such as email,
	// IPv6 and hyphenated words extend the default Unicode word boundaries.
	for _, end := range wordEnds {
		if end <= len(limit) && graphemeBoundary[end] && !ttsInsideTechnicalWord(window, end) && !ttsSuppressSentenceBoundary(window, end) &&
			s.quoteBoundary(window, end, final) == end {
			found = end
		}
	}
	if found > 0 {
		return found
	}
	// A single word may exceed the budget; split it at a complete grapheme.
	for _, end := range graphemeEnds {
		if end <= len(limit) {
			found = end
		}
	}
	if found > 0 {
		return found
	}
	// A pathological combining sequence can itself exceed the hard rune budget.
	// Preserve code points and bounded progress even when no full grapheme fits.
	if full {
		return len(limit)
	}
	return 0
}

func ttsBreaks(data []byte, final bool, split bufio.SplitFunc) []int {
	var ends []int
	for offset := 0; offset < len(data); {
		advance, _, err := split(data[offset:], final)
		if err != nil || advance <= 0 {
			break
		}
		offset += advance
		ends = append(ends, offset)
	}
	return ends
}

func ttsInsideTechnicalWord(text string, end int) bool {
	if end == 0 || end == len(text) {
		return false
	}
	prev, _ := utf8.DecodeLastRuneInString(text[:end])
	next, _ := utf8.DecodeRuneInString(text[end:])
	if unicode.IsNumber(prev) && (next == '\u00a0' || next == '\u202f') ||
		unicode.IsNumber(next) && (prev == '\u00a0' || prev == '\u202f') {
		return true
	}
	identifier := func(r rune) bool {
		return ttsASCIIWord(r) || strings.ContainsRune(".:@'-", r)
	}
	return identifier(prev) && identifier(next)
}

func ttsSentencePause(text string) bool {
	text = strings.TrimRightFunc(text, func(r rune) bool {
		return ttsClosingPunctuation(r) || unicode.IsSpace(r) || unicode.IsMark(r) || unicode.Is(unicode.Cf, r)
	})
	r, _ := utf8.DecodeLastRuneInString(text)
	return r == '.' || unicode.Is(unicode.Sentence_Terminal, r)
}

func ttsClosingPunctuation(r rune) bool {
	// Pi includes the closing quotation mark in German „…“.
	return unicode.Is(unicode.Pe, r) || unicode.Is(unicode.Pf, r) || unicode.Is(unicode.Pi, r) || r == '"' || r == '\''
}

func prefixRunes(text string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return text, false
	}
	count := 0
	for index := range text {
		if count == maxRunes {
			return text[:index], true
		}
		count++
	}
	return text, count >= maxRunes
}
