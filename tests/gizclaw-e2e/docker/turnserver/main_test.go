package main

import (
	"testing"
	"time"

	"github.com/pion/turn/v4"
)

func TestAuthKeyAcceptsStaticAndFreshRESTCredentials(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cfg := config{
		realm:      "gizclaw-e2e",
		username:   "edge",
		credential: "secret",
	}
	if got, ok := authKey(cfg, cfg.username, cfg.realm, now); !ok {
		t.Fatal("static credential rejected")
	} else if want := turn.GenerateAuthKey(cfg.username, cfg.realm, cfg.credential); string(got) != string(want) {
		t.Fatal("static credential generated the wrong auth key")
	}
	restUsername := "1700000060:edge"
	if _, ok := authKey(cfg, restUsername, cfg.realm, now); !ok {
		t.Fatal("fresh TURN REST credential rejected")
	}
}

func TestAuthKeyRejectsInvalidRESTCredentials(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cfg := config{
		realm:      "gizclaw-e2e",
		username:   "edge",
		credential: "secret",
	}
	for _, username := range []string{
		"",
		"not-a-timestamp:edge",
		"1699999999:edge",
		"1700000060:other",
	} {
		if _, ok := authKey(cfg, username, cfg.realm, now); ok {
			t.Fatalf("authKey accepted %q", username)
		}
	}
	if _, ok := authKey(cfg, "1700000060:edge", "other-realm", now); ok {
		t.Fatal("authKey accepted the wrong realm")
	}
}
