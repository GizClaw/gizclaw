package giztestcmd

import (
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

func TestBenchmarkAndFirstResponseGiztestDocuments(t *testing.T) {
	for _, pattern := range []string{
		"benchmark.eino-*.giztest.yaml",
		"benchmark.flowcraft-*.giztest.yaml",
		"eino-concurrency-assistant.*.giztest.yaml",
		"flowcraft-voice-assistant.workspace-reload-initiative.giztest.yaml",
		"server.device.find*.giztest.yaml",
	} {
		paths, err := filepath.Glob(filepath.Join("../../../../tests/gizclaw-e2e/giztest", pattern))
		if err != nil || len(paths) == 0 {
			t.Fatalf("documents %q: %v, error = %v", pattern, paths, err)
		}
		for _, path := range paths {
			t.Run(filepath.Base(path), func(t *testing.T) {
				if _, err := giztest.LoadDocument(path, newDriver(false, nil)); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestConcurrencyBenchmarksDeclareTimingMode(t *testing.T) {
	paths, err := filepath.Glob("../../../../tests/gizclaw-e2e/giztest/benchmark.*concurrency*.giztest.yaml")
	if err != nil || len(paths) == 0 {
		t.Fatalf("paths = %v, error = %v", paths, err)
	}
	for _, path := range paths {
		doc, err := giztest.LoadDocument(path, newDriver(false, nil))
		if err != nil {
			t.Fatal(err)
		}
		if doc.StartJitter == "" || doc.StepJitter == "" || doc.Stagger == "" {
			t.Fatalf("%s lacks an explicit timing mode", path)
		}
	}
}
