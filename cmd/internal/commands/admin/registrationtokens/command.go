package registrationtokenscmd

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
	cmd := &cobra.Command{Use: "registration-tokens", Short: "Manage RegistrationTokens"}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newListCmd(&ctxName),
		newCreateCmd(&ctxName),
		newPutCmd(&ctxName),
		newGetCmd(&ctxName),
		newDeleteCmd(&ctxName),
	)
	return cmd
}

func newListCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List RegistrationTokens", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			items, err := adminapi.ListRegistrationTokens(context.Background(), client)
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
		Use: "create -f <file>", Short: "Create a RegistrationToken", Args: cobra.NoArgs,
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
			item, err := adminapi.CreateRegistrationToken(context.Background(), client, request)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "RegistrationToken JSON file, or '-' for stdin")
	return cmd
}

func newPutCmd(ctxName *string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use: "put <name> -f <file>", Short: "Create or replace a RegistrationToken", Args: cobra.ExactArgs(1),
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
			item, err := adminapi.PutRegistrationToken(context.Background(), client, args[0], request)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "RegistrationToken JSON file, or '-' for stdin")
	return cmd
}

func newGetCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use: "get <name>", Short: "Get a RegistrationToken", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.GetRegistrationToken(context.Background(), client, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func newDeleteCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use: "delete <name>", Short: "Delete a RegistrationToken", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.DeleteRegistrationToken(context.Background(), client, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func readUpsert(cmd *cobra.Command, file string) (adminhttp.RegistrationTokenUpsert, error) {
	var out adminhttp.RegistrationTokenUpsert
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
