package firmwarescmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "firmwares",
		Short: "Manage declarative firmware channels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newListCmd(&ctxName),
		newCreateCmd(&ctxName),
		newGetCmd(&ctxName),
		newPutCmd(&ctxName),
		newDeleteCmd(&ctxName),
	)
	return cmd
}

func newListCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List firmwares",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			items, err := adminapi.ListFirmwares(context.Background(), client)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
		},
	}
}

func newCreateCmd(ctxName *string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use: "create -f <file>", Short: "Create a firmware", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			request, err := readFirmwareUpsert(cmd, file)
			if err != nil {
				return err
			}
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.CreateFirmware(context.Background(), client, request)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "firmware JSON file, or '-' for stdin")
	return cmd
}

func newGetCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use: "get <id>", Short: "Get a firmware", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.GetFirmware(context.Background(), client, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func newPutCmd(ctxName *string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use: "put <id> -f <file>", Short: "Update a firmware", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := readFirmwareUpsert(cmd, file)
			if err != nil {
				return err
			}
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.PutFirmware(context.Background(), client, args[0], request)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "firmware JSON file, or '-' for stdin")
	return cmd
}

func newDeleteCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use: "delete <id>", Short: "Delete a firmware", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.DeleteFirmware(context.Background(), client, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func readFirmwareUpsert(cmd *cobra.Command, file string) (adminhttp.FirmwareUpsert, error) {
	if strings.TrimSpace(file) == "" {
		return adminhttp.FirmwareUpsert{}, fmt.Errorf("required flag: --file")
	}
	var reader io.Reader
	if file == "-" {
		reader = cmd.InOrStdin()
	} else {
		opened, err := os.Open(file)
		if err != nil {
			return adminhttp.FirmwareUpsert{}, err
		}
		defer opened.Close()
		reader = opened
	}
	var request adminhttp.FirmwareUpsert
	if err := json.NewDecoder(reader).Decode(&request); err != nil {
		return adminhttp.FirmwareUpsert{}, err
	}
	return request, nil
}
