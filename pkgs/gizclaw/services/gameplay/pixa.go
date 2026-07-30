package gameplay

import (
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	pixa "github.com/GizClaw/pixa/pkgs/go"
)

const (
	// petDefPixaMaxEncodedBytes bounds the untrusted upload before parsing.
	petDefPixaMaxEncodedBytes = 16 << 20
	// The encoded-size limit bounds parser output. These count limits then
	// bound the work performed before VisitClipFramesRGBA starts traversal.
	petDefPixaMaxClips            = 256
	petDefPixaMaxFrames           = 4096
	petDefPixaMaxReferencedFrames = 4096
	// petDefPixaMaxCanvasRGBABytes bounds the one caller-owned RGBA canvas
	// reused while validating every frame.
	petDefPixaMaxCanvasRGBABytes uint64 = 16 << 20
)

func validatePetDefPixa(data []byte, metadata apitypes.PetDefPixaMetadata) error {
	asset, err := pixa.ParseWithLimits(data, pixa.ParseLimits{
		MaxClips:            petDefPixaMaxClips,
		MaxFrames:           petDefPixaMaxFrames,
		MaxReferencedFrames: petDefPixaMaxReferencedFrames,
	})
	if err != nil {
		return err
	}
	if int64(asset.Width) != metadata.Canvas.Width || int64(asset.Height) != metadata.Canvas.Height {
		return fmt.Errorf("petdef pixa canvas is %dx%d, want %dx%d", asset.Width, asset.Height, metadata.Canvas.Width, metadata.Canvas.Height)
	}
	for i, metadataClip := range metadata.Clips {
		if !hasPixaClip(asset, metadataClip.PixaClipName) {
			return fmt.Errorf(
				"petdef pixa is missing metadata clip %q at visual.pixa.metadata.clips[%d].pixa_clip_name",
				metadataClip.PixaClipName,
				i,
			)
		}
	}

	canvasBytes := asset.CanvasRGBABytes()
	if canvasBytes > petDefPixaMaxCanvasRGBABytes {
		return fmt.Errorf(
			"petdef pixa decoded canvas requires %d bytes, limit is %d",
			canvasBytes,
			petDefPixaMaxCanvasRGBABytes,
		)
	}
	canvas := make([]byte, int(canvasBytes))
	return asset.VisitClipFramesRGBA(canvas, func(clipIndex int, localFrame uint32, rgba []byte) error {
		clipName := asset.Clips[clipIndex].Name
		if !pixaFrameHasVisiblePixel(rgba) {
			return fmt.Errorf("petdef clip %q local frame %d is fully transparent", clipName, localFrame)
		}
		if !pixaFrameHasTransparentBorder(rgba, int(asset.Width), int(asset.Height)) {
			return fmt.Errorf("petdef clip %q local frame %d has a visible pixel on the outer border", clipName, localFrame)
		}
		return nil
	})
}

func validateBadgeDefPixa(data []byte) error {
	asset, err := pixa.Parse(data)
	if err != nil {
		return err
	}
	for _, clip := range asset.Clips {
		if clip.Name != "icon" {
			continue
		}
		if clip.FrameCount != 1 {
			return fmt.Errorf("badgedef icon clip must contain exactly one frame, got %d", clip.FrameCount)
		}
		if clip.FirstFrame >= uint32(len(asset.Frames)) {
			return errors.New("badgedef icon clip references a missing frame")
		}
		if asset.Frames[clip.FirstFrame].Type != 0 {
			return errors.New("badgedef icon frame must be a key frame")
		}
		return nil
	}
	return errors.New(`badgedef pixa must contain an "icon" clip`)
}

func hasPixaClip(asset pixa.Asset, name string) bool {
	for _, clip := range asset.Clips {
		if clip.Name == name {
			return true
		}
	}
	return false
}

func pixaFrameHasVisiblePixel(rgba []byte) bool {
	for alpha := 3; alpha < len(rgba); alpha += 4 {
		if rgba[alpha] != 0 {
			return true
		}
	}
	return false
}

func pixaFrameHasTransparentBorder(rgba []byte, width, height int) bool {
	for x := range width {
		if rgba[x*4+3] != 0 || rgba[((height-1)*width+x)*4+3] != 0 {
			return false
		}
	}
	for y := 1; y+1 < height; y++ {
		if rgba[(y*width)*4+3] != 0 || rgba[(y*width+width-1)*4+3] != 0 {
			return false
		}
	}
	return true
}
