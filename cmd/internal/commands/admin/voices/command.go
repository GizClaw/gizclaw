package voicescmd

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
		Use:   "voices",
		Short: "Manage voice catalog",
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newListCmd(&ctxName),
		newGetCmd(&ctxName),
	)
	return cmd
}

func newListCmd(ctxName *string) *cobra.Command {
	var source, providerKind, providerID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List voices",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			items, err := adminapi.ListVoices(context.Background(), c, source, providerKind, providerID)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "filter by source")
	cmd.Flags().StringVar(&providerKind, "provider-kind", "", "filter by provider kind")
	cmd.Flags().StringVar(&providerID, "provider-id", "", "filter by provider ID")
	return cmd
}

func newGetCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a voice",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.GetVoice(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}
