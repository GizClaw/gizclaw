package appconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type State struct {
	SelectedContext string `json:"selected_context,omitempty"`
	SelectedView    string `json:"selected_view,omitempty"`
}

const DefaultView = "admin"

func NormalizeView(view string) string {
	switch view {
	case "admin", "play":
		return view
	default:
		return DefaultView
	}
}

type StateStore struct {
	File string
}

func (s StateStore) Load() (State, error) {
	if s.File == "" {
		return State{}, fmt.Errorf("appconfig: state file is empty")
	}
	data, err := os.ReadFile(s.File)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("appconfig: read state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("appconfig: parse state: %w", err)
	}
	return state, nil
}

func (s StateStore) Save(state State) error {
	if s.File == "" {
		return fmt.Errorf("appconfig: state file is empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.File), 0o700); err != nil {
		return fmt.Errorf("appconfig: mkdir state: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("appconfig: marshal state: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.File, data, 0o600); err != nil {
		return fmt.Errorf("appconfig: write state: %w", err)
	}
	return nil
}
