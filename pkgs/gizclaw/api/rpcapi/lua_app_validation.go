package rpcapi

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

var luaAppID = regexp.MustCompile(`^[a-z0-9_-][a-z0-9_.-]{0,31}$`)
var luaAppVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func luaAppText(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}

// LuaAppDataURLMaxBytes leaves room for tool/RPC envelopes below the C SDK's
// bounded 1 MiB receive limit. HTTP(S) URLs retain the 1024-byte address limit.
const LuaAppDataURLMaxBytes = 256 * 1024

// LuaAppArchiveMaxBytes bounds a single compressed Binary upload to 16 MiB.
const LuaAppArchiveMaxBytes = 16 * 1024 * 1024

func validLuaAppInstallSource(source string) bool {
	if !luaAppText(source, LuaAppDataURLMaxBytes) || strings.ContainsFunc(source, func(r rune) bool { return unicode.IsSpace(r) || r == '\\' }) {
		return false
	}
	for _, prefix := range []string{"data:application/zlib;base64,", "data:application/octet-stream;base64,"} {
		if encoded, ok := strings.CutPrefix(source, prefix); ok {
			if encoded == "" || len(encoded)%4 != 0 {
				return false
			}
			decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
			return err == nil && len(decoded) != 0
		}
	}
	u, err := url.Parse(source)
	return err == nil && (strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")) && len(source) <= 1024 && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.Opaque == "" && u.User == nil && !strings.Contains(source, "#")
}

// ValidateLuaAppRequest checks the portable device contract without fetching
// URLs or imposing a particular product's filesystem and runtime policies.
func ValidateLuaAppRequest(message proto.Message) error {
	invalid := fmt.Errorf("rpc: invalid Lua app arguments")
	if message == nil || !message.ProtoReflect().IsValid() {
		return invalid
	}
	switch request := message.(type) {
	case *rpcpb.ClientLuaAppInstallStreamRequest:
		if request.ContentLength == 0 || request.ContentLength > LuaAppArchiveMaxBytes || !clientToolFirmwareDigest.MatchString(request.Sha256) {
			return invalid
		}
		return nil
	case *rpcpb.ClientLuaAppListRequest:
		return nil
	case *rpcpb.ClientLuaAppInstallRequest:
		if !validLuaAppInstallSource(request.Url) {
			return invalid
		}
		if request.Sha256 != nil && !clientToolFirmwareDigest.MatchString(*request.Sha256) {
			return invalid
		}
	case *rpcpb.ClientLuaAppRunRequest:
		if !luaAppID.MatchString(request.AppId) || len(request.Params) > 16 {
			return invalid
		}
		total := 0
		for key, value := range request.Params {
			if key == "" || !luaAppText(key, 64) || !luaAppText(value, 1024) {
				return invalid
			}
			total += len(key) + len(value)
		}
		if total > 4096 {
			return invalid
		}
	default:
		return invalid
	}
	return nil
}

// ValidateLuaAppResponse bounds device-owned catalog data before a controller
// or model can consume it. Display metadata is descriptive, never authority.
func ValidateLuaAppResponse(message proto.Message) error {
	invalid := fmt.Errorf("rpc: invalid Lua app response")
	if message == nil || !message.ProtoReflect().IsValid() {
		return invalid
	}
	check := func(app *rpcpb.LuaAppInfo) bool {
		return app != nil && luaAppID.MatchString(app.AppId) && len(app.Version) <= 31 && luaAppVersion.MatchString(app.Version) && luaAppText(app.GetDisplayName(), 128) && luaAppText(app.GetDescription(), 1024)
	}
	switch response := message.(type) {
	case *rpcpb.ClientLuaAppListResponse:
		if len(response.Apps) > 32 {
			return invalid
		}
		seen := make(map[string]bool, len(response.Apps))
		for _, app := range response.Apps {
			if !check(app) || seen[app.AppId] {
				return invalid
			}
			seen[app.AppId] = true
		}
	case *rpcpb.ClientLuaAppInstallResponse:
		if !check(response.App) {
			return invalid
		}
	case *rpcpb.ClientLuaAppRunResponse:
	default:
		return invalid
	}
	return nil
}
