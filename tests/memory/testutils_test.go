//go:build gizclaw_memory_e2e

package memoryintegration_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/GizClaw/gizclaw-go/pkgs/agent/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/agent/recall"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/generators"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/profilers"
	"github.com/GizClaw/gizclaw-go/pkgs/genx/segmentors"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const integrationSeparator byte = 0x1F

type realModelRuntime struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

type segtestCase struct {
	Name     string
	Desc     string
	Tier     string
	Group    string
	Path     string
	Messages []memory.Message
}

type segtestCorpus struct {
	Groups        map[string][]segtestCase
	TotalCases    int
	TotalMessages int
}

type scenarioMeta struct {
	Name          string
	TotalMessages int
	Conversations int
}

func requireRealModelRuntime(t *testing.T) realModelRuntime {
	t.Helper()

	apiKey := strings.TrimSpace(os.Getenv("GIZCLAW_MEMORY_E2E_API_KEY"))
	if apiKey == "" {
		t.Fatal("set GIZCLAW_MEMORY_E2E_API_KEY in tests/memory/.env")
	}

	return realModelRuntime{
		APIKey:  apiKey,
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Model:   "qwen-turbo-latest",
		Timeout: 45 * time.Second,
	}
}

func loadSegtestCorpus(t *testing.T) *segtestCorpus {
	t.Helper()

	root := filepath.Join("testdata", "corpus", "seg_cases")
	groups := []string{"simple", "complex", "long"}
	out := &segtestCorpus{Groups: make(map[string][]segtestCase, len(groups))}

	for _, group := range groups {
		pattern := filepath.Join(root, group, "*.yaml")
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		sort.Strings(files)
		if len(files) == 0 {
			t.Fatalf("no dataset files found for group %s", group)
		}

		for _, file := range files {
			parsed, err := parseSegtestCase(file, group)
			if err != nil {
				t.Fatalf("parse segtest case %s: %v", file, err)
			}
			out.Groups[group] = append(out.Groups[group], parsed)
			out.TotalCases++
			out.TotalMessages += len(parsed.Messages)
		}
	}

	return out
}

func listScenarioNames(t *testing.T) []string {
	t.Helper()

	root := filepath.Join("testdata", "corpus", "scenarios")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read scenario dir %s: %v", root, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "m") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatalf("no memory scenarios found in %s", root)
	}
	return names
}

func loadScenarioMeta(t *testing.T, scenario string) scenarioMeta {
	t.Helper()

	metaPath := filepath.Join("testdata", "corpus", "scenarios", scenario, "meta.yaml")
	content, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta %s: %v", metaPath, err)
	}

	meta := scenarioMeta{Name: scenario}
	for _, line := range strings.Split(string(content), "\n") {
		s := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(s, "total_messages:"):
			raw := strings.TrimSpace(strings.TrimPrefix(s, "total_messages:"))
			v, err := strconv.Atoi(raw)
			if err != nil {
				t.Fatalf("parse total_messages in %s: %v", metaPath, err)
			}
			meta.TotalMessages = v
		case strings.HasPrefix(s, "conversations:"):
			raw := strings.TrimSpace(strings.TrimPrefix(s, "conversations:"))
			v, err := strconv.Atoi(raw)
			if err != nil {
				t.Fatalf("parse conversations in %s: %v", metaPath, err)
			}
			meta.Conversations = v
		}
	}

	if meta.TotalMessages <= 0 {
		t.Fatalf("meta %s has invalid total_messages=%d", metaPath, meta.TotalMessages)
	}
	if meta.Conversations <= 0 {
		t.Fatalf("meta %s has invalid conversations=%d", metaPath, meta.Conversations)
	}

	return meta
}

func sumScenarioMessagesByMeta(t *testing.T) int {
	t.Helper()

	total := 0
	for _, name := range listScenarioNames(t) {
		meta := loadScenarioMeta(t, name)
		total += meta.TotalMessages
	}
	return total
}

func loadScenarioMessages(t *testing.T, scenario string, maxMessages int) []memory.Message {
	t.Helper()

	pattern := filepath.Join("testdata", "corpus", "scenarios", scenario, "conv_*.yaml")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatalf("no conversation files for scenario %s", scenario)
	}

	all := make([]memory.Message, 0, 1024)
	ts := int64(1_000_000_000)
	for _, file := range files {
		msgs, nextTS, err := parseScenarioConversationFile(file, ts)
		if err != nil {
			t.Fatalf("parse conversation file %s: %v", file, err)
		}
		ts = nextTS
		all = append(all, msgs...)
		if maxMessages > 0 && len(all) >= maxMessages {
			all = all[:maxMessages]
			break
		}
	}

	if len(all) == 0 {
		t.Fatalf("scenario %s has zero messages", scenario)
	}
	return all
}

func parseScenarioConversationFile(path string, tsStart int64) ([]memory.Message, int64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, tsStart, err
	}

	lines := strings.Split(string(content), "\n")
	messages := make([]memory.Message, 0, 32)
	ts := tsStart
	inMessages := false
	var cur memory.Message
	hasCur := false

	flush := func() {
		if !hasCur {
			return
		}
		if strings.TrimSpace(cur.Content) == "" {
			hasCur = false
			cur = memory.Message{}
			return
		}
		cur.Timestamp = ts
		ts += 1_000_000
		messages = append(messages, cur)
		hasCur = false
		cur = memory.Message{}
	}

	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}

		if strings.HasPrefix(s, "messages:") {
			inMessages = true
			continue
		}
		if !inMessages {
			continue
		}

		if strings.HasPrefix(s, "- role:") {
			flush()
			rawRole := strings.TrimSpace(strings.TrimPrefix(s, "- role:"))
			roleText, err := parseYAMLScalar(rawRole)
			if err != nil {
				return nil, tsStart, fmt.Errorf("parse role scalar: %w", err)
			}
			cur = memory.Message{Role: parseRoleName(roleText)}
			hasCur = true
			continue
		}

		if !hasCur {
			continue
		}

		if strings.HasPrefix(s, "name:") {
			rawName := strings.TrimSpace(strings.TrimPrefix(s, "name:"))
			name, err := parseYAMLScalar(rawName)
			if err != nil {
				return nil, tsStart, fmt.Errorf("parse name scalar: %w", err)
			}
			cur.Name = name
			continue
		}

		if strings.HasPrefix(s, "content:") {
			rawContent := strings.TrimSpace(strings.TrimPrefix(s, "content:"))
			body, err := parseYAMLScalar(rawContent)
			if err != nil {
				return nil, tsStart, fmt.Errorf("parse content scalar: %w", err)
			}
			cur.Content = body
			continue
		}
	}

	flush()
	return messages, ts, nil
}

func parseSegtestCase(path, group string) (segtestCase, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return segtestCase{}, err
	}

	lines := strings.Split(string(content), "\n")
	parsed := segtestCase{Group: group, Path: path}
	var ts int64 = 1_000_000_000
	inMessages := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "name:"):
			parsed.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			inMessages = false
			continue
		case strings.HasPrefix(trimmed, "desc:"):
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "desc:"))
			text, err := parseYAMLScalar(raw)
			if err != nil {
				return segtestCase{}, fmt.Errorf("line %d parse desc: %w", i+1, err)
			}
			parsed.Desc = text
			inMessages = false
			continue
		case strings.HasPrefix(trimmed, "tier:"):
			parsed.Tier = strings.TrimSpace(strings.TrimPrefix(trimmed, "tier:"))
			inMessages = false
			continue
		case strings.HasPrefix(trimmed, "messages:"):
			inMessages = true
			continue
		}

		if !inMessages {
			continue
		}

		if !strings.HasPrefix(trimmed, "- ") {
			inMessages = false
			continue
		}

		raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		text, err := parseYAMLScalar(raw)
		if err != nil {
			return segtestCase{}, fmt.Errorf("line %d parse message: %w", i+1, err)
		}

		role, body := parseRoleAndContent(text)
		parsed.Messages = append(parsed.Messages, memory.Message{
			Role:      role,
			Content:   body,
			Timestamp: ts,
		})
		ts += 1_000_000
	}

	if parsed.Name == "" {
		parsed.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if parsed.Tier == "" {
		parsed.Tier = group
	}
	if len(parsed.Messages) == 0 {
		return segtestCase{}, fmt.Errorf("case %s has no messages", path)
	}
	return parsed, nil
}

func parseYAMLScalar(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		text, err := strconv.Unquote(raw)
		if err != nil {
			return "", err
		}
		return text, nil
	}

	if strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'") && len(raw) >= 2 {
		text := raw[1 : len(raw)-1]
		text = strings.ReplaceAll(text, "''", "'")
		return text, nil
	}

	return raw, nil
}

func parseRoleAndContent(text string) (memory.Role, string) {
	parts := strings.SplitN(text, ":", 2)
	if len(parts) < 2 {
		return memory.RoleUser, strings.TrimSpace(text)
	}

	roleName := strings.ToLower(strings.TrimSpace(parts[0]))
	content := strings.TrimSpace(parts[1])
	return parseRoleName(roleName), content
}

func parseRoleName(roleName string) memory.Role {
	roleName = strings.ToLower(strings.TrimSpace(roleName))
	switch roleName {
	case "assistant", "model":
		return memory.RoleModel
	case "tool":
		return memory.RoleTool
	case "user":
		fallthrough
	default:
		return memory.RoleUser
	}
}

func chunkMessages(messages []memory.Message, chunkSize int) [][]memory.Message {
	if chunkSize <= 0 {
		return nil
	}
	chunks := make([][]memory.Message, 0, (len(messages)+chunkSize-1)/chunkSize)
	for i := 0; i < len(messages); i += chunkSize {
		j := i + chunkSize
		if j > len(messages) {
			j = len(messages)
		}
		part := make([]memory.Message, j-i)
		copy(part, messages[i:j])
		chunks = append(chunks, part)
	}
	return chunks
}

func newIntegrationHost(t *testing.T, compressor memory.Compressor) *memory.Host {
	t.Helper()

	store := mustBadgerInMemory(t, &kv.Options{Separator: integrationSeparator})
	host, err := memory.NewHost(context.Background(), memory.HostConfig{
		Store:          store,
		ObjectStore:    objectstore.Dir(t.TempDir()),
		Compressor:     compressor,
		CompressPolicy: memory.CompressPolicy{MaxMessages: 1 << 30, MaxChars: 1 << 30},
		Separator:      integrationSeparator,
	})
	if err != nil {
		t.Fatalf("new integration host: %v", err)
	}
	t.Cleanup(func() { _ = host.Close() })
	return host
}

func graphEntityCount(ctx context.Context, mem *memory.Memory) (int, error) {
	count := 0
	for _, err := range mem.Graph().ListEntities(ctx, "") {
		if err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func ingestMessagesWithGroup(ctx context.Context, mem *memory.Memory, compressor memory.Compressor, messages []memory.Message, groupLabel string) error {
	result, err := compressor.CompressMessages(ctx, messages)
	if err != nil {
		return err
	}

	for _, seg := range result.Segments {
		seg.Labels = appendUnique(seg.Labels, groupLabel)
		if err := mem.StoreSegment(ctx, seg, recall.Bucket1H); err != nil {
			return err
		}
	}

	update, err := compressor.ExtractEntities(ctx, messages)
	if err != nil {
		return err
	}
	if update == nil {
		update = &memory.EntityUpdate{}
	}
	update.Entities = append(update.Entities, memory.EntityInput{
		Label: groupLabel,
		Attrs: map[string]any{"group": groupLabel},
	})

	return mem.ApplyEntityUpdate(ctx, update)
}

func appendUnique(list []string, item string) []string {
	for _, v := range list {
		if v == item {
			return list
		}
	}
	return append(list, item)
}

func segmentHasLabel(seg memory.ScoredSegment, label string) bool {
	for _, l := range seg.Labels {
		if l == label {
			return true
		}
	}
	return false
}

func allSegmentsHaveLabel(segments []memory.ScoredSegment, label string) bool {
	if len(segments) == 0 {
		return false
	}
	for _, seg := range segments {
		if !segmentHasLabel(seg, label) {
			return false
		}
	}
	return true
}

func anySegmentHasLabel(segments []memory.ScoredSegment, label string) bool {
	for _, seg := range segments {
		if segmentHasLabel(seg, label) {
			return true
		}
	}
	return false
}

func requireNoLiveError(t *testing.T, phase string, err error) {
	t.Helper()
	if err == nil {
		return
	}
	if isTransientError(err) {
		t.Fatalf("transient provider error at %s: %v", phase, err)
	}
	t.Fatalf("%s: %v", phase, err)
}

func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "status 502") ||
		strings.Contains(msg, "status 503") {
		return true
	}
	return false
}

const integrationGeneratorPattern = "memory-it/model"

func newRealModelCompressor(runtime realModelRuntime) memory.Compressor {
	client := openai.NewClient(
		option.WithAPIKey(runtime.APIKey),
		option.WithBaseURL(runtime.BaseURL),
	)
	gen := &genx.OpenAIGenerator{
		Client:           &client,
		Model:            runtime.Model,
		SupportToolCalls: true,
		PromptRole:       genx.PromptRoleSystem,
		InvokeParams:     &genx.ModelParams{MaxTokens: 4096},
	}

	gmux := generators.NewMux()
	if err := gmux.Handle(integrationGeneratorPattern, gen); err != nil {
		panic("register generator: " + err.Error())
	}

	seg := segmentors.NewGenXWithMux(segmentors.Config{Generator: integrationGeneratorPattern}, gmux)
	prof := profilers.NewGenXWithMux(profilers.Config{Generator: integrationGeneratorPattern}, gmux)

	smux := segmentors.NewMux()
	if err := smux.Handle(integrationGeneratorPattern, seg); err != nil {
		panic("register segmentor: " + err.Error())
	}
	pmux := profilers.NewMux()
	if err := pmux.Handle(integrationGeneratorPattern, prof); err != nil {
		panic("register profiler: " + err.Error())
	}

	c, err := memory.NewLLMCompressor(memory.LLMCompressorConfig{
		Segmentor:    integrationGeneratorPattern,
		Profiler:     integrationGeneratorPattern,
		SegmentorMux: smux,
		ProfilerMux:  pmux,
	})
	if err != nil {
		panic("NewLLMCompressor: " + err.Error())
	}
	return c
}
