package giztest

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	exitValidation = 2
	exitReview     = 3
	exitExecution  = 4
)

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

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test", Short: "Validate and run declarative Peer scenarios", SilenceUsage: true}
	cmd.AddCommand(newValidateCmd(), newRunCmd())
	return cmd
}
func newValidateCmd() *cobra.Command {
	var files []string
	cmd := &cobra.Command{Use: "validate -f <file-or-directory>", Short: "Validate Giztest documents without connecting", Args: func(_ *cobra.Command, args []string) error {
		if len(args) != 0 {
			return codedError(exitValidation, fmt.Errorf("validate accepts no positional arguments"))
		}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		paths, err := discover(files)
		if err != nil {
			return codedError(exitValidation, err)
		}
		docs, err := loadDocuments(paths)
		if err != nil {
			return codedError(exitValidation, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "validated %d Giztest documents\n", len(docs))
		return nil
	}}
	cmd.Flags().StringSliceVarP(&files, "file", "f", nil, "Giztest file or directory (repeatable)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
func newRunCmd() *cobra.Command {
	var parallel int
	var output string
	var evidence string
	cmd := &cobra.Command{Use: "run <file-or-directory>...", Short: "Run Giztest documents", Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return codedError(exitValidation, fmt.Errorf("run requires at least one file or directory"))
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateRunOptions(parallel, output, evidence); err != nil {
			return codedError(exitValidation, err)
		}
		paths, err := discover(args)
		if err != nil {
			return codedError(exitValidation, err)
		}
		docs, err := loadDocuments(paths)
		if err != nil {
			return codedError(exitValidation, err)
		}
		for _, doc := range docs {
			if doc.Review && parallel != 1 {
				return codedError(exitValidation, fmt.Errorf("review document %s requires --parallel 1", doc.Name))
			}
		}
		report := runDocuments(cmd.Context(), docs, runOptions{parallel: parallel, in: os.Stdin, out: cmd.OutOrStdout(), fullEvidence: evidence == "full"})
		if err := writeReport(output, report); err != nil {
			return codedError(exitExecution, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Giztest %s: %d tasks in %dms\n", report.Status, len(report.Tasks), report.DurationMS)
		if report.Status != "passed" {
			if reportHasReviewFailure(report) {
				return codedError(exitReview, fmt.Errorf("Giztest review rejected"))
			}
			return codedError(exitExecution, fmt.Errorf("Giztest execution failed"))
		}
		return nil
	}}
	cmd.Flags().IntVar(&parallel, "parallel", 1, "maximum concurrent tasks across all selected documents")
	cmd.Flags().StringVar(&output, "output", "", "write an atomic JSON report")
	cmd.Flags().StringVar(&evidence, "evidence", "redacted", "report evidence mode: redacted or full (full may contain sensitive relay text)")
	return cmd
}

func validateRunOptions(parallel int, output, evidence string) error {
	if parallel < 1 {
		return fmt.Errorf("parallel must be positive")
	}
	if evidence != "redacted" && evidence != "full" {
		return fmt.Errorf("evidence must be redacted or full")
	}
	if evidence == "full" && output == "" {
		return fmt.Errorf("full evidence requires --output")
	}
	return nil
}

func reportHasReviewFailure(report Report) bool {
	for _, task := range report.Tasks {
		for _, step := range task.Steps {
			if step.Operation == "review" && step.Status != "passed" {
				return true
			}
		}
	}
	return false
}
