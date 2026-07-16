package workflowscmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "workflows",
		Short: "Manage workflows",
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newListCmd(&ctxName),
		newGetCmd(&ctxName),
		newUploadIconCmd(&ctxName),
		newDownloadIconCmd(&ctxName),
		newDeleteIconCmd(&ctxName),
	)
	return cmd
}

func newUploadIconCmd(ctxName *string) *cobra.Command {
	var format string
	var file string
	cmd := &cobra.Command{
		Use:   "upload-icon <name> --format <png|pixa> -f <asset>",
		Short: "Upload or replace a workflow icon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateIconFormat(format); err != nil {
				return err
			}
			reader, closeFn, err := openIconUpload(cmd, file)
			if err != nil {
				return err
			}
			defer closeFn()
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.UploadWorkflowIcon(context.Background(), c, args[0], format, reader)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "icon format: png or pixa")
	cmd.Flags().StringVarP(&file, "file", "f", "", "icon file, or '-' for stdin")
	return cmd
}

func newDownloadIconCmd(ctxName *string) *cobra.Command {
	var format string
	var output string
	cmd := &cobra.Command{
		Use:   "download-icon <name> --format <png|pixa> -o <asset>",
		Short: "Download a workflow icon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateIconFormat(format); err != nil {
				return err
			}
			if strings.TrimSpace(output) == "" {
				return fmt.Errorf("required flag: --output")
			}
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			body, err := adminapi.DownloadWorkflowIcon(context.Background(), c, args[0], format)
			if err != nil {
				return err
			}
			if err := os.WriteFile(output, body, 0o644); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"output": output, "bytes": len(body)})
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "icon format: png or pixa")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output icon file")
	return cmd
}

func newDeleteIconCmd(ctxName *string) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "delete-icon <name> --format <png|pixa>",
		Short: "Delete a workflow icon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateIconFormat(format); err != nil {
				return err
			}
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.DeleteWorkflowIcon(context.Background(), c, args[0], format)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "icon format: png or pixa")
	return cmd
}

func validateIconFormat(format string) error {
	if format != "png" && format != "pixa" {
		return fmt.Errorf("--format must be png or pixa")
	}
	return nil
}

func openIconUpload(cmd *cobra.Command, file string) (io.Reader, func() error, error) {
	if strings.TrimSpace(file) == "" {
		return nil, func() error { return nil }, fmt.Errorf("required flag: --file")
	}
	if file == "-" {
		return cmd.InOrStdin(), func() error { return nil }, nil
	}
	handle, err := os.Open(file)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return handle, handle.Close, nil
}

func newListCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List workflows",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			items, err := adminapi.ListWorkflows(context.Background(), c)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
		},
	}
}

func newGetCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Get a workflow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.GetWorkflow(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}
