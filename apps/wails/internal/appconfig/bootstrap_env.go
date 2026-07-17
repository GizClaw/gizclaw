package appconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// BootstrapEnvironmentStore persists Desktop-global, write-only environment
// values used only while initializing future local Pods.
type BootstrapEnvironmentStore struct {
	Path string
}

func (s BootstrapEnvironmentStore) Load() (map[string]string, error) {
	info, err := os.Lstat(s.Path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("appconfig: inspect bootstrap environment: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("appconfig: bootstrap environment must be a regular file")
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("appconfig: read bootstrap environment: %w", err)
	}
	var values map[string]string
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&values); err != nil {
		return nil, fmt.Errorf("appconfig: parse bootstrap environment: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, errors.New("appconfig: parse bootstrap environment: trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("appconfig: parse bootstrap environment: trailing data: %w", err)
	}
	if values == nil {
		values = map[string]string{}
	}
	for name := range values {
		if !environmentNamePattern.MatchString(name) {
			return nil, fmt.Errorf("appconfig: invalid bootstrap environment name %q", name)
		}
	}
	if err := os.Chmod(s.Path, 0o600); err != nil {
		return nil, fmt.Errorf("appconfig: secure bootstrap environment: %w", err)
	}
	return values, nil
}

// Update applies write-only changes. An empty value removes a saved entry;
// omitted names remain unchanged.
func (s BootstrapEnvironmentStore) Update(changes map[string]string) error {
	values, err := s.Load()
	if err != nil {
		return err
	}
	for name, value := range changes {
		if !environmentNamePattern.MatchString(name) {
			return fmt.Errorf("appconfig: invalid bootstrap environment name %q", name)
		}
		if value == "" {
			delete(values, name)
		} else {
			values[name] = value
		}
	}
	ordered := make([]string, 0, len(values))
	for name := range values {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	stable := make(map[string]string, len(values))
	for _, name := range ordered {
		stable[name] = values[name]
	}
	data, err := json.MarshalIndent(stable, "", "  ")
	if err != nil {
		return fmt.Errorf("appconfig: encode bootstrap environment: %w", err)
	}
	data = append(data, '\n')
	if err := secureDir(filepath.Dir(s.Path)); err != nil {
		return fmt.Errorf("appconfig: secure bootstrap environment directory: %w", err)
	}
	return atomicWrite(s.Path, data, 0o600)
}
