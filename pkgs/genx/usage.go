package genx

import "context"

// UsageUnit is the billing quantity of one provider model. A provider model
// reports every record in the same unit; units of different provider models
// are not comparable.
type UsageUnit string

const (
	// UsageUnitToken counts provider-reported tokens.
	UsageUnitToken UsageUnit = "token"
	// UsageUnitCharacter counts billed text characters as the provider counts
	// them, for example one per Unicode character or two per CJK character.
	UsageUnitCharacter UsageUnit = "character"
	// UsageUnitMillisecond counts billed audio duration.
	UsageUnitMillisecond UsageUnit = "millisecond"
)

// UsageModality is the kind of content a UsageRecord counts. Providers price
// text and audio of the same model separately.
type UsageModality string

const (
	UsageModalityText  UsageModality = "text"
	UsageModalityAudio UsageModality = "audio"
)

// UsageRecord is the billable consumption a provider reported for one
// response, request, or session. Input, CachedInput, and Output are disjoint:
// CachedInput is input billed at the provider's cache rate and is not part of
// Input.
type UsageRecord struct {
	// Provider names the upstream vendor, for example "openai" or "volc".
	Provider string
	// Model is the provider model, version, or resource ID that bills the
	// usage, never a voice or request ID.
	Model    string
	Modality UsageModality
	Unit     UsageUnit

	Input       int64
	CachedInput int64
	Output      int64
}

// IsZero reports whether the record counts no quantity.
func (r UsageRecord) IsZero() bool {
	return r.Input == 0 && r.CachedInput == 0 && r.Output == 0
}

// UsageRecorder receives the usage providers report. Generators and
// Transformers call it from their own goroutines, possibly concurrently, as
// soon as the provider reports usage; it must be safe for concurrent use and
// must not block.
type UsageRecorder func(UsageRecord)

type usageRecorderKey struct{}

// WithUsageRecorder returns a context whose Generator and Transformer calls
// report provider usage to record. It replaces any recorder ctx carries.
func WithUsageRecorder(ctx context.Context, record UsageRecorder) context.Context {
	return context.WithValue(ctx, usageRecorderKey{}, record)
}

// RecordUsage reports records to the recorder ctx carries. Records without any
// quantity are dropped. Provider adapters call it with the usage the provider
// reported; usage a provider never reports, for example of a response the
// caller canceled before the provider's usage event, is not recorded.
func RecordUsage(ctx context.Context, records ...UsageRecord) {
	if ctx == nil {
		return
	}
	record, _ := ctx.Value(usageRecorderKey{}).(UsageRecorder)
	if record == nil {
		return
	}
	for _, item := range records {
		if !item.IsZero() {
			record(item)
		}
	}
}

// TokenUsage is a provider token report in the common "totals with details"
// shape: InputTokens includes InputAudioTokens and both cached counts,
// InputAudioTokens includes CachedAudioTokens, and OutputTokens includes
// OutputAudioTokens.
type TokenUsage struct {
	InputTokens       int64
	InputAudioTokens  int64
	CachedTextTokens  int64
	CachedAudioTokens int64
	OutputTokens      int64
	OutputAudioTokens int64
}

// Records splits the report into disjoint text and audio token records.
// Negative remainders of inconsistent reports become zero.
func (u TokenUsage) Records(provider, model string) []UsageRecord {
	return []UsageRecord{
		{
			Provider: provider, Model: model, Modality: UsageModalityText, Unit: UsageUnitToken,
			Input:       max(u.InputTokens-u.InputAudioTokens-u.CachedTextTokens, 0),
			CachedInput: u.CachedTextTokens,
			Output:      max(u.OutputTokens-u.OutputAudioTokens, 0),
		},
		{
			Provider: provider, Model: model, Modality: UsageModalityAudio, Unit: UsageUnitToken,
			Input:       max(u.InputAudioTokens-u.CachedAudioTokens, 0),
			CachedInput: u.CachedAudioTokens,
			Output:      u.OutputAudioTokens,
		},
	}
}
