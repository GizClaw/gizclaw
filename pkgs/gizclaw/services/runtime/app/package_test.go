package app

import (
	"archive/tar"
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
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
