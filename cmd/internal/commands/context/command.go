package contextcmd

import (
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/cmd/internal/clicontext"
	"github.com/GizClaw/gizclaw-go/pkg/giznet/giznoise"
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
	Name             string `json:"name"`
	Current          bool   `json:"current"`
	ServerAddress    string `json:"server_address"`
	ServerTransport  string `json:"server_transport"`
	ServerPublicKey  string `json:"server_public_key"`
	ServerCipherMode string `json:"server_cipher_mode,omitempty"`
	IdentityPublic   string `json:"identity_public"`
}

func buildContextInfo(ctx *clicontext.CLIContext, current string) contextInfo {
	return contextInfo{
		Name:             ctx.Name,
		Current:          ctx.Name == current,
		ServerAddress:    ctx.Config.Server.Address,
		ServerTransport:  ctx.Config.Server.Transport,
		ServerPublicKey:  ctx.Config.Server.PublicKey.String(),
		ServerCipherMode: string(ctx.Config.Server.CipherMode),
		IdentityPublic:   ctx.KeyPair.Public.String(),
	}
}

func newCreateCmd() *cobra.Command {
	var serverAddr, publicKey, cipherMode, transport string
	var publicAPIPort, noiseUDPPort, icePort int

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
			if err := store.CreateWithOptions(name, serverAddr, clicontext.CreateOptions{
				ServerPublicKey: publicKey,
				CipherMode:      giznoise.CipherMode(cipherMode),
				Transport:       transport,
				PublicAPIPort:   publicAPIPort,
				NoiseUDPPort:    noiseUDPPort,
				ICEPort:         icePort,
			}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Context %q created.\n", name)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "", "server address (host:port)")
	cmd.Flags().StringVar(&publicKey, "public-key", "", "server public key (base58btc)")
	cmd.Flags().StringVar(&transport, "transport", "noise", "giznet transport: noise or webrtc")
	cmd.Flags().StringVar(&cipherMode, "cipher-mode", "", "giznet cipher mode: chacha_poly, aes_256_gcm, or plaintext")
	cmd.Flags().IntVar(&publicAPIPort, "public-api-port", 0, "server public API/signaling TCP port")
	cmd.Flags().IntVar(&noiseUDPPort, "noise-udp-port", 0, "server Noise-over-UDP port")
	cmd.Flags().IntVar(&icePort, "ice-port", 0, "server WebRTC ICE UDP/TCP port")
	_ = cmd.MarkFlagRequired("server")
	_ = cmd.MarkFlagRequired("public-key")

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
