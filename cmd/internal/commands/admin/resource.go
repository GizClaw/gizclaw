package admincmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GizClaw/gizclaw-go/cmd/internal/connection"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
)

type resourceClient interface {
	ApplyResource(context.Context, apitypes.Resource) (apitypes.ApplyResult, error)
	DeleteResource(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)
	GetResource(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)
	Close() error
}

type adminResourceAPI interface {
	ApplyResourceWithResponse(ctx context.Context, body adminservice.ApplyResourceJSONRequestBody, reqEditors ...adminservice.RequestEditorFn) (*adminservice.ApplyResourceResponse, error)
	DeleteResourceWithResponse(ctx context.Context, kind adminservice.ResourceKind, name string, reqEditors ...adminservice.RequestEditorFn) (*adminservice.DeleteResourceResponse, error)
	GetResourceWithResponse(ctx context.Context, kind adminservice.ResourceKind, name string, reqEditors ...adminservice.RequestEditorFn) (*adminservice.GetResourceResponse, error)
}

type resourceClientBridge struct {
	api   adminResourceAPI
	close func() error
}

func (r *resourceClientBridge) ApplyResource(ctx context.Context, resource apitypes.Resource) (apitypes.ApplyResult, error) {
	resp, err := r.api.ApplyResourceWithResponse(ctx, resource)
	if err != nil {
		return apitypes.ApplyResult{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.ApplyResult{}, resourceResponseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON409, resp.JSON500, resp.JSON501)
}

func (r *resourceClientBridge) DeleteResource(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
	resp, err := r.api.DeleteResourceWithResponse(ctx, kind, string(name))
	if err != nil {
		return apitypes.Resource{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Resource{}, resourceResponseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON409, resp.JSON500)
}

func (r *resourceClientBridge) GetResource(ctx context.Context, kind apitypes.ResourceKind, name string) (apitypes.Resource, error) {
	resp, err := r.api.GetResourceWithResponse(ctx, kind, string(name))
	if err != nil {
		return apitypes.Resource{}, err
	}
	if resp.JSON200 != nil {
		return *resp.JSON200, nil
	}
	return apitypes.Resource{}, resourceResponseError(resp.StatusCode(), resp.Body, resp.JSON400, resp.JSON404, resp.JSON500, resp.JSON501)
}

func (r *resourceClientBridge) Close() error {
	if r.close == nil {
		return nil
	}
	return r.close()
}

var openResourceClient = func(ctxName string) (resourceClient, error) {
	c, err := connection.ConnectFromContext(ctxName)
	if err != nil {
		return nil, err
	}
	api, err := c.ServerAdminClient()
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return &resourceClientBridge{
		api:   api,
		close: c.Close,
	}, nil
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

func newShowCmd(ctxName *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <kind> <name>",
		Short: "Show a named admin resource",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, name, err := parseNamedResourceArgs(args)
			if err != nil {
				return err
			}
			c, err := openResourceClient(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			resource, err := c.GetResource(context.Background(), kind, name)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(resource)
		},
	}
	cmd.Flags().StringVar(ctxName, "context", "", "context name (default: current)")
	return cmd
}

func newDeleteCmd(ctxName *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <kind> <name>",
		Short: "Delete a named admin resource",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, name, err := parseNamedResourceArgs(args)
			if err != nil {
				return err
			}
			c, err := openResourceClient(*ctxName)
			if err != nil {
				return err
			}
			defer c.Close()
			resource, err := c.DeleteResource(context.Background(), kind, name)
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
	var reader io.Reader
	if path == "-" {
		reader = cmd.InOrStdin()
	} else {
		file, err := os.Open(path)
		if err != nil {
			return apitypes.Resource{}, err
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return apitypes.Resource{}, err
	}
	return decodeResourceData(path, data)
}

func decodeResourceData(path string, data []byte) (apitypes.Resource, error) {
	switch resourceFileFormat(path) {
	case "json":
		var err error
		data, err = expandResourceEnv(data)
		if err != nil {
			return apitypes.Resource{}, err
		}
		return decodeJSONResource(data)
	case "yaml":
		return decodeYAMLResource(data)
	default:
		return apitypes.Resource{}, fmt.Errorf("unsupported resource file extension %q; use .json, .yaml, or .yml", filepath.Ext(path))
	}
}

func resourceFileFormat(path string) string {
	if path == "-" {
		return "json"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}

func decodeJSONResource(data []byte) (apitypes.Resource, error) {
	var resource apitypes.Resource
	if err := json.Unmarshal(data, &resource); err != nil {
		return apitypes.Resource{}, err
	}
	if err := validateResourceKind(data); err != nil {
		return apitypes.Resource{}, err
	}
	return resource, nil
}

func validateResourceKind(data []byte) error {
	var header struct {
		Kind apitypes.ResourceKind `json:"kind"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	if !header.Kind.Valid() {
		return fmt.Errorf("unknown resource kind %q", header.Kind)
	}
	return nil
}

func decodeYAMLResource(data []byte) (apitypes.Resource, error) {
	var value any
	if err := yaml.Unmarshal(data, &value); err != nil {
		return apitypes.Resource{}, err
	}
	expanded, err := expandResourceYAMLValue(value)
	if err != nil {
		return apitypes.Resource{}, err
	}
	jsonData, err := json.Marshal(expanded)
	if err != nil {
		return apitypes.Resource{}, err
	}
	return decodeJSONResource(jsonData)
}

func expandResourceYAMLValue(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			expanded, err := expandResourceYAMLValue(item)
			if err != nil {
				return nil, err
			}
			out[key] = expanded
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("resource YAML map key must be a string, got %T", key)
			}
			expanded, err := expandResourceYAMLValue(item)
			if err != nil {
				return nil, err
			}
			out[name] = expanded
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			expanded, err := expandResourceYAMLValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = expanded
		}
		return out, nil
	case string:
		return expandResourceEnvString(v)
	default:
		return value, nil
	}
}

var resourceEnvPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

func expandResourceEnv(data []byte) ([]byte, error) {
	input := string(data)
	expanded, err := expandResourceEnvWith(input, func(replacement string, offset int) string {
		if insideJSONString(input, offset) {
			return escapeJSONStringFragment(replacement)
		}
		return replacement
	})
	if err != nil {
		return nil, err
	}
	return []byte(expanded), nil
}

func expandResourceEnvString(input string) (string, error) {
	return expandResourceEnvWith(input, func(replacement string, _ int) string {
		return replacement
	})
}

func expandResourceEnvWith(input string, formatReplacement func(string, int) string) (string, error) {
	matches := resourceEnvPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return input, nil
	}
	var firstErr error
	var expanded strings.Builder
	last := 0
	for _, match := range matches {
		if firstErr != nil {
			break
		}
		expanded.WriteString(input[last:match[0]])
		name := input[match[2]:match[3]]
		replacement := ""
		if value, ok := os.LookupEnv(name); ok && value != "" {
			replacement = value
		} else if match[4] != -1 {
			replacement = input[match[6]:match[7]]
		} else {
			firstErr = fmt.Errorf("environment variable %s is required", name)
			break
		}
		expanded.WriteString(formatReplacement(replacement, match[0]))
		last = match[1]
	}
	if firstErr != nil {
		return "", firstErr
	}
	expanded.WriteString(input[last:])
	return expanded.String(), nil
}

func insideJSONString(input string, offset int) bool {
	inString := false
	escaped := false
	for i := 0; i < offset; i++ {
		switch input[i] {
		case '\\':
			if escaped {
				escaped = false
			} else {
				escaped = true
			}
		case '"':
			if !escaped {
				inString = !inString
			}
			escaped = false
		default:
			escaped = false
		}
	}
	return inString
}

func escapeJSONStringFragment(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	quoted := string(data)
	return quoted[1 : len(quoted)-1]
}

func parseNamedResourceArgs(args []string) (apitypes.ResourceKind, string, error) {
	kind := apitypes.ResourceKind(args[0])
	if !kind.Valid() {
		return "", "", fmt.Errorf("unknown resource kind %q", args[0])
	}
	if kind == apitypes.ResourceKindResourceList {
		return "", "", fmt.Errorf("resource kind %q cannot be addressed by name", kind)
	}
	name := strings.TrimSpace(args[1])
	if name == "" {
		return "", "", fmt.Errorf("resource name is required")
	}
	return kind, name, nil
}

func resourceResponseError(status int, body []byte, errs ...interface{}) error {
	for _, errResp := range errs {
		switch e := errResp.(type) {
		case *apitypes.ErrorResponse:
			if e != nil {
				return fmt.Errorf("%s: %s", e.Error.Code, e.Error.Message)
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if text != "" {
		return fmt.Errorf("unexpected status %d: %s", status, text)
	}
	if status != 0 {
		return fmt.Errorf("unexpected status %d", status)
	}
	return fmt.Errorf("unexpected empty response")
}
