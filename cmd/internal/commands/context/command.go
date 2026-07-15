package contextcmd

import (
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clicontext"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/contextstore"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage server connection contexts",
	}

	cmd.AddCommand(
		newCreateCmd(),
		newUseCmd(),
		newDeleteCmd(),
		newListCmd(),
		newInfoCmd(),
		newShowCmd(),
	)

	return cmd
}

type contextInfo struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Current        bool   `json:"current"`
	ServerEndpoint string `json:"server_endpoint"`
	IdentityPublic string `json:"identity_public"`
}

func buildContextInfo(ctx *contextstore.Context, current string) contextInfo {
	return contextInfo{
		Name:           ctx.Name,
		Description:    ctx.Config.Description,
		Current:        ctx.Name == current,
		ServerEndpoint: ctx.Config.Server.Endpoint,
		IdentityPublic: ctx.KeyPair.Public.String(),
	}
}

func newCreateCmd() *cobra.Command {
	var serverAddr, description string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := clicontext.DefaultStore()
			if err != nil {
				return err
			}
			name := args[0]
			if err := store.CreateWithOptions(name, serverAddr, contextstore.CreateOptions{
				Description: description,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Context %q created.\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "", "server endpoint (host:port)")
	cmd.Flags().StringVar(&description, "description", "", "human-readable context description")
	_ = cmd.MarkFlagRequired("server")

	return cmd
}

func newUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch to a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := clicontext.DefaultStore()
			if err != nil {
				return err
			}
			if err := store.Use(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched to context %q.\n", args[0])
			return nil
		},
	}
}

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := clicontext.DefaultStore()
			if err != nil {
				return err
			}
			if err := store.Delete(args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted context %q.\n", args[0])
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all contexts",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := clicontext.DefaultStore()
			if err != nil {
				return err
			}
			names, current, err := store.List()
			if err != nil {
				return err
			}
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No contexts found.")
				return nil
			}
			for _, name := range names {
				marker := "  "
				if name == current {
					marker = "* "
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", marker, name)
			}
			return nil
		},
	}
}

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show current context information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := clicontext.DefaultStore()
			if err != nil {
				return err
			}
			ctx, err := store.Current()
			if err != nil {
				return err
			}
			if ctx == nil {
				return fmt.Errorf("no active context; run 'gizclaw context create' first")
			}
			_, current, err := store.List()
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(buildContextInfo(ctx, current))
		},
	}
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show context information by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := clicontext.DefaultStore()
			if err != nil {
				return err
			}
			ctx, err := store.LoadByName(args[0])
			if err != nil {
				return err
			}
			_, current, err := store.List()
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(buildContextInfo(ctx, current))
		},
	}
}
