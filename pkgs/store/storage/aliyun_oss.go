package storage

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type aliyunOSSResource struct {
	bucket *oss.Bucket
	client *http.Client
}

// AliyunOSS returns the named physical Alibaba Cloud OSS bucket client.
func (s *Storage) AliyunOSS(name string) (*oss.Bucket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	resource, ok := s.aliyunOSS[name]
	if !ok {
		return nil, fmt.Errorf("storage: aliyun-oss %q not found", name)
	}
	return resource.bucket, nil
}

func newAliyunOSS(name string, cfg AliyunOSSConfig) (aliyunOSSResource, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	bucketName := strings.TrimSpace(cfg.Bucket)
	accessKeyID := strings.TrimSpace(cfg.AccessKeyID)
	accessKeySecret := strings.TrimSpace(cfg.AccessKeySecret)
	for _, field := range []struct{ name, value string }{
		{"endpoint", endpoint}, {"bucket", bucketName},
		{"access_key_id", accessKeyID}, {"access_key_secret", accessKeySecret},
	} {
		if field.value == "" {
			return aliyunOSSResource{}, fmt.Errorf("storage: aliyun-oss %q requires %s", name, field.name)
		}
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return aliyunOSSResource{}, fmt.Errorf("storage: aliyun-oss %q endpoint must be an HTTPS URL", name)
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	options := []oss.ClientOption{oss.HTTPClient(httpClient), oss.Timeout(30, 30)}
	if token := strings.TrimSpace(cfg.SecurityToken); token != "" {
		options = append(options, oss.SecurityToken(token))
	}
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret, options...)
	if err != nil {
		return aliyunOSSResource{}, fmt.Errorf("storage: aliyun-oss %q create client: %w", name, err)
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		httpClient.CloseIdleConnections()
		return aliyunOSSResource{}, fmt.Errorf("storage: aliyun-oss %q bucket: %w", name, err)
	}
	if _, err := client.GetBucketInfo(bucketName); err != nil {
		httpClient.CloseIdleConnections()
		return aliyunOSSResource{}, &externalOperationError{operation: fmt.Sprintf("storage: aliyun-oss %q readiness", name), err: err}
	}
	return aliyunOSSResource{bucket: bucket, client: httpClient}, nil
}
