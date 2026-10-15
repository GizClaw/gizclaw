package streamkit

import (
	_ "embed"
	"encoding/xml"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/words"
)

// English sentence suppressions from Unicode CLDR release 48. The source
// (with trailing blank lines removed) and its Unicode-3.0 license are in data/.
// https://github.com/unicode-org/cldr/blob/release-48/common/segments/en.xml
//
//go:embed data/cldr-en.xml
var ttsCLDREnglish []byte

var ttsAbbreviations = loadTTSAbbreviations()

func loadTTSAbbreviations() map[string]bool {
	var data struct {
		Suppressions []string `xml:"segmentations>segmentation>suppressions>suppression"`
	}
	if err := xml.Unmarshal(ttsCLDREnglish, &data); err != nil {
		panic(err)
	}
	abbreviations := make(map[string]bool, len(data.Suppressions)+1)
	for _, text := range data.Suppressions {
		abbreviations[strings.ToLower(text)] = true
	}
	// CLDR's English list omits this common honorific; keep it with the name.
	abbreviations["dr."] = true
	return abbreviations
}

func ttsSuppressSentenceBoundary(text string, end int) bool {
	prefix := strings.TrimRightFunc(text[:end], func(r rune) bool {
		return unicode.IsSpace(r) || ttsClosingPunctuation(r)
	})
	if !strings.HasSuffix(prefix, ".") {
		return false
	}
	// Unicode word boundaries also separate an adjacent CJK prefix from a
	// Latin abbreviation, as in "请提醒Dr. Smith".
	ends := ttsBreaks([]byte(strings.TrimSuffix(prefix, ".")), true, words.SplitFunc)
	start := 0
	if len(ends) > 1 {
		start = ends[len(ends)-2]
	}
	word := prefix[start:]
	if ttsAbbreviations[strings.ToLower(word)] {
		return true
	}
	initial := strings.TrimSuffix(word, ".")
	r, size := utf8.DecodeRuneInString(initial)
	return size == len(initial) && unicode.IsUpper(r)
}
