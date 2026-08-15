package objectstore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type aliyunOSSBackend struct{ bucket *oss.Bucket }

// NewAliyunOSS creates an ObjectStore that borrows an Alibaba Cloud OSS bucket client.
func NewAliyunOSS(bucket *oss.Bucket) (ObjectStore, error) {
	if bucket == nil {
		return nil, errors.New("objectstore: aliyun-oss bucket is nil")
	}
	return newCloudStore("aliyun-oss", &aliyunOSSBackend{bucket: bucket})
}

func (b *aliyunOSSBackend) get(ctx context.Context, name string) (io.ReadCloser, cloudObjectAttrs, error) {
	attrs, err := b.attrs(ctx, name)
	if err != nil {
		return nil, cloudObjectAttrs{}, err
	}
	body, err := b.bucket.GetObject(name)
	if err != nil {
		return nil, cloudObjectAttrs{}, aliyunOSSError(ctx, err)
	}
	return body, attrs, nil
}

func (b *aliyunOSSBackend) put(ctx context.Context, name string, reader io.Reader, metadata map[string]string) error {
	options := make([]oss.Option, 0, len(metadata))
	for key, value := range metadata {
		options = append(options, oss.Meta(key, value))
	}
	return aliyunOSSError(ctx, b.bucket.PutObject(name, reader, options...))
}

func (b *aliyunOSSBackend) delete(ctx context.Context, name string) error {
	return aliyunOSSError(ctx, b.bucket.DeleteObject(name))
}

func (b *aliyunOSSBackend) list(ctx context.Context, prefix string) ([]cloudObjectAttrs, error) {
	options := []oss.Option{oss.Prefix(prefix), oss.MaxKeys(1000)}
	var out []cloudObjectAttrs
	for {
		page, err := b.bucket.ListObjectsV2(options...)
		if err != nil {
			return nil, aliyunOSSError(ctx, err)
		}
		for _, object := range page.Objects {
			attrs, err := b.attrs(ctx, object.Key)
			if err != nil {
				return nil, err
			}
			attrs.size = object.Size
			out = append(out, attrs)
		}
		if !page.IsTruncated {
			return out, nil
		}
		if page.NextContinuationToken == "" {
			return nil, errors.New("aliyun-oss pagination did not advance")
		}
		options = []oss.Option{oss.Prefix(prefix), oss.MaxKeys(1000), oss.ContinuationToken(page.NextContinuationToken)}
	}
}

func (b *aliyunOSSBackend) attrs(ctx context.Context, name string) (cloudObjectAttrs, error) {
	header, err := b.bucket.GetObjectDetailedMeta(name)
	if err != nil {
		return cloudObjectAttrs{}, aliyunOSSError(ctx, err)
	}
	size, err := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	if err != nil {
		return cloudObjectAttrs{}, errors.New("aliyun-oss returned invalid content length")
	}
	metadata := map[string]string{}
	for key, values := range header {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "x-oss-meta-") && len(values) != 0 {
			metadata[strings.TrimPrefix(lower, "x-oss-meta-")] = values[0]
		}
	}
	return cloudObjectAttrs{name: name, size: size, metadata: metadata}, nil
}

func aliyunOSSError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var serviceErr oss.ServiceError
	if errors.As(err, &serviceErr) {
		if serviceErr.StatusCode == http.StatusNotFound {
			return fs.ErrNotExist
		}
		return errors.New("aliyun-oss request failed")
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return errors.New("aliyun-oss request failed")
}

var _ cloudBackend = (*aliyunOSSBackend)(nil)
