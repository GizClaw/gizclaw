package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/volcengine/volc-sdk-golang/base"
)

const volcMemoryAPIVersion = "2025-10-10"

// VolcConfig configures Volcengine AgentKit/Viking MEM0. The control plane
// resolves a Mem0 API key; fact traffic then uses the Mem0 HTTP protocol.
type VolcConfig struct {
	Mem0            Mem0Config `yaml:"mem0"`
	APIKeyID        string     `yaml:"api_key_id"`
	MemoryProjectID string     `yaml:"memory_project_id"`
	ControlEndpoint string     `yaml:"control_endpoint"`
	Region          string     `yaml:"region"`
	AccessKeyID     string     `yaml:"access_key_id"`
	AccessKeySecret string     `yaml:"access_key_secret"`
}

// VolcCredentialResolver resolves a Volc memory project's Mem0 API key.
type VolcCredentialResolver interface {
	ResolveMem0APIKey(ctx context.Context, config VolcConfig) (string, error)
}

// VolcStore is a Volcengine credential adapter over the shared Mem0 data plane.
type VolcStore struct {
	*Mem0Store
}

// OpenVolcStore resolves control-plane credentials when needed and constructs
// the Mem0 data-plane adapter.
func OpenVolcStore(ctx context.Context, config VolcConfig, resolver VolcCredentialResolver, client HTTPClient) (*VolcStore, error) {
	if strings.TrimSpace(config.Mem0.Endpoint) == "" {
		return nil, fmt.Errorf("%w: volc memory mem0 endpoint is required", ErrInvalidInput)
	}
	if config.Mem0.Flavor == "" {
		config.Mem0.Flavor = Mem0Platform
	}
	if config.Mem0.APIKey == "" {
		if resolver == nil {
			var err error
			resolver, err = newVolcCredentialClient(config)
			if err != nil {
				return nil, err
			}
		}
		key, err := resolver.ResolveMem0APIKey(ctx, config)
		if err != nil {
			return nil, err
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%w: volc memory API key is empty", ErrUnavailable)
		}
		config.Mem0.APIKey = key
	}
	store, err := NewMem0Store(config.Mem0, client)
	if err != nil {
		return nil, err
	}
	return &VolcStore{Mem0Store: store}, nil
}

type volcCredentialClient struct {
	client *base.Client
}

func newVolcCredentialClient(config VolcConfig) (*volcCredentialClient, error) {
	if config.AccessKeyID == "" || config.AccessKeySecret == "" {
		return nil, fmt.Errorf("%w: volc access_key_id and access_key_secret are required", ErrInvalidInput)
	}
	host := config.ControlEndpoint
	if host == "" {
		host = "open.volcengineapi.com"
	}
	scheme := "https"
	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("%w: volc control_endpoint must contain only scheme and host", ErrInvalidInput)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, fmt.Errorf("%w: volc control_endpoint scheme must be http or https", ErrInvalidInput)
		}
		scheme, host = parsed.Scheme, parsed.Host
	}
	region := config.Region
	if region == "" {
		region = "cn-beijing"
	}
	serviceInfo := &base.ServiceInfo{
		Timeout: 30 * time.Second,
		Scheme:  scheme,
		Host:    host,
		Header:  http.Header{"Accept": []string{"application/json"}},
		Credentials: base.Credentials{
			AccessKeyID: config.AccessKeyID, SecretAccessKey: config.AccessKeySecret,
			Service: "mem0", Region: region,
		},
	}
	apiInfo := func(action string) *base.ApiInfo {
		return &base.ApiInfo{Method: http.MethodPost, Path: "/", Query: url.Values{"Action": []string{action}, "Version": []string{volcMemoryAPIVersion}}, Header: http.Header{"Content-Type": []string{"application/json"}}}
	}
	client := base.NewClient(serviceInfo, map[string]*base.ApiInfo{
		"DescribeMemoryProjectDetail": apiInfo("DescribeMemoryProjectDetail"),
		"DescribeAPIKeyDetail":        apiInfo("DescribeAPIKeyDetail"),
	})
	// base.NewClient may read process-level credentials; this provider's
	// explicit config remains authoritative.
	client.SetCredential(serviceInfo.Credentials)
	return &volcCredentialClient{client: client}, nil
}

func (c *volcCredentialClient) ResolveMem0APIKey(ctx context.Context, config VolcConfig) (string, error) {
	projectID := strings.TrimSpace(config.MemoryProjectID)
	if projectID == "" {
		return "", fmt.Errorf("%w: volc memory_project_id is required", ErrInvalidInput)
	}
	apiKeyID := strings.TrimSpace(config.APIKeyID)
	if apiKeyID == "" {
		body, _ := json.Marshal(map[string]string{"MemoryProjectId": projectID})
		raw, status, err := c.client.CtxJson(ctx, "DescribeMemoryProjectDetail", nil, string(body))
		if err != nil {
			return "", mapVolcControlError("describe memory project", status, err)
		}
		var response struct {
			ResponseMetadata volcResponseMetadata `json:"ResponseMetadata"`
			Result           struct {
				APIKeyInfos []struct {
					APIKeyID string `json:"APIKeyId"`
				} `json:"APIKeyInfos"`
			} `json:"Result"`
		}
		if err := json.Unmarshal(raw, &response); err != nil {
			return "", fmt.Errorf("%w: decode volc memory project response", ErrUnavailable)
		}
		if err := response.ResponseMetadata.err(); err != nil {
			return "", err
		}
		for _, info := range response.Result.APIKeyInfos {
			if strings.TrimSpace(info.APIKeyID) != "" {
				apiKeyID = info.APIKeyID
				break
			}
		}
		if apiKeyID == "" {
			return "", fmt.Errorf("%w: volc memory project has no API key", ErrNotFound)
		}
	}
	body, _ := json.Marshal(map[string]string{"MemoryProjectId": projectID, "APIKeyId": apiKeyID})
	raw, status, err := c.client.CtxJson(ctx, "DescribeAPIKeyDetail", nil, string(body))
	if err != nil {
		return "", mapVolcControlError("describe API key", status, err)
	}
	var response struct {
		ResponseMetadata volcResponseMetadata `json:"ResponseMetadata"`
		Result           struct {
			APIKeyValue string `json:"APIKeyValue"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", fmt.Errorf("%w: decode volc API key response", ErrUnavailable)
	}
	if err := response.ResponseMetadata.err(); err != nil {
		return "", err
	}
	return response.Result.APIKeyValue, nil
}

type volcResponseMetadata struct {
	Error *struct {
		Code    string `json:"Code"`
		Message string `json:"Message"`
	} `json:"Error"`
}

func (m volcResponseMetadata) err() error {
	if m.Error == nil {
		return nil
	}
	code := strings.ToLower(m.Error.Code)
	switch {
	case strings.Contains(code, "notfound") || strings.Contains(code, "not_found"):
		return fmt.Errorf("%w: volc memory resource not found", ErrNotFound)
	case strings.Contains(code, "invalid"):
		return fmt.Errorf("%w: volc memory request is invalid", ErrInvalidInput)
	default:
		return fmt.Errorf("%w: volc memory control plane returned %s", ErrUnavailable, truncate(m.Error.Code, 128))
	}
}

func mapVolcControlError(operation string, status int, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if status == http.StatusNotFound {
		return fmt.Errorf("%w: volc %s", ErrNotFound, operation)
	}
	if status >= 400 && status < 500 {
		return fmt.Errorf("%w: volc %s", ErrInvalidInput, operation)
	}
	return fmt.Errorf("%w: volc %s", ErrUnavailable, operation)
}

var _ Store = (*VolcStore)(nil)
var _ OperationWaiter = (*VolcStore)(nil)
