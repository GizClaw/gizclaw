package testdata_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

type workflowNodePublication struct {
	ID      string `json:"id" yaml:"id"`
	Publish *bool  `json:"publish" yaml:"publish"`
}

var workflowFixtureFiles = []string{
	"00-ast-translate-tts.yaml",
	"01-ast-translate-zh-jp.yaml",
	"02-ast-translate.yaml",
	"03-chatroom.yaml",
	"04-doubao-realtime.yaml",
	"05-flowcraft-basic.yaml",
	"06-flowcraft-chat.yaml",
	"07-flowcraft-func-chat.yaml",
	"08-flowcraft-journey.yaml",
	"09-flowcraft-match-route.yaml",
	"10-flowcraft-multi-role-storyteller.yaml",
	"11-flowcraft-murder-mystery.yaml",
	"12-flowcraft-poetry-adventure-li-bai.yaml",
	"13-flowcraft-werewolf.yaml",
	"14-ast-translate-zh-en.yaml",
	"20-flowcraft-assistant.yaml",
	"21-flowcraft-support.yaml",
	"22-chatroom-direct.yaml",
	"23-pet-care.yaml",
	"30-family-circle-chatroom.yaml",
}

type workflowFixture struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	I18n workflowI18n   `yaml:"i18n"`
	Icon map[string]any `yaml:"icon"`
}

type workflowI18n struct {
	DefaultLocale string          `yaml:"default_locale"`
	En            workflowCatalog `yaml:"en"`
	ZhCN          workflowCatalog `yaml:"zh-CN"`
}

type workflowCatalog struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func TestWorkflowCatalogFixtures(t *testing.T) {
	workflowDir := filepath.Join("resources", "04-workflows")
	assetDir := filepath.Join("assets", "workflows")
	pngHashes := map[[sha256.Size]byte]string{}
	pixaHashes := map[[sha256.Size]byte]string{}
	for _, filename := range workflowFixtureFiles {
		filename := filename
		t.Run(filename, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(workflowDir, filename))
			if err != nil {
				t.Fatal(err)
			}
			var fixture workflowFixture
			if err := yaml.Unmarshal(raw, &fixture); err != nil {
				t.Fatal(err)
			}
			if fixture.Kind != "Workflow" || fixture.Metadata.Name == "" {
				t.Fatalf("fixture identity = kind %q name %q", fixture.Kind, fixture.Metadata.Name)
			}
			if fixture.Icon != nil {
				t.Fatalf("fixture YAML must not inject icon object names: %#v", fixture.Icon)
			}
			if fixture.I18n.DefaultLocale != "en" {
				t.Fatalf("default locale = %q, want en", fixture.I18n.DefaultLocale)
			}
			validateCatalog(t, "en", fixture.I18n.En)
			validateCatalog(t, "zh-CN", fixture.I18n.ZhCN)

			pngPath := filepath.Join(assetDir, fixture.Metadata.Name, "icon.png")
			pixaPath := filepath.Join(assetDir, fixture.Metadata.Name, "icon.pixa")
			pngBytes := readAsset(t, pngPath)
			pixaBytes := readAsset(t, pixaPath)
			pngImage, err := png.Decode(bytes.NewReader(pngBytes))
			if err != nil {
				t.Fatalf("decode PNG: %v", err)
			}
			bounds := pngImage.Bounds()
			if bounds.Dx() != bounds.Dy() || bounds.Dx() == 0 {
				t.Fatalf("PNG dimensions = %dx%d, want non-empty square", bounds.Dx(), bounds.Dy())
			}
			_, _, _, alpha := pngImage.At(bounds.Min.X, bounds.Min.Y).RGBA()
			if alpha != 0 {
				t.Fatalf("PNG corner alpha = %d, want transparent background", alpha)
			}
			width, height, pixaRGB := validatePIXA(t, pixaBytes)
			if width*2 != bounds.Dx() || height*2 != bounds.Dy() {
				t.Fatalf("PIXA dimensions = %dx%d, PNG = %dx%d", width, height, bounds.Dx(), bounds.Dy())
			}
			pngR, pngG, pngB, _ := pngImage.At(bounds.Dx()/2, bounds.Dy()*3/4).RGBA()
			pixaR, pixaG, pixaB := pixaPixel(pixaRGB, width, width/2, height*3/4)
			if colorDelta(uint8(pngR>>8), pixaR)+colorDelta(uint8(pngG>>8), pixaG)+colorDelta(uint8(pngB>>8), pixaB) > 20 {
				t.Fatal("PNG and PIXA do not preserve the same visual identity color")
			}
			requireDistinctHash(t, pngHashes, sha256.Sum256(pngBytes), fixture.Metadata.Name, "PNG")
			requireDistinctHash(t, pixaHashes, sha256.Sum256(pixaBytes), fixture.Metadata.Name, "PIXA")
		})
	}
}

func TestWorkflowIconProvisioningScriptsUseOwnerAPI(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "setup", "reset-data.sh"),
		filepath.Join("..", "docker", "setup", "reset_data.sh"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		script := string(raw)
		if !strings.Contains(script, `admin workflows upload-icon "$workflow_id" --format "$format"`) {
			t.Fatalf("%s does not provision through the Workflow owner CLI", path)
		}
		for _, forbidden := range []string{"objectstore", "data/objects", "workflow-assets/"} {
			if strings.Contains(script, forbidden) {
				t.Fatalf("%s directly references %q", path, forbidden)
			}
		}
	}
}

func TestWerewolfLifecycleToolNodesAreInternal(t *testing.T) {
	var resource struct {
		Spec struct {
			Flowcraft struct {
				Agent struct {
					Graph struct {
						Nodes []workflowNodePublication `yaml:"nodes"`
					} `yaml:"graph"`
				} `yaml:"agent"`
			} `yaml:"flowcraft"`
		} `yaml:"spec"`
	}
	resourceRaw, err := os.ReadFile(filepath.Join("resources", "04-workflows", "13-flowcraft-werewolf.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(resourceRaw, &resource); err != nil {
		t.Fatal(err)
	}
	assertWerewolfLifecycleNodesInternal(t, "resource", resource.Spec.Flowcraft.Agent.Graph.Nodes)

	var workspace struct {
		Workflow struct {
			Flowcraft struct {
				Agent struct {
					Graph struct {
						Nodes []workflowNodePublication `json:"nodes"`
					} `json:"graph"`
				} `json:"agent"`
			} `json:"flowcraft"`
		} `json:"workflow"`
	}
	workspaceRaw, err := os.ReadFile(filepath.Join("workspaces", "flowcraft-werewolf.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(workspaceRaw, &workspace); err != nil {
		t.Fatal(err)
	}
	assertWerewolfLifecycleNodesInternal(t, "workspace", workspace.Workflow.Flowcraft.Agent.Graph.Nodes)
}

func assertWerewolfLifecycleNodesInternal(t *testing.T, source string, nodes []workflowNodePublication) {
	t.Helper()
	want := map[string]bool{"call_game_event": false, "call_game_over_event": false}
	for _, node := range nodes {
		if _, ok := want[node.ID]; !ok {
			continue
		}
		if node.Publish == nil || *node.Publish {
			t.Fatalf("%s node %s publish = %v, want explicit false", source, node.ID, node.Publish)
		}
		delete(want, node.ID)
	}
	if len(want) != 0 {
		t.Fatalf("%s missing lifecycle nodes: %v", source, want)
	}
}

func TestE2EServerConfigProvidesOwnerAssetStores(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("server-workspace", "config.yaml.template"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Stores map[string]struct {
			Kind    string `yaml:"kind"`
			Storage string `yaml:"storage"`
			Prefix  string `yaml:"prefix"`
		} `yaml:"stores"`
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	wants := map[string]string{
		"gameplay-assets":  "gameplay",
		"peer-assets":      "peers",
		"workspace-assets": "workspaces",
		"workflow-assets":  "workflows",
	}
	for name, prefix := range wants {
		store, ok := config.Stores[name]
		if !ok {
			t.Fatalf("missing owner asset store %q", name)
		}
		if store.Kind != "objectstore" || store.Storage != "local-files" || store.Prefix != prefix {
			t.Fatalf("owner asset store %q = %#v", name, store)
		}
	}
}

func validateCatalog(t *testing.T, locale string, catalog workflowCatalog) {
	t.Helper()
	if strings.TrimSpace(catalog.Name) == "" || strings.TrimSpace(catalog.Description) == "" {
		t.Fatalf("%s catalog must contain name and description: %#v", locale, catalog)
	}
	if locale == "en" {
		value := strings.ToLower(catalog.Name + " " + catalog.Description)
		for _, forbidden := range []string{"raid", "fixture", " fake", " test"} {
			if strings.Contains(value, forbidden) {
				t.Fatalf("%s catalog contains implementation term %q", locale, forbidden)
			}
		}
	}
}

func readAsset(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || len(data) > 2*1024*1024 {
		t.Fatalf("asset %s size = %d", path, len(data))
	}
	return data
}

func validatePIXA(t *testing.T, data []byte) (int, int, []byte) {
	t.Helper()
	if len(data) < 40 || string(data[:4]) != "PIXA" {
		t.Fatal("invalid PIXA header")
	}
	width := int(binary.LittleEndian.Uint16(data[8:10]))
	height := int(binary.LittleEndian.Uint16(data[10:12]))
	clipCount := binary.LittleEndian.Uint16(data[14:16])
	frameCount := binary.LittleEndian.Uint32(data[16:20])
	frameOffset := int(binary.LittleEndian.Uint32(data[28:32]))
	payloadOffset := int(binary.LittleEndian.Uint32(data[32:36]))
	payloadLength := int(binary.LittleEndian.Uint32(data[36:40]))
	if binary.LittleEndian.Uint16(data[4:6]) != 1 || binary.LittleEndian.Uint16(data[6:8]) != 40 || width == 0 || height == 0 || clipCount == 0 || frameCount == 0 {
		t.Fatal("PIXA is not a displayable version 1 asset")
	}
	if frameOffset < 0 || frameOffset+16 > len(data) || payloadOffset < 0 || payloadLength < width*height*2 || payloadOffset+payloadLength > len(data) {
		t.Fatal("PIXA frame or payload range is invalid")
	}
	if data[frameOffset+2] != 0 || int(binary.LittleEndian.Uint32(data[frameOffset+8:frameOffset+12])) < width*height*2 {
		t.Fatal("PIXA first frame must be a complete key frame")
	}
	return width, height, data[payloadOffset : payloadOffset+payloadLength]
}

func pixaPixel(data []byte, width, x, y int) (uint8, uint8, uint8) {
	position := (y*width + x) * 2
	value := binary.LittleEndian.Uint16(data[position : position+2])
	return uint8(((value >> 11) & 0x1f) * 255 / 31), uint8(((value >> 5) & 0x3f) * 255 / 63), uint8((value & 0x1f) * 255 / 31)
}

func colorDelta(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func requireDistinctHash(t *testing.T, seen map[[sha256.Size]byte]string, hash [sha256.Size]byte, name, format string) {
	t.Helper()
	if previous := seen[hash]; previous != "" {
		t.Fatalf("%s duplicates %s asset for %s", name, format, previous)
	}
	seen[hash] = name
}
