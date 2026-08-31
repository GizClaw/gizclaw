package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunConvertsFixtureAndManifest(t *testing.T) {
	t.Parallel()
	temporaryDirectory := t.TempDir()
	sourcePath := filepath.Join(temporaryDirectory, "locomo10.json")
	licensePath := filepath.Join(temporaryDirectory, "LICENSE")
	outputPath := filepath.Join(temporaryDirectory, "conv-test.jsonl")
	manifestPath := filepath.Join(temporaryDirectory, "conv-test.manifest.json")
	source, err := json.Marshal([]sourceSample{{
		SampleID: "conv-test",
		Conversation: map[string]json.RawMessage{
			"session_2":           json.RawMessage(`[{"speaker":"Bob","dia_id":"D2:1","text":"Later"}]`),
			"session_2_date_time": json.RawMessage(`"2:30 pm on 3 January, 2024"`),
			"session_1":           json.RawMessage(`[{"speaker":"Alice","dia_id":"D1:1","text":"Hello"},{"speaker":"Bob","dia_id":"D1:2","text":"Hi"}]`),
			"session_1_date_time": json.RawMessage(`"1:00 pm on 2 January, 2024"`),
		},
		Questions: []sourceQuestion{
			{Question: "Who replied?", Answer: json.RawMessage(`"Bob"`), Evidence: []string{"D1:2"}, Category: 4},
			{Question: "Unsupported?", Answer: json.RawMessage(`"nothing"`), Category: 5},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(licensePath, []byte("license\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration := options{
		input: sourcePath, output: outputPath, manifest: manifestPath,
		conversationID: "conv-test", sourceRepository: "https://example.com/locomo",
		sourceCommit: "0123456789abcdef", sourcePath: "data/locomo10.json",
		sourceSHA256: sha256Hex(source), licensePath: licensePath,
		licenseURL: "https://example.com/license", licenseSPDX: "CC-BY-NC-4.0",
	}
	if err := run(configuration); err != nil {
		t.Fatal(err)
	}

	fixture, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	records := bytes.Split(bytes.TrimSpace(fixture), []byte{'\n'})
	if len(records) != 3 {
		t.Fatalf("records=%d, want 3", len(records))
	}
	var conversation fixtureConversation
	if err := json.Unmarshal(records[0], &conversation); err != nil {
		t.Fatal(err)
	}
	if len(conversation.Turns) != 3 || conversation.Turns[0].SessionID != "session_1" || conversation.Turns[2].SessionID != "session_2" {
		t.Fatalf("turns are not ordered by session: %+v", conversation.Turns)
	}
	if conversation.Turns[0].Role != "assistant" || conversation.Turns[1].Role != "user" || conversation.Turns[1].EvidenceID != "conv-test:D1:2" {
		t.Fatalf("converted turns = %+v", conversation.Turns[:2])
	}
	if want := time.Date(2024, time.January, 2, 13, 0, 0, 0, time.UTC); !conversation.Turns[0].ObservedAt.Equal(want) {
		t.Fatalf("observed_at=%s, want %s", conversation.Turns[0].ObservedAt, want)
	}
	var answerable, adversarial fixtureQuestion
	if err := json.Unmarshal(records[1], &answerable); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(records[2], &adversarial); err != nil {
		t.Fatal(err)
	}
	if !answerable.Answerable || len(answerable.GoldAnswers) != 1 || answerable.GoldAnswers[0] != "Bob" || answerable.EvidenceIDs[0] != "conv-test:D1:2" {
		t.Fatalf("answerable question = %+v", answerable)
	}
	if adversarial.Answerable || len(adversarial.GoldAnswers) != 0 || strings.Join(adversarial.Tags, ",") != "cat5,adversarial" {
		t.Fatalf("adversarial question = %+v", adversarial)
	}

	encodedManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata manifest
	if err := json.Unmarshal(encodedManifest, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Source.SHA256 != configuration.sourceSHA256 || metadata.Derived.SHA256 != sha256Hex(fixture) || metadata.License.SHA256 != sha256Hex([]byte("license\n")) {
		t.Fatalf("manifest hashes = %+v", metadata)
	}
	if metadata.Source.Commit != configuration.sourceCommit || strings.Join(metadata.Derived.SessionIDs, ",") != "conv-test:session_1,conv-test:session_2" || len(metadata.Derived.QuestionIDs) != 2 {
		t.Fatalf("manifest provenance = %+v", metadata)
	}

	configuration.sourceSHA256 = strings.Repeat("0", 64)
	if err := run(configuration); err == nil || !strings.Contains(err.Error(), "source dataset SHA-256") {
		t.Fatalf("run() hash mismatch error = %v", err)
	}
}

func TestDecodeAnswer(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		raw  string
		want string
	}{
		"string": {raw: `"January, 2023"`, want: "January, 2023"},
		"number": {raw: `2022`, want: "2022"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := decodeAnswer(json.RawMessage(test.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("decodeAnswer() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDecodeAnswerRejectsStructuredValue(t *testing.T) {
	t.Parallel()
	if _, err := decodeAnswer(json.RawMessage(`["answer"]`)); err == nil {
		t.Fatal("decodeAnswer() should reject structured values")
	}
}
