package peerhttp

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"go.step.sm/crypto/x25519"
)

func TestSignedBearerJavaScriptVectors(t *testing.T) {
	// These synthetic vectors are also reproduced byte-for-byte by the JS
	// signer tests, independently of the Go implementation used for verification.
	data, err := os.ReadFile("../../../../sdk/js/gizclaw/testdata/signed-bearer.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		PublicKey string `json:"publicKey"`
		Target    string `json:"target"`
		Now       int64  `json:"now"`
		Token     string `json:"token"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 {
		t.Fatal("missing cross-language vectors")
	}
	for _, vector := range vectors {
		caller, target, err := VerifySignedBearerToken(vector.Token, time.Unix(vector.Now, 0))
		if err != nil || caller.String() != vector.PublicKey || target.String() != vector.Target {
			t.Fatalf("JavaScript signature rejected: %v", err)
		}
		if _, _, err := VerifySignedBearerToken(vector.Token, time.Unix(vector.Now+60, 0)); !errors.Is(err, ErrInvalidSignedBearer) {
			t.Fatal("expired JavaScript signature accepted")
		}
	}
}

func TestSignedBearerAuthenticatesCallerAndBindsTarget(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	other, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []giznet.PublicKey{{}, other.Public} {
		token, err := NewSignedBearerToken(key, target, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		caller, actualTarget, err := VerifySignedBearerToken(token, time.Now())
		if target.IsZero() {
			target = key.Public
		}
		if err != nil || caller != key.Public || actualTarget != target {
			t.Fatalf("verified identity: %v", err)
		}
		if _, _, err := VerifySignedBearerToken(token, time.Now().Add(time.Minute)); !errors.Is(err, ErrInvalidSignedBearer) {
			t.Fatal("expired token accepted")
		}
		message, signature, _ := strings.Cut(token, ".")
		payload, _ := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(message, signedBearerPrefix))
		var claims signedBearerClaims
		if json.Unmarshal(payload, &claims) != nil {
			t.Fatal("invalid claims")
		}
		claims.Target = key.Public.String()
		if target == key.Public {
			claims.Target = other.Public.String()
		}
		changed, _ := json.Marshal(claims)
		for _, forged := range []string{
			signedBearerPrefix + base64.RawURLEncoding.EncodeToString(changed) + "." + signature,
			message + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 64)),
			token + ".extra", token[:len(token)-1], strings.Repeat("x", 3000),
		} {
			if _, _, err := VerifySignedBearerToken(forged, time.Now()); !errors.Is(err, ErrInvalidSignedBearer) {
				t.Fatal("tampered bearer accepted")
			}
		}
	}
	for _, invalid := range []*giznet.KeyPair{nil, {}, {Public: other.Public, Private: key.Private}} {
		if _, err := NewSignedBearerToken(invalid, other.Public, time.Minute); err == nil {
			t.Fatal("invalid signing key accepted")
		}
	}
	for _, lifetime := range []time.Duration{0, -time.Second, time.Millisecond, MaxSignedBearerLifetime + time.Second} {
		if _, err := NewSignedBearerToken(key, other.Public, lifetime); err == nil {
			t.Fatal("unbounded lifetime accepted")
		}
	}
}

func TestSignedBearerRejectsInvalidClaimsAndImpersonation(t *testing.T) {
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	other, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	valid := signedBearerClaims{Issuer: key.Public.String(), Audience: signedBearerAudience, Target: other.Public.String(), IssuedAt: now.Unix(), Expires: now.Add(time.Minute).Unix()}
	for name, mutate := range map[string]func(*signedBearerClaims){
		"audience":            func(c *signedBearerClaims) { c.Audience = "other-service" },
		"issuer":              func(c *signedBearerClaims) { c.Issuer = other.Public.String() },
		"zero issuer":         func(c *signedBearerClaims) { c.Issuer = (giznet.PublicKey{}).String() },
		"missing target":      func(c *signedBearerClaims) { c.Target = "" },
		"noncanonical issuer": func(c *signedBearerClaims) { c.Issuer += " " },
		"expired":             func(c *signedBearerClaims) { c.Expires = now.Unix() },
		"no issued at":        func(c *signedBearerClaims) { c.IssuedAt = 0 },
		"future":              func(c *signedBearerClaims) { c.IssuedAt = now.Add(time.Minute).Unix(); c.Expires += 120 },
		"long lifetime":       func(c *signedBearerClaims) { c.Expires = now.Add(MaxSignedBearerLifetime + time.Second).Unix() },
		"reverse lifetime":    func(c *signedBearerClaims) { c.Expires = c.IssuedAt },
	} {
		t.Run(name, func(t *testing.T) {
			claims := valid
			mutate(&claims)
			payload, _ := json.Marshal(claims)
			message := signedBearerPrefix + base64.RawURLEncoding.EncodeToString(payload)
			signature, err := x25519.Sign(rand.Reader, key.Private[:], []byte(message))
			if err != nil {
				t.Fatal(err)
			}
			token := message + "." + base64.RawURLEncoding.EncodeToString(signature)
			if _, _, err := VerifySignedBearerToken(token, now); !errors.Is(err, ErrInvalidSignedBearer) {
				t.Fatal("invalid claims accepted")
			}
		})
	}
	// A low-order Montgomery identity allows an algebraic signature forgery and
	// must never authenticate even if such a Peer was configured accidentally.
	valid.Issuer = (giznet.PublicKey{1}).String()
	payload, _ := json.Marshal(valid)
	signature := make([]byte, 64)
	signature[0] = 1 // R = identity, s = 0
	token := signedBearerPrefix + base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature)
	if _, _, err := VerifySignedBearerToken(token, now); !errors.Is(err, ErrInvalidSignedBearer) {
		t.Fatal("low-order forgery accepted")
	}
}

func FuzzVerifySignedBearerToken(f *testing.F) {
	for _, token := range []string{"", "gizclaw_sig_v1_", "gizclaw_sig_v1_e30.AA", "gizclaw_sk_v1_legacy", "gizclaw_pk_1"} {
		f.Add(token)
	}
	f.Fuzz(func(t *testing.T, token string) {
		_, _, _ = VerifySignedBearerToken(token, time.Unix(1792530000, 0))
	})
}
