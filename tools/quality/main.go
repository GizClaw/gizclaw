// Command quality runs repository-wide quality checks on tracked handwritten source.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var generatedGoHeader = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)
var diagnostic = regexp.MustCompile(`^(.*?):[0-9]+:[0-9]+: `)

var generatedDirectories = []string{
	"guides/node_modules/",
	"sdk/c/gizclaw/generated/",
	"sdk/flutter/gizclaw/lib/src/generated/",
	"sdk/js/gizclaw/generated/",
}

var generatedFiles = map[string]bool{
	"pkgs/gizclaw/api/apitypes/codegen.go": true,
}

var thirdPartyDirectories = []string{
	"third_party/",
}

type sourceProvenance uint8

const (
	provenanceHandwritten sourceProvenance = iota
	provenanceGenerated
	provenanceThirdParty
	provenanceExcluded
)

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
		exemptions := flags.String("exemptions", "tools/quality/modernize.exemptions", "repository-relative generated/third-party diagnostic exemptions")
		writeExemptions := flags.Bool("write-exemptions", false, "replace generated/third-party diagnostic exemptions")
		_ = flags.Parse(os.Args[2:])
		if err := runModernize(root, *binary, *exemptions, *writeExemptions); err != nil {
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

func runModernize(root, binary, exemptions string, writeExemptions bool) error {
	modules, err := goModuleDirectories(root)
	if err != nil {
		return err
	}
	tracked, err := trackedFileSet(root)
	if err != nil {
		return err
	}

	var handwritten []string
	var exemptible []string
	var toolFailed bool
	for _, module := range modules {
		command := exec.Command(binary, "./...")
		command.Dir = filepath.Join(root, module)
		output, err := command.CombinedOutput()
		if err != nil && !analyzerReportedDiagnostics(err) {
			return fmt.Errorf("modernize failed in %s: %w\n%s", module, err, output)
		}
		if err != nil && len(output) == 0 {
			return fmt.Errorf("modernize exited without diagnostics in %s: %w", module, err)
		}
		moduleHandwritten, moduleExemptible, failed := modernizeDiagnosticsFromOutput(root, module, output, tracked)
		handwritten = append(handwritten, moduleHandwritten...)
		exemptible = append(exemptible, moduleExemptible...)
		if failed {
			toolFailed = true
		}
	}
	if toolFailed {
		return errors.New("modernize reported a tool or package-loading failure")
	}
	sort.Strings(handwritten)
	handwritten = compactStrings(handwritten)
	if len(handwritten) != 0 {
		_, _ = fmt.Fprintln(os.Stderr, strings.Join(handwritten, "\n"))
		return errors.New("modernize reported diagnostics in handwritten Go")
	}
	sort.Strings(exemptible)
	exemptible = compactStrings(exemptible)
	return checkModernizeExemptions(root, exemptions, exemptible, writeExemptions, tracked)
}

func analyzerReportedDiagnostics(err error) bool {
	var exitError *exec.ExitError
	return errors.As(err, &exitError) && exitError.ExitCode() == 3
}

func modernizeDiagnosticsFromOutput(
	root string,
	module string,
	output []byte,
	tracked map[string]bool,
) ([]string, []string, bool) {
	var handwritten []string
	var exemptible []string
	var toolFailed bool
	for line := range bytes.SplitSeq(output, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		text := string(line)
		path, ok := diagnosticPath(text)
		if !ok {
			_, _ = os.Stderr.Write(append(line, '\n'))
			toolFailed = true
			continue
		}
		rel, external := diagnosticRepositoryPath(root, module, path, tracked)
		if external {
			continue
		}
		normalized := rel + strings.TrimPrefix(text, path)
		switch classifySource(root, rel, tracked[rel]) {
		case provenanceGenerated, provenanceThirdParty:
			exemptible = append(exemptible, normalized)
		case provenanceExcluded:
			continue
		default:
			handwritten = append(handwritten, normalized)
		}
	}
	return handwritten, exemptible, toolFailed
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

func checkModernizeExemptions(
	root string,
	exemptions string,
	diagnostics []string,
	writeExemptions bool,
	tracked map[string]bool,
) error {
	exemptionsPath, err := repositoryOwnedFile(root, exemptions)
	if err != nil {
		return fmt.Errorf("modernize exemptions: %w", err)
	}
	actual := serializedDiagnostics(diagnostics)
	if writeExemptions {
		return writeFileAtomically(exemptionsPath, actual)
	}

	expected, err := os.ReadFile(exemptionsPath)
	if err != nil {
		return fmt.Errorf("read exemptions: %w", err)
	}
	if err := validateModernizeExemptions(root, expected, tracked); err != nil {
		return err
	}
	if bytes.Equal(expected, actual) {
		return nil
	}
	_, _ = fmt.Fprintf(os.Stderr, "diagnostics differ from %s\n", exemptions)
	return errors.New("modernize exemption mismatch")
}

func validateModernizeExemptions(root string, contents []byte, tracked map[string]bool) error {
	if len(contents) == 0 {
		return nil
	}
	if contents[len(contents)-1] != '\n' {
		return errors.New("modernize exemptions must end with a newline")
	}
	previous := ""
	lineNumber := 0
	for line := range strings.SplitSeq(strings.TrimSuffix(string(contents), "\n"), "\n") {
		lineNumber++
		if line == "" {
			return fmt.Errorf("modernize exemption line %d is empty", lineNumber)
		}
		path, ok := diagnosticPath(line)
		if !ok || filepath.IsAbs(path) {
			return fmt.Errorf("modernize exemption line %d is not a normalized diagnostic", lineNumber)
		}
		rel, external := diagnosticRepositoryPath(root, ".", path, tracked)
		if external || rel != path || !tracked[rel] {
			return fmt.Errorf("modernize exemption line %d does not reference a tracked repository file", lineNumber)
		}
		provenance := classifySource(root, rel, true)
		if provenance != provenanceGenerated && provenance != provenanceThirdParty {
			return fmt.Errorf("modernize exemption line %d references handwritten Go", lineNumber)
		}
		if previous != "" && line <= previous {
			return fmt.Errorf("modernize exemption line %d is not sorted and unique", lineNumber)
		}
		previous = line
	}
	return nil
}

func serializedDiagnostics(diagnostics []string) []byte {
	if len(diagnostics) == 0 {
		return nil
	}
	return []byte(strings.Join(diagnostics, "\n") + "\n")
}

func repositoryOwnedFile(root, file string) (string, error) {
	if filepath.IsAbs(file) {
		return "", errors.New("path must be repository-relative")
	}
	abs := filepath.Clean(filepath.Join(root, file))
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path must remain inside the repository")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	realDirectory, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("resolve exemptions directory: %w", err)
	}
	realRelative, err := filepath.Rel(realRoot, realDirectory)
	if err != nil || realRelative == ".." || strings.HasPrefix(realRelative, ".."+string(filepath.Separator)) {
		return "", errors.New("path must not traverse a symlink outside the repository")
	}
	if info, statErr := os.Lstat(abs); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("path must not be a symlink")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", fmt.Errorf("inspect exemptions path: %w", statErr)
	}
	return abs, nil
}

func writeFileAtomically(file string, contents []byte) (err error) {
	directory := filepath.Dir(file)
	temporary, err := os.CreateTemp(directory, ".modernize-exemptions-*")
	if err != nil {
		return fmt.Errorf("create temporary exemptions: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if err != nil {
			_ = os.Remove(temporaryName)
		}
	}()
	if err = temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("chmod temporary exemptions: %w", err)
	}
	if _, err = temporary.Write(contents); err != nil {
		return fmt.Errorf("write temporary exemptions: %w", err)
	}
	if err = temporary.Close(); err != nil {
		return fmt.Errorf("close temporary exemptions: %w", err)
	}
	if err = os.Rename(temporaryName, file); err != nil {
		return fmt.Errorf("replace exemptions: %w", err)
	}
	return nil
}

func diagnosticsFromOutput(root, module string, output []byte) ([]string, bool) {
	var diagnostics []string
	var toolFailed bool
	for line := range bytes.SplitSeq(output, []byte{'\n'}) {
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

func diagnosticRepositoryPath(
	root string,
	module string,
	path string,
	tracked map[string]bool,
) (string, bool) {
	if filepath.IsAbs(path) {
		return repositoryRelativePath(root, path)
	}

	moduleCandidate := filepath.Join(root, module, filepath.FromSlash(path))
	moduleRelative, moduleExternal := repositoryRelativePath(root, moduleCandidate)
	if !moduleExternal && tracked[moduleRelative] {
		return moduleRelative, false
	}

	rootCandidate := filepath.Join(root, filepath.FromSlash(path))
	rootRelative, rootExternal := repositoryRelativePath(root, rootCandidate)
	if !rootExternal && tracked[rootRelative] {
		return rootRelative, false
	}
	if !moduleExternal {
		return moduleRelative, false
	}
	if !rootExternal {
		return rootRelative, false
	}
	return "", true
}

func repositoryRelativePath(root, file string) (string, bool) {
	rel, err := filepath.Rel(root, filepath.Clean(file))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", true
	}
	return filepath.ToSlash(rel), false
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
	return classifySource(root, file, true) != provenanceHandwritten
}

func classifySource(root, file string, tracked bool) sourceProvenance {
	file = filepath.ToSlash(filepath.Clean(file))
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil || !info.Mode().IsRegular() {
		return provenanceHandwritten
	}
	for _, directory := range thirdPartyDirectories {
		if strings.HasPrefix(file, directory) {
			if !tracked {
				return provenanceExcluded
			}
			return provenanceThirdParty
		}
	}
	for _, directory := range generatedDirectories {
		if strings.HasPrefix(file, directory) {
			if !tracked {
				return provenanceExcluded
			}
			return provenanceGenerated
		}
	}
	if !tracked {
		return provenanceHandwritten
	}
	if generatedFiles[file] {
		return provenanceGenerated
	}
	if filepath.Ext(file) != ".go" {
		return provenanceHandwritten
	}
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return provenanceHandwritten
	}
	parsed, err := parser.ParseFile(
		token.NewFileSet(),
		file,
		contents,
		parser.PackageClauseOnly|parser.ParseComments,
	)
	if err != nil {
		return provenanceHandwritten
	}
	for _, group := range parsed.Comments {
		for _, comment := range group.List {
			if generatedGoHeader.MatchString(comment.Text) {
				return provenanceGenerated
			}
		}
	}
	return provenanceHandwritten
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

func trackedFileSet(root string) (map[string]bool, error) {
	files, err := trackedFiles(root)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(files))
	for _, file := range files {
		result[file] = true
	}
	return result, nil
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
