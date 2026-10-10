// Command benchmark-runner runs benchmark documents one at a time, retaining
// each document's repeated tasks as concurrent clients.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

type documentRunner func(context.Context, string, *giztest.Document, string) (int, error)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: benchmark-runner <gizclaw> <report> <document>...")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1], os.Args[2], os.Args[3:], os.Stdout, runDocument); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runDocument(ctx context.Context, binary string, doc *giztest.Document, output string) (int, error) {
	cmd := exec.CommandContext(ctx, binary, "test", "run", doc.Path,
		"--parallel", strconv.Itoa(doc.Repeat), "--output", output)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exited, ok := errors.AsType[*exec.ExitError](err); ok {
			return exited.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

func run(ctx context.Context, binary, output string, paths []string, out io.Writer, execute documentRunner) error {
	docs := make([]*giztest.Document, 0, len(paths))
	for _, path := range paths {
		path, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		doc, err := giztest.LoadDocument(path, nil)
		if err != nil {
			return fmt.Errorf("load benchmark %s: %w", path, err)
		}
		docs = append(docs, doc)
	}
	if len(docs) == 0 {
		return fmt.Errorf("no benchmark documents selected")
	}
	if err := os.MkdirAll(output+".d", 0o755); err != nil {
		return err
	}
	partial := output + ".partial.json"
	for _, path := range []string{output, partial} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	started := time.Now()
	combined := giztest.Report{Version: "v1", Status: "passed", StartedAt: started}
	failed := 0
	for index, doc := range docs {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprintf(out, "==> benchmark %d/%d: %s parallel=%d\n", index+1, len(docs), doc.Name, doc.Repeat)
		childPath := filepath.Join(output+".d", doc.Name+".json")
		// Never consume a stale report if this invocation exits before writing.
		if err := os.Remove(childPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		code, err := execute(ctx, binary, doc, childPath)
		if err != nil {
			return fmt.Errorf("run benchmark %s: %w", doc.Name, err)
		}
		if code != 0 && code != 4 {
			return fmt.Errorf("benchmark %s exited with unexpected status %d", doc.Name, code)
		}
		data, err := os.ReadFile(childPath)
		if err != nil {
			return fmt.Errorf("read benchmark %s report: %w", doc.Name, err)
		}
		var child giztest.Report
		if err := json.Unmarshal(data, &child); err != nil {
			return fmt.Errorf("decode benchmark %s report: %w", doc.Name, err)
		}
		if child.Version != "v1" || len(child.Tasks) != doc.Repeat {
			return fmt.Errorf("benchmark %s report has an invalid version or task count", doc.Name)
		}
		seen := make(map[int]bool, doc.Repeat)
		childFailed := false
		for _, task := range child.Tasks {
			if task.Name != doc.Name || task.Path != doc.Path || task.RepeatIndex < 0 || task.RepeatIndex >= doc.Repeat || seen[task.RepeatIndex] {
				return fmt.Errorf("benchmark %s report has invalid task identity", doc.Name)
			}
			seen[task.RepeatIndex] = true
			switch task.Status {
			case "passed":
			case "failed":
				childFailed = true
				failed++
			default:
				return fmt.Errorf("benchmark %s report has invalid task status %q", doc.Name, task.Status)
			}
		}
		if childFailed != (code != 0) || childFailed && child.Status != "failed" || !childFailed && child.Status != "passed" {
			return fmt.Errorf("benchmark %s report disagrees with exit status", doc.Name)
		}
		combined.Tasks = append(combined.Tasks, child.Tasks...)
		combined.DurationMS = time.Since(started).Milliseconds()
		if childFailed {
			combined.Status = "failed"
		}
		if err := giztest.WriteReport(partial, combined); err != nil {
			return err
		}
	}
	// Publish the combined report only after every selected document ran.
	if err := os.Rename(partial, output); err != nil {
		return err
	}
	if failed != 0 {
		return fmt.Errorf("%d benchmark tasks failed", failed)
	}
	return nil
}
