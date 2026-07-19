package peerhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type optionalBodyDoer func(*http.Request) (*http.Response, error)

func (f optionalBodyDoer) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestLoginClientPreservesBodylessAndTypedBodyHelpers(t *testing.T) {
	requests := make([]*http.Request, 0, 3)
	client, err := NewClient("https://example.test", WithHTTPClient(optionalBodyDoer(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"token","token_type":"Bearer","expires_at":1}`)),
		}, nil
	})))
	if err != nil {
		t.Fatalf("NewClient error = %v", err)
	}
	params := &LoginParams{XPublicKey: "controller", Authorization: "Bearer assertion"}

	response, err := client.Login(context.Background(), params)
	if err != nil {
		t.Fatalf("Login error = %v", err)
	}
	_ = response.Body.Close()
	response, err = client.LoginWithJSONBody(context.Background(), params, LoginJSONRequestBody{
		GrantType:   SideControl,
		DeviceToken: "device-token",
	})
	if err != nil {
		t.Fatalf("LoginWithJSONBody error = %v", err)
	}
	_ = response.Body.Close()
	responseClient := &ClientWithResponses{ClientInterface: client}
	parsed, err := responseClient.LoginWithResponse(context.Background(), params)
	if err != nil {
		t.Fatalf("LoginWithResponse error = %v", err)
	}
	if parsed.JSON200 == nil || parsed.JSON200.AccessToken != "token" {
		t.Fatalf("LoginWithResponse result = %+v", parsed.JSON200)
	}

	if len(requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(requests))
	}
	var primaryBody []byte
	if requests[0].Body != nil {
		primaryBody, err = io.ReadAll(requests[0].Body)
		if err != nil {
			t.Fatalf("read primary body: %v", err)
		}
	}
	if len(primaryBody) != 0 || requests[0].Header.Get("Content-Type") != "" {
		t.Fatalf("primary body = %q content-type = %q, want bodyless", primaryBody, requests[0].Header.Get("Content-Type"))
	}
	sideBody, err := io.ReadAll(requests[1].Body)
	if err != nil {
		t.Fatalf("read side body: %v", err)
	}
	var grant LoginRequest
	if err := json.Unmarshal(sideBody, &grant); err != nil {
		t.Fatalf("decode side body %q: %v", sideBody, err)
	}
	if grant.GrantType != SideControl || grant.DeviceToken != "device-token" || requests[1].Header.Get("Content-Type") != "application/json" {
		t.Fatalf("side grant = %+v content-type = %q", grant, requests[1].Header.Get("Content-Type"))
	}
	if requests[2].Body != nil || requests[2].Header.Get("Content-Type") != "" {
		t.Fatalf("response helper body = %v content-type = %q, want bodyless", requests[2].Body, requests[2].Header.Get("Content-Type"))
	}
}

func TestParseLoginResponseHandlesBadRequest(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"code":"INVALID_REQUEST","message":"malformed login request"}}`)),
	}
	parsed, err := ParseLoginResponse(response)
	if err != nil {
		t.Fatalf("ParseLoginResponse error = %v", err)
	}
	if parsed.JSON400 == nil || parsed.JSON400.Error.Code != "INVALID_REQUEST" {
		t.Fatalf("ParseLoginResponse JSON400 = %+v", parsed.JSON400)
	}
}
