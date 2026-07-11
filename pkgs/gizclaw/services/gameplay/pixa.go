package gameplay

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const (
	pixaMagic          = "PIXA"
	pixaHeaderSize     = 40
	pixaClipEntrySize  = 56
	pixaFrameEntrySize = 16
	pixaClipNameSize   = 32
	pixaVersion        = 1
	pixaMaxCanvasSize  = 1<<16 - 1
)

type pixaAsset struct {
	width         uint16
	height        uint16
	clipCount     uint16
	frameCount    uint32
	payloadLength uint32
	clips         []pixaClip
	frames        []pixaFrame
}

type pixaClip struct {
	name       string
	firstFrame uint32
	frameCount uint32
}

type pixaFrame struct {
	typeCode      uint8
	payloadOffset uint32
	payloadLength uint32
}

func validatePetDefPixa(data []byte, metadata apitypes.PetDefPixaMetadata) error {
	asset, err := parsePixa(data)
	if err != nil {
		return err
	}
	if asset.clipCount == 0 || asset.frameCount == 0 {
		return errors.New("petdef pixa must contain at least one clip and one frame")
	}
	if int64(asset.width) != metadata.Canvas.Width || int64(asset.height) != metadata.Canvas.Height {
		return fmt.Errorf("petdef pixa canvas is %dx%d, want %dx%d", asset.width, asset.height, metadata.Canvas.Width, metadata.Canvas.Height)
	}
	for i, clip := range metadata.Clips {
		if _, ok := asset.clipByName(clip.PixaClipName); !ok {
			return fmt.Errorf("petdef pixa is missing metadata clip %q at visual.pixa.metadata.clips[%d].pixa_clip_name", clip.PixaClipName, i)
		}
	}
	return nil
}

func validateBadgeDefPixa(data []byte) error {
	asset, err := parsePixa(data)
	if err != nil {
		return err
	}
	clip, ok := asset.clipByName("icon")
	if !ok {
		return errors.New(`badgedef pixa must contain an "icon" clip`)
	}
	if clip.frameCount != 1 {
		return fmt.Errorf("badgedef icon clip must contain exactly one frame, got %d", clip.frameCount)
	}
	if clip.firstFrame >= uint32(len(asset.frames)) {
		return errors.New("badgedef icon clip references a missing frame")
	}
	if asset.frames[clip.firstFrame].typeCode != 0 {
		return errors.New("badgedef icon frame must be a key frame")
	}
	return nil
}

func parsePixa(data []byte) (pixaAsset, error) {
	if len(data) < pixaHeaderSize {
		return pixaAsset{}, errors.New("invalid PIXA file: header is too short")
	}
	if string(data[:4]) != pixaMagic {
		return pixaAsset{}, errors.New("invalid PIXA magic")
	}
	version := binary.LittleEndian.Uint16(data[4:6])
	if version != pixaVersion {
		return pixaAsset{}, fmt.Errorf("unsupported PIXA version %d", version)
	}
	headerSize := binary.LittleEndian.Uint16(data[6:8])
	if headerSize != pixaHeaderSize {
		return pixaAsset{}, fmt.Errorf("invalid PIXA header size %d", headerSize)
	}
	asset := pixaAsset{
		width:         binary.LittleEndian.Uint16(data[8:10]),
		height:        binary.LittleEndian.Uint16(data[10:12]),
		clipCount:     binary.LittleEndian.Uint16(data[14:16]),
		frameCount:    binary.LittleEndian.Uint32(data[16:20]),
		payloadLength: binary.LittleEndian.Uint32(data[36:40]),
	}
	if asset.width == 0 || asset.height == 0 {
		return pixaAsset{}, errors.New("invalid PIXA canvas size")
	}

	colorCount := binary.LittleEndian.Uint16(data[12:14])
	paletteOffset := binary.LittleEndian.Uint32(data[20:24])
	clipOffset := binary.LittleEndian.Uint32(data[24:28])
	frameOffset := binary.LittleEndian.Uint32(data[28:32])
	payloadOffset := binary.LittleEndian.Uint32(data[32:36])

	if err := requirePixaRange(len(data), paletteOffset, uint64(colorCount)*2, "palette"); err != nil {
		return pixaAsset{}, err
	}
	if err := requirePixaRange(len(data), clipOffset, uint64(asset.clipCount)*pixaClipEntrySize, "clip table"); err != nil {
		return pixaAsset{}, err
	}
	if err := requirePixaRange(len(data), frameOffset, uint64(asset.frameCount)*pixaFrameEntrySize, "frame table"); err != nil {
		return pixaAsset{}, err
	}
	if err := requirePixaRange(len(data), payloadOffset, uint64(asset.payloadLength), "payload"); err != nil {
		return pixaAsset{}, err
	}

	clips, err := parsePixaClips(data, clipOffset, asset.clipCount, asset.frameCount)
	if err != nil {
		return pixaAsset{}, err
	}
	frames, err := parsePixaFrames(data, frameOffset, asset.frameCount, asset.payloadLength)
	if err != nil {
		return pixaAsset{}, err
	}
	asset.clips = clips
	asset.frames = frames
	return asset, nil
}

func parsePixaClips(data []byte, clipOffset uint32, clipCount uint16, frameCount uint32) ([]pixaClip, error) {
	clips := make([]pixaClip, 0, clipCount)
	for i := uint32(0); i < uint32(clipCount); i++ {
		base := int(clipOffset) + int(i)*pixaClipEntrySize
		firstFrame := binary.LittleEndian.Uint32(data[base+36 : base+40])
		clipFrameCount := binary.LittleEndian.Uint32(data[base+40 : base+44])
		if firstFrame > frameCount || clipFrameCount > frameCount-firstFrame {
			return nil, errors.New("invalid PIXA clip frame range")
		}
		if clipFrameCount == 0 {
			return nil, errors.New("invalid PIXA empty clip")
		}
		clips = append(clips, pixaClip{
			name:       readPixaName(data[base : base+pixaClipNameSize]),
			firstFrame: firstFrame,
			frameCount: clipFrameCount,
		})
	}
	return clips, nil
}

func parsePixaFrames(data []byte, frameOffset uint32, frameCount uint32, payloadLength uint32) ([]pixaFrame, error) {
	frames := make([]pixaFrame, 0, frameCount)
	for i := uint32(0); i < frameCount; i++ {
		base := int(frameOffset) + int(i)*pixaFrameEntrySize
		payloadOffset := binary.LittleEndian.Uint32(data[base+4 : base+8])
		framePayloadLength := binary.LittleEndian.Uint32(data[base+8 : base+12])
		if payloadOffset > payloadLength || framePayloadLength > payloadLength-payloadOffset {
			return nil, errors.New("invalid PIXA frame payload range")
		}
		frames = append(frames, pixaFrame{
			typeCode:      data[base+2],
			payloadOffset: payloadOffset,
			payloadLength: framePayloadLength,
		})
	}
	return frames, nil
}

func (a pixaAsset) clipByName(name string) (pixaClip, bool) {
	for _, clip := range a.clips {
		if clip.name == name {
			return clip, true
		}
	}
	return pixaClip{}, false
}

func requirePixaRange(fileLength int, offset uint32, length uint64, label string) error {
	if uint64(offset) > uint64(fileLength) || length > uint64(fileLength)-uint64(offset) {
		return fmt.Errorf("invalid PIXA %s range", label)
	}
	return nil
}

func readPixaName(data []byte) string {
	if i := bytes.IndexByte(data, 0); i >= 0 {
		data = data[:i]
	}
	return string(data)
}
