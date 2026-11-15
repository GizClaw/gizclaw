package server

import (
	"path/filepath"
	"strings"
	"testing"

	giztestcmd "github.com/GizClaw/gizclaw-go/cmd/internal/commands/giztest"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestLiteLinkMCPGiztestDocumentsValidate(t *testing.T) {
	root := filepath.Join("..", "..", "..", "tests", "gizclaw-e2e", "testdata", "mcp-litelink")
	command := giztestcmd.NewCmd()
	command.SetArgs([]string{"validate", "-f", filepath.Join(root, "music.giztest.yaml"), "-f", filepath.Join(root, "music-clarification.giztest.yaml"), "-f", filepath.Join(root, "album.giztest.yaml")})
	if err := command.ExecuteContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestLiteLinkAlbumReferencePreservesCompleteCatalogOrder(t *testing.T) {
	data := `{"id":"history.three-kingdoms","work_count":2,"works":[{"id":"episode-b","title":"Second alphabetically, first in catalog","audio":{"url":"https://example.test/b.ogg"}},{"id":"episode-a","title":"First alphabetically, second in catalog","audio":{"url":"https://example.test/a.ogg"}}]}`
	reference, err := liteLinkAlbumReference([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	items := reference["playlist"].(map[string]any)["items"].([]*rpcpb.AudioPlayerItem)
	if reference["count"] != 2 || len(items) != 2 || items[0].GetSourceRef() != "episode-b" || items[1].GetSourceRef() != "episode-a" {
		t.Fatalf("reference lost or reordered episodes: %v", reference)
	}
	if items[0].GetTitle() != "Second alphabetically, first in catalog" || items[1].Url != "https://example.test/a.ogg" {
		t.Fatalf("reference changed playback metadata: %v", items)
	}
	readback := reference["readback"].(map[string]any)["items"].([]*rpcpb.AudioPlayerItem)
	if len(readback) != len(items) || readback[1].GetSourceRef() != "episode-a" {
		t.Fatal("readback reference is incomplete")
	}
	for name, invalid := range map[string]string{
		"wrong album":      strings.Replace(data, `"id":"history.three-kingdoms"`, `"id":"other-album"`, 1),
		"truncated album":  strings.Replace(data, `"work_count":2`, `"work_count":3`, 1),
		"repeated episode": strings.Replace(data, `"id":"episode-a"`, `"id":"episode-b"`, 1),
		"missing URL":      strings.Replace(data, `"url":"https://example.test/a.ogg"`, `"url":""`, 1),
		"invalid URL":      strings.Replace(data, `https://example.test/a.ogg`, `http://example.test/a.ogg`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := liteLinkAlbumReference([]byte(invalid)); err == nil {
				t.Fatal("invalid catalog reference accepted")
			}
		})
	}
}
