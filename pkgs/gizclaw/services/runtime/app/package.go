// Package app owns device App packages and their verified manifests.
package app

import (
	"archive/tar"
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const (
	MaxArchiveBytes  = 16 << 20
	MaxUnpackedBytes = 64 << 20
	MaxFiles         = 1024
)

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)
var runtimePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)

func validPath(name string) bool {
	if name == "" || strings.Contains(name, `\`) || (len(name) > 1 && name[1] == ':') {
		return false
	}
	for segment := range strings.SplitSeq(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// Fetch verifies the exact compressed package before inspecting its archive.
func Fetch(ctx context.Context, client *http.Client, pkg apitypes.FirmwarePackage) (apitypes.AppManifest, error) {
	if pkg.Size < 1 || pkg.Size > MaxArchiveBytes {
		return apitypes.AppManifest{}, fmt.Errorf("app: archive size must be between 1 and %d", MaxArchiveBytes)
	}
	digest, err := hex.DecodeString(pkg.Sha256)
	if err != nil || len(digest) != sha256.Size || strings.ToLower(pkg.Sha256) != pkg.Sha256 {
		return apitypes.AppManifest{}, fmt.Errorf("app: invalid sha256")
	}
	u, err := url.Parse(pkg.Url)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return apitypes.AppManifest{}, fmt.Errorf("app: package URL must use HTTPS without userinfo")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pkg.Url, nil)
	if err != nil {
		return apitypes.AppManifest{}, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	bounded := *client
	bounded.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.User != nil {
			return fmt.Errorf("app: invalid redirect")
		}
		if len(via) >= 5 {
			return fmt.Errorf("app: too many redirects")
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(req, via)
		}
		return nil
	}
	response, err := bounded.Do(request)
	if err != nil {
		return apitypes.AppManifest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return apitypes.AppManifest{}, fmt.Errorf("app: package HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, pkg.Size+1))
	if err != nil {
		return apitypes.AppManifest{}, err
	}
	if int64(len(data)) != pkg.Size {
		return apitypes.AppManifest{}, fmt.Errorf("app: archive size mismatch")
	}
	sum := sha256.Sum256(data)
	if !bytes.Equal(sum[:], digest) {
		return apitypes.AppManifest{}, fmt.Errorf("app: archive sha256 mismatch")
	}
	return Parse(data)
}

// Parse validates a bounded tar.zlib archive without writing files to disk.
func Parse(data []byte) (apitypes.AppManifest, error) {
	var manifest apitypes.AppManifest
	if len(data) > MaxArchiveBytes {
		return manifest, fmt.Errorf("app: archive too large")
	}
	compressed := bytes.NewReader(data)
	zr, err := zlib.NewReader(compressed)
	if err != nil {
		return manifest, err
	}
	defer zr.Close()
	unpacked, err := io.ReadAll(io.LimitReader(zr, MaxUnpackedBytes+1))
	if err != nil {
		return manifest, err
	}
	if len(unpacked) > MaxUnpackedBytes || compressed.Len() != 0 {
		return manifest, fmt.Errorf("app: oversized or trailing archive data")
	}
	tr := tar.NewReader(bytes.NewReader(unpacked))
	files := map[string][]byte{}
	seen := map[string]bool{}
	for count := 0; ; count++ {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, err
		}
		if count >= MaxFiles || !validPath(header.Name) || seen[header.Name] {
			return manifest, fmt.Errorf("app: invalid or duplicate archive path %q, or too many files", header.Name)
		}
		seen[header.Name] = true
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || header.Size > MaxUnpackedBytes {
			return manifest, fmt.Errorf("app: unsupported archive entry %q", header.Name)
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return manifest, err
		}
		if strings.HasSuffix(header.Name, ".lua") && (!utf8.Valid(content) || bytes.ContainsRune(content, 0) || bytes.HasPrefix(content, []byte{0x1b})) {
			return manifest, fmt.Errorf("app: Lua source must be text: %s", header.Name)
		}
		files[header.Name] = content
	}
	raw, ok := files["app.json"]
	if !ok || len(raw) > 256<<10 {
		return manifest, fmt.Errorf("app: missing or oversized app.json")
	}
	var required map[string]json.RawMessage
	if err := json.Unmarshal(raw, &required); err != nil {
		return manifest, err
	}
	for _, key := range []string{"app_name", "runtime", "entry", "methods"} {
		if len(required[key]) == 0 || bytes.Equal(required[key], []byte("null")) {
			return manifest, fmt.Errorf("app: missing %s", key)
		}
	}
	var rawMethods []map[string]json.RawMessage
	if err := json.Unmarshal(required["methods"], &rawMethods); err != nil {
		return manifest, err
	}
	for _, method := range rawMethods {
		for _, key := range []string{"name", "mode", "description", "input_schema"} {
			if len(method[key]) == 0 || bytes.Equal(method[key], []byte("null")) {
				return manifest, fmt.Errorf("app: missing method %s", key)
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return manifest, fmt.Errorf("app: trailing manifest data")
	}
	if !namePattern.MatchString(manifest.AppName) || !runtimePattern.MatchString(manifest.Runtime) || !validPath(manifest.Entry) || !strings.HasSuffix(manifest.Entry, ".lua") {
		return manifest, fmt.Errorf("app: invalid manifest identity, runtime or entry")
	}
	if _, ok := files[manifest.Entry]; !ok {
		return manifest, fmt.Errorf("app: entry is missing")
	}
	if manifest.Methods == nil || len(manifest.Methods) > 128 {
		return manifest, fmt.Errorf("app: invalid methods")
	}
	methods := map[string]bool{}
	for _, method := range manifest.Methods {
		if !namePattern.MatchString(method.Name) || methods[method.Name] || (method.Mode != "call" && method.Mode != "job") || utf8.RuneCountInString(method.Description) > 1024 || method.InputSchema.Type != "object" {
			return manifest, fmt.Errorf("app: invalid method %q", method.Name)
		}
		if _, err := method.InputSchema.Resolve(nil); err != nil {
			return manifest, fmt.Errorf("app: method %s schema: %w", method.Name, err)
		}
		methods[method.Name] = true
	}
	return manifest, nil
}
