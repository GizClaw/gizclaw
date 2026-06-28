package gizwebrtc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const signalingNonceBytes = 16

func deriveSignaling(local *giznet.KeyPair, remote giznet.PublicKey, clientNonce string, ts int64, mode CipherMode) (request cipher.AEAD, requestNonce []byte, response cipher.AEAD, responseNonce []byte, err error) {
	shared, err := local.DH(remote)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	nonceRaw, err := base64.RawURLEncoding.DecodeString(clientNonce)
	if err != nil || len(nonceRaw) != signalingNonceBytes {
		return nil, nil, nil, nil, fmt.Errorf("gizwebrtc: invalid signaling nonce")
	}
	salt := append([]byte{}, nonceRaw...)
	salt = strconv.AppendInt(salt, ts, 10)

	reqKey, err := hkdfBytes(shared[:], salt, "giznet/gizwebrtc/http-signaling/v1 c2s", keySizeForMode(mode))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	respKey, err := hkdfBytes(shared[:], salt, "giznet/gizwebrtc/http-signaling/v1 s2c", keySizeForMode(mode))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	reqNonce, err := hkdfBytes(shared[:], salt, "giznet/gizwebrtc/http-signaling/v1 c2s nonce", 12)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	respNonce, err := hkdfBytes(shared[:], salt, "giznet/gizwebrtc/http-signaling/v1 s2c nonce", 12)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	reqAEAD, err := newAEAD(mode, reqKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	respAEAD, err := newAEAD(mode, respKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return reqAEAD, reqNonce, respAEAD, respNonce, nil
}

func keySizeForMode(mode CipherMode) int {
	switch mode {
	case CipherModeAES256GCM:
		return 32
	case CipherModePlaintext:
		return 32
	default:
		return chacha20poly1305.KeySize
	}
}

func hkdfBytes(secret, salt []byte, info string, n int) ([]byte, error) {
	out := make([]byte, n)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, salt, []byte(info)), out); err != nil {
		return nil, err
	}
	return out, nil
}

func newAEAD(mode CipherMode, key []byte) (cipher.AEAD, error) {
	switch mode {
	case "", CipherModeChaChaPoly:
		return chacha20poly1305.New(key)
	case CipherModeAES256GCM:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	case CipherModePlaintext:
		return plaintextAEAD{}, nil
	default:
		return nil, fmt.Errorf("gizwebrtc: unsupported cipher mode %q", mode)
	}
}

func requestAAD(client giznet.PublicKey, ts int64, nonce string) []byte {
	return []byte(strings.Join([]string{
		"POST",
		SignalingPath,
		client.String(),
		strconv.FormatInt(ts, 10),
		nonce,
	}, "\n"))
}

func responseAAD(client giznet.PublicKey, ts int64, nonce string) []byte {
	return []byte(strings.Join([]string{
		"POST",
		SignalingPath,
		client.String(),
		strconv.FormatInt(ts, 10),
		nonce,
		"answer",
	}, "\n"))
}

type plaintextAEAD struct{}

func (plaintextAEAD) NonceSize() int { return 12 }
func (plaintextAEAD) Overhead() int  { return 0 }
func (plaintextAEAD) Seal(dst, _nonce, plaintext, _aad []byte) []byte {
	return append(dst, plaintext...)
}
func (plaintextAEAD) Open(dst, _nonce, ciphertext, _aad []byte) ([]byte, error) {
	return append(dst, ciphertext...), nil
}
