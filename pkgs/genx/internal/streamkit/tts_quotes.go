package streamkit

import (
	"unicode"
	"unicode/utf8"
)

// The fixed stack keeps quote context bounded, including malformed input.
// German „…“ and Chinese/English “…” share a mark with opposite roles.
type ttsQuoteState struct {
	closers  [64]rune
	depth    int
	overflow int
	last     rune
}

func (s ttsQuoteState) opens(r rune) bool {
	if s.depth != 0 && s.closers[s.depth-1] == r {
		return false
	}
	return ttsQuoteCloser(r) != 0
}

func (s *ttsQuoteState) consume(r, next rune) {
	previous := s.last
	s.last = r
	// A typographic apostrophe between Latin letters is part of the word.
	if r == '’' && unicode.Is(unicode.Latin, previous) && unicode.Is(unicode.Latin, next) {
		return
	}
	if s.depth != 0 && s.closers[s.depth-1] == r {
		if s.overflow > 0 {
			s.overflow--
		} else {
			s.depth--
		}
		return
	}
	if closer := ttsQuoteCloser(r); closer != 0 {
		if s.depth == len(s.closers) {
			s.overflow++
		} else {
			s.closers[s.depth] = closer
			s.depth++
		}
	}
}

func (s ttsQuoteState) after(text string) ttsQuoteState {
	for index, r := range text {
		next, _ := utf8.DecodeRuneInString(text[index+utf8.RuneLen(r):])
		s.consume(r, next)
	}
	return s
}

func ttsQuoteCloser(r rune) rune {
	switch r {
	case '"':
		return '"'
	case '“':
		return '”'
	case '„':
		return '“'
	case '«':
		return '»'
	case '‹':
		return '›'
	case '‘':
		return '’'
	case '「':
		return '」'
	case '『':
		return '』'
	}
	return 0
}

func (s *ttsSentenceSegmenter) quoteBoundary(text string, end int, final bool) int {
	quotes := s.quotes
	opening := -1
	for index, r := range text[:end] {
		if quotes.opens(r) {
			if opening < 0 {
				opening = index
			}
		} else if !unicode.IsSpace(r) {
			opening = -1
		}
		next, _ := utf8.DecodeRuneInString(text[index+utf8.RuneLen(r):])
		quotes.consume(r, next)
	}
	// UAX Close also contains marks that can open the next quotation.
	if opening >= 0 {
		return opening
	}
	quote := end
	for quotes.depth > 0 {
		for quote < len(text) {
			r, width := utf8.DecodeRuneInString(text[quote:])
			if !unicode.IsSpace(r) {
				break
			}
			quote += width
		}
		if quote == len(text) {
			if !final {
				return 0
			}
			return end
		}
		r, width := utf8.DecodeRuneInString(text[quote:])
		if quotes.closers[quotes.depth-1] != r {
			return end
		}
		quote += width
		end = quote
		next, _ := utf8.DecodeRuneInString(text[quote:])
		quotes.consume(r, next)
	}
	return end
}
