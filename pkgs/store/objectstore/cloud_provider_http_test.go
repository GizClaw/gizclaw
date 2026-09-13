package objectstore

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	gcs "cloud.google.com/go/storage"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"google.golang.org/api/option"
)

type providerRoundTripFunc func(*http.Request) (*http.Response, error)

func (f providerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func providerResponse(request *http.Request, status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestProviderSDKMissingErrorsAreMappedAndRedacted(t *testing.T) {
	const secret = "provider-secret-response"
	tests := map[string]func(*testing.T, *http.Client) ObjectStore{
		"volc-tos": func(t *testing.T, httpClient *http.Client) ObjectStore {
			client, err := tos.NewClientV2("https://tos.example.test",
				tos.WithCredentials(tos.NewStaticCredentials("id", "secret")),
				tos.WithRegion("cn-test"), tos.WithHTTPTransport(httpClient.Transport), tos.WithMaxRetryCount(0),
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			store, err := NewVolcTOS(client, "test-bucket")
			if err != nil {
				t.Fatal(err)
			}
			return store
		},
		"aliyun-oss": func(t *testing.T, httpClient *http.Client) ObjectStore {
			client, err := oss.New("https://oss.example.test", "id", "secret", oss.HTTPClient(httpClient), oss.AuthVersion(oss.AuthV2))
			if err != nil {
				t.Fatal(err)
			}
			bucket, err := client.Bucket("test-bucket")
			if err != nil {
				t.Fatal(err)
			}
			store, err := NewAliyunOSS(bucket)
			if err != nil {
				t.Fatal(err)
			}
			return store
		},
		"gcs": func(t *testing.T, httpClient *http.Client) ObjectStore {
			client, err := gcs.NewClient(context.Background(), option.WithHTTPClient(httpClient), option.WithEndpoint("https://storage.googleapis.test/storage/v1/"), option.WithoutAuthentication())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			store, err := NewGCS(client, "test-bucket")
			if err != nil {
				t.Fatal(err)
			}
			return store
		},
		"azure-blob": func(t *testing.T, httpClient *http.Client) ObjectStore {
			client, err := azblob.NewClientWithNoCredential("https://account.blob.example.test", &azblob.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: httpClient}})
			if err != nil {
				t.Fatal(err)
			}
			store, err := NewAzureBlob(client, "test-container")
			if err != nil {
				t.Fatal(err)
			}
			return store
		},
	}
	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			transport := providerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch name {
				case "volc-tos", "gcs":
					return providerResponse(request, http.StatusNotFound, "application/json", `{"Code":"NoSuchKey","error":{"code":404,"message":"`+secret+`"}}`), nil
				default:
					return providerResponse(request, http.StatusNotFound, "application/xml", `<Error><Code>BlobNotFound</Code><Message>`+secret+`</Message></Error>`), nil
				}
			})
			store := build(t, &http.Client{Transport: transport})
			_, err := store.Get("missing")
			if !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("Get missing error = %v", err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("Get missing leaked provider body: %v", err)
			}
		})
	}
}
