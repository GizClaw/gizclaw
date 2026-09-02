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
	"os"

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
	var coded commandError
	if errors.As(err, &coded) {
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
			docs, err := load(files)
			if err != nil {
				return codedError(exitValidation, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "validated %d Giztest documents\n", len(docs))
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
			docs, err := load(args)
			if err != nil {
				return codedError(exitValidation, err)
			}
			report := giztest.Run(cmd.Context(), docs, giztest.Options{
				Driver:   driver{},
				Parallel: parallel,
				In:       os.Stdin,
				Out:      cmd.OutOrStdout(),
			})
			if err := giztest.WriteReport(output, report); err != nil {
				return codedError(exitExecution, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Giztest %s: %d tasks in %dms\n", report.Status, len(report.Tasks), report.DurationMS)
			if report.Status != "passed" {
				return codedError(exitExecution, fmt.Errorf("Giztest execution failed"))
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&parallel, "parallel", 1, "maximum concurrent tasks across all selected documents")
	cmd.Flags().StringVar(&output, "output", "", "write an atomic JSON report")
	return cmd
}

func load(inputs []string) ([]*giztest.Document, error) {
	paths, err := giztest.Discover(inputs)
	if err != nil {
		return nil, err
	}
	return giztest.LoadDocuments(paths, driver{})
}
