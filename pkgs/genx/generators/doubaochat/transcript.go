package doubaochat

import "strings"

const (
	transcriptOpen  = "<asr>"
	transcriptClose = "</asr"
)

// transcriptFilter removes the first <asr>…</asr> segment from streamed model
// text wherever it appears. The transcript ends at the first closing tag or
// line break, because the model sometimes misspells the closing bracket, as
// in </asr], while keeping the transcript on one line.
type transcriptFilter struct {
	// pending holds text that may still turn out to be part of the segment:
	// a possible opening-tag prefix before the segment, or the unfinished
	// segment itself.
	pending strings.Builder
	inside  bool
	done    bool
	// trimNext drops the line break that separates the segment from the
	// reply text after it.
	trimNext bool
	// replying reports that visible reply text was released; leading
	// whitespace before it is dropped.
	replying bool
}

// consume takes one streamed text delta. It returns the reply text that can
// be released before the segment, and, when this delta closes the segment,
// the transcript and the reply text that follows it.
func (filter *transcriptFilter) consume(delta string) (before, transcript string, closed bool, after string) {
	if filter.done {
		return filter.afterSegment(delta), "", false, ""
	}
	filter.pending.WriteString(delta)
	buffered := filter.pending.String()
	if !filter.inside {
		openAt := strings.Index(strings.ToLower(buffered), transcriptOpen)
		if openAt < 0 {
			keep := partialOpenSuffix(buffered)
			filter.pending.Reset()
			filter.pending.WriteString(buffered[len(buffered)-keep:])
			return filter.visible(buffered[:len(buffered)-keep]), "", false, ""
		}
		filter.inside = true
		filter.pending.Reset()
		filter.pending.WriteString(buffered[openAt+len(transcriptOpen):])
		_, transcript, closed, after := filter.consume("")
		return filter.visible(buffered[:openAt]), transcript, closed, after
	}
	text, rest, ok := splitTranscript(buffered)
	if !ok {
		return "", "", false, ""
	}
	filter.inside = false
	filter.done = true
	filter.trimNext = true
	filter.pending.Reset()
	return "", strings.TrimSpace(text), true, filter.afterSegment(rest)
}

// finish returns text still held at the end of the stream and the transcript
// of a segment the model opened but never closed.
func (filter *transcriptFilter) finish() (string, string, bool) {
	held := filter.pending.String()
	filter.pending.Reset()
	if filter.inside {
		filter.inside = false
		filter.done = true
		return "", strings.TrimSpace(held), true
	}
	return filter.visible(held), "", false
}

func (filter *transcriptFilter) afterSegment(delta string) string {
	if filter.trimNext && delta != "" {
		delta = strings.TrimLeft(delta, "\r\n")
		filter.trimNext = delta == ""
	}
	return filter.visible(delta)
}

// visible drops whitespace ahead of the first visible reply text.
func (filter *transcriptFilter) visible(text string) string {
	if !filter.replying {
		text = strings.TrimLeft(text, " \t\r\n")
		filter.replying = text != ""
	}
	return text
}

// partialOpenSuffix reports how many trailing bytes of text could begin the
// opening tag.
func partialOpenSuffix(text string) int {
	lower := strings.ToLower(text)
	for size := min(len(transcriptOpen)-1, len(lower)); size > 0; size-- {
		if strings.HasPrefix(transcriptOpen, lower[len(lower)-size:]) {
			return size
		}
	}
	return 0
}

// splitTranscript finds the end of the transcript in body, the text after
// the opening tag. It reports false while more text is needed.
func splitTranscript(body string) (string, string, bool) {
	body = strings.TrimLeft(body, " \t\r\n")
	if body == "" {
		return "", "", false
	}
	closeAt := strings.Index(strings.ToLower(body), transcriptClose)
	lineAt := strings.IndexAny(body, "\r\n")
	if lineAt >= 0 && (closeAt < 0 || lineAt < closeAt) {
		return body[:lineAt], body[lineAt:], true
	}
	if closeAt < 0 {
		return "", "", false
	}
	after := closeAt + len(transcriptClose)
	if after == len(body) {
		return "", "", false
	}
	if strings.ContainsRune(">])", rune(body[after])) {
		after++
	}
	return body[:closeAt], body[after:], true
}
