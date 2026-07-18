package runtimeprofile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestRegistrationTokenIsReturnedOnceAndStoredAsHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	now := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	store := kv.NewMemory(nil)
	s := &Server{
		Store:          store,
		Now:            func() time.Time { return now },
		Random:         strings.NewReader(strings.Repeat("x", tokenBytes)),
		FirmwareExists: func(context.Context, string) (bool, error) { return true, nil },
	}
	createProfile(t, s, "pet-runtime", map[string]string{
		"primary":   "model-a",
		"secondary": " model-b ",
		"duplicate": "model-a",
	})

	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "pet-board", FirmwareName: "h106", RuntimeProfileName: "pet-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok || created.Token == "" {
		t.Fatalf("create response = %#v, want one-time token", response)
	}
	raw := created.Token
	stored, err := store.Get(ctx, tokenKey("pet-board"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), raw) {
		t.Fatal("stored record contains raw token")
	}
	var private tokenRecord
	if err := json.Unmarshal(stored, &private); err != nil {
		t.Fatal(err)
	}
	if private.TokenHash != tokenDigest(raw) {
		t.Fatalf("stored digest = %q, want SHA-256 digest", private.TokenHash)
	}

	gotResponse, err := s.GetRegistrationToken(ctx, adminhttp.GetRegistrationTokenRequestObject{Name: "pet-board"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := gotResponse.(adminhttp.GetRegistrationToken200JSONResponse)
	if !ok || got.Name != "pet-board" {
		t.Fatalf("get response = %#v", gotResponse)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), raw) || strings.Contains(string(encoded), "token_hash") {
		t.Fatalf("get response leaked token material: %s", encoded)
	}

	registration, err := s.ResolveRegistration(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if registration.FirmwareName != "h106" || registration.RuntimeProfile.Name != "pet-runtime" {
		t.Fatalf("registration = %#v", registration)
	}
	models := *registration.RuntimeProfile.Spec.Resources.Models
	if len(models) != 3 || models["primary"] != "model-a" || models["secondary"] != "model-b" || models["duplicate"] != "model-a" {
		t.Fatalf("normalized models = %#v", models)
	}
}

func TestRegistrationTokenCanBeReusedUntilDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	s := &Server{Store: store, Random: strings.NewReader(strings.Repeat("y", tokenBytes))}
	createProfile(t, s, "pet-runtime", nil)
	response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
		Name: "pet-board", FirmwareName: "h106", RuntimeProfileName: "pet-runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := response.(adminhttp.CreateRegistrationToken200JSONResponse)
	for range 2 {
		if _, err := s.ResolveRegistration(ctx, created.Token); err != nil {
			t.Fatalf("reusable token resolve: %v", err)
		}
	}
	if _, err := s.DeleteRegistrationToken(ctx, adminhttp.DeleteRegistrationTokenRequestObject{Name: "pet-board"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveRegistration(ctx, created.Token); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("resolve after delete error = %v, want not found", err)
	}
}

func TestConcurrentRegistrationTokenCreateKeepsNameAndHashIndexesConsistent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := kv.NewMemory(nil)
	s := &Server{Store: store}
	createProfile(t, s, "pet-runtime", nil)

	const attempts = 16
	responses := make(chan adminhttp.CreateRegistrationTokenResponseObject, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			response, err := s.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{Body: &adminhttp.RegistrationTokenUpsert{
				Name: "pet-board", FirmwareName: "h106", RuntimeProfileName: "pet-runtime",
			}})
			if err != nil {
				t.Errorf("CreateRegistrationToken() error = %v", err)
				return
			}
			responses <- response
		})
	}
	wg.Wait()
	close(responses)

	created := 0
	conflicts := 0
	var raw string
	for response := range responses {
		switch value := response.(type) {
		case adminhttp.CreateRegistrationToken200JSONResponse:
			created++
			raw = value.Token
		case adminhttp.CreateRegistrationToken409JSONResponse:
			conflicts++
		default:
			t.Fatalf("CreateRegistrationToken() response = %#v", response)
		}
	}
	if created != 1 || conflicts != attempts-1 || raw == "" {
		t.Fatalf("created=%d conflicts=%d raw_empty=%t", created, conflicts, raw == "")
	}
	if _, err := s.ResolveRegistration(ctx, raw); err != nil {
		t.Fatalf("ResolveRegistration() error = %v", err)
	}
}

func TestDanglingRuntimeProfileResourceNamesAreAccepted(t *testing.T) {
	t.Parallel()
	s := &Server{Store: kv.NewMemory(nil)}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: "pet-runtime",
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{
			Workflows: new(map[string]string{"missing": "missing-workflow"}),
			Models:    new(map[string]string{"missing": "missing-model"}),
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("response = %#v, want success", response)
	}
}

func createProfile(t *testing.T, s *Server, name string, models map[string]string) {
	t.Helper()
	resources := apitypes.RuntimeProfileResources{}
	if models != nil {
		resources.Models = &models
	}
	response, err := s.CreateRuntimeProfile(context.Background(), adminhttp.CreateRuntimeProfileRequestObject{Body: &adminhttp.RuntimeProfileUpsert{
		Name: name, Spec: apitypes.RuntimeProfileSpec{Resources: resources},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("create profile response = %#v", response)
	}
}
