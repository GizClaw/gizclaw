//go:build gizclaw_e2e

package internal

/*
#cgo CFLAGS: -I. -I../../../../c/gizclaw/include -I../../../../c/gizclaw/generated
#include "bridge.h"
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"time"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"runtime/cgo"
)

func backendFromHandle(handle C.uint64_t) *backend {
	if handle == 0 {
		return nil
	}
	h := cgo.Handle(uintptr(handle))
	b, _ := h.Value().(*backend)
	return b
}

//export gzcGoBackendCreate
func gzcGoBackendCreate(identityDir *C.char) C.uint64_t {
	b, err := newBackend(C.GoString(identityDir))
	if err != nil {
		return 0
	}
	return C.uint64_t(cgo.NewHandle(b))
}

//export gzcGoBackendDestroy
func gzcGoBackendDestroy(handle C.uint64_t) {
	b := backendFromHandle(handle)
	if b != nil {
		b.clearCBackend()
		b.close()
	}
	cgo.Handle(uintptr(handle)).Delete()
}

//export gzcGoBackendSetCBackend
func gzcGoBackendSetCBackend(handle C.uint64_t, cBackend *C.gzc_cgo_backend_t) {
	if b := backendFromHandle(handle); b != nil {
		b.setCBackend(unsafe.Pointer(cBackend))
	}
}

//export gzcGoRandom
func gzcGoRandom(out *C.uint8_t, length C.size_t) C.int {
	if length == 0 {
		return C.GZC_OK
	}
	if out == nil && length != 0 {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	n, ok := cIntLen(length)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	outBytes := unsafe.Slice((*byte)(unsafe.Pointer(out)), n)
	if _, err := io.ReadFull(rand.Reader, outBytes); err != nil {
		return C.GZC_ERR_SIGNALING
	}
	return C.GZC_OK
}

//export gzcGoTimeUnixMs
func gzcGoTimeUnixMs() C.int64_t {
	return C.int64_t(time.Now().UnixMilli())
}

//export gzcGoBackendClientConfig
func gzcGoBackendClientConfig(handle C.uint64_t, signalingURL *C.char, signalingURLCap C.size_t, privateKey *C.char, privateKeyCap C.size_t, serverPublicKey *C.char, serverPublicKeyCap C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if rc := writeCString(signalingURL, signalingURLCap, "http://"+b.endpoint+signalingPath); rc != C.GZC_OK {
		return rc
	}
	if rc := writeCString(privateKey, privateKeyCap, b.key.Private.String()); rc != C.GZC_OK {
		return rc
	}
	return writeCString(serverPublicKey, serverPublicKeyCap, b.serverPK.String())
}

//export gzcGoHTTPRequest
func gzcGoHTTPRequest(handle C.uint64_t, method C.int, urlData *C.char, urlLen C.size_t, headers *C.gzc_http_header_t, headerCount C.size_t, data *C.uint8_t, length C.size_t, outStatus *C.int, outData **C.uint8_t, outLen *C.size_t) C.int {
	if backendFromHandle(handle) == nil || urlData == nil || outStatus == nil || outData == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	methodText, ok := httpMethod(method)
	if !ok {
		return C.GZC_ERR_UNSUPPORTED
	}
	url, ok := goString(urlData, urlLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	body, ok := goBytes(unsafe.Pointer(data), length)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, methodText, url, bytes.NewReader(body))
	if err != nil {
		return C.GZC_ERR_HTTP
	}
	headerList, ok := cHeaders(headers, headerCount)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	for _, header := range headerList {
		name, ok := goString(header.name.data, C.size_t(header.name.len))
		if !ok {
			return C.GZC_ERR_INVALID_ARGUMENT
		}
		value, ok := goString(header.value.data, C.size_t(header.value.len))
		if !ok {
			return C.GZC_ERR_INVALID_ARGUMENT
		}
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return C.GZC_ERR_HTTP
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return C.GZC_ERR_HTTP
	}
	*outStatus = C.int(resp.StatusCode)
	if len(respBody) > 0 {
		*outData = (*C.uint8_t)(C.CBytes(respBody))
		*outLen = C.size_t(len(respBody))
	} else {
		*outData = nil
		*outLen = 0
	}
	return C.GZC_OK
}

//export gzcGoKeyPairFromPrivate
func gzcGoKeyPairFromPrivate(privateKey *C.uint8_t, outPrivateKey *C.uint8_t, outPublicKey *C.uint8_t) C.int {
	if privateKey == nil || outPrivateKey == nil || outPublicKey == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	var private giznet.Key
	copy(private[:], unsafe.Slice((*byte)(unsafe.Pointer(privateKey)), giznet.KeySize))
	kp, err := giznet.NewKeyPair(private)
	if err != nil {
		return C.GZC_ERR_SIGNALING
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(outPrivateKey)), giznet.KeySize), kp.Private[:])
	copy(unsafe.Slice((*byte)(unsafe.Pointer(outPublicKey)), giznet.KeySize), kp.Public[:])
	return C.GZC_OK
}

//export gzcGoKeyFromText
func gzcGoKeyFromText(text *C.char, textLen C.size_t, outKey *C.uint8_t) C.int {
	if text == nil || outKey == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	var key giznet.Key
	textValue, ok := goString(text, textLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := key.UnmarshalText([]byte(textValue)); err != nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(outKey)), giznet.KeySize), key[:])
	return C.GZC_OK
}

//export gzcGoKeyToText
func gzcGoKeyToText(keyData *C.uint8_t, outText *C.char, outTextCap C.size_t, outTextLen *C.size_t) C.int {
	if keyData == nil || outText == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	var key giznet.Key
	copy(key[:], unsafe.Slice((*byte)(unsafe.Pointer(keyData)), giznet.KeySize))
	return writeCStringLen(outText, outTextCap, key.String(), outTextLen)
}

//export gzcGoDH
func gzcGoDH(privateKey *C.uint8_t, remotePublicKey *C.uint8_t, outShared *C.uint8_t) C.int {
	if privateKey == nil || remotePublicKey == nil || outShared == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	var private giznet.Key
	var remote giznet.PublicKey
	copy(private[:], unsafe.Slice((*byte)(unsafe.Pointer(privateKey)), giznet.KeySize))
	copy(remote[:], unsafe.Slice((*byte)(unsafe.Pointer(remotePublicKey)), giznet.KeySize))
	kp, err := giznet.NewKeyPair(private)
	if err != nil {
		return C.GZC_ERR_SIGNALING
	}
	shared, err := kp.DH(remote)
	if err != nil {
		return C.GZC_ERR_SIGNALING
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(outShared)), giznet.KeySize), shared[:])
	return C.GZC_OK
}

//export gzcGoHKDFSHA256
func gzcGoHKDFSHA256(secret *C.uint8_t, secretLen C.size_t, salt *C.uint8_t, saltLen C.size_t, info *C.char, infoLen C.size_t, out *C.uint8_t, outLen C.size_t) C.int {
	if secret == nil || info == nil || out == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	secretBytes, ok := goBytes(unsafe.Pointer(secret), secretLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	saltBytes, ok := goBytes(unsafe.Pointer(salt), saltLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	infoText, ok := goString(info, infoLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	infoBytes := []byte(infoText)
	outLenInt, ok := cIntLen(outLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	outBytes := unsafe.Slice((*byte)(unsafe.Pointer(out)), outLenInt)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secretBytes, saltBytes, infoBytes), outBytes); err != nil {
		return C.GZC_ERR_SIGNALING
	}
	return C.GZC_OK
}

//export gzcGoAEADSeal
func gzcGoAEADSeal(mode C.int, key *C.uint8_t, keyLen C.size_t, nonce *C.uint8_t, nonceLen C.size_t, plaintext *C.uint8_t, plaintextLen C.size_t, aad *C.uint8_t, aadLen C.size_t, outData **C.uint8_t, outLen *C.size_t) C.int {
	return gzcGoAEAD(true, mode, key, keyLen, nonce, nonceLen, plaintext, plaintextLen, aad, aadLen, outData, outLen)
}

//export gzcGoAEADOpen
func gzcGoAEADOpen(mode C.int, key *C.uint8_t, keyLen C.size_t, nonce *C.uint8_t, nonceLen C.size_t, ciphertext *C.uint8_t, ciphertextLen C.size_t, aad *C.uint8_t, aadLen C.size_t, outData **C.uint8_t, outLen *C.size_t) C.int {
	return gzcGoAEAD(false, mode, key, keyLen, nonce, nonceLen, ciphertext, ciphertextLen, aad, aadLen, outData, outLen)
}

func gzcGoAEAD(seal bool, mode C.int, key *C.uint8_t, keyLen C.size_t, nonce *C.uint8_t, nonceLen C.size_t, input *C.uint8_t, inputLen C.size_t, aad *C.uint8_t, aadLen C.size_t, outData **C.uint8_t, outLen *C.size_t) C.int {
	if key == nil || nonce == nil || outData == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	inputBytes, ok := goBytes(unsafe.Pointer(input), inputLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	aadBytes, ok := goBytes(unsafe.Pointer(aad), aadLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	keyBytes, ok := goBytes(unsafe.Pointer(key), keyLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	nonceBytes, ok := goBytes(unsafe.Pointer(nonce), nonceLen)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	aead, err := newPlatformAEAD(mode, keyBytes)
	if err != nil {
		return C.GZC_ERR_UNSUPPORTED
	}
	if len(nonceBytes) != aead.NonceSize() {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	var out []byte
	if seal {
		out = aead.Seal(nil, nonceBytes, inputBytes, aadBytes)
	} else {
		out, err = aead.Open(nil, nonceBytes, inputBytes, aadBytes)
		if err != nil {
			return C.GZC_ERR_SIGNALING
		}
	}
	if len(out) > 0 {
		*outData = (*C.uint8_t)(C.CBytes(out))
		*outLen = C.size_t(len(out))
	} else {
		*outData = nil
		*outLen = 0
	}
	return C.GZC_OK
}

func writeCString(dst *C.char, cap C.size_t, value string) C.int {
	return writeCStringLen(dst, cap, value, nil)
}

func writeCStringLen(dst *C.char, cap C.size_t, value string, outLen *C.size_t) C.int {
	if dst == nil || cap == 0 {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	capInt, ok := cIntLen(cap)
	if !ok {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if len(value)+1 > capInt {
		return C.GZC_ERR_NO_MEMORY
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(dst)), capInt)
	copy(buf, value)
	buf[len(value)] = 0
	if outLen != nil {
		*outLen = C.size_t(len(value))
	}
	return C.GZC_OK
}

func goBytes(ptr unsafe.Pointer, n C.size_t) ([]byte, bool) {
	if n == 0 {
		return nil, true
	}
	if ptr == nil && n != 0 {
		return nil, false
	}
	i, ok := cIntLen(n)
	if !ok {
		return nil, false
	}
	return C.GoBytes(ptr, C.int(i)), true
}

func goString(ptr *C.char, n C.size_t) (string, bool) {
	if n == 0 {
		return "", true
	}
	if ptr == nil && n != 0 {
		return "", false
	}
	i, ok := cIntLen(n)
	if !ok {
		return "", false
	}
	return C.GoStringN(ptr, C.int(i)), true
}

func cIntLen(n C.size_t) (int, bool) {
	const maxCInt = int64(1<<31 - 1)
	if uint64(n) > uint64(maxCInt) {
		return 0, false
	}
	return int(n), true
}

func httpMethod(method C.int) (string, bool) {
	switch method {
	case C.GZC_HTTP_METHOD_GET:
		return http.MethodGet, true
	case C.GZC_HTTP_METHOD_POST:
		return http.MethodPost, true
	case C.GZC_HTTP_METHOD_PUT:
		return http.MethodPut, true
	case C.GZC_HTTP_METHOD_PATCH:
		return http.MethodPatch, true
	case C.GZC_HTTP_METHOD_DELETE:
		return http.MethodDelete, true
	case C.GZC_HTTP_METHOD_HEAD:
		return http.MethodHead, true
	case C.GZC_HTTP_METHOD_OPTIONS:
		return http.MethodOptions, true
	default:
		return "", false
	}
}

func cHeaders(headers *C.gzc_http_header_t, count C.size_t) ([]C.gzc_http_header_t, bool) {
	if headers == nil || count == 0 {
		return nil, true
	}
	countInt, ok := cIntLen(count)
	if !ok {
		return nil, false
	}
	out := make([]C.gzc_http_header_t, countInt)
	size := unsafe.Sizeof(*headers)
	base := uintptr(unsafe.Pointer(headers))
	for i := range out {
		out[i] = *(*C.gzc_http_header_t)(unsafe.Pointer(base + uintptr(i)*size))
	}
	return out, true
}

func newPlatformAEAD(mode C.int, key []byte) (cipher.AEAD, error) {
	switch mode {
	case C.GZC_CIPHER_CHACHA20_POLY1305:
		return chacha20poly1305.New(key)
	case C.GZC_CIPHER_AES_256_GCM:
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	case C.GZC_CIPHER_PLAINTEXT:
		return plaintextAEAD{}, nil
	default:
		return nil, fmt.Errorf("unsupported cipher mode %d", int(mode))
	}
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

//export gzcGoPeerCreate
func gzcGoPeerCreate(handle C.uint64_t) C.int {
	if b := backendFromHandle(handle); b != nil {
		if err := b.createPeer(); err != nil {
			return C.GZC_ERR_WEBRTC
		}
		return C.GZC_OK
	}
	return C.GZC_ERR_INVALID_ARGUMENT
}

//export gzcGoPeerStartOffer
func gzcGoPeerStartOffer(handle C.uint64_t, outSDP **C.char, outLen *C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil || outSDP == nil || outLen == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	sdp, err := b.startOffer()
	if err != nil {
		return C.GZC_ERR_WEBRTC
	}
	mem := C.CBytes([]byte(sdp))
	*outSDP = (*C.char)(mem)
	*outLen = C.size_t(len(sdp))
	return C.GZC_OK
}

//export gzcGoPeerSetRemoteSDP
func gzcGoPeerSetRemoteSDP(handle C.uint64_t, sdp *C.char, length C.size_t) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.setRemoteSDP(string(C.GoBytes(unsafe.Pointer(sdp), C.int(length)))); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoPeerCreateDataChannel
func gzcGoPeerCreateDataChannel(handle C.uint64_t, label *C.char, length C.size_t, channelID C.int, ordered C.bool, reliable C.bool) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.createDataChannel(
		string(C.GoBytes(unsafe.Pointer(label), C.int(length))),
		int(channelID),
		bool(ordered),
		bool(reliable),
	); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoPeerPoll
func gzcGoPeerPoll(handle C.uint64_t, timeoutMS C.int) C.int {
	if backendFromHandle(handle) == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if timeoutMS > 0 {
		time.Sleep(time.Duration(timeoutMS) * time.Millisecond)
	}
	return C.GZC_OK
}

//export gzcGoChannelSend
func gzcGoChannelSend(handle C.uint64_t, channelID C.int, data *C.uint8_t, length C.size_t, isText C.bool) C.int {
	b := backendFromHandle(handle)
	if b == nil {
		return C.GZC_ERR_INVALID_ARGUMENT
	}
	if err := b.send(int(channelID), C.GoBytes(unsafe.Pointer(data), C.int(length)), bool(isText)); err != nil {
		return C.GZC_ERR_WEBRTC
	}
	return C.GZC_OK
}

//export gzcGoChannelClose
func gzcGoChannelClose(handle C.uint64_t, channelID C.int) {
	if b := backendFromHandle(handle); b != nil {
		b.closeDataChannel(int(channelID))
	}
}

//export gzcGoPeerClose
func gzcGoPeerClose(handle C.uint64_t) {
	if b := backendFromHandle(handle); b != nil {
		b.close()
	}
}
