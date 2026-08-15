package objectstore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

type tosBackend struct {
	client *tos.ClientV2
	bucket string
}

// NewVolcTOS creates an ObjectStore that borrows a Volcengine TOS client.
func NewVolcTOS(client *tos.ClientV2, bucket string) (ObjectStore, error) {
	if client == nil {
		return nil, errors.New("objectstore: volc-tos client is nil")
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, errors.New("objectstore: volc-tos bucket is empty")
	}
	return newCloudStore("volc-tos", &tosBackend{client: client, bucket: bucket})
}

func (b *tosBackend) get(ctx context.Context, name string) (io.ReadCloser, cloudObjectAttrs, error) {
	out, err := b.client.GetObjectV2(ctx, &tos.GetObjectV2Input{Bucket: b.bucket, Key: name})
	if err != nil {
		return nil, cloudObjectAttrs{}, tosObjectError(err)
	}
	return out.Content, cloudObjectAttrs{name: name, size: out.ContentLength, metadata: tosMetadata(out.Meta)}, nil
}

func (b *tosBackend) put(ctx context.Context, name string, reader io.Reader, metadata map[string]string) error {
	_, err := b.client.PutObjectV2(ctx, &tos.PutObjectV2Input{
		PutObjectBasicInput: tos.PutObjectBasicInput{Bucket: b.bucket, Key: name, Meta: metadata},
		Content:             reader,
	})
	return tosObjectError(err)
}

func (b *tosBackend) delete(ctx context.Context, name string) error {
	_, err := b.client.DeleteObjectV2(ctx, &tos.DeleteObjectV2Input{Bucket: b.bucket, Key: name})
	return tosObjectError(err)
}

func (b *tosBackend) list(ctx context.Context, prefix string) ([]cloudObjectAttrs, error) {
	input := &tos.ListObjectsType2Input{Bucket: b.bucket, Prefix: prefix, MaxKeys: 1000, FetchMeta: true, ListOnlyOnce: true}
	var out []cloudObjectAttrs
	for {
		page, err := b.client.ListObjectsType2(ctx, input)
		if err != nil {
			return nil, tosObjectError(err)
		}
		for _, object := range page.Contents {
			out = append(out, cloudObjectAttrs{name: object.Key, size: object.Size, metadata: tosMetadata(object.Meta)})
		}
		if !page.IsTruncated {
			return out, nil
		}
		if page.NextContinuationToken == "" || page.NextContinuationToken == input.ContinuationToken {
			return nil, errors.New("volc-tos pagination did not advance")
		}
		input.ContinuationToken = page.NextContinuationToken
	}
}

func tosMetadata(metadata tos.Metadata) map[string]string {
	out := map[string]string{}
	if metadata != nil {
		metadata.Range(func(key, value string) bool {
			out[strings.ToLower(key)] = value
			return true
		})
	}
	return out
}

func tosObjectError(err error) error {
	if err == nil {
		return nil
	}
	var serverErr *tos.TosServerError
	if errors.As(err, &serverErr) {
		if serverErr.StatusCode == http.StatusNotFound {
			return fs.ErrNotExist
		}
		return errors.New("volc-tos request failed")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New("volc-tos request failed")
}

var _ cloudBackend = (*tosBackend)(nil)
