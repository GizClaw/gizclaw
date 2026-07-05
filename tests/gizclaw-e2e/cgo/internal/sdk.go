//go:build gizclaw_e2e

package internal

/*
#cgo CFLAGS: -I. -I../../../../c/gizclaw/include -I../../../../c/gizclaw/generated
#include "gzc_common.h"
#include "sdk_client.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unsafe"

	"github.com/GizClaw/gizclaw-go/c/gizclaw/cgobackend"
)

type Client struct {
	session *C.gzc_cgo_session_t
}

func NewClient(identityDir string) (*Client, error) {
	cfg, err := readClientConfig(identityDir)
	if err != nil {
		return nil, err
	}
	cSignalingURL := C.CString(cfg.signalingURL)
	defer C.free(unsafe.Pointer(cSignalingURL))
	cPrivateKey := C.CString(cfg.privateKey)
	defer C.free(unsafe.Pointer(cPrivateKey))
	cServerPublicKey := C.CString(cfg.serverPublicKey)
	defer C.free(unsafe.Pointer(cServerPublicKey))
	errbuf := make([]byte, 1024)
	var session *C.gzc_cgo_session_t
	rc := C.gzc_cgo_session_open(
		cSignalingURL,
		cPrivateKey,
		cServerPublicKey,
		&session,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return nil, fmt.Errorf("open C SDK session rc=%d: %s", int(rc), cString(errbuf))
	}
	return &Client{session: session}, nil
}

func (c *Client) Close() {
	if c == nil || c.session == nil {
		return
	}
	C.gzc_cgo_session_close(c.session)
	c.session = nil
}

func (c *Client) CallJSON(method string, params json.RawMessage) (json.RawMessage, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("closed C SDK client")
	}
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	cParams := C.CString(string(params))
	defer C.free(unsafe.Pointer(cParams))
	errbuf := make([]byte, 1024)
	var result *C.char
	var resultLen C.ulong
	rc := C.gzc_cgo_session_call_json(
		c.session,
		cMethod,
		cParams,
		&result,
		&resultLen,
		(*C.char)(unsafe.Pointer(&errbuf[0])),
		C.ulong(len(errbuf)),
	)
	if rc != C.GZC_OK {
		return nil, fmt.Errorf("call %s rc=%d: %s", method, int(rc), cString(errbuf))
	}
	defer C.gzc_cgo_free(unsafe.Pointer(result))
	return append([]byte(nil), C.GoBytes(unsafe.Pointer(result), C.int(resultLen))...), nil
}

func CSDKPing(t *testing.T, identityDir string) {
	t.Helper()
	client, err := NewClient(identityDir)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.CallJSON("all.ping", json.RawMessage(`{"client_send_time":12345}`))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		ServerTime int64 `json:"server_time"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatalf("decode ping result: %v: %s", err, string(result))
	}
	if response.ServerTime <= 0 {
		t.Fatalf("invalid server_time: %d", response.ServerTime)
	}
}

func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return fmt.Sprintf("%q", string(buf))
}

type clientConfig struct {
	signalingURL    string
	privateKey      string
	serverPublicKey string
}

func readClientConfig(identityDir string) (clientConfig, error) {
	data, err := os.ReadFile(filepath.Join(identityDir, "config.yaml"))
	if err != nil {
		return clientConfig{}, err
	}
	config := string(data)
	endpoint := matchConfigValue(config, `endpoint:\s*"?([^"\s]+)"?`)
	privateKey := matchConfigValue(config, `private-key:\s*"?([^"\s]+)"?`)
	serverPublicKey := matchConfigValue(config, `public-key:\s*"?([^"\s]+)"?`)
	if endpoint == "" || privateKey == "" || serverPublicKey == "" {
		return clientConfig{}, fmt.Errorf("incomplete C SDK identity config %s", filepath.Join(identityDir, "config.yaml"))
	}
	return clientConfig{
		signalingURL:    "http://" + endpoint + cgobackend.SignalingPath,
		privateKey:      privateKey,
		serverPublicKey: serverPublicKey,
	}, nil
}

func matchConfigValue(config, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(config)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
