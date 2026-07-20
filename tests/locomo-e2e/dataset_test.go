//go:build gizclaw_locomo_e2e

package locomo_e2e

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type benchmarkDataset struct {
	Conversations []benchmarkConversation
	Questions     []benchmarkQuestion
}

type benchmarkConversation struct {
	Type                   string          `json:"type"`
	ID                     string          `json:"id"`
	MinimumFactsPerSession int             `json:"minimum_facts_per_session"`
	Turns                  []benchmarkTurn `json:"turns"`
}

type benchmarkTurn struct {
	Role       string    `json:"role"`
	Speaker    string    `json:"speaker"`
	Content    string    `json:"content"`
	EvidenceID string    `json:"evidence_id"`
	SessionID  string    `json:"session_id"`
	ObservedAt time.Time `json:"observed_at"`
}

type benchmarkQuestion struct {
	Type           string   `json:"type"`
	ID             string   `json:"id"`
	ConversationID string   `json:"conversation_id"`
	Query          string   `json:"query"`
	GoldAnswers    []string `json:"gold_answers"`
	EvidenceIDs    []string `json:"evidence_ids"`
	Tags           []string `json:"tags"`
}

type datasetLine struct {
	Type string `json:"type"`
}

func loadDataset(path string) (*benchmarkDataset, string, error) {
	absolute := repoPath(path)
	raw, err := os.ReadFile(absolute)
	if err != nil {
		return nil, "", err
	}
	if bytes.HasPrefix(raw, []byte("version https://git-lfs.github.com/spec/v1")) {
		return nil, "", errors.New("LoCoMo dataset is a Git LFS pointer; run git lfs pull")
	}
	dataset := &benchmarkDataset{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var header datasetLine
		if err := json.Unmarshal(line, &header); err != nil {
			return nil, "", fmt.Errorf("dataset line %d: %w", lineNumber, err)
		}
		switch header.Type {
		case "conversation":
			var conversation benchmarkConversation
			if err := decodeStrict(line, &conversation); err != nil {
				return nil, "", fmt.Errorf("dataset line %d: %w", lineNumber, err)
			}
			dataset.Conversations = append(dataset.Conversations, conversation)
		case "question":
			var question benchmarkQuestion
			if err := decodeStrict(line, &question); err != nil {
				return nil, "", fmt.Errorf("dataset line %d: %w", lineNumber, err)
			}
			dataset.Questions = append(dataset.Questions, question)
		default:
			return nil, "", fmt.Errorf("dataset line %d: unknown type %q", lineNumber, header.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	if err := validateDataset(dataset); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(raw)
	identity := filepath.Base(path) + ":sha256:" + hex.EncodeToString(sum[:])
	return dataset, identity, nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("dataset record contains trailing JSON data")
	}
	return nil
}

func validateDataset(dataset *benchmarkDataset) error {
	if len(dataset.Conversations) == 0 || len(dataset.Questions) == 0 {
		return errors.New("LoCoMo dataset requires at least one conversation and one question")
	}
	conversations := make(map[string]map[string]struct{}, len(dataset.Conversations))
	for _, conversation := range dataset.Conversations {
		if strings.TrimSpace(conversation.ID) == "" || len(conversation.Turns) == 0 {
			return errors.New("LoCoMo conversation requires an ID and turns")
		}
		if conversation.MinimumFactsPerSession < 0 {
			return fmt.Errorf("conversation %q has a negative minimum_facts_per_session", conversation.ID)
		}
		if _, exists := conversations[conversation.ID]; exists {
			return fmt.Errorf("duplicate conversation ID %q", conversation.ID)
		}
		evidence := make(map[string]struct{}, len(conversation.Turns))
		sessionTimes := make(map[string]time.Time)
		for index, turn := range conversation.Turns {
			if turn.Role != "user" && turn.Role != "assistant" {
				return fmt.Errorf("conversation %q turn %d has invalid role %q", conversation.ID, index, turn.Role)
			}
			if strings.TrimSpace(turn.Speaker) == "" || strings.TrimSpace(turn.Content) == "" || strings.TrimSpace(turn.EvidenceID) == "" || strings.TrimSpace(turn.SessionID) == "" || turn.ObservedAt.IsZero() {
				return fmt.Errorf("conversation %q turn %d requires speaker, content, evidence_id, session_id, and observed_at", conversation.ID, index)
			}
			if observedAt, exists := sessionTimes[turn.SessionID]; exists && !observedAt.Equal(turn.ObservedAt) {
				return fmt.Errorf("conversation %q session %q has inconsistent observed_at values", conversation.ID, turn.SessionID)
			}
			sessionTimes[turn.SessionID] = turn.ObservedAt
			if _, exists := evidence[turn.EvidenceID]; exists {
				return fmt.Errorf("duplicate evidence ID %q", turn.EvidenceID)
			}
			evidence[turn.EvidenceID] = struct{}{}
		}
		conversations[conversation.ID] = evidence
	}
	questions := make(map[string]struct{}, len(dataset.Questions))
	for _, question := range dataset.Questions {
		if strings.TrimSpace(question.ID) == "" || strings.TrimSpace(question.Query) == "" || len(question.GoldAnswers) == 0 {
			return errors.New("LoCoMo question requires an ID, query, and gold answers")
		}
		for _, answer := range question.GoldAnswers {
			if strings.TrimSpace(answer) == "" {
				return fmt.Errorf("question %q has an empty gold answer", question.ID)
			}
		}
		if _, exists := questions[question.ID]; exists {
			return fmt.Errorf("duplicate question ID %q", question.ID)
		}
		questions[question.ID] = struct{}{}
		evidence, exists := conversations[question.ConversationID]
		if !exists {
			return fmt.Errorf("question %q references unknown conversation %q", question.ID, question.ConversationID)
		}
		for _, evidenceID := range question.EvidenceIDs {
			if strings.TrimSpace(evidenceID) == "" {
				return fmt.Errorf("question %q has an empty evidence ID", question.ID)
			}
			if _, exists := evidence[evidenceID]; !exists {
				return fmt.Errorf("question %q references unknown evidence %q", question.ID, evidenceID)
			}
		}
	}
	return nil
}
