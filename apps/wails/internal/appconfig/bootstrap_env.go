package appconfig

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const maxBootstrapEnvironmentSize = 1024 * 1024

// BootstrapEnvironmentStore persists Desktop-global environment values used
// only while initializing future local Pods.
type BootstrapEnvironmentStore struct {
	Path string
}

// Content returns the editable dotenv source, or an empty string when the file
// does not exist.
func (s BootstrapEnvironmentStore) Content() (string, error) {
	info, err := os.Lstat(s.Path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("appconfig: inspect bootstrap environment: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("appconfig: bootstrap environment must be a regular file")
	}
	if info.Size() > maxBootstrapEnvironmentSize {
		return "", fmt.Errorf("appconfig: bootstrap environment exceeds %d bytes", maxBootstrapEnvironmentSize)
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return "", fmt.Errorf("appconfig: read bootstrap environment: %w", err)
	}
	if err := os.Chmod(s.Path, 0o600); err != nil {
		return "", fmt.Errorf("appconfig: secure bootstrap environment: %w", err)
	}
	return string(data), nil
}

// Load parses the current dotenv source.
func (s BootstrapEnvironmentStore) Load() (map[string]string, error) {
	content, err := s.Content()
	if err != nil {
		return nil, err
	}
	return ParseBootstrapEnvironment(content)
}

// Replace validates and atomically writes editable dotenv source.
func (s BootstrapEnvironmentStore) Replace(content string) error {
	if _, err := ParseBootstrapEnvironment(content); err != nil {
		return err
	}
	if err := secureDir(filepath.Dir(s.Path)); err != nil {
		return fmt.Errorf("appconfig: secure bootstrap environment directory: %w", err)
	}
	return atomicWrite(s.Path, []byte(content), 0o600)
}

// ParseBootstrapEnvironment parses dotenv-style KEY=value lines. Blank lines
// and comments are ignored; duplicate or malformed names are rejected.
func ParseBootstrapEnvironment(content string) (map[string]string, error) {
	if len(content) > maxBootstrapEnvironmentSize {
		return nil, fmt.Errorf("appconfig: bootstrap environment exceeds %d bytes", maxBootstrapEnvironmentSize)
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if assignment, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(assignment)
		}
		name, rawValue, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || !environmentNamePattern.MatchString(name) {
			return nil, fmt.Errorf("appconfig: parse bootstrap environment line %d: invalid assignment", lineNumber)
		}
		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("appconfig: parse bootstrap environment line %d: duplicate name %q", lineNumber, name)
		}
		value, err := parseBootstrapEnvironmentValue(strings.TrimSpace(rawValue))
		if err != nil {
			return nil, fmt.Errorf("appconfig: parse bootstrap environment line %d for %s: %w", lineNumber, name, err)
		}
		values[name] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("appconfig: scan bootstrap environment: %w", err)
	}
	return values, nil
}

func parseBootstrapEnvironmentValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	switch raw[0] {
	case '\'':
		end := strings.IndexByte(raw[1:], '\'')
		if end < 0 {
			return "", errors.New("unterminated single-quoted value")
		}
		end++
		if err := validateBootstrapEnvironmentValueTail(raw[end+1:]); err != nil {
			return "", err
		}
		return raw[1:end], nil
	case '"':
		end := 1
		escaped := false
		for ; end < len(raw); end++ {
			if raw[end] == '"' && !escaped {
				break
			}
			if raw[end] == '\\' {
				escaped = !escaped
			} else {
				escaped = false
			}
		}
		if end == len(raw) {
			return "", errors.New("unterminated double-quoted value")
		}
		if err := validateBootstrapEnvironmentValueTail(raw[end+1:]); err != nil {
			return "", err
		}
		value, err := strconv.Unquote(raw[:end+1])
		if err != nil {
			return "", fmt.Errorf("invalid double-quoted value: %w", err)
		}
		return value, nil
	default:
		for i := 1; i < len(raw); i++ {
			if raw[i] == '#' && (raw[i-1] == ' ' || raw[i-1] == '\t') {
				raw = raw[:i]
				break
			}
		}
		return strings.TrimSpace(raw), nil
	}
}

func validateBootstrapEnvironmentValueTail(tail string) error {
	tail = strings.TrimSpace(tail)
	if tail == "" || strings.HasPrefix(tail, "#") {
		return nil
	}
	return errors.New("unexpected content after quoted value")
}
