package historymigratecmd

import (
	"encoding/json"

	"github.com/GizClaw/gizclaw-go/cmd/internal/server"
	"github.com/spf13/cobra"
)

// NewCmd creates the one-time, offline Workspace History migration command.
func NewCmd() *cobra.Command {
	var workspaceTable, einoTable string
	var dryRun bool
	command := &cobra.Command{
		Use:   "history-migrate <dir>",
		Short: "Import Workspace History and remove both retired history tables",
		Long:  "Stop every Server using the database and update config.yaml to the final SQL History configuration first. This one-time migration preserves Workspace records and audio references, discards Eino History, and deletes both selected source tables atomically. Use --dry-run to validate without changing data.",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := server.MigrateWorkspaceHistory(command.Context(), args[0], workspaceTable, einoTable, dryRun)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&workspaceTable, "workspace-table", "", "dedicated legacy Workspace History table to import and delete")
	command.Flags().StringVar(&einoTable, "eino-table", "", "dedicated legacy Eino History table to discard and delete")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "validate sources and report counts without importing or deleting data")
	_ = command.MarkFlagRequired("workspace-table")
	_ = command.MarkFlagRequired("eino-table")
	return command
}
