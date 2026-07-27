package credential

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	volcbase "github.com/volcengine/volc-sdk-golang/base"
)

type HTTPAuthConfig struct {
	Method     string
	Credential string
	Region     string
	Service    string
	Action     string
	Version    string
}

type HTTPAuthOptions struct {
	Now   func() time.Time
	Nonce func() (string, error)
}

func (s *Server) HTTPAuthorizer(ctx context.Context, config HTTPAuthConfig) (giztools.HTTPAuthorizer, error) {
	return s.httpAuthorizer(ctx, config, HTTPAuthOptions{})
}

func (s *Server) httpAuthorizer(ctx context.Context, config HTTPAuthConfig, options HTTPAuthOptions) (giztools.HTTPAuthorizer, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	record, err := getCredentialRecord(ctx, store, strings.TrimSpace(config.Credential))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return nil, fmt.Errorf("credential: %q not found", config.Credential)
		}
		return nil, fmt.Errorf("credential: load %q: %w", config.Credential, err)
	}
	switch config.Method {
	case "volc_ark":
		if !providerIs(record.Provider, "volc") {
			return nil, providerMismatch(record.Provider, config.Method)
		}
		body, err := record.Body.AsVolcCredentialBody()
		if err != nil || empty(body.ArkApiKey) {
			return nil, missingField(config.Method, "ark_api_key")
		}
		return bearerAuthorizer(*body.ArkApiKey), nil
	case "volc_search":
		if !providerIs(record.Provider, "volc") {
			return nil, providerMismatch(record.Provider, config.Method)
		}
		body, err := record.Body.AsVolcCredentialBody()
		if err != nil || empty(body.SearchApiKey) {
			return nil, missingField(config.Method, "search_api_key")
		}
		return bearerAuthorizer(*body.SearchApiKey), nil
	case "volc_openapi":
		if !providerIs(record.Provider, "volc") {
			return nil, providerMismatch(record.Provider, config.Method)
		}
		body, err := record.Body.AsVolcCredentialBody()
		if err != nil || empty(body.OpenapiAccessKeyId) || empty(body.OpenapiAccessKey) {
			return nil, missingField(config.Method, "openapi_access_key_id/openapi_access_key")
		}
		return volcOpenAPIAuthorizer{
			accessKeyID:  *body.OpenapiAccessKeyId,
			secret:       *body.OpenapiAccessKey,
			sessionToken: value(body.OpenapiSessionToken),
			region:       strings.TrimSpace(config.Region),
			service:      strings.TrimSpace(config.Service),
			now:          nowFunc(options.Now),
		}, nil
	case "aliyun_app_code":
		if !providerIs(record.Provider, "aliyun") {
			return nil, providerMismatch(record.Provider, config.Method)
		}
		body, err := record.Body.AsAliyunCredentialBody()
		if err != nil || empty(body.AppCode) {
			return nil, missingField(config.Method, "app_code")
		}
		return headerAuthorizer("Authorization", "APPCODE "+*body.AppCode), nil
	case "aliyun_openapi_v3":
		if !providerIs(record.Provider, "aliyun") {
			return nil, providerMismatch(record.Provider, config.Method)
		}
		body, err := record.Body.AsAliyunCredentialBody()
		if err != nil || empty(body.AccessKeyId) || empty(body.AccessKeySecret) {
			return nil, missingField(config.Method, "access_key_id/access_key_secret")
		}
		return aliyunV3Authorizer{
			accessKeyID:   *body.AccessKeyId,
			secret:        *body.AccessKeySecret,
			securityToken: value(body.SecurityToken),
			action:        strings.TrimSpace(config.Action),
			version:       strings.TrimSpace(config.Version),
			now:           nowFunc(options.Now),
			nonce:         nonceFunc(options.Nonce),
		}, nil
	default:
		return nil, fmt.Errorf("credential: unsupported HTTP auth method %q", config.Method)
	}
}

func bearerAuthorizer(token string) giztools.HTTPAuthorizer {
	return headerAuthorizer("Authorization", "Bearer "+token)
}

func headerAuthorizer(name, value string) giztools.HTTPAuthorizer {
	return giztools.HTTPAuthorizerFunc(func(_ context.Context, request *http.Request) error {
		request.Header.Set(name, value)
		return nil
	})
}

type volcOpenAPIAuthorizer struct {
	accessKeyID  string
	secret       string
	sessionToken string
	region       string
	service      string
	now          func() time.Time
}

func (a volcOpenAPIAuthorizer) Authorize(_ context.Context, request *http.Request) error {
	if a.region == "" || a.service == "" {
		return errors.New("credential: volc_openapi region and service are required")
	}
	body, err := requestPayload(request)
	if err != nil {
		return err
	}
	host := request.Host
	if host == "" {
		host = request.URL.Host
	}
	signature := volcbase.GetSignRequest(volcbase.RequestParam{
		Body: body, Method: request.Method, Date: a.now().UTC(),
		Path: request.URL.Path, Host: host, QueryList: request.URL.Query(),
		Headers: request.Header,
	}, volcbase.Credentials{
		AccessKeyID: a.accessKeyID, SecretAccessKey: a.secret,
		SessionToken: a.sessionToken, Region: a.region, Service: a.service,
	})
	request.Header.Set("Host", signature.Host)
	request.Header.Set("Content-Type", signature.ContentType)
	request.Header.Set("X-Date", signature.XDate)
	request.Header.Set("X-Content-Sha256", signature.XContentSha256)
	request.Header.Set("Authorization", signature.Authorization)
	if signature.XSecurityToken != "" {
		request.Header.Set("X-Security-Token", signature.XSecurityToken)
	}
	return nil
}

type aliyunV3Authorizer struct {
	accessKeyID   string
	secret        string
	securityToken string
	action        string
	version       string
	now           func() time.Time
	nonce         func() (string, error)
}

func (a aliyunV3Authorizer) Authorize(_ context.Context, request *http.Request) error {
	if a.action == "" || a.version == "" {
		return errors.New("credential: aliyun_openapi_v3 action and version are required")
	}
	nonce, err := a.nonce()
	if err != nil {
		return fmt.Errorf("credential: create Aliyun signature nonce: %w", err)
	}
	payloadHash, err := requestPayloadHash(request)
	if err != nil {
		return err
	}
	request.Header.Set("X-Acs-Action", a.action)
	request.Header.Set("X-Acs-Version", a.version)
	request.Header.Set("X-Acs-Date", a.now().UTC().Format("2006-01-02T15:04:05Z"))
	request.Header.Set("X-Acs-Signature-Nonce", nonce)
	request.Header.Set("X-Acs-Content-Sha256", payloadHash)
	if a.securityToken != "" {
		request.Header.Set("X-Acs-Security-Token", a.securityToken)
	}
	canonicalHeaders, signedHeaders := canonicalHeaders(request)
	canonicalRequest := strings.Join([]string{
		request.Method,
		escapedPath(request.URL),
		canonicalQuery(request.URL),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	stringToSign := "ACS3-HMAC-SHA256\n" + sha256Hex([]byte(canonicalRequest))
	signature := hex.EncodeToString(hmacSHA256([]byte(a.secret), stringToSign))
	request.Header.Set("Authorization", "ACS3-HMAC-SHA256 Credential="+a.accessKeyID+
		",SignedHeaders="+signedHeaders+",Signature="+signature)
	return nil
}

func requestPayloadHash(request *http.Request) (string, error) {
	body, err := requestPayload(request)
	if err != nil {
		return "", err
	}
	return sha256Hex(body), nil
}

func requestPayload(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("credential: read request body for signing: %w", err)
	}
	request.Body.Close()
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	return body, nil
}

func canonicalHeaders(request *http.Request) (string, string) {
	headers := make(map[string]string, len(request.Header)+1)
	headers["host"] = strings.TrimSpace(request.Host)
	if headers["host"] == "" {
		headers["host"] = request.URL.Host
	}
	for name, values := range request.Header {
		lower := strings.ToLower(name)
		if lower == "authorization" {
			continue
		}
		normalized := make([]string, len(values))
		for i, item := range values {
			normalized[i] = strings.Join(strings.Fields(item), " ")
		}
		headers[lower] = strings.Join(normalized, ",")
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name)
		canonical.WriteByte(':')
		canonical.WriteString(headers[name])
		canonical.WriteByte('\n')
	}
	return canonical.String(), strings.Join(names, ";")
}

func canonicalQuery(target *url.URL) string {
	query := target.Query()
	return strings.ReplaceAll(query.Encode(), "+", "%20")
}

func escapedPath(target *url.URL) string {
	path := target.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func nowFunc(value func() time.Time) func() time.Time {
	if value != nil {
		return value
	}
	return time.Now
}

func nonceFunc(value func() (string, error)) func() (string, error) {
	if value != nil {
		return value
	}
	return func() (string, error) {
		data := make([]byte, 16)
		if _, err := rand.Read(data); err != nil {
			return "", err
		}
		return hex.EncodeToString(data), nil
	}
}

func providerIs(actual string, allowed ...string) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	return slices.Contains(allowed, actual)
}

func providerMismatch(provider, method string) error {
	return fmt.Errorf("credential: provider %q does not support auth method %q", provider, method)
}

func missingField(method, field string) error {
	return fmt.Errorf("credential: auth method %q requires Credential field %s", method, field)
}

func empty(value *string) bool {
	return value == nil || strings.TrimSpace(*value) == ""
}

func value(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
