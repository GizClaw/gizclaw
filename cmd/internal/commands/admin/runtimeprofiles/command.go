package runtimeprofilescmd

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
	cmd := &cobra.Command{Use: "runtime-profiles", Short: "Manage RuntimeProfiles"}
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
		Use: "list", Short: "List RuntimeProfiles", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			items, err := adminapi.ListRuntimeProfiles(context.Background(), client)
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
		Use: "create -f <file>", Short: "Create a RuntimeProfile", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			request, err := readUpsert(cmd, file)
			if err != nil {
				return err
			}
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.CreateRuntimeProfile(context.Background(), client, request)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "RuntimeProfile JSON file, or '-' for stdin")
	return cmd
}

func newGetCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use: "get <name>", Short: "Get a RuntimeProfile", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.GetRuntimeProfile(context.Background(), client, args[0])
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
		Use: "put <name> -f <file>", Short: "Create or update a RuntimeProfile", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			request, err := readUpsert(cmd, file)
			if err != nil {
				return err
			}
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.PutRuntimeProfile(context.Background(), client, args[0], request)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "RuntimeProfile JSON file, or '-' for stdin")
	return cmd
}

func newDeleteCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use: "delete <name>", Short: "Delete a RuntimeProfile", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.DeleteRuntimeProfile(context.Background(), client, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func readUpsert(cmd *cobra.Command, file string) (adminhttp.RuntimeProfileUpsert, error) {
	var out adminhttp.RuntimeProfileUpsert
	file = strings.TrimSpace(file)
	if file == "" {
		return out, fmt.Errorf("required flag: --file")
	}
	var reader io.Reader
	if file == "-" {
		reader = cmd.InOrStdin()
	} else {
		handle, err := os.Open(file)
		if err != nil {
			return out, err
		}
		defer handle.Close()
		reader = handle
	}
	if err := json.NewDecoder(reader).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}
