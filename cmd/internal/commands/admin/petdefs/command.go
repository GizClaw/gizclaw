package petdefscmd

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
		Use:   "pet-defs",
		Short: "Manage pet definitions",
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(newUploadPixaCmd(&ctxName))
	return cmd
}

func newUploadPixaCmd(ctxName *string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "upload-pixa <name> -f <asset.pixa>",
		Short: "Upload a PetDef PIXA asset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body, closeFn, err := openPixaUpload(cmd, file)
			if err != nil {
				return err
			}
			defer closeFn()

			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()

			item, err := adminapi.UploadPetDefPixa(context.Background(), client, args[0], body)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "PIXA file, or '-' for stdin")
	return cmd
}

func openPixaUpload(cmd *cobra.Command, file string) (io.Reader, func() error, error) {
	if strings.TrimSpace(file) == "" {
		return nil, nil, fmt.Errorf("required flag: --file")
	}
	if file == "-" {
		return cmd.InOrStdin(), func() error { return nil }, nil
	}
	body, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	return body, body.Close, nil
}
