// Package pendingdeletionscmd manages deletion recovery through the online Admin API.
package pendingdeletionscmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type connector func(context.Context, string) (*adminhttp.ClientWithResponses, func(), error)

// NewCmd creates bounded online preview, retry and inspection commands.
func NewCmd() *cobra.Command {
	return newCmd(func(ctx context.Context, name string) (*adminhttp.ClientWithResponses, func(), error) {
		client, err := contextconn.Connect(ctx, contextconn.Options{Context: name})
		if err != nil {
			return nil, nil, err
		}
		api, err := client.ServerAdminClient()
		if err != nil {
			client.Close()
			return nil, nil, err
		}
		return api, func() { client.Close() }, nil
	})
}

func newCmd(connect connector) *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{Use: "pending-deletions", Short: "Inspect and recover durable deletion tasks"}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context of the authoritative regional Server")
	var source string
	get := &cobra.Command{Use: "get <deletion-id>", Short: "Read one active task", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(args[0])
		if err != nil || source == "" {
			return errors.New("a UUID and --source are required")
		}
		api, closeClient, err := connect(cmd.Context(), ctxName)
		if err != nil {
			return err
		}
		defer closeClient()
		response, err := api.GetPendingDeletionWithResponse(cmd.Context(), id, &adminhttp.GetPendingDeletionParams{Source: source})
		if err != nil {
			return err
		}
		if response.JSON200 == nil {
			return fmt.Errorf("get pending deletion: HTTP %d", response.StatusCode())
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(response.JSON200)
	}}
	get.Flags().StringVar(&source, "source", "", "task source (peer, workspace, friend_group)")
	cmd.AddCommand(get, newRecoverCmd(&ctxName, connect))
	return cmd
}

type recoveryOptions struct {
	source, kind, errorCode, start, end, cursor string
	limit                                       int
	execute                                     bool
	interval, wait                              time.Duration
}

func newRecoverCmd(ctxName *string, connect connector) *cobra.Command {
	o := recoveryOptions{}
	cmd := &cobra.Command{Use: "recover", Short: "Preview one bounded failed-task page; --execute retries matching tasks", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		params, err := o.parameters()
		if err != nil {
			return err
		}
		api, closeClient, err := connect(cmd.Context(), *ctxName)
		if err != nil {
			return err
		}
		defer closeClient()
		cmd.SilenceUsage = true
		return recoverPage(cmd.Context(), api, o, params, cmd.OutOrStdout())
	}}
	cmd.Flags().StringVar(&o.source, "source", "", "required source (peer, workspace, friend_group)")
	cmd.Flags().StringVar(&o.kind, "kind", "", "optional exact resource kind")
	cmd.Flags().StringVar(&o.errorCode, "error-code", "", "optional exact last failure code")
	cmd.Flags().StringVar(&o.start, "from", "", "inclusive creation time (RFC3339)")
	cmd.Flags().StringVar(&o.end, "until", "", "exclusive creation time (RFC3339)")
	cmd.Flags().StringVar(&o.cursor, "cursor", "", "continue after the previous page's next_cursor")
	cmd.Flags().IntVar(&o.limit, "limit", 100, "maximum scanned tasks in this invocation (1..500)")
	cmd.Flags().BoolVar(&o.execute, "execute", false, "retry selected failed tasks after emitting the preview")
	cmd.Flags().DurationVar(&o.interval, "interval", 200*time.Millisecond, "minimum interval between recovery requests (100ms..1m); concurrency is one")
	cmd.Flags().DurationVar(&o.wait, "wait", 0, "wait up to this duration for accepted tasks (0..30m); zero reports queued only")
	return cmd
}

func (o recoveryOptions) parameters() (*adminhttp.ListPendingDeletionsParams, error) {
	if o.source != "peer" && o.source != "workspace" && o.source != "friend_group" {
		return nil, errors.New("--source must be peer, workspace or friend_group")
	}
	if o.limit < 1 || o.limit > 500 {
		return nil, errors.New("--limit must be 1..500")
	}
	if o.interval < 100*time.Millisecond || o.interval > time.Minute {
		return nil, errors.New("--interval must be 100ms..1m")
	}
	if o.wait < 0 || o.wait > 30*time.Minute || (!o.execute && o.wait != 0) {
		return nil, errors.New("--wait must be 0..30m and requires --execute")
	}
	params := &adminhttp.ListPendingDeletionsParams{Source: &o.source, Status: new(apitypes.PendingDeletionStatusFailed), Limit: new(int32(o.limit))}
	if o.kind != "" {
		kind := apitypes.PendingDeletionKind(o.kind)
		if !kind.Valid() {
			return nil, errors.New("--kind is invalid")
		}
		params.Kind = &kind
	}
	for _, value := range []struct {
		text   string
		target **int64
	}{{o.start, &params.StartTimeMs}, {o.end, &params.EndTimeMs}} {
		if value.text == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339Nano, value.text)
		if err != nil {
			return nil, errors.New("--from and --until require RFC3339 timestamps")
		}
		*value.target = new(parsed.UnixMilli())
	}
	if params.StartTimeMs != nil && params.EndTimeMs != nil && *params.EndTimeMs <= *params.StartTimeMs {
		return nil, errors.New("--until must be after --from")
	}
	if o.cursor != "" {
		params.Cursor = &o.cursor
	}
	return params, nil
}

type recoveryEvent struct {
	Type       string                        `json:"type"`
	Task       *apitypes.PendingDeletionTask `json:"task,omitempty"`
	Outcome    string                        `json:"outcome,omitempty"`
	Code       string                        `json:"code,omitempty"`
	Scanned    int                           `json:"scanned,omitempty"`
	Selected   int                           `json:"selected,omitempty"`
	Counts     map[string]int                `json:"counts,omitempty"`
	NextCursor *string                       `json:"next_cursor,omitempty"`
}

func recoverPage(ctx context.Context, api *adminhttp.ClientWithResponses, o recoveryOptions, params *adminhttp.ListPendingDeletionsParams, out io.Writer) error {
	page, err := api.ListPendingDeletionsWithResponse(ctx, params)
	if err != nil {
		return err
	}
	if page.JSON200 == nil {
		return fmt.Errorf("list failed tasks: HTTP %d", page.StatusCode())
	}
	if len(page.JSON200.Items) > o.limit {
		return errors.New("Server exceeded requested page limit")
	}
	var selected []apitypes.PendingDeletionTask
	counts := map[string]int{}
	for _, task := range page.JSON200.Items {
		if task.Source != o.source || task.Status != apitypes.PendingDeletionStatusFailed || (o.kind != "" && string(task.Kind) != o.kind) {
			return errors.New("Server returned a task outside the requested scope")
		}
		code := taskErrorCode(task)
		if o.errorCode != "" && code != o.errorCode {
			continue
		}
		selected = append(selected, task)
		counts[task.Source+"/"+string(task.Kind)+"/"+code]++
	}
	encoder := json.NewEncoder(out)
	for _, task := range selected {
		if err := encoder.Encode(recoveryEvent{Type: "preview", Task: &task}); err != nil {
			return err
		}
	}
	if err := encoder.Encode(recoveryEvent{Type: "selection", Scanned: len(page.JSON200.Items), Selected: len(selected), Counts: counts, NextCursor: page.JSON200.NextCursor}); err != nil {
		return err
	}
	if !o.execute {
		return nil
	}
	// Schedule from the actual previous request start. A periodic ticker can
	// retain a stale tick and burst two requests after a slow response.
	nextRequest := time.Now().Add(o.interval)
	waitTurn := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if delay := time.Until(nextRequest); delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
		nextRequest = time.Now().Add(o.interval)
		return nil
	}
	results := map[string]int{}
	var accepted []apitypes.PendingDeletionTask
	for _, task := range selected {
		if err := waitTurn(ctx); err != nil {
			return err
		}
		current, err := api.GetPendingDeletionWithResponse(ctx, task.DeletionId, &adminhttp.GetPendingDeletionParams{Source: task.Source})
		if err != nil {
			return err
		}
		outcome, code := "changed", ""
		if current.JSON200 != nil && current.JSON200.Status == apitypes.PendingDeletionStatusFailed && current.JSON200.UpdatedAt.Equal(task.UpdatedAt) && taskErrorCode(*current.JSON200) == taskErrorCode(task) {
			if err := waitTurn(ctx); err != nil {
				return err
			}
			retry, err := api.RetryPendingDeletionWithResponse(ctx, task.DeletionId, &adminhttp.RetryPendingDeletionParams{Source: task.Source})
			if err != nil {
				return err
			}
			switch {
			case retry.JSON200 != nil:
				outcome = "queued"
				accepted = append(accepted, *retry.JSON200)
			case retry.JSON404 != nil:
				outcome = "already_absent"
			case retry.JSON409 != nil:
				outcome = "rejected"
				code = retry.JSON409.Error.Code
			default:
				outcome = "request_failed"
				code = fmt.Sprintf("HTTP_%d", retry.StatusCode())
			}
		} else if current.StatusCode() == 404 {
			outcome = "already_absent"
		} else if current.JSON200 == nil {
			outcome = "request_failed"
			code = fmt.Sprintf("HTTP_%d", current.StatusCode())
		}
		results[outcome]++
		if err := encoder.Encode(recoveryEvent{Type: "retry", Task: &task, Outcome: outcome, Code: code}); err != nil {
			return err
		}
	}
	if o.wait > 0 && len(accepted) > 0 {
		waitCtx, cancel := context.WithTimeout(ctx, o.wait)
		defer cancel()
		for len(accepted) > 0 && waitCtx.Err() == nil {
			remaining := make([]apitypes.PendingDeletionTask, 0, len(accepted))
			for i, task := range accepted {
				if err := waitTurn(waitCtx); err != nil {
					remaining = append(remaining, accepted[i:]...)
					break
				}
				response, err := api.GetPendingDeletionWithResponse(waitCtx, task.DeletionId, &adminhttp.GetPendingDeletionParams{Source: task.Source})
				if err != nil {
					if waitCtx.Err() != nil {
						remaining = append(remaining, accepted[i:]...)
						break
					}
					return err
				}
				outcome := ""
				if response.StatusCode() == 404 {
					outcome = "completed"
				} else if response.JSON200 != nil && response.JSON200.Status == apitypes.PendingDeletionStatusFailed {
					outcome = "failed"
					task = *response.JSON200
				} else if response.JSON200 == nil {
					return fmt.Errorf("observe recovery: HTTP %d", response.StatusCode())
				} else {
					remaining = append(remaining, *response.JSON200)
					continue
				}
				results[outcome]++
				if err := encoder.Encode(recoveryEvent{Type: "completion", Task: &task, Outcome: outcome, Code: taskErrorCode(task)}); err != nil {
					return err
				}
			}
			accepted = remaining
		}
		results["pending"] = len(accepted)
		for _, task := range accepted {
			if err := encoder.Encode(recoveryEvent{Type: "completion", Task: &task, Outcome: "pending"}); err != nil {
				return err
			}
		}
	}
	if err := encoder.Encode(recoveryEvent{Type: "result", Counts: results, NextCursor: page.JSON200.NextCursor}); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if results["rejected"]+results["failed"]+results["pending"]+results["request_failed"]+results["changed"] != 0 {
		return errors.New("some selected tasks remain unresolved; inspect the reported outcomes before retrying")
	}
	return nil
}

func taskErrorCode(task apitypes.PendingDeletionTask) string {
	if task.LastErrorCode == nil {
		return ""
	}
	return *task.LastErrorCode
}
