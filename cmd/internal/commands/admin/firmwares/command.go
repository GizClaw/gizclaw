package firmwarescmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/spf13/cobra"
)

func NewCmd() *cobra.Command {
	var ctxName string
	cmd := &cobra.Command{
		Use:   "firmwares",
		Short: "Manage firmware release lines",
	}
	cmd.PersistentFlags().StringVar(&ctxName, "context", "", "context name (default: current)")
	cmd.AddCommand(
		newListCmd(&ctxName),
		newCreateCmd(&ctxName),
		newGetCmd(&ctxName),
		newPutCmd(&ctxName),
		newDeleteCmd(&ctxName),
		newReleaseCmd(&ctxName),
		newRollbackCmd(&ctxName),
		newUploadBinCmd(&ctxName),
	)
	return cmd
}

func newListCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List firmwares",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			items, err := adminapi.ListFirmwares(context.Background(), c)
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
		Use:   "create -f <file>",
		Short: "Create a firmware",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := readFirmwareUpsert(cmd, file)
			if err != nil {
				return err
			}
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.CreateFirmware(context.Background(), c, req)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "firmware JSON file, or '-' for stdin")
	return cmd
}

func newGetCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Get a firmware",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.GetFirmware(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func newPutCmd(ctxName *string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "put <name> -f <file>",
		Short: "Create or update a firmware",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := readFirmwareUpsert(cmd, file)
			if err != nil {
				return err
			}
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.PutFirmware(context.Background(), c, args[0], req)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "firmware JSON file, or '-' for stdin")
	return cmd
}

func newDeleteCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a firmware",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.DeleteFirmware(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func newReleaseCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "release <name>",
		Short: "Promote firmware slots",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.ReleaseFirmware(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func newRollbackCmd(ctxName *string) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <name>",
		Short: "Rollback firmware stable slot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.RollbackFirmware(context.Background(), c, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
}

func newUploadBinCmd(ctxName *string) *cobra.Command {
	var channel string
	var bin string
	var file string
	cmd := &cobra.Command{
		Use:   "upload-bin <name> --channel <channel> --bin <bin> -f <file>",
		Short: "Upload a firmware bin payload",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(channel) == "" {
				return fmt.Errorf("required flag: --channel")
			}
			if strings.TrimSpace(bin) == "" {
				return fmt.Errorf("required flag: --bin")
			}
			r, closeFn, err := openUploadFile(cmd, file)
			if err != nil {
				return err
			}
			defer closeFn()
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.UploadFirmwareBin(context.Background(), c, args[0], channel, bin, r)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "firmware channel/slot")
	cmd.Flags().StringVar(&bin, "bin", "", "firmware bin name")
	cmd.Flags().StringVarP(&file, "file", "f", "", "bin file, or '-' for stdin")
	return cmd
}

func openUploadFile(cmd *cobra.Command, file string) (io.Reader, func(), error) {
	if strings.TrimSpace(file) == "" {
		return nil, func() {}, fmt.Errorf("required flag: --file")
	}
	if file == "-" {
		return cmd.InOrStdin(), func() {}, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { _ = f.Close() }, nil
}

func readFirmwareUpsert(cmd *cobra.Command, file string) (adminservice.FirmwareUpsert, error) {
	if strings.TrimSpace(file) == "" {
		return adminservice.FirmwareUpsert{}, fmt.Errorf("required flag: --file")
	}
	var r io.Reader
	if file == "-" {
		r = cmd.InOrStdin()
	} else {
		f, err := os.Open(file)
		if err != nil {
			return adminservice.FirmwareUpsert{}, err
		}
		defer f.Close()
		r = f
	}
	var req adminservice.FirmwareUpsert
	if err := json.NewDecoder(r).Decode(&req); err != nil {
		return adminservice.FirmwareUpsert{}, err
	}
	return req, nil
}
