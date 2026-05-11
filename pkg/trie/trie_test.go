package trie

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestTrieBasic(t *testing.T) {
	tr := New[string]()

	if err := tr.SetValue("a/b/c", "value1"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	val, ok := tr.GetValue("a/b/c")
	if !ok || val != "value1" {
		t.Errorf("GetValue failed: got %v, %v", val, ok)
	}
}

func TestTrieWildcard(t *testing.T) {
	tr := New[string]()

	if err := tr.SetValue("device/+/state", "single_wildcard"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}
	if err := tr.SetValue("logs/#", "multi_wildcard"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	val, ok := tr.GetValue("device/gear-001/state")
	if !ok || val != "single_wildcard" {
		t.Errorf("Single wildcard match failed: got %v, %v", val, ok)
	}

	val, ok = tr.GetValue("logs/app/debug/line1")
	if !ok || val != "multi_wildcard" {
		t.Errorf("Multi wildcard match failed: got %v, %v", val, ok)
	}
}

func TestTrieInvalidPatternAndGetMiss(t *testing.T) {
	tr := New[int]()
	err := tr.Set("a/#/b", func(ptr *int, existed bool) error {
		*ptr = 1
		return nil
	})
	if err != ErrInvalidPattern {
		t.Fatalf("expected ErrInvalidPattern, got: %v", err)
	}

	if _, ok := tr.Get("missing"); ok {
		t.Fatal("expected missing key to return ok=false")
	}
	v, ok := tr.GetValue("missing")
	if ok || v != 0 {
		t.Fatalf("expected zero value and ok=false, got v=%v ok=%v", v, ok)
	}
}

func TestTrieSetExistingAndSetFuncError(t *testing.T) {
	tr := New[int]()
	if err := tr.Set("a/b", func(ptr *int, existed bool) error {
		if existed {
			t.Fatalf("first set should not exist")
		}
		*ptr = 10
		return nil
	}); err != nil {
		t.Fatalf("first set failed: %v", err)
	}

	if err := tr.Set("a/b", func(ptr *int, existed bool) error {
		if !existed {
			t.Fatalf("second set should exist")
		}
		*ptr = 20
		return nil
	}); err != nil {
		t.Fatalf("second set failed: %v", err)
	}

	v, ok := tr.GetValue("a/b")
	if !ok || v != 20 {
		t.Fatalf("expected updated value 20, got v=%v ok=%v", v, ok)
	}

	wantErr := fmt.Errorf("boom")
	if err := tr.Set("a/c", func(ptr *int, existed bool) error {
		_ = ptr
		_ = existed
		return wantErr
	}); err == nil || err.Error() != wantErr.Error() {
		t.Fatalf("expected set function error, got: %v", err)
	}
}

func TestTrieMatchWalkStringAndLen(t *testing.T) {
	tr := New[string]()
	if err := tr.SetValue("a/b/c", "exact"); err != nil {
		t.Fatalf("set exact failed: %v", err)
	}
	if err := tr.SetValue("a/+/state", "single"); err != nil {
		t.Fatalf("set single wildcard failed: %v", err)
	}
	if err := tr.SetValue("a/#", "multi"); err != nil {
		t.Fatalf("set multi wildcard failed: %v", err)
	}

	route, val, ok := tr.Match("a/x/state")
	if !ok || route != "/a/+/state" || val == nil || *val != "single" {
		t.Fatalf("unexpected wildcard match: route=%q val=%v ok=%v", route, val, ok)
	}

	route, val, ok = tr.Match("a/whatever/else")
	if !ok || route != "/a/#" || val == nil || *val != "multi" {
		t.Fatalf("unexpected multi match: route=%q val=%v ok=%v", route, val, ok)
	}

	paths := make([]string, 0)
	tr.Walk(func(path string, value string, set bool) {
		if set {
			paths = append(paths, path+"="+value)
		}
	})
	if len(paths) != tr.Len() {
		t.Fatalf("walk set count %d != len %d", len(paths), tr.Len())
	}
	if tr.Len() != 3 {
		t.Fatalf("expected len=3, got %d", tr.Len())
	}

	s := tr.String()
	if !strings.Contains(s, "a/#: multi") || !strings.Contains(s, "a/b/c: exact") {
		t.Fatalf("unexpected string output: %q", s)
	}

	if !slices.Contains(paths, "a/b/c=exact") {
		t.Fatalf("walk paths missing exact route: %#v", paths)
	}
}

func generatePaths(count int) []string {
	paths := make([]string, count)
	for i := range count {
		a := i % 10
		b := (i / 10) % 10
		c := (i / 100) % 10
		paths[i] = fmt.Sprintf("device/gear-%03d/sensor/%d/data/%d", i, a, b*10+c)
	}
	return paths
}

func BenchmarkTrieSet(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		paths := generatePaths(size)
		b.Run(fmt.Sprintf("exact_paths/%d", size), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tr := New[int]()
				for j, path := range paths {
					tr.SetValue(path, j)
				}
			}
		})
	}
}

func BenchmarkTrieGetExact(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		paths := generatePaths(size)
		tr := New[int]()
		for j, path := range paths {
			tr.SetValue(path, j)
		}

		b.Run(fmt.Sprintf("lookup/%d", size), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, path := range paths {
					tr.Get(path)
				}
			}
		})
	}
}

func BenchmarkTrieGetWildcard(b *testing.B) {
	tr := New[string]()
	patterns := []string{
		"device/+/sensor/+/data/+",
		"device/gear-001/+/+/data/+",
		"device/#",
		"device/+/#",
		"logs/#",
	}
	for _, pattern := range patterns {
		tr.SetValue(pattern, pattern)
	}

	testPaths := []string{
		"device/gear-001/sensor/0/data/1",
		"device/gear-999/sensor/5/data/99",
		"device/gear-001/state/online",
		"logs/app/debug/line1",
		"logs/system/error",
	}

	b.Run("wildcard_match", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, path := range testPaths {
				tr.Get(path)
			}
		}
	})
}

func BenchmarkTrieMatchPath(b *testing.B) {
	tr := New[int]()

	exactPaths := generatePaths(1000)
	for j, path := range exactPaths {
		tr.SetValue(path, j)
	}

	tr.SetValue("device/+/sensor/+/data/+", -1)
	tr.SetValue("device/#", -2)
	tr.SetValue("logs/#", -3)

	testPaths := []string{
		"device/gear-500/sensor/5/data/50",
		"device/gear-9999/sensor/0/data/0",
		"device/unknown/state",
		"logs/anything/here",
	}

	b.Run("mixed_match", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, path := range testPaths {
				tr.Match(path)
			}
		}
	})
}

func BenchmarkTrieWalk(b *testing.B) {
	for _, size := range []int{100, 1000} {
		paths := generatePaths(size)
		tr := New[int]()
		for j, path := range paths {
			tr.SetValue(path, j)
		}

		b.Run(fmt.Sprintf("walk_all/%d", size), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				count := 0
				tr.Walk(func(_ string, _ int, set bool) {
					if set {
						count++
					}
				})
				_ = count
			}
		})
	}
}

func BenchmarkTrieDeepPaths(b *testing.B) {
	deepPaths := make([]string, 100)
	for i := range 100 {
		deepPaths[i] = fmt.Sprintf("a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/%d", i)
	}

	b.Run("deep_set", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tr := New[int]()
			for j, path := range deepPaths {
				tr.SetValue(path, j)
			}
		}
	})

	tr := New[int]()
	for j, path := range deepPaths {
		tr.SetValue(path, j)
	}

	b.Run("deep_get", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, path := range deepPaths {
				tr.Get(path)
			}
		}
	})
}

func BenchmarkTrieMemory(b *testing.B) {
	paths := generatePaths(1000)

	b.Run("set_allocs", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tr := New[int]()
			for j, path := range paths {
				tr.SetValue(path, j)
			}
		}
	})

	tr := New[int]()
	for j, path := range paths {
		tr.SetValue(path, j)
	}

	b.Run("get_allocs", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for _, path := range paths {
				tr.Get(path)
			}
		}
	})
}
