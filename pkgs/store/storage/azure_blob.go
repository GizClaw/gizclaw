package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type azureBlobResource struct {
	client    *azblob.Client
	container string
	transport *http.Transport
}

// AzureBlob returns the named physical Azure Blob client and container.
func (s *Storage) AzureBlob(name string) (*azblob.Client, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", errors.New("storage: registry is closed")
	}
	resource, ok := s.azureBlob[name]
	if !ok {
		return nil, "", fmt.Errorf("storage: azure-blob %q not found", name)
	}
	return resource.client, resource.container, nil
}

func newAzureBlob(name string, cfg AzureBlobConfig) (azureBlobResource, error) {
	accountURL := strings.TrimSpace(cfg.AccountURL)
	container := strings.TrimSpace(cfg.Container)
	if accountURL == "" {
		return azureBlobResource{}, fmt.Errorf("storage: azure-blob %q requires account_url", name)
	}
	if container == "" {
		return azureBlobResource{}, fmt.Errorf("storage: azure-blob %q requires container", name)
	}
	parsed, err := url.Parse(accountURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return azureBlobResource{}, fmt.Errorf("storage: azure-blob %q account_url must be an HTTPS URL without query", name)
	}
	credential, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return azureBlobResource{}, errors.New("storage: azure-blob create default credential")
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, ForceAttemptHTTP2: true}
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaultTransport.Clone()
	}
	httpClient := &http.Client{Transport: transport}
	client, err := azblob.NewClient(accountURL, credential, &azblob.ClientOptions{ClientOptions: azcore.ClientOptions{
		Transport: httpClient,
		Retry:     policy.RetryOptions{MaxRetries: 2, TryTimeout: 30 * time.Second},
	}})
	if err != nil {
		transport.CloseIdleConnections()
		return azureBlobResource{}, fmt.Errorf("storage: azure-blob %q create client: %w", name, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := client.ServiceClient().NewContainerClient(container).GetProperties(ctx, nil); err != nil {
		transport.CloseIdleConnections()
		return azureBlobResource{}, &externalOperationError{operation: fmt.Sprintf("storage: azure-blob %q readiness", name), err: err}
	}
	return azureBlobResource{client: client, container: container, transport: transport}, nil
}
