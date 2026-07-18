package peerscmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/spf13/cobra"
)

var (
	connectFromContext = connection.ConnectFromContext
	listPeers          = adminapi.ListPeers
	getPeer            = adminapi.GetPeer
	findPubKeyBySN     = adminapi.FindPubKeyBySN
	findPubKeyByIMEI   = adminapi.FindPubKeyByIMEI
	approvePeer        = adminapi.ApprovePeer
	blockPeer          = adminapi.BlockPeer
	getPeerInfo        = adminapi.GetPeerInfo
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
