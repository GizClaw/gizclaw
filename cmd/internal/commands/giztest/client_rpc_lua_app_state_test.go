package giztestcmd

import (
	"archive/tar"
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
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func luaArchive(t *testing.T, version string, files map[string]string, corruptManifest bool) []byte {
	t.Helper()
	entries := make([]map[string]any, 0, len(files))
	paths := make([]string, 0, len(files))
	for name, content := range files {
		sum := sha256.Sum256([]byte(content))
		digest := hex.EncodeToString(sum[:])
		if corruptManifest {
			digest = strings.Repeat("0", 64)
		}
		entries = append(entries, map[string]any{"path": name, "size": len(content), "sha256": digest})
		paths = append(paths, name)
	}
	var dataDir any
	if len(files) > 1 {
		dataDir = "tetris"
	}
	manifest, err := json.Marshal(map[string]any{"format": 1, "type": "lua-app", "app_id": "tetris", "version": version, "entry": "tetris.lua", "data_dir": dataDir, "compact": false, "files": entries})
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "manifest.json")
	slices.Sort(paths)
	var output bytes.Buffer
	compressed := zlib.NewWriter(&output)
	archive := tar.NewWriter(compressed)
	for _, name := range paths {
		content := []byte(files[name])
		if name == "manifest.json" {
			content = manifest
		}
		if err := archive.WriteHeader(&tar.Header{Name: name, Size: int64(len(content)), Mode: 0644, Format: tar.FormatUSTAR}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func luaFixture(t *testing.T, capacity int64, archives map[string][]byte) *luaAppFixture {
	t.Helper()
	packages := map[string]string{}
	for url, data := range archives {
		packages[url] = base64.StdEncoding.EncodeToString(data)
	}
	fixture, err := newLuaAppFixture(map[string]any{"capacity_bytes": capacity, "packages": packages})
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestLuaAppSimulatorDownloadInstallListRun(t *testing.T) {
	files := map[string]string{"tetris.lua": `return args.mode`, "tetris/data/levels.json": `{"levels":[1,2]}`}
	data := luaArchive(t, "1.2.3", files, false)
	var requests atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		for offset := 0; offset < len(data); offset += 7 {
			_, _ = w.Write(data[offset:min(offset+7, len(data))])
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()
	fixture := luaFixture(t, 4096, nil)
	fixture.client = server.Client()
	sum := sha256.Sum256(data)
	message, err := fixture.invoke(t.Context(), &rpcpb.ClientLuaAppInstallRequest{Url: server.URL, Sha256: new(hex.EncodeToString(sum[:]))})
	if err != nil {
		t.Fatal(err)
	}
	installed := message.(*rpcpb.ClientLuaAppInstallResponse).App
	if installed.AppId != "tetris" || installed.Version != "1.2.3" || requests.Load() != 1 {
		t.Fatalf("installed=%v requests=%d", installed, requests.Load())
	}
	for name, content := range files {
		if string(fixture.files["tetris"][name]) != content {
			t.Fatalf("wrong installed content: %s", name)
		}
	}
	listed, err := fixture.invoke(t.Context(), &rpcpb.ClientLuaAppListRequest{})
	if err != nil || len(listed.(*rpcpb.ClientLuaAppListResponse).Apps) != 1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	params := map[string]string{"mode": "single", "level": "2", "text": "玩俄罗斯方块"}
	_, err = fixture.invoke(t.Context(), &rpcpb.ClientLuaAppRunRequest{AppId: "tetris", Params: params})
	if err != nil || !proto.Equal(fixture.lastRun, &rpcpb.ClientLuaAppRunRequest{AppId: "tetris", Params: params}) {
		t.Fatalf("run=%v err=%v", fixture.lastRun, err)
	}
	params["mode"] = "mutated"
	if fixture.lastRun.Params["mode"] != "single" {
		t.Fatal("launch retained caller's mutable params")
	}
}

func TestLuaAppSimulatorFailuresPreserveInstallation(t *testing.T) {
	good := luaArchive(t, "1.0.0", map[string]string{"tetris.lua": "return 1"}, false)
	badChecksum := luaArchive(t, "2.0.0", map[string]string{"tetris.lua": "return 2"}, true)
	large := luaArchive(t, "2.0.0", map[string]string{"tetris.lua": strings.Repeat(" ", 1000)}, false)
	for name, tc := range map[string]struct {
		archive []byte
		digest  *string
		code    rpcapi.StatusCode
	}{
		"manifest checksum": {badChecksum, nil, rpcapi.StatusCodeInvalidArgument},
		"archive checksum":  {good, new(strings.Repeat("0", 64)), rpcapi.StatusCodeInvalidArgument},
		"truncated trailer": {good[:len(good)-1], nil, rpcapi.StatusCodeInvalidArgument},
		"trailing bytes":    {append(slices.Clone(good), 1), nil, rpcapi.StatusCodeInvalidArgument},
		"storage full":      {large, nil, rpcapi.StatusCodeUnimplemented},
		"path escape":       {luaArchive(t, "2.0.0", map[string]string{"tetris.lua": "return 2", "../other": "oops"}, false), nil, rpcapi.StatusCodeInvalidArgument},
		"bytecode":          {luaArchive(t, "2.0.0", map[string]string{"tetris.lua": "\x1bLua"}, false), nil, rpcapi.StatusCodeInvalidArgument},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := luaFixture(t, 64, map[string][]byte{"https://apps.test/old": good, "https://apps.test/new": tc.archive})
			if _, err := fixture.invoke(t.Context(), &rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/old"}); err != nil {
				t.Fatal(err)
			}
			_, err := fixture.invoke(t.Context(), &rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/new", Sha256: tc.digest})
			status, ok := err.(rpcapi.Error)
			if !ok || status.Code != tc.code {
				t.Fatalf("error=%v want=%v", err, tc.code)
			}
			if fixture.apps["tetris"].Version != "1.0.0" || string(fixture.files["tetris"]["tetris.lua"]) != "return 1" || fixture.used != 8 {
				t.Fatal("failure changed installed app")
			}
		})
	}
}

func TestLuaAppSimulatorRequiresCompleteTarEnding(t *testing.T) {
	old := luaArchive(t, "1.0.0", map[string]string{"tetris.lua": "return 1"}, false)
	// Zero-filled resource blocks are valid payload, not archive terminators.
	valid := luaArchive(t, "2.0.0", map[string]string{"tetris.lua": "return 2", "tetris/zeros.bin": strings.Repeat("\x00", 1024)}, false)
	reader, err := zlib.NewReader(bytes.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if len(raw) < 1024 || !bytes.Equal(raw[len(raw)-1024:], make([]byte, 1024)) {
		t.Fatal("fixture has no canonical tar ending")
	}
	for _, tc := range []struct {
		blocks, padding int
		valid           bool
	}{{0, 0, false}, {1, 0, false}, {2, 0, true}, {2, 512, true}, {2, 1, false}} {
		t.Run(fmt.Sprintf("blocks_%d_padding_%d", tc.blocks, tc.padding), func(t *testing.T) {
			var compressed bytes.Buffer
			writer := zlib.NewWriter(&compressed)
			if _, err := writer.Write(raw[:len(raw)-1024]); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Write(make([]byte, tc.blocks*512+tc.padding)); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			fixture := luaFixture(t, 4096, map[string][]byte{"https://apps.test/old": old, "https://apps.test/new": compressed.Bytes()})
			if _, err := fixture.invoke(t.Context(), &rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/old"}); err != nil {
				t.Fatal(err)
			}
			checksum := sha256.Sum256(compressed.Bytes())
			_, err = fixture.invoke(t.Context(), &rpcpb.ClientLuaAppInstallRequest{Url: "https://apps.test/new", Sha256: new(hex.EncodeToString(checksum[:]))})
			if tc.valid {
				if err != nil || fixture.apps["tetris"].Version != "2.0.0" {
					t.Fatalf("valid archive rejected: %v", err)
				}
				return
			}
			if status, ok := err.(rpcapi.Error); !ok || status.Code != rpcapi.StatusCodeInvalidArgument {
				t.Fatalf("incomplete tar accepted: %v", err)
			}
			if fixture.apps["tetris"].Version != "1.0.0" || string(fixture.files["tetris"]["tetris.lua"]) != "return 1" || fixture.used != 8 {
				t.Fatal("incomplete tar replaced the installed app")
			}
		})
	}
}

func TestLuaAppSimulatorRejectsMissingAppAndCancelledDownload(t *testing.T) {
	fixture := luaFixture(t, 1024, nil)
	_, err := fixture.invoke(t.Context(), &rpcpb.ClientLuaAppRunRequest{AppId: "missing"})
	if status, ok := err.(rpcapi.Error); !ok || status.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("error=%v", err)
	}
	if fixture.lastRun != nil {
		t.Fatal("missing app was launched")
	}
	data := luaArchive(t, "1.0.0", map[string]string{"tetris.lua": "return 1"}, false)
	started, aborted := make(chan struct{}), make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data[:8])
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(aborted)
	}))
	defer server.Close()
	fixture.client = server.Client()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := fixture.invoke(ctx, &rpcpb.ClientLuaAppInstallRequest{Url: server.URL}); done <- err }()
	<-started
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled download succeeded")
	}
	<-aborted
	if len(fixture.apps) != 0 || fixture.used != 0 {
		t.Fatal("cancelled install became visible")
	}
}
