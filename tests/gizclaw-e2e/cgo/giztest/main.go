// Command giztest runs Giztest scenarios with the C SDKs.
//
// It accepts the same command line as `gizclaw test` and writes the same
// report JSON, so tests/gizclaw-e2e/run_tests.sh can add a C phase over the
// scenarios in tests/gizclaw-e2e/giztest. The scenario language, variables,
// expectations, and report come from pkgs/giztest; only the client driver
// differs.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/spf13/cobra"
)

const (
	exitValidation = 2
	exitExecution  = 4
)

func main() {
	err := newRootCmd().Execute()
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	code := exitExecution
	if coded, ok := errors.AsType[commandError](err); ok {
		code = coded.ExitCode()
	}
	os.Exit(code)
}

// commandError carries the exit code the e2e harness distinguishes:
// exitValidation for a document the C runner cannot accept, exitExecution for
// a run that failed.
type commandError struct {
	code int
	err  error
}

func (e commandError) Error() string { return e.err.Error() }
func (e commandError) Unwrap() error { return e.err }
func (e commandError) ExitCode() int { return e.code }

func codedError(code int, err error) error {
	if err == nil {
		return nil
	}
	return commandError{code: code, err: err}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "giztest",
		Short:        "Run Giztest scenarios with the GizClaw C SDKs",
		SilenceUsage: true,
	}
	test := &cobra.Command{Use: "test", Short: "Validate and run declarative Peer scenarios", SilenceUsage: true}
	test.AddCommand(newValidateCmd(), newRunCmd())
	root.AddCommand(test)
	return root
}

func newValidateCmd() *cobra.Command {
	var files []string
	cmd := &cobra.Command{
		Use:   "validate -f <file-or-directory>",
		Short: "Validate Giztest documents against the C runner without connecting",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 0 {
				return codedError(exitValidation, fmt.Errorf("validate accepts no positional arguments"))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			docs, skipped, err := load(cmd.ErrOrStderr(), files)
			if err != nil {
				return codedError(exitValidation, err)
			}
			fmt.Fprintf(
				cmd.OutOrStdout(), "validated %d Giztest documents, skipped %d\n", len(docs), len(skipped))
			return nil
		},
	}
	cmd.Flags().StringSliceVarP(&files, "file", "f", nil, "Giztest file or directory (repeatable)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func newRunCmd() *cobra.Command {
	var parallel int
	var output string
	cmd := &cobra.Command{
		Use:   "run <file-or-directory>...",
		Short: "Run Giztest documents with the C SDKs",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return codedError(exitValidation, fmt.Errorf("run requires at least one file or directory"))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if parallel < 1 {
				return codedError(exitValidation, fmt.Errorf("parallel must be positive"))
			}
			docs, skipped, err := load(cmd.ErrOrStderr(), args)
			if err != nil {
				return codedError(exitValidation, err)
			}
			timing := commandTiming(cmd)
			if err := giztest.ValidateTiming(docs, timing); err != nil {
				return codedError(exitValidation, err)
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			fmt.Fprintf(cmd.OutOrStdout(), "Giztest starting: %d documents, parallel=%d\n", len(docs), parallel)
			report := giztest.Run(ctx, docs, giztest.Options{
				Timing:   timing,
				Driver:   driver{},
				Parallel: parallel,
				In:       os.Stdin,
				Out:      cmd.OutOrStdout(),
			})
			if err := giztest.WriteReport(output, report); err != nil {
				return codedError(exitExecution, err)
			}
			fmt.Fprintf(
				cmd.OutOrStdout(), "Giztest %s: %d tasks in %dms, %d documents skipped\n",
				report.Status, len(report.Tasks), report.DurationMS, len(skipped))
			if report.Status != "passed" {
				return codedError(exitExecution, fmt.Errorf("Giztest execution failed"))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&parallel, "parallel", 1, "maximum concurrent tasks across all selected documents")
	cmd.Flags().StringVar(&output, "output", "", "write an atomic JSON report")
	addTimingFlags(cmd)
	return cmd
}

/*
load selects the documents this runner can execute.

The scenario directory is shared with the Go, JavaScript, and Flutter runners,
so it holds documents built from steps the C SDKs have no client for. Those are
reported as skipped on stderr, naming the step and operation, the same way the
JavaScript and Flutter runners do. They are never counted as passing, and a
document that is malformed rather than merely unsupported is still an error.
*/
func load(stderr io.Writer, inputs []string) ([]*giztest.Document, []giztest.SkippedDocument, error) {
	paths, err := giztest.Discover(inputs)
	if err != nil {
		return nil, nil, err
	}
	documents, skipped, err := giztest.LoadSupportedDocuments(paths, driver{})
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range skipped {
		fmt.Fprintf(stderr, "skipped %s: %s\n", entry.Path, entry.Reason)
	}
	return documents, skipped, nil
}

// addTimingFlags keeps flag presence separate from its value: --start-jitter=0
// must override a non-zero document value, and --seed=0 is a real seed.
func addTimingFlags(cmd *cobra.Command) {
	cmd.Flags().String("start-jitter", "0", "override uniform task start jitter (exclusive upper bound)")
	cmd.Flags().String("stagger", "0", "override repeat-index-based task start spacing")
	cmd.Flags().String("step-jitter", "0", "override uniform think time before steps after the first")
	cmd.Flags().Int64("seed", 0, "scheduling seed (0..9007199254740991); generated and reported when omitted")
}

func commandTiming(cmd *cobra.Command) giztest.TimingOverrides {
	var result giztest.TimingOverrides
	for _, field := range []struct {
		name   string
		target **string
	}{
		{"start-jitter", &result.StartJitter}, {"stagger", &result.Stagger}, {"step-jitter", &result.StepJitter},
	} {
		if cmd.Flags().Changed(field.name) {
			value, _ := cmd.Flags().GetString(field.name)
			*field.target = &value
		}
	}
	if cmd.Flags().Changed("seed") {
		value, _ := cmd.Flags().GetInt64("seed")
		result.Seed = &value
	}
	return result
}
