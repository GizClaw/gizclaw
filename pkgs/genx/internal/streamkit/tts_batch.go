package streamkit

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// ttsTextBatch owns confirmed, still mergeable text for one speech identity.
// A segment boundary does not open a provider request.
type ttsTextBatch struct {
	config   genx.TTSBatchConfig
	text     strings.Builder
	runes    int
	started  time.Time
	complete bool
	sentence bool
}

func (b *ttsTextBatch) fits(segment ttsTextSegment) bool {
	return b.runes+utf8.RuneCountInString(segment.text) <= b.config.MaxRunes
}

func (b *ttsTextBatch) append(segment ttsTextSegment, now time.Time) {
	if b.runes == 0 {
		b.started = now
	}
	b.text.WriteString(segment.text)
	b.runes += utf8.RuneCountInString(segment.text)
	b.complete = segment.sentence && !segment.quoteOpen
	b.sentence = segment.sentence
}

func (b *ttsTextBatch) due(now time.Time, first, force bool, budget *ttsSynthesisBudget) (bool, time.Time) {
	if b.runes == 0 {
		return false, time.Time{}
	}
	if force || b.runes == b.config.MaxRunes {
		return true, time.Time{}
	}
	if b.complete && first {
		return true, time.Time{}
	}
	if first {
		minimum := b.config.FirstMinRunes
		if b.sentence {
			minimum = min(minimum, 4)
		}
		if b.runes < minimum {
			return false, time.Time{}
		}
	}
	if first {
		deadline := b.started.Add(b.config.FirstWait)
		return !now.Before(deadline), deadline
	}
	deadline, awaiting := budget.deadline(now)
	if b.config.MaxWait > 0 {
		limit := b.started.Add(b.config.MaxWait)
		if deadline.IsZero() || limit.Before(deadline) {
			deadline, awaiting = limit, false
		}
	}
	if awaiting {
		return false, time.Time{}
	}
	return deadline.IsZero() || !now.Before(deadline), deadline
}

func (b *ttsTextBatch) take() string {
	text := b.text.String()
	*b = ttsTextBatch{config: b.config}
	return text
}
