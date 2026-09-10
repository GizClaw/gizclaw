package admincmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
	"github.com/spf13/cobra"
)

type resourceClient interface {
	ApplyResource(context.Context, apitypes.Resource) (apitypes.ApplyResult, error)
	DeleteResource(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)
	GetResource(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)
	Close() error
}

var openResourceClient = func(ctxName string) (resourceClient, error) {
	c, err := adminresource.Connect(contextconn.Options{Context: ctxName})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func newApplyCmd(ctxName *string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "apply -f <file>",
		Short: "Apply an admin resource",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(file) == "" {
				return fmt.Errorf("required flag: --file")
			}
			resource, err := readResourceFile(cmd, file)
			if err != nil {
				return err
			}
			c, err := openResourceClient(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := c.ApplyResource(context.Background(), resource)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "resource JSON/YAML file, or '-' for JSON stdin")
	cmd.Flags().StringVar(ctxName, "context", "", "context name (default: current)")
	return cmd
}

func newDeleteCmd(ctxName *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <kind> <id>",
		Short: "Delete an admin resource by canonical ID",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, id, err := parseResourceIDArgs(args)
			if err != nil {
				return err
			}
			c, err := openResourceClient(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			resource, err := c.DeleteResource(context.Background(), kind, id)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(resource)
		},
	}
	cmd.Flags().StringVar(ctxName, "context", "", "context name (default: current)")
	return cmd
}

func readResourceFile(cmd *cobra.Command, path string) (apitypes.Resource, error) {
	data, err := readResourceData(cmd, path)
	if err != nil {
		return apitypes.Resource{}, err
	}
	return decodeResourceData(path, data)
}

func readResourceData(cmd *cobra.Command, path string) ([]byte, error) {
	var reader io.Reader
	if path == "-" {
		reader = cmd.InOrStdin()
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func decodeResourceData(path string, data []byte) (apitypes.Resource, error) {
	prepared, err := prepareResourceData(path, data)
	if err != nil {
		return apitypes.Resource{}, err
	}
	return adminresource.DecodePrepared(prepared)
}

func prepareResourceData(path string, data []byte) ([]byte, error) {
	format, err := adminresource.FormatForPath(path)
	if err != nil {
		return nil, err
	}
	return adminresource.PrepareManifest(format, data)
}

func parseResourceIDArgs(args []string) (apitypes.ResourceKind, string, error) {
	ref, err := adminresource.ParseReference(args[0], args[1])
	if err != nil {
		return "", "", err
	}
	return ref.Kind, ref.ID, nil
}
