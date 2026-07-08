package edgecmd

import (
	"github.com/GizClaw/gizclaw-go/cmd/internal/edge"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edge",
		Short: "Manage edge-node ingress",
	}
	cmd.AddCommand(newServeCmd())
	return cmd
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve <dir>",
		Short: "Serve an edge-node workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return edge.Serve(args[0])
		},
	}
}
