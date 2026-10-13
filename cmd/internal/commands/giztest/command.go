package giztestcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

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
			var record *replayRecord
			if output != "" {
				record, err = newReplayRecord(output)
				if err != nil {
					return codedError(exitValidation, err)
				}
				defer record.directory.abort()
			}
			session, err := newPlaySession(cmd.OutOrStdout())
			if err != nil {
				return codedError(exitExecution, err)
			}
			session.discardRecording = true
			if err := session.cue(); err != nil {
				_ = session.close()
				return codedError(exitExecution, err)
			}
			testDriver := newDriver(false, session.observe)
			testDriver.recording = record
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			report := giztest.Run(ctx, docs, giztest.Options{
				Driver:        testDriver,
				Parallel:      1,
				In:            os.Stdin,
				Out:           cmd.OutOrStdout(),
				ObserveOutput: record.observeOutput,
			})
			if closeErr := session.close(); closeErr != nil {
				markPlayReportFailed(&report, fmt.Errorf("close playback: %w", closeErr))
			}
			if record != nil {
				if err := record.commit(report); err != nil {
					return codedError(exitExecution, err)
				}
				printReplayPaths(cmd.OutOrStdout(), record)
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
	cmd.Flags().StringVarP(&output, "output", "o", "", "export each test's HTML, text and audio into a new output directory")
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
	var outDirectory string
	cmd := &cobra.Command{Use: "run <file-or-directory>...", Short: "giztest.Run Giztest documents", Args: func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return codedError(exitValidation, fmt.Errorf("run requires at least one file or directory"))
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		validationOutput := output
		if outDirectory != "" && validationOutput == "" {
			validationOutput = filepath.Join(outDirectory, "report.json")
		}
		if err := validateRunOptions(parallel, validationOutput, evidence); err != nil {
			return codedError(exitValidation, err)
		}
		if err := validateReplayReportOutput(outDirectory, output); err != nil {
			return codedError(exitValidation, err)
		}
		paths, err := giztest.Discover(args)
		if err != nil {
			return codedError(exitValidation, err)
		}
		if outDirectory != "" {
			if len(args) != 1 || len(paths) != 1 {
				return codedError(exitValidation, fmt.Errorf("--output records one Giztest file per command"))
			}
			info, err := os.Stat(args[0])
			if err != nil || !info.Mode().IsRegular() {
				return codedError(exitValidation, fmt.Errorf("--output requires one regular Giztest file"))
			}
		}
		fullEvidence := evidence == "full"
		timing := commandTiming(cmd)
		docs, err := giztest.LoadDocumentsWithTiming(paths, newDriver(fullEvidence, nil), timing)
		if err != nil {
			return codedError(exitValidation, err)
		}
		for _, doc := range docs {
			if doc.Review && parallel != 1 {
				return codedError(exitValidation, fmt.Errorf("review document %s requires --parallel 1", doc.Name))
			}
		}
		if err := giztest.ValidateTiming(docs, timing); err != nil {
			return codedError(exitValidation, err)
		}
		var record *replayRecord
		if outDirectory != "" {
			record, err = newReplayRecord(outDirectory)
			if err != nil {
				return codedError(exitValidation, err)
			}
			defer record.directory.abort()
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		fmt.Fprintf(cmd.OutOrStdout(), "Giztest starting: %d documents, parallel=%d\n", len(docs), parallel)
		testDriver := newDriver(fullEvidence, nil)
		testDriver.recording = record
		report := giztest.Run(ctx, docs, giztest.Options{
			Timing:        timing,
			Driver:        testDriver,
			Parallel:      parallel,
			In:            os.Stdin,
			Out:           cmd.OutOrStdout(),
			ObserveOutput: record.observeOutput,
		})
		if err := writeRunReports(output, record, report); err != nil {
			return codedError(exitExecution, err)
		}
		if record != nil {
			printReplayPaths(cmd.OutOrStdout(), record)
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
	cmd.Flags().StringVar(&output, "report", "", "write a separate atomic JSON acceptance report")
	cmd.Flags().StringVarP(&outDirectory, "output", "o", "", "render one Giztest file's standalone HTML, data and audio into a new directory")
	cmd.Flags().StringVar(&evidence, "evidence", "redacted", "report evidence mode: redacted or full (full may contain sensitive relay text)")
	addTimingFlags(cmd)
	return cmd
}

func writeRunReports(output string, record *replayRecord, report giztest.Report) error {
	reportErr := giztest.WriteReport(output, report)
	if record == nil {
		return reportErr
	}
	return errors.Join(reportErr, record.commit(report))
}

func printReplayPaths(out io.Writer, record *replayRecord) {
	fmt.Fprintf(out, "Giztest output: %s (%d test replays)\n", record.directory.target, len(record.pages))
	for _, page := range record.pages {
		fmt.Fprintln(out, filepath.Join(record.directory.target, filepath.FromSlash(page)))
	}
}

func validateReplayReportOutput(directory, report string) error {
	if directory == "" || report == "" {
		return nil
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(report)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	if relative != "report.json" && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("--report inside --output must be report.json to avoid overwriting replay files")
	}
	return nil
}

func validateRunOptions(parallel int, output, evidence string) error {
	if parallel < 1 {
		return fmt.Errorf("parallel must be positive")
	}
	if evidence != "redacted" && evidence != "full" {
		return fmt.Errorf("evidence must be redacted or full")
	}
	if evidence == "full" && output == "" {
		return fmt.Errorf("full evidence requires --output or --report")
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
