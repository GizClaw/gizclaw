package iconasset

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestReadValidated(t *testing.T) {
	t.Parallel()
	pngBytes := testPNG(t)
	if _, err := ReadValidated(bytes.NewReader(pngBytes), FormatPNG); err != nil {
		t.Fatalf("ReadValidated(PNG) error = %v", err)
	}
	if _, err := ReadValidated(bytes.NewReader(testPIXA()), FormatPixa); err != nil {
		t.Fatalf("ReadValidated(PIXA) error = %v", err)
	}
	if _, err := ReadValidated(strings.NewReader("not png"), FormatPNG); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ReadValidated(invalid PNG) error = %v", err)
	}
	exact := append(pngBytes, make([]byte, int(MaxBytes)-len(pngBytes))...)
	if _, err := ReadValidated(bytes.NewReader(exact), FormatPNG); err != nil {
		t.Fatalf("ReadValidated(2 MiB) error = %v", err)
	}
	if _, err := ReadValidated(bytes.NewReader(append(exact, 0)), FormatPNG); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ReadValidated(2 MiB + 1) error = %v", err)
	}
}

func TestReadValidatedRejectsOversizedPNGDimensions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "dimension", width: maxPNGDimension + 1, height: 1},
		{name: "pixel count", width: 2049, height: 2048},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data := encodedGrayPNG(t, tt.width, tt.height)
			if _, err := ReadValidated(bytes.NewReader(data), FormatPNG); !errors.Is(err, ErrInvalid) {
				t.Fatalf("ReadValidated(%dx%d PNG) error = %v, want ErrInvalid", tt.width, tt.height, err)
			}
		})
	}
}

func TestReadValidatedRejectsTruncatedPNG(t *testing.T) {
	t.Parallel()
	pngBytes := testPNG(t)
	truncated := pngBytes[:len(pngBytes)-20]
	if _, err := png.DecodeConfig(bytes.NewReader(truncated)); err != nil {
		t.Fatalf("DecodeConfig(truncated PNG) error = %v, want header to remain valid", err)
	}
	if _, err := ReadValidated(bytes.NewReader(truncated), FormatPNG); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ReadValidated(truncated PNG) error = %v, want ErrInvalid", err)
	}
}

func TestOwnerObjectNames(t *testing.T) {
	t.Parallel()
	if got, want := ObjectName("team/demo", FormatPNG), "team%2Fdemo/icon.png"; got != want {
		t.Fatalf("ObjectName() = %q, want %q", got, want)
	}
	if got, want := GameDefObjectName("game one", FormatPixa), "game-defs/game%20one/icon.pixa"; got != want {
		t.Fatalf("GameDefObjectName() = %q, want %q", got, want)
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func encodedGrayPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func testPIXA() []byte {
	const headerSize = 40
	const clipSize = 56
	const frameSize = 16
	paletteOffset := headerSize
	clipOffset := paletteOffset + 2
	frameOffset := clipOffset + clipSize
	payloadOffset := frameOffset + frameSize
	data := make([]byte, payloadOffset+2)
	copy(data, "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], 1)
	binary.LittleEndian.PutUint16(data[10:12], 1)
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], 1)
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], 2)
	binary.LittleEndian.PutUint32(data[clipOffset+40:clipOffset+44], 1)
	binary.LittleEndian.PutUint32(data[frameOffset+8:frameOffset+12], 2)
	return data
}
