package giztest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

const (
	defaultTaskTimeout = 5 * time.Minute
	maxDocumentBytes   = 4 << 20
)

var (
	namePattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	referencePattern = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
)

type Document struct {
	Path      string                  `json:"-" yaml:"-"`
	Version   string                  `json:"version" yaml:"version"`
	Name      string                  `json:"name" yaml:"name"`
	Repeat    int                     `json:"repeat,omitempty" yaml:"repeat,omitempty"`
	Timeout   string                  `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Clients   map[string]ClientSpec   `json:"clients" yaml:"clients"`
	Variables map[string]VariableSpec `json:"variables" yaml:"variables"`
	Steps     []Step                  `json:"steps" yaml:"steps"`
	Finally   []Step                  `json:"finally,omitempty" yaml:"finally,omitempty"`
	Review    bool                    `json:"review,omitempty" yaml:"review,omitempty"`
	Report    ReportSpec              `json:"report" yaml:"report,omitempty"`
}

type ReportSpec struct {
	Redact []string `json:"redact,omitempty" yaml:"redact,omitempty"`
}

type ClientSpec struct {
	Identity          string `json:"identity" yaml:"identity"`
	Connection        string `json:"connection" yaml:"connection"`
	AccessPoint       string `json:"access_point" yaml:"access_point"`
	RegistrationToken string `json:"registration_token,omitempty" yaml:"registration_token,omitempty"`
}

type VariableSpec struct {
	Direction string `json:"direction" yaml:"direction"`
	Type      string `json:"type" yaml:"type"`
	Value     any    `json:"value,omitempty" yaml:"value,omitempty"`
	Env       string `json:"env,omitempty" yaml:"env,omitempty"`
	Generate  string `json:"generate,omitempty" yaml:"generate,omitempty"`
	Secret    bool   `json:"secret,omitempty" yaml:"secret,omitempty"`
	MaxBytes  int    `json:"max_bytes,omitempty" yaml:"max_bytes,omitempty"`
	MediaType string `json:"media_type,omitempty" yaml:"media_type,omitempty"`
	Codec     string `json:"codec,omitempty" yaml:"codec,omitempty"`
}

type Step struct {
	ID             string                   `json:"id,omitempty" yaml:"id,omitempty"`
	Client         string                   `json:"client,omitempty" yaml:"client,omitempty"`
	RPC            *RPCOperation            `json:"rpc,omitempty" yaml:"rpc,omitempty"`
	RPCStream      *RPCStreamOperation      `json:"rpc_stream,omitempty" yaml:"rpc_stream,omitempty"`
	ClientRPC      *ClientRPCOperation      `json:"client_rpc,omitempty" yaml:"client_rpc,omitempty"`
	Speech         *SpeechOperation         `json:"speech,omitempty" yaml:"speech,omitempty"`
	PeerStream     *PeerStreamOperation     `json:"peer_stream,omitempty" yaml:"peer_stream,omitempty"`
	Output         *OutputOperation         `json:"output,omitempty" yaml:"output,omitempty"`
	ReviewOp       *ReviewOperation         `json:"review_op,omitempty" yaml:"review_op,omitempty"`
	Barrier        *BarrierOperation        `json:"barrier,omitempty" yaml:"barrier,omitempty"`
	WorkspaceRelay *WorkspaceRelayOperation `json:"workspace_relay,omitempty" yaml:"workspace_relay,omitempty"`
	SaveAs         string                   `json:"save_as,omitempty" yaml:"save_as,omitempty"`
	Capture        map[string]string        `json:"capture,omitempty" yaml:"capture,omitempty"`
	Expect         map[string]Expectation   `json:"expect,omitempty" yaml:"expect,omitempty"`
	ExpectError    *ErrorExpectation        `json:"expect_error,omitempty" yaml:"expect_error,omitempty"`
	Timeout        string                   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

type ErrorExpectation struct {
	Code            int32  `json:"code" yaml:"code"`
	MessageContains string `json:"message_contains,omitempty" yaml:"message_contains,omitempty"`
}

type RPCOperation struct {
	Method  string `json:"method" yaml:"method"`
	Request any    `json:"request" yaml:"request"`
}
type RPCStreamOperation struct {
	Method   string `json:"method" yaml:"method"`
	Request  any    `json:"request" yaml:"request"`
	MaxBytes int    `json:"max_bytes,omitempty" yaml:"max_bytes,omitempty"`
}
type ClientRPCOperation struct {
	Method      string `json:"method" yaml:"method"`
	Response    any    `json:"response,omitempty" yaml:"response,omitempty"`
	ExpectCalls int    `json:"expect_calls,omitempty" yaml:"expect_calls,omitempty"`
}
type SpeechOperation struct {
	Method  string `json:"method" yaml:"method"`
	Request any    `json:"request" yaml:"request"`
	Input   any    `json:"input,omitempty" yaml:"input,omitempty"`
	Cache   string `json:"cache,omitempty" yaml:"cache,omitempty"`
}
type PeerStreamOperation struct {
	Mode           string `json:"mode" yaml:"mode"`
	Input          any    `json:"input,omitempty" yaml:"input,omitempty"`
	Pacing         string `json:"pacing,omitempty" yaml:"pacing,omitempty"`
	InterruptAfter string `json:"interrupt_after,omitempty" yaml:"interrupt_after,omitempty"`
	TerminalLabel  string `json:"terminal_label,omitempty" yaml:"terminal_label,omitempty"`
	RequireText    *bool  `json:"require_text,omitempty" yaml:"require_text,omitempty"`
	RequireAudio   *bool  `json:"require_audio,omitempty" yaml:"require_audio,omitempty"`
	WaitForHistory bool   `json:"wait_for_history,omitempty" yaml:"wait_for_history,omitempty"`
}
type OutputOperation struct {
	Variable string `json:"variable" yaml:"variable"`
}
type ReviewOperation struct {
	Message string `json:"message" yaml:"message"`
}
type BarrierOperation struct {
	Participants int `json:"participants,omitempty" yaml:"participants,omitempty"`
}
type WorkspaceRelayOperation struct {
	FirstClient    string `json:"first_client" yaml:"first_client"`
	SecondClient   string `json:"second_client" yaml:"second_client"`
	Input          any    `json:"input" yaml:"input"`
	Media          string `json:"media" yaml:"media"`
	MaxTurns       int    `json:"max_turns" yaml:"max_turns"`
	TerminalClient string `json:"terminal_client" yaml:"terminal_client"`
}
type Expectation struct {
	Equals      any      `json:"equals,omitempty" yaml:"equals,omitempty"`
	Present     *bool    `json:"present,omitempty" yaml:"present,omitempty"`
	NonEmpty    *bool    `json:"non_empty,omitempty" yaml:"non_empty,omitempty"`
	Count       *int     `json:"count,omitempty" yaml:"count,omitempty"`
	Contains    string   `json:"contains,omitempty" yaml:"contains,omitempty"`
	ContainsAll []string `json:"contains_all,omitempty" yaml:"contains_all,omitempty"`
	ContainsAny []string `json:"contains_any,omitempty" yaml:"contains_any,omitempty"`
	NotContains any      `json:"not_contains,omitempty" yaml:"not_contains,omitempty"`
	Pattern     string   `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	MinLength   *int     `json:"min_length,omitempty" yaml:"min_length,omitempty"`
	MaxLength   *int     `json:"max_length,omitempty" yaml:"max_length,omitempty"`
}

const (
	maxNeedleRunes  = 256
	maxNeedleCount  = 16
	maxLengthBound  = 1 << 20
	maxPatternBytes = 256
)

// notContainsList normalizes the YAML string-or-list operand into one list.
func (e Expectation) notContainsList() ([]string, error) {
	switch v := e.NotContains.(type) {
	case nil:
		return nil, nil
	case string:
		return []string{v}, nil
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("not_contains entries must be strings")
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("not_contains must be a string or a list of strings")
}

func (e Expectation) hasValueMatcher() bool {
	notContains, _ := e.notContainsList()
	return e.Equals != nil || e.NonEmpty != nil || e.Count != nil ||
		e.Contains != "" || len(e.ContainsAll) > 0 || len(e.ContainsAny) > 0 ||
		len(notContains) > 0 || e.Pattern != "" ||
		e.Minimum != nil || e.Maximum != nil || e.MinLength != nil || e.MaxLength != nil
}

func validateNeedles(matcher string, needles []string) error {
	if len(needles) > maxNeedleCount {
		return fmt.Errorf("%s allows at most %d entries", matcher, maxNeedleCount)
	}
	for _, needle := range needles {
		if needle == "" {
			return fmt.Errorf("%s entries must be non-empty", matcher)
		}
		if utf8.RuneCountInString(needle) > maxNeedleRunes {
			return fmt.Errorf("%s entries must be at most %d runes", matcher, maxNeedleRunes)
		}
	}
	return nil
}

func (e Expectation) validate() error {
	notContains, err := e.notContainsList()
	if err != nil {
		return err
	}
	if e.NotContains != nil && len(notContains) == 0 {
		return fmt.Errorf("not_contains requires at least one entry")
	}
	for matcher, needles := range map[string][]string{
		"contains":     nonEmptyList(e.Contains),
		"contains_all": e.ContainsAll,
		"contains_any": e.ContainsAny,
		"not_contains": notContains,
	} {
		if err := validateNeedles(matcher, needles); err != nil {
			return err
		}
	}
	if (e.ContainsAll != nil && len(e.ContainsAll) == 0) || (e.ContainsAny != nil && len(e.ContainsAny) == 0) {
		return fmt.Errorf("contains_all and contains_any require at least one entry")
	}
	if e.Pattern != "" {
		if len(e.Pattern) > maxPatternBytes {
			return fmt.Errorf("pattern must be at most %d bytes", maxPatternBytes)
		}
		if _, err := regexp.Compile(e.Pattern); err != nil {
			return fmt.Errorf("pattern does not compile as RE2")
		}
	}
	for matcher, bound := range map[string]*int{"min_length": e.MinLength, "max_length": e.MaxLength} {
		if bound != nil && (*bound < 0 || *bound > maxLengthBound) {
			return fmt.Errorf("%s must be between 0 and %d", matcher, maxLengthBound)
		}
	}
	if e.MinLength != nil && e.MaxLength != nil && *e.MinLength > *e.MaxLength {
		return fmt.Errorf("min_length exceeds max_length")
	}
	if e.Minimum != nil && e.Maximum != nil && *e.Minimum > *e.Maximum {
		return fmt.Errorf("minimum exceeds maximum")
	}
	if e.Present != nil && !*e.Present && e.hasValueMatcher() {
		return fmt.Errorf("present: false cannot combine with value matchers")
	}
	return nil
}

func nonEmptyList(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}

func loadDocument(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > maxDocumentBytes {
		return nil, fmt.Errorf("document exceeds %d bytes", maxDocumentBytes)
	}
	if err := validateUserStory(data); err != nil {
		return nil, err
	}
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	jsonData, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("convert YAML: %w", err)
	}
	if err := validateSchema(bytes.NewReader(jsonData)); err != nil {
		return nil, err
	}
	var doc Document
	if err := yaml.UnmarshalWithOptions(data, &doc, yaml.DisallowUnknownField()); err != nil {
		return nil, fmt.Errorf("strict decode: %w", err)
	}
	doc.Path = path
	if doc.Repeat == 0 {
		doc.Repeat = 1
	}
	if err := doc.validateSemantics(); err != nil {
		return nil, err
	}
	return &doc, nil
}

func validateUserStory(data []byte) error {
	lines := strings.Split(string(data), "\n")
	want := []string{"User Story:", "As a ", "I want ", "So that "}
	got := make([]string, 0, 4)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			break
		}
		got = append(got, strings.TrimSpace(strings.TrimPrefix(line, "#")))
		if len(got) == 4 {
			break
		}
	}
	if len(got) != 4 {
		return fmt.Errorf("first content must be four User Story comment lines")
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) || (i > 0 && strings.TrimSpace(strings.TrimPrefix(got[i], want[i])) == "") {
			return fmt.Errorf("User Story line %d must start with %q and contain detail", i+1, want[i])
		}
	}
	return nil
}

func (d *Document) validateSemantics() error {
	if d.Version != "gizclaw.test/v1alpha1" || !namePattern.MatchString(d.Name) {
		return fmt.Errorf("invalid version or name")
	}
	if _, err := d.taskTimeout(); err != nil {
		return err
	}
	for name, c := range d.Clients {
		if !isReferenceOrLiteral(c.AccessPoint) {
			return fmt.Errorf("client %s has invalid access_point", name)
		}
		if c.RegistrationToken != "" && !isReferenceOrLiteral(c.RegistrationToken) {
			return fmt.Errorf("client %s has invalid registration_token", name)
		}
	}
	for _, name := range d.Report.Redact {
		if _, ok := d.Variables[name]; !ok {
			return fmt.Errorf("report redact references unknown variable %q", name)
		}
	}
	produced := map[string]bool{}
	for name, v := range d.Variables {
		produced[name] = v.Direction == "input"
	}
	for i := range d.Finally {
		if d.Finally[i].ID == "" {
			d.Finally[i].ID = fmt.Sprintf("finally_%d", i+1)
		}
	}
	ids := map[string]bool{}
	workspaceSelected := map[string]bool{}
	all := append(append([]Step(nil), d.Steps...), d.Finally...)
	for i, step := range all {
		if i < len(d.Steps) && step.RPC != nil && step.RPC.Method == "server.run.workspace.set" && step.Client != "" {
			workspaceSelected[step.Client] = true
		}
		if step.WorkspaceRelay != nil {
			if i >= len(d.Steps) {
				return fmt.Errorf("step %s workspace_relay is not allowed in finally", step.ID)
			}
			if err := d.validateWorkspaceRelay(step, workspaceSelected); err != nil {
				return err
			}
		}
		if step.ID == "" {
			return fmt.Errorf("step %d requires id", i+1)
		}
		op := step.operation()
		if ids[step.ID] {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		ids[step.ID] = true
		if step.Client != "" {
			if _, ok := d.Clients[step.Client]; !ok {
				return fmt.Errorf("step %s references unknown client %q", step.ID, step.Client)
			}
		}
		if operationNeedsClient(op) && step.Client == "" {
			return fmt.Errorf("step %s operation %s requires client", step.ID, op)
		}
		for _, name := range collectReferences(step) {
			if !produced[name] {
				return fmt.Errorf("step %s references unavailable variable %q", step.ID, name)
			}
		}
		for _, name := range append(mapKeys(step.Capture), step.SaveAs) {
			if name == "" {
				continue
			}
			v, ok := d.Variables[name]
			if !ok || v.Direction != "output" {
				return fmt.Errorf("step %s assigns non-output variable %q", step.ID, name)
			}
			if produced[name] {
				return fmt.Errorf("output variable %q has multiple producers", name)
			}
			produced[name] = true
		}
		if op == "review" && !d.Review {
			return fmt.Errorf("step %s requires top-level review: true", step.ID)
		}
		if op == "barrier" && step.Barrier.Participants != 0 && step.Barrier.Participants != d.Repeat {
			return fmt.Errorf("step %s barrier participants must equal repeat", step.ID)
		}
		if step.Timeout != "" {
			if duration, err := time.ParseDuration(step.Timeout); err != nil || duration <= 0 {
				return fmt.Errorf("step %s has invalid timeout %q", step.ID, step.Timeout)
			}
		}
		for path, expectation := range step.Expect {
			if err := expectation.validate(); err != nil {
				return fmt.Errorf("step %s expect %s: %w", step.ID, path, err)
			}
		}
		if step.PeerStream != nil {
			if step.PeerStream.Input == nil {
				return fmt.Errorf("step %s peer_stream requires input", step.ID)
			}
			if step.PeerStream.Pacing != "" {
				if duration, err := time.ParseDuration(step.PeerStream.Pacing); err != nil || duration < 0 {
					return fmt.Errorf("step %s has invalid pacing %q", step.ID, step.PeerStream.Pacing)
				}
			}
			if step.PeerStream.InterruptAfter != "" {
				if duration, err := time.ParseDuration(step.PeerStream.InterruptAfter); err != nil || duration <= 0 {
					return fmt.Errorf("step %s has invalid interrupt_after %q", step.ID, step.PeerStream.InterruptAfter)
				}
			}
			if step.PeerStream.RequireText != nil && step.PeerStream.RequireAudio != nil && !*step.PeerStream.RequireText && !*step.PeerStream.RequireAudio {
				return fmt.Errorf("step %s peer_stream must require text, audio, or both", step.ID)
			}
		}
		if step.Speech != nil && step.Speech.Method != "server.speech.synthesize" && step.Speech.Input == nil {
			return fmt.Errorf("step %s speech %s requires input", step.ID, step.Speech.Method)
		}
		if step.Speech != nil && step.Speech.Cache != "" {
			if step.Speech.Method != "server.speech.synthesize" {
				return fmt.Errorf("step %s speech cache is only supported for synthesis", step.ID)
			}
			if step.SaveAs == "" {
				return fmt.Errorf("step %s cached speech synthesis requires save_as", step.ID)
			}
		}
		if step.RPC != nil {
			if err := validateRPCRequestShape(step.RPC.Method, step.RPC.Request, d.Variables); err != nil {
				return fmt.Errorf("step %s: %w", step.ID, err)
			}
		}
		if step.RPCStream != nil {
			if err := validateRPCRequestShape(step.RPCStream.Method, step.RPCStream.Request, d.Variables); err != nil {
				return fmt.Errorf("step %s: %w", step.ID, err)
			}
		}
	}
	return nil
}

func (d *Document) validateWorkspaceRelay(step Step, selected map[string]bool) error {
	op := step.WorkspaceRelay
	if step.Client != "" {
		return fmt.Errorf("step %s workspace_relay names its clients through first_client and second_client", step.ID)
	}
	if op.FirstClient == op.SecondClient {
		return fmt.Errorf("step %s workspace_relay requires two distinct clients", step.ID)
	}
	for _, name := range []string{op.FirstClient, op.SecondClient} {
		if _, ok := d.Clients[name]; !ok {
			return fmt.Errorf("step %s workspace_relay references unknown client %q", step.ID, name)
		}
		if !selected[name] {
			return fmt.Errorf("step %s workspace_relay requires a preceding server.run.workspace.set for client %q", step.ID, name)
		}
	}
	if op.Media != "text" && op.Media != "audio" {
		return fmt.Errorf("step %s workspace_relay media must be text or audio", step.ID)
	}
	if op.MaxTurns < 2 || op.MaxTurns > 256 {
		return fmt.Errorf("step %s workspace_relay max_turns must be between 2 and 256", step.ID)
	}
	if op.Input == nil {
		return fmt.Errorf("step %s workspace_relay requires input", step.ID)
	}
	terminal := op.FirstClient
	if op.MaxTurns%2 == 0 {
		terminal = op.SecondClient
	}
	if op.TerminalClient != terminal {
		return fmt.Errorf("step %s workspace_relay terminal_client %q does not match first_client with max_turns %d (want %q)", step.ID, op.TerminalClient, op.MaxTurns, terminal)
	}
	if step.SaveAs != "" {
		return fmt.Errorf("step %s workspace_relay does not support save_as", step.ID)
	}
	for name, pointer := range step.Capture {
		spec, ok := d.Variables[name]
		if !ok {
			continue // the shared step loop reports unknown capture variables
		}
		switch {
		case pointer == "/terminal/text" && op.Media == "text":
			if spec.Type != "string" {
				return fmt.Errorf("step %s capture %q of /terminal/text must target a string output variable", step.ID, name)
			}
		case pointer == "/terminal/audio" && op.Media == "audio":
			if spec.Type != "audio" || spec.Codec != "opus" || spec.MediaType != "audio/ogg" || spec.MaxBytes <= 0 || spec.MaxBytes > relayMaxAudioBytes {
				return fmt.Errorf("step %s capture %q of /terminal/audio must target audio/ogg opus output with max_bytes up to %d", step.ID, name, relayMaxAudioBytes)
			}
		default:
			return fmt.Errorf("step %s workspace_relay allows capturing only the terminal %s", step.ID, op.Media)
		}
	}
	return nil
}

func (d *Document) taskTimeout() (time.Duration, error) {
	if d.Timeout == "" {
		return defaultTaskTimeout, nil
	}
	v, err := time.ParseDuration(d.Timeout)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid timeout %q", d.Timeout)
	}
	return v, nil
}

func operationNeedsClient(op string) bool {
	return op != "barrier" && op != "output" && op != "review" && op != "workspace_relay"
}
func (s Step) operation() string {
	switch {
	case s.RPC != nil:
		return "rpc"
	case s.RPCStream != nil:
		return "rpc_stream"
	case s.ClientRPC != nil:
		return "client_rpc"
	case s.Speech != nil:
		return "speech"
	case s.PeerStream != nil:
		return "peer_stream"
	case s.Output != nil:
		return "output"
	case s.ReviewOp != nil:
		return "review"
	case s.Barrier != nil:
		return "barrier"
	case s.WorkspaceRelay != nil:
		return "workspace_relay"
	}
	return ""
}
func isReferenceOrLiteral(v string) bool { return strings.TrimSpace(v) != "" }
func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
func collectReferences(v any) []string {
	data, _ := json.Marshal(v)
	matches := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`).FindAllStringSubmatch(string(data), -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}
