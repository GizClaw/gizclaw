package doubaoast

import (
	"strings"
	"unicode"
)

func realtimeTextDelta(previous, current string) string {
	if current == "" || current == previous {
		return ""
	}
	if previous != "" && strings.HasPrefix(current, previous) {
		return current[len(previous):]
	}
	if previous != "" {
		if suffix, ok := realtimeTextSuffixAfterNormalizedPrefix(previous, current); ok {
			return suffix
		}
		previousNorm := realtimeNormalizeText(previous)
		currentNorm := realtimeNormalizeText(current)
		if previousNorm != "" && currentNorm != "" && strings.Contains(previousNorm, currentNorm) {
			return ""
		}
	}
	return current
}

func realtimeTextSuffixAfterNormalizedPrefix(previous, current string) (string, bool) {
	previousNorm := realtimeNormalizeText(previous)
	if previousNorm == "" {
		return current, true
	}
	matched := 0
	for i, r := range current {
		norm := realtimeNormalizeText(string(r))
		if norm == "" {
			continue
		}
		if matched >= len(previousNorm) || !strings.HasPrefix(previousNorm[matched:], norm) {
			return "", false
		}
		matched += len(norm)
		if matched == len(previousNorm) {
			trailing := previous[len(strings.TrimRightFunc(previous, func(r rune) bool {
				return !unicode.IsLetter(r) && !unicode.IsNumber(r)
			})):]
			return strings.TrimPrefix(current[i+len(string(r)):], trailing), true
		}
	}
	return "", matched == len(previousNorm)
}

func realtimeNormalizeText(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
