package minimaxtenantscmd

import (
	"context"
	"encoding/json"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "minimax-tenants",
		Short: "Manage MiniMax tenants",
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newListCmd(&ctxName),
		newGetCmd(&ctxName),
		newSyncVoicesCmd(&ctxName),
	)
	return cmd
}

func newListCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List MiniMax tenants",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			items, err := adminapi.ListMiniMaxTenants(context.Background(), c)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
		},
	}
}

func newGetCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a MiniMax tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.GetMiniMaxTenant(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func newSyncVoicesCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "sync-voices <id>",
		Short: "Sync MiniMax tenant voices",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := adminapi.SyncMiniMaxTenantVoices(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
}
