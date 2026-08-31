package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const sourceTimeLayout = "3:04 pm on 2 January, 2006"

type options struct {
	input            string
	output           string
	manifest         string
	conversationID   string
	sourceRepository string
	sourceCommit     string
	sourcePath       string
	sourceSHA256     string
	licensePath      string
	licenseURL       string
	licenseSPDX      string
}

type sourceSample struct {
	SampleID     string                     `json:"sample_id"`
	Conversation map[string]json.RawMessage `json:"conversation"`
	Questions    []sourceQuestion           `json:"qa"`
}

type sourceTurn struct {
	Speaker string `json:"speaker"`
	ID      string `json:"dia_id"`
	Text    string `json:"text"`
}

type sourceQuestion struct {
	Question string          `json:"question"`
	Answer   json.RawMessage `json:"answer"`
	Evidence []string        `json:"evidence"`
	Category int             `json:"category"`
}

type fixtureConversation struct {
	Type                   string        `json:"type"`
	ID                     string        `json:"id"`
	MinimumFactsPerSession int           `json:"minimum_facts_per_session"`
	Turns                  []fixtureTurn `json:"turns"`
}

type fixtureTurn struct {
	Role       string    `json:"role"`
	Speaker    string    `json:"speaker"`
	Content    string    `json:"content"`
	EvidenceID string    `json:"evidence_id"`
	SessionID  string    `json:"session_id"`
	ObservedAt time.Time `json:"observed_at"`
}

type fixtureQuestion struct {
	Type           string   `json:"type"`
	ID             string   `json:"id"`
	ConversationID string   `json:"conversation_id"`
	Query          string   `json:"query"`
	GoldAnswers    []string `json:"gold_answers"`
	EvidenceIDs    []string `json:"evidence_ids"`
	Tags           []string `json:"tags"`
	Category       int      `json:"category"`
	Answerable     bool     `json:"answerable"`
}

type manifest struct {
	Derived manifestDerived `json:"derived"`
	License manifestLicense `json:"license"`
	Name    string          `json:"name"`
	Source  manifestSource  `json:"source"`
}

type manifestDerived struct {
	Changes         string   `json:"changes"`
	ConversationIDs []string `json:"conversation_ids"`
	Path            string   `json:"path"`
	QuestionIDs     []string `json:"question_ids"`
	SessionIDs      []string `json:"session_ids"`
	SHA256          string   `json:"sha256"`
	TurnCount       int      `json:"turn_count"`
}

type manifestLicense struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	SPDX   string `json:"spdx"`
	URL    string `json:"url"`
	Use    string `json:"use"`
}

type manifestSource struct {
	Commit     string `json:"commit"`
	Path       string `json:"path"`
	Repository string `json:"repository"`
	SHA256     string `json:"sha256"`
}

func main() {
	configuration := parseFlags()
	if err := run(configuration); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() options {
	var configuration options
	flag.StringVar(&configuration.input, "input", "", "path to upstream locomo10.json")
	flag.StringVar(&configuration.output, "output", "", "path for the derived JSONL fixture")
	flag.StringVar(&configuration.manifest, "manifest", "", "path for the derived fixture manifest")
	flag.StringVar(&configuration.conversationID, "conversation", "", "upstream conversation ID to select")
	flag.StringVar(&configuration.sourceRepository, "source-repository", "https://github.com/snap-research/locomo", "upstream repository URL")
	flag.StringVar(&configuration.sourceCommit, "source-commit", "", "pinned upstream commit")
	flag.StringVar(&configuration.sourcePath, "source-path", "data/locomo10.json", "path within the upstream repository")
	flag.StringVar(&configuration.sourceSHA256, "source-sha256", "", "expected SHA-256 of the upstream input")
	flag.StringVar(&configuration.licensePath, "license", "", "path to the copied upstream license")
	flag.StringVar(&configuration.licenseURL, "license-url", "https://creativecommons.org/licenses/by-nc/4.0/", "upstream license URL")
	flag.StringVar(&configuration.licenseSPDX, "license-spdx", "CC-BY-NC-4.0", "upstream SPDX identifier")
	flag.Parse()
	return configuration
}

func run(configuration options) error {
	if configuration.input == "" || configuration.output == "" || configuration.manifest == "" ||
		configuration.conversationID == "" || configuration.sourceCommit == "" ||
		configuration.sourceSHA256 == "" || configuration.licensePath == "" {
		return errors.New("input, output, manifest, conversation, source-commit, source-sha256, and license are required")
	}
	source, err := os.ReadFile(configuration.input)
	if err != nil {
		return fmt.Errorf("read source dataset: %w", err)
	}
	sourceHash := sha256Hex(source)
	if sourceHash != configuration.sourceSHA256 {
		return fmt.Errorf("source dataset SHA-256 is %s, want %s", sourceHash, configuration.sourceSHA256)
	}
	license, err := os.ReadFile(configuration.licensePath)
	if err != nil {
		return fmt.Errorf("read license: %w", err)
	}
	var samples []sourceSample
	if err := json.Unmarshal(source, &samples); err != nil {
		return fmt.Errorf("decode source dataset: %w", err)
	}
	selected, err := selectSample(samples, configuration.conversationID)
	if err != nil {
		return err
	}
	fixture, metadata, err := convertSample(selected)
	if err != nil {
		return err
	}
	if err := writeFile(configuration.output, fixture); err != nil {
		return fmt.Errorf("write fixture: %w", err)
	}
	metadata.Derived.Path = filepath.Base(configuration.output)
	metadata.Derived.SHA256 = sha256Hex(fixture)
	metadata.License = manifestLicense{
		Path: filepath.Base(configuration.licensePath), SHA256: sha256Hex(license),
		SPDX: configuration.licenseSPDX, URL: configuration.licenseURL, Use: "noncommercial only",
	}
	metadata.Name = strings.TrimSuffix(filepath.Base(configuration.output), filepath.Ext(configuration.output))
	metadata.Source = manifestSource{
		Commit: configuration.sourceCommit, Path: configuration.sourcePath,
		Repository: configuration.sourceRepository, SHA256: sourceHash,
	}
	encodedManifest, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	encodedManifest = append(encodedManifest, '\n')
	if err := writeFile(configuration.manifest, encodedManifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

func selectSample(samples []sourceSample, conversationID string) (sourceSample, error) {
	for _, sample := range samples {
		if sample.SampleID == conversationID {
			return sample, nil
		}
	}
	return sourceSample{}, fmt.Errorf("conversation %q is not present in source dataset", conversationID)
}

func convertSample(sample sourceSample) ([]byte, manifest, error) {
	sessionNumbers := make([]int, 0)
	for key := range sample.Conversation {
		if !strings.HasPrefix(key, "session_") || strings.HasSuffix(key, "_date_time") {
			continue
		}
		number, err := strconv.Atoi(strings.TrimPrefix(key, "session_"))
		if err != nil {
			return nil, manifest{}, fmt.Errorf("conversation %q has invalid session key %q", sample.SampleID, key)
		}
		sessionNumbers = append(sessionNumbers, number)
	}
	sort.Ints(sessionNumbers)
	conversation := fixtureConversation{Type: "conversation", ID: sample.SampleID, MinimumFactsPerSession: 1}
	sessionIDs := make([]string, 0, len(sessionNumbers))
	for _, number := range sessionNumbers {
		sessionID := "session_" + strconv.Itoa(number)
		var observedAtSource string
		if err := json.Unmarshal(sample.Conversation[sessionID+"_date_time"], &observedAtSource); err != nil {
			return nil, manifest{}, fmt.Errorf("conversation %q %s timestamp: %w", sample.SampleID, sessionID, err)
		}
		observedAt, err := time.Parse(sourceTimeLayout, observedAtSource)
		if err != nil {
			return nil, manifest{}, fmt.Errorf("conversation %q %s timestamp %q: %w", sample.SampleID, sessionID, observedAtSource, err)
		}
		var turns []sourceTurn
		if err := json.Unmarshal(sample.Conversation[sessionID], &turns); err != nil {
			return nil, manifest{}, fmt.Errorf("conversation %q %s turns: %w", sample.SampleID, sessionID, err)
		}
		for index, turn := range turns {
			role := "assistant"
			if index%2 == 1 {
				role = "user"
			}
			conversation.Turns = append(conversation.Turns, fixtureTurn{
				Role: role, Speaker: turn.Speaker, Content: turn.Text,
				EvidenceID: sample.SampleID + ":" + turn.ID, SessionID: sessionID,
				ObservedAt: observedAt.UTC(),
			})
		}
		sessionIDs = append(sessionIDs, sample.SampleID+":"+sessionID)
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(conversation); err != nil {
		return nil, manifest{}, fmt.Errorf("encode conversation: %w", err)
	}
	questionIDs := make([]string, 0, len(sample.Questions))
	for index, source := range sample.Questions {
		questionID := sample.SampleID + "-q" + strconv.Itoa(index+1)
		answerable := source.Category != 5
		var answers []string
		if answerable {
			answer, err := decodeAnswer(source.Answer)
			if err != nil {
				return nil, manifest{}, fmt.Errorf("question %q answer: %w", questionID, err)
			}
			answers = []string{answer}
		}
		tag, err := categoryTag(source.Category)
		if err != nil {
			return nil, manifest{}, fmt.Errorf("question %q: %w", questionID, err)
		}
		evidence := make([]string, len(source.Evidence))
		for evidenceIndex, evidenceID := range source.Evidence {
			evidence[evidenceIndex] = sample.SampleID + ":" + evidenceID
		}
		question := fixtureQuestion{
			Type: "question", ID: questionID, ConversationID: sample.SampleID,
			Query: source.Question, GoldAnswers: answers, EvidenceIDs: evidence,
			Tags:     []string{"cat" + strconv.Itoa(source.Category), tag},
			Category: source.Category, Answerable: answerable,
		}
		if err := encoder.Encode(question); err != nil {
			return nil, manifest{}, fmt.Errorf("encode question %q: %w", questionID, err)
		}
		questionIDs = append(questionIDs, questionID)
	}
	metadata := manifest{Derived: manifestDerived{
		Changes:         "Selected the complete " + sample.SampleID + " conversation and all of its QA records. Category 5 is represented explicitly as unanswerable with no gold answer. Source speaker names and session wall-clock timestamps are preserved on every turn; timezone-free source values are mapped onto UTC only for deterministic Go timestamps. Roles preserve the existing fixture convention of alternating from assistant at the start of each session. Each session requires at least one materialized fact. Image metadata and generated summaries are excluded.",
		ConversationIDs: []string{sample.SampleID}, QuestionIDs: questionIDs,
		SessionIDs: sessionIDs, TurnCount: len(conversation.Turns),
	}}
	return output.Bytes(), metadata, nil
}

func decodeAnswer(raw json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	switch value := value.(type) {
	case string:
		return value, nil
	case json.Number:
		return value.String(), nil
	default:
		return "", fmt.Errorf("must be a string or number, got %T", value)
	}
}

func categoryTag(category int) (string, error) {
	tag, exists := map[int]string{1: "multi-hop", 2: "temporal", 3: "commonsense", 4: "single-hop", 5: "adversarial"}[category]
	if !exists {
		return "", fmt.Errorf("category must be 1 through 5, got %d", category)
	}
	return tag, nil
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".locomo-fixture-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
