package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/cfg"
)

type mutexScopeRecord struct {
	Fingerprint    string   `json:"fingerprint"`
	File           string   `json:"file"`
	Line           int      `json:"line"`
	Function       string   `json:"function"`
	Kind           string   `json:"kind"`
	Receiver       string   `json:"receiver"`
	Release        string   `json:"release"`
	Risks          []string `json:"risks,omitempty"`
	Classification string   `json:"classification"`
	Rationale      string   `json:"rationale"`
}

type mutexScopeCandidate struct {
	mutexScopeRecord
	criticalSource string
}

func runMutexScope(root, reviewedFile string, writeReviewed bool) error {
	candidates, err := inventoryMutexScopes(root)
	if err != nil {
		return err
	}
	records := make([]mutexScopeRecord, 0, len(candidates))
	for _, candidate := range candidates {
		record := candidate.mutexScopeRecord
		record.Classification = "intentional"
		record.Rationale = mutexScopeRationale(record)
		records = append(records, record)
	}
	path, err := repositoryOwnedFile(root, reviewedFile)
	if err != nil {
		return fmt.Errorf("reviewed file: %w", err)
	}
	if writeReviewed {
		contents, err := marshalMutexScopeRecords(records)
		if err != nil {
			return err
		}
		return writeFileAtomically(path, contents)
	}
	reviewed, err := readMutexScopeRecords(path)
	if err != nil {
		return err
	}
	if err := validateMutexScopeReview(reviewed); err != nil {
		return err
	}
	if !slices.EqualFunc(records, reviewed, equalMutexScopeRecord) {
		return errors.New("mutexscope review mismatch; inspect the changed critical sections and run mutexscope -write-reviewed only after review")
	}
	fmt.Printf("mutexscope: %d reviewed critical sections and lock escapes\n", len(records))
	return nil
}

func inventoryMutexScopes(root string) ([]mutexScopeCandidate, error) {
	files, err := handwrittenFiles(root, "go")
	if err != nil {
		return nil, err
	}
	var records []mutexScopeCandidate
	for _, relative := range files {
		contents, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", relative, err)
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, relative, contents, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", relative, err)
		}
		functions := mutexScopeFunctions(file)
		for _, function := range functions {
			functionRecords, err := inventoryFunctionMutexScopes(fileSet, relative, contents, function)
			if err != nil {
				return nil, err
			}
			records = append(records, functionRecords...)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		left, right := records[i].mutexScopeRecord, records[j].mutexScopeRecord
		return mutexScopeSortKey(left) < mutexScopeSortKey(right)
	})
	return records, nil
}

type mutexScopeFunction struct {
	name string
	body *ast.BlockStmt
}

func mutexScopeFunctions(file *ast.File) []mutexScopeFunction {
	var functions []mutexScopeFunction
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Body == nil {
			continue
		}
		name := function.Name.Name
		if function.Recv != nil && len(function.Recv.List) != 0 {
			name = "(" + expressionString(token.NewFileSet(), function.Recv.List[0].Type) + ")." + name
		}
		functions = append(functions, mutexScopeFunction{name: name, body: function.Body})
	}
	return functions
}

func inventoryFunctionMutexScopes(fileSet *token.FileSet, file string, source []byte, function mutexScopeFunction) ([]mutexScopeCandidate, error) {
	type callSite struct {
		call     *ast.CallExpr
		receiver string
		method   string
		deferred bool
	}
	var calls []callSite
	reachableCalls := make(map[token.Pos]bool)
	graph := cfg.New(function.body, func(*ast.CallExpr) bool { return true })
	for _, block := range graph.Blocks {
		if !block.Live {
			continue
		}
		for _, node := range block.Nodes {
			ast.Inspect(node, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					reachableCalls[call.Pos()] = true
				}
				return true
			})
		}
	}
	parents := make(map[ast.Node]ast.Node)
	functionReturnsUnlock := false
	var stack []ast.Node
	ast.Inspect(function.body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if len(stack) != 0 {
			parents[node] = stack[len(stack)-1]
		}
		stack = append(stack, node)
		call, ok := node.(*ast.CallExpr)
		if statement, ok := node.(*ast.ReturnStmt); ok {
			for _, result := range statement.Results {
				functionReturnsUnlock = functionReturnsUnlock || isUnlockEscape(result)
			}
		}
		if !ok {
			return true
		}
		if !reachableCalls[call.Pos()] {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (selector.Sel.Name != "Lock" && selector.Sel.Name != "RLock" && selector.Sel.Name != "Unlock" && selector.Sel.Name != "RUnlock") {
			return true
		}
		_, deferred := parents[call].(*ast.DeferStmt)
		calls = append(calls, callSite{call: call, receiver: expressionString(fileSet, selector.X), method: selector.Sel.Name, deferred: deferred})
		return true
	})

	occurrence := make(map[string]int)
	var records []mutexScopeCandidate
	for index, call := range calls {
		if call.method != "Lock" && call.method != "RLock" {
			continue
		}
		release := "unresolved"
		end := function.body.End()
		for next := index + 1; next < len(calls); next++ {
			candidate := calls[next]
			if candidate.receiver != call.receiver || !matchingUnlock(call.method, candidate.method) {
				continue
			}
			if candidate.deferred {
				release = "defer"
			} else {
				release = "manual"
				end = candidate.call.End()
			}
			break
		}
		if release == "unresolved" {
			for previous := index - 1; previous >= 0; previous-- {
				candidate := calls[previous]
				if candidate.receiver != call.receiver || !matchingUnlock(call.method, candidate.method) {
					continue
				}
				if candidate.deferred {
					release = "defer"
				} else {
					release = "caller"
				}
				break
			}
		}
		if release == "unresolved" && (functionReturnsUnlock || assignedToUnlock(parents[call.call], call.call) || returnedLockHelper(parents[call.call])) {
			release = "caller"
		}
		startOffset := fileSet.Position(call.call.Pos()).Offset
		endOffset := fileSet.Position(end).Offset
		if startOffset < 0 || endOffset < startOffset || endOffset > len(source) {
			return nil, fmt.Errorf("mutexscope: invalid source range in %s", file)
		}
		critical := string(source[startOffset:endOffset])
		key := function.name + "|" + call.receiver + "|" + call.method
		occurrence[key]++
		fingerprintInput := strings.Join([]string{file, function.name, call.receiver, call.method, fmt.Sprint(occurrence[key]), normalizeMutexScopeSource(critical)}, "\x00")
		sum := sha256.Sum256([]byte(fingerprintInput))
		records = append(records, mutexScopeCandidate{
			mutexScopeRecord: mutexScopeRecord{
				Fingerprint: hex.EncodeToString(sum[:]), File: file,
				Line: fileSet.Position(call.call.Pos()).Line, Function: function.name,
				Kind: strings.ToLower(call.method), Receiver: call.receiver, Release: release,
				Risks: mutexScopeRisks(critical),
			},
			criticalSource: critical,
		})
	}

	ast.Inspect(function.body, func(node ast.Node) bool {
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for resultIndex, result := range statement.Results {
			text := expressionString(fileSet, result)
			if !isUnlockEscape(result) {
				continue
			}
			input := strings.Join([]string{file, function.name, "lock_escape", fmt.Sprint(resultIndex), normalizeMutexScopeSource(text)}, "\x00")
			sum := sha256.Sum256([]byte(input))
			records = append(records, mutexScopeCandidate{mutexScopeRecord: mutexScopeRecord{
				Fingerprint: hex.EncodeToString(sum[:]), File: file, Line: fileSet.Position(result.Pos()).Line,
				Function: function.name, Kind: "lock_escape", Receiver: text, Release: "caller",
				Risks: []string{"ownership-transfer"},
			}, criticalSource: text})
		}
		return true
	})
	return records, nil
}

func returnedLockHelper(parent ast.Node) bool {
	_, ok := parent.(*ast.ReturnStmt)
	return ok
}

func assignedToUnlock(parent ast.Node, call *ast.CallExpr) bool {
	assignment, ok := parent.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for index, expression := range assignment.Rhs {
		if expression != call || index >= len(assignment.Lhs) {
			continue
		}
		identifier, ok := assignment.Lhs[index].(*ast.Ident)
		return ok && strings.Contains(strings.ToLower(identifier.Name), "unlock")
	}
	return false
}

func isUnlockEscape(expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.SelectorExpr:
		return value.Sel.Name == "Unlock" || value.Sel.Name == "RUnlock"
	case *ast.Ident:
		return strings.Contains(strings.ToLower(value.Name), "unlock")
	case *ast.FuncLit:
		found := false
		ast.Inspect(value.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && (selector.Sel.Name == "Unlock" || selector.Sel.Name == "RUnlock") {
				found = true
				return false
			}
			return true
		})
		return found
	default:
		return false
	}
}

func matchingUnlock(lock, unlock string) bool {
	return lock == "Lock" && unlock == "Unlock" || lock == "RLock" && unlock == "RUnlock"
}

func expressionString(fileSet *token.FileSet, expression ast.Expr) string {
	var output bytes.Buffer
	if err := format.Node(&output, fileSet, expression); err != nil {
		return "<invalid>"
	}
	return output.String()
}

func normalizeMutexScopeSource(source string) string {
	return strings.Join(strings.Fields(source), " ")
}

func mutexScopeRisks(source string) []string {
	lower := strings.ToLower(source)
	var risks []string
	checks := []struct{ value, risk string }{
		{" go ", "goroutine"}, {"<-", "channel"}, {"http.", "network-io"},
		{".do(", "external-call"}, {".list(", "store-scan"}, {".get(", "store-io"},
		{".set(", "store-io"}, {".put(", "store-io"}, {".lock(", "nested-lock"},
	}
	for _, check := range checks {
		if strings.Contains(" "+lower, check.value) {
			risks = append(risks, check.risk)
		}
	}
	sort.Strings(risks)
	return slices.Compact(risks)
}

func mutexScopeRationale(record mutexScopeRecord) string {
	boundary := record.Receiver + " in " + record.Function
	if record.Kind == "lock_escape" {
		return "caller owns the exact release boundary returned by " + record.Function
	}
	if len(record.Risks) == 0 {
		return boundary + " protects its local state invariant until the recorded " + record.Release + " release"
	}
	return boundary + " intentionally contains " + strings.Join(record.Risks, ",") + " and is reviewed through the recorded " + record.Release + " release"
}

func marshalMutexScopeRecords(records []mutexScopeRecord) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func readMutexScopeRecords(path string) ([]mutexScopeRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read mutexscope review: %w", err)
	}
	defer file.Close()
	var records []mutexScopeRecord
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		var record mutexScopeRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("mutexscope review line %d: %w", line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func validateMutexScopeReview(records []mutexScopeRecord) error {
	seen := make(map[string]bool, len(records))
	previous := ""
	for index, record := range records {
		key := mutexScopeSortKey(record)
		if record.Fingerprint == "" || record.File == "" || record.Function == "" || record.Kind == "" || record.Receiver == "" {
			return fmt.Errorf("mutexscope review line %d is incomplete", index+1)
		}
		if seen[record.Fingerprint] {
			return fmt.Errorf("mutexscope review line %d duplicates fingerprint %s", index+1, record.Fingerprint)
		}
		seen[record.Fingerprint] = true
		if previous != "" && key <= previous {
			return fmt.Errorf("mutexscope review line %d is not sorted and unique", index+1)
		}
		previous = key
		if record.Classification != "intentional" || strings.TrimSpace(record.Rationale) == "" {
			return fmt.Errorf("mutexscope review line %d must contain an intentional classification and rationale", index+1)
		}
		if record.Release == "unresolved" {
			return fmt.Errorf("mutexscope review line %d has unresolved lock ownership", index+1)
		}
		if strings.ContainsAny(record.File, "*?[") {
			return fmt.Errorf("mutexscope review line %d contains a wildcard", index+1)
		}
	}
	return nil
}

func equalMutexScopeRecord(left, right mutexScopeRecord) bool {
	return left.Fingerprint == right.Fingerprint && left.File == right.File && left.Line == right.Line &&
		left.Function == right.Function && left.Kind == right.Kind && left.Receiver == right.Receiver &&
		left.Release == right.Release && slices.Equal(left.Risks, right.Risks) &&
		left.Classification == right.Classification && left.Rationale == right.Rationale
}

func mutexScopeSortKey(record mutexScopeRecord) string {
	return fmt.Sprintf("%s\x00%09d\x00%s\x00%s", record.File, record.Line, record.Kind, record.Fingerprint)
}
