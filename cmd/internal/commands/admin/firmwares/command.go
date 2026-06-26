package firmwarescmd

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/adminapi"
	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
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
		newUploadArtifactCmd(&ctxName),
		newDownloadArtifactCmd(&ctxName),
		newDeleteArtifactCmd(&ctxName),
		newArtifactCmd(&ctxName),
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

func newUploadArtifactCmd(ctxName *string) *cobra.Command {
	var channel string
	var file string
	var dir string
	cmd := &cobra.Command{
		Use:   "upload-artifact <name> --channel <channel> (-f artifact.tar | -d dir)",
		Short: "Upload a firmware channel artifact tar",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(channel) == "" {
				return fmt.Errorf("required flag: --channel")
			}
			r, closeFn, err := openArtifactUpload(cmd, file, dir)
			if err != nil {
				return err
			}
			closed := false
			defer func() {
				if !closed {
					_ = closeFn()
				}
			}()
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.UploadFirmwareArtifact(context.Background(), c, args[0], channel, r)
			closeErr := closeFn()
			closed = true
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "firmware channel/slot")
	cmd.Flags().StringVarP(&file, "file", "f", "", "artifact tar file, or '-' for stdin")
	cmd.Flags().StringVarP(&dir, "dir", "d", "", "directory to pack and upload as artifact.tar")
	return cmd
}

func newDownloadArtifactCmd(ctxName *string) *cobra.Command {
	var channel string
	var output string
	cmd := &cobra.Command{
		Use:   "download-artifact <name> --channel <channel> -o artifact.tar",
		Short: "Download a firmware channel artifact tar",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(channel) == "" {
				return fmt.Errorf("required flag: --channel")
			}
			if strings.TrimSpace(output) == "" {
				return fmt.Errorf("required flag: --output")
			}
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			body, err := adminapi.DownloadFirmwareArtifact(context.Background(), c, args[0], channel)
			if err != nil {
				return err
			}
			if err := os.WriteFile(output, body, 0644); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"output": output, "bytes": len(body)})
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "firmware channel/slot")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output tar file")
	return cmd
}

func newDeleteArtifactCmd(ctxName *string) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "delete-artifact <name> --channel <channel>",
		Short: "Delete a firmware channel artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(channel) == "" {
				return fmt.Errorf("required flag: --channel")
			}
			c, err := connection.ConnectFromContext(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			item, err := adminapi.DeleteFirmwareArtifact(context.Background(), c, args[0], channel)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "firmware channel/slot")
	return cmd
}

func newArtifactCmd(ctxName *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "artifact",
		Short: "Inspect or download firmware artifact files",
	}
	cmd.AddCommand(
		newArtifactListCmd(ctxName),
		newArtifactTreeCmd(ctxName),
		newArtifactStatCmd(ctxName),
		newArtifactDownloadCmd(ctxName),
	)
	return cmd
}

func newArtifactListCmd(ctxName *string) *cobra.Command {
	var channel string
	var entryPath string
	cmd := &cobra.Command{
		Use:   "ls <name> --channel <channel> [--path dir-or-file]",
		Short: "List artifact entries",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFirmwareAdmin(ctxName, channel)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := adminapi.ListFirmwareArtifactEntries(context.Background(), c, args[0], channel, entryPath)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "firmware channel/slot")
	cmd.Flags().StringVar(&entryPath, "path", "", "artifact path")
	return cmd
}

func newArtifactTreeCmd(ctxName *string) *cobra.Command {
	var channel string
	var entryPath string
	cmd := &cobra.Command{
		Use:   "tree <name> --channel <channel> [--path dir]",
		Short: "List artifact entries recursively",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFirmwareAdmin(ctxName, channel)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := adminapi.TreeFirmwareArtifactEntries(context.Background(), c, args[0], channel, entryPath)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "firmware channel/slot")
	cmd.Flags().StringVar(&entryPath, "path", "", "artifact path")
	return cmd
}

func newArtifactStatCmd(ctxName *string) *cobra.Command {
	var channel string
	var entryPath string
	cmd := &cobra.Command{
		Use:   "stat <name> --channel <channel> [--path file-or-dir]",
		Short: "Get artifact stats",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFirmwareAdmin(ctxName, channel)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := adminapi.StatFirmwareArtifactEntry(context.Background(), c, args[0], channel, entryPath)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "firmware channel/slot")
	cmd.Flags().StringVar(&entryPath, "path", "", "artifact path")
	return cmd
}

func newArtifactDownloadCmd(ctxName *string) *cobra.Command {
	var channel string
	var entryPath string
	var output string
	cmd := &cobra.Command{
		Use:   "dl <name> --channel <channel> --path file -o output",
		Short: "Download an artifact file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := connectFirmwareAdmin(ctxName, channel)
			if err != nil {
				return err
			}
			defer c.Close()
			if strings.TrimSpace(entryPath) == "" {
				return fmt.Errorf("required flag: --path")
			}
			if strings.TrimSpace(output) == "" {
				return fmt.Errorf("required flag: --output")
			}
			body, err := adminapi.DownloadFirmwareArtifactEntry(context.Background(), c, args[0], channel, entryPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(output, body, 0644); err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"output": output, "bytes": len(body)})
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "firmware channel/slot")
	cmd.Flags().StringVar(&entryPath, "path", "", "artifact file path")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file")
	return cmd
}

func connectFirmwareAdmin(ctxName *string, channel string) (*gizcli.Client, error) {
	if strings.TrimSpace(channel) == "" {
		return nil, fmt.Errorf("required flag: --channel")
	}
	return connection.ConnectFromContext(*ctxName)
}

func openArtifactUpload(cmd *cobra.Command, file, dir string) (io.Reader, func() error, error) {
	file = strings.TrimSpace(file)
	dir = strings.TrimSpace(dir)
	if (file == "" && dir == "") || (file != "" && dir != "") {
		return nil, func() error { return nil }, fmt.Errorf("exactly one of --file or --dir is required")
	}
	if dir != "" {
		return openUploadDir(dir)
	}
	if file == "-" {
		return cmd.InOrStdin(), func() error { return nil }, nil
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return f, f.Close, nil
}

func openUploadDir(dir string) (io.Reader, func() error, error) {
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		errCh <- writeDirTar(pw, dir)
	}()
	closeFn := func() error {
		_ = pr.Close()
		return <-errCh
	}
	return pr, closeFn, nil
}

func writeDirTar(w *io.PipeWriter, dir string) error {
	tw := tar.NewWriter(w)
	err := filepath.WalkDir(dir, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == dir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		if mode.Type()&^(fs.ModeDir|fs.ModeSymlink) != 0 || mode&fs.ModeSymlink != 0 {
			return fmt.Errorf("unsupported artifact entry: %s", name)
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		return writeTarFileBody(tw, name)
	})
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = w.CloseWithError(err)
		return err
	}
	return w.Close()
}

func writeTarFileBody(tw *tar.Writer, name string) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(tw, file)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
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
