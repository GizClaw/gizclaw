package admincmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

type concreteResourceValidationSummary struct {
	Valid bool                  `json:"valid"`
	Kind  apitypes.ResourceKind `json:"kind"`
	ID    string                `json:"id"`
}

type resourceListValidationSummary struct {
	Valid bool                  `json:"valid"`
	Kind  apitypes.ResourceKind `json:"kind"`
	Items int                   `json:"items"`
}

func newValidateCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:          "validate -f <file>",
		Short:        "Validate an admin resource offline",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(file) == "" {
				return errors.New("required flag: --file")
			}
			data, err := readResourceData(cmd, file)
			if err != nil {
				return err
			}
			prepared, err := prepareResourceData(file, data)
			if err != nil {
				return safeResourceInputError(file, err)
			}
			if err := apitypes.ValidateResourceJSON(prepared); err != nil {
				return fmt.Errorf("%s: %w", resourceInputName(file), err)
			}
			resource, err := decodePreparedResource(prepared)
			if err != nil {
				return fmt.Errorf("%s: validated resource could not be decoded", resourceInputName(file))
			}
			if _, err := resource.ValueByDiscriminator(); err != nil {
				return fmt.Errorf("%s: validated resource kind could not be decoded", resourceInputName(file))
			}
			return writeResourceValidationSummary(cmd, resource)
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "resource JSON/YAML file, or '-' for JSON stdin")
	return cmd
}

func writeResourceValidationSummary(cmd *cobra.Command, resource apitypes.Resource) error {
	kind, id, err := resourceKindAndID(resource)
	if err != nil {
		return errors.New("validated resource summary could not be encoded")
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	if kind != apitypes.ResourceKindResourceList {
		return encoder.Encode(concreteResourceValidationSummary{Valid: true, Kind: kind, ID: id})
	}
	list, err := resource.AsResourceListResource()
	if err != nil {
		return errors.New("validated ResourceList summary could not be encoded")
	}
	return encoder.Encode(resourceListValidationSummary{Valid: true, Kind: kind, Items: len(list.Spec.Items)})
}

func resourceKindAndID(resource apitypes.Resource) (apitypes.ResourceKind, string, error) {
	var header struct {
		Kind     apitypes.ResourceKind `json:"kind"`
		Metadata struct {
			ID string `json:"id"`
		} `json:"metadata"`
	}
	data, err := json.Marshal(resource)
	if err != nil {
		return "", "", err
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return "", "", err
	}
	return header.Kind, header.Metadata.ID, nil
}

func safeResourceInputError(path string, err error) error {
	input := resourceInputName(path)
	var formatError *unsupportedResourceFormatError
	if errors.As(err, &formatError) {
		return fmt.Errorf("%s: %w", input, formatError)
	}
	var envError *missingResourceEnvError
	if errors.As(err, &envError) {
		return fmt.Errorf("%s: environment variable %s is required", input, envError.name)
	}
	var kindError *unknownResourceKindError
	if errors.As(err, &kindError) {
		return fmt.Errorf("%s: /kind [discriminator]: invalid resource kind", input)
	}
	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		return fmt.Errorf("%s: invalid JSON at byte %d", input, syntaxError.Offset)
	}
	var yamlError yaml.Error
	if errors.As(err, &yamlError) {
		if token := yamlError.GetToken(); token != nil && token.Position != nil {
			return fmt.Errorf("%s: invalid YAML at line %d, column %d", input, token.Position.Line, token.Position.Column)
		}
		return fmt.Errorf("%s: invalid YAML", input)
	}
	return fmt.Errorf("%s: invalid resource input", input)
}

func resourceInputName(path string) string {
	if path == "-" {
		return "<stdin>"
	}
	return path
}
