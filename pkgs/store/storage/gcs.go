package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	gcs "cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

type gcsResource struct {
	client *gcs.Client
	bucket string
}

// GCS returns the named physical Google Cloud Storage client and bucket.
func (s *Storage) GCS(name string) (*gcs.Client, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", errors.New("storage: registry is closed")
	}
	resource, ok := s.gcs[name]
	if !ok {
		return nil, "", fmt.Errorf("storage: gcs %q not found", name)
	}
	return resource.client, resource.bucket, nil
}

func newGCS(name string, cfg GCSConfig) (gcsResource, error) {
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return gcsResource{}, fmt.Errorf("storage: gcs %q requires bucket", name)
	}
	var options []option.ClientOption
	if file := strings.TrimSpace(cfg.CredentialsFile); file != "" {
		info, err := os.Stat(file)
		if err != nil || !info.Mode().IsRegular() {
			return gcsResource{}, fmt.Errorf("storage: gcs %q credentials_file must be a readable regular file", name)
		}
		options = append(options, option.WithCredentialsFile(file))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := gcs.NewClient(ctx, options...)
	if err != nil {
		return gcsResource{}, fmt.Errorf("storage: gcs %q create client: %w", name, err)
	}
	if _, err := client.Bucket(bucket).Attrs(ctx); err != nil {
		_ = client.Close()
		return gcsResource{}, &externalOperationError{operation: fmt.Sprintf("storage: gcs %q readiness", name), err: err}
	}
	return gcsResource{client: client, bucket: bucket}, nil
}
