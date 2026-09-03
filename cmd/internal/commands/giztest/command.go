package giztestcmd

import (
	"fmt"
	"os"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
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
	cmd.AddCommand(newValidateCmd(), newRunCmd(), newPlayCmd())
	return cmd
}

func newPlayCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "play [--output <record-directory>] <file.giztest.yaml>",
		Short: "giztest.Run and audibly play one Giztest document",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 1 {
				return codedError(exitValidation, fmt.Errorf("play requires exactly one Giztest file"))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validatePlayOutput(output); err != nil {
				return codedError(exitValidation, err)
			}
			paths, err := giztest.Discover(args)
			if err != nil {
				return codedError(exitValidation, err)
			}
			docs, err := giztest.LoadDocuments(paths, newDriver(false, nil))
			if err != nil {
				return codedError(exitValidation, err)
			}
			if err := validatePlayDocument(args[0], docs); err != nil {
				return codedError(exitValidation, err)
			}
			var record *playRecord
			if output != "" {
				record, err = newPlayRecord(output)
				if err != nil {
					return codedError(exitValidation, err)
				}
				defer record.abort()
			}
			session, err := newPlaySession(cmd.OutOrStdout())
			if err != nil {
				return codedError(exitExecution, err)
			}
			session.discardRecording = record == nil
			if err := session.cue(); err != nil {
				_ = session.close()
				return codedError(exitExecution, err)
			}
			report := giztest.Run(cmd.Context(), docs, giztest.Options{
				Driver:   newDriver(false, session.observe),
				Parallel: 1,
				In:       os.Stdin,
				Out:      cmd.OutOrStdout(),
			})
			if closeErr := session.close(); closeErr != nil {
				markPlayReportFailed(&report, fmt.Errorf("close playback: %w", closeErr))
			}
			if record != nil {
				if err := record.commit(report, session.packets); err != nil {
					return codedError(exitExecution, err)
				}
			}
			receivedMS, playbackMS := session.latencySummary()
			recordPath := "disabled"
			if output != "" {
				recordPath = output
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Giztest play %s: %d tasks in %dms, first_downlink_received=%dms, first_downlink_playback=%dms, audio=%d bytes, record=%s\n", report.Status, len(report.Tasks), report.DurationMS, receivedMS, playbackMS, session.bytes, recordPath)
			if report.Status != "passed" {
				if reportHasReviewFailure(report) {
					return codedError(exitReview, fmt.Errorf("Giztest review rejected"))
				}
				return codedError(exitExecution, fmt.Errorf("Giztest play failed"))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "write report.json and received audio.ogg to a new record directory")
	return cmd
}

func markPlayReportFailed(report *giztest.Report, err error) {
	if report == nil || err == nil {
		return
	}
	report.Status = "failed"
	if len(report.Tasks) == 0 {
		return
	}
	report.Tasks[0].Status = "failed"
	if report.Tasks[0].Error == "" {
		report.Tasks[0].Error = giztest.SafeError(err)
	}
}
func newValidateCmd() *cobra.Command {
	var files []string
	cmd := &cobra.Command{Use: "validate -f <file-or-directory>", Short: "Validate Giztest documents without connecting", Args: func(_ *cobra.Command, args []string) error {
		if len(args) != 0 {
			return codedError(exitValidation, fmt.Errorf("validate accepts no positional arguments"))
		}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		paths, err := giztest.Discover(files)
		if err != nil {
			return codedError(exitValidation, err)
		}
		docs, err := giztest.LoadDocuments(paths, newDriver(false, nil))
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
	cmd := &cobra.Command{Use: "run <file-or-directory>...", Short: "giztest.Run Giztest documents", Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return codedError(exitValidation, fmt.Errorf("run requires at least one file or directory"))
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateRunOptions(parallel, output, evidence); err != nil {
			return codedError(exitValidation, err)
		}
		paths, err := giztest.Discover(args)
		if err != nil {
			return codedError(exitValidation, err)
		}
		fullEvidence := evidence == "full"
		docs, err := giztest.LoadDocuments(paths, newDriver(fullEvidence, nil))
		if err != nil {
			return codedError(exitValidation, err)
		}
		for _, doc := range docs {
			if doc.Review && parallel != 1 {
				return codedError(exitValidation, fmt.Errorf("review document %s requires --parallel 1", doc.Name))
			}
		}
		report := giztest.Run(cmd.Context(), docs, giztest.Options{
			Driver:   newDriver(fullEvidence, nil),
			Parallel: parallel,
			In:       os.Stdin,
			Out:      cmd.OutOrStdout(),
		})
		if err := giztest.WriteReport(output, report); err != nil {
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

func reportHasReviewFailure(report giztest.Report) bool {
	for _, task := range report.Tasks {
		for _, step := range task.Steps {
			if step.Operation == "review" && step.Status != "passed" {
				return true
			}
		}
	}
	return false
}
