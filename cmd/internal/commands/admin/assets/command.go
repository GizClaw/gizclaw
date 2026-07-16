package assetscmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/spf13/cobra"
)

// NewCmd creates the Admin AssetService command group.
func NewCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "assets",
		Short: "Manage shared immutable assets",
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newUploadCmd(&ctxName),
		newGetCmd(&ctxName),
		newDownloadCmd(&ctxName),
		newDeleteCmd(&ctxName),
	)
	return cmd
}

func newUploadCmd(ctxName *string) *cobra.Command {
	var file string
	var mediaType string
	var expiresAt string
	cmd := &cobra.Command{
		Use:   "upload -f <file> --media-type <type>",
		Short: "Upload a new immutable asset",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(mediaType) == "" {
				return fmt.Errorf("required flag: --media-type")
			}
			body, closeBody, err := openUpload(cmd, file)
			if err != nil {
				return err
			}
			defer closeBody()
			deadline, err := parseExpiration(expiresAt)
			if err != nil {
				return err
			}
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			stored, err := adminapi.UploadAsset(context.Background(), client, mediaType, deadline, body)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(stored)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "asset file, or '-' for stdin")
	cmd.Flags().StringVar(&mediaType, "media-type", "", "canonical media type, for example image/png")
	cmd.Flags().StringVar(&expiresAt, "expires-at", "", "optional RFC3339 expiration deadline")
	return cmd
}

func newGetCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <asset-ref>",
		Short: "Get asset metadata and reverse bindings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			stored, err := adminapi.GetAsset(context.Background(), client, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(stored)
		},
	}
}

func newDownloadCmd(ctxName *string) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "download <asset-ref> -o <file>",
		Short: "Download asset bytes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(output) == "" {
				return fmt.Errorf("required flag: --output")
			}
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			body, err := adminapi.DownloadAsset(context.Background(), client, args[0])
			if err != nil {
				return err
			}
			if err := os.WriteFile(output, body, 0o644); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"bytes":  len(body),
				"output": output,
			})
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file")
	return cmd
}

func newDeleteCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <asset-ref>",
		Short: "Delete an unbound asset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			if err := adminapi.DeleteAsset(context.Background(), client, args[0]); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"deleted": true,
				"ref":     args[0],
			})
		},
	}
}

func openUpload(cmd *cobra.Command, file string) (io.Reader, func() error, error) {
	switch strings.TrimSpace(file) {
	case "":
		return nil, nil, fmt.Errorf("required flag: --file")
	case "-":
		return cmd.InOrStdin(), func() error { return nil }, nil
	default:
		opened, err := os.Open(file)
		if err != nil {
			return nil, nil, err
		}
		return opened, opened.Close, nil
	}
}

func parseExpiration(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("invalid --expires-at: %w", err)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}
