package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

func TestInventoryFunctionMutexScopesCoversReleaseAndControlFlow(t *testing.T) {
	source := []byte(`package fixture
import "sync"
func deferred(mu *sync.Mutex, callback func()) {
	mu.Lock()
	defer mu.Unlock()
	callback()
}
func manual(mu *sync.RWMutex, ready <-chan struct{}) {
	for range 2 {
		if true {
			mu.RLock()
			<-ready
			mu.RUnlock()
		}
	}
}
func escape(mu *sync.Mutex) func() {
	mu.Lock()
	return mu.Unlock
}
`)
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "fixture.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var records []mutexScopeCandidate
	for _, function := range mutexScopeFunctions(file) {
		found, err := inventoryFunctionMutexScopes(fileSet, "fixture.go", source, function)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, found...)
	}
	if len(records) != 4 {
		t.Fatalf("inventory length = %d, want 4: %#v", len(records), records)
	}
	want := []string{"defer", "manual", "unresolved", "caller"}
	got := make([]string, 0, len(records))
	for _, record := range records {
		got = append(got, record.Release)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("releases = %q, want %q", got, want)
	}
	if !slices.Contains(records[1].Risks, "channel") {
		t.Fatalf("manual critical-section risks = %q, want channel", records[1].Risks)
	}
}

func TestMutexScopeFingerprintChangesWithCriticalSection(t *testing.T) {
	fingerprint := func(source string) string {
		t.Helper()
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, "fixture.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		function := file.Decls[0].(*ast.FuncDecl)
		records, err := inventoryFunctionMutexScopes(fileSet, "fixture.go", []byte(source), mutexScopeFunction{name: function.Name.Name, body: function.Body})
		if err != nil || len(records) != 1 {
			t.Fatalf("inventory = %#v, %v", records, err)
		}
		return records[0].Fingerprint
	}
	first := fingerprint("package p\nfunc f(mu interface{ Lock(); Unlock() }) { mu.Lock(); one(); mu.Unlock() }")
	second := fingerprint("package p\nfunc f(mu interface{ Lock(); Unlock() }) { mu.Lock(); two(); mu.Unlock() }")
	if first == second {
		t.Fatal("critical-section body change retained the same fingerprint")
	}
}

func TestValidateMutexScopeReviewFailsClosed(t *testing.T) {
	valid := mutexScopeRecord{
		Fingerprint: "abc", File: "fixture.go", Line: 1, Function: "f", Kind: "lock",
		Receiver: "mu", Release: "defer", Classification: "intentional", Rationale: "mu protects f state",
	}
	if err := validateMutexScopeReview([]mutexScopeRecord{valid}); err != nil {
		t.Fatalf("valid review: %v", err)
	}
	tests := map[string][]mutexScopeRecord{
		"duplicate":         {valid, valid},
		"wildcard":          {func() mutexScopeRecord { copy := valid; copy.File = "*.go"; return copy }()},
		"missing rationale": {func() mutexScopeRecord { copy := valid; copy.Rationale = ""; return copy }()},
		"unresolved":        {func() mutexScopeRecord { copy := valid; copy.Classification = "unresolved"; return copy }()},
	}
	for name, records := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateMutexScopeReview(records); err == nil {
				t.Fatal("invalid review was accepted")
			}
		})
	}
}

func TestMutexScopeRiskClassificationIsStable(t *testing.T) {
	risks := mutexScopeRisks("mu.Lock(); store.List(ctx, key); <-ready; other.Lock(); mu.Unlock()")
	if got := strings.Join(risks, ","); got != "channel,nested-lock,store-scan" {
		t.Fatalf("risks = %q", got)
	}
}

func TestIsUnlockEscapeRejectsStringComparisons(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "fixture.go", `package p
func f(mu interface{ Unlock() }, unlock string) (func(), bool) {
	return mu.Unlock, unlock == "Unlock"
}`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	statement := file.Decls[0].(*ast.FuncDecl).Body.List[0].(*ast.ReturnStmt)
	if !isUnlockEscape(statement.Results[0]) {
		t.Fatal("returned Unlock method was not classified as an ownership transfer")
	}
	if isUnlockEscape(statement.Results[1]) {
		t.Fatal("string comparison was classified as an unlock ownership transfer")
	}
}
