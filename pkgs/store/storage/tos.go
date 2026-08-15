package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
)

type volcTOSResource struct {
	client *tos.ClientV2
	bucket string
}

// VolcTOS returns the named physical Volcengine TOS client and bucket.
func (s *Storage) VolcTOS(name string) (*tos.ClientV2, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", errors.New("storage: registry is closed")
	}
	resource, ok := s.tos[name]
	if !ok {
		return nil, "", fmt.Errorf("storage: volc-tos %q not found", name)
	}
	return resource.client, resource.bucket, nil
}

func newVolcTOS(name string, cfg VolcTOSConfig) (volcTOSResource, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	region := strings.TrimSpace(cfg.Region)
	bucket := strings.TrimSpace(cfg.Bucket)
	accessKeyID := strings.TrimSpace(cfg.AccessKeyID)
	accessKeySecret := strings.TrimSpace(cfg.AccessKeySecret)
	for _, field := range []struct{ name, value string }{
		{"endpoint", endpoint}, {"region", region}, {"bucket", bucket},
		{"access_key_id", accessKeyID}, {"access_key_secret", accessKeySecret},
	} {
		if field.value == "" {
			return volcTOSResource{}, fmt.Errorf("storage: volc-tos %q requires %s", name, field.name)
		}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return volcTOSResource{}, fmt.Errorf("storage: volc-tos %q endpoint must be an HTTPS URL", name)
	}
	credentials := tos.NewStaticCredentials(accessKeyID, accessKeySecret)
	credentials.WithSecurityToken(strings.TrimSpace(cfg.SessionToken))
	client, err := tos.NewClientV2(endpoint,
		tos.WithCredentials(credentials),
		tos.WithRegion(region),
		tos.WithRequestTimeout(30*time.Second),
		tos.WithConnectionTimeout(30*time.Second),
		tos.WithSocketTimeout(30*time.Second, 30*time.Second),
		tos.WithMaxRetryCount(2),
	)
	if err != nil {
		return volcTOSResource{}, fmt.Errorf("storage: volc-tos %q create client: %w", name, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := client.HeadBucket(ctx, &tos.HeadBucketInput{Bucket: bucket}); err != nil {
		client.Close()
		return volcTOSResource{}, &externalOperationError{operation: fmt.Sprintf("storage: volc-tos %q readiness", name), err: err}
	}
	return volcTOSResource{client: client, bucket: bucket}, nil
}
