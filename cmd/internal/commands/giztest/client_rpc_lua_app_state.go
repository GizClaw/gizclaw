package giztestcmd

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

const luaFixtureMaxBytes = 64 << 20

// luaAppFixture is a device protocol simulator. Package files occupy its
// bounded in-memory filesystem; run accepts and records a launch without a Lua
// VM. URL fixtures contain real base64-encoded .lua-app.tar.zlib archives. URLs
// not in that explicit table are fetched over HTTPS with the request context.
type luaAppFixture struct {
	operation chan struct{}
	capacity  int64
	used      int64
	apps      map[string]*rpcpb.LuaAppInfo
	files     map[string]map[string][]byte
	packages  map[string]string
	client    *http.Client
	lastRun   *rpcpb.ClientLuaAppRunRequest
}

func newLuaAppFixture(value any) (*luaAppFixture, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var config struct {
		CapacityBytes int64               `json:"capacity_bytes"`
		Installed     []*rpcpb.LuaAppInfo `json:"installed"`
		Packages      map[string]string   `json:"packages"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("Lua app simulator configuration: %w", err)
	}
	if config.CapacityBytes < 0 || config.CapacityBytes > luaFixtureMaxBytes {
		return nil, fmt.Errorf("Lua app simulator capacity_bytes must be 0..%d", luaFixtureMaxBytes)
	}
	if err := rpcapi.ValidateLuaAppResponse(&rpcpb.ClientLuaAppListResponse{Apps: config.Installed}); err != nil {
		return nil, err
	}
	fixture := &luaAppFixture{
		operation: make(chan struct{}, 1), capacity: config.CapacityBytes,
		apps: make(map[string]*rpcpb.LuaAppInfo), files: make(map[string]map[string][]byte), packages: config.Packages,
		client: &http.Client{Timeout: 110 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many package redirects")
			}
			return rpcapi.ValidateLuaAppRequest(&rpcpb.ClientLuaAppInstallRequest{Url: req.URL.String()})
		}},
	}
	for _, app := range config.Installed {
		fixture.apps[app.AppId] = proto.CloneOf(app)
	}
	for url, archive := range config.Packages {
		if err := rpcapi.ValidateLuaAppRequest(&rpcpb.ClientLuaAppInstallRequest{Url: url}); err != nil {
			return nil, err
		}
		if len(archive) > base64.StdEncoding.EncodedLen(luaFixtureMaxBytes) {
			return nil, fmt.Errorf("Lua app simulator package exceeds its limit")
		}
		if _, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding, strings.NewReader(archive))); err != nil {
			return nil, fmt.Errorf("Lua app simulator package must be base64")
		}
	}
	return fixture, nil
}

func (f *luaAppFixture) invoke(ctx context.Context, message proto.Message) (proto.Message, error) {
	if err := rpcapi.ValidateLuaAppRequest(message); err != nil {
		return nil, rpcapi.Error{Code: rpcapi.StatusCodeInvalidArgument, Message: "invalid Lua app request"}
	}
	// Per-device operations serialize without holding a state mutex over I/O.
	select {
	case f.operation <- struct{}{}:
		defer func() { <-f.operation }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch request := message.(type) {
	case *rpcpb.ClientLuaAppListRequest:
		apps := make([]*rpcpb.LuaAppInfo, 0, len(f.apps))
		for _, app := range f.apps {
			apps = append(apps, proto.CloneOf(app))
		}
		slices.SortFunc(apps, func(a, b *rpcpb.LuaAppInfo) int { return strings.Compare(a.AppId, b.AppId) })
		return &rpcpb.ClientLuaAppListResponse{Apps: apps}, nil
	case *rpcpb.ClientLuaAppInstallRequest:
		return f.install(ctx, request)
	case *rpcpb.ClientLuaAppRunRequest:
		if f.apps[request.AppId] == nil {
			return nil, rpcapi.Error{Code: rpcapi.StatusCodeNotFound, Message: "Lua app is not installed"}
		}
		f.lastRun = proto.CloneOf(request)
		return &rpcpb.ClientLuaAppRunResponse{}, nil
	default:
		return nil, rpcapi.Error{Code: rpcapi.StatusCodeUnimplemented, Message: "unsupported Lua app procedure"}
	}
}

func (f *luaAppFixture) download(ctx context.Context, url string) (io.ReadCloser, error) {
	if encoded, ok := f.packages[url]; ok {
		// Decode incrementally too: this fixture does not require another full
		// compressed-package buffer before installation can start.
		return io.NopCloser(base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("package download returned HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}

type luaPackageManifest struct {
	Format  int     `json:"format"`
	Type    string  `json:"type"`
	AppID   string  `json:"app_id"`
	Version string  `json:"version"`
	Entry   string  `json:"entry"`
	DataDir *string `json:"data_dir"`
	Compact bool    `json:"compact"`
	Files   []struct {
		Path   string `json:"path"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

type luaContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r luaContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func luaPackagePath(name string) bool {
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.Contains(name, "\\") {
		return false
	}
	for part := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(part, ".") {
			return false
		}
	}
	return !strings.ContainsFunc(name, func(r rune) bool { return r < 32 || r == 127 })
}

func (f *luaAppFixture) install(ctx context.Context, request *rpcpb.ClientLuaAppInstallRequest) (proto.Message, error) {
	invalid := rpcapi.Error{Code: rpcapi.StatusCodeInvalidArgument, Message: "invalid Lua app package"}
	noSpace := rpcapi.Error{Code: rpcapi.StatusCodeUnimplemented, Message: "Lua app installation unsupported: insufficient storage"}
	body, err := f.download(ctx, request.Url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	digest := sha256.New()
	compressed := &io.LimitedReader{R: luaContextReader{ctx, body}, N: luaFixtureMaxBytes + 1}
	buffered := bufio.NewReader(io.TeeReader(compressed, digest))
	decoded, err := zlib.NewReader(buffered)
	if err != nil {
		return nil, invalid
	}
	defer decoded.Close()
	// The tar overhead and manifest are bounded separately from payload capacity.
	unpacked := &io.LimitedReader{R: decoded, N: luaFixtureMaxBytes + (256 << 10) + 1}
	archive := tar.NewReader(unpacked)
	staged := map[string][]byte{}
	portable := map[string]bool{}
	var manifestBytes []byte
	var stagedBytes int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil || header.Format != tar.FormatUSTAR || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) || !luaPackagePath(header.Name) || header.Size < 0 || (len(staged) >= 256 && header.Name != "manifest.json") {
			return nil, invalid
		}
		key := strings.ToLower(header.Name)
		if portable[key] {
			return nil, invalid
		}
		portable[key] = true
		if header.Name == "manifest.json" {
			if header.Size > 64<<10 {
				return nil, invalid
			}
		} else {
			if header.Size > f.capacity-f.used-stagedBytes {
				return nil, noSpace
			}
			stagedBytes += header.Size
		}
		content, err := io.ReadAll(archive)
		if err != nil || int64(len(content)) != header.Size {
			return nil, invalid
		}
		if header.Name == "manifest.json" {
			manifestBytes = content
		} else {
			staged[header.Name] = content
		}
	}
	// Reading to zlib EOF checks the trailer. Tar padding must contain zeros;
	// trailing compressed bytes are rejected, including a second zlib stream.
	padding := make([]byte, 4096)
	var zeros [4096]byte
	for {
		n, err := unpacked.Read(padding)
		if !bytes.Equal(padding[:n], zeros[:n]) {
			return nil, invalid
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalid
		}
	}
	if unpacked.N == 0 || compressed.N == 0 {
		return nil, invalid
	}
	if _, err := buffered.ReadByte(); err != io.EOF {
		return nil, invalid
	}
	if request.Sha256 != nil && !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), *request.Sha256) {
		return nil, invalid
	}
	var manifest luaPackageManifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, invalid
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, invalid
	}
	app := &rpcpb.LuaAppInfo{AppId: manifest.AppID, Version: manifest.Version}
	if rpcapi.ValidateLuaAppResponse(&rpcpb.ClientLuaAppInstallResponse{App: app}) != nil || manifest.Format != 1 || manifest.Type != "lua-app" || manifest.Entry != manifest.AppID+".lua" || len(manifest.Files) != len(staged) || len(staged) == 0 {
		return nil, invalid
	}
	if len(staged) > 1 && (manifest.DataDir == nil || *manifest.DataDir != manifest.AppID) || len(staged) == 1 && manifest.DataDir != nil {
		return nil, invalid
	}
	seen := map[string]bool{}
	for _, file := range manifest.Files {
		content, ok := staged[file.Path]
		if !ok || seen[file.Path] || file.Size != int64(len(content)) || (file.Path != manifest.Entry && !strings.HasPrefix(file.Path, manifest.AppID+"/")) {
			return nil, invalid
		}
		seen[file.Path] = true
		for parent := path.Dir(file.Path); parent != "."; parent = path.Dir(parent) {
			if portable[strings.ToLower(parent)] {
				return nil, invalid
			}
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != file.SHA256 {
			return nil, invalid
		}
	}
	entry, ok := staged[manifest.Entry]
	if !ok || bytes.IndexByte(entry, 0) >= 0 || bytes.HasPrefix(entry, []byte("\x1bLua")) {
		return nil, invalid
	}
	if f.apps[app.AppId] == nil && len(f.apps) >= 32 {
		return nil, noSpace
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Only the final commit touches the visible installation. A failed update
	// leaves both the previous files and catalog entry intact.
	if previous := f.apps[app.AppId]; previous != nil {
		app.DisplayName, app.Description = previous.DisplayName, previous.Description
	}
	for _, content := range f.files[app.AppId] {
		f.used -= int64(len(content))
	}
	f.files[app.AppId], f.apps[app.AppId] = staged, app
	f.used += stagedBytes
	return &rpcpb.ClientLuaAppInstallResponse{App: proto.CloneOf(app)}, nil
}
