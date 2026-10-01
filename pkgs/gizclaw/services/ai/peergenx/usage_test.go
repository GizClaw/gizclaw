package peergenx

import (
	"context"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerusage"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type billedBuilder struct{}

func (billedBuilder) BuildGenerator(context.Context, GeneratorConfig) (genx.Generator, error) {
	return billedGenerator{}, nil
}
func (billedBuilder) BuildTransformer(context.Context, TransformerConfig) (genx.Transformer, error) {
	return billedTransformer{}, nil
}

type billedGenerator struct{}

func (billedGenerator) GenerateStream(ctx context.Context, _ string, _ genx.ModelContext) (genx.Stream, error) {
	genx.RecordUsage(ctx, genx.UsageRecord{Model: "billing-model", Input: 3, CachedInput: 2, Output: 4})
	return nil, nil
}
func (billedGenerator) Invoke(ctx context.Context, _ string, _ genx.ModelContext, _ *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	genx.RecordUsage(ctx, genx.UsageRecord{Model: "billing-model", Input: 5})
	return genx.Usage{}, nil, nil
}

type billedTransformer struct{}

func (billedTransformer) Transform(ctx context.Context, _ genx.Stream) (genx.Stream, error) {
	genx.RecordUsage(ctx, genx.UsageRecord{Model: "tts-resource", Input: 7})
	return nil, nil
}

func TestProviderBuildersPersistBillingModelsAndPreserveCallerRecorder(t *testing.T) {
	db := sqlx.MustOpen("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	defer db.Close()
	store, err := peerusage.NewStore(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	peer := giznet.PublicKey{10}
	meter := peerusage.NewRecorder(store)
	service := New(Service{Builder: billedBuilder{}, Usage: meter.Handler(peer)})
	var observed int64
	ctx := genx.WithUsageRecorder(t.Context(), func(item genx.UsageRecord) { observed += item.Input + item.CachedInput + item.Output })
	g, err := service.builder().BuildGenerator(ctx, GeneratorConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.GenerateStream(ctx, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := g.Invoke(ctx, "", nil, nil); err != nil {
		t.Fatal(err)
	}
	transformer, err := service.builder().BuildTransformer(ctx, TransformerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transformer.Transform(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := meter.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	hour := time.Now().UTC().Truncate(time.Hour)
	rows, err := store.Query(t.Context(), peer, "", hour, hour.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ModelID != "billing-model" || rows[0].Quantity != 14 || rows[1].ModelID != "tts-resource" || rows[1].Quantity != 7 || observed != 21 {
		t.Fatalf("rows=%+v caller=%d", rows, observed)
	}
}

func TestNestedUsageScopeDoesNotDuplicateProductMetering(t *testing.T) {
	var external, outer, inner int
	parent := genx.WithUsageRecorder(t.Context(), func(genx.UsageRecord) { external++ })
	ctx := providerUsageContext(parent, func(genx.UsageRecord) { outer++ })
	ctx = providerUsageContext(ctx, func(genx.UsageRecord) { inner++ })
	genx.RecordUsage(ctx, genx.UsageRecord{Input: 1})
	if external != 1 || outer != 0 || inner != 1 {
		t.Fatalf("external=%d outer=%d inner=%d", external, outer, inner)
	}
}
