package app

import (
	"archive/tar"
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/toolkittest"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func archive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	tw := tar.NewWriter(zw)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func validFiles() map[string][]byte {
	return map[string][]byte{
		"app.json":         []byte(`{"app_name":"desk_clock","runtime":"runtime.lua.gizos","entry":"main.lua","methods":[{"name":"get_alarm","mode":"call","description":"Read alarm","input_schema":{"type":"object"}}]}`),
		"main.lua":         []byte(`return {get_alarm=function(args) return {hour=8} end}`),
		"assets/image.bin": {0, 1, 2},
	}
}

func TestPackageValidation(t *testing.T) {
	value, err := Parse(archive(t, validFiles()))
	if err != nil || value.AppName != "desk_clock" {
		t.Fatalf("valid package: %v %v", value, err)
	}
	for _, path := range []string{"/absolute", "a//b", "a/../b", "./main.lua", `a\b`} {
		t.Run(path, func(t *testing.T) {
			files := validFiles()
			files[path] = []byte("x")
			if _, err := Parse(archive(t, files)); err == nil {
				t.Fatal("unsafe path accepted")
			}
		})
	}
	tests := map[string]func(map[string][]byte){
		"bytecode":         func(f map[string][]byte) { f["main.lua"] = []byte{0x1b, 'L', 'u', 'a'} },
		"missing entry":    func(f map[string][]byte) { delete(f, "main.lua") },
		"missing manifest": func(f map[string][]byte) { delete(f, "app.json") },
		"unknown field": func(f map[string][]byte) {
			f["app.json"] = bytes.Replace(f["app.json"], []byte(`"app_name"`), []byte(`"enabled":true,"app_name"`), 1)
		},
		"invalid method": func(f map[string][]byte) {
			f["app.json"] = bytes.ReplaceAll(f["app.json"], []byte(`"call"`), []byte(`"stream"`))
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			files := validFiles()
			mutate(files)
			if _, err := Parse(archive(t, files)); err == nil {
				t.Fatal("invalid package accepted")
			}
		})
	}
}

func TestFetchVerifiesArchiveIdentity(t *testing.T) {
	data := archive(t, validFiles())
	sum := sha256.Sum256(data)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	pkg := apitypes.FirmwarePackage{Url: server.URL, Sha256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	if _, err := Fetch(t.Context(), server.Client(), pkg); err != nil {
		t.Fatal(err)
	}
	pkg.Size++
	if _, err := Fetch(t.Context(), server.Client(), pkg); err == nil {
		t.Fatal("size mismatch accepted")
	}
	pkg.Size--
	pkg.Sha256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if _, err := Fetch(t.Context(), server.Client(), pkg); err == nil {
		t.Fatal("hash mismatch accepted")
	}
}

func TestUnpackedLimit(t *testing.T) {
	files := validFiles()
	files["bomb.bin"] = make([]byte, MaxUnpackedBytes+1)
	if _, err := Parse(archive(t, files)); err == nil {
		t.Fatal("oversized unpacked package accepted")
	}
}

func TestRejectSpecialEntriesAndFileCount(t *testing.T) {
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeFifo, tar.TypeChar} {
		var out bytes.Buffer
		zw := zlib.NewWriter(&out)
		tw := tar.NewWriter(zw)
		if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: kind, Linkname: "main.lua"}); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(out.Bytes()); err == nil {
			t.Fatalf("accepted entry type %v", kind)
		}
	}
	files := validFiles()
	for i := range MaxFiles {
		files[fmt.Sprintf("asset%d", i)] = nil
	}
	if _, err := Parse(archive(t, files)); err == nil {
		t.Fatal("file count limit not enforced")
	}
}

func TestCapabilityRequirements(t *testing.T) {
	many := make([]string, 65)
	for i := range many {
		many[i] = fmt.Sprintf("host.cap%d", i)
	}
	manyJSON, err := json.Marshal(many)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, raw string
		valid     bool
	}{
		{"empty", "[]", true},
		{"namespaced", `["litelink.notify","host.audio_play","host.ui.display"]`, true},
		{"duplicate", `["host.notify","host.notify"]`, false},
		{"unnamespaced", `["notify"]`, false},
		{"uppercase", `["Host.notify"]`, false},
		{"empty segment", `["host..notify"]`, false},
		{"hyphen", `["host.audio-play"]`, false},
		{"null", "null", false},
		{"null item", "[null]", false},
		{"too many", string(manyJSON), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := validFiles()
			files["app.json"] = bytes.Replace(files["app.json"], []byte(`"app_name"`), []byte(`"requires":`+tc.raw+`,"app_name"`), 1)
			value, err := Parse(archive(t, files))
			if (err == nil) != tc.valid {
				t.Fatalf("Parse: %v", err)
			}
			if tc.valid && value.Requires == nil {
				t.Fatal("requirements not retained")
			}
		})
	}
}

func TestStoreRetainsCapabilityRequirements(t *testing.T) {
	files := validFiles()
	files["app.json"] = bytes.Replace(files["app.json"], []byte(`"app_name"`), []byte(`"requires":["litelink.notify"],"app_name"`), 1)
	data := archive(t, files)
	sum := sha256.Sum256(data)
	transport := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) }))
	defer transport.Close()
	catalog := toolkittest.New(t)
	server := &Server{DB: catalog.DB, HTTP: transport.Client()}
	if err := server.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	spec := apitypes.AppSpec{Package: apitypes.FirmwarePackage{Url: transport.URL, Sha256: hex.EncodeToString(sum[:]), Size: int64(len(data))}}
	if _, err := server.Put(t.Context(), "clock", spec, true); err != nil {
		t.Fatal(err)
	}
	value, err := server.Get(t.Context(), "clock")
	if err != nil {
		t.Fatal(err)
	}
	if value.Requires == nil || !slices.Equal(*value.Requires, []string{"litelink.notify"}) {
		t.Fatalf("stored requirements: %v", value.Requires)
	}
}
