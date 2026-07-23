// Command quality runs repository-wide quality checks on tracked handwritten source.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var generatedGoHeader = regexp.MustCompile(`(?m)^// Code generated .* DO NOT EDIT\.$`)
var diagnostic = regexp.MustCompile(`^(.*?):[0-9]+:[0-9]+: `)

var generatedDirectories = []string{
	"apps/gizclaw-app/lib/l10n/generated/",
	"apps/wails/frontend/src/generated/",
	"apps/wails/frontend/wailsjs/",
	"sdk/c/gizclaw/generated/",
	"sdk/flutter/gizclaw/lib/src/generated/",
	"sdk/js/gizclaw/generated/",
	"third_party/",
}

func main() {
	if len(os.Args) < 2 {
		fatal("usage: quality <gofmt|modernize|vet|files> [flags]")
	}

	root, err := repositoryRoot()
	if err != nil {
		fatal("find repository root: %v", err)
	}

	switch os.Args[1] {
	case "gofmt":
		if err := runGofmt(root); err != nil {
			fatal("gofmt: %v", err)
		}
	case "modernize":
		flags := flag.NewFlagSet("modernize", flag.ExitOnError)
		binary := flags.String("binary", "modernize", "modernize executable")
		baseline := flags.String("baseline", "tools/quality/modernize.baseline", "repository-relative diagnostic baseline")
		writeBaseline := flags.Bool("write-baseline", false, "replace the diagnostic baseline")
		_ = flags.Parse(os.Args[2:])
		if err := runModernize(root, *binary, *baseline, *writeBaseline); err != nil {
			fatal("modernize: %v", err)
		}
	case "vet":
		flags := flag.NewFlagSet("vet", flag.ExitOnError)
		baseline := flags.String("baseline", "tools/quality/vet.baseline", "repository-relative diagnostic baseline")
		writeBaseline := flags.Bool("write-baseline", false, "replace the diagnostic baseline")
		_ = flags.Parse(os.Args[2:])
		if err := runVet(root, *baseline, *writeBaseline); err != nil {
			fatal("vet: %v", err)
		}
	case "files":
		flags := flag.NewFlagSet("files", flag.ExitOnError)
		language := flags.String("language", "", "source language: go, c, dart, or typescript")
		print0 := flags.Bool("print0", false, "separate paths with NUL")
		_ = flags.Parse(os.Args[2:])
		files, err := handwrittenFiles(root, *language)
		if err != nil {
			fatal("list files: %v", err)
		}
		separator := "\n"
		if *print0 {
			separator = "\x00"
		}
		for _, file := range files {
			_, _ = fmt.Fprint(os.Stdout, file, separator)
		}
	default:
		fatal("unknown command %q", os.Args[1])
	}
}

func runGofmt(root string) error {
	files, err := handwrittenFiles(root, "go")
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	command := exec.Command("gofmt", append([]string{"-l"}, files...)...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run gofmt: %w\n%s", err, output)
	}
	if len(output) != 0 {
		_, _ = os.Stderr.Write(output)
		return errors.New("handwritten Go files need formatting")
	}
	return nil
}

func runModernize(root, binary, baseline string, writeBaseline bool) error {
	modules, err := goModuleDirectories(root)
	if err != nil {
		return err
	}

	var diagnostics []string
	var toolFailed bool
	for _, module := range modules {
		command := exec.Command(binary, "./...")
		command.Dir = filepath.Join(root, module)
		output, err := command.CombinedOutput()
		if err != nil && len(output) == 0 {
			return fmt.Errorf("%s exited without diagnostics: %w", module, err)
		}
		moduleDiagnostics, failed := diagnosticsFromOutput(root, module, output)
		diagnostics = append(diagnostics, moduleDiagnostics...)
		if failed {
			toolFailed = true
		}
	}
	if toolFailed {
		return errors.New("modernize reported a tool or package-loading failure")
	}
	sort.Strings(diagnostics)
	diagnostics = compactStrings(diagnostics)
	return checkBaseline(root, baseline, diagnostics, writeBaseline)
}

func runVet(root, baseline string, writeBaseline bool) error {
	modules, err := goModuleDirectories(root)
	if err != nil {
		return err
	}

	var diagnostics []string
	for _, module := range modules {
		command := exec.Command("go", "vet", "./...")
		command.Dir = filepath.Join(root, module)
		command.Env = append(os.Environ(), "GOFLAGS=-mod=readonly")
		output, err := command.CombinedOutput()
		if err != nil && len(output) == 0 {
			return fmt.Errorf("%s exited without diagnostics: %w", module, err)
		}
		moduleDiagnostics, failed := diagnosticsFromOutput(root, module, output)
		diagnostics = append(diagnostics, moduleDiagnostics...)
		if failed {
			return errors.New("go vet reported a tool or package-loading failure")
		}
	}
	sort.Strings(diagnostics)
	return checkBaseline(root, baseline, compactStrings(diagnostics), writeBaseline)
}

func checkBaseline(root, baseline string, diagnostics []string, writeBaseline bool) error {
	baselinePath := filepath.Join(root, baseline)
	actual := strings.Join(diagnostics, "\n") + "\n"
	if writeBaseline {
		return os.WriteFile(baselinePath, []byte(actual), 0o600)
	}
	expected, err := os.ReadFile(baselinePath)
	if err != nil {
		return fmt.Errorf("read baseline: %w", err)
	}
	if string(expected) == actual {
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "diagnostics differ from %s\n", baseline)
	return errors.New("diagnostic baseline mismatch")
}

func diagnosticsFromOutput(root, module string, output []byte) ([]string, bool) {
	var diagnostics []string
	var toolFailed bool
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		path, ok := diagnosticPath(string(line))
		if ok && ignoredDiagnostic(root, module, path) {
			continue
		}
		if ok {
			diagnostics = append(diagnostics, normalizedDiagnostic(root, module, string(line), path))
			continue
		}
		_, _ = os.Stderr.Write(append(line, '\n'))
		toolFailed = true
	}
	return diagnostics, toolFailed
}

func normalizedDiagnostic(root, module, line, path string) string {
	absPath, err := diagnosticAbsolutePath(root, module, path)
	if err != nil {
		return line
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return line
	}
	return filepath.ToSlash(rel) + strings.TrimPrefix(line, path)
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func diagnosticPath(line string) (string, bool) {
	matches := diagnostic.FindStringSubmatch(line)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func ignoredDiagnostic(root, module, path string) bool {
	absPath, err := diagnosticAbsolutePath(root, module, path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, absPath)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return true
	}
	return isGenerated(root, filepath.ToSlash(rel))
}

func diagnosticAbsolutePath(root, module, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Join(root, module, path), nil
}

func handwrittenFiles(root, language string) ([]string, error) {
	extensions, ok := languageExtensions(language)
	if !ok {
		return nil, fmt.Errorf("unsupported language %q", language)
	}
	files, err := trackedFiles(root)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, file := range files {
		if !extensions[filepath.Ext(file)] || isGenerated(root, file) {
			continue
		}
		result = append(result, file)
	}
	return result, nil
}

func languageExtensions(language string) (map[string]bool, bool) {
	switch language {
	case "go":
		return map[string]bool{".go": true}, true
	case "c":
		return map[string]bool{".c": true, ".h": true, ".cc": true, ".cpp": true}, true
	case "dart":
		return map[string]bool{".dart": true}, true
	case "typescript":
		return map[string]bool{".ts": true, ".tsx": true, ".mts": true, ".cts": true}, true
	default:
		return nil, false
	}
}

func isGenerated(root, file string) bool {
	file = filepath.ToSlash(filepath.Clean(file))
	for _, directory := range generatedDirectories {
		if strings.HasPrefix(file, directory) {
			return true
		}
	}
	if filepath.Ext(file) != ".go" {
		return false
	}
	contents, err := os.ReadFile(filepath.Join(root, file))
	return err == nil && generatedGoHeader.Match(contents)
}

func trackedFiles(root string) ([]string, error) {
	command := exec.Command("git", "ls-files", "-z")
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	files := make([]string, 0)
	for file := range strings.SplitSeq(strings.TrimSuffix(string(output), "\x00"), "\x00") {
		if file != "" {
			files = append(files, file)
		}
	}
	sort.Strings(files)
	return files, nil
}

func goModuleDirectories(root string) ([]string, error) {
	files, err := trackedFiles(root)
	if err != nil {
		return nil, err
	}
	modules := make([]string, 0)
	for _, file := range files {
		if filepath.Base(file) == "go.mod" && !isGenerated(root, file) {
			directory := filepath.ToSlash(filepath.Dir(file))
			if directory == "." {
				modules = append(modules, ".")
				continue
			}
			modules = append(modules, directory)
		}
	}
	sort.Strings(modules)
	return modules, nil
}

func repositoryRoot() (string, error) {
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "quality: "+format+"\n", args...)
	os.Exit(1)
}
