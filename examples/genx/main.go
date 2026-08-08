package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

//go:embed models/*_openai.json
var embeddedModels embed.FS

type invokeArg struct {
	Result string `json:"result"`
}

type modelExpectation struct {
	GenerateSuccess                   bool `json:"generate_success"`
	InvokeJSONOutputSuccess           bool `json:"invoke_json_output_success"`
	InvokeToolCallsSuccess            bool `json:"invoke_tool_calls_success"`
	InvalidKeyGenerateSuccess         bool `json:"invalid_key_generate_success"`
	InvalidKeyInvokeJSONOutputSuccess bool `json:"invalid_key_invoke_json_output_success"`
	InvalidKeyInvokeToolCallsSuccess  bool `json:"invalid_key_invoke_tool_calls_success"`
}

type modelEntry struct {
	Name         string            `json:"name"`
	Model        string            `json:"model"`
	BaseURL      string            `json:"base_url,omitempty"`
	Expect       *modelExpectation `json:"expect,omitempty"`
	InvokeParams struct {
		MaxTokens int `json:"max_tokens"`
	} `json:"invoke_params"`
}

type modelConfig struct {
	Schema  string           `json:"schema"`
	Type    string           `json:"type"`
	APIKey  string           `json:"api_key"`
	BaseURL string           `json:"base_url"`
	Expect  modelExpectation `json:"expect"`
	Models  []modelEntry     `json:"models"`
}

type probeResult struct {
	Success      bool
	Skipped      bool
	Detail       string
	Connectivity bool
}

type modelReport struct {
	ProviderFile string
	ModelName    string
	Generate     probeResult
	InvokeJSON   probeResult
	InvokeTool   probeResult
	Mismatches   []string
	Notes        []string
}

var (
	bazelEnvOnce sync.Once
	bazelEnvData map[string]string
	bazelEnvErr  error
)

func main() {
	configs, err := loadConfigs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load configs failed: %v\n", err)
		os.Exit(1)
	}

	reports := make([]modelReport, 0)
	totalMismatches := 0

	for _, cfgFile := range configs {
		cfg, err := parseConfig(cfgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s failed: %v\n", cfgFile, err)
			os.Exit(1)
		}

		if cfg.Schema != "openai/chat/v1" {
			fmt.Fprintf(os.Stderr, "skip %s: unsupported schema %s\n", cfgFile, cfg.Schema)
			continue
		}

		for _, m := range cfg.Models {
			rep := probeModel(cfgFile, cfg, m)
			totalMismatches += len(rep.Mismatches)
			reports = append(reports, rep)
		}
	}

	printTable(reports)

	if totalMismatches > 0 {
		fmt.Printf("\nresult: %d mismatches found (actual vs expect).\n", totalMismatches)
		os.Exit(2)
	}
	fmt.Println("\nresult: all model expectations matched.")
}

func loadConfigs() ([]string, error) {
	files, err := fs.Glob(embeddedModels, "models/*_openai.json")
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no models/*_openai.json found")
	}
	sort.Strings(files)
	return files, nil
}

func parseConfig(file string) (*modelConfig, error) {
	b, err := embeddedModels.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var cfg modelConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func probeModel(file string, cfg *modelConfig, m modelEntry) modelReport {
	rep := modelReport{
		ProviderFile: strings.TrimPrefix(file, "models/"),
		ModelName:    m.Name,
	}

	endpoint := strings.TrimSpace(cfg.BaseURL)
	if strings.TrimSpace(m.BaseURL) != "" {
		endpoint = strings.TrimSpace(m.BaseURL)
	}
	if endpoint == "" {
		rep.Notes = append(rep.Notes, "empty base_url")
		rep.Generate = probeResult{Skipped: true, Detail: "empty base_url"}
		rep.InvokeJSON = probeResult{Skipped: true, Detail: "empty base_url"}
		rep.InvokeTool = probeResult{Skipped: true, Detail: "empty base_url"}
		return rep
	}

	model := strings.TrimSpace(m.Model)
	if model == "" {
		rep.Notes = append(rep.Notes, "empty model")
		rep.Generate = probeResult{Skipped: true, Detail: "empty model"}
		rep.InvokeJSON = probeResult{Skipped: true, Detail: "empty model"}
		rep.InvokeTool = probeResult{Skipped: true, Detail: "empty model"}
		return rep
	}

	key, ok := loadKey(cfg.APIKey)
	if !ok {
		rep.Notes = append(rep.Notes, fmt.Sprintf("missing key: %s", cfg.APIKey))
		rep.Generate = probeResult{Skipped: true, Detail: "missing api key"}
		rep.InvokeJSON = probeResult{Skipped: true, Detail: "missing api key"}
		rep.InvokeTool = probeResult{Skipped: true, Detail: "missing api key"}
		return rep
	}

	maxTokens := m.InvokeParams.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 256
	}

	rep.Generate = runProbe(func(ctx context.Context) (string, error) {
		return runOpenAIGenerate(ctx, endpoint, key, model)
	})
	rep.InvokeJSON = runProbe(func(ctx context.Context) (string, error) {
		return runOpenAIInvokeJSONOutput(ctx, endpoint, key, model, maxTokens)
	})
	rep.InvokeTool = runProbe(func(ctx context.Context) (string, error) {
		return runOpenAIInvokeToolCalls(ctx, endpoint, key, model, maxTokens)
	})

	invalidKey := key + "-invalid"
	invalidGenerate := runProbe(func(ctx context.Context) (string, error) {
		return runOpenAIGenerate(ctx, endpoint, invalidKey, model)
	})
	invalidJSON := runProbe(func(ctx context.Context) (string, error) {
		return runOpenAIInvokeJSONOutput(ctx, endpoint, invalidKey, model, maxTokens)
	})
	invalidTool := runProbe(func(ctx context.Context) (string, error) {
		return runOpenAIInvokeToolCalls(ctx, endpoint, invalidKey, model, maxTokens)
	})

	expect := cfg.Expect
	if m.Expect != nil {
		expect = *m.Expect
	}

	rep.Mismatches = append(rep.Mismatches, compareExpectation("generate(valid)", expect.GenerateSuccess, rep.Generate)...)
	rep.Mismatches = append(rep.Mismatches, compareExpectation("invoke(json,valid)", expect.InvokeJSONOutputSuccess, rep.InvokeJSON)...)
	rep.Mismatches = append(rep.Mismatches, compareExpectation("invoke(tool,valid)", expect.InvokeToolCallsSuccess, rep.InvokeTool)...)
	rep.Mismatches = append(rep.Mismatches, compareExpectation("generate(invalid_key)", expect.InvalidKeyGenerateSuccess, invalidGenerate)...)
	rep.Mismatches = append(rep.Mismatches, compareExpectation("invoke(json,invalid_key)", expect.InvalidKeyInvokeJSONOutputSuccess, invalidJSON)...)
	rep.Mismatches = append(rep.Mismatches, compareExpectation("invoke(tool,invalid_key)", expect.InvalidKeyInvokeToolCallsSuccess, invalidTool)...)

	if rep.InvokeJSON.Success == false && !rep.InvokeJSON.Skipped {
		rep.Notes = append(rep.Notes, "json_output: "+classifyError(rep.InvokeJSON.Detail))
	}
	if rep.InvokeTool.Success == false && !rep.InvokeTool.Skipped {
		rep.Notes = append(rep.Notes, "tool_calls: "+classifyError(rep.InvokeTool.Detail))
	}

	if len(rep.Mismatches) > 0 {
		rep.Notes = append(rep.Notes, "mismatch="+strings.Join(rep.Mismatches, "; "))
	}

	return rep
}

func runProbe(call func(context.Context) (string, error)) probeResult {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	text, err := call(ctx)
	if err != nil {
		if isConnectivityErr(err) {
			return probeResult{Skipped: true, Connectivity: true, Detail: err.Error()}
		}
		return probeResult{Success: false, Detail: err.Error()}
	}
	if strings.TrimSpace(text) == "" {
		return probeResult{Success: false, Detail: "empty response"}
	}
	return probeResult{Success: true, Detail: short(text)}
}

func compareExpectation(caseName string, expected bool, actual probeResult) []string {
	if actual.Skipped {
		return nil
	}
	if actual.Success == expected {
		return nil
	}
	return []string{fmt.Sprintf("%s expected=%v actual=%v", caseName, expected, actual.Success)}
}

func runOpenAIGenerate(ctx context.Context, endpoint, key, model string) (string, error) {
	client := openai.NewClient(option.WithBaseURL(endpoint), option.WithAPIKey(key))
	g := &genx.OpenAIGenerator{Client: &client, Model: model}
	stream, err := g.GenerateStream(ctx, "", buildSimpleContext("reply with one word: ok"))
	if err != nil {
		return "", err
	}
	return readTextStream(stream)
}

func runOpenAIInvokeJSONOutput(ctx context.Context, endpoint, key, model string, maxTokens int) (string, error) {
	client := openai.NewClient(option.WithBaseURL(endpoint), option.WithAPIKey(key))
	g := &genx.OpenAIGenerator{
		Client:            &client,
		Model:             model,
		SupportJSONOutput: true,
		InvokeParams:      &genx.ModelParams{MaxTokens: maxTokens},
	}
	tool := genx.MustNewFuncTool[invokeArg]("extract_result", "extract structured result")
	_, call, err := g.Invoke(ctx, "", buildSimpleContext("return json with field result set to ok"), tool)
	if err != nil {
		return "", err
	}
	if call == nil {
		return "", errors.New("openai invoke(json_output) returned nil function call")
	}
	return call.Arguments, nil
}

func runOpenAIInvokeToolCalls(ctx context.Context, endpoint, key, model string, maxTokens int) (string, error) {
	client := openai.NewClient(option.WithBaseURL(endpoint), option.WithAPIKey(key))
	g := &genx.OpenAIGenerator{
		Client:             &client,
		Model:              model,
		SupportToolCalls:   true,
		InvokeWithToolName: true,
		InvokeParams:       &genx.ModelParams{MaxTokens: maxTokens},
	}
	tool := genx.MustNewFuncTool[invokeArg]("extract_result", "extract structured result")
	_, call, err := g.Invoke(ctx, "", buildSimpleContext("call tool extract_result and return json result"), tool)
	if err != nil {
		return "", err
	}
	if call == nil {
		return "", errors.New("openai invoke(tool_calls) returned nil function call")
	}
	return call.Arguments, nil
}

func buildSimpleContext(text string) genx.ModelContext {
	var mcb genx.ModelContextBuilder
	mcb.UserText("user", text)
	return mcb.Build()
}

func readTextStream(stream genx.Stream) (string, error) {
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

func loadKey(ref string) (string, bool) {
	envName := strings.TrimPrefix(strings.TrimSpace(ref), "$")
	if envName == "" || envName == ref {
		return "", false
	}
	candidates := keyCandidates(envName)
	for _, name := range candidates {
		v := strings.TrimSpace(os.Getenv(name))
		if v != "" {
			return v, true
		}
	}
	vars, err := loadBazelRCUserActionEnv()
	if err == nil {
		for _, name := range candidates {
			v := strings.TrimSpace(vars[name])
			if v != "" {
				return v, true
			}
		}
	}
	return "", false
}

func keyCandidates(envName string) []string {
	switch envName {
	case "MINIMAX_CN_API_KEY":
		return []string{"MINIMAX_CN_API_KEY", "MINIMAX_API_KEY"}
	default:
		return []string{envName}
	}
}

func loadBazelRCUserActionEnv() (map[string]string, error) {
	bazelEnvOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			bazelEnvErr = err
			return
		}
		path := envOr("GX_BAZELRC_USER", filepath.Join(home, "Vibing", "gizclaw", "main", ".bazelrc.user"))
		content, err := os.ReadFile(path)
		if err != nil {
			bazelEnvErr = err
			return
		}

		m := make(map[string]string)
		for line := range strings.SplitSeq(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			_, after, ok := strings.Cut(line, "--action_env=")
			if !ok {
				continue
			}
			envKV := strings.TrimSpace(after)
			k, v, ok := strings.Cut(envKV, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if k == "" || v == "" {
				continue
			}
			m[k] = v
		}
		bazelEnvData = m
	})

	if bazelEnvErr != nil {
		return nil, bazelEnvErr
	}
	return bazelEnvData, nil
}

func printTable(reports []modelReport) {
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "MODEL\tJSON_OUTPUT\tTOOL_CALLS\tGENERATE\tEXPECT\tNOTES")
	for _, r := range reports {
		expect := "ok"
		if len(r.Mismatches) > 0 {
			expect = "mismatch"
		}
		notes := strings.Join(r.Notes, " | ")
		if notes == "" {
			notes = "-"
		}
		fmt.Fprintf(
			w,
			"%s (%s)\t%s\t%s\t%s\t%s\t%s\n",
			r.ModelName,
			r.ProviderFile,
			probeSupport(r.InvokeJSON),
			probeSupport(r.InvokeTool),
			probeSupport(r.Generate),
			expect,
			notes,
		)
	}
	_ = w.Flush()
}

func probeSupport(r probeResult) string {
	if r.Skipped {
		return "unknown"
	}
	if r.Success {
		return "yes"
	}
	return "no"
}

func isConnectivityErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "network is unreachable")
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func short(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= 180 {
		return s
	}
	return s[:180] + "..."
}

func classifyError(detail string) string {
	d := strings.ToLower(detail)
	switch {
	case strings.Contains(d, "response_format") && strings.Contains(d, "unavailable"):
		return "response_format_unsupported"
	case strings.Contains(d, "unexpected finish reason: stop"):
		return "finish_reason_stop_with_tool_calls"
	case strings.Contains(d, "unauthorized") || strings.Contains(d, "authentication"):
		return "auth_failed"
	case strings.Contains(d, "max_completion_tokens"):
		return "max_completion_tokens_unsupported"
	default:
		return short(detail)
	}
}
