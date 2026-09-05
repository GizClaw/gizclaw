package admincmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/spf13/cobra"
)

const showConcurrency = 8

type resourceReference struct {
	Kind apitypes.ResourceKind `json:"kind"`
	ID   string                `json:"id"`
}

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

func readResourceReferences(cmd *cobra.Command, file string) ([]resourceReference, error) {
	data, err := readResourceData(cmd, file)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var refs []resourceReference
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

	errs := make([]error, len(refs))
	var workers sync.WaitGroup
	for worker := range min(showConcurrency, len(refs)) {
		workers.Go(func() {
			// Each worker owns disjoint indices; output is written only after Wait.
			for i := worker; i < len(refs); i += showConcurrency {
				ref := refs[i]
				requestErr := cmd.Context().Err()
				if requestErr == nil {
					var resource apitypes.Resource
					resource, requestErr = c.GetResource(cmd.Context(), ref.Kind, ref.ID)
					if requestErr == nil {
						results[i] = &resource
					}
				}
				if requestErr != nil {
					errs[i] = fmt.Errorf("resource [%d] %s/%s: %w", i, ref.Kind, ref.ID, requestErr)
				}
			}
		})
	}
	workers.Wait()
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(results); err != nil {
		return err
	}
	// Cobra reports these indexed errors on stderr and the executable exits non-zero.
	return errors.Join(errs...)
}
