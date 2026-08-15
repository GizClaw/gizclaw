package objectstore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type azureBlobBackend struct {
	client    *azblob.Client
	container string
}

// NewAzureBlob creates an ObjectStore that borrows an Azure Blob client.
func NewAzureBlob(client *azblob.Client, container string) (ObjectStore, error) {
	if client == nil {
		return nil, errors.New("objectstore: azure-blob client is nil")
	}
	container = strings.TrimSpace(container)
	if container == "" {
		return nil, errors.New("objectstore: azure-blob container is empty")
	}
	return newCloudStore("azure-blob", &azureBlobBackend{client: client, container: container})
}

func (b *azureBlobBackend) get(ctx context.Context, name string) (io.ReadCloser, cloudObjectAttrs, error) {
	response, err := b.client.DownloadStream(ctx, b.container, name, nil)
	if err != nil {
		return nil, cloudObjectAttrs{}, azureBlobError(err)
	}
	size := int64(0)
	if response.ContentLength != nil {
		size = *response.ContentLength
	}
	return response.Body, cloudObjectAttrs{name: name, size: size, metadata: azureMetadata(response.Metadata)}, nil
}

func (b *azureBlobBackend) put(ctx context.Context, name string, reader io.Reader, metadata map[string]string) error {
	values := make(map[string]*string, len(metadata))
	for key, value := range metadata {
		copy := value
		values[key] = &copy
	}
	_, err := b.client.UploadStream(ctx, b.container, name, reader, &azblob.UploadStreamOptions{
		Concurrency: 1,
		Metadata:    values,
	})
	return azureBlobError(err)
}

func (b *azureBlobBackend) delete(ctx context.Context, name string) error {
	_, err := b.client.DeleteBlob(ctx, b.container, name, nil)
	return azureBlobError(err)
}

func (b *azureBlobBackend) list(ctx context.Context, prefix string) ([]cloudObjectAttrs, error) {
	options := &azblob.ListBlobsFlatOptions{Prefix: &prefix, Include: azblob.ListBlobsInclude{Metadata: true}}
	pager := b.client.NewListBlobsFlatPager(b.container, options)
	var out []cloudObjectAttrs
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, azureBlobError(err)
		}
		if page.Segment == nil {
			continue
		}
		for _, item := range page.Segment.BlobItems {
			if item == nil || item.Name == nil || item.Properties == nil {
				return nil, errors.New("azure-blob returned incomplete list item")
			}
			size := int64(0)
			if item.Properties.ContentLength != nil {
				size = *item.Properties.ContentLength
			}
			out = append(out, cloudObjectAttrs{name: *item.Name, size: size, metadata: azureMetadata(item.Metadata)})
		}
	}
	return out, nil
}

func azureMetadata(values map[string]*string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		if value != nil {
			out[strings.ToLower(key)] = *value
		}
	}
	return out
}

func azureBlobError(err error) error {
	if err == nil {
		return nil
	}
	var responseErr *azcore.ResponseError
	if errors.As(err, &responseErr) {
		if responseErr.StatusCode == http.StatusNotFound {
			return fs.ErrNotExist
		}
		return errors.New("azure-blob request failed")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New("azure-blob request failed")
}

var _ cloudBackend = (*azureBlobBackend)(nil)
