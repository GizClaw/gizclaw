package noise

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/encoding/base32"
)

func TestKeyIsZero(t *testing.T) {
	var zero Key
	if !zero.IsZero() {
		t.Error("zero key should be zero")
	}

	nonZero := Key{1}
	if nonZero.IsZero() {
		t.Error("non-zero key should not be zero")
	}
}

func TestKeyString(t *testing.T) {
	k := Key{0x01, 0x02, 0x03, 0x04}
	got := k.ShortString()
	want := "01020304"
	if got != want {
		t.Errorf("ShortString() = %q, want %q", got, want)
	}

	full := k.String()
	if len(full) != 43 {
		t.Errorf("String() length = %d, want 43", len(full))
	}
	if full != "4wBqpWMLpZcD5z7ZeZezzLQBtTmiGotBQ5GcRFZS6YX" {
		t.Errorf("String() = %q", full)
	}
}

func TestKeyEqual(t *testing.T) {
	k1 := Key{1, 2, 3, 4}
	k2 := Key{1, 2, 3, 4}
	k3 := Key{1, 2, 3, 5}

	if !k1.Equal(k2) {
		t.Error("equal keys should be equal")
	}
	if k1.Equal(k3) {
		t.Error("different keys should not be equal")
	}
}

func TestKeyFromHex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20", false},
		{"invalid hex", "xyz", true},
		{"wrong length", "0102030405", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := KeyFromHex(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("KeyFromHex() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestKeyTextEncoding(t *testing.T) {
	var key Key
	for i := range key {
		key[i] = byte(i + 1)
	}

	text, err := key.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error = %v", err)
	}
	want := "4wBqpZM9xaSheZzJSMawUKKwhdpChKbZ5eu5ky4Vigw"
	if string(text) != want {
		t.Fatalf("MarshalText = %q, want %q", string(text), want)
	}

	var got Key
	if err := got.UnmarshalText(text); err != nil {
		t.Fatalf("UnmarshalText(base58btc) error = %v", err)
	}
	if got != key {
		t.Fatalf("UnmarshalText(base58btc) = %v, want %v", got, key)
	}
}

func TestKeyTextUnmarshalAcceptsLegacyBase64(t *testing.T) {
	var want Key
	for i := range want {
		want[i] = byte(i + 1)
	}

	var got Key
	input := base64.RawURLEncoding.EncodeToString(want[:])
	if err := got.UnmarshalText([]byte(input)); err != nil {
		t.Fatalf("UnmarshalText(base64url) error = %v", err)
	}
	if got != want {
		t.Fatalf("UnmarshalText(base64url) = %v, want %v", got, want)
	}
}

func TestKeyTextUnmarshalAcceptsLegacyHex(t *testing.T) {
	input := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	want, err := KeyFromHex(input)
	if err != nil {
		t.Fatalf("KeyFromHex error = %v", err)
	}

	var got Key
	if err := got.UnmarshalText([]byte(input)); err != nil {
		t.Fatalf("UnmarshalText(hex) error = %v", err)
	}
	if got != want {
		t.Fatalf("UnmarshalText(hex) = %v, want %v", got, want)
	}
}

func TestKeyTextUnmarshalAcceptsLegacyCrockfordAliases(t *testing.T) {
	var want Key
	for i := range want {
		want[i] = byte(i + 1)
	}
	canonical := base32.EncodeToString(want[:])
	input := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(canonical, "0", "O"), "1", "L"))
	input = input[:13] + "-" + input[13:26] + "-" + input[26:39] + "-" + input[39:]

	var got Key
	if err := got.UnmarshalText([]byte(input)); err != nil {
		t.Fatalf("UnmarshalText(crockford aliases) error = %v", err)
	}
	if got != want {
		t.Fatalf("UnmarshalText(crockford aliases) = %v, want %v", got, want)
	}
}

func TestKeyTextUnmarshalRejectsInvalid(t *testing.T) {
	var key Key
	for _, input := range []string{
		"",
		"not-a-key",
		base64.RawURLEncoding.EncodeToString([]byte("short")),
		"041061050R3GG28A1C60T3GF208H44RM2MB1E60S38DHR78Y3WG1",
	} {
		if err := key.UnmarshalText([]byte(input)); err == nil {
			t.Fatalf("UnmarshalText(%q) should fail", input)
		}
	}
}

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	if kp.Private.IsZero() {
		t.Error("private key should not be zero")
	}
	if kp.Public.IsZero() {
		t.Error("public key should not be zero")
	}

	// Verify clamping was applied
	if kp.Private[0]&7 != 0 {
		t.Error("private key not properly clamped (low bits)")
	}
	if kp.Private[31]&128 != 0 {
		t.Error("private key not properly clamped (high bit)")
	}
	if kp.Private[31]&64 == 0 {
		t.Error("private key not properly clamped (second high bit)")
	}
}

func TestGenerateKeyPairDeterministic(t *testing.T) {
	// Use a fixed seed for deterministic testing
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	kp1, err := GenerateKeyPairFrom(bytes.NewReader(seed))
	if err != nil {
		t.Fatalf("GenerateKeyPairFrom() error = %v", err)
	}

	// Reset seed
	for i := range seed {
		seed[i] = byte(i)
	}
	kp2, err := GenerateKeyPairFrom(bytes.NewReader(seed))
	if err != nil {
		t.Fatalf("GenerateKeyPairFrom() error = %v", err)
	}

	if !kp1.Private.Equal(kp2.Private) {
		t.Error("deterministic generation should produce same private key")
	}
	if !kp1.Public.Equal(kp2.Public) {
		t.Error("deterministic generation should produce same public key")
	}
}

func TestNewKeyPair(t *testing.T) {
	var priv Key
	rand.Read(priv[:])

	kp, err := NewKeyPair(priv)
	if err != nil {
		t.Fatalf("NewKeyPair() error = %v", err)
	}

	// Verify the public key can be used for DH
	if kp.Public.IsZero() {
		t.Error("public key should not be zero")
	}
}

func TestDH(t *testing.T) {
	// Generate two key pairs
	alice, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	bob, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	// Perform DH from both sides
	sharedAlice, err := alice.DH(bob.Public)
	if err != nil {
		t.Fatalf("alice.DH() error = %v", err)
	}

	sharedBob, err := bob.DH(alice.Public)
	if err != nil {
		t.Fatalf("bob.DH() error = %v", err)
	}

	// Both should arrive at the same shared secret
	if !sharedAlice.Equal(sharedBob) {
		t.Error("DH shared secrets should be equal")
	}

	// Shared secret should not be zero
	if sharedAlice.IsZero() {
		t.Error("shared secret should not be zero")
	}
}

func TestDHWithKnownVectors(t *testing.T) {
	// Test vector from RFC 7748
	alicePrivHex := "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a"
	bobPubHex := "de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f"
	expectedHex := "4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742"

	alicePriv, _ := KeyFromHex(alicePrivHex)
	bobPub, _ := KeyFromHex(bobPubHex)
	expected, _ := KeyFromHex(expectedHex)

	alice, _ := NewKeyPair(alicePriv)
	shared, err := alice.DH(bobPub)
	if err != nil {
		t.Fatalf("DH() error = %v", err)
	}

	if !shared.Equal(expected) {
		t.Errorf("DH result mismatch\ngot:  %s\nwant: %s", shared.String(), expected.String())
	}
}
