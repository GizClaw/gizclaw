package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func runTestFiles(root, exemptionsFile string, writeExemptions bool) error {
	files, err := trackedFiles(root)
	if err != nil {
		return err
	}
	tracked := make(map[string]bool, len(files))
	for _, file := range files {
		tracked[file] = true
	}
	violations, pure := testFileOwnershipViolations(files)
	if len(violations) != 0 {
		_, _ = fmt.Fprintln(os.Stderr, strings.Join(violations, "\n"))
		return errors.New("test files must be named after an owning source file")
	}

	path, err := repositoryOwnedFile(root, exemptionsFile)
	if err != nil {
		return fmt.Errorf("test file exemptions: %w", err)
	}
	actual := serializedDiagnostics(pure)
	if writeExemptions {
		return writeFileAtomically(path, actual)
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read exemptions: %w", err)
	}
	if err := validateTestFileExemptions(expected, tracked, pure); err != nil {
		return err
	}
	if !bytes.Equal(expected, actual) {
		return fmt.Errorf("pure-test-package exemptions differ from %s", exemptionsFile)
	}
	fmt.Printf("testfiles: checked %d test files with %d explicit pure-package exemptions\n", countTestFiles(files), len(pure))
	return nil
}

func testFileOwnershipViolations(files []string) ([]string, []string) {
	sources := make(map[string][]string)
	for _, file := range files {
		if filepath.Ext(file) != ".go" || strings.HasSuffix(file, "_test.go") || excludedTestOwnershipPath(file) {
			continue
		}
		directory := filepath.ToSlash(filepath.Dir(file))
		stem := strings.TrimSuffix(filepath.Base(file), ".go")
		sources[directory] = append(sources[directory], stem)
	}

	var violations []string
	var pure []string
	for _, file := range files {
		if !strings.HasSuffix(file, "_test.go") || excludedTestOwnershipPath(file) {
			continue
		}
		directory := filepath.ToSlash(filepath.Dir(file))
		stem := strings.TrimSuffix(filepath.Base(file), "_test.go")
		if testStemHasOwner(stem, sources[directory]) {
			continue
		}
		if len(sources[directory]) == 0 {
			pure = append(pure, file)
			continue
		}
		violations = append(violations, file)
	}
	sort.Strings(violations)
	sort.Strings(pure)
	return violations, pure
}

func testStemHasOwner(testStem string, sourceStems []string) bool {
	for _, sourceStem := range sourceStems {
		if testStem == sourceStem || strings.HasPrefix(testStem, sourceStem+"_") {
			return true
		}
	}
	return false
}

func excludedTestOwnershipPath(file string) bool {
	file = filepath.ToSlash(filepath.Clean(file))
	for _, directory := range append(append([]string{"vendor/"}, generatedDirectories...), thirdPartyDirectories...) {
		if strings.HasPrefix(file, directory) {
			return true
		}
	}
	return false
}

func validateTestFileExemptions(contents []byte, tracked map[string]bool, pure []string) error {
	if len(contents) == 0 {
		if len(pure) == 0 {
			return nil
		}
		return errors.New("pure test packages require explicit exemptions")
	}
	if contents[len(contents)-1] != '\n' {
		return errors.New("test file exemptions must end with a newline")
	}
	pureSet := make(map[string]bool, len(pure))
	for _, file := range pure {
		pureSet[file] = true
	}
	previous := ""
	for index, line := range strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n") {
		lineNumber := index + 1
		if line == "" || filepath.IsAbs(line) || filepath.ToSlash(filepath.Clean(line)) != line {
			return fmt.Errorf("test file exemption line %d is not a normalized repository path", lineNumber)
		}
		if !tracked[line] || !strings.HasSuffix(line, "_test.go") {
			return fmt.Errorf("test file exemption line %d does not reference a tracked test file", lineNumber)
		}
		if excludedTestOwnershipPath(line) {
			return fmt.Errorf("test file exemption line %d references excluded generated or third-party code", lineNumber)
		}
		if !pureSet[line] {
			return fmt.Errorf("test file exemption line %d is stale or its package has production Go source", lineNumber)
		}
		if previous != "" && line <= previous {
			return fmt.Errorf("test file exemption line %d is not sorted and unique", lineNumber)
		}
		previous = line
	}
	return nil
}

func countTestFiles(files []string) int {
	count := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") && !excludedTestOwnershipPath(file) {
			count++
		}
	}
	return count
}
