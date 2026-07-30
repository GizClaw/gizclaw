package gameplay

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type testPixaClip struct {
	name       string
	firstFrame uint32
	frameCount uint32
}

type testPixaFrame struct {
	frameType uint8
	encoding  uint8
	payload   []byte
}

func TestValidatePetDefPixaAcceptsPaletteRLEWithTransparentBorder(t *testing.T) {
	data := makePixaFixture(t, 4, 4, []uint16{0, 0x07e0},
		[]testPixaClip{
			{name: "default", firstFrame: 0, frameCount: 1},
			{name: "bath", firstFrame: 0, frameCount: 1},
		},
		[]testPixaFrame{{
			frameType: 0,
			encoding:  1,
			payload:   paletteRLE([]byte{0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0}),
		}},
	)
	if err := validatePetDefPixa(data, petDefPixaMetadata(4, 4)); err != nil {
		t.Fatalf("validatePetDefPixa() error = %v", err)
	}
}

func TestValidatePetDefPixaRejectsLegalRGB565PetSprite(t *testing.T) {
	payload := make([]byte, 4*4*2)
	for offset := 0; offset < len(payload); offset += 2 {
		binary.LittleEndian.PutUint16(payload[offset:], 0x07e0)
	}
	data := makePixaFixture(t, 4, 4, nil,
		[]testPixaClip{{name: "default", firstFrame: 0, frameCount: 1}},
		[]testPixaFrame{{frameType: 0, encoding: 2, payload: payload}},
	)
	err := validatePetDefPixa(data, petDefPixaMetadataForClips(4, 4, "default"))
	if err == nil || !strings.Contains(err.Error(), `clip "default" local frame 0`) ||
		!strings.Contains(err.Error(), "outer border") {
		t.Fatalf("validatePetDefPixa() error = %v, want transparent-border rejection with frame context", err)
	}
}

func TestValidatePetDefPixaRejectsInvalidDecodedFrames(t *testing.T) {
	key := testPixaFrame{
		frameType: 0,
		encoding:  1,
		payload:   paletteRLE([]byte{0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0}),
	}
	tests := []struct {
		name       string
		frames     []testPixaFrame
		wantFrame  string
		wantReason string
	}{
		{
			name: "opaque border",
			frames: []testPixaFrame{{
				frameType: 0,
				encoding:  1,
				payload:   paletteRLE([]byte{1, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0}),
			}},
			wantFrame:  "local frame 0",
			wantReason: "outer border",
		},
		{
			name: "fully transparent",
			frames: []testPixaFrame{{
				frameType: 0,
				encoding:  1,
				payload:   paletteRLE(make([]byte, 16)),
			}},
			wantFrame:  "local frame 0",
			wantReason: "fully transparent",
		},
		{
			name: "diff makes border opaque",
			frames: []testPixaFrame{
				key,
				{frameType: 1, payload: diffPayload(0, 0, 1, 1, []byte{1, 1})},
			},
			wantFrame:  "local frame 1",
			wantReason: "outer border",
		},
		{
			name: "diff clears content",
			frames: []testPixaFrame{
				key,
				{frameType: 1, payload: diffPayload(1, 1, 2, 2, []byte{4, 0})},
			},
			wantFrame:  "local frame 1",
			wantReason: "fully transparent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makePixaFixture(t, 4, 4, []uint16{0, 0x07e0},
				[]testPixaClip{{name: "default", firstFrame: 0, frameCount: uint32(len(tt.frames))}},
				tt.frames,
			)
			err := validatePetDefPixa(data, petDefPixaMetadataForClips(4, 4, "default"))
			if err == nil || !strings.Contains(err.Error(), `clip "default"`) ||
				!strings.Contains(err.Error(), tt.wantFrame) || !strings.Contains(err.Error(), tt.wantReason) {
				t.Fatalf("validatePetDefPixa() error = %v, want clip/frame context and %q", err, tt.wantReason)
			}
		})
	}
}

func TestValidatePetDefPixaPropagatesUpstreamDecodeErrors(t *testing.T) {
	tests := []struct {
		name  string
		frame testPixaFrame
		want  string
	}{
		{
			name:  "unsupported encoding",
			frame: testPixaFrame{frameType: 0, encoding: 9},
			want:  "unsupported key encoding 9",
		},
		{
			name:  "invalid palette index",
			frame: testPixaFrame{frameType: 0, encoding: 1, payload: []byte{16, 2}},
			want:  "palette index 2 exceeds color count 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makePixaFixture(t, 4, 4, []uint16{0, 0x07e0},
				[]testPixaClip{{name: "default", firstFrame: 0, frameCount: 1}},
				[]testPixaFrame{tt.frame},
			)
			err := validatePetDefPixa(data, petDefPixaMetadataForClips(4, 4, "default"))
			if err == nil || !strings.Contains(err.Error(), "clip 0 frame 0") || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validatePetDefPixa() error = %v, want upstream frame error %q", err, tt.want)
			}
		})
	}
}

func TestValidatePetDefPixaMatchesMetadata(t *testing.T) {
	data := makePixaFixture(t, 4, 4, []uint16{0, 0x07e0},
		[]testPixaClip{{name: "default", firstFrame: 0, frameCount: 1}},
		[]testPixaFrame{{frameType: 0, encoding: 1, payload: paletteRLE([]byte{0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0})}},
	)
	t.Run("canvas", func(t *testing.T) {
		err := validatePetDefPixa(data, petDefPixaMetadataForClips(5, 4, "default"))
		if err == nil || !strings.Contains(err.Error(), "canvas is 4x4, want 5x4") {
			t.Fatalf("validatePetDefPixa() error = %v", err)
		}
	})
	t.Run("clip", func(t *testing.T) {
		err := validatePetDefPixa(data, petDefPixaMetadataForClips(4, 4, "missing"))
		if err == nil || !strings.Contains(err.Error(), `missing metadata clip "missing"`) {
			t.Fatalf("validatePetDefPixa() error = %v", err)
		}
	})
}

func TestValidatePetDefPixaChecksCanvasLimitBeforeAllocation(t *testing.T) {
	data := makePixaFixture(t, 65535, 65535, []uint16{0},
		[]testPixaClip{{name: "default", firstFrame: 0, frameCount: 1}},
		[]testPixaFrame{{frameType: 0, encoding: 1}},
	)
	err := validatePetDefPixa(data, petDefPixaMetadataForClips(65535, 65535, "default"))
	if err == nil || !strings.Contains(err.Error(), "decoded canvas requires 17179344900 bytes") ||
		!strings.Contains(err.Error(), "limit is 16777216") {
		t.Fatalf("validatePetDefPixa() error = %v, want resource-limit rejection", err)
	}
}

func petDefPixaMetadata(width, height int64) apitypes.PetDefPixaMetadata {
	return petDefPixaMetadataForClips(width, height, "default", "bath")
}

func petDefPixaMetadataForClips(width, height int64, clips ...string) apitypes.PetDefPixaMetadata {
	metadata := apitypes.PetDefPixaMetadata{
		Version: "1",
		Canvas:  apitypes.PetDefPixaCanvasMetadata{Width: width, Height: height},
		Clips:   make([]apitypes.PetDefPixaClipMetadata, len(clips)),
	}
	for i, clip := range clips {
		metadata.Clips[i] = apitypes.PetDefPixaClipMetadata{Id: clip, PixaClipName: clip}
	}
	return metadata
}

func makePixaFixture(
	t *testing.T,
	width, height uint16,
	palette []uint16,
	clips []testPixaClip,
	frames []testPixaFrame,
) []byte {
	t.Helper()
	const (
		headerSize     = 40
		clipEntrySize  = 56
		frameEntrySize = 16
	)
	paletteOffset := headerSize
	clipOffset := paletteOffset + len(palette)*2
	frameOffset := clipOffset + len(clips)*clipEntrySize
	payloadOffset := frameOffset + len(frames)*frameEntrySize
	payloadLength := 0
	for _, frame := range frames {
		payloadLength += len(frame.payload)
	}
	data := make([]byte, payloadOffset+payloadLength)
	copy(data[:4], "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], width)
	binary.LittleEndian.PutUint16(data[10:12], height)
	binary.LittleEndian.PutUint16(data[12:14], uint16(len(palette)))
	binary.LittleEndian.PutUint16(data[14:16], uint16(len(clips)))
	binary.LittleEndian.PutUint32(data[16:20], uint32(len(frames)))
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], uint32(payloadLength))
	for i, color := range palette {
		binary.LittleEndian.PutUint16(data[paletteOffset+i*2:], color)
	}
	for i, clip := range clips {
		if len(clip.name) >= 32 {
			t.Fatalf("clip name %q is too long", clip.name)
		}
		base := clipOffset + i*clipEntrySize
		copy(data[base:base+32], clip.name)
		binary.LittleEndian.PutUint32(data[base+36:base+40], clip.firstFrame)
		binary.LittleEndian.PutUint32(data[base+40:base+44], clip.frameCount)
	}
	nextPayloadOffset := 0
	for i, frame := range frames {
		base := frameOffset + i*frameEntrySize
		data[base+2] = frame.frameType
		data[base+3] = frame.encoding
		binary.LittleEndian.PutUint32(data[base+4:base+8], uint32(nextPayloadOffset))
		binary.LittleEndian.PutUint32(data[base+8:base+12], uint32(len(frame.payload)))
		copy(data[payloadOffset+nextPayloadOffset:], frame.payload)
		nextPayloadOffset += len(frame.payload)
	}
	return data
}

func paletteRLE(indices []byte) []byte {
	var encoded []byte
	for start := 0; start < len(indices); {
		end := start + 1
		for end < len(indices) && indices[end] == indices[start] && end-start < 255 {
			end++
		}
		encoded = append(encoded, byte(end-start), indices[start])
		start = end
	}
	return encoded
}

func diffPayload(x, y, width, height uint16, rle []byte) []byte {
	payload := make([]byte, 13+len(rle))
	payload[0] = 1
	binary.LittleEndian.PutUint16(payload[1:3], x)
	binary.LittleEndian.PutUint16(payload[3:5], y)
	binary.LittleEndian.PutUint16(payload[5:7], width)
	binary.LittleEndian.PutUint16(payload[7:9], height)
	binary.LittleEndian.PutUint32(payload[9:13], uint32(len(rle)))
	copy(payload[13:], rle)
	return payload
}
