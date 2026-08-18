package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

// OpenAIConversation is the immutable Workspace-backed Conversation projection.
type OpenAIConversation struct {
	ID        string            `json:"id"`
	Metadata  map[string]string `json:"metadata"`
	CreatedAt time.Time         `json:"created_at"`
}

// OpenAIItem correlates one OpenAI message with its authoritative History entry.
type OpenAIItem struct {
	ID        string    `json:"id"`
	HistoryID string    `json:"history_id"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	Sequence  uint64    `json:"sequence"`
	CreatedAt time.Time `json:"created_at"`
}

// OpenAIResponse is the durable lifecycle record for one Responses request.
type OpenAIResponse struct {
	ID             string            `json:"id"`
	ConversationID string            `json:"conversation_id"`
	WorkspaceID    string            `json:"workspace_id"`
	Model          string            `json:"model"`
	Status         string            `json:"status"`
	Metadata       map[string]string `json:"metadata"`
	InputItemIDs   []string          `json:"input_item_ids"`
	OutputItemIDs  []string          `json:"output_item_ids"`
	CreatedAt      time.Time         `json:"created_at"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
	ErrorCode      string            `json:"error_code,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty"`
}

// OpenAIStateStore owns OpenAI adapter state below one Workspace runtime prefix.
type OpenAIStateStore struct {
	Objects objectstore.ObjectStore
	Prefix  string
}

func NewOpenAIStateStore(objects objectstore.ObjectStore, runtimePrefix string) *OpenAIStateStore {
	return &OpenAIStateStore{Objects: objects, Prefix: path.Join(strings.TrimSuffix(runtimePrefix, "/"), "openai")}
}

func (s *OpenAIStateStore) PutConversation(ctx context.Context, value OpenAIConversation) error {
	return s.putJSON(ctx, "conversation.json", value)
}

func (s *OpenAIStateStore) Conversation(ctx context.Context) (OpenAIConversation, error) {
	var value OpenAIConversation
	err := s.getJSON(ctx, "conversation.json", &value)
	return value, err
}

func (s *OpenAIStateStore) PutItem(ctx context.Context, value OpenAIItem) error {
	if strings.TrimSpace(value.ID) == "" {
		return fmt.Errorf("workspace openai: item id is required")
	}
	return s.putJSON(ctx, path.Join("items", value.ID+".json"), value)
}

func (s *OpenAIStateStore) Item(ctx context.Context, id string) (OpenAIItem, error) {
	var value OpenAIItem
	err := s.getJSON(ctx, path.Join("items", strings.TrimSpace(id)+".json"), &value)
	return value, err
}

func (s *OpenAIStateStore) Items(ctx context.Context) ([]OpenAIItem, error) {
	infos, err := s.list(ctx, "items")
	if err != nil {
		return nil, err
	}
	items := make([]OpenAIItem, 0, len(infos))
	for _, info := range infos {
		var item OpenAIItem
		if err := s.getObjectJSON(ctx, info.Name, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Sequence == items[j].Sequence {
			return items[i].ID < items[j].ID
		}
		return items[i].Sequence < items[j].Sequence
	})
	return items, nil
}

func (s *OpenAIStateStore) PutResponse(ctx context.Context, value OpenAIResponse) error {
	if strings.TrimSpace(value.ID) == "" {
		return fmt.Errorf("workspace openai: response id is required")
	}
	return s.putJSON(ctx, path.Join("responses", value.ID+".json"), value)
}

func (s *OpenAIStateStore) Response(ctx context.Context, id string) (OpenAIResponse, error) {
	var value OpenAIResponse
	err := s.getJSON(ctx, path.Join("responses", strings.TrimSpace(id)+".json"), &value)
	return value, err
}

func (s *OpenAIStateStore) putJSON(ctx context.Context, name string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.Objects == nil {
		return fmt.Errorf("workspace openai: object store is required")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("workspace openai: encode %s: %w", name, err)
	}
	if err := s.Objects.Put(path.Join(s.Prefix, name), bytes.NewReader(data)); err != nil {
		return fmt.Errorf("workspace openai: write %s: %w", name, err)
	}
	return nil
}

func (s *OpenAIStateStore) getJSON(ctx context.Context, name string, value any) error {
	return s.getObjectJSON(ctx, path.Join(s.Prefix, name), value)
}

func (s *OpenAIStateStore) getObjectJSON(ctx context.Context, name string, value any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.Objects == nil {
		return fmt.Errorf("workspace openai: object store is required")
	}
	r, err := s.Objects.Get(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fs.ErrNotExist
		}
		return err
	}
	defer r.Close()
	if err := json.NewDecoder(r).Decode(value); err != nil {
		return fmt.Errorf("workspace openai: decode %s: %w", name, err)
	}
	return nil
}

func (s *OpenAIStateStore) list(ctx context.Context, prefix string) ([]objectstore.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.Objects == nil {
		return nil, fmt.Errorf("workspace openai: object store is required")
	}
	return s.Objects.List(path.Join(s.Prefix, prefix))
}
