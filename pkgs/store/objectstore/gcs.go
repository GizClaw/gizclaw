package objectstore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"

	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
)

type gcsBackend struct{ bucket *gcs.BucketHandle }

// NewGCS creates an ObjectStore that borrows a Google Cloud Storage client.
func NewGCS(client *gcs.Client, bucket string) (ObjectStore, error) {
	if client == nil {
		return nil, errors.New("objectstore: gcs client is nil")
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, errors.New("objectstore: gcs bucket is empty")
	}
	return newCloudStore("gcs", &gcsBackend{bucket: client.Bucket(bucket)})
}

func (b *gcsBackend) get(ctx context.Context, name string) (io.ReadCloser, cloudObjectAttrs, error) {
	object := b.bucket.Object(name)
	attrs, err := object.Attrs(ctx)
	if err != nil {
		return nil, cloudObjectAttrs{}, gcsObjectError(err)
	}
	reader, err := object.NewReader(ctx)
	if err != nil {
		return nil, cloudObjectAttrs{}, gcsObjectError(err)
	}
	return reader, cloudObjectAttrs{name: name, size: attrs.Size, metadata: attrs.Metadata}, nil
}

func (b *gcsBackend) put(ctx context.Context, name string, reader io.Reader, metadata map[string]string) error {
	writer := b.bucket.Object(name).NewWriter(ctx)
	writer.Metadata = metadata
	if _, err := io.Copy(writer, reader); err != nil {
		_ = writer.CloseWithError(err)
		return gcsObjectError(err)
	}
	return gcsObjectError(writer.Close())
}

func (b *gcsBackend) delete(ctx context.Context, name string) error {
	return gcsObjectError(b.bucket.Object(name).Delete(ctx))
}

func (b *gcsBackend) list(ctx context.Context, prefix string) ([]cloudObjectAttrs, error) {
	it := b.bucket.Objects(ctx, &gcs.Query{Prefix: prefix})
	var out []cloudObjectAttrs
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, gcsObjectError(err)
		}
		out = append(out, cloudObjectAttrs{name: attrs.Name, size: attrs.Size, metadata: attrs.Metadata})
	}
}

func gcsObjectError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gcs.ErrObjectNotExist) {
		return fs.ErrNotExist
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code == 404 {
			return fs.ErrNotExist
		}
		return errors.New("gcs request failed")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New("gcs request failed")
}

var _ cloudBackend = (*gcsBackend)(nil)
