package admincmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
	"github.com/spf13/cobra"
)

func newShowCmd(ctxName *string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "show <kind> <id> | show -f <file>",
		Short: "Show one resource or a JSON array of resource references",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("file") {
				if file == "" {
					return fmt.Errorf("--file must not be empty")
				}
				return cobra.NoArgs(cmd, args)
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("file") {
				cmd.SilenceUsage = true
				return showResourceBatch(cmd, *ctxName, file)
			}
			kind, id, err := parseResourceIDArgs(args)
			if err != nil {
				return err
			}
			c, err := openResourceClient(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			resource, err := c.GetResource(cmd.Context(), kind, id)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(resource)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "JSON array of {kind,id} references, or '-' for stdin")
	cmd.Flags().StringVar(ctxName, "context", "", "context name (default: current)")
	return cmd
}

func readResourceReferences(cmd *cobra.Command, file string) ([]adminresource.Reference, error) {
	data, err := readResourceData(cmd, file)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var refs []adminresource.Reference
	if err := decoder.Decode(&refs); err != nil {
		return nil, fmt.Errorf("decode resource references: %w", err)
	}
	if refs == nil {
		return nil, fmt.Errorf("resource references must be a JSON array")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("resource references must contain exactly one JSON array")
	}
	for i, ref := range refs {
		if _, _, err := parseResourceIDArgs([]string{string(ref.Kind), ref.ID}); err != nil {
			return nil, fmt.Errorf("resource reference [%d]: %w", i, err)
		}
	}
	return refs, nil
}

func showResourceBatch(cmd *cobra.Command, ctxName, file string) error {
	refs, err := readResourceReferences(cmd, file)
	if err != nil {
		return err
	}
	results := make([]*apitypes.Resource, len(refs))
	if len(refs) == 0 {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(results)
	}
	if err := cmd.Context().Err(); err != nil {
		return err
	}
	c, err := openResourceClient(ctxName)
	if err != nil {
		return err
	}
	defer c.Close()

	var errs []error
	for i, result := range adminresource.GetResources(cmd.Context(), c, refs) {
		results[i] = result.Resource
		if result.Err != nil {
			errs = append(errs, fmt.Errorf("resource [%d] %s/%s: %w", i, refs[i].Kind, refs[i].ID, result.Err))
		}
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(results); err != nil {
		return err
	}
	// Cobra reports these indexed errors on stderr and the executable exits non-zero.
	return errors.Join(errs...)
}
