package peerscmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/commands/admin/cmdutil"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/spf13/cobra"
)

type peerConfigClient interface {
	GetPeerConfig(ctx context.Context, publicKey string) (apitypes.Configuration, error)
	PutPeerConfig(ctx context.Context, publicKey string, cfg apitypes.Configuration) (apitypes.Configuration, error)
	Close() error
}

type peerConfigBridge struct {
	c *gizcli.Client
}

func (g *peerConfigBridge) GetPeerConfig(ctx context.Context, publicKey string) (apitypes.Configuration, error) {
	return getPeerConfig(ctx, g.c, publicKey)
}

func (g *peerConfigBridge) PutPeerConfig(ctx context.Context, publicKey string, cfg apitypes.Configuration) (apitypes.Configuration, error) {
	return putPeerConfig(ctx, g.c, publicKey, cfg)
}

func (g *peerConfigBridge) Close() error {
	return g.c.Close()
}

var openPeerConfigClient = func(ctxName string) (peerConfigClient, error) {
	c, err := connectFromContext(ctxName)
	if err != nil {
		return nil, err
	}
	return &peerConfigBridge{c: c}, nil
}

var (
	connectFromContext = connection.ConnectFromContext
	listPeers          = adminapi.ListPeers
	getPeer            = adminapi.GetPeer
	findPubKeyBySN     = adminapi.FindPubKeyBySN
	findPubKeyByIMEI   = adminapi.FindPubKeyByIMEI
	approvePeer        = adminapi.ApprovePeer
	blockPeer          = adminapi.BlockPeer
	getPeerInfo        = adminapi.GetPeerInfo
	getPeerConfig      = adminapi.GetPeerConfig
	putPeerConfig      = adminapi.PutPeerConfig
	getPeerRuntime     = adminapi.GetPeerRuntime
	deletePeer         = adminapi.DeletePeer
	refreshPeer        = adminapi.RefreshPeer
)

func NewCmd() *cobra.Command {
	return newCmd("peers", "Manage peers")
}

func newCmd(use, short string) *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List peers",
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				items, err := listPeers(context.Background(), c)
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(items)
			},
		},
		&cobra.Command{
			Use:   "get <pubkey>",
			Short: "Get peer registration",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				item, err := getPeer(context.Background(), c, args[0])
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
			},
		},
		&cobra.Command{
			Use:   "resolve-sn <sn>",
			Short: "Resolve public key by SN",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				publicKey, err := findPubKeyBySN(context.Background(), c, args[0])
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), publicKey)
				return nil
			},
		},
		&cobra.Command{
			Use:   "resolve-imei <tac> <serial>",
			Short: "Resolve public key by IMEI",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				publicKey, err := findPubKeyByIMEI(context.Background(), c, args[0], args[1])
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), publicKey)
				return nil
			},
		},
		&cobra.Command{
			Use:   "approve <pubkey> <role>",
			Short: "Approve peer role",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				item, err := approvePeer(context.Background(), c, args[0], apitypes.PeerRole(args[1]))
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), item.PublicKey, item.Role, item.Status)
				return nil
			},
		},
		&cobra.Command{
			Use:   "block <pubkey>",
			Short: "Block peer",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				item, err := blockPeer(context.Background(), c, args[0])
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), item.PublicKey, item.Status)
				return nil
			},
		},
		&cobra.Command{
			Use:   "info <pubkey>",
			Short: "Get peer info snapshot",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				item, err := getPeerInfo(context.Background(), c, args[0])
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
			},
		},
		&cobra.Command{
			Use:   "config <pubkey>",
			Short: "Get peer config snapshot",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				item, err := getPeerConfig(context.Background(), c, args[0])
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
			},
		},
		newPutConfigCmd(&ctxName),
		&cobra.Command{
			Use:   "runtime <pubkey>",
			Short: "Get peer runtime snapshot",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				item, err := getPeerRuntime(context.Background(), c, args[0])
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
			},
		},
		&cobra.Command{
			Use:   "delete <pubkey>",
			Short: "Delete peer registration",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				item, err := deletePeer(context.Background(), c, args[0])
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
			},
		},
		&cobra.Command{
			Use:   "refresh <pubkey>",
			Short: "Refresh peer from device-side API",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				c, err := connectFromContext(ctxName)
				if err != nil {
					return err
				}
				defer c.Close()
				item, err := refreshPeer(context.Background(), c, args[0])
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
			},
		},
	)
	return cmd
}

func newPutConfigCmd(ctxName *string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "put-config <pubkey> --file <config.json>",
		Short: "Replace peer config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := cmdutil.ReadJSONFile[apitypes.Configuration](file)
			if err != nil {
				return err
			}
			c, err := openPeerConfigClient(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := c.PutPeerConfig(context.Background(), args[0], cfg)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "path to config JSON")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
