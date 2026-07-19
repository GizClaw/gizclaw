package genx_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/generators"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/labelers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/match"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/profilers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/segmentors"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/transformers"
)

type gxMockGenerator struct{}

func (gxMockGenerator) GenerateStream(_ context.Context, _ string, mctx genx.ModelContext) (genx.Stream, error) {
	sb := genx.NewStreamBuilder(mctx, 4)
	if err := sb.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text("topic_rule: topic=恐龙\n")}); err != nil {
		return nil, err
	}
	if err := sb.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return sb.Stream(), nil
}

func (gxMockGenerator) Invoke(_ context.Context, _ string, _ genx.ModelContext, tool *genx.FuncTool) (genx.Usage, *genx.FuncCall, error) {
	switch tool.Name {
	case "select_labels":
		return genx.Usage{}, tool.NewFuncCall(`{"matches":[{"label":"topic:恐龙","score":0.99}]}`), nil
	case "extract":
		return genx.Usage{}, tool.NewFuncCall(`{
			"segment":{"summary":"小明喜欢恐龙","keywords":["恐龙"],"labels":["topic:恐龙"]},
			"entities":[{"label":"person:小明","attrs":[{"key":"age","value":"5"}]}],
			"relations":[{"from":"person:小明","to":"topic:恐龙","rel_type":"likes"}]
		}`), nil
	case "update_profiles":
		return genx.Usage{}, tool.NewFuncCall(`{
			"schema_changes":[{"entity_type":"person","field":"age","def":{"type":"int","desc":"年龄"},"action":"add"}],
			"profile_updates":{"person:小明":{"age":5}},
			"relations":[{"from":"person:小明","to":"topic:恐龙","rel_type":"likes"}]
		}`), nil
	default:
		return genx.Usage{}, nil, fmt.Errorf("unexpected tool: %s", tool.Name)
	}
}

type gxUpperTransformer struct{}

func (gxUpperTransformer) Transform(_ context.Context, input genx.Stream) (genx.Stream, error) {
	defer input.Close()

	var mcb genx.ModelContextBuilder
	outCtx := mcb.Build()
	b := genx.NewStreamBuilder(outCtx, 8)

	for {
		chunk, err := input.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if chunk == nil || chunk.Part == nil {
			continue
		}
		text, ok := chunk.Part.(genx.Text)
		if !ok {
			continue
		}
		if err := b.Add(&genx.MessageChunk{Role: chunk.Role, Name: chunk.Name, Part: genx.Text(strings.ToUpper(string(text)))}); err != nil {
			return nil, err
		}
	}

	if err := b.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return b.Stream(), nil
}

func TestGXMinimalIntegrationChain(t *testing.T) {
	ctx := context.Background()
	mock := gxMockGenerator{}

	rules := []*match.Rule{{
		Name: "topic_rule",
		Vars: map[string]match.Var{
			"topic": {Label: "主题", Type: "string"},
		},
		Patterns: []match.Pattern{{Input: "我喜欢[topic]"}},
	}}

	matcher, err := match.Compile(rules)
	if err != nil {
		t.Fatalf("compile rules failed: %v", err)
	}

	var qctxBuilder genx.ModelContextBuilder
	qctxBuilder.UserText("user", "我喜欢恐龙")
	matchResults, err := match.Collect(matcher.Match(ctx, "gx/mock", qctxBuilder.Build(), match.WithGenerator(mock)))
	if err != nil {
		t.Fatalf("match failed: %v", err)
	}
	if len(matchResults) != 1 || matchResults[0].Rule != "topic_rule" {
		t.Fatalf("unexpected match results: %#v", matchResults)
	}
	if got := matchResults[0].Args["topic"].Value; got != "恐龙" {
		t.Fatalf("unexpected match arg topic: %#v", got)
	}

	gm := generators.NewMux()
	if err := gm.Handle("gx/mock", mock); err != nil {
		t.Fatalf("register mock generator failed: %v", err)
	}

	lb := labelers.NewGenXWithMux(labelers.Config{Generator: "gx/mock"}, gm)
	lbRes, err := lb.Process(ctx, labelers.Input{
		Text:       "恐龙相关记忆",
		Candidates: []string{"topic:恐龙", "topic:猫"},
		TopK:       1,
	})
	if err != nil {
		t.Fatalf("labeler process failed: %v", err)
	}
	if len(lbRes.Matches) != 1 || lbRes.Matches[0].Label != "topic:恐龙" {
		t.Fatalf("unexpected labeler result: %#v", lbRes)
	}

	seg := segmentors.NewGenXWithMux(segmentors.Config{Generator: "gx/mock"}, gm)
	segRes, err := seg.Process(ctx, segmentors.Input{Messages: []string{"小明喜欢恐龙"}})
	if err != nil {
		t.Fatalf("segmentor process failed: %v", err)
	}
	if segRes.Segment.Summary != "小明喜欢恐龙" {
		t.Fatalf("unexpected segment summary: %#v", segRes.Segment)
	}

	prof := profilers.NewGenXWithMux(profilers.Config{Generator: "gx/mock"}, gm)
	profRes, err := prof.Process(ctx, profilers.Input{
		Messages:  []string{"小明喜欢恐龙"},
		Extracted: segRes,
	})
	if err != nil {
		t.Fatalf("profiler process failed: %v", err)
	}
	if profRes.ProfileUpdates["person:小明"]["age"] != float64(5) {
		t.Fatalf("unexpected profiler updates: %#v", profRes.ProfileUpdates)
	}

	tm := transformers.NewMux()
	if err := tm.Handle("gx/upper", gxUpperTransformer{}); err != nil {
		t.Fatalf("register transformer failed: %v", err)
	}

	in, err := textStream("summary: " + segRes.Segment.Summary)
	if err != nil {
		t.Fatalf("build input stream failed: %v", err)
	}
	out, err := tm.Transform(ctx, "gx/upper", in)
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}

	transformed, err := readAllText(out)
	if err != nil {
		t.Fatalf("read transformed stream failed: %v", err)
	}
	if transformed != "SUMMARY: 小明喜欢恐龙" {
		t.Fatalf("unexpected transformed text: %q", transformed)
	}
}

func textStream(text string) (genx.Stream, error) {
	var mcb genx.ModelContextBuilder
	ctx := mcb.Build()
	b := genx.NewStreamBuilder(ctx, 4)
	if err := b.Add(&genx.MessageChunk{Role: genx.RoleModel, Part: genx.Text(text)}); err != nil {
		return nil, err
	}
	if err := b.Done(genx.Usage{}); err != nil {
		return nil, err
	}
	return b.Stream(), nil
}

func readAllText(stream genx.Stream) (string, error) {
	defer stream.Close()
	var sb strings.Builder
	for {
		chunk, err := stream.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) || errors.Is(err, io.EOF) {
				return sb.String(), nil
			}
			return "", err
		}
		if chunk == nil || chunk.Part == nil {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			sb.WriteString(string(text))
		}
	}
}
