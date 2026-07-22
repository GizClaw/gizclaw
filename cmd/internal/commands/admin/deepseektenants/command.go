package deepseektenantscmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "deepseek-tenants",
		Short: "Manage DeepSeek tenants",
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newListCmd(&ctxName),
		newGetCmd(&ctxName),
		newCreateCmd(&ctxName),
		newUpdateCmd(&ctxName),
		newDeleteCmd(&ctxName),
	)
	return cmd
}

func newCreateCmd(ctxName *string) *cobra.Command {
	return newWriteCmd(ctxName, false)
}

func newUpdateCmd(ctxName *string) *cobra.Command {
	return newWriteCmd(ctxName, true)
}

func newWriteCmd(ctxName *string, update bool) *cobra.Command {
	var credentialName, baseURL, description string
	operation := "create"
	short := "Create a DeepSeek tenant"
	if update {
		operation = "update"
		short = "Update a DeepSeek tenant"
	}
	cmd := &cobra.Command{
		Use:   operation + " <name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := tenantName(args[0])
			if err != nil {
				return err
			}
			request := adminhttp.DeepSeekTenantUpsert{Name: name, CredentialName: strings.TrimSpace(credentialName)}
			if value := strings.TrimSpace(baseURL); value != "" {
				request.BaseUrl = &value
			}
			if value := strings.TrimSpace(description); value != "" {
				request.Description = &value
			}
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			var item any
			if update {
				item, err = adminapi.PutDeepSeekTenant(context.Background(), client, name, request)
			} else {
				item, err = adminapi.CreateDeepSeekTenant(context.Background(), client, request)
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&credentialName, "credential-name", "", "DeepSeek credential name")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "optional absolute DeepSeek HTTP(S) endpoint")
	cmd.Flags().StringVar(&description, "description", "", "optional tenant description")
	_ = cmd.MarkFlagRequired("credential-name")
	return cmd
}

func newDeleteCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a DeepSeek tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := tenantName(args[0])
			if err != nil {
				return err
			}
			client, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer client.Close()
			item, err := adminapi.DeleteDeepSeekTenant(context.Background(), client, name)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func newListCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List DeepSeek tenants",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			items, err := adminapi.ListDeepSeekTenants(context.Background(), c)
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
		Short: "Get a DeepSeek tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := tenantName(args[0])
			if err != nil {
				return err
			}
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.GetDeepSeekTenant(context.Background(), c, name)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func tenantName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", errors.New("tenant name must not be empty")
	}
	return name, nil
}
